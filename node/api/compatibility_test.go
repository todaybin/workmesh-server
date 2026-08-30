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

func TestNormalizeServeMuxPattern(t *testing.T) {
	if got := normalizeServeMuxPattern("GET /api/v2/apps/:key"); got != "GET /api/v2/apps/{key}" {
		t.Fatalf("unexpected named pattern: %s", got)
	}
	if got := normalizeServeMuxPattern("GET /api/v2/images/*filename"); got != "GET /api/v2/images/{filename...}" {
		t.Fatalf("unexpected catch-all pattern: %s", got)
	}
}

func TestFallbackRouteCRUD(t *testing.T) {
	mux := http.NewServeMux()
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	registerFallbackRoute(mux, "POST /api/v2/apps/{key}")
	registerFallbackRoute(mux, "GET /api/v2/apps/{key}")

	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/apps/demo", bytes.NewBufferString(`{"name":"demo"}`)))
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d", create.Code)
	}
	var created map[string]any
	if err := json.NewDecoder(create.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created["code"] != float64(200) {
		t.Fatalf("unexpected envelope: %#v", created)
	}

	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v2/apps/demo?page=1&pageSize=10", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d", list.Code)
	}
}
