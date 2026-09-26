// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import "net/http"

// registerLegacyDatabasesRoutes 注册 databases 领域尚未迁移的兼容路由。
func registerLegacyDatabasesRoutes(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v2/databases/db/:name", func(w http.ResponseWriter, r *http.Request) {
		databaseRoute(w, r)
	})
	mux.HandleFunc("GET /api/v2/databases/db/item/:type", func(w http.ResponseWriter, r *http.Request) {
		databaseRoute(w, r)
	})
	mux.HandleFunc("GET /api/v2/databases/db/list/:type", func(w http.ResponseWriter, r *http.Request) {
		databaseRoute(w, r)
	})
	// Redis CLI check 已由 registerDatabaseRoutes 注册真实 handler。
	mux.HandleFunc("POST /api/v2/databases/change/password", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/common/info", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/common/load/file", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/common/update/conf", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/db", func(w http.ResponseWriter, r *http.Request) {
		handleDatabaseCreate(w, r)
	})
	mux.HandleFunc("POST /api/v2/databases/db/check", func(w http.ResponseWriter, r *http.Request) {
		handleDatabaseCheck(w, r)
	})
	mux.HandleFunc("POST /api/v2/databases/db/del", func(w http.ResponseWriter, r *http.Request) {
		handleDatabaseDelete(w, r)
	})
	mux.HandleFunc("POST /api/v2/databases/db/del/check", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/db/search", func(w http.ResponseWriter, r *http.Request) {
		handleDatabaseSearch(w, r)
	})
	mux.HandleFunc("POST /api/v2/databases/db/update", func(w http.ResponseWriter, r *http.Request) {
		handleDatabaseUpdate(w, r)
	})
	mux.HandleFunc("POST /api/v2/databases/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/del/check", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/description/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/format/options", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/grants", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/grants/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/grants/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/grants/summary", fallbackRouteHandler)
	// MySQL/MariaDB 远程同步已由 registerDatabaseRoutes 注册真实 handler。
	// MongoDB 远程同步已由 registerDatabaseRoutes 注册真实 handler。
	// PostgreSQL 生命周期与权限接口已由真实执行器承接。
	// PostgreSQL description/password 已由真实 handler 注册。
	// Redis 状态、配置、持久化和密码接口已由真实执行器承接。
	// redis-cli 安装接口明确返回 503，不执行系统包安装。
	mux.HandleFunc("POST /api/v2/databases/remote", fallbackRouteHandler)
	// MySQL 库内列表由 databaseRoute 分派到 handleMySQLDatabaseSearch。
	mux.HandleFunc("POST /api/v2/databases/search", handleMySQLDatabaseSearch)
	mux.HandleFunc("POST /api/v2/databases/status", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/users", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/users/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/users/password", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/users/password/save", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/users/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/databases/users/update", fallbackRouteHandler)
	// MySQL 变量查询/更新和 root 访问已由 RegisterDatabaseAdminRoutes 注册真实 handler。
}
