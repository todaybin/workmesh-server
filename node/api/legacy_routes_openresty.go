// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

// registerLegacyOpenrestyRoutes 注册 openresty 领域尚未迁移的兼容路由。
func registerLegacyOpenrestyRoutes(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v2/openresty/https", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/openresty/modules", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/openresty/status", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/openresty/build", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/openresty/file", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/openresty/https", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/openresty/modules/update", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/openresty/scope", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/openresty/update", fallbackRouteHandler)
}
