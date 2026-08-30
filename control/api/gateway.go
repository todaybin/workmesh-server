// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
	Account     string         `json:"account,omitempty"`
	UpdatedAt   string         `json:"updatedAt"`
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

// Start 启动节点自动注册和周期心跳；未配置云端客户端时不创建后台任务。
func (s *GatewayStateStore) Start(ctx context.Context, capabilities []string) {
	s.startMu.Lock()
	if s.started {
		s.startMu.Unlock()
		return
	}
	s.started = true
	s.startMu.Unlock()
	if s.client == nil {
		return
	}
	go func() {
		connect := func() {
			s.mu.RLock()
			bound := s.status.Registration == gateway.RegistrationRegistered && s.auth.BindingID != ""
			registration := gateway.Registration{NodeID: s.status.NodeID, BindingID: s.auth.BindingID, Role: s.status.Role, Registered: bound}
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
				// 绑定凭据仍然有效时保留 registered；仅连接状态降级，避免前端误要求重新绑定。
				s.status.Registration = gateway.RegistrationRegistered
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
				registration := gateway.Registration{NodeID: s.status.NodeID, BindingID: s.auth.BindingID, Role: s.status.Role, Registered: s.status.Registration == gateway.RegistrationRegistered}
				s.mu.RUnlock()
				if err := s.client.Heartbeat(ctx, registration); err != nil {
					s.mu.Lock()
					s.status.Connected = false
					s.status.Reason = err.Error()
					s.mu.Unlock()
					_ = s.persist()
				} else {
					s.mu.Lock()
					if s.auth.BindingID != "" {
						s.status.Registration = gateway.RegistrationRegistered
					}
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
func RegisterGatewayRoutes(mux *http.ServeMux, nodeID, role string, authorizers ...RequestAuthorizer) *GatewayStateStore {
	dataDir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dataDir == "" {
		dataDir = "./data"
	}
	store := &GatewayStateStore{status: gateway.Status{Registration: gateway.RegistrationUnregistered, NodeID: nodeID, GatewayID: os.Getenv("WORKMESH_GATEWAY_ID"), Role: role}, statePath: filepath.Join(dataDir, "gateway-binding.json"), identityPath: filepath.Join(dataDir, "gateway-identity.ed25519")}
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

func (s *GatewayStateStore) loginHandler(w http.ResponseWriter, r *http.Request) {
	var request gateway.LoginRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// 允许绑定表单显式指定 Gateway 地址；地址只影响本次及后续持久化连接。
	if strings.TrimSpace(request.GatewayURL) != "" {
		baseURL := strings.TrimRight(strings.TrimSpace(request.GatewayURL), "/")
		if err := validateGatewayBaseURL(baseURL); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		identity, identityErr := gateway.LoadOrCreateIdentity(s.identityPath)
		if identityErr != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("加载 Gateway 节点身份失败: %w", identityErr))
			return
		}
		client := gateway.NewHTTPClientWithIdentity(baseURL, os.Getenv("WORKMESH_GATEWAY_ID"), os.Getenv("WORKMESH_GATEWAY_SECRET"), identity)
		s.mu.Lock()
		s.client = client
		s.gatewayURL = baseURL
		s.mu.Unlock()
	}
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()
	if client == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("Gateway 未配置"))
		return
	}
	request.Username = strings.TrimSpace(request.Username)
	if request.Username == "" || request.Password == "" {
		writeError(w, http.StatusBadRequest, errors.New("Gateway 用户名和密码不能为空"))
		return
	}
	// 显式绑定必须重新验证用户凭据；本地存在旧快照不能代替本次账号认证。
	auth, err := client.Login(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	s.mu.RLock()
	nodeID := s.status.NodeID
	role := s.status.Role
	existingBindingID := s.auth.BindingID
	s.mu.RUnlock()
	if strings.TrimSpace(nodeID) == "" {
		writeError(w, http.StatusInternalServerError, errNodeIDRequired)
		return
	}

	// 已有绑定先用新登录令牌验证心跳。验证成功时复用 bindingId，保证重复绑定幂等。
	bound := false
	if existingBindingID != "" {
		registration := gateway.Registration{NodeID: nodeID, BindingID: existingBindingID, Role: role, Registered: true}
		if heartbeatErr := client.Heartbeat(r.Context(), registration); heartbeatErr == nil {
			auth.BindingID = existingBindingID
			bound = true
		}
	}
	if !bound {
		registered, registerErr := client.Register(r.Context(), gateway.RegisterRequest{
			NodeID: nodeID, DisplayName: nodeID, Role: role, ProtocolVersion: "v1", Capabilities: append([]string(nil), gatewayCapabilities...),
		})
		if registerErr != nil {
			writeError(w, http.StatusBadGateway, fmt.Errorf("Gateway 节点注册失败: %w", registerErr))
			return
		}
		if registered.BindingID == "" {
			writeError(w, http.StatusBadGateway, errors.New("Gateway 节点注册响应缺少绑定标识"))
			return
		}
		auth.BindingID = registered.BindingID
		if len(registered.Scopes) > 0 {
			auth.Scopes = registered.Scopes
		}
		if registered.ExpiresAt != "" {
			auth.ExpiresAt = registered.ExpiresAt
		}
		auth.Refreshable = auth.Refreshable || registered.Refreshable
		if auth.AccessToken == "" {
			auth.AccessToken = registered.AccessToken
		}
		registration := gateway.Registration{NodeID: nodeID, BindingID: auth.BindingID, Role: role, Registered: true}
		if heartbeatErr := client.Heartbeat(r.Context(), registration); heartbeatErr != nil {
			writeError(w, http.StatusBadGateway, fmt.Errorf("Gateway 节点注册后心跳验证失败: %w", heartbeatErr))
			return
		}
	}
	if auth.BindingID == "" {
		writeError(w, http.StatusBadGateway, errors.New("Gateway 账号登录成功，但节点尚未完成绑定"))
		return
	}
	s.mu.Lock()
	s.status.Registration = gateway.RegistrationRegistered
	s.status.Connected = true
	s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
	s.status.AuthorizationExpireAt = auth.ExpiresAt
	s.status.Reason = ""
	s.auth = auth
	s.account = request.Username
	if s.gatewayURL == "" {
		s.gatewayURL = strings.TrimRight(strings.TrimSpace(os.Getenv("WORKMESH_GATEWAY_URL")), "/")
	}
	if err := s.persistLocked(); err != nil {
		s.mu.Unlock()
		writeError(w, http.StatusInternalServerError, fmt.Errorf("保存 Gateway 授权失败: %w", err))
		return
	}
	s.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"bound": true, "account": request.Username, "nodeId": nodeID, "status": "running"}})
}

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

