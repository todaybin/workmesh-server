// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGatewayRoutesUseExternalProtocolClient(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
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
	wantCalls := map[string]int{"/workmesh/node/register": 1, "/workmesh/auth/login": 1, "/workmesh/node/heartbeat": 2, "/api/workmesh/v1/nodes/authorization/refresh": 1, "/api/workmesh/v1/nodes/authorization/revoke": 1}
	for path, want := range wantCalls {
		if calls[path] != want {
			t.Errorf("cloud %s calls = %d, want %d", path, calls[path], want)
		}
	}
}

func TestGatewayRegisterRejectsMissingClient(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
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

func TestGatewayLoginRegistersNodeAndPersistsBinding(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	registered := false
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/workmesh/auth/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"token": "session-token", "expiresIn": 3600}})
		case "/workmesh/node/register":
			registered = true
			// 真实 Gateway 返回 data.item；客户端使用稳定 nodeId 作为本地 bindingId。
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"item": map[string]any{"nodeId": "node-login"}}})
		case "/workmesh/node/heartbeat":
			if !registered {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": "ERR", "message": "record not found"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": nil})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer cloud.Close()
	t.Setenv("WORKMESH_GATEWAY_URL", cloud.URL)
	t.Setenv("WORKMESH_GATEWAY_ID", "gateway-login")
	t.Setenv("WORKMESH_GATEWAY_SECRET", "gateway-secret")

	mux := http.NewServeMux()
	RegisterGatewayRoutes(mux, "node-login", "primary")
	login := httptest.NewRecorder()
	mux.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v2/workmesh/gateway/login", strings.NewReader(`{"username":"workmesh","password":"secret"}`)))
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), `"bound":true`) {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
	}

	status := httptest.NewRecorder()
	mux.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v2/workmesh/gateway/status", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"configured":true`) || !strings.Contains(status.Body.String(), `"account":"workmesh"`) {
		t.Fatalf("status = %d, body = %s", status.Code, status.Body.String())
	}
	statePath := filepath.Join(dataDir, "gateway-binding.json")
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("binding state missing: %v", err)
	}

	reloadedMux := http.NewServeMux()
	RegisterGatewayRoutes(reloadedMux, "node-login", "primary")
	reloaded := httptest.NewRecorder()
	reloadedMux.ServeHTTP(reloaded, httptest.NewRequest(http.MethodGet, "/api/v2/workmesh/gateway/status", nil))
	if !strings.Contains(reloaded.Body.String(), `"configured":true`) || !strings.Contains(reloaded.Body.String(), `"account":"workmesh"`) {
		t.Fatalf("reloaded status = %s", reloaded.Body.String())
	}
}

func TestGatewayLoginDoesNotReportSuccessWhenNodeRegistrationFails(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/workmesh/auth/login" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"token": "session-token"}})
			return
		}
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "ERR", "message": "register unavailable"})
	}))
	defer cloud.Close()
	t.Setenv("WORKMESH_GATEWAY_URL", cloud.URL)

	mux := http.NewServeMux()
	RegisterGatewayRoutes(mux, "node-failed", "secondary")
	login := httptest.NewRecorder()
	mux.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v2/workmesh/gateway/login", strings.NewReader(`{"username":"workmesh","password":"secret"}`)))
	if login.Code != http.StatusBadGateway || strings.Contains(login.Body.String(), `"bound":true`) {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
	}
}
