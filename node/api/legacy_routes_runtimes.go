// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

// registerLegacyRuntimesRoutes 注册 runtimes 领域尚未迁移的兼容路由。
func registerLegacyRuntimesRoutes(mux routeRegistrar) {
	// 兼容契约路径：PHP 扩展与配置动态段由下方统一子路径处理器承接。
	// HandleFunc("GET /api/v2/runtimes/php/:id/extensions", ...)
	// HandleFunc("GET /api/v2/runtimes/php/config/:id", ...)
	mux.HandleFunc("GET /api/v2/runtimes/:id", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/runtimes/installed/delete/check/:id", fallbackRouteHandler)
	// PHP 子路径统一进入兼容处理器，避免 ServeMux 动态段与静态段产生歧义。
	mux.HandleFunc("GET /api/v2/runtimes/php/{path...}", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/runtimes/php/container/:id", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/runtimes/php/fpm/config/:id", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/runtimes/php/fpm/status/:id", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/runtimes/supervisor/process/:id", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/node/modules", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/node/modules/operate", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/node/package", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/operate", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/php/config", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/php/container/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/install", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/uninstall", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/php/file", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/php/fpm/config", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/php/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/remark", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/supervisor/process", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/supervisor/process/file", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/sync", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/runtimes/update", fallbackRouteHandler)
}
