// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"net/http"
	"os"
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
	client gateway.ProtocolClient
}

// Start 启动节点自动注册和周期心跳；未配置云端客户端时不创建后台任务。
func (s *GatewayStateStore) Start(ctx context.Context, capabilities []string) {
	if s.client == nil {
		return
	}
	go func() {
		register := func() {
			if err := s.loginIfConfigured(ctx); err != nil {
				s.mu.Lock()
				s.status.Registration = gateway.RegistrationPending
				s.status.Connected = false
				s.status.Reason = err.Error()
				s.mu.Unlock()
				return
			}
			s.mu.RLock()
			request := gateway.RegisterRequest{NodeID: s.status.NodeID, Role: s.status.Role, ProtocolVersion: "v1", Capabilities: capabilities}
			s.mu.RUnlock()
			auth, err := s.client.Register(ctx, request)
			s.mu.Lock()
			defer s.mu.Unlock()
			if err != nil {
				s.status.Registration = gateway.RegistrationPending
				s.status.Connected = false
				s.status.Reason = err.Error()
				return
			}
			s.status.Registration = gateway.RegistrationRegistered
			s.status.Connected = true
			s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
			s.status.Reason = ""
			s.auth = auth
		}
		register()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.mu.RLock()
				registration := gateway.Registration{NodeID: s.status.NodeID, BindingID: s.auth.BindingID, Registered: s.status.Registration == gateway.RegistrationRegistered}
				s.mu.RUnlock()
				if err := s.client.Heartbeat(ctx, registration); err != nil {
					s.mu.Lock()
					s.status.Connected = false
					s.status.Reason = err.Error()
					s.mu.Unlock()
				} else {
					s.mu.Lock()
					s.status.Connected = true
					s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
					s.mu.Unlock()
				}
			}
		}
	}()
}

func (s *GatewayStateStore) loginIfConfigured(ctx context.Context) error {
	username := os.Getenv("WORKMESH_GATEWAY_USERNAME")
	password := os.Getenv("WORKMESH_GATEWAY_PASSWORD")
	if username == "" && password == "" {
		return nil
	}
	if username == "" || password == "" {
		return errors.New("Gateway 登录凭据配置不完整")
	}
	_, err := s.client.Login(ctx, gateway.LoginRequest{Username: username, Password: password})
	return err
}

// RegisterGatewayRoutes 注册前端使用的 Gateway 状态、注册、心跳和授权接口。
func RegisterGatewayRoutes(mux *http.ServeMux, nodeID, role string) *GatewayStateStore {
	store := &GatewayStateStore{status: gateway.Status{Registration: gateway.RegistrationUnregistered, NodeID: nodeID, GatewayID: os.Getenv("WORKMESH_GATEWAY_ID"), Role: role}}
	// 配置 Gateway 地址后启用真实云端协议；未配置时保留离线开发模式。
	if baseURL := os.Getenv("WORKMESH_GATEWAY_URL"); baseURL != "" {
		store.client = gateway.NewHTTPClient(baseURL, os.Getenv("WORKMESH_GATEWAY_ID"), os.Getenv("WORKMESH_GATEWAY_SECRET"))
	}
	mux.HandleFunc("GET /api/v2/gateway/status", store.statusHandler)
	mux.HandleFunc("GET /api/v2/workmesh/gateway/status", store.statusHandler)
	mux.HandleFunc("POST /api/v2/gateway/register", store.registerHandler)
	mux.HandleFunc("POST /api/v2/workmesh/gateway/register", store.registerHandler)
	mux.HandleFunc("POST /api/v2/workmesh/gateway/login", store.loginHandler)
	mux.HandleFunc("POST /api/v2/gateway/heartbeat", store.heartbeatHandler)
	mux.HandleFunc("POST /api/v2/gateway/authorization/refresh", store.refreshHandler)
	mux.HandleFunc("POST /api/v2/gateway/unbind", store.unbindHandler)
	mux.HandleFunc("POST /api/v2/workmesh/gateway/unbind", store.unbindHandler)
	return store
}

func (s *GatewayStateStore) loginHandler(w http.ResponseWriter, r *http.Request) {
	var request gateway.LoginRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if s.client == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("Gateway 未配置"))
		return
	}
	auth, err := s.client.Login(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	s.mu.Lock()
	s.status.Registration = gateway.RegistrationRegistered
	s.status.Connected = true
	s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
	s.status.Reason = ""
	s.auth = auth
	s.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": auth})
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
	if s.client != nil {
		if err := s.loginIfConfigured(r.Context()); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		auth, err := s.client.Register(r.Context(), request)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		s.mu.Lock()
		s.status.Registration = gateway.RegistrationRegistered
		s.status.NodeID = request.NodeID
		s.status.Role = request.Role
		s.status.Connected = true
		s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
		s.status.Reason = ""
		s.auth = auth
		s.mu.Unlock()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": auth})
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

func (s *GatewayStateStore) heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	if s.client != nil {
		s.mu.RLock()
		registration := gateway.Registration{NodeID: s.status.NodeID, BindingID: s.auth.BindingID, Registered: s.status.Registration == gateway.RegistrationRegistered}
		s.mu.RUnlock()
		if err := s.client.Heartbeat(r.Context(), registration); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	s.mu.Lock()
	s.status.Connected = true
	s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
	s.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
}

func (s *GatewayStateStore) refreshHandler(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	auth := s.auth
	registered := s.status.Registration == gateway.RegistrationRegistered
	s.mu.RUnlock()
	if !registered {
		writeError(w, http.StatusUnauthorized, errGatewayUnregistered)
		return
	}
	if s.client != nil {
		refreshed, err := s.client.Refresh(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		s.mu.Lock()
		s.auth = refreshed
		auth = refreshed
		s.mu.Unlock()
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": auth})
}

func (s *GatewayStateStore) unbindHandler(w http.ResponseWriter, r *http.Request) {
	if s.client != nil {
		if err := s.client.Revoke(r.Context()); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	s.mu.Lock()
	s.status.Registration = gateway.RegistrationUnregistered
	s.status.Connected = false
	s.status.Reason = "用户解除绑定"
	s.auth = gateway.Authorization{}
	s.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
}
