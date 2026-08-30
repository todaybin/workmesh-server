// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProcessWebSocketNonUpgrade(t *testing.T) {
	mux := http.NewServeMux()
	registerProcessRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/process/ws", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 for capability probe, got %d", res.Code)
	}
	if got := res.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("expected JSON capability response, got %q", got)
	}
}
