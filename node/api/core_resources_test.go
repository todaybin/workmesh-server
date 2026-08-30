// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScriptRunRequiresCommandToken(t *testing.T) {
	mux := http.NewServeMux()
	registerCoreResourceRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/core/script/run?command=echo+ok", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", res.Code)
	}
}
