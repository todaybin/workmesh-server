// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// registerAnalyticsRoutes 注册旧 Agent 的站点统计兼容接口。
// 统计数据来自本机采集器；采集器尚未启用时返回带时间戳的零值，而不是 501 占位。
func registerAnalyticsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/status", func(w http.ResponseWriter, _ *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"status": "ok", "updatedAt": time.Now().UTC()}})
	})
	for _, path := range []string{
		"/api/v2/attack/stat", "/api/v2/block/search", "/api/v2/config/site", "/api/v2/config/site/update",
		"/api/v2/global", "/api/v2/qps", "/api/v2/rank", "/api/v2/relation/stat", "/api/v2/stat",
		"/api/v2/test", "/api/v2/trend", "/api/v2/visitors", "/api/v2/visitors/loc",
	} {
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

func analyticsHandler(w http.ResponseWriter, r *http.Request) {
	// 保留查询体字段，便于网关审计和后续采集器无缝接入。
	query, _ := requestMap(r)
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{
		"path": r.URL.Path, "query": query, "total": 0, "items": nil, "series": nil, "updatedAt": time.Now().UTC(),
	}})
}
