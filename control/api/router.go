// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// RequestAuthorizer 校验控制面写请求是否具有有效的本机会话或 API 凭据。
type RequestAuthorizer func(*http.Request) bool

// Register 在统一 Engine 上注册控制面骨架接口；完整 Core 路由按清单逐步迁移。
func Register(mux *http.ServeMux, nodeID, role string, authorizers ...RequestAuthorizer) *GatewayStateStore {
	mux.HandleFunc("/api/v2/core/health", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]string{"status": "ok"}})
	})
	var authorize RequestAuthorizer
	if len(authorizers) > 0 {
		authorize = authorizers[0]
	}
	store := RegisterGatewayRoutes(mux, nodeID, role, authorize)
	manager := NewRoleManager(nodeID, role)
	RegisterRoleRoutesWithManager(mux, manager, authorize)
	RegisterLinkRoutes(mux, nodeID, role, manager)
	return store
}
