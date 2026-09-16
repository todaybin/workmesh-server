// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import "net/http"

// registerLegacyCoreRoutes 注册 core 领域尚未迁移的兼容路由。
func registerLegacyCoreRoutes(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v2/core/auth/captcha", func(w http.ResponseWriter, r *http.Request) {
		handleCoreCaptcha(w, r)
	})
	mux.HandleFunc("GET /api/v2/core/auth/current", func(w http.ResponseWriter, r *http.Request) {
		handleCoreCurrent(w, r)
	})
	mux.HandleFunc("GET /api/v2/core/auth/setting", func(w http.ResponseWriter, r *http.Request) {
		handleCoreAuthSetting(w, r)
	})
	mux.HandleFunc("GET /api/v2/core/auth/welcome", func(w http.ResponseWriter, r *http.Request) {
		handleCoreWelcome(w, r)
	})
	mux.HandleFunc("GET /api/v2/core/backups/client/:clientType", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/core/script/run", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/core/settings/apps/store/config", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/core/settings/interface", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/core/settings/memo", func(w http.ResponseWriter, r *http.Request) {
		handleCoreSettings(w, r)
	})
	mux.HandleFunc("GET /api/v2/core/settings/search/available", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/core/settings/ssl/info", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/core/settings/upgrade", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/core/settings/upgrade/releases", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/auth/login", func(w http.ResponseWriter, r *http.Request) {
		handleCoreLogin(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		handleCoreLogout(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/backups/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/backups/refresh/token", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/backups/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/commands/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/commands/export", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/commands/import", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/commands/list", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/commands/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/commands/tree", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/commands/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/commands/upload", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/groups/del", func(w http.ResponseWriter, r *http.Request) {
		handleCoreGroups(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/groups/search", func(w http.ResponseWriter, r *http.Request) {
		handleCoreGroups(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/groups/update", func(w http.ResponseWriter, r *http.Request) {
		handleCoreGroups(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/logs/clean", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/logs/login", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/logs/operation", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/script/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/script/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/script/sync", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/script/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/settings/apps/store/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/settings/bind/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/settings/memo", func(w http.ResponseWriter, r *http.Request) {
		handleCoreSettings(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/settings/menu/default", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/settings/menu/update", func(w http.ResponseWriter, r *http.Request) {
		handleCoreSettings(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/settings/port/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/settings/proxy/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/settings/search", func(w http.ResponseWriter, r *http.Request) {
		handleCoreSettings(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/settings/search/base", func(w http.ResponseWriter, r *http.Request) {
		handleCoreSettings(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/settings/ssl/download", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/settings/ssl/reload", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/settings/ssl/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/settings/terminal/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/settings/terminal/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/settings/update", func(w http.ResponseWriter, r *http.Request) {
		handleCoreSettings(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/settings/upgrade", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/core/settings/upgrade/notes", fallbackRouteHandler)
}
