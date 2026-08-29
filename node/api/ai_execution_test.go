// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAIProviderAndSandboxStatus(t *testing.T) {
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	provider := httptest.NewRecorder()
	mux.ServeHTTP(provider, httptest.NewRequest(http.MethodGet, "/api/v2/ai/accounts/providers", nil))
	if provider.Code != http.StatusOK || !strings.Contains(provider.Body.String(), `"code":200`) {
		t.Fatalf("provider response: %d %s", provider.Code, provider.Body.String())
	}
	sandbox := httptest.NewRecorder()
	mux.ServeHTTP(sandbox, httptest.NewRequest(http.MethodGet, "/api/v2/cubesandbox/status", nil))
	if sandbox.Code != http.StatusOK || !strings.Contains(sandbox.Body.String(), "available") {
		t.Fatalf("sandbox response: %d %s", sandbox.Code, sandbox.Body.String())
	}
}

func TestTaskExecRequiresToken(t *testing.T) {
	t.Setenv("WORKMESH_TASK_TOKEN", "expected")
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/workmesh/tasks/exec", strings.NewReader(`{"program":"echo","args":["ok"]}`))
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.Code)
	}
}
