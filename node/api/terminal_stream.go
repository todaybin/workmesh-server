// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
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
	Cols      int    `json:"cols,omitempty"`
	Rows      int    `json:"rows,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

type terminalServerMessage struct {
	Type      string `json:"type"`
	Data      string `json:"data,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

const (
	// terminalIdleTimeout 限制终端连接长时间无输入时占用的资源。
	terminalIdleTimeout  = 30 * time.Minute
	maxTerminalDimension = 500
)

// handleTerminalStream 建立本地、容器或 SSH 终端的双向 WebSocket 会话。
func handleTerminalStream(w http.ResponseWriter, r *http.Request) {
	if !requireStreamAuth(w, r, "WORKMESH_TERMINAL_TOKEN", "WORKMESH_STREAM_TOKEN") {
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"websocket": true, "upgradeRequired": true}})
		return
	}
	cols, rows, err := terminalDimensions(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "TERMINAL_PARAMETERS_INVALID"}, "message": err.Error()})
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

	session, err := startTerminalSession(command, cols, rows)
	if err != nil {
		_ = writeTerminalError(ws, err)
		return
	}
	defer session.Close()

	// 输出泵与输入循环共享完成信号；任一方向断开都会终止子进程并释放 PTY。
	done := make(chan struct{})
	var once sync.Once
	finish := func() {
		once.Do(func() {
			close(done)
			session.Close()
		})
	}
	defer finish()
	go pumpTerminalOutput(ws, session.output, finish)
	if session.errOutput != nil {
		go pumpTerminalOutput(ws, session.errOutput, finish)
	}
	go func() { _ = command.Wait(); finish() }()

	for {
		opcode, payload, err := ws.readFrame()
		if err != nil {
			return
		}
		switch opcode {
		case 0x8:
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
			continue
		case 0x1:
			if err := handleTerminalInputWithResize(ws, session.input, payload, session.resizeFn); err != nil {
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

// terminalDimensions 解析并限制浏览器报告的终端列数和行数。
func terminalDimensions(r *http.Request) (int, int, error) {
	cols, rows := 80, 40
	if raw := strings.TrimSpace(r.URL.Query().Get("cols")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maxTerminalDimension {
			return 0, 0, errors.New("cols 必须是 1 到 500 之间的整数")
		}
		cols = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("rows")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maxTerminalDimension {
			return 0, 0, errors.New("rows 必须是 1 到 500 之间的整数")
		}
		rows = value
	}
	return cols, rows, nil
}

// terminalCommand 构造受限的本地、Docker 或 SSH 终端命令。
func terminalCommand(r *http.Request) (*exec.Cmd, error) {
	ctx := r.Context()
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
		// -i 与 -t 同时启用，使 Docker 会话接收控制序列和窗口尺寸。
		args := []string{"exec", "-i", "-t"}
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
			return nil, errors.New("host 参数不能为空且不能包含空白字符")
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
		// 仅使用 SSH 配置或 Agent 的非交互认证，绝不从 URL 接收密码或私钥。
		args := []string{"-tt", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-p", strconv.Itoa(port), target}
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

// validTerminalProgram 仅允许容器内单个可执行文件名，禁止注入 shell 参数。
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

// handleTerminalInput 保留旧调用签名，供非 PTY 测试和内部调用使用。
func handleTerminalInput(ws *streamWebSocket, stdin io.Writer, payload []byte) error {
	return handleTerminalInputWithResize(ws, stdin, payload, nil)
}

// handleTerminalInputWithResize 处理终端输入、心跳和窗口调整消息。
func handleTerminalInputWithResize(ws *streamWebSocket, stdin io.Writer, payload []byte, resize func(int, int) error) error {
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
		if stdin == nil {
			return writeTerminalError(ws, errors.New("终端输入通道不可用"))
		}
		_, err = stdin.Write(decoded)
		return err
	case "heartbeat":
		payload, _ := json.Marshal(terminalServerMessage{Type: "heartbeat", Timestamp: message.Timestamp})
		return ws.writeText(payload)
	case "resize":
		if message.Cols < 1 || message.Cols > maxTerminalDimension || message.Rows < 1 || message.Rows > maxTerminalDimension {
			return writeTerminalError(ws, errors.New("终端窗口尺寸必须在 1 到 500 之间"))
		}
		if resize == nil {
			return writeTerminalError(ws, errors.New("终端窗口调整不可用"))
		}
		if err := resize(message.Cols, message.Rows); err != nil {
			// 调整失败不终止会话，客户端可以继续使用当前尺寸。
			return writeTerminalError(ws, fmt.Errorf("调整终端窗口失败: %w", err))
		}
		return nil
	default:
		return writeTerminalError(ws, errors.New("终端消息类型不受支持"))
	}
}

// pumpTerminalOutput 将 PTY 或普通管道输出编码为前端约定的 base64 文本帧。
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
