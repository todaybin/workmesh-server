// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnalyticsRoutesReturnTypedData(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerAnalyticsRoutes(mux)
	paths := []string{"/api/v2/status", "/api/v2/attack/stat", "/api/v2/block/search", "/api/v2/qps", "/api/v2/rank", "/api/v2/relation/stat", "/api/v2/stat", "/api/v2/test", "/api/v2/trend", "/api/v2/visitors", "/api/v2/visitors/loc"}
	for _, path := range paths {
		r := httptest.NewRecorder()
		method := http.MethodPost
		if path == "/api/v2/status" {
			method = http.MethodGet
		}
		mux.ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(`{"websiteID":1}`)))
		if r.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, r.Code, r.Body.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(r.Body.Bytes(), &envelope); err != nil || envelope["code"] != float64(200) {
			t.Fatalf("%s invalid envelope: %s", path, r.Body.String())
		}
	}
}

func TestAnalyticsConfigPersists(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerAnalyticsRoutes(mux)
	update := httptest.NewRecorder()
	mux.ServeHTTP(update, httptest.NewRequest(http.MethodPost, "/api/v2/config/site/update", strings.NewReader(`{"websiteID":7,"enabled":false}`)))
	if update.Code != http.StatusOK {
		t.Fatalf("update status=%d", update.Code)
	}
	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest(http.MethodPost, "/api/v2/config/site", strings.NewReader(`{"websiteID":7}`)))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"enabled":false`) {
		t.Fatalf("config body=%s", get.Body.String())
	}
}