func (s *GatewayStateStore) registerHandler(w http.ResponseWriter, r *http.Request) {
	var request gatewayRegisterRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request.NodeID = strings.TrimSpace(request.NodeID)
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
	// 前端绑定表单可在请求中提供 Gateway 地址和登录令牌。地址必须经过严格校验，
	// 令牌只注入临时客户端，避免把凭据写入节点配置或响应。
	client := s.client
	if strings.TrimSpace(request.GatewayURL) != "" {
		baseURL := strings.TrimRight(strings.TrimSpace(request.GatewayURL), "/")
		if err := validateGatewayBaseURL(baseURL); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(request.RegistrationToken) == "" {
			writeError(w, http.StatusBadRequest, errors.New("Gateway 注册令牌不能为空"))
			return
		}
		temporary := gateway.NewHTTPClient(baseURL, os.Getenv("WORKMESH_GATEWAY_ID"), os.Getenv("WORKMESH_GATEWAY_SECRET"))
		temporary.AccessToken = strings.TrimSpace(request.RegistrationToken)
		client = temporary
	}
	if client != nil {
		if strings.TrimSpace(request.RegistrationToken) == "" {
			if err := s.loginIfConfigured(r.Context()); err != nil {
				writeError(w, http.StatusBadGateway, err)
				return
			}
		}
		registerRequest := gateway.RegisterRequest{NodeID: request.NodeID, PublicKey: request.PublicKey, DisplayName: request.DisplayName, Role: request.Role, ProtocolVersion: request.ProtocolVersion, Capabilities: request.Capabilities, Metadata: request.Metadata}
		if registerRequest.DisplayName == "" {
			registerRequest.DisplayName = request.NodeID
		}
		if registerRequest.Role == "" {
			s.mu.RLock()
			registerRequest.Role = s.status.Role
			s.mu.RUnlock()
		}
		if registerRequest.ProtocolVersion == "" {
			registerRequest.ProtocolVersion = "v1"
		}
		if len(registerRequest.Capabilities) == 0 {
			registerRequest.Capabilities = append([]string(nil), gatewayCapabilities...)
		}
		auth, err := client.Register(r.Context(), registerRequest)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		s.mu.Lock()
		s.client = client
		s.gatewayURL = strings.TrimRight(strings.TrimSpace(request.GatewayURL), "/")
		if s.gatewayURL == "" {
			s.gatewayURL = strings.TrimRight(strings.TrimSpace(os.Getenv("WORKMESH_GATEWAY_URL")), "/")
		}
		s.status.Registration = gateway.RegistrationRegistered
		s.status.NodeID = registerRequest.NodeID
		s.status.Role = registerRequest.Role
		s.status.Connected = true
		s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
		s.status.AuthorizationExpireAt = auth.ExpiresAt
		s.status.Reason = ""
		s.auth = auth
		if err := s.persistLocked(); err != nil {
			s.mu.Unlock()
			writeError(w, http.StatusInternalServerError, fmt.Errorf("保存 Gateway 绑定失败: %w", err))
			return
		}
		s.mu.Unlock()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"registered": true, "gatewayUrl": s.gatewayURL, "nodeId": registerRequest.NodeID, "status": "running", "bindingId": auth.BindingID}})
		return
	}
	// 未配置真实 Gateway 客户端时禁止伪造注册成功，避免节点绕过云端授权。
	writeError(w, http.StatusServiceUnavailable, errors.New("Gateway 未配置有效凭据，无法完成节点注册"))
}

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
		s.status.AuthorizationExpireAt = refreshed.ExpiresAt
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
	s.account = ""
	if err := s.removePersisted(); err != nil {
		s.mu.Unlock()
		writeError(w, http.StatusInternalServerError, fmt.Errorf("清理 Gateway 绑定失败: %w", err))
		return
	}
	s.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
}

