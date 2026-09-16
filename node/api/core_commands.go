// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	_ "modernc.org/sqlite"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// quickCommand 是快捷命令的持久化模型，字段保持与旧前端兼容。
type quickCommand struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Command     string `json:"command"`
	GroupID     uint   `json:"groupID"`
	GroupBelong string `json:"groupBelong"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type commandStore struct {
	mu         sync.RWMutex
	db         *sql.DB
	repository storage.Transactional
	items      []quickCommand
}

var commandStoreMu sync.Mutex
var commandStoreInstance *commandStore

func getCommandStore() *commandStore {
	commandStoreMu.Lock()
	defer commandStoreMu.Unlock()
	db := sharedDB()
	if db == nil {
		fallback, _ := sql.Open("sqlite", ":memory:")
		db = fallback
	}
	if commandStoreInstance != nil && commandStoreInstance.db == db {
		return commandStoreInstance
	}
	repository, err := storage.NewSQLiteRepository(db)
	s := &commandStore{db: db, repository: repository, items: []quickCommand{}}
	if err != nil {
		commandStoreInstance = s
		return s
	}
	if _, err := repository.Exec(`CREATE TABLE IF NOT EXISTS node_quick_commands (id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL DEFAULT 'command', command TEXT NOT NULL, group_id INTEGER NOT NULL DEFAULT 0, group_belong TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', payload BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		commandStoreInstance = s
		return s
	}
	rows, _ := repository.Query(`SELECT id,name,type,command,group_id,group_belong,description,payload,created_at,updated_at FROM node_quick_commands ORDER BY name,id`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var item quickCommand
			var payload []byte
			_ = rows.Scan(&item.ID, &item.Name, &item.Type, &item.Command, &item.GroupID, &item.GroupBelong, &item.Description, &payload, &item.CreatedAt, &item.UpdatedAt)
			_ = json.Unmarshal(payload, &item)
			s.items = append(s.items, item)
		}
	}
	commandStoreInstance = s
	return s
}

