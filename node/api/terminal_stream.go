// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

type terminalClientMessage struct {
	Type      string `json:"type"`
	Data      string `json:"data"`
	Timestamp string `json:"timestamp,omitempty"`
}

type terminalServerMessage struct {
	Type      string `json:"type"`
	Data      string `json:"data,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

func handleTerminalStream(w http.ResponseWriter, r *http.Request) {
	if !requireStreamAuth(w, r, "WORKMESH_TERMINAL_TOKEN", "WORKMESH_STREAM_TOKEN") {
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"websocket": true, "upgradeRequired": true}})
		return
	}
	command, err := terminalCommand(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "TERMINAL_PARAMETERS_INVALID"}, "message": err.Error()})
		return
	}
	ws, err := upgradeStreamWebSocket(w, r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	defer ws.close()

	stdin, err := command.StdinPipe()
	if err != nil {
		writeTerminalError(ws, err)
		return
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		writeTerminalError(ws, err)
		return
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		writeTerminalError(ws, err)
		return
	}
	if err := command.Start(); err != nil {
		writeTerminalError(ws, fmt.Errorf("启动终端进程失败: %w", err))
		return
	}

	// 输出泵与输入循环共享一个关闭信号，任一方向断开都会取消进程并释放管道。
	done := make(chan struct{})
	var once sync.Once
	finish := func() {
		once.Do(func() {
			close(done)
			if command.Process != nil {
				_ = command.Process.Kill()
			}
		})
	}
	defer finish()
	go pumpTerminalOutput(ws, stdout, finish)
	go pumpTerminalOutput(ws, stderr, finish)
	go func() { _ = command.Wait(); finish() }()

	for {
		opcode, payload, err := ws.readFrame()
		if err != nil {
			return
		}
		switch opcode {
		case 0x8:
			// 客户端关闭时回送相同状态码，确保浏览器和反向代理完成正常关闭握手。
			code := uint16(1000)
			if len(payload) >= 2 {
				code = binary.BigEndian.Uint16(payload[:2])
			}
			_ = ws.closeWithCode(code, "")
			return
		case 0x9:
			ws.writeMu.Lock()
			_ = writeStreamFrame(ws.conn, 0xA, payload)
			ws.writeMu.Unlock()
		case 0xA:
			// Pong 会刷新 readFrame 的空闲截止时间，无需额外响应。
			continue
		case 0x1:
			if err := handleTerminalInput(ws, stdin, payload); err != nil {
				return
			}
		}
		select {
		case <-done:
			return
		default:
		}
	}
}

func terminalCommand(r *http.Request) (*exec.Cmd, error) {
	ctx, cancel := context.WithCancel(r.Context())
	_ = cancel // exec.CommandContext 在连接断开或 handler 返回时负责终止子进程。
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case strings.HasSuffix(path, "/local"):
		shell := strings.TrimSpace(os.Getenv("SHELL"))
		if runtime.GOOS == "windows" {
			shell = strings.TrimSpace(os.Getenv("COMSPEC"))
		}
		if shell == "" {
			if runtime.GOOS == "windows" {
				shell = "cmd.exe"
			} else {
				shell = "/bin/sh"
			}
		}
		command := r.URL.Query().Get("command")
		if command == "" {
			return exec.CommandContext(ctx, shell), nil
		}
		if len(command) > 4096 || strings.IndexByte(command, 0) >= 0 {
			return nil, errors.New("command 参数无效")
		}
		if runtime.GOOS == "windows" {
			return exec.CommandContext(ctx, shell, "/C", command), nil
		}
		return exec.CommandContext(ctx, shell, "-c", command), nil
	case strings.HasSuffix(path, "/container"):
		containerID := strings.TrimSpace(r.URL.Query().Get("containerid"))
		program := strings.TrimSpace(r.URL.Query().Get("command"))
		if !validDockerIdentifier(containerID) || !validTerminalProgram(program) {
			return nil, errors.New("containerid 或 command 参数无效")
		}
		args := []string{"exec", "-i"}
		if user := strings.TrimSpace(r.URL.Query().Get("user")); user != "" {
			if !validDockerIdentifier(user) {
				return nil, errors.New("user 参数无效")
			}
			args = append(args, "-u", user)
		}
		args = append(args, containerID, program)
		return exec.CommandContext(ctx, "docker", args...), nil
	case strings.HasSuffix(path, "/ssh"):
		host := strings.TrimSpace(r.URL.Query().Get("host"))
		user := strings.TrimSpace(r.URL.Query().Get("user"))
		if host == "" || strings.ContainsAny(host, " \t\r\n\x00") || len(host) > 255 {
			return nil, errors.New("host 参数不能为空")
		}
		port := 22
		if raw := r.URL.Query().Get("port"); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 65535 {
				return nil, errors.New("port 参数无效")
			}
			port = value
		}
		target := host
		if user != "" {
			if !validDockerIdentifier(user) {
				return nil, errors.New("user 参数无效")
			}
			target = user + "@" + host
		}
		args := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-p", strconv.Itoa(port), target}
		if command := strings.TrimSpace(r.URL.Query().Get("command")); command != "" {
			if len(command) > 4096 || strings.IndexByte(command, 0) >= 0 {
				return nil, errors.New("command 参数无效")
			}
			args = append(args, command)
		}
		return exec.CommandContext(ctx, "ssh", args...), nil
	default:
		return nil, errors.New("终端类型不受支持")
	}
}

func validTerminalProgram(program string) bool {
	if program == "" || len(program) > 128 || strings.ContainsAny(program, " \t\r\n\x00/\\") {
		return false
	}
	for _, ch := range program {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || strings.ContainsRune("._-", ch)) {
			return false
		}
	}
	return true
}

func handleTerminalInput(ws *streamWebSocket, stdin io.Writer, payload []byte) error {
	var message terminalClientMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		return writeTerminalError(ws, errors.New("终端消息不是有效 JSON"))
	}
	switch message.Type {
	case "cmd":
		decoded, err := base64.StdEncoding.DecodeString(message.Data)
		if err != nil || len(decoded) > 64<<10 {
			return writeTerminalError(ws, errors.New("终端输入无效或超过 64KiB"))
		}
		_, err = stdin.Write(decoded)
		return err
	case "heartbeat":
		payload, _ := json.Marshal(terminalServerMessage{Type: "heartbeat", Timestamp: message.Timestamp})
		return ws.writeText(payload)
	case "resize":
		// 标准库无跨平台 PTY resize 能力；接受消息以保持协议兼容，进程生命周期不受影响。
		return nil
	default:
		return writeTerminalError(ws, errors.New("终端消息类型不受支持"))
	}
}

func pumpTerminalOutput(ws *streamWebSocket, reader io.Reader, finish func()) {
	defer finish()
	buffer := make([]byte, 32<<10)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			message, _ := json.Marshal(terminalServerMessage{Type: "cmd", Data: base64.StdEncoding.EncodeToString(buffer[:n])})
			if writeErr := ws.writeText(message); writeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func writeTerminalError(ws *streamWebSocket, err error) error {
	message, _ := json.Marshal(terminalServerMessage{Type: "cmd", Data: base64.StdEncoding.EncodeToString([]byte(err.Error() + "\r\n"))})
	return ws.writeText(message)
}

// terminalIdleTimeout 给调用方保留统一的会话超时定义，后续接入 PTY 时沿用该边界。
const terminalIdleTimeout = 30 * time.Minute
