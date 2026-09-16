// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

// registerLegacySettingsRoutes 注册 settings 领域尚未迁移的兼容路由。
func registerLegacySettingsRoutes(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v2/settings/basedir", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/settings/search/available", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/settings/snapshot/load", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/settings/ssh/conn", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/settings/website/dir", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/description/save", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/file-history/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/file-history/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/files/ai/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/files/ai/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/snapshot", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/snapshot/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/snapshot/description/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/snapshot/import", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/snapshot/recover", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/snapshot/recreate", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/snapshot/rollback", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/snapshot/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/ssh", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/ssh/check", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/ssh/check/info", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/ssh/default", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/terminal/ai/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/terminal/ai/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/settings/update", fallbackRouteHandler)
}
