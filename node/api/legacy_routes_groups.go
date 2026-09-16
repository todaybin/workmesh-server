// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

// registerLegacyGroupsRoutes 注册 groups 领域尚未迁移的兼容路由。
func registerLegacyGroupsRoutes(mux routeRegistrar) {
	mux.HandleFunc("POST /api/v2/groups/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/groups/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/groups/update", fallbackRouteHandler)
}
