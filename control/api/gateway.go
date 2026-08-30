// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	// statePath 位于数据目录内，仅保存本机绑定快照和受限访问令牌。
	statePath string
}

// persistedGatewayState 是磁盘上的绑定快照。AccessToken 不会通过 HTTP 响应序列化。
type persistedGatewayState struct {
	Status      gateway.Status `json:"status"`
	BindingID   string         `json:"bindingId,omitempty"`
	Scopes      []string       `json:"scopes,omitempty"`
	ExpiresAt   string         `json:"expiresAt,omitempty"`
	Refreshable bool           `json:"refreshable"`
	AccessToken string         `json:"accessToken,omitempty"`
	GatewayURL  string         `json:"gatewayUrl,omitempty"`
	GatewayID   string         `json:"gatewayId,omitempty"`
	UpdatedAt   string         `json:"updatedAt"`
}

// Start 启动节点自动注册和周期心跳；未配置云端客户端时不创建后台任务。
func (s *GatewayStateStore) Start(ctx context.Context, capabilities []string) {
	if s.client == nil {
		return
	}
	go func() {
		connect := func() {
			s.mu.RLock()
			bound := s.status.Registration == gateway.RegistrationRegistered && s.auth.BindingID != ""
			registration := gateway.Registration{NodeID: s.status.NodeID, BindingID: s.auth.BindingID, Registered: bound}
			s.mu.RUnlock()
			// 已绑定节点优先恢复心跳，不再次要求账号登录或创建新绑定。
			if bound {
				if err := s.client.Heartbeat(ctx, registration); err == nil {
					s.mu.Lock()
					s.status.Connected = true
					s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
					s.status.Reason = ""
					s.mu.Unlock()
					_ = s.persist()
					return
				}
				s.mu.Lock()
				s.status.Connected = false
				s.status.Registration = gateway.RegistrationPending
				s.status.Reason = "Gateway 绑定已保存，但心跳恢复失败"
				s.mu.Unlock()
				_ = s.persist()
				return
			}
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
			_ = s.persistLocked()
		}
		connect()
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
					_ = s.persist()
				} else {
					s.mu.Lock()
					s.status.Connected = true
					s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
					s.mu.Unlock()
					_ = s.persist()
				}
			}
		}
	}()
}

func (s *GatewayStateStore) loginIfConfigured(ctx context.Context) error {
	s.mu.RLock()
	hasToken := s.auth.AccessToken != ""
	s.mu.RUnlock()
	if hasToken {
		return nil
	}
	username := os.Getenv("WORKMESH_GATEWAY_USERNAME")
	password := os.Getenv("WORKMESH_GATEWAY_PASSWORD")
	if username == "" && password == "" {
		return nil
	}
	if username == "" || password == "" {
		return errors.New("Gateway 登录凭据配置不完整")
	}
	auth, err := s.client.Login(ctx, gateway.LoginRequest{Username: username, Password: password})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.auth = auth
	err = s.persistLocked()
	s.mu.Unlock()
	return err
}

