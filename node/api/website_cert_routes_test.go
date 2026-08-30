// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/todaybin/workmesh-server/node/service"
)

func TestWebsiteCertificateRoutesLifecycle(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	mux := http.NewServeMux()
	security := service.NewWebsiteSecurityService(dataDir)
	registerWebsiteCertificateRoutes(mux, security)

	bad := requestJSON(t, mux, http.MethodPost, "/api/v2/websites/acme", map[string]any{"email": "bad", "type": "letsencrypt", "keyType": "RSA2048"})
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("无效 ACME 参数应返回 400，得到 %d", bad.Code)
	}
	created := requestJSON(t, mux, http.MethodPost, "/api/v2/websites/acme", map[string]any{"email": "admin@example.com", "type": "letsencrypt", "keyType": "RSA2048"})
	if created.Code != http.StatusOK {
		t.Fatalf("创建 ACME 失败: %d %s", created.Code, created.Body.String())
	}
	var acme struct {
		Data struct {
			ID         uint   `json:"id"`
			PrivateKey string `json:"privateKey"`
			EabHmacKey string `json:"eabHmacKey"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &acme); err != nil {
		t.Fatal(err)
	}
	if acme.Data.ID == 0 || acme.Data.PrivateKey != "" || acme.Data.EabHmacKey != "" {
		t.Fatalf("ACME 响应泄漏敏感字段或 ID 无效: %+v", acme.Data)
	}
	duplicate := requestJSON(t, mux, http.MethodPost, "/api/v2/websites/acme", map[string]any{"email": "admin@example.com", "type": "letsencrypt", "keyType": "RSA2048"})
	if duplicate.Code != http.StatusBadRequest {
		t.Fatalf("重复 ACME 应返回 400，得到 %d", duplicate.Code)
	}
	search := requestJSON(t, mux, http.MethodPost, "/api/v2/websites/acme/search", map[string]any{"page": 1, "pageSize": 20, "keyword": "admin"})
	if search.Code != http.StatusOK || !bytes.Contains(search.Body.Bytes(), []byte("admin@example.com")) {
		t.Fatalf("ACME 搜索未返回持久记录: %d %s", search.Code, search.Body.String())
	}

	caResp := requestJSON(t, mux, http.MethodPost, "/api/v2/websites/ca", map[string]any{"name": "test-ca", "commonName": "Test Root", "country": "CN", "organization": "WorkMesh", "keyType": "RSA2048"})
	if caResp.Code != http.StatusOK {
		t.Fatalf("创建 CA 失败: %d %s", caResp.Code, caResp.Body.String())
	}
	var ca struct {
		Data struct {
			ID          uint   `json:"id"`
			Certificate string `json:"certificate"`
			PrivateKey  string `json:"privateKey"`
		} `json:"data"`
	}
	if err := json.Unmarshal(caResp.Body.Bytes(), &ca); err != nil {
		t.Fatal(err)
	}
	if ca.Data.ID == 0 || ca.Data.Certificate == "" || ca.Data.PrivateKey != "" {
		t.Fatalf("CA 响应字段异常或泄漏私钥: %+v", ca.Data)
	}
	certResp := requestJSON(t, mux, http.MethodPost, "/api/v2/websites/ca/obtain", map[string]any{"id": ca.Data.ID, "domains": "example.com\nwww.example.com", "keyType": "RSA2048", "unit": "year", "time": 1})
	if certResp.Code != http.StatusOK {
		t.Fatalf("签发自签证书失败: %d %s", certResp.Code, certResp.Body.String())
	}
	var cert struct {
		Data struct {
			ID            uint   `json:"id"`
			Certificate   string `json:"certificate"`
			PrivateKey    string `json:"privateKey"`
			PrimaryDomain string `json:"primaryDomain"`
		} `json:"data"`
	}
	if err := json.Unmarshal(certResp.Body.Bytes(), &cert); err != nil {
		t.Fatal(err)
	}
	if cert.Data.ID == 0 || cert.Data.Certificate == "" || cert.Data.PrivateKey != "" || cert.Data.PrimaryDomain != "example.com" {
		t.Fatalf("证书返回字段异常: %+v", cert.Data)
	}
	protected := requestJSON(t, mux, http.MethodPost, "/api/v2/websites/ca/del", map[string]any{"id": ca.Data.ID})
	if protected.Code != http.StatusBadRequest {
		t.Fatalf("被证书引用的 CA 不应删除，得到 %d", protected.Code)
	}
	renew := requestJSON(t, mux, http.MethodPost, "/api/v2/websites/ca/renew", map[string]any{"SSLID": cert.Data.ID})
	if renew.Code != http.StatusOK {
		t.Fatalf("证书续期失败: %d %s", renew.Code, renew.Body.String())
	}

	downloadReq := requestJSON(t, mux, http.MethodPost, "/api/v2/websites/ca/download", map[string]any{"id": ca.Data.ID})
	if downloadReq.Code != http.StatusOK || downloadReq.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("CA 下载响应异常: %d %s", downloadReq.Code, downloadReq.Header().Get("Content-Type"))
	}
	reader, err := zip.NewReader(bytes.NewReader(downloadReq.Body.Bytes()), int64(downloadReq.Body.Len()))
	if err != nil {
		t.Fatalf("CA ZIP 无法读取: %v", err)
	}
	seen := map[string]bool{}
	for _, file := range reader.File {
		seen[file.Name] = true
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.ReadAll(rc)
		_ = rc.Close()
	}
	if !seen["ca.crt"] || !seen["ca.key"] {
		t.Fatalf("CA ZIP 缺少证书或私钥: %v", seen)
	}

	// 重建服务验证 JSON 持久化后仍可查询。
	reloaded := service.NewWebsiteSecurityService(filepath.Clean(dataDir))
	total, items := reloaded.ListCA("test-ca", 1, 20)
	if total != 1 || len(items) != 1 || items[0].ID != ca.Data.ID {
		t.Fatalf("CA 重载失败: total=%d items=%+v", total, items)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "website-ca.json")); err != nil {
		t.Fatalf("CA 状态文件不存在: %v", err)
	}
}

func requestJSON(t *testing.T, mux *http.ServeMux, method, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	return resp
}
