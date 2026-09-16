// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/todaybin/workmesh-server/runtime/gateway"
)

func TestGatewayRoutesUseExternalProtocolClient(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	var calls map[string]int
	calls = make(map[string]int)
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		w.Header().Set("Content-Type", "application/json")
		response := map[string]any{"code": 200, "data": map[string]any{"bindingId": "binding-1", "refreshable": true, "scopes": []string{"node"}}}
		if r.URL.Path == "/api/workmesh/v2/nodes/heartbeat" || r.URL.Path == "/api/workmesh/v2/nodes/authorization/revoke" {
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
	wantCalls := map[string]int{"/workmesh/node/register": 1, "/workmesh/auth/login": 1, "/workmesh/node/heartbeat": 2, "/api/workmesh/v2/nodes/authorization/refresh": 1, "/api/workmesh/v2/nodes/authorization/revoke": 1}
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
	t.Setenv("WORKMESH_GATEWAY_ALLOW_HTTP", "1")
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

func TestGatewayHeartbeatDisconnectRecoveryKeepsBinding(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	t.Setenv("WORKMESH_GATEWAY_ALLOW_HTTP", "1")
	t.Setenv("WORKMESH_GATEWAY_ID", "gateway-recovery")
	t.Setenv("WORKMESH_GATEWAY_SECRET", "gateway-recovery-secret")
	var available atomic.Bool
	available.Store(true)
	var registers atomic.Int32
	var heartbeats atomic.Int32
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !available.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"code":"ERR","message":"gateway unavailable"}`))
			return
		}
		switch r.URL.Path {
		case "/workmesh/node/register":
			registers.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"item": map[string]any{"nodeId": "node-recovery", "bindingId": "binding-recovery"}}})
		case "/workmesh/node/heartbeat":
			heartbeats.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"accepted": true}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer cloud.Close()
	t.Setenv("WORKMESH_GATEWAY_URL", cloud.URL)

	store := RegisterGatewayRoutes(http.NewServeMux(), "node-recovery", "secondary")
	store.connectGateway(context.Background(), []string{"system"})
	if store.auth.BindingID != "binding-recovery" || store.status.Registration != gateway.RegistrationRegistered {
		t.Fatalf("initial registration failed: status=%+v auth=%+v", store.status, store.auth)
	}
	if registers.Load() != 1 {
		t.Fatalf("expected one registration, got %d", registers.Load())
	}

	available.Store(false)
	store.gatewayHeartbeat(context.Background())
	if store.status.Connected || store.status.Registration != gateway.RegistrationRegistered {
		t.Fatalf("disconnect should preserve binding and mark offline: status=%+v", store.status)
	}

	available.Store(true)
	store.connectGateway(context.Background(), []string{"system"})
	if !store.status.Connected || store.status.Registration != gateway.RegistrationRegistered {
		t.Fatalf("recovery heartbeat failed: status=%+v", store.status)
	}
	if registers.Load() != 1 || heartbeats.Load() != 1 {
		t.Fatalf("recovery must heartbeat without re-registering: registers=%d heartbeats=%d", registers.Load(), heartbeats.Load())
	}
}

func TestGatewayRegisterAcceptsFrontendBindingPayloadAndRestoresURL(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	t.Setenv("WORKMESH_GATEWAY_ALLOW_HTTP", "1")
	t.Setenv("WORKMESH_GATEWAY_ID", "gateway-front")
	var authHeader string
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"item": map[string]any{"nodeId": "node-front"}}})
	}))
	defer cloud.Close()
	mux := http.NewServeMux()
	RegisterGatewayRoutes(mux, "node-front", "secondary")
	request := httptest.NewRequest(http.MethodPost, "/api/v2/workmesh/gateway/register", strings.NewReader(`{"gatewayUrl":"`+cloud.URL+`","nodeId":"node-front","registrationToken":"registration-token","displayName":"front"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"registered":true`) {
		t.Fatalf("注册响应错误 status=%d body=%s", response.Code, response.Body.String())
	}
	if authHeader != "Bearer registration-token" {
		t.Fatalf("未转发 registrationToken，Authorization=%q", authHeader)
	}
	state, err := os.ReadFile(filepath.Join(dataDir, "gateway-binding.json"))
	if err != nil || !strings.Contains(string(state), cloud.URL) {
		t.Fatalf("绑定快照未正确保存: err=%v state=%s", err, state)
	}

	// 重启时只提供数据目录，不提供 WORKMESH_GATEWAY_URL，也应恢复地址和绑定摘要。
	reloadedMux := http.NewServeMux()
	reloaded := RegisterGatewayRoutes(reloadedMux, "node-front", "secondary")
	if reloaded.gatewayURL != cloud.URL || reloaded.auth.BindingID == "" {
		t.Fatalf("重启未恢复 Gateway 地址/绑定: url=%q auth=%+v", reloaded.gatewayURL, reloaded.auth)
	}
}

func TestGatewayRegisterRejectsCredentialBearingURL(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	RegisterGatewayRoutes(mux, "node-front", "secondary")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v2/workmesh/gateway/register", strings.NewReader(`{"gatewayUrl":"https://user:pass@example.com","nodeId":"node-front","registrationToken":"token"}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("带凭据 URL 应拒绝，status=%d body=%s", response.Code, response.Body.String())
	}
}
