// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestContainerMethodValidation(t *testing.T) {
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodDelete, "/api/v2/containers/demo", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", res.Code)
	}
}
