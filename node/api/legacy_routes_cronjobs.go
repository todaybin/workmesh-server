// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

// registerLegacyCronjobsRoutes 注册 cronjobs 领域尚未迁移的兼容路由。
func registerLegacyCronjobsRoutes(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v2/cronjobs/script/options", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/cronjobs/export", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/cronjobs/group/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/cronjobs/import", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/cronjobs/load/info", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/cronjobs/next", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/cronjobs/records/clean", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/cronjobs/records/log", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/cronjobs/search/records", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/cronjobs/status", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/cronjobs/stop", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/cronjobs/update", fallbackRouteHandler)
}
