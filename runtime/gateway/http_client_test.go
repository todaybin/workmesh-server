// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPClientRegisterUsesSignedEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-WorkMesh-Signature") == "" || r.Header.Get("X-WorkMesh-Nonce") == "" {
			t.Fatal("Gateway 请求缺少签名头")
		}
		if _, err := time.Parse(time.RFC3339, r.Header.Get("X-Timestamp")); err != nil {
			t.Fatalf("Runner 时间戳不是 RFC3339: %q", r.Header.Get("X-Timestamp"))
		}
		if r.Header.Get("X-Signature") == "" {
			t.Fatal("Runner 请求缺少 Ed25519 签名")
		}
		if r.URL.Path != "/workmesh/node/register" {
			t.Fatalf("注册路径错误: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": Authorization{BindingID: "binding-1", Refreshable: true}})
	}))
	defer server.Close()
	client := NewHTTPClient(server.URL, "gateway-test", "secret")
	auth, err := client.Register(context.Background(), RegisterRequest{NodeID: "node-test", Role: "secondary", ProtocolVersion: "v1"})
	if err != nil || auth.BindingID != "binding-1" || !auth.Refreshable {
		t.Fatalf("Gateway 注册失败: %+v, %v", auth, err)
	}
}

func TestHTTPClientRejectsGatewayHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "denied", http.StatusForbidden) }))
	defer server.Close()
	client := NewHTTPClient(server.URL, "gateway-test", "")
	if _, err := client.Refresh(context.Background()); err == nil {
		t.Fatal("Gateway HTTP 错误应返回错误")
	}
}

func TestHTTPClientLoginStoresBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/workmesh/auth/login" {
			t.Fatalf("登录路径错误: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"token": "jwt-test", "expiresIn": 60}})
	}))
	defer server.Close()
	client := NewHTTPClient(server.URL, "gateway-test", "secret")
	auth, err := client.Login(context.Background(), LoginRequest{Username: "u", Password: "p"})
	if err != nil || auth.AccessToken != "jwt-test" || client.AccessToken != "jwt-test" {
		t.Fatalf("Gateway 登录失败: %+v, %v", auth, err)
	}
}

func TestHTTPClientStatusMapsNodeIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/workmesh/node/index" {
			t.Fatalf("状态路径错误: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"items": []map[string]any{{"nodeId": "node-1", "status": "online"}}}})
	}))
	defer server.Close()
	client := NewHTTPClient(server.URL, "gateway-test", "secret")
	status, err := client.Status(context.Background())
	if err != nil || status.NodeID != "node-1" || status.Registration != RegistrationRegistered || !status.Connected {
		t.Fatalf("状态映射失败: %+v, %v", status, err)
	}
}
