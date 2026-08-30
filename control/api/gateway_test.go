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

func TestGatewayRoutesUseExternalProtocolClient(t *testing.T) {
	var calls map[string]int
	calls = make(map[string]int)
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		w.Header().Set("Content-Type", "application/json")
		response := map[string]any{"code": 200, "data": map[string]any{"bindingId": "binding-1", "refreshable": true, "scopes": []string{"node"}}}
		if r.URL.Path == "/api/workmesh/v1/nodes/heartbeat" || r.URL.Path == "/api/workmesh/v1/nodes/authorization/revoke" {
			response = map[string]any{"code": 200, "data": nil}
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer cloud.Close()
	t.Setenv("WORKMESH_GATEWAY_URL", cloud.URL)
	t.Setenv("WORKMESH_GATEWAY_ID", "test-gateway")
	t.Setenv("WORKMESH_GATEWAY_SECRET", "test-secret")

	mux := http.NewServeMux()
	RegisterGatewayRoutes(mux, "node-1", "secondary")

	registerBody := `{"nodeId":"node-1","role":"secondary","capabilities":["files"]}`
	register := httptest.NewRecorder()
	mux.ServeHTTP(register, httptest.NewRequest(http.MethodPost, "/api/v2/gateway/register", strings.NewReader(registerBody)))
	if register.Code != http.StatusOK {
		t.Fatalf("register status = %d, body = %s", register.Code, register.Body.String())
	}
	login := httptest.NewRecorder()
	mux.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v2/workmesh/gateway/login", strings.NewReader(`{"username":"u","password":"p"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
	}

	heartbeat := httptest.NewRecorder()
	mux.ServeHTTP(heartbeat, httptest.NewRequest(http.MethodPost, "/api/v2/gateway/heartbeat", nil))
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("heartbeat status = %d, body = %s", heartbeat.Code, heartbeat.Body.String())
	}
	refresh := httptest.NewRecorder()
	mux.ServeHTTP(refresh, httptest.NewRequest(http.MethodPost, "/api/v2/gateway/authorization/refresh", nil))
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, body = %s", refresh.Code, refresh.Body.String())
	}
	unbind := httptest.NewRecorder()
	mux.ServeHTTP(unbind, httptest.NewRequest(http.MethodPost, "/api/v2/gateway/unbind", nil))
	if unbind.Code != http.StatusOK {
		t.Fatalf("unbind status = %d, body = %s", unbind.Code, unbind.Body.String())
	}
	for _, path := range []string{"/workmesh/node/register", "/workmesh/auth/login", "/workmesh/node/heartbeat", "/api/workmesh/v1/nodes/authorization/refresh", "/api/workmesh/v1/nodes/authorization/revoke"} {
		if calls[path] != 1 {
			t.Errorf("cloud %s calls = %d, want 1", path, calls[path])
		}
	}
}

func TestGatewayRegisterRejectsMissingClient(t *testing.T) {
	t.Setenv("WORKMESH_GATEWAY_URL", "")
	t.Setenv("WORKMESH_GATEWAY_ID", "")
	t.Setenv("WORKMESH_GATEWAY_SECRET", "")
	mux := http.NewServeMux()
	RegisterGatewayRoutes(mux, "node-local", "secondary")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v2/gateway/register", strings.NewReader(`{"nodeId":"node-local","role":"secondary"}`)))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("register status = %d, want %d; body=%s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
	var envelope struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != "ERR" {
		t.Fatalf("error envelope code = %q, want ERR", envelope.Code)
	}
}
