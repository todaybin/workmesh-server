// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/runtime/gateway"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var (
	errNodeIDRequired      = errors.New("节点 ID 不能为空")
	errGatewayUnregistered = errors.New("节点尚未注册 Gateway")
)

var gatewayDB *sql.DB

// SetGatewayDatabase 注入共享 SQLite，Gateway 绑定状态不再依赖 JSON 文件。
func SetGatewayDatabase(db *sql.DB) { gatewayDB = db }

// GatewayStateStore 保存本机 Gateway 授权摘要；访问令牌不会序列化到响应。
type GatewayStateStore struct {
	mu      sync.RWMutex
	startMu sync.Mutex
	started bool
	status  gateway.Status
	auth    gateway.Authorization
	client  gateway.ProtocolClient
	account string
	// gatewayURL 缓存绑定时使用的地址；即使环境变量未注入，重启后也能恢复连接。
	gatewayURL string
	// statePath 位于数据目录内，仅保存本机绑定快照和受限访问令牌。
	statePath    string
	identityPath string
	db           *sql.DB
	repository   storage.Transactional
}

var gatewayCapabilities = []string{"system", "containers", "files", "databases", "websites", "tasks"}

// gatewayRegisterRequest 同时兼容控制面注册契约和前端绑定表单字段。
// RegistrationToken 仅用于本次请求的 Bearer 认证，不写入响应或持久化快照。
type gatewayRegisterRequest struct {
	NodeID            string            `json:"nodeId"`
	PublicKey         string            `json:"publicKey,omitempty"`
	DisplayName       string            `json:"displayName,omitempty"`
	Role              string            `json:"role,omitempty"`
	ProtocolVersion   string            `json:"protocolVersion,omitempty"`
	Capabilities      []string          `json:"capabilities,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	GatewayURL        string            `json:"gatewayUrl,omitempty"`
	RegistrationToken string            `json:"registrationToken,omitempty"`
	BootstrapToken    string            `json:"bootstrapToken,omitempty"`
	EndpointURL       string            `json:"endpointUrl,omitempty"`
}

// RegisterGatewayRoutes 注册前端使用的 Gateway 状态、注册、心跳和授权接口。
func RegisterGatewayRoutes(mux *http.ServeMux, nodeID, role string, authorizers ...RequestAuthorizer) *GatewayStateStore {
	dataDir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dataDir == "" {
		dataDir = "./data"
	}
	store := &GatewayStateStore{status: gateway.Status{Registration: gateway.RegistrationUnregistered, NodeID: nodeID, GatewayID: os.Getenv("WORKMESH_GATEWAY_ID"), Role: role}, statePath: filepath.Join(dataDir, "gateway-binding.json"), identityPath: filepath.Join(dataDir, "gateway-identity.ed25519"), db: gatewayDB}
	if store.db != nil {
		if _, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS gateway_binding (id INTEGER PRIMARY KEY CHECK(id=1), status BLOB NOT NULL, auth BLOB NOT NULL, gateway_url TEXT NOT NULL DEFAULT '', account TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL)`); err != nil {
			store.status.Registration = gateway.RegistrationPending
			store.status.Reason = fmt.Sprintf("初始化 Gateway 持久化失败: %v", err)
		}
		if repository, err := storage.NewSQLiteRepository(store.db); err != nil {
			store.status.Registration = gateway.RegistrationPending
			store.status.Reason = fmt.Sprintf("初始化 Gateway repository 失败: %v", err)
		} else {
			store.repository = repository
		}
	}
	store.load()
	// 配置 Gateway 地址后启用真实云端协议；未配置时保留离线开发模式。
	baseURL := strings.TrimSpace(os.Getenv("WORKMESH_GATEWAY_URL"))
	if baseURL == "" {
		store.mu.RLock()
		baseURL = store.gatewayURL
		store.mu.RUnlock()
	}
	if baseURL != "" {
		store.gatewayURL = strings.TrimRight(baseURL, "/")
		identity, identityErr := gateway.LoadOrCreateIdentity(store.identityPath)
		if identityErr != nil {
			store.status.Registration = gateway.RegistrationPending
			store.status.Reason = fmt.Sprintf("加载 Gateway 节点身份失败: %v", identityErr)
		} else {
			store.client = gateway.NewHTTPClientWithIdentity(baseURL, os.Getenv("WORKMESH_GATEWAY_ID"), os.Getenv("WORKMESH_GATEWAY_SECRET"), identity)
		}
		if store.auth.AccessToken != "" {
			if httpClient, ok := store.client.(*gateway.HTTPClient); ok {
				httpClient.AccessToken = store.auth.AccessToken
			}
		}
	}
	var authorize RequestAuthorizer
	if len(authorizers) > 0 {
		authorize = authorizers[0]
	}
	write := func(handler http.HandlerFunc) http.HandlerFunc {
		if authorize == nil {
			return handler
		}
		return func(w http.ResponseWriter, r *http.Request) {
			if !authorize(r) {
				wmhttp.JSON(w, http.StatusUnauthorized, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "LOCAL_AUTH_REQUIRED"}, "message": "需要有效的本地登录会话"})
				return
			}
			handler(w, r)
		}
	}
	mux.HandleFunc("GET /api/v2/gateway/status", store.statusHandler)
	mux.HandleFunc("GET /api/v2/workmesh/gateway/status", store.statusHandler)
	mux.HandleFunc("POST /api/v2/gateway/register", write(store.registerHandler))
	mux.HandleFunc("POST /api/v2/workmesh/gateway/register", write(store.registerHandler))
	mux.HandleFunc("POST /api/v2/workmesh/gateway/login", write(store.loginHandler))
	mux.HandleFunc("POST /api/v2/gateway/heartbeat", write(store.heartbeatHandler))
	mux.HandleFunc("POST /api/v2/gateway/authorization/refresh", write(store.refreshHandler))
	mux.HandleFunc("POST /api/v2/gateway/unbind", write(store.unbindHandler))
	mux.HandleFunc("POST /api/v2/workmesh/gateway/unbind", write(store.unbindHandler))
	return store
}

