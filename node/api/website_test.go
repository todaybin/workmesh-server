// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebsiteWAFRoutesCRUD(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	create := httptest.NewRequest(http.MethodPost, "/api/v2/websites", bytes.NewBufferString(`{"primaryDomain":"example.com","alias":"站点"}`))
	create.Header.Set("Content-Type", "application/json")
	record := httptest.NewRecorder()
	mux.ServeHTTP(record, create)
	if record.Code != http.StatusOK {
		t.Fatalf("创建网站状态码: %d %s", record.Code, record.Body.String())
	}
	var envelope struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(record.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.ID == 0 {
		t.Fatal("创建响应缺少 ID")
	}
	rulesBody := bytes.NewBufferString(`{"websiteID":1,"name":"阻止管理路径","location":"uri","operator":"contains","value":"/admin","action":"block","enabled":true}`)
	rules := httptest.NewRequest(http.MethodPost, "/api/v2/websites/waf/rules", rulesBody)
	rules.Header.Set("Content-Type", "application/json")
	rulesRecord := httptest.NewRecorder()
	mux.ServeHTTP(rulesRecord, rules)
	if rulesRecord.Code != http.StatusOK {
		t.Fatalf("新增规则状态码: %d %s", rulesRecord.Code, rulesRecord.Body.String())
	}
	get := httptest.NewRequest(http.MethodGet, "/api/v2/websites/waf/sites/1/rules", nil)
	getRecord := httptest.NewRecorder()
	mux.ServeHTTP(getRecord, get)
	if getRecord.Code != http.StatusOK || !bytes.Contains(getRecord.Body.Bytes(), []byte("阻止管理路径")) {
		t.Fatalf("规则查询失败: %d %s", getRecord.Code, getRecord.Body.String())
	}
}

func TestWebsiteAdvancedRoutes(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	created := call(http.MethodPost, "/api/v2/websites", `{"primaryDomain":"advanced.example"}`)
	if created.Code != http.StatusOK {
		t.Fatalf("创建网站失败: %d %s", created.Code, created.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v2/websites/operate", `{"id":1,"operate":"stop"}`); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"stopped"`)) {
		t.Fatalf("网站停止失败: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v2/websites/domains", `{"websiteID":1,"domain":"www.advanced.example","port":443,"ssl":true}`); rec.Code != http.StatusOK {
		t.Fatalf("域名新增失败: %d %s", rec.Code, rec.Body.String())
	}
	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v2/websites/domains/1", nil))
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte("www.advanced.example")) {
		t.Fatalf("域名列表失败: %d %s", list.Code, list.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v2/websites/config/update", `{"websiteID":1,"type":"nginx","config":{"content":"server {}"}}`); rec.Code != http.StatusOK {
		t.Fatalf("配置更新失败: %d %s", rec.Code, rec.Body.String())
	}
	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v2/websites/1/config/nginx", nil))
	if get.Code != http.StatusOK || !bytes.Contains(get.Body.Bytes(), []byte("server {}")) {
		t.Fatalf("配置读取失败: %d %s", get.Code, get.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v2/websites/1/https", `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("HTTPS 更新失败: %d %s", rec.Code, rec.Body.String())
	}
	getHTTPS := httptest.NewRecorder()
	mux.ServeHTTP(getHTTPS, httptest.NewRequest(http.MethodGet, "/api/v2/websites/1/https", nil))
	if getHTTPS.Code != http.StatusOK || !bytes.Contains(getHTTPS.Body.Bytes(), []byte(`"enabled":true`)) {
		t.Fatalf("HTTPS 读取失败: %d %s", getHTTPS.Code, getHTTPS.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v2/websites/rewrite/custom", `{"websiteID":1,"config":{"rules":"/old /new"}}`); rec.Code != http.StatusOK {
		t.Fatalf("自定义重写更新失败: %d %s", rec.Code, rec.Body.String())
	}
	rewriteGet := httptest.NewRecorder()
	mux.ServeHTTP(rewriteGet, httptest.NewRequest(http.MethodGet, "/api/v2/websites/rewrite/custom?websiteID=1", nil))
	if rewriteGet.Code != http.StatusOK || !bytes.Contains(rewriteGet.Body.Bytes(), []byte("/old /new")) {
		t.Fatalf("自定义重写读取失败: %d %s", rewriteGet.Code, rewriteGet.Body.String())
	}
}

func TestWebsiteAdvancedRouteValidation(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/websites/domains", bytes.NewBufferString(`{"websiteID":999,"domain":"x.example"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("不存在网站应返回 404，实际 %d: %s", rec.Code, rec.Body.String())
	}
}