func (s *commandStore) saveLocked() error {
	if s.repository == nil {
		return errors.New("公共数据库未初始化")
	}
	return s.repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
		if _, err := tx.Exec("DELETE FROM node_quick_commands"); err != nil {
			return err
		}
		for _, item := range s.items {
			payload, _ := json.Marshal(item)
			if _, err := tx.Exec(`INSERT INTO node_quick_commands(id,name,type,command,group_id,group_belong,description,payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, item.ID, item.Name, item.Type, item.Command, item.GroupID, item.GroupBelong, item.Description, payload, item.CreatedAt, item.UpdatedAt); err != nil {
				return err
			}
		}
		return nil
	})
}

func isCoreCommandRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return path == "/api/v2/core/commands" || strings.HasPrefix(path, "/api/v2/core/commands/")
}

// registerCoreCommandRoutes 注册旧 core/commands 的全部真实行为。
func registerCoreCommandRoutes(mux *http.ServeMux) {
	s := getCommandStore()
	mux.HandleFunc("POST /api/v2/core/commands", func(w http.ResponseWriter, r *http.Request) { commandCreate(w, r, s) })
	mux.HandleFunc("POST /api/v2/core/commands/list", func(w http.ResponseWriter, r *http.Request) { commandList(w, r, s) })
	mux.HandleFunc("POST /api/v2/core/commands/search", func(w http.ResponseWriter, r *http.Request) { commandSearch(w, r, s) })
	mux.HandleFunc("POST /api/v2/core/commands/tree", func(w http.ResponseWriter, r *http.Request) { commandTree(w, r, s) })
	mux.HandleFunc("POST /api/v2/core/commands/update", func(w http.ResponseWriter, r *http.Request) { commandUpdate(w, r, s) })
	mux.HandleFunc("POST /api/v2/core/commands/del", func(w http.ResponseWriter, r *http.Request) { commandDelete(w, r, s) })
	mux.HandleFunc("POST /api/v2/core/commands/export", func(w http.ResponseWriter, _ *http.Request) { commandExport(w, s) })
	mux.HandleFunc("POST /api/v2/core/commands/import", func(w http.ResponseWriter, r *http.Request) { commandImport(w, r, s) })
	mux.HandleFunc("POST /api/v2/core/commands/upload", commandUpload)
}

// commandCreate 校验并创建一条快捷命令。
func commandCreate(w http.ResponseWriter, r *http.Request, s *commandStore) {
	var in quickCommand
	if err := decodeJSON(r, &in); err != nil {
		commandError(w, http.StatusBadRequest, "INVALID_JSON", err)
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Command) == "" {
		commandError(w, http.StatusBadRequest, "INVALID_COMMAND", errors.New("name and command are required"))
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	in.ID = strconv.FormatInt(time.Now().UnixNano(), 10)
	if in.Type == "" {
		in.Type = "command"
	}
	in.CreatedAt, in.UpdatedAt = now, now
	s.mu.Lock()
	previous := append([]quickCommand(nil), s.items...)
	s.items = append(s.items, in)
	err := s.saveLocked()
	if err != nil {
		s.items = previous
	}
	s.mu.Unlock()
	if err != nil {
		commandError(w, http.StatusInternalServerError, "PERSIST_FAILED", err)
		return
	}
	commandSuccess(w, in)
}

// commandList 返回可选类型过滤后的完整快捷命令列表。
func commandList(w http.ResponseWriter, r *http.Request, s *commandStore) {
	var filter struct {
		Type string `json:"type"`
	}
	_ = decodeJSON(r, &filter)
	commandSuccess(w, s.list(filter.Type, "", 0, 0))
}

// commandSearch 按名称和命令内容分页查询快捷命令。
func commandSearch(w http.ResponseWriter, r *http.Request, s *commandStore) {
	var query struct {
		Info     string `json:"info"`
		Name     string `json:"name"`
		Type     string `json:"type"`
		Page     int    `json:"page"`
		PageSize int    `json:"pageSize"`
	}
	_ = decodeJSON(r, &query)
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 200 {
		query.PageSize = 20
	}
	needle := strings.ToLower(firstNonEmpty(query.Info, query.Name))
	items := s.list(query.Type, needle, query.Page, query.PageSize)
	s.mu.RLock()
	total := 0
	for _, item := range s.items {
		if (query.Type == "" || item.Type == query.Type) && (needle == "" || strings.Contains(strings.ToLower(item.Name+" "+item.Command), needle)) {
			total++
		}
	}
	s.mu.RUnlock()
	commandSuccess(w, map[string]any{"items": items, "total": total, "page": query.Page, "pageSize": query.PageSize})
}

// commandTree 按分组构造前端需要的快捷命令树。
func commandTree(w http.ResponseWriter, r *http.Request, s *commandStore) {
	var filter struct {
		Type string `json:"type"`
	}
	_ = decodeJSON(r, &filter)
	groups := map[uint]map[string]any{}
	for _, item := range s.list(filter.Type, "", 0, 0) {
		group := groups[item.GroupID]
		if group == nil {
			group = map[string]any{"id": item.GroupID, "name": item.GroupBelong, "children": []quickCommand{}}
			groups[item.GroupID] = group
		}
		group["children"] = append(group["children"].([]quickCommand), item)
	}
	result := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		result = append(result, group)
	}
	commandSuccess(w, result)
}

// commandDelete 删除请求中列出的快捷命令并持久化一次快照。
func commandDelete(w http.ResponseWriter, r *http.Request, s *commandStore) {
	var in struct {
		IDs []uint `json:"ids"`
		ID  string `json:"id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		commandError(w, http.StatusBadRequest, "INVALID_JSON", err)
		return
	}
	removed := 0
	s.mu.Lock()
	previous := append([]quickCommand(nil), s.items...)
	kept := s.items[:0]
	for _, item := range s.items {
		match := in.ID != "" && in.ID == item.ID
		for _, id := range in.IDs {
			match = match || strconv.FormatUint(uint64(id), 10) == item.ID
		}
		if match {
			removed++
		} else {
			kept = append(kept, item)
		}
	}
	s.items = kept
	err := s.saveLocked()
	if err != nil {
		s.items = previous
	}
	s.mu.Unlock()
	if err != nil {
		commandError(w, http.StatusInternalServerError, "PERSIST_FAILED", err)
		return
	}
	commandSuccess(w, map[string]any{"deleted": removed})
}

// commandExport 将当前快捷命令快照导出为 JSON。
func commandExport(w http.ResponseWriter, s *commandStore) {
	s.mu.RLock()
	payload, err := json.Marshal(s.items)
	s.mu.RUnlock()
	if err != nil {
		commandError(w, http.StatusInternalServerError, "EXPORT_FAILED", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

// commandImport 导入有效快捷命令，并在批次结束后统一持久化。
func commandImport(w http.ResponseWriter, r *http.Request, s *commandStore) {
	var in struct {
		Items    []quickCommand `json:"items"`
		Commands []quickCommand `json:"commands"`
	}
	if err := decodeJSON(r, &in); err != nil {
		commandError(w, http.StatusBadRequest, "INVALID_JSON", err)
		return
	}
	items := in.Items
	if len(items) == 0 {
		items = in.Commands
	}
	valid := normalizeImportedCommands(items)
	s.mu.Lock()
	previous := append([]quickCommand(nil), s.items...)
	s.items = append(s.items, valid...)
	err := s.saveLocked()
	if err != nil {
		s.items = previous
	}
	s.mu.Unlock()
	if err != nil {
		commandError(w, http.StatusInternalServerError, "PERSIST_FAILED", err)
		return
	}
	commandSuccess(w, map[string]any{"imported": len(valid)})
}

// normalizeImportedCommands 过滤无效导入项并补齐 ID 和类型。
func normalizeImportedCommands(items []quickCommand) []quickCommand {
	result := make([]quickCommand, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Command) == "" {
			continue
		}
		item.ID = strconv.FormatInt(time.Now().UnixNano()+int64(len(result)), 10)
		if item.Type == "" {
			item.Type = "command"
		}
		result = append(result, item)
	}
	return result
}

// commandUpload 解析最多 4 MiB 的 CSV 文件并返回待导入命令。
func commandUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		commandError(w, http.StatusBadRequest, "INVALID_MULTIPART", err)
		return
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		commandError(w, http.StatusBadRequest, "FILE_REQUIRED", errors.New("file is required"))
		return
	}
	file, err := files[0].Open()
	if err != nil {
		commandError(w, http.StatusBadRequest, "FILE_OPEN_FAILED", err)
		return
	}
	defer file.Close()
	reader := csv.NewReader(io.LimitReader(file, 4<<20))
	_, _ = reader.Read()
	result := make([]quickCommand, 0)
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			commandError(w, http.StatusBadRequest, "CSV_INVALID", err)
			return
		}
		if len(row) >= 2 && strings.TrimSpace(row[0]) != "" && strings.TrimSpace(row[1]) != "" {
			result = append(result, quickCommand{Name: row[0], Type: "command", Command: row[1]})
		}
	}
	commandSuccess(w, result)
}

