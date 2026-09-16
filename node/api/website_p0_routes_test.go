// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// TestWebsiteP0StaticDomainHTTPSAndOpenRestyRoutes verifies routes that static
// fallback scans can mistake for 501 because they use grouped ServeMux paths.
func TestWebsiteP0StaticDomainHTTPSAndOpenRestyRoutes(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	t.Setenv("WORKMESH_WEBSITE_ROOT", filepath.Join(dataDir, "wwwroot"))
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(dataDir, "missing-openresty"))
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	RegisterSSLRoutes(mux)

	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}
	expectJSON := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		res := call(method, path, body)
		if res.Code != http.StatusOK {
			t.Fatalf("%s %s returned %d: %s", method, path, res.Code, res.Body.String())
		}
		if !strings.Contains(res.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("%s %s missing JSON content type: %s", method, path, res.Header().Get("Content-Type"))
		}
		return res
	}

	expectJSON(http.MethodPost, "/api/v2/websites", `{"primaryDomain":"p0-static.example","type":"static"}`)
	expectJSON(http.MethodGet, "/api/v2/websites/1", "")
	expectJSON(http.MethodPost, "/api/v2/websites/domains", `{"websiteID":1,"domain":"www.p0-static.example","port":80,"ssl":false}`)
	if body := expectJSON(http.MethodGet, "/api/v2/websites/domains/1", "").Body.String(); !strings.Contains(body, "www.p0-static.example") {
		t.Fatalf("domain list did not contain persisted domain: %s", body)
	}
	expectJSON(http.MethodPost, "/api/v2/websites/config/update", `{"websiteID":1,"type":"proxy","config":{"enabled":true,"proxyPass":"http://127.0.0.1:18080"}}`)
	if body := expectJSON(http.MethodGet, "/api/v2/websites/1/config/proxy", "").Body.String(); !strings.Contains(body, "127.0.0.1:18080") {
		t.Fatalf("proxy config was not read from persisted config: %s", body)
	}
	expectJSON(http.MethodGet, "/api/v2/websites/1/https", "")
	expectJSON(http.MethodPost, "/api/v2/websites/1/https", `{"enable":false,"httpConfig":"HTTPToHTTPS"}`)
	expectJSON(http.MethodGet, "/api/v2/websites/1/lbs", "")
	expectJSON(http.MethodGet, "/api/v2/websites/cors/1", "")
	expectJSON(http.MethodGet, "/api/v2/websites/resource/1", "")
	expectJSON(http.MethodGet, "/api/v2/websites/default/html/index", "")

	expectJSON(http.MethodPost, "/api/v2/openresty/file", `{"content":"events {}\nhttp {\n  server_tokens off;\n}"}`)
	expectJSON(http.MethodGet, "/api/v2/openresty", "")
	expectJSON(http.MethodGet, "/api/v2/openresty/modules", "")
	expectJSON(http.MethodGet, "/api/v2/openresty/https", "")
	expectJSON(http.MethodPost, "/api/v2/openresty/scope", `{"scope":"http-per","params":{"gzip":"off"}}`)
	if res := call(http.MethodPost, "/api/v2/openresty/build", `{"modules":[]}`); res.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing OpenResty binary should return explicit 503, got %d: %s", res.Code, res.Body.String())
	}
}
