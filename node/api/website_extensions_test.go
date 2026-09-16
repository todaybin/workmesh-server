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
	"testing"
)

func TestWebsiteExtensionTemplatePersists(t *testing.T) {
	root := filepath.Join(".tmp", "website-extension-test")
	_ = os.RemoveAll(root)
	defer os.RemoveAll(root)
	t.Setenv("WORKMESH_DATA_DIR", root)
	mux := http.NewServeMux()
	registerWebsiteExtensionRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/websites/templates", bytes.NewBufferString(`{"name":"demo","content":"hello"}`))
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("create template status=%d body=%s", res.Code, res.Body.String())
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil || envelope.Data["id"] == nil {
		t.Fatalf("invalid template response: %s", res.Body.String())
	}
	mux2 := http.NewServeMux()
	registerWebsiteExtensionRoutes(mux2)
	search := httptest.NewRequest(http.MethodPost, "/api/v2/websites/templates/search", bytes.NewBufferString(`{"page":1,"pageSize":20}`))
	searchRes := httptest.NewRecorder()
	mux2.ServeHTTP(searchRes, search)
	if searchRes.Code != http.StatusOK || !bytes.Contains(searchRes.Body.Bytes(), []byte(`"demo"`)) {
		t.Fatalf("persisted template missing: %s", searchRes.Body.String())
	}
}

func TestWebsiteExtensionRejectsUnsafeComposerPath(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", filepath.Join(".tmp", "website-extension-security-test"))
	mux := http.NewServeMux()
	registerWebsiteExtensionRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/websites/exec/composer", bytes.NewBufferString(`{"path":""}`))
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d", res.Code)
	}
}

func TestWebsiteDatabasesReturnsOnlyRealRecords(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteExtensionRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v2/websites/databases", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("database list status=%d body=%s", res.Code, res.Body.String())
	}
	var envelope struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid database response: %v body=%s", err, res.Body.String())
	}
	if envelope.Data == nil {
		t.Fatalf("empty database list must be [] rather than null: %s", res.Body.String())
	}
	if bytes.Contains(res.Body.Bytes(), []byte("website_extension_state")) {
		t.Fatalf("database list leaked extension state: %s", res.Body.String())
	}
}
