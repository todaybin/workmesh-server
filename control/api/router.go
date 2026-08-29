// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
)

// Register 在统一 Engine 上注册控制面骨架接口；完整 Core 路由按清单逐步迁移。
func Register(mux *http.ServeMux, nodeID, role string) {
	mux.HandleFunc("/api/v2/core/health", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]string{"status": "ok"}})
	})
	RegisterGatewayRoutes(mux, nodeID, role)
	RegisterRoleRoutes(mux, nodeID, role)
}
