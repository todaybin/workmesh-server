// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package link

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/runtime/role"
	_ "modernc.org/sqlite"
)

type localTaskSnapshot struct {
	IdempotencyKey string   `json:"idempotencyKey"`
	Status         string   `json:"status"`
	Logs           []string `json:"logs"`
}

func TestGatewayLocalTaskSQLiteLifecycleAndFencing(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "gateway-local.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager, err := role.NewSQLite(db, "primary-local", role.Primary)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewSQLiteSyncStore(db)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerOptions{NodeID: "primary-local", Role: role.Primary, RoleManager: manager, SyncStore: store})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server.Register(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewHTTPClientWithOptions(ts.URL, HTTPClientOptions{NodeID: "secondary-local", MaxRetries: 0})

	created := localTaskSnapshot{IdempotencyKey: "deploy-001", Status: "created", Logs: []string{"任务已接收"}}
	createdPayload := mustJSON(t, created)
	next, err := client.Push(context.Background(), SyncCursor{Stream: "tasks.deploy-001", RoleEpoch: 1}, createdPayload)
	if err != nil || next.Version != 1 || next.RoleEpoch != 1 {
		t.Fatalf("创建任务失败: cursor=%+v err=%v", next, err)
	}
	// 相同幂等键和内容的网络重试不能创建第二个版本。
	retried, err := client.Push(context.Background(), SyncCursor{Stream: "tasks.deploy-001", RoleEpoch: 1}, createdPayload)
	if err != nil || retried.Version != 1 {
		t.Fatalf("任务幂等重试失败: cursor=%+v err=%v", retried, err)
	}
	completed := localTaskSnapshot{IdempotencyKey: "deploy-001", Status: "completed", Logs: []string{"任务已接收", "执行完成"}}
	next, err = client.Push(context.Background(), SyncCursor{Stream: "tasks.deploy-001", Version: 1, RoleEpoch: 1}, mustJSON(t, completed))
	if err != nil || next.Version != 2 {
		t.Fatalf("任务完成回写失败: cursor=%+v err=%v", next, err)
	}

	state, err := manager.Switch(context.Background(), 1, role.Secondary)
	if err != nil || state.RoleEpoch != 2 {
		t.Fatalf("角色切换失败: state=%+v err=%v", state, err)
	}
	failed := localTaskSnapshot{IdempotencyKey: "deploy-002", Status: "failed", Logs: []string{"执行失败: controlled"}}
	_, err = client.Push(context.Background(), SyncCursor{Stream: "tasks.deploy-002", RoleEpoch: 1}, mustJSON(t, failed))
	if err == nil || !strings.Contains(err.Error(), "HTTP 409") {
		t.Fatalf("旧 epoch 任务未被拒绝: %v", err)
	}
	next, err = client.Push(context.Background(), SyncCursor{Stream: "tasks.deploy-002", RoleEpoch: 2}, mustJSON(t, failed))
	if err != nil || next.Version != 1 || next.RoleEpoch != 2 {
		t.Fatalf("当前 epoch 失败回写异常: cursor=%+v err=%v", next, err)
	}

	assertTaskSQLiteSnapshot(t, db, "tasks.deploy-001", 2, completed)
	assertTaskSQLiteSnapshot(t, db, "tasks.deploy-002", 1, failed)
	restored, err := role.NewSQLite(db, "primary-local", role.Primary)
	if err != nil {
		t.Fatal(err)
	}
	if got := restored.State(context.Background()); got.Role != role.Secondary || got.RoleEpoch != 2 {
		t.Fatalf("SQLite 未恢复单调 epoch: %+v", got)
	}
}

