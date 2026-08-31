// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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
	mu    sync.RWMutex
	path  string
	items []quickCommand
}

var commandStoreMu sync.Mutex
var commandStoreInstance *commandStore

func getCommandStore() *commandStore {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = "./data"
	}
	path := filepath.Join(dir, "commands.json")
	commandStoreMu.Lock()
	defer commandStoreMu.Unlock()
	if commandStoreInstance != nil && commandStoreInstance.path == path {
		return commandStoreInstance
	}
	s := &commandStore{path: path, items: []quickCommand{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &s.items)
	}
	commandStoreInstance = s
	return s
}

func (s *commandStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	b, err := json.Marshal(s.items)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
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
	mux.HandleFunc("POST /api/v2/core/commands", func(w http.ResponseWriter, r *http.Request) {
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
		s.items = append(s.items, in)
		err := s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			commandError(w, http.StatusInternalServerError, "PERSIST_FAILED", err)
			return
		}
		commandSuccess(w, in)
	})

	mux.HandleFunc("POST /api/v2/core/commands/list", func(w http.ResponseWriter, r *http.Request) {
		var filter struct {
			Type string `json:"type"`
		}
		_ = decodeJSON(r, &filter)
		items := s.list(filter.Type, "", 0, 0)
		commandSuccess(w, items)
	})
	mux.HandleFunc("POST /api/v2/core/commands/search", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Info     string `json:"info"`
			Name     string `json:"name"`
			Type     string `json:"type"`
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
		items := s.list(q.Type, firstNonEmpty(q.Info, q.Name), q.Page, q.PageSize)
		s.mu.RLock()
		total := 0
		needle := strings.ToLower(firstNonEmpty(q.Info, q.Name))
		for _, item := range s.items {
			if (q.Type == "" || item.Type == q.Type) && (needle == "" || strings.Contains(strings.ToLower(item.Name+" "+item.Command), needle)) {
				total++
			}
		}
		s.mu.RUnlock()
		commandSuccess(w, map[string]any{"items": items, "total": total, "page": q.Page, "pageSize": q.PageSize})
	})
	mux.HandleFunc("POST /api/v2/core/commands/tree", func(w http.ResponseWriter, r *http.Request) {
		var filter struct {
			Type string `json:"type"`
		}
		_ = decodeJSON(r, &filter)
		items := s.list(filter.Type, "", 0, 0)
		groups := map[uint]map[string]any{}
		for _, item := range items {
			g := groups[item.GroupID]
			if g == nil {
				g = map[string]any{"id": item.GroupID, "name": item.GroupBelong, "children": []quickCommand{}}
				groups[item.GroupID] = g
			}
			g["children"] = append(g["children"].([]quickCommand), item)
		}
		result := make([]map[string]any, 0, len(groups))
		for _, g := range groups {
			result = append(result, g)
		}
		commandSuccess(w, result)
	})
	mux.HandleFunc("POST /api/v2/core/commands/update", func(w http.ResponseWriter, r *http.Request) { commandUpdate(w, r, s) })
	mux.HandleFunc("POST /api/v2/core/commands/del", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			IDs []uint `json:"ids"`
			ID  string `json:"id"`
		}
		if err := decodeJSON(r, &in); err != nil {
			commandError(w, 400, "INVALID_JSON", err)
			return
		}
		removed := 0
		s.mu.Lock()
		kept := s.items[:0]
		for _, item := range s.items {
			match := false
			for _, id := range in.IDs {
				if strconv.FormatUint(uint64(id), 10) == item.ID {
					match = true
				}
			}
			if in.ID != "" && in.ID == item.ID {
				match = true
			}
			if match {
				removed++
			} else {
				kept = append(kept, item)
			}
		}
		s.items = kept
		err := s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			commandError(w, 500, "PERSIST_FAILED", err)
			return
		}
		commandSuccess(w, map[string]any{"deleted": removed})
	})
	mux.HandleFunc("POST /api/v2/core/commands/export", func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		b, err := json.Marshal(s.items)
		s.mu.RUnlock()
		if err != nil {
			commandError(w, 500, "EXPORT_FAILED", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	})
	mux.HandleFunc("POST /api/v2/core/commands/import", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Items    []quickCommand `json:"items"`
			Commands []quickCommand `json:"commands"`
		}
		if err := decodeJSON(r, &in); err != nil {
			commandError(w, 400, "INVALID_JSON", err)
			return
		}
		items := in.Items
		if len(items) == 0 {
			items = in.Commands
		}
		n := 0
		for _, item := range items {
			if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Command) == "" {
				continue
			}
			item.ID = strconv.FormatInt(time.Now().UnixNano()+int64(n), 10)
			if item.Type == "" {
				item.Type = "command"
			}
			s.mu.Lock()
			s.items = append(s.items, item)
			s.mu.Unlock()
			n++
		}
		s.mu.Lock()
		err := s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			commandError(w, 500, "PERSIST_FAILED", err)
			return
		}
		commandSuccess(w, map[string]any{"imported": n})
	})
	mux.HandleFunc("POST /api/v2/core/commands/upload", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			commandError(w, 400, "INVALID_MULTIPART", err)
			return
		}
		files := r.MultipartForm.File["file"]
		if len(files) == 0 {
			commandError(w, 400, "FILE_REQUIRED", errors.New("file is required"))
			return
		}
		f, err := files[0].Open()
		if err != nil {
			commandError(w, 400, "FILE_OPEN_FAILED", err)
			return
		}
		defer f.Close()
		reader := csv.NewReader(io.LimitReader(f, 4<<20))
		_, _ = reader.Read()
		result := make([]quickCommand, 0)
		for {
			row, e := reader.Read()
			if e == io.EOF {
				break
			}
			if e != nil {
				commandError(w, 400, "CSV_INVALID", e)
				return
			}
			if len(row) < 2 || strings.TrimSpace(row[0]) == "" || strings.TrimSpace(row[1]) == "" {
				continue
			}
			result = append(result, quickCommand{Name: row[0], Type: "command", Command: row[1]})
		}
		commandSuccess(w, result)
	})
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
			in.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if in.Type == "" {
				in.Type = s.items[i].Type
			}
			if in.CreatedAt == "" {
				in.CreatedAt = s.items[i].CreatedAt
			}
			s.items[i] = in
			if err := s.saveLocked(); err != nil {
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
