// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

// registerLegacyAlertRoutes 注册 alert 领域尚未迁移的兼容路由。
func registerLegacyAlertRoutes(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v2/alert/clams/list", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/alert/disks/list", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/alert/config/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/alert/config/info", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/alert/config/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/alert/config/test", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/alert/config/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/alert/cronjob/list", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/alert/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/alert/logs/clean", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/alert/logs/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/alert/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/alert/status", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/alert/update", fallbackRouteHandler)
}
