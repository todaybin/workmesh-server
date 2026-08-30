// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// registerAnalyticsRoutes 注册 Agent 站点监控统计兼容接口。
func registerAnalyticsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/status", analyticsHandler)
	for _, path := range []string{"/api/v2/attack/stat", "/api/v2/block/search", "/api/v2/config/site", "/api/v2/config/site/update", "/api/v2/global", "/api/v2/qps", "/api/v2/rank", "/api/v2/relation/stat", "/api/v2/stat", "/api/v2/test", "/api/v2/trend", "/api/v2/visitors", "/api/v2/visitors/loc"} {
		mux.HandleFunc("POST "+path, analyticsHandler)
	}
}

func isAnalyticsRoute(pattern string) bool {
	for _, path := range []string{"/api/v2/status", "/api/v2/attack/stat", "/api/v2/block/search", "/api/v2/config/site", "/api/v2/config/site/update", "/api/v2/global", "/api/v2/qps", "/api/v2/rank", "/api/v2/relation/stat", "/api/v2/stat", "/api/v2/test", "/api/v2/trend", "/api/v2/visitors", "/api/v2/visitors/loc"} {
		if pattern == "GET "+path || pattern == "POST "+path {
			return true
		}
	}
	return false
}

// analyticsHandler 返回旧 DTO 兼容结构，并持久化全局及站点监控配置。
func analyticsHandler(w http.ResponseWriter, r *http.Request) {
	query, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	store := getDomainStore()
	path := r.URL.Path
	if path == "/api/v2/config/site/update" || path == "/api/v2/global" {
		store.mu.Lock()
		if store.state.Settings == nil {
			store.state.Settings = map[string]any{}
		}
		key := "monitorGlobal"
		if path == "/api/v2/config/site/update" {
			key = "monitorSite:" + strconv.FormatUint(uint64(analyticsWebsiteID(query)), 10)
		}
		store.state.Settings[key] = query
		err = store.saveLocked()
		store.mu.Unlock()
		if err != nil {
			domainError(w, http.StatusInternalServerError, "STATE_SAVE", err.Error())
			return
		}
		successAnalytics(w, query)
		return
	}
	if path == "/api/v2/config/site" {
		key := "monitorSite:" + strconv.FormatUint(uint64(analyticsWebsiteID(query)), 10)
		store.mu.RLock()
		value := store.state.Settings[key]
		store.mu.RUnlock()
		if value == nil {
			value = analyticsDefaultConfig(analyticsWebsiteID(query))
		}
		successAnalytics(w, value)
		return
	}
	if path == "/api/v2/status" {
		store.mu.RLock()
		value := store.state.Settings["monitorGlobal"]
		store.mu.RUnlock()
		enabled := true
		if config, ok := value.(map[string]any); ok {
			if raw, exists := config["enabled"].(bool); exists {
				enabled = raw
			}
		}
		successAnalytics(w, map[string]any{"enabled": enabled})
		return
	}
	successAnalytics(w, analyticsData(path, query))
}

func successAnalytics(w http.ResponseWriter, data any) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}

func analyticsWebsiteID(query map[string]any) uint {
	for _, key := range []string{"websiteID", "websiteId", "id"} {
		switch value := query[key].(type) {
		case float64:
			if value > 0 {
				return uint(value)
			}
		case string:
			id, _ := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
			if id > 0 {
				return uint(id)
			}
		}
	}
	return 0
}

func analyticsDefaultConfig(websiteID uint) map[string]any {
	return map[string]any{"websiteID": websiteID, "enabled": true, "storeDays": 30, "storeSize": int64(1073741824), "excludeStatus": "", "excludeExt": "", "excludeURI": "", "excludeIP": "", "excludeUA": "", "cdnType": "", "realIPHeader": ""}
}

func analyticsData(path string, query map[string]any) any {
	now := time.Now().UTC()
	switch path {
	case "/api/v2/qps":
		return map[string]any{"flow": int64(0), "qps": int64(0), "updatedAt": now}
	case "/api/v2/rank", "/api/v2/visitors", "/api/v2/visitors/loc":
		return []map[string]any{}
	case "/api/v2/attack/stat", "/api/v2/block/search", "/api/v2/relation/stat":
		return map[string]any{"total": 0, "items": []map[string]any{}, "updatedAt": now}
	case "/api/v2/test":
		return map[string]any{"passed": true, "query": query, "updatedAt": now}
	default:
		return []map[string]any{{"day": now.Format("2006-01-02"), "pv": int64(0), "uv": int64(0), "ip": int64(0), "flow": int64(0), "spider": int64(0), "req": int64(0), "count4xx": int64(0), "count5xx": int64(0)}}
	}
}
