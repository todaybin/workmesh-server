// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

// registerLegacyBackupsRoutes 注册 backups 领域尚未迁移的兼容路由。
func registerLegacyBackupsRoutes(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v2/backups/check/:name", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/backups/local", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/backups/options", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/backup", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/buckets", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/conn/check", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/record/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/record/description/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/record/download", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/record/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/record/search/bycronjob", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/record/size", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/recover", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/recover/byupload", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/refresh/token", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/search/files", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/backups/upload", fallbackRouteHandler)
}
