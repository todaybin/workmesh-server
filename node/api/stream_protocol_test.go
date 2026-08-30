// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"net"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestContainerLogArgsValidateInput(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/v2/containers/search/log?container=web&tail=200&follow=true&timestamp=true", nil)
	args, follow, err := containerLogArgs(request)
	if err != nil {
		t.Fatalf("解析容器日志参数失败: %v", err)
	}
	expected := []string{"logs", "--follow", "--tail", "200", "--timestamps", "web"}
	if !follow || !reflect.DeepEqual(args, expected) {
		t.Fatalf("容器日志参数错误: follow=%v args=%v", follow, args)
	}
	for _, target := range []string{
		"/api/v2/containers/search/log",
		"/api/v2/containers/search/log?container=web&compose=/tmp/a.yml",
		"/api/v2/containers/search/log?container=web&tail=10001",
	} {
		if _, _, parseErr := containerLogArgs(httptest.NewRequest("GET", target, nil)); parseErr == nil {
			t.Fatalf("非法参数应被拒绝: %s", target)
		}
	}
}

func TestStreamAuthAndOrigin(t *testing.T) {
	t.Setenv("WORKMESH_STREAM_TOKEN", "stream-secret")
	request := httptest.NewRequest("GET", "http://node.example/api/v2/hosts/terminal/local", nil)
	response := httptest.NewRecorder()
	if requireStreamAuth(response, request, "WORKMESH_STREAM_TOKEN") || response.Code != 401 {
		t.Fatalf("无令牌流式请求应返回 401，实际 %d", response.Code)
	}
	request.Header.Set("X-WorkMesh-Token", "stream-secret")
	if !requireStreamAuth(httptest.NewRecorder(), request, "WORKMESH_STREAM_TOKEN") {
		t.Fatal("有效流式令牌应通过鉴权")
	}
	request.Header.Set("Origin", "https://evil.example")
	if validStreamOrigin(request) {
		t.Fatal("跨站 Origin 应被拒绝")
	}
	request.Header.Set("Origin", "http://node.example")
	if !validStreamOrigin(request) {
		t.Fatal("同源 Origin 应被允许")
	}
}

func TestWebSocketMaskedFrameAndWriteLimit(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	ws := &streamWebSocket{conn: server, read: bufio.NewReader(server), idleTimeout: time.Second}
	payload := []byte("hello")
	mask := [4]byte{1, 2, 3, 4}
	masked := make([]byte, len(payload))
	for index := range payload {
		masked[index] = payload[index] ^ mask[index%4]
	}
	go func() {
		frame := append([]byte{0x81, 0x80 | byte(len(payload))}, mask[:]...)
		frame = append(frame, masked...)
		_, _ = client.Write(frame)
	}()
	opcode, actual, err := ws.readFrame()
	if err != nil || opcode != 1 || string(actual) != "hello" {
		t.Fatalf("读取 WebSocket 帧失败: opcode=%d payload=%q err=%v", opcode, actual, err)
	}

	if err := writeStreamFrame(server, 1, make([]byte, maxWebSocketMessage+1)); err == nil {
		t.Fatal("超过 1MiB 的 WebSocket 帧应被拒绝")
	}
}
