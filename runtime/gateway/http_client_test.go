// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClientRegisterUsesSignedEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-WorkMesh-Signature") == "" || r.Header.Get("X-WorkMesh-Nonce") == "" {
			t.Fatal("Gateway 请求缺少签名头")
		}
		if r.URL.Path != "/api/workmesh/v1/nodes/register" {
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
