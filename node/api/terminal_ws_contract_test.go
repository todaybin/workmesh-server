// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestExternalContainerTerminalPTY verifies the container terminal path against
// a disposable Docker container, including real input/output and cleanup.
func TestExternalContainerTerminalPTY(t *testing.T) {
	if os.Getenv("WORKMESH_TERMINAL_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_TERMINAL_EXTERNAL_TEST=1 to run container terminal acceptance")
	}
	name := fmt.Sprintf("workmesh-acceptance-terminal-%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	if output, err := exec.Command("docker", "run", "-d", "--name", name, "python:3.12-alpine", "python", "-c", "import time; time.sleep(120)").CombinedOutput(); err != nil {
		t.Skipf("Docker 容器启动失败: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	t.Setenv("WORKMESH_TERMINAL_TOKEN", "terminal-external-token")
	mux := http.NewServeMux()
	registerTerminalRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	conn := dialTerminalTCP(t, host, "/api/v2/hosts/terminal/container?containerid="+url.QueryEscape(name)+"&command=sh", websocketHeaders("terminal-external-token", server.URL))
	defer conn.Close()
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil || response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("容器终端握手失败: status=%v err=%v", response, err)
	}
	message, _ := json.Marshal(terminalClientMessage{Type: "cmd", Data: base64.StdEncoding.EncodeToString([]byte("printf container-pty-ok\n"))})
	if err := writeClientWebSocketFrame(conn, 0x1, message); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		opcode, payload, readErr := readServerWebSocketFrame(reader)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if opcode != 0x1 {
			continue
		}
		var output terminalServerMessage
		if json.Unmarshal(payload, &output) == nil && output.Type == "cmd" {
			decoded, _ := base64.StdEncoding.DecodeString(output.Data)
			if strings.Contains(string(decoded), "container-pty-ok") {
				return
			}
		}
	}
}

// TestTerminalWebSocketTCPHandshakeAndOutput verifies a browser-compatible
// RFC6455 handshake and real local shell output over a TCP httptest server.
func TestTerminalWebSocketTCPHandshakeAndOutput(t *testing.T) {
	t.Setenv("WORKMESH_TERMINAL_TOKEN", "terminal-test-token")
	mux := http.NewServeMux()
	registerTerminalRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	conn := dialTerminalTCP(t, serverURL.Host, "/api/v2/hosts/terminal/local?cols=80&rows=24&command="+url.QueryEscape("printf ws-ok"), websocketHeaders("terminal-test-token", server.URL))
	defer conn.Close()

	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("读取 WebSocket 握手响应失败: %v", err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("握手状态码 = %d，响应=%s", response.StatusCode, response.Status)
	}
	if !strings.EqualFold(response.Header.Get("Upgrade"), "websocket") {
		t.Fatalf("握手 Upgrade 响应头错误: %q", response.Header.Get("Upgrade"))
	}
	if response.Header.Get("Sec-WebSocket-Accept") != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("握手 Accept 响应头错误: %q", response.Header.Get("Sec-WebSocket-Accept"))
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	foundOutput := false
	foundClose := false
	for !foundClose {
		opcode, payload, readErr := readServerWebSocketFrame(reader)
		if readErr != nil {
			t.Fatalf("读取终端服务端帧失败: %v", readErr)
		}
		switch opcode {
		case 0x1:
			var message terminalServerMessage
			if err := json.Unmarshal(payload, &message); err != nil {
				t.Fatalf("终端输出不是 JSON: %v", err)
			}
			if message.Type == "cmd" {
				data, err := base64.StdEncoding.DecodeString(message.Data)
				if err != nil {
					t.Fatalf("终端输出 base64 无效: %v", err)
				}
				foundOutput = foundOutput || strings.Contains(string(data), "ws-ok")
			}
		case 0x8:
			foundClose = true
		}
	}
	if !foundOutput {
		t.Fatal("未收到真实本地命令输出")
	}
}

// TestTerminalWebSocketAuthOriginAndHandshakeValidation verifies the public
// failure contract without starting a shell for rejected requests.
func TestTerminalWebSocketAuthOriginAndHandshakeValidation(t *testing.T) {
	t.Setenv("WORKMESH_TERMINAL_TOKEN", "terminal-test-token")
	mux := http.NewServeMux()
	registerTerminalRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")

	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{name: "missing token", headers: websocketHeaders("", server.URL), want: http.StatusUnauthorized},
		{name: "cross origin", headers: websocketHeaders("terminal-test-token", "https://evil.example"), want: http.StatusForbidden},
		{name: "missing version", headers: func() map[string]string {
			h := websocketHeaders("terminal-test-token", server.URL)
			delete(h, "Sec-WebSocket-Version")
			return h
		}(), want: http.StatusBadRequest},
		{name: "invalid nonce", headers: func() map[string]string {
			h := websocketHeaders("terminal-test-token", server.URL)
			h["Sec-WebSocket-Key"] = base64.StdEncoding.EncodeToString([]byte("short"))
			return h
		}(), want: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conn := dialTerminalTCP(t, host, "/api/v2/hosts/terminal/local?command="+url.QueryEscape("printf should-not-run"), tc.headers)
			defer conn.Close()
			response, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if err != nil {
				t.Fatalf("读取拒绝响应失败: %v", err)
			}
			if response.StatusCode != tc.want {
				t.Fatalf("状态码 = %d，want %d", response.StatusCode, tc.want)
			}
		})
	}
}

