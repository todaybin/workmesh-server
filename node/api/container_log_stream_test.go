// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestContainerLogHelper 为日志流取消测试提供可控的子进程输出。
func TestContainerLogHelper(t *testing.T) {
	if os.Getenv("WORKMESH_CONTAINER_LOG_HELPER") != "1" {
		return
	}
	_, _ = fmt.Fprintln(os.Stdout, "line-one")
	// 保持进程运行，直到父进程取消 context，验证断开后 Docker 进程会被终止。
	select {}
}

type captureSSEWriter struct {
	mu   sync.Mutex
	head http.Header
	body bytes.Buffer
	seen chan struct{}
	once sync.Once
}

func (w *captureSSEWriter) Header() http.Header { return w.head }

func (w *captureSSEWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.body.Write(p)
	if bytes.Contains(p, []byte("line-one")) {
		w.once.Do(func() { close(w.seen) })
	}
	return n, err
}

func (w *captureSSEWriter) WriteHeader(statusCode int) {}

func (w *captureSSEWriter) Flush() {}

func (w *captureSSEWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String()
}

func TestContainerLogSSEUsesDefaultMessageAndCancelsProcess(t *testing.T) {
	t.Setenv("WORKMESH_STREAM_TOKEN", "container-stream-test")
	oldCommand := containerLogCommand
	defer func() { containerLogCommand = oldCommand }()
	var captured []string
	containerLogCommand = func(ctx context.Context, args ...string) *exec.Cmd {
		captured = append([]string(nil), args...)
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestContainerLogHelper$")
		command.Env = append(os.Environ(), "WORKMESH_CONTAINER_LOG_HELPER=1")
		return command
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/containers/search/log?container=web&since=all&tail=0&follow=true", nil).WithContext(ctx)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-WorkMesh-Token", "container-stream-test")
	writer := &captureSSEWriter{head: make(http.Header), seen: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		handleContainerLogStream(writer, req)
		close(done)
	}()
	select {
	case <-writer.seen:
	case <-time.After(3 * time.Second):
		cancel()
		<-done
		t.Fatal("等待 Docker 日志输出超时")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("请求取消后日志进程未退出")
	}
	body := writer.String()
	if !strings.Contains(body, "id: 2\ndata: line-one\n\n") {
		t.Fatalf("SSE 日志必须使用默认 message 事件，响应=%q", body)
	}
	if strings.Contains(body, "event: log") {
		t.Fatalf("日志不应使用命名事件，否则前端 onmessage 无法接收: %q", body)
	}
	expected := []string{"logs", "--follow", "web"}
	if len(captured) != len(expected) {
		t.Fatalf("tail=0/since=all 应省略对应参数，实际 args=%v", captured)
	}
	for i := range expected {
		if captured[i] != expected[i] {
			t.Fatalf("日志参数不符合旧接口语义: args=%v", captured)
		}
	}
}

func TestContainerSSELastEventIDContinuesSequence(t *testing.T) {
	if got := parseLastEventID("42"); got != 42 {
		t.Fatalf("Last-Event-ID 解析错误: %d", got)
	}
	for _, value := range []string{"", "-1", "abc", "1.5"} {
		if got := parseLastEventID(value); got != 0 {
			t.Fatalf("非法 Last-Event-ID 应回退 0: %q => %d", value, got)
		}
	}
	writer := &captureSSEWriter{head: make(http.Header), seen: make(chan struct{})}
	s := &containerSSEWriter{writer: writer, flusher: writer, nextID: 42}
	if err := s.event("ready", map[string]any{"ok": true}); err != nil {
		t.Fatalf("写入 SSE 事件失败: %v", err)
	}
	if !strings.Contains(writer.String(), "id: 43\nevent: ready\n") {
		t.Fatalf("SSE 重连后事件编号未延续: %q", writer.String())
	}
}

func TestContainerSSEResumeReplaysBoundedHistory(t *testing.T) {
	key := fmt.Sprintf("resume-test-%d", time.Now().UnixNano())
	firstWriter := &captureSSEWriter{head: make(http.Header), seen: make(chan struct{})}
	first := newContainerSSEWriter(firstWriter, firstWriter, nil, key, 0)
	if err := first.event("ready", map[string]any{"source": "test"}); err != nil {
		t.Fatalf("写入初始事件失败: %v", err)
	}
	if _, err := first.Write([]byte("line-one\n")); err != nil {
		t.Fatalf("写入初始日志失败: %v", err)
	}
	secondWriter := &captureSSEWriter{head: make(http.Header), seen: make(chan struct{})}
	second := newContainerSSEWriter(secondWriter, secondWriter, nil, key, 1)
	if err := second.replay(1); err != nil {
		t.Fatalf("重放 SSE 事件失败: %v", err)
	}
	body := secondWriter.String()
	if !strings.Contains(body, "id: 2\ndata: line-one\n\n") {
		t.Fatalf("Last-Event-ID 后应重放日志事件，响应=%q", body)
	}
	if strings.Contains(body, "id: 1\nevent: ready") {
		t.Fatalf("Last-Event-ID=1 不应重复 ready 事件，响应=%q", body)
	}
}

func TestContainerSSEBackpressureIsBounded(t *testing.T) {
	called := false
	s := &containerSSEWriter{
		writer:  &captureSSEWriter{head: make(http.Header), seen: make(chan struct{})},
		flusher: &captureSSEWriter{head: make(http.Header), seen: make(chan struct{})},
		onError: func() { called = true },
	}
	if _, err := s.Write(bytes.Repeat([]byte("x"), maxContainerSSEPending+1)); err == nil {
		t.Fatal("超过积压上限的 SSE 输出必须失败")
	}
	if !called {
		t.Fatal("SSE 背压失败时应触发取消回调")
	}
}

func TestContainerLogArgsComposeAndAllSemantics(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v2/containers/search/log?compose=/srv/a.yml,/srv/b.yml&since=all&tail=0", nil)
	args, follow, err := containerLogArgs(req)
	if err != nil || follow {
		t.Fatalf("Compose 参数解析失败: args=%v follow=%v err=%v", args, follow, err)
	}
	expected := []string{"compose", "-f", "/srv/a.yml", "-f", "/srv/b.yml", "logs"}
	if len(args) != len(expected) {
		t.Fatalf("Compose 多文件参数错误: %v", args)
	}
	for i := range expected {
		if args[i] != expected[i] {
			t.Fatalf("Compose 多文件参数错误: %v", args)
		}
	}
	for _, value := range []string{"/srv/../secret.yml", "/srv/a.yml,", "/srv/a.yml,/srv/../b.yml"} {
		bad := httptest.NewRequest(http.MethodGet, "/api/v2/containers/search/log?compose="+value, nil)
		if _, _, err := containerLogArgs(bad); err == nil {
			t.Fatalf("路径穿越或空 Compose 文件段应被拒绝: %s", value)
		}
	}
}
