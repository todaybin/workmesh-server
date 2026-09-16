// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package link

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/runtime/role"
)

var testNonceCounter atomic.Uint64

func TestHTTPClientSignsHandshakeAndRetriesWithFreshNonce(t *testing.T) {
	const secret = "link-test-secret"
	clock := func() time.Time { return time.Unix(1_700_000_000, 0) }
	manager, err := role.New("remote", role.Secondary)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerOptions{
		NodeID:      "remote",
		Role:        role.Secondary,
		Secret:      []byte(secret),
		RoleManager: manager,
		Clock:       clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server.Register(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	var nonceNumber atomic.Int32
	client := NewHTTPClientWithOptions(ts.URL, HTTPClientOptions{
		NodeID:       "local",
		Secret:       []byte(secret),
		Clock:        clock,
		MaxRetries:   1,
		RetryBackoff: time.Millisecond,
		Nonce: func() (string, error) {
			return "nonce-" + strconv.Itoa(int(nonceNumber.Add(1))), nil
		},
	})
	response, err := client.Handshake(context.Background(), Handshake{
		NodeID: "local", Role: role.Primary, RoleEpoch: 1, ProtocolVersion: "v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.NodeID != "remote" || response.Role != role.Secondary {
		t.Fatalf("握手响应错误: %+v", response)
	}
	if nonceNumber.Load() != 1 {
		t.Fatalf("成功请求不应重复发送，nonce 次数=%d", nonceNumber.Load())
	}
}

func TestHTTPClientRetriesTransientHTTPError(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"code":"ERR","message":"暂时不可用"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":200,"data":{"nodeId":"remote","role":"secondary","roleEpoch":1,"protocolVersion":"v2"}}`)
	}))
	defer server.Close()
	client := NewHTTPClientWithOptions(server.URL, HTTPClientOptions{MaxRetries: 1, RetryBackoff: time.Millisecond})
	response, err := client.Handshake(context.Background(), Handshake{NodeID: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if response.NodeID != "remote" || requests.Load() != 2 {
		t.Fatalf("重试结果错误 response=%+v requests=%d", response, requests.Load())
	}
}

func TestLinkServerSyncAndFencingRejectsStaleEpoch(t *testing.T) {
	const secret = "sync-secret"
	clock := func() time.Time { return time.Unix(1_700_000_000, 0) }
	server, err := NewServer(ServerOptions{NodeID: "remote", Role: role.Primary, Secret: []byte(secret), Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server.Register(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewHTTPClientWithOptions(ts.URL, HTTPClientOptions{NodeID: "local", Secret: []byte(secret), Clock: clock, MaxRetries: 0, RetryBackoff: time.Millisecond})
	next, err := client.Push(context.Background(), SyncCursor{Stream: "control", Version: 0}, []byte(`{"revision":1}`))
	if err != nil || next.Version != 1 {
		t.Fatalf("推送失败 cursor=%+v err=%v", next, err)
	}
	payload, cursor, err := client.Pull(context.Background(), SyncCursor{Stream: "control", Version: 0})
	if err != nil || string(payload) != `{"revision":1}` || cursor.Version != 1 {
		t.Fatalf("拉取失败 payload=%s cursor=%+v err=%v", payload, cursor, err)
	}

	stale := role.Transition{OperationID: "stale", NodeID: "remote", To: role.Secondary, ExpectedEpoch: 0}
	status, body := signedPost(t, ts.URL+"/api/v2/link/fencing/check", secret, "local", clock, stale)
	if status != http.StatusConflict || body == "" {
		t.Fatalf("过期 epoch 未被拒绝 status=%d body=%s", status, body)
	}

	transition := role.Transition{OperationID: "switch-1", NodeID: "remote", To: role.Secondary, ExpectedEpoch: 1}
	status, _ = signedPost(t, ts.URL+"/api/v2/link/fencing/prepare", secret, "local", clock, transition)
	if status != http.StatusOK {
		t.Fatalf("prepare status=%d", status)
	}
	status, body = signedPost(t, ts.URL+"/api/v2/link/fencing/commit", secret, "local", clock, transition)
	if status != http.StatusOK {
		t.Fatalf("commit status=%d body=%s", status, body)
	}
	status, _ = signedPost(t, ts.URL+"/api/v2/link/fencing/check", secret, "local", clock, transition)
	if status != http.StatusConflict {
		t.Fatalf("旧 epoch 在切换后应被拒绝 status=%d", status)
	}
}

// TestLinkStatusRequiresAuthenticationWhenSecretConfigured 验证 status 与其他链路接口使用相同认证策略。
func TestLinkStatusRequiresAuthenticationWhenSecretConfigured(t *testing.T) {
	const secret = "status-secret"
	clock := func() time.Time { return time.Unix(1_700_000_000, 0) }
	server, err := NewServer(ServerOptions{NodeID: "status-node", Role: role.Primary, Secret: []byte(secret), Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server.Register(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	response, err := (&http.Client{}).Get(ts.URL + "/api/v2/link/status")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未签名 status status=%d, want 401", response.StatusCode)
	}

	status, body := signedGet(t, ts.URL+"/api/v2/link/status", secret, "peer-status", "status-nonce-1", clock)
	if status != http.StatusOK || !strings.Contains(body, `"code":200`) {
		t.Fatalf("签名 status 失败 status=%d body=%s", status, body)
	}
	status, body = signedGet(t, ts.URL+"/api/v2/workmesh/link/status", secret, "peer-status", "status-nonce-2", clock)
	if status != http.StatusOK || !strings.Contains(body, `"code":200`) {
		t.Fatalf("兼容前缀 status 失败 status=%d body=%s", status, body)
	}
	status, body = signedGet(t, ts.URL+"/api/v2/link/status", secret, "peer-status", "status-nonce-1", clock)
	if status != http.StatusConflict || !strings.Contains(body, `"code":"ERR"`) {
		t.Fatalf("status nonce 重放未拒绝 status=%d body=%s", status, body)
	}
}

// TestLinkStatusAllowsAnonymousDevelopmentMode 保留未配置密钥时的本地开发兼容行为。
func TestLinkStatusAllowsAnonymousDevelopmentMode(t *testing.T) {
	server, err := NewServer(ServerOptions{NodeID: "dev-status-node", Role: role.Secondary})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server.Register(mux)
	request := httptest.NewRequest(http.MethodGet, "/api/v2/link/status", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":200`) {
		t.Fatalf("开发模式匿名 status 失败 status=%d body=%s", response.Code, response.Body.String())
	}
}

func signedPost(t *testing.T, endpoint, secret, nodeID string, clock Clock, value any) (int, string) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	timestamp := strconv.FormatInt(clock().Unix(), 10)
	nonce := "test-" + strconv.FormatUint(testNonceCounter.Add(1), 10)
	req.Header.Set(HeaderNodeID, nodeID)
	req.Header.Set(HeaderTimestamp, timestamp)
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderSignature, Sign([]byte(secret), req.Method, req.URL.RequestURI(), timestamp, nonce, body))
	// 通过请求目标的测试服务器发送，保留请求头和签名。
	result, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	data, _ := io.ReadAll(result.Body)
	return result.StatusCode, string(data)
}

func signedGet(t *testing.T, endpoint, secret, nodeID, nonce string, clock Clock) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	timestamp := strconv.FormatInt(clock().Unix(), 10)
	body := []byte{}
	req.Header.Set(HeaderNodeID, nodeID)
	req.Header.Set(HeaderTimestamp, timestamp)
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderSignature, Sign([]byte(secret), req.Method, req.URL.RequestURI(), timestamp, nonce, body))
	result, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	data, _ := io.ReadAll(result.Body)
	return result.StatusCode, string(data)
}