// TestTerminalWebSocketInputAndCloseReleasesSession verifies masked client
// input, command output and an explicit close frame on one live session.
func TestTerminalWebSocketInputAndCloseReleasesSession(t *testing.T) {
	t.Setenv("WORKMESH_TERMINAL_TOKEN", "terminal-test-token")
	mux := http.NewServeMux()
	registerTerminalRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	conn := dialTerminalTCP(t, host, "/api/v2/hosts/terminal/local?cols=80&rows=24&command="+url.QueryEscape("read line; printf 'input:%s' \"$line\"; sleep 5"), websocketHeaders("terminal-test-token", server.URL))
	defer conn.Close()
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil || response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("终端输入会话握手失败: status=%v err=%v", response, err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	message, _ := json.Marshal(terminalClientMessage{Type: "cmd", Data: base64.StdEncoding.EncodeToString([]byte("ping\n"))})
	if err := writeClientWebSocketFrame(conn, 0x1, message); err != nil {
		t.Fatalf("写入终端输入帧失败: %v", err)
	}
	foundOutput := false
	for !foundOutput {
		opcode, payload, readErr := readServerWebSocketFrame(reader)
		if readErr != nil {
			t.Fatalf("读取终端输入输出失败: %v", readErr)
		}
		if opcode != 0x1 {
			continue
		}
		var serverMessage terminalServerMessage
		if json.Unmarshal(payload, &serverMessage) == nil && serverMessage.Type == "cmd" {
			data, _ := base64.StdEncoding.DecodeString(serverMessage.Data)
			foundOutput = strings.Contains(string(data), "input:ping")
		}
	}
	if err := writeClientWebSocketFrame(conn, 0x8, []byte{0x03, 0xe8}); err != nil {
		t.Fatalf("写入终端关闭帧失败: %v", err)
	}
	for {
		opcode, _, readErr := readServerWebSocketFrame(reader)
		if readErr != nil {
			break
		}
		if opcode == 0x8 {
			break
		}
	}
}

func websocketHeaders(token, origin string) map[string]string {
	return map[string]string{
		"Connection":            "keep-alive, Upgrade",
		"Upgrade":               "websocket",
		"Sec-WebSocket-Version": "13",
		"Sec-WebSocket-Key":     "dGhlIHNhbXBsZSBub25jZQ==",
		"X-WorkMesh-Token":      token,
		"Origin":                origin,
	}
}

func dialTerminalTCP(t *testing.T, host, path string, headers map[string]string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", host, 2*time.Second)
	if err != nil {
		t.Fatalf("连接 httptest TCP 服务失败: %v", err)
	}
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\n", path, host)
	for key, value := range headers {
		fmt.Fprintf(conn, "%s: %s\r\n", key, value)
	}
	fmt.Fprint(conn, "\r\n")
	return conn
}

func writeClientWebSocketFrame(conn net.Conn, opcode byte, payload []byte) error {
	if len(payload) > 125 {
		return fmt.Errorf("test frame payload too large: %d", len(payload))
	}
	mask := [4]byte{0x13, 0x37, 0x42, 0x99}
	frame := []byte{0x80 | opcode, 0x80 | byte(len(payload)), mask[0], mask[1], mask[2], mask[3]}
	for index, value := range payload {
		frame = append(frame, value^mask[index%4])
	}
	_, err := conn.Write(frame)
	return err
}

func readServerWebSocketFrame(reader *bufio.Reader) (byte, []byte, error) {
	first, err := reader.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	second, err := reader.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	length := uint64(second & 0x7f)
	if length == 126 {
		var size [2]byte
		if _, err := io.ReadFull(reader, size[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(size[:]))
	} else if length == 127 {
		var size [8]byte
		if _, err := io.ReadFull(reader, size[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(size[:])
	}
	if length > maxWebSocketMessage {
		return 0, nil, fmt.Errorf("test frame too large: %d", length)
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return 0, nil, err
	}
	return first & 0x0f, payload, nil
}
