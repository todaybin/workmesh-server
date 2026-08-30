// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"io"
	"net/http"
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
	for _, pattern := range []string{
		"GET /api/v2/core/script/run",
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