// statusHandler 返回脱敏的 Gateway 绑定、连接和授权过期状态。
func (s *GatewayStateStore) statusHandler(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	status := s.status
	auth := s.auth
	account := s.account
	s.mu.RUnlock()
	// configured 表示本机已有持久化绑定；网络暂时断开只影响运行状态，不应要求用户重新输入账号密码。
	configured := auth.BindingID != ""
	runtimeStatus := "not_configured"
	if configured && status.Connected {
		runtimeStatus = "running"
	} else if configured || status.Registration == gateway.RegistrationPending {
		runtimeStatus = "error"
	}
	// 同时保留底层 registration/connected 字段，并提供现有前端使用的 configured/status 契约。
	data := map[string]any{
		"configured": configured, "bindingRequired": !configured, "gatewayUrl": s.gatewayURLValue(),
		"nodeId": status.NodeID, "gatewayId": status.GatewayID, "role": status.Role, "account": account,
		"status": runtimeStatus, "registration": status.Registration, "connected": status.Connected,
		"authorizationExpiresAt": status.AuthorizationExpireAt, "lastSeenAt": status.LastSeenAt, "lastError": status.Reason, "reason": status.Reason,
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}

// gatewayURLValue 返回当前配置的 Gateway 地址；环境变量优先于持久化快照。
func (s *GatewayStateStore) gatewayURLValue() string {
	if value := strings.TrimSpace(os.Getenv("WORKMESH_GATEWAY_URL")); value != "" {
		return strings.TrimRight(value, "/")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gatewayURL
}

// heartbeatHandler 向 Gateway 验证当前绑定并刷新本地连接时间戳。
func (s *GatewayStateStore) heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	if s.client != nil {
		s.mu.RLock()
		registration := gateway.Registration{NodeID: s.status.NodeID, BindingID: s.auth.BindingID, Role: s.status.Role, Registered: s.status.Registration == gateway.RegistrationRegistered}
		s.mu.RUnlock()
		if err := s.client.Heartbeat(r.Context(), registration); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	s.mu.Lock()
	previous := s.status
	s.status.Connected = true
	s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.persistLocked(); err != nil {
		s.status = previous
		s.mu.Unlock()
		writeError(w, http.StatusInternalServerError, fmt.Errorf("保存 Gateway 心跳状态失败: %w", err))
		return
	}
	s.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
}

// refreshHandler 刷新已注册节点的 Gateway 授权并原子保存新令牌。
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
		previousAuth, previousStatus := s.auth, s.status
		previousAccessToken := ""
		if httpClient, ok := s.client.(*gateway.HTTPClient); ok {
			previousAccessToken = httpClient.AccessToken
		}
		s.auth = refreshed
		auth = refreshed
		s.status.AuthorizationExpireAt = refreshed.ExpiresAt
		if httpClient, ok := s.client.(*gateway.HTTPClient); ok {
			httpClient.AccessToken = refreshed.AccessToken
		}
		if err := s.persistLocked(); err != nil {
			s.auth, s.status = previousAuth, previousStatus
			if httpClient, ok := s.client.(*gateway.HTTPClient); ok {
				httpClient.AccessToken = previousAccessToken
			}
			s.mu.Unlock()
			writeError(w, http.StatusInternalServerError, fmt.Errorf("保存 Gateway 刷新授权失败: %w", err))
			return
		}
		s.mu.Unlock()
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": auth})
}

// unbindHandler 撤销远端授权并清除本地绑定状态，保证解绑后不可继续透传。
func (s *GatewayStateStore) unbindHandler(w http.ResponseWriter, r *http.Request) {
	if s.client != nil {
		if err := s.client.Revoke(r.Context()); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	s.mu.Lock()
	previousStatus, previousAuth, previousAccount := s.status, s.auth, s.account
	s.status.Registration = gateway.RegistrationUnregistered
	s.status.Connected = false
	s.status.Reason = "用户解除绑定"
	s.auth = gateway.Authorization{}
	s.account = ""
	if err := s.removePersisted(); err != nil {
		s.status, s.auth, s.account = previousStatus, previousAuth, previousAccount
		s.mu.Unlock()
		writeError(w, http.StatusInternalServerError, fmt.Errorf("清理 Gateway 绑定失败: %w", err))
		return
	}
	s.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
}
