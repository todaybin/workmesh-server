// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/runtime/gateway"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var (
	errNodeIDRequired      = errors.New("节点 ID 不能为空")
	errGatewayUnregistered = errors.New("节点尚未注册 Gateway")
)

// GatewayStateStore 保存本机 Gateway 授权摘要；访问令牌不会序列化到响应。
type GatewayStateStore struct {
	mu     sync.RWMutex
	status gateway.Status
	auth   gateway.Authorization
}

// RegisterGatewayRoutes 注册前端使用的 Gateway 状态、注册、心跳和授权接口。
func RegisterGatewayRoutes(mux *http.ServeMux, nodeID, role string) {
	store := &GatewayStateStore{status: gateway.Status{Registration: gateway.RegistrationUnregistered, NodeID: nodeID, Role: role}}
	mux.HandleFunc("GET /api/v2/gateway/status", store.statusHandler)
	mux.HandleFunc("GET /api/v2/workmesh/gateway/status", store.statusHandler)
	mux.HandleFunc("POST /api/v2/gateway/register", store.registerHandler)
	mux.HandleFunc("POST /api/v2/workmesh/gateway/register", store.registerHandler)
	mux.HandleFunc("POST /api/v2/gateway/heartbeat", store.heartbeatHandler)
	mux.HandleFunc("POST /api/v2/gateway/authorization/refresh", store.refreshHandler)
	mux.HandleFunc("POST /api/v2/gateway/unbind", store.unbindHandler)
}

func (s *GatewayStateStore) statusHandler(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	status := s.status
	s.mu.RUnlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": status})
}

func (s *GatewayStateStore) registerHandler(w http.ResponseWriter, r *http.Request) {
	var request gateway.RegisterRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if request.NodeID == "" {
		writeError(w, http.StatusBadRequest, errNodeIDRequired)
		return
	}
	s.mu.Lock()
	s.status.Registration = gateway.RegistrationRegistered
	s.status.NodeID = request.NodeID
	s.status.Role = request.Role
	s.status.Connected = true
	s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
	s.status.Reason = ""
	s.auth = gateway.Authorization{BindingID: request.NodeID, Refreshable: true}
	s.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": s.auth})
}

func (s *GatewayStateStore) heartbeatHandler(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	s.status.Connected = true
	s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
	s.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
}

func (s *GatewayStateStore) refreshHandler(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	auth := s.auth
	registered := s.status.Registration == gateway.RegistrationRegistered
	s.mu.RUnlock()
	if !registered {
		writeError(w, http.StatusUnauthorized, errGatewayUnregistered)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": auth})
}

func (s *GatewayStateStore) unbindHandler(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	s.status.Registration = gateway.RegistrationUnregistered
	s.status.Connected = false
	s.status.Reason = "用户解除绑定"
	s.auth = gateway.Authorization{}
	s.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
}