// RegisterGatewayRoutes 注册前端使用的 Gateway 状态、注册、心跳和授权接口。
func RegisterGatewayRoutes(mux *http.ServeMux, nodeID, role string) *GatewayStateStore {
	dataDir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dataDir == "" {
		dataDir = "./data"
	}
	store := &GatewayStateStore{status: gateway.Status{Registration: gateway.RegistrationUnregistered, NodeID: nodeID, GatewayID: os.Getenv("WORKMESH_GATEWAY_ID"), Role: role}, statePath: filepath.Join(dataDir, "gateway-binding.json")}
	store.load()
	// 配置 Gateway 地址后启用真实云端协议；未配置时保留离线开发模式。
	if baseURL := os.Getenv("WORKMESH_GATEWAY_URL"); baseURL != "" {
		store.client = gateway.NewHTTPClient(baseURL, os.Getenv("WORKMESH_GATEWAY_ID"), os.Getenv("WORKMESH_GATEWAY_SECRET"))
		if store.auth.AccessToken != "" {
			store.client.(*gateway.HTTPClient).AccessToken = store.auth.AccessToken
		}
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
	s.mu.RLock()
	// 仅在绑定且已有访问令牌时复用快照；部分 Gateway 注册响应只返回 bindingId，
	// 此时显式登录仍需向云端补齐令牌，避免后续心跳因缺少授权而失败。
	if s.status.Registration == gateway.RegistrationRegistered && s.auth.BindingID != "" && s.auth.AccessToken != "" {
		auth := s.auth
		s.mu.RUnlock()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": auth})
		return
	}
	s.mu.RUnlock()
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
	if err := s.persistLocked(); err != nil {
		s.mu.Unlock()
		writeError(w, http.StatusInternalServerError, fmt.Errorf("保存 Gateway 授权失败: %w", err))
		return
	}
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
	s.mu.RLock()
	if s.status.Registration == gateway.RegistrationRegistered && s.auth.BindingID != "" {
		boundNode := s.status.NodeID
		auth := s.auth
		s.mu.RUnlock()
		if request.NodeID == boundNode {
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": auth})
		} else {
			writeError(w, http.StatusConflict, errors.New("Gateway 已绑定其他节点，如需切换请先解绑"))
		}
		return
	}
	s.mu.RUnlock()
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
		if err := s.persistLocked(); err != nil {
			s.mu.Unlock()
			writeError(w, http.StatusInternalServerError, fmt.Errorf("保存 Gateway 绑定失败: %w", err))
			return
		}
		s.mu.Unlock()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": auth})
		return
	}
	// 未配置真实 Gateway 客户端时禁止伪造注册成功，避免节点绕过云端授权。
	writeError(w, http.StatusServiceUnavailable, errors.New("Gateway 未配置有效凭据，无法完成节点注册"))
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
		if httpClient, ok := s.client.(*gateway.HTTPClient); ok {
			httpClient.AccessToken = refreshed.AccessToken
		}
		if err := s.persistLocked(); err != nil {
			s.mu.Unlock()
			writeError(w, http.StatusInternalServerError, fmt.Errorf("保存 Gateway 刷新授权失败: %w", err))
			return
		}
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
	if err := s.removePersisted(); err != nil {
		s.mu.Unlock()
		writeError(w, http.StatusInternalServerError, fmt.Errorf("清理 Gateway 绑定失败: %w", err))
		return
	}
	s.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
}

// load 从数据目录恢复绑定。损坏快照只会进入 pending，不会伪造已注册状态。
func (s *GatewayStateStore) load() {
	raw, err := os.ReadFile(s.statePath)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		s.status.Registration = gateway.RegistrationPending
		s.status.Reason = fmt.Sprintf("读取 Gateway 绑定状态失败: %v", err)
		return
	}
	var saved persistedGatewayState
	if err := json.Unmarshal(raw, &saved); err != nil || saved.Status.NodeID == "" {
		s.status.Registration = gateway.RegistrationPending
		s.status.Reason = "Gateway 绑定状态文件损坏"
		return
	}
	s.status = saved.Status
	s.auth = gateway.Authorization{BindingID: saved.BindingID, Scopes: saved.Scopes, ExpiresAt: saved.ExpiresAt, Refreshable: saved.Refreshable, AccessToken: saved.AccessToken}
	if s.status.Registration == gateway.RegistrationRegistered && s.auth.BindingID == "" {
		s.status.Registration = gateway.RegistrationPending
		s.status.Connected = false
	}
}

// persist 将当前绑定原子写入数据目录，文件权限限制为仅所有者可读写。
func (s *GatewayStateStore) persist() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.persistSnapshot(s.status, s.auth)
}

func (s *GatewayStateStore) persistLocked() error {
	return s.persistSnapshot(s.status, s.auth)
}

func (s *GatewayStateStore) persistSnapshot(status gateway.Status, auth gateway.Authorization) error {
	if err := os.MkdirAll(filepath.Dir(s.statePath), 0o700); err != nil {
		return err
	}
	saved := persistedGatewayState{Status: status, BindingID: auth.BindingID, Scopes: auth.Scopes, ExpiresAt: auth.ExpiresAt, Refreshable: auth.Refreshable, AccessToken: auth.AccessToken, GatewayURL: os.Getenv("WORKMESH_GATEWAY_URL"), GatewayID: status.GatewayID, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	if saved.ExpiresAt != "" {
		saved.Status.AuthorizationExpireAt = saved.ExpiresAt
	}
	raw, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.statePath), ".gateway-binding-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.statePath)
}

func (s *GatewayStateStore) removePersisted() error {
	if err := os.Remove(s.statePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
