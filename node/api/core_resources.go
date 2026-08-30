// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

type coreResourceStore struct {
	mu      sync.RWMutex
	items   map[string][]map[string]any
	updated time.Time
}

var coreResources = coreResourceStore{items: map[string][]map[string]any{}}

func registerCoreResourceRoutes(mux *http.ServeMux) {
	// 脚本运行必须先经过受保护的专用处理器，不能落入普通资源 CRUD。
	mux.HandleFunc("GET /api/v2/core/script/run", handleScriptRun)
	for _, pattern := range []string{
		"POST /api/v2/groups/del",
		"POST /api/v2/groups/search",
		"POST /api/v2/groups/update",
		"POST /api/v2/core/groups/del",
		"POST /api/v2/core/groups/search",
		"POST /api/v2/core/groups/update",
		"POST /api/v2/core/script",
		"POST /api/v2/core/script/search",
		"POST /api/v2/core/script/update",
		"POST /api/v2/core/script/del",
		"POST /api/v2/core/script/sync",
	} {
		mux.HandleFunc(pattern, coreResourceHandler)
	}
	for _, prefix := range []string{"/api/v2/core/commands/", "/api/v2/core/script/", "/api/v2/core/logs/", "/api/v2/core/groups/", "/api/v2/groups/"} {
		mux.HandleFunc(prefix, coreResourceHandler)
	}
}

// handleScriptRun 在显式配置命令令牌时执行短时脚本；未配置令牌时拒绝执行，避免
// 迁移期间把脚本接口意外暴露成任意命令执行入口。
func handleScriptRun(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(os.Getenv("WORKMESH_COMMAND_TOKEN"))
	if token == "" || r.Header.Get("X-WorkMesh-Token") != token {
		wmhttp.JSON(w, http.StatusUnauthorized, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "COMMAND_AUTH_REQUIRED"}})
		return
	}
	command := strings.TrimSpace(r.URL.Query().Get("command"))
	if command == "" {
		command = strings.TrimSpace(r.URL.Query().Get("script"))
	}
	if command == "" {
		var body struct {
			Command string `json:"command"`
			Script  string `json:"script"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body)
		}
		command, _ = strings.CutPrefix(strings.TrimSpace(body.Command), "")
		if command == "" {
			command = strings.TrimSpace(body.Script)
		}
	}
	if command == "" || len(command) > 64<<10 {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "COMMAND_REQUIRED"}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	output, err := cmd.CombinedOutput()
	data := map[string]any{"command": command, "output": string(output), "exitCode": 0, "timedOut": errors.Is(ctx.Err(), context.DeadlineExceeded)}
	if err != nil {
		data["exitCode"] = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			data["exitCode"] = exitErr.ExitCode()
		}
	}
	status := http.StatusOK
	if err != nil {
		status = http.StatusUnprocessableEntity
		wmhttp.JSON(w, status, map[string]any{"code": "ERR", "details": map[string]any{"errCode": "SCRIPT_FAILED", "exitCode": data["exitCode"], "timedOut": data["timedOut"]}, "data": data})
		return
	}
	wmhttp.JSON(w, status, map[string]any{"code": 200, "data": data})
}

func isCoreResourceRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	if path == "/api/v2/core/script" {
		return true
	}
	for _, prefix := range []string{"/api/v2/core/commands/", "/api/v2/core/script/", "/api/v2/core/logs/", "/api/v2/core/groups/", "/api/v2/groups/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func isCoreAuthRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return strings.HasPrefix(path, "/api/v2/core/auth/")
}

func coreResourceHandler(w http.ResponseWriter, r *http.Request) {
	key := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/core/"), "/")
	if key == "" {
		wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "RESOURCE_NOT_FOUND"}})
		return
	}
	if r.Method == http.MethodGet || strings.HasSuffix(key, "/search") || strings.HasSuffix(key, "/list") || strings.HasSuffix(key, "/tree") {
		writeCoreResourceList(w, key)
		return
	}
	var payload map[string]any
	if r.Body != nil {
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&payload); err != nil && err != io.EOF {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "INVALID_JSON"}, "message": err.Error()})
			return
		}
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if _, ok := payload["id"]; !ok {
		payload["id"] = "core-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	payload["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	coreResources.mu.Lock()
	if strings.HasSuffix(key, "/del") || strings.HasSuffix(key, "/delete") {
		delete(coreResources.items, key)
	} else {
		coreResources.items[key] = append(coreResources.items[key], payload)
	}
	coreResources.updated = time.Now().UTC()
	coreResources.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": payload})
}

func writeCoreResourceList(w http.ResponseWriter, key string) {
	coreResources.mu.RLock()
	items := append([]map[string]any(nil), coreResources.items[key]...)
	coreResources.mu.RUnlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": 50}})
}