// validateGatewayBaseURL 校验外部 Gateway 地址，拒绝凭据、查询和片段以避免 SSRF 与凭据泄露。
func validateGatewayBaseURL(value string) error {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(value), "/"))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && strings.TrimSpace(os.Getenv("WORKMESH_GATEWAY_ALLOW_HTTP")) == "1")) {
		return errors.New("Gateway 地址必须是无凭据 HTTPS URL；本地 HTTP 需显式启用 WORKMESH_GATEWAY_ALLOW_HTTP=1")
	}
	return nil
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
	s.gatewayURL = strings.TrimRight(strings.TrimSpace(saved.GatewayURL), "/")
	if s.status.GatewayID == "" {
		s.status.GatewayID = strings.TrimSpace(saved.GatewayID)
	}
	s.auth = gateway.Authorization{BindingID: saved.BindingID, Scopes: saved.Scopes, ExpiresAt: saved.ExpiresAt, Refreshable: saved.Refreshable, AccessToken: saved.AccessToken}
	s.account = saved.Account
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
	// 调用方在 persistLocked 中已持有写锁，在 persist 中持有读锁；此处不能再次申请写锁。
	gatewayURL := s.gatewayURL
	account := s.account
	if configuredURL := strings.TrimSpace(os.Getenv("WORKMESH_GATEWAY_URL")); configuredURL != "" {
		gatewayURL = strings.TrimRight(configuredURL, "/")
	}
	saved := persistedGatewayState{Status: status, BindingID: auth.BindingID, Scopes: auth.Scopes, ExpiresAt: auth.ExpiresAt, Refreshable: auth.Refreshable, AccessToken: auth.AccessToken, GatewayURL: gatewayURL, GatewayID: status.GatewayID, Account: account, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
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
