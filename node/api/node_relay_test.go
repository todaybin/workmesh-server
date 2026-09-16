// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newIPv4TestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}
	server.Start()
	return server
}

func TestNodeRelayRetriesReplaySafeRequestAfterDisconnect(t *testing.T) {
	dataDir := t.TempDir()
	const secret = "relay-retry-secret"
	var calls atomic.Int32
	remote := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("测试服务器不支持连接劫持")
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = conn.Close()
			return
		}
		_, _ = w.Write([]byte(`{"code":200,"data":{"retried":true}}`))
	}))
	defer remote.Close()
	if err := os.WriteFile(filepath.Join(dataDir, "nodes.json"), []byte(`[{"nodeId":"secondary","addr":"`+remote.URL+`"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	relay := NewNodeRelay(http.NotFoundHandler(), RelayOptions{
		DataDir: dataDir, NodeID: "primary", Secret: []byte(secret), MaxRetries: 1,
		RetryBackoff: 1,
		RoleEpoch:    func(context.Context) (uint64, error) { return 4, nil },
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v2/containers/list?operateNode=secondary", nil)
	response := httptest.NewRecorder()
	relay.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"retried":true`) {
		t.Fatalf("断线重试失败: status=%d body=%s calls=%d", response.Code, response.Body.String(), calls.Load())
	}
	if calls.Load() != 2 {
		t.Fatalf("应只重试一次，实际请求数=%d", calls.Load())
	}
}

func TestNodeRelayDoesNotReplayNonIdempotentWriteWithoutKey(t *testing.T) {
	dataDir := t.TempDir()
	var calls atomic.Int32
	remote := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("测试服务器不支持连接劫持")
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.Close()
	}))
	defer remote.Close()
	if err := os.WriteFile(filepath.Join(dataDir, "nodes.json"), []byte(`[{"nodeId":"secondary","addr":"`+remote.URL+`"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	relay := NewNodeRelay(http.NotFoundHandler(), RelayOptions{DataDir: dataDir, NodeID: "primary", MaxRetries: 2, RetryBackoff: 1, RoleEpoch: func(context.Context) (uint64, error) { return 1, nil }})
	request := httptest.NewRequest(http.MethodPost, "/api/v2/websites/create?operateNode=secondary", strings.NewReader(`{"name":"site"}`))
	response := httptest.NewRecorder()
	relay.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway || calls.Load() != 1 {
		t.Fatalf("非幂等写请求不应重放: status=%d calls=%d body=%s", response.Code, calls.Load(), response.Body.String())
	}
}

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
	remoteServer := newIPv4TestServer(t, remoteRelay)
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

func TestNodeRelayForwardsSSEIncrementallyWithoutShortTimeout(t *testing.T) {
	dataDir := t.TempDir()
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseStream := func() {
		releaseOnce.Do(func() { close(release) })
	}
	defer releaseStream()
	remote := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		_, _ = w.Write([]byte("data: first\n\n"))
		flusher.Flush()
		<-release
		_, _ = w.Write([]byte("data: second\n\n"))
		flusher.Flush()
	}))
	defer remote.Close()
	if err := os.WriteFile(filepath.Join(dataDir, "nodes.json"), []byte(`[{"nodeId":"secondary","addr":"`+remote.URL+`"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	relay := NewNodeRelay(http.NotFoundHandler(), RelayOptions{
		DataDir: dataDir, NodeID: "primary", MaxRetries: 1, Timeout: 20 * time.Millisecond,
		RoleEpoch: func(context.Context) (uint64, error) { return 1, nil },
	})
	server := newIPv4TestServer(t, relay)
	defer server.Close()

	first := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		request, err := http.NewRequest(http.MethodGet, server.URL+"/api/v2/containers/search/log?operateNode=secondary", nil)
		if err != nil {
			done <- err
			return
		}
		request.Header.Set("Accept", "text/event-stream")
		response, err := (&http.Client{Timeout: time.Second}).Do(request)
		if err != nil {
			done <- err
			return
		}
		defer response.Body.Close()
		line, err := bufio.NewReader(response.Body).ReadString('\n')
		if err != nil {
			done <- err
			return
		}
		first <- line
		body, err := io.ReadAll(response.Body)
		if err == nil && !strings.Contains(string(body), "data: second") {
			err = fmt.Errorf("后续 SSE 事件缺失: %q", body)
		}
		done <- err
	}()

	select {
	case line := <-first:
		if line != "data: first\n" {
			t.Fatalf("SSE 首个事件未增量到达: %q", line)
		}
	case err := <-done:
		t.Fatalf("SSE 透传提前失败: %v", err)
	case <-time.After(time.Second):
		t.Fatal("SSE 透传等待首个事件超时")
	}
	releaseStream()
	if err := <-done; err != nil {
		t.Fatal(err)
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
