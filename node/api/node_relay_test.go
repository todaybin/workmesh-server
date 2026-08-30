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
	"testing"
)

func TestNodeRelayForwardsSignedOperateNodeRequest(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "nodes.json"), []byte(`[{"nodeId":"secondary","addr":"REMOTE"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	const secret = "relay-secret"
	remoteNext := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("operateNode") != "" {
			t.Fatal("透传请求不应继续携带 operateNode")
		}
		if r.Header.Get("X-WorkMesh-Forwarded") != "" {
			t.Fatal("接收端应清理透传标记")
		}
		if !IsForwardedRequestVerified(r) {
			t.Fatal("接收端应向下游中间件传递已验签上下文")
		}
		_, _ = w.Write([]byte(`{"code":200,"data":{"node":"secondary"}}`))
	})
	remoteRelay := NewNodeRelay(remoteNext, RelayOptions{DataDir: dataDir, NodeID: "secondary", Secret: []byte(secret), RoleEpoch: func(context.Context) (uint64, error) { return 3, nil }})
	remoteServer := httptest.NewServer(remoteRelay)
	defer remoteServer.Close()
	updated := strings.ReplaceAll(`[ {"nodeId":"secondary","addr":"REMOTE"} ]`, "REMOTE", remoteServer.URL)
	if err := os.WriteFile(filepath.Join(dataDir, "nodes.json"), []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}

	localNext := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("目标节点透传失败时不应调用本地处理器")
	})
	localRelay := NewNodeRelay(localNext, RelayOptions{DataDir: dataDir, NodeID: "primary", Secret: []byte(secret), RoleEpoch: func(context.Context) (uint64, error) { return 3, nil }})
	request := httptest.NewRequest(http.MethodPost, "/api/v2/files/search?operateNode=secondary&page=1", strings.NewReader(`{"path":"/"}`))
	response := httptest.NewRecorder()
	localRelay.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"secondary"`) {
		t.Fatalf("透传响应错误: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestNodeRelayRejectsMissingOrStaleSignature(t *testing.T) {
	called := false
	relay := NewNodeRelay(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }), RelayOptions{NodeID: "secondary", Secret: []byte("secret"), RoleEpoch: func(context.Context) (uint64, error) { return 2, nil }})
	request := httptest.NewRequest(http.MethodPost, "/api/v2/files/search", strings.NewReader(`{}`))
	request.Header.Set("X-WorkMesh-Forwarded", "1")
	response := httptest.NewRecorder()
	relay.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || called {
		t.Fatalf("缺少签名应拒绝: status=%d called=%v", response.Code, called)
	}
}

func TestNodeRelayRejectsUnknownNode(t *testing.T) {
	relay := NewNodeRelay(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), RelayOptions{DataDir: t.TempDir(), NodeID: "primary", Secret: []byte("secret")})
	response := httptest.NewRecorder()
	relay.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v2/files/search?operateNode=missing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("未知节点应返回 404: %d", response.Code)
	}
	var envelope map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope["code"] != "ERR" {
		t.Fatalf("错误响应 envelope 无效: %s", response.Body.String())
	}
}

func TestNodeRelayRejectsExpiredTimestamp(t *testing.T) {
	relay := NewNodeRelay(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), RelayOptions{NodeID: "secondary", Secret: []byte("secret"), RoleEpoch: func(context.Context) (uint64, error) { return 1, nil }})
	request := httptest.NewRequest(http.MethodPost, "/api/v2/files/search", strings.NewReader(`{}`))
	request.Header.Set("X-WorkMesh-Forwarded", "1")
	request.Header.Set("X-WorkMesh-Node-ID", "primary")
	request.Header.Set("X-WorkMesh-Timestamp", "1")
	request.Header.Set("X-WorkMesh-Nonce", "n1")
	request.Header.Set("X-WorkMesh-Role-Epoch", "1")
	request.Header.Set("X-WorkMesh-Signature", "invalid")
	response := httptest.NewRecorder()
	relay.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("过期请求应拒绝: %d", response.Code)
	}
}