func TestGatewayLocalTCPAuthenticationReplayTimeoutAndReconnect(t *testing.T) {
	const secret = "gateway-local-secret"
	clock := func() time.Time { return time.Unix(1_700_000_000, 0) }
	linkServer, err := NewServer(ServerOptions{NodeID: "primary-tcp", Role: role.Primary, Secret: []byte(secret), Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	linkServer.Register(mux)
	slowStarted := make(chan struct{}, 1)
	slowDone := make(chan struct{}, 1)
	mux.HandleFunc("POST /slow", func(w http.ResponseWriter, r *http.Request) {
		slowStarted <- struct{}{}
		timer := time.NewTimer(200 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
		case <-timer.C:
		}
		slowDone <- struct{}{}
	})
	var opened atomic.Int32
	var created atomic.Int32
	ts := httptest.NewUnstartedServer(mux)
	ts.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		switch state {
		case http.StateNew:
			created.Add(1)
			opened.Add(1)
		case http.StateClosed, http.StateHijacked:
			opened.Add(-1)
		}
	}
	ts.Start()

	transport := &http.Transport{}
	httpClient := &http.Client{Transport: transport}
	client := NewHTTPClientWithOptions(ts.URL, HTTPClientOptions{HTTP: httpClient, NodeID: "secondary-tcp", Secret: []byte(secret), Clock: clock, MaxRetries: 0})
	response, err := client.Handshake(context.Background(), Handshake{NodeID: "secondary-tcp", Role: role.Secondary, RoleEpoch: 1, ProtocolVersion: "v2"})
	if err != nil || response.NodeID != "primary-tcp" {
		t.Fatalf("TCP 握手失败: response=%+v err=%v", response, err)
	}

	body := mustJSON(t, Heartbeat{NodeID: "secondary-tcp", RoleEpoch: 1, Version: "v2"})
	if status := sendSignedTCPRequest(t, httpClient, ts.URL+"/api/v2/link/heartbeat", body, secret, "secondary-tcp", "fixed-replay", clock); status != http.StatusOK {
		t.Fatalf("首次心跳 status=%d", status)
	}
	if status := sendSignedTCPRequest(t, httpClient, ts.URL+"/api/v2/link/heartbeat", body, secret, "secondary-tcp", "fixed-replay", clock); status != http.StatusConflict {
		t.Fatalf("nonce 重放 status=%d, want 409", status)
	}
	mismatch := mustJSON(t, Heartbeat{NodeID: "forged-node", RoleEpoch: 1, Version: "v2"})
	if status := sendSignedTCPRequest(t, httpClient, ts.URL+"/api/v2/link/heartbeat", mismatch, secret, "secondary-tcp", "node-mismatch", clock); status != http.StatusUnauthorized {
		t.Fatalf("签名节点与正文不一致 status=%d, want 401", status)
	}

	transport.CloseIdleConnections()
	ts.CloseClientConnections()
	waitForConnections(t, &opened, 0)
	transport = &http.Transport{}
	httpClient = &http.Client{Transport: transport}
	client = NewHTTPClientWithOptions(ts.URL, HTTPClientOptions{HTTP: httpClient, NodeID: "secondary-tcp", Secret: []byte(secret), Clock: clock, MaxRetries: 0})
	if err := client.Heartbeat(context.Background(), Heartbeat{NodeID: "secondary-tcp", RoleEpoch: 1, Version: "v2"}); err != nil {
		t.Fatalf("断线重连心跳失败: %v", err)
	}
	if created.Load() < 2 {
		t.Fatalf("未建立新 TCP 连接: created=%d", created.Load())
	}

	timeoutClient := NewHTTPClientWithOptions(ts.URL, HTTPClientOptions{HTTP: httpClient, NodeID: "secondary-tcp", Secret: []byte(secret), Clock: clock, Timeout: 30 * time.Millisecond, MaxRetries: 0})
	started := time.Now()
	err = timeoutClient.doJSON(context.Background(), http.MethodPost, "/slow", map[string]string{"probe": "timeout"}, nil)
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("TCP 超时未受控: elapsed=%s err=%v", time.Since(started), err)
	}
	select {
	case <-slowStarted:
	case <-time.After(time.Second):
		t.Fatal("超时请求未到达 TCP 服务")
	}
	select {
	case <-slowDone:
	case <-time.After(time.Second):
		t.Fatal("超时请求处理未结束")
	}

	transport.CloseIdleConnections()
	ts.CloseClientConnections()
	waitForConnections(t, &opened, 0)
	ts.Close()
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func assertTaskSQLiteSnapshot(t *testing.T, db *sql.DB, stream string, wantVersion uint64, want localTaskSnapshot) {
	t.Helper()
	var version uint64
	var payload []byte
	if err := db.QueryRow(`SELECT version,payload FROM link_sync WHERE stream=?`, stream).Scan(&version, &payload); err != nil {
		t.Fatal(err)
	}
	var got localTaskSnapshot
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if version != wantVersion || got.IdempotencyKey != want.IdempotencyKey || got.Status != want.Status || fmt.Sprint(got.Logs) != fmt.Sprint(want.Logs) {
		t.Fatalf("SQLite 任务快照异常: version=%d snapshot=%+v", version, got)
	}
}

func sendSignedTCPRequest(t *testing.T, client *http.Client, endpoint string, body []byte, secret, nodeID, nonce string, clock Clock) int {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	timestamp := strconv.FormatInt(clock().Unix(), 10)
	request.Header.Set(HeaderNodeID, nodeID)
	request.Header.Set(HeaderTimestamp, timestamp)
	request.Header.Set(HeaderNonce, nonce)
	request.Header.Set(HeaderSignature, Sign([]byte(secret), request.Method, request.URL.RequestURI(), timestamp, nonce, body))
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode
}

func waitForConnections(t *testing.T, opened *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if opened.Load() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("TCP 连接未释放: opened=%d want=%d", opened.Load(), want)
}
