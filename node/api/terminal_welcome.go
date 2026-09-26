// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"strings"
)

func isLocalInteractiveTerminal(r *http.Request) bool {
	if r == nil || runtime.GOOS == "windows" {
		return false
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	return strings.HasSuffix(path, "/local") && strings.TrimSpace(r.URL.Query().Get("command")) == ""
}

func terminalShellEnvironment() []string {
	env := os.Environ()
	hasTerm := false
	for _, item := range env {
		if strings.HasPrefix(item, "TERM=") && strings.TrimPrefix(item, "TERM=") != "" {
			hasTerm = true
			break
		}
	}
	if !hasTerm {
		env = append(env, "TERM=xterm-256color")
	}
	return env
}

// localTerminalWelcome 输出系统 MOTD。1Panel 的本地终端走 SSH，欢迎信息由登录会话带出。
func localTerminalWelcome() string {
	var builder strings.Builder
	host, _ := os.Hostname()
	user := strings.TrimSpace(os.Getenv("USER"))
	if user == "" {
		user = strings.TrimSpace(os.Getenv("LOGNAME"))
	}
	if user == "" {
		user = "root"
	}
	builder.WriteString("Welcome to WorkMesh\r\n")
	builder.WriteString("System: " + runtime.GOOS + "  Host: " + host + "  User: " + user + "\r\n")
	for _, path := range []string{"/run/motd.dynamic", "/etc/motd"} {
		content, err := os.ReadFile(path)
		text := strings.TrimSpace(string(content))
		if err != nil || text == "" {
			continue
		}
		builder.WriteString(strings.ReplaceAll(text, "\n", "\r\n"))
		builder.WriteString("\r\n")
		break
	}
	builder.WriteString("\r\n")
	return builder.String()
}

func writeTerminalText(ws *streamWebSocket, text string) error {
	if ws == nil || text == "" {
		return nil
	}
	message, err := json.Marshal(terminalServerMessage{Type: "cmd", Data: base64.StdEncoding.EncodeToString([]byte(text))})
	if err != nil {
		return err
	}
	return ws.writeText(message)
}
