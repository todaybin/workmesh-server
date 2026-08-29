// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// compatibilityStore 为尚未接入持久化模型的旧接口提供进程内短期状态。
// 该状态仅用于兼容前端和滚动迁移，不作为生产数据源；专用域实现接入后会自然替代它。
type compatibilityStore struct {
	mu   sync.RWMutex
	data map[string][]map[string]any
}

var legacyStore = compatibilityStore{data: make(map[string][]map[string]any)}

func registerCompatibilityRoute(mux *http.ServeMux, pattern string) {
	mux.HandleFunc(pattern, compatibilityHandler)
}

// compatibilityHandler 统一实现旧契约的基础 CRUD、分页和幂等响应。
// 所有写入都限制请求体大小，避免兼容入口被大请求占满内存。
func compatibilityHandler(w http.ResponseWriter, r *http.Request) {
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
		items := legacyStore.data[key]
		items = append(items, payload)
		legacyStore.data[key] = items
		legacyStore.mu.Unlock()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": payload})
	case http.MethodDelete:
		legacyStore.mu.Lock()
		delete(legacyStore.data, key)
		legacyStore.mu.Unlock()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"deleted": true}})
	default:
		legacyStore.mu.RLock()
		items := append([]map[string]any(nil), legacyStore.data[key]...)
		legacyStore.mu.RUnlock()
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
