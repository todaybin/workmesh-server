// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

// registerLegacyDeploymentRoutes 注册 deployment 领域尚未迁移的兼容路由。
func registerLegacyDeploymentRoutes(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v2/deployment/status", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/deployment/execute", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/deployment/rollback", fallbackRouteHandler)
}
