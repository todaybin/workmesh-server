// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	mu      sync.RWMutex
	db      *sql.DB
	path    string
	items   []scriptLibraryItem
	initErr error
}

var scriptStoreMu sync.Mutex
var scriptStore *scriptLibraryStore

func getScriptStore() *scriptLibraryStore {
	scriptStoreMu.Lock()
	defer scriptStoreMu.Unlock()
	db := sharedDB()
	if scriptStore != nil && scriptStore.db == db {
		return scriptStore
	}
	{
		dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
		if dir == "" {
			dir = "./data"
		}
		scriptStore = &scriptLibraryStore{db: db, path: filepath.Join(dir, "scripts.json")}
		if scriptStore.db == nil {
			scriptStore.initErr = errors.New("公共数据库未初始化")
			return scriptStore
		}
		_, scriptStore.initErr = scriptStore.db.Exec(`CREATE TABLE IF NOT EXISTS script_library (id TEXT PRIMARY KEY, name TEXT NOT NULL, script TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', version TEXT NOT NULL DEFAULT '', approved INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`)
		if scriptStore.initErr != nil {
			return scriptStore
		}
		var count int
		_ = scriptStore.db.QueryRow(`SELECT COUNT(*) FROM script_library`).Scan(&count)
		if count == 0 {
			if b, e := os.ReadFile(scriptStore.path); e == nil {
				var legacy []scriptLibraryItem
				if json.Unmarshal(b, &legacy) == nil {
					for _, it := range legacy {
						_, _ = scriptStore.db.Exec(`INSERT OR IGNORE INTO script_library(id,name,script,description,version,approved,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, it.ID, it.Name, it.Script, it.Description, it.Version, boolInt(it.Approved), it.CreatedAt, it.UpdatedAt)
					}
				}
			}
		}
		rows, err := scriptStore.db.Query(`SELECT id,name,script,description,version,approved,created_at,updated_at FROM script_library ORDER BY name,id`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var it scriptLibraryItem
				var approved int
				if rows.Scan(&it.ID, &it.Name, &it.Script, &it.Description, &it.Version, &approved, &it.CreatedAt, &it.UpdatedAt) == nil {
					it.Approved = approved != 0
					scriptStore.items = append(scriptStore.items, it)
				}
			}
		}
	}
	return scriptStore
}
func (s *scriptLibraryStore) saveLocked() error {
	if s.initErr != nil || s.db == nil {
		return errors.New("公共数据库未初始化")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM script_library`); err != nil {
		return err
	}
	for _, it := range s.items {
		if _, err = tx.Exec(`INSERT INTO script_library(id,name,script,description,version,approved,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, it.ID, it.Name, it.Script, it.Description, it.Version, boolInt(it.Approved), it.CreatedAt, it.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func registerCoreResourceRoutes(mux *http.ServeMux) {
	// 脚本运行必须先经过受保护的专用处理器，不能落入普通资源 CRUD。
	mux.HandleFunc("GET /api/v2/core/script/run", handleScriptRun)
	for _, pattern := range []string{
		"POST /api/v2/core/groups",
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
		case "POST /api/v2/core/groups", "POST /api/v2/core/groups/update":
			mux.HandleFunc(pattern, handleGroupUpsert)
		case "POST /api/v2/core/groups/del":
			mux.HandleFunc(pattern, handleGroupDelete)
		case "POST /api/v2/core/groups/search":
			mux.HandleFunc(pattern, handleGroupSearch)
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
		}
	}
	for _, prefix := range []string{"/api/v2/core/commands/", "/api/v2/core/script/", "/api/v2/core/logs/", "/api/v2/core/groups/"} {
		mux.HandleFunc(prefix, coreResourceHandler)
	}
}

// handleScriptRun 在显式配置命令令牌且脚本已审核时执行短时脚本。
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
	if path == "/api/v2/core/script" || path == "/api/v2/core/groups" {
		return true
	}
	for _, prefix := range []string{"/api/v2/core/commands/", "/api/v2/core/script/", "/api/v2/core/logs/", "/api/v2/core/groups/"} {
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
	// 未接入真实仓储的旧资源必须明确返回 501，不能把内存临时列表当作成功结果。
	wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "NOT_IMPLEMENTED", "resource": key}, "message": "该资源尚未接入真实存储"})
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
			if err := s.saveLocked(); err != nil {
				s.items = append(s.items, scriptLibraryItem{})
				copy(s.items[i+1:], s.items[i:])
				s.items[i] = it
				wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
				return
			}
			wmhttp.JSON(w, 200, map[string]any{"code": 200})
			return
		}
	}
	wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_NOT_FOUND"}})
}
func handleScriptSync(w http.ResponseWriter, r *http.Request) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("WORKMESH_SCRIPT_REPO_URL")), "/")
	if base == "" {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_REPOSITORY_UNAVAILABLE"}, "message": "未配置脚本库远程地址"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base, nil)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_REPOSITORY_INVALID"}, "message": err.Error()})
		return
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_SYNC_FAILED"}, "message": err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "details": map[string]any{"errCode": "SCRIPT_SYNC_FAILED", "status": resp.StatusCode}, "message": fmt.Sprintf("脚本库远端返回 HTTP %d", resp.StatusCode)})
		return
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_SYNC_FAILED"}, "message": err.Error()})
		return
	}
	var incoming []scriptLibraryItem
	if err := json.Unmarshal(payload, &incoming); err != nil {
		var envelope struct {
			Scripts []scriptLibraryItem `json:"scripts"`
		}
		if e := json.Unmarshal(payload, &envelope); e != nil {
			wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_REPOSITORY_INVALID"}, "message": e.Error()})
			return
		}
		incoming = envelope.Scripts
	}
	if len(incoming) == 0 {
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_REPOSITORY_EMPTY"}, "message": "远端脚本库为空"})
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	store := getScriptStore()
	if store.initErr != nil || store.db == nil {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_STORAGE_UNAVAILABLE"}, "message": "脚本库 SQLite 未初始化"})
		return
	}
	store.mu.Lock()
	for i := range incoming {
		incoming[i].ID = strings.TrimSpace(incoming[i].ID)
		if incoming[i].ID == "" {
			incoming[i].ID = "remote-" + fmt.Sprintf("%d", i)
		}
		if strings.TrimSpace(incoming[i].Name) == "" || strings.TrimSpace(incoming[i].Script) == "" || len(incoming[i].Script) > 64<<10 {
			store.mu.Unlock()
			wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_REPOSITORY_INVALID"}, "message": "远端脚本字段无效"})
			return
		}
		if incoming[i].CreatedAt == "" {
			incoming[i].CreatedAt = now
		}
		incoming[i].UpdatedAt = now
	}
	store.items = incoming
	err = store.saveLocked()
	store.mu.Unlock()
	if err != nil {
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_SAVE_FAILED"}, "message": err.Error()})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"count": len(incoming), "syncedAt": now}})
}
