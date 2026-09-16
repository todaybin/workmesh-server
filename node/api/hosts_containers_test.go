// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
)

func TestHostDiagnostics(t *testing.T) {
	mux := http.NewServeMux()
	registerHostRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v2/hosts/diagnostics/summary", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"code":200`) {
		t.Fatalf("unexpected host response: %d %s", res.Code, res.Body.String())
	}
}

func TestRuntimeProfileDownload(t *testing.T) {
	mux := http.NewServeMux()
	registerHostRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/diagnostics/profiles", bytes.NewBufferString(`{"type":"goroutine","duration":1}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("profile status=%d body=%s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Content-Type"); got != "application/gzip" {
		t.Fatalf("content type=%q", got)
	}
	if len(res.Body.Bytes()) < 20 || !bytes.HasPrefix(res.Body.Bytes(), []byte{0x1f, 0x8b}) {
		t.Fatalf("profile is not gzip payload")
	}
}

func TestContainerMethodValidation(t *testing.T) {
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodDelete, "/api/v2/containers/demo", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", res.Code)
	}
}

func TestHostCRUDPersists(t *testing.T) {
	dir := filepath.Join(".tmp", "host-test")
	_ = os.RemoveAll(dir)
	t.Setenv("WORKMESH_DATA_DIR", dir)
	store, err := storage.Open(filepath.Join(dir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := SetSharedStore(store); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetSharedStoreForTest)
	mux := http.NewServeMux()
	registerHostRoutes(mux)
	payload, _ := json.Marshal(map[string]any{"name": "test-host", "address": "127.0.0.1", "port": 22})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/hosts", bytes.NewReader(payload)))
	if res.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", res.Code, res.Body.String())
	}
	if len(loadHosts()) != 1 {
		t.Fatalf("host not persisted")
	}
	search := httptest.NewRecorder()
	mux.ServeHTTP(search, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/search", bytes.NewBufferString(`{"page":1,"pageSize":20}`)))
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), `"total":1`) {
		t.Fatalf("search status=%d body=%s", search.Code, search.Body.String())
	}
}

func TestContainerPathValidation(t *testing.T) {
	if validContainerPath("relative") || validContainerPath("/var/../etc") || validContainerPath("/var/\x00x") {
		t.Fatal("unsafe path accepted")
	}
	if !validContainerPath("/var/lib/app") {
		t.Fatal("valid path rejected")
	}
}

// TestDockerStatusContract 确保状态接口始终返回前端需要的 isExist/isActive 字段，
// 即使测试机未安装 Docker 也不能退化为通用命令结果。
func TestDockerStatusContract(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v2/containers/docker/status", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("docker status code=%d body=%s", res.Code, res.Body.String())
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			IsExist  *bool `json:"isExist"`
			IsActive *bool `json:"isActive"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode docker status: %v", err)
	}
	if envelope.Code != http.StatusOK || envelope.Data.IsExist == nil || envelope.Data.IsActive == nil {
		t.Fatalf("invalid docker status envelope: %s", res.Body.String())
	}
}
