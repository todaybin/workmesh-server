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
	"path/filepath"
	"strings"
	"sync"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

type scriptLibraryItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Script      string `json:"script"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	Approved    bool   `json:"approved"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}
type scriptLibraryStore struct {
	mu    sync.RWMutex
	path  string
	items []scriptLibraryItem
}

var scriptStoreOnce sync.Once
var scriptStore *scriptLibraryStore

func getScriptStore() *scriptLibraryStore {
	scriptStoreOnce.Do(func() {
		dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
		if dir == "" {
			dir = ".workmesh-data"
		}
		scriptStore = &scriptLibraryStore{path: filepath.Join(dir, "scripts.json")}
		if b, e := os.ReadFile(scriptStore.path); e == nil {
			_ = json.Unmarshal(b, &scriptStore.items)
		}
	})
	return scriptStore
}
func (s *scriptLibraryStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	b, e := json.Marshal(s.items)
	if e != nil {
		return e
	}
	tmp := s.path + ".tmp"
	if e = os.WriteFile(tmp, b, 0o600); e != nil {
		return e
	}
	return os.Rename(tmp, s.path)
}

type coreResourceStore struct {
	mu      sync.RWMutex
	items   map[string][]map[string]any
	updated time.Time
}

var coreResources = coreResourceStore{items: make(map[string][]map[string]any)}

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
		switch pattern {
		case "POST /api/v2/core/script":
			mux.HandleFunc(pattern, handleScriptCreate)
		case "POST /api/v2/core/script/search":
			mux.HandleFunc(pattern, handleScriptSearch)
		case "POST /api/v2/core/script/update":
			mux.HandleFunc(pattern, handleScriptUpdate)
		case "POST /api/v2/core/script/del":
			mux.HandleFunc(pattern, handleScriptDelete)
		case "POST /api/v2/core/script/sync":
			mux.HandleFunc(pattern, handleScriptSync)
		default:
			mux.HandleFunc(pattern, coreResourceHandler)
		}
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
	scriptID := strings.TrimSpace(r.URL.Query().Get("script_id"))
	if scriptID == "" {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_ID_REQUIRED"}})
		return
	}
	store := getScriptStore()
	store.mu.RLock()
	var script scriptLibraryItem
	for _, item := range store.items {
		if item.ID == scriptID {
			script = item
			break
		}
	}
	store.mu.RUnlock()
	if !script.Approved {
		wmhttp.JSON(w, http.StatusForbidden, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_NOT_APPROVED"}})
		return
	}
	command := strings.TrimSpace(script.Script)
	if command == "" || len(command) > 64<<10 {
		wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_NOT_FOUND"}})
		return
	}
	if strings.ContainsAny(command, ";|&><`$") {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_UNSAFE"}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	output, err := cmd.CombinedOutput()
	data := map[string]any{"scriptId": scriptID, "output": string(output), "exitCode": 0, "timedOut": errors.Is(ctx.Err(), context.DeadlineExceeded)}
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

func handleScriptCreate(w http.ResponseWriter, r *http.Request) {
	var in scriptLibraryItem
	if err := decodeJSON(r, &in); err != nil || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Script) == "" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_INVALID"}})
		return
	}
	in.ID = "script-" + time.Now().UTC().Format("20060102150405.000000000")
	in.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	in.UpdatedAt = in.CreatedAt
	if !in.Approved {
		in.Approved = false
	}
	s := getScriptStore()
	s.mu.Lock()
	s.items = append(s.items, in)
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": in})
}
func handleScriptSearch(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Name     string `json:"name"`
		Page     int    `json:"page"`
		PageSize int    `json:"pageSize"`
	}
	_ = decodeJSON(r, &q)
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 200 {
		q.PageSize = 20
	}
	s := getScriptStore()
	s.mu.RLock()
	all := append([]scriptLibraryItem(nil), s.items...)
	s.mu.RUnlock()
	filtered := make([]scriptLibraryItem, 0)
	needle := strings.ToLower(strings.TrimSpace(q.Name))
	for _, it := range all {
		if needle == "" || strings.Contains(strings.ToLower(it.Name), needle) {
			filtered = append(filtered, it)
		}
	}
	total := len(filtered)
	start := (q.Page - 1) * q.PageSize
	if start > total {
		start = total
	}
	end := start + q.PageSize
	if end > total {
		end = total
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": filtered[start:end], "total": total, "page": q.Page, "pageSize": q.PageSize}})
}
func handleScriptUpdate(w http.ResponseWriter, r *http.Request) {
	var in scriptLibraryItem
	if err := decodeJSON(r, &in); err != nil || in.ID == "" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_ID_REQUIRED"}})
		return
	}
	s := getScriptStore()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == in.ID {
			if in.Name != "" {
				s.items[i].Name = in.Name
			}
			if in.Script != "" {
				s.items[i].Script = in.Script
			}
			s.items[i].Description = in.Description
			s.items[i].Version = in.Version
			s.items[i].Approved = in.Approved
			s.items[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if err := s.saveLocked(); err != nil {
				wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
				return
			}
			wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": s.items[i]})
			return
		}
	}
	wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_NOT_FOUND"}})
}
func handleScriptDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(r, &in); err != nil || in.ID == "" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR"})
		return
	}
	s := getScriptStore()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, it := range s.items {
		if it.ID == in.ID {
			s.items = append(s.items[:i], s.items[i+1:]...)
			_ = s.saveLocked()
			wmhttp.JSON(w, 200, map[string]any{"code": 200})
			return
		}
	}
	wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_NOT_FOUND"}})
}
func handleScriptSync(w http.ResponseWriter, r *http.Request) {
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"synced": 0, "source": "local"}})
}
