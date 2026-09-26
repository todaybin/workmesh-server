// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

type scriptLibraryItem struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Script        string   `json:"script"`
	Description   string   `json:"description,omitempty"`
	Version       string   `json:"version,omitempty"`
	Approved      bool     `json:"approved"`
	Groups        string   `json:"groups,omitempty"`
	IsInteractive bool     `json:"isInteractive"`
	GroupList     []uint   `json:"groupList,omitempty"`
	GroupBelong   []string `json:"groupBelong,omitempty"`
	IsSystem      bool     `json:"isSystem"`
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt"`
}
type scriptLibraryStore struct {
	mu         sync.RWMutex
	db         *sql.DB
	repository storage.Transactional
	path       string
	items      []scriptLibraryItem
	initErr    error
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
		scriptStore.repository, scriptStore.initErr = storage.NewSQLiteRepository(scriptStore.db)
		if scriptStore.initErr != nil {
			return scriptStore
		}
		_, scriptStore.initErr = scriptStore.repository.Exec(`CREATE TABLE IF NOT EXISTS script_library (id TEXT PRIMARY KEY, name TEXT NOT NULL, script TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', version TEXT NOT NULL DEFAULT '', approved INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`)
		if scriptStore.initErr != nil {
			return scriptStore
		}
		for _, statement := range []string{
			`ALTER TABLE script_library ADD COLUMN groups TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE script_library ADD COLUMN is_interactive INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE script_library ADD COLUMN is_system INTEGER NOT NULL DEFAULT 0`,
		} {
			if _, err := scriptStore.repository.Exec(statement); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				scriptStore.initErr = err
				return scriptStore
			}
		}
		var count int
		_ = scriptStore.repository.QueryRow(`SELECT COUNT(*) FROM script_library`).Scan(&count)
		if count == 0 {
			if b, e := os.ReadFile(scriptStore.path); e == nil {
				var legacy []scriptLibraryItem
				if json.Unmarshal(b, &legacy) == nil {
					for _, it := range legacy {
						_, _ = scriptStore.repository.Exec(`INSERT OR IGNORE INTO script_library(id,name,script,description,version,approved,groups,is_interactive,is_system,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, it.ID, it.Name, it.Script, it.Description, it.Version, boolInt(it.Approved), it.Groups, boolInt(it.IsInteractive), boolInt(it.IsSystem), it.CreatedAt, it.UpdatedAt)
					}
				}
			}
		}
		rows, err := scriptStore.repository.Query(`SELECT id,name,script,description,version,approved,groups,is_interactive,is_system,created_at,updated_at FROM script_library ORDER BY name,id`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var it scriptLibraryItem
				var approved, interactive, system int
				if rows.Scan(&it.ID, &it.Name, &it.Script, &it.Description, &it.Version, &approved, &it.Groups, &interactive, &system, &it.CreatedAt, &it.UpdatedAt) == nil {
					it.Approved = approved != 0
					it.IsInteractive = interactive != 0
					it.IsSystem = system != 0
					it.GroupList = scriptGroupIDs(it.Groups)
					scriptStore.items = append(scriptStore.items, it)
				}
			}
			rows.Close()
		}
		if err := scriptStore.ensureSystemScripts(); err != nil {
			scriptStore.initErr = err
			return scriptStore
		}
	}
	return scriptStore
}
func (s *scriptLibraryStore) saveLocked() error {
	if s.initErr != nil || s.repository == nil {
		return errors.New("公共数据库未初始化")
	}
	return s.repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
		if _, err := tx.Exec(`DELETE FROM script_library`); err != nil {
			return err
		}
		for _, it := range s.items {
			if _, err := tx.Exec(`INSERT INTO script_library(id,name,script,description,version,approved,groups,is_interactive,is_system,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, it.ID, it.Name, it.Script, it.Description, it.Version, boolInt(it.Approved), it.Groups, boolInt(it.IsInteractive), boolInt(it.IsSystem), it.CreatedAt, it.UpdatedAt); err != nil {
				return err
			}
		}
		return nil
	})
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

// handleScriptRun 为管理员提供与脚本库兼容的交互式 WebSocket 执行通道。
func handleScriptRun(w http.ResponseWriter, r *http.Request) {
	if !authorizeScriptRun(w, r) {
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"websocket": true, "upgradeRequired": true}})
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
	if !script.Approved && !script.IsSystem {
		wmhttp.JSON(w, http.StatusForbidden, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_NOT_APPROVED"}})
		return
	}
	command := strings.TrimSpace(script.Script)
	if command == "" || len(command) > 64<<10 {
		wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_NOT_FOUND"}})
		return
	}
	cols, rows, err := terminalDimensions(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "TERMINAL_PARAMETERS_INVALID"}, "message": err.Error()})
		return
	}
	if _, err := exec.LookPath("bash"); err != nil {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_SHELL_UNAVAILABLE"}, "message": "目标节点未安装 bash"})
		return
	}
	ws, err := upgradeStreamWebSocket(w, r)
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "WEBSOCKET_ORIGIN_DENIED" {
			status = http.StatusForbidden
		} else if err.Error() == "WEBSOCKET_UNAVAILABLE" {
			status = http.StatusServiceUnavailable
		}
		wmhttp.JSON(w, status, map[string]any{"code": "ERR", "details": map[string]string{"errCode": err.Error()}, "message": err.Error()})
		return
	}
	defer ws.close()
	cmd := exec.CommandContext(r.Context(), "bash", "-c", command)
	session, err := startTerminalSession(cmd, cols, rows)
	if err != nil {
		_ = writeTerminalError(ws, err)
		return
	}
	defer session.Close()

	done := make(chan struct{})
	var once sync.Once
	finish := func() {
		once.Do(func() {
			close(done)
			session.Close()
		})
	}
	defer finish()
	go func() {
		<-done
		_ = ws.close()
	}()
	var pumps sync.WaitGroup
	pumps.Add(1)
	go func() {
		defer pumps.Done()
		pumpTerminalOutput(ws, session.output, func() {})
	}()
	if session.errOutput != nil {
		pumps.Add(1)
		go func() {
			defer pumps.Done()
			pumpTerminalOutput(ws, session.errOutput, func() {})
		}()
	}
	go func() {
		_ = cmd.Wait()
		pumps.Wait()
		finish()
	}()
	for {
		opcode, payload, readErr := ws.readFrame()
		if readErr != nil {
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

func authorizeScriptRun(w http.ResponseWriter, r *http.Request) bool {
	if IsForwardedRequestVerified(r) {
		return true
	}
	if sessionID := coreSessionID(r); sessionID != "" {
		if user, err := localCore.Current(sessionID); err == nil {
			if strings.EqualFold(strings.TrimSpace(user.Role), "ADMIN") {
				return true
			}
			wmhttp.JSON(w, http.StatusForbidden, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_ADMIN_REQUIRED"}, "message": "只有管理员可以运行脚本"})
			return false
		}
	}
	token := strings.TrimSpace(os.Getenv("WORKMESH_COMMAND_TOKEN"))
	if token != "" && r.Header.Get("X-WorkMesh-Token") == token {
		return true
	}
	wmhttp.JSON(w, http.StatusUnauthorized, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "LOCAL_AUTH_REQUIRED"}, "message": "需要有效的管理员登录会话"})
	return false
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
	in.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	in.UpdatedAt = in.CreatedAt
	if !in.Approved {
		in.Approved = false
	}
	in.IsSystem = false
	s := getScriptStore()
	s.mu.Lock()
	in.ID = strconv.FormatUint(uint64(nextScriptNumericID(s.items)), 10)
	previous := append([]scriptLibraryItem(nil), s.items...)
	s.items = append(s.items, in)
	err := s.saveLocked()
	if err != nil {
		s.items = previous
	}
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
		Info     string `json:"info"`
		GroupID  uint   `json:"groupID"`
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
	needle := strings.ToLower(strings.TrimSpace(q.Info))
	if needle == "" {
		needle = strings.ToLower(strings.TrimSpace(q.Name))
	}
	groupNames := scriptGroupNames()
	for _, it := range all {
		if q.GroupID != 0 && !scriptHasGroup(it.Groups, q.GroupID) {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(it.Name+" "+it.Description), needle) {
			continue
		}
		it.GroupList = scriptGroupIDs(it.Groups)
		it.GroupBelong = scriptBelongNames(it.GroupList, groupNames)
		filtered = append(filtered, it)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		left, right := scriptNumericID(filtered[i].ID), scriptNumericID(filtered[j].ID)
		if left == 0 || right == 0 {
			return filtered[i].ID < filtered[j].ID
		}
		return left < right
	})
	total := len(filtered)
	start := (q.Page - 1) * q.PageSize
	if start > total {
		start = total
	}
	end := start + q.PageSize
	if end > total {
		end = total
	}
	views := make([]map[string]any, 0, end-start)
	for _, it := range filtered[start:end] {
		views = append(views, scriptSearchView(it))
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": views, "total": total}})
}
func handleScriptUpdate(w http.ResponseWriter, r *http.Request) {
	var raw map[string]any
	if err := decodeJSON(r, &raw); err != nil || scriptIDString(raw["id"]) == "" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_ID_REQUIRED"}})
		return
	}
	in := scriptLibraryItem{ID: scriptIDString(raw["id"]), Name: runtimeString(raw, "name"), Script: runtimeString(raw, "script"), Description: runtimeString(raw, "description"), Version: runtimeString(raw, "version"), Groups: runtimeString(raw, "groups")}
	if value, ok := raw["approved"].(bool); ok {
		in.Approved = value
	}
	if value, ok := raw["isInteractive"].(bool); ok {
		in.IsInteractive = value
	}
	s := getScriptStore()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == in.ID {
			if s.items[i].IsSystem {
				wmhttp.JSON(w, http.StatusForbidden, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_SYSTEM_READONLY"}, "message": "系统脚本不能修改"})
				return
			}
			previous := s.items[i]
			if in.Name != "" {
				s.items[i].Name = in.Name
			}
			if in.Script != "" {
				s.items[i].Script = in.Script
			}
			s.items[i].Description = in.Description
			s.items[i].Version = in.Version
			s.items[i].Approved = in.Approved
			s.items[i].Groups = in.Groups
			s.items[i].IsInteractive = in.IsInteractive
			s.items[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if err := s.saveLocked(); err != nil {
				s.items[i] = previous
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
		ID  string `json:"id"`
		IDs []any  `json:"ids"`
	}
	if err := decodeJSON(r, &in); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR"})
		return
	}
	wanted := map[string]struct{}{}
	if strings.TrimSpace(in.ID) != "" {
		wanted[strings.TrimSpace(in.ID)] = struct{}{}
	}
	for _, item := range in.IDs {
		id := scriptIDString(item)
		if id != "" && id != "0" {
			wanted[id] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR"})
		return
	}
	s := getScriptStore()
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := make([]scriptLibraryItem, 0, len(s.items))
	removed := 0
	blocked := 0
	for _, it := range s.items {
		if _, ok := wanted[it.ID]; ok {
			if it.IsSystem {
				blocked++
				kept = append(kept, it)
				continue
			}
			removed++
			continue
		}
		kept = append(kept, it)
	}
	if removed == 0 {
		if blocked > 0 {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_SYSTEM_READONLY"}, "message": "系统脚本不能删除"})
			return
		}
		wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_NOT_FOUND"}})
		return
	}
	previous := s.items
	s.items = kept
	if err := s.saveLocked(); err != nil {
		s.items = previous
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200})
}

func scriptGroupIDs(groups string) []uint {
	ids := make([]uint, 0)
	for _, part := range strings.Split(groups, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.ParseUint(part, 10, 64)
		if err == nil && value > 0 {
			ids = append(ids, uint(value))
		}
	}
	return ids
}

func scriptHasGroup(groups string, id uint) bool {
	for _, item := range scriptGroupIDs(groups) {
		if item == id {
			return true
		}
	}
	return false
}

func scriptGroupNames() map[uint]string {
	names := map[uint]string{}
	for _, group := range currentGroupService().List("script") {
		id, _ := group.ID, group.Name
		if id > 0 {
			names[id] = group.Name
		}
	}
	return names
}

func scriptBelongNames(ids []uint, names map[uint]string) []string {
	belong := make([]string, 0, len(ids))
	for _, id := range ids {
		if name := names[id]; name != "" {
			belong = append(belong, name)
		}
	}
	return belong
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
	previous := append([]scriptLibraryItem(nil), store.items...)
	kept := make([]scriptLibraryItem, 0)
	for _, item := range previous {
		if item.IsSystem || systemScriptByName(item.Name) {
			kept = append(kept, item)
		}
	}
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
		if systemScriptByName(incoming[i].Name) {
			continue
		}
		kept = append(kept, incoming[i])
	}
	store.items = kept
	err = store.saveLocked()
	if err != nil {
		store.items = previous
	}
	store.mu.Unlock()
	if err != nil {
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SCRIPT_SAVE_FAILED"}, "message": err.Error()})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"count": len(incoming), "syncedAt": now}})
}