func (s *commandStore) list(typ, query string, page, pageSize int) []quickCommand {
	s.mu.RLock()
	defer s.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]quickCommand, 0)
	for _, item := range s.items {
		if typ != "" && item.Type != typ {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(item.Name+" "+item.Command), query) {
			continue
		}
		result = append(result, item)
	}
	if page > 0 {
		start := (page - 1) * pageSize
		if start >= len(result) {
			return []quickCommand{}
		}
		end := start + pageSize
		if end > len(result) {
			end = len(result)
		}
		return result[start:end]
	}
	return result
}

func commandUpdate(w http.ResponseWriter, r *http.Request, s *commandStore) {
	var in quickCommand
	if err := decodeJSON(r, &in); err != nil {
		commandError(w, 400, "INVALID_JSON", err)
		return
	}
	if in.ID == "" || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Command) == "" {
		commandError(w, 400, "INVALID_COMMAND", errors.New("id, name and command are required"))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == in.ID {
			previous := s.items[i]
			in.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if in.Type == "" {
				in.Type = s.items[i].Type
			}
			if in.CreatedAt == "" {
				in.CreatedAt = s.items[i].CreatedAt
			}
			s.items[i] = in
			if err := s.saveLocked(); err != nil {
				s.items[i] = previous
				commandError(w, 500, "PERSIST_FAILED", err)
				return
			}
			commandSuccess(w, in)
			return
		}
	}
	commandError(w, 404, "COMMAND_NOT_FOUND", errors.New("command not found"))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
func commandSuccess(w http.ResponseWriter, data any) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}
func commandError(w http.ResponseWriter, status int, code string, err error) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error(), "details": map[string]string{"errCode": code}})
}
