// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

func handleFirewallNativeDetail(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Provider   string `json:"provider"`
		NativeKind string `json:"nativeKind"`
		Name       string `json:"name"`
		Permanent  bool   `json:"permanent"`
	}
	if err := decodeJSON(r, &request); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	provider := strings.ToLower(strings.TrimSpace(request.Provider))
	name := strings.TrimSpace(request.Name)
	if name == "" || len(name) > 128 || strings.ContainsAny(name, "\x00\r\n;|&$`\\") {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "原生规则名称无效"})
		return
	}
	var command string
	var args []string
	switch provider {
	case "firewalld":
		if request.NativeKind != "zone_service" && request.NativeKind != "rich_rule" {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "firewalld 原生类型无效"})
			return
		}
		command = "firewall-cmd"
		args = []string{"--zone=public", "--query-service=" + name}
		if request.NativeKind == "rich_rule" {
			args = []string{"--zone=public", "--query-rich-rule=" + name}
		}
		if request.Permanent {
			args = append([]string{"--permanent"}, args...)
		}
	case "ufw":
		if request.NativeKind != "ufw_application" {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "ufw 原生类型无效"})
			return
		}
		command = "ufw"
		args = []string{"app", "info", name}
	default:
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "不支持的防火墙后端"})
		return
	}
	path, err := exec.LookPath(command)
	if err != nil {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": fmt.Sprintf("%s 不可用: %v", command, err)})
		return
	}
	result, runErr := runCommandWithPath(r, path, args...)
	if runErr != nil && strings.TrimSpace(result) == "" {
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": runErr.Error()})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"provider": provider, "nativeKind": request.NativeKind, "name": name, "permanent": request.Permanent, "raw": result, "exists": runErr == nil}})
}

func runCommandWithPath(r *http.Request, path string, args ...string) (string, error) {
	ctx := r.Context()
	command := exec.CommandContext(ctx, path, args...)
	out, err := command.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil && text != "" {
		return text, fmt.Errorf("命令执行失败: %s", text)
	}
	return text, err
}
