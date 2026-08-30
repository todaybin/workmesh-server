// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/runtime/role"
)

func TestRoleWriteRoutesRequireAuthorizer(t *testing.T) {
	mux := http.NewServeMux()
	manager, err := role.New("auth-test", role.Secondary)
	if err != nil {
		t.Fatal(err)
	}
	RegisterRoleRoutesWithManager(mux, manager, func(*http.Request) bool { return false })
	request := httptest.NewRequest(http.MethodPost, "/api/v2/core/nodes/add", strings.NewReader(`{"nodeId":"other"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("未授权写请求 status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestGatewayWriteRoutesUseAuthorizer(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	RegisterGatewayRoutes(mux, "auth-test", "secondary", func(*http.Request) bool { return false })
	request := httptest.NewRequest(http.MethodPost, "/api/v2/gateway/unbind", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("未授权 Gateway 写请求 status=%d body=%s", response.Code, response.Body.String())
	}
}
