package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenRestyModulesContractContainsModulesField(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v2/openresty/modules", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"modules"`) || !strings.Contains(rec.Body.String(), `"dynamicSupported"`) {
		t.Fatalf("unexpected modules contract: %s", rec.Body.String())
	}
}

func TestOpenRestyHTTPSContractSupportsReadAndOperate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(root, "missing-openresty"))
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	read := httptest.NewRecorder()
	mux.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/api/v2/openresty/https", nil))
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"https"`) || !strings.Contains(read.Body.String(), `"sslRejectHandshake"`) {
		t.Fatalf("HTTPS GET contract mismatch: %d %s", read.Code, read.Body.String())
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v2/openresty/https", strings.NewReader(`{"operate":"enable","sslRejectHandshake":true}`))
	request.Header.Set("Content-Type", "application/json")
	updated := httptest.NewRecorder()
	mux.ServeHTTP(updated, request)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"defaultHttps":true`) {
		t.Fatalf("HTTPS operate contract mismatch: %d %s", updated.Code, updated.Body.String())
	}
}

func TestOpenRestyModuleCRUDAndScopeGet(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(root, "missing-openresty"))
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	created := call(http.MethodPost, "/api/v2/openresty/modules", `{"operate":"create","name":"headers-more","enable":true,"script":"ngx_http_headers_more_filter_module","packages":"libpcre3","params":"--with-compat","buildMode":"dynamic","provider":"local","loadOrder":20}`)
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), `"custom":true`) {
		t.Fatalf("module create failed: %d %s", created.Code, created.Body.String())
	}
	modules := call(http.MethodGet, "/api/v2/openresty/modules", "")
	for _, expected := range []string{`"name":"headers-more"`, `"enable":true`, `"script":"ngx_http_headers_more_filter_module"`} {
		if modules.Code != http.StatusOK || !strings.Contains(modules.Body.String(), expected) {
			t.Fatalf("module list missing %s: %d %s", expected, modules.Code, modules.Body.String())
		}
	}
	updated := call(http.MethodPost, "/api/v2/openresty/modules/update", `{"operate":"update","name":"headers-more","enable":false}`)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"loadStatus":"disabled"`) {
		t.Fatalf("module update failed: %d %s", updated.Code, updated.Body.String())
	}
	configPath := filepath.Join(root, "openresty.conf")
	if err := os.WriteFile(configPath, []byte("events {}\nhttp {\n  gzip off;\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	scope := call(http.MethodGet, "/api/v2/openresty/scope?scope=http-per", "")
	if scope.Code != http.StatusOK || !strings.Contains(scope.Body.String(), `"name":"gzip"`) || !strings.Contains(scope.Body.String(), `"params":["off"]`) {
		t.Fatalf("scope GET failed: %d %s", scope.Code, scope.Body.String())
	}
	updatedScope := call(http.MethodPost, "/api/v2/openresty/update", `{"operate":"update","scope":"http-per","params":{"gzip":"on"}}`)
	if updatedScope.Code != http.StatusOK {
		t.Fatalf("legacy scope update failed: %d %s", updatedScope.Code, updatedScope.Body.String())
	}
	updatedConfig, err := os.ReadFile(configPath)
	if err != nil || !strings.Contains(string(updatedConfig), "gzip on;") {
		t.Fatalf("legacy scope update did not change OpenResty config: err=%v content=%s", err, updatedConfig)
	}
	deleted := call(http.MethodPost, "/api/v2/openresty/modules/update", `{"operate":"delete","name":"headers-more"}`)
	if deleted.Code != http.StatusOK || !strings.Contains(deleted.Body.String(), `"modules":[]`) {
		t.Fatalf("module delete failed: %d %s", deleted.Code, deleted.Body.String())
	}
}
