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

func TestFallbackRouteReturnsExplicitNotImplemented(t *testing.T) {
	mux := http.NewServeMux()
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	registerFallbackRoute(mux, "POST /api/v2/apps/{key}")
	registerFallbackRoute(mux, "GET /api/v2/apps/{key}")

	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/apps/demo", bytes.NewBufferString(`{"name":"demo"}`)))
	if create.Code != http.StatusNotImplemented {
		t.Fatalf("create status = %d", create.Code)
	}
	var created map[string]any
	if err := json.NewDecoder(create.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created["code"] != "ERR" {
		t.Fatalf("unexpected envelope: %#v", created)
	}
	details, ok := created["details"].(map[string]any)
	if !ok || details["errCode"] != "NOT_IMPLEMENTED" {
		t.Fatalf("unexpected error details: %#v", created["details"])
	}

	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v2/apps/demo?page=1&pageSize=10", nil))
	if list.Code != http.StatusNotImplemented {
		t.Fatalf("list status = %d", list.Code)
	}
}

func TestV2ContractRouteReturnsExplicitNotImplemented(t *testing.T) {
	mux := http.NewServeMux()
	RegisterLegacyCompatibilityRoutes(mux)

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v2/images/missing.png", nil))
	if response.Code != http.StatusNotImplemented {
		t.Fatalf("contract route status = %d", response.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	details, ok := body["details"].(map[string]any)
	if !ok || details["errCode"] != "NOT_IMPLEMENTED" {
		t.Fatalf("unexpected contract error: %#v", body)
	}
}
