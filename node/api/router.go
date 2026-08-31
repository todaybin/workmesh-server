// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// Register 在统一 Engine 上注册节点执行面骨架接口；完整 Agent 路由按清单逐步迁移。
func Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/health", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]string{"status": "ok"}})
	})
	RegisterHostContainerCronRoutes(mux)
	RegisterSSLRoutes(mux)
	registerCoreAuthExtras(mux)
	// 登录、会话和仪表盘使用专用处理器，不能依赖兼容层的动态注册。
	mux.HandleFunc("POST /api/v2/core/auth/login", handleCoreLogin)
	mux.HandleFunc("POST /api/v2/core/auth/logout", handleCoreLogout)
	mux.HandleFunc("GET /api/v2/core/auth/current", handleCoreCurrent)
	mux.HandleFunc("GET /api/v2/dashboard/app/launcher", handleDashboardLauncher)
	mux.HandleFunc("GET /api/v2/dashboard/current/node", handleDashboardNode)
	mux.HandleFunc("GET /api/v2/dashboard/current/top/cpu", handleDashboardTopCPU)
	mux.HandleFunc("GET /api/v2/dashboard/current/top/mem", handleDashboardTopMem)
	mux.HandleFunc("GET /api/v2/dashboard/quick/option", handleDashboardQuickOption)
	mux.HandleFunc("POST /api/v2/dashboard/app/launcher/option", handleDashboardLauncherOption)
	mux.HandleFunc("POST /api/v2/dashboard/app/launcher/show", handleDashboardMutation)
	mux.HandleFunc("POST /api/v2/dashboard/quick/change", handleDashboardMutation)
	RegisterRuntimeToolboxRoutes(mux)
	RegisterAppRoutes(mux)
	registerAnalyticsRoutes(mux)
}
