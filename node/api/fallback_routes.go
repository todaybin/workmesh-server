// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// fallbackStore 保存尚未接入专用领域模型的契约数据，确保重启后不会丢失用户写入。
// 专用领域处理器注册后会优先命中，不会经过此入口。
type fallbackStore struct {
	mu     sync.RWMutex
	path   string
	loaded bool
	data   map[string][]map[string]any
}

var legacyStore = fallbackStore{}

func registerFallbackRoute(mux *http.ServeMux, pattern string) {
	mux.HandleFunc(pattern, fallbackRouteHandler)
}

func (s *fallbackStore) loadLocked() {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = ".workmesh-data"
	}
	path := filepath.Join(dir, "fallback-state.json")
	if s.loaded && s.path == path {
		return
	}
	s.path, s.loaded = path, true
	s.data = make(map[string][]map[string]any)
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &s.data)
	}
}

func (s *fallbackStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	raw, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// fallbackRouteHandler 处理尚未接入专用领域模型的契约，提供受限持久化 CRUD。
// 所有写入都限制请求体大小和单路由记录数，避免兜底入口形成无界资源占用。
func fallbackRouteHandler(w http.ResponseWriter, r *http.Request) {
	key := r.Method + " " + r.URL.Path
	var payload map[string]any
	if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodDelete {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
		if err := decoder.Decode(&payload); err != nil && !strings.Contains(err.Error(), "EOF") {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "INVALID_JSON"}, "message": err.Error()})
			return
		}
	}

	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		if payload == nil {
			payload = map[string]any{}
		}
		if _, ok := payload["id"]; !ok {
			payload["id"] = fmt.Sprintf("compat-%d", time.Now().UnixNano())
		}
		payload["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
		legacyStore.mu.Lock()
		legacyStore.loadLocked()
		items := legacyStore.data[key]
		items = append(items, payload)
		if len(items) > 2000 {
			items = items[len(items)-2000:]
		}
		legacyStore.data[key] = items
		if err := legacyStore.saveLocked(); err != nil {
			legacyStore.mu.Unlock()
			wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "FALLBACK_PERSIST_FAILED"}, "message": err.Error()})
			return
		}
		legacyStore.mu.Unlock()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": payload})
	case http.MethodDelete:
		legacyStore.mu.Lock()
		legacyStore.loadLocked()
		delete(legacyStore.data, key)
		if err := legacyStore.saveLocked(); err != nil {
			legacyStore.mu.Unlock()
			wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "FALLBACK_PERSIST_FAILED"}, "message": err.Error()})
			return
		}
		legacyStore.mu.Unlock()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"deleted": true}})
	default:
		legacyStore.mu.Lock()
		legacyStore.loadLocked()
		items := append([]map[string]any(nil), legacyStore.data[key]...)
		legacyStore.mu.Unlock()
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		if pageSize < 1 || pageSize > 200 {
			pageSize = 50
		}
		start := (page - 1) * pageSize
		if start > len(items) {
			start = len(items)
		}
		end := start + pageSize
		if end > len(items) {
			end = len(items)
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{
			"items": items[start:end], "total": len(items), "page": page, "pageSize": pageSize,
		}})
	}
}
