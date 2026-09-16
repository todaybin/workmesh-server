// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

// registerLegacyAppsRoutes 注册 apps 领域尚未迁移的兼容路由。
func registerLegacyAppsRoutes(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v2/apps/:key", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/apps/checkupdate", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/apps/detail/:appId/:version/:type", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/apps/detail/node/:appKey/:version", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/apps/details/:id", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/apps/icon/:key", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/apps/ignored/detail", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/apps/installed/delete/check/:appInstallId", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/apps/installed/info/:appInstallId", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/apps/installed/list", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/apps/installed/params/:appInstallId", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/apps/services/:key", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/apps/tags", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/ignored/cancel", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/install", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/installed/check", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/installed/conf", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/installed/config/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/installed/conninfo", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/installed/ignore", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/installed/loadport", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/installed/op", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/installed/params/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/installed/port/change", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/installed/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/installed/sort/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/installed/sync", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/installed/update/versions", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/sync/local", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/apps/sync/remote", fallbackRouteHandler)
}
