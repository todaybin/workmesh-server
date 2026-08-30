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
	RegisterRuntimeToolboxRoutes(mux)
	RegisterAppRoutes(mux)
	registerAnalyticsRoutes(mux)
}
