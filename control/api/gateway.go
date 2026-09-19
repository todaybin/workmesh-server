// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/runtime/gateway"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"github.com/todaybin/workmesh-server/runtime/machineid"
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
	mu              sync.RWMutex
	startMu         sync.Mutex
	started         bool
	runCtx          context.Context
	runCapabilities []string
	status          gateway.Status
	auth            gateway.Authorization
	client          gateway.ProtocolClient
	account         string
	// gatewayURL 缓存绑定时使用的地址；即使环境变量未注入，重启后也能恢复连接。
	gatewayURL string
	// statePath 位于数据目录内，仅保存本机绑定快照和受限访问令牌。
	statePath           string
	identityPath        string
	identityMachinePath string
	db                  *sql.DB
	repository          storage.Transactional
	machineCode         string
	boundMachineCode    string
	fingerprintVersion  int
	identityStatus      string
	previousBindingID   string
	deviceDisplayName   string
	resourceSnapshot    func() any
	policyConsumer      func(gateway.ResourcePolicy, int64) error
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

// SetResourceSnapshotProvider 注入节点执行面的即时资源快照函数。
func (s *GatewayStateStore) SetResourceSnapshotProvider(provider func() any) {
	s.mu.Lock()
	s.resourceSnapshot = provider
	s.mu.Unlock()
}

// SetResourcePolicyConsumer 注入 Gateway 软资源策略的本机应用函数。
func (s *GatewayStateStore) SetResourcePolicyConsumer(consumer func(gateway.ResourcePolicy, int64) error) {
	s.mu.Lock()
	s.policyConsumer = consumer
	s.mu.Unlock()
}

// RegisterGatewayRoutes 注册前端使用的 Gateway 状态、注册、心跳和授权接口。
func RegisterGatewayRoutes(mux *http.ServeMux, nodeID, role string, authorizers ...RequestAuthorizer) *GatewayStateStore {
	dataDir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dataDir == "" {
		dataDir = "./data"
	}
	_ = role // 面板内部主/子节点角色不属于 Gateway 设备关系。
	store := &GatewayStateStore{status: gateway.Status{Registration: gateway.RegistrationUnregistered, GatewayID: os.Getenv("WORKMESH_GATEWAY_ID"), Role: "device"}, deviceDisplayName: strings.TrimSpace(nodeID), statePath: filepath.Join(dataDir, "gateway-binding.json"), identityPath: filepath.Join(dataDir, "gateway-identity.ed25519"), identityMachinePath: filepath.Join(dataDir, "gateway-identity.machine"), db: gatewayDB, identityStatus: "unavailable"}
	if store.db != nil {
		if err := ensureGatewayBindingSchema(context.Background(), store.db); err != nil {
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
	if fingerprint, err := machineid.Current(); err != nil {
		store.status.Registration = gateway.RegistrationPending
		store.status.Reason = fmt.Sprintf("读取本机机器身份失败: %v", err)
	} else {
		store.machineCode = fingerprint.Code
		store.fingerprintVersion = fingerprint.Version
		store.identityStatus = "ready"
		store.status.NodeID = gatewayDeviceID(fingerprint.Code)
	}
	store.load()
	if store.machineCode != "" {
		store.status.NodeID = gatewayDeviceID(store.machineCode)
		store.status.Role = "device"
	} else {
		store.identityStatus = "unavailable"
	}
	rotateIdentity := store.reconcileMachineIdentity()
	if !rotateIdentity && store.machineCode != "" {
		rotateIdentity = store.reconcileIdentityMachineMarker()
	}
	var preparedIdentity *gateway.Identity
	var preparedIdentityErr error
	if rotateIdentity {
		preparedIdentity, preparedIdentityErr = gateway.RotateIdentity(store.identityPath)
		if preparedIdentityErr != nil {
			store.identityStatus = "identity_error"
			store.status.Reason = fmt.Sprintf("轮换 Gateway 节点身份失败: %v", preparedIdentityErr)
		}
	}
	// 配置 Gateway 地址后启用真实云端协议；未配置时保留离线开发模式。
	baseURL := strings.TrimSpace(os.Getenv("WORKMESH_GATEWAY_URL"))
	if baseURL == "" {
		store.mu.RLock()
		baseURL = store.gatewayURL
		store.mu.RUnlock()
	}
	if baseURL != "" {
		store.gatewayURL = strings.TrimRight(baseURL, "/")
		identity, identityErr := preparedIdentity, preparedIdentityErr
		if identity == nil && identityErr == nil {
			identity, identityErr = gateway.LoadOrCreateIdentity(store.identityPath)
		}
		if identityErr != nil {
			store.status.Registration = gateway.RegistrationPending
			store.status.Reason = fmt.Sprintf("加载 Gateway 节点身份失败: %v", identityErr)
		} else {
			_ = store.persistIdentityMachineMarker()
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

type gatewaySchemaExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

const gatewayBindingSchema = `CREATE TABLE IF NOT EXISTS gateway_binding (id INTEGER PRIMARY KEY CHECK(id=1), status BLOB NOT NULL, auth BLOB NOT NULL, gateway_url TEXT NOT NULL DEFAULT '', account TEXT NOT NULL DEFAULT '', machine_code TEXT NOT NULL DEFAULT '', bound_machine_code TEXT NOT NULL DEFAULT '', fingerprint_version INTEGER NOT NULL DEFAULT 0, identity_status TEXT NOT NULL DEFAULT '', previous_binding_id TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL)`

// GatewayBindingMigration 返回机器身份绑定的幂等 SQLite 前向迁移。
func GatewayBindingMigration() storage.Migration {
	sum := sha256.Sum256([]byte(gatewayBindingSchema + "|machine-identity-v1"))
	return storage.Migration{
		ID: "0016-gateway-machine-identity", Checksum: hex.EncodeToString(sum[:]),
		Up: func(ctx context.Context, tx *sql.Tx) error { return ensureGatewayBindingSchema(ctx, tx) },
	}
}

func ensureGatewayBindingSchema(ctx context.Context, db gatewaySchemaExecutor) error {
	if _, err := db.ExecContext(ctx, gatewayBindingSchema); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(gateway_binding)`)
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		columns[name] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	definitions := []struct{ name, sql string }{
		{"machine_code", "TEXT NOT NULL DEFAULT ''"},
		{"bound_machine_code", "TEXT NOT NULL DEFAULT ''"},
		{"fingerprint_version", "INTEGER NOT NULL DEFAULT 0"},
		{"identity_status", "TEXT NOT NULL DEFAULT ''"},
		{"previous_binding_id", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range definitions {
		if !columns[column.name] {
			if _, err := db.ExecContext(ctx, `ALTER TABLE gateway_binding ADD COLUMN `+column.name+` `+column.sql); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *GatewayStateStore) reconcileMachineIdentity() bool {
	if s.machineCode == "" {
		return false
	}
	if s.auth.BindingID == "" {
		s.identityStatus = "ready"
		return false
	}
	if s.boundMachineCode == s.machineCode && s.boundMachineCode != "" {
		s.identityStatus = "ready"
		return false
	}
	// 在重新登录完成前每次启动都轮换，避免进程在“已持久化隔离状态、尚未改名旧私钥”之间崩溃后复用源机器身份。
	rotate := true
	s.previousBindingID = s.auth.BindingID
	s.auth = gateway.Authorization{}
	s.status.Registration = gateway.RegistrationPending
	s.status.Connected = false
	s.status.Reason = "检测到机器硬件身份变化，已停用旧绑定，请重新登录 Gateway"
	s.identityStatus = "machine_changed"
	if err := s.persistLocked(); err != nil {
		s.status.Reason = fmt.Sprintf("检测到机器硬件身份变化，但保存隔离状态失败: %v", err)
	}
	return rotate
}

func (s *GatewayStateStore) reconcileIdentityMachineMarker() bool {
	data, err := os.ReadFile(s.identityMachinePath)
	if err == nil && strings.TrimSpace(string(data)) == s.machineCode {
		return false
	}
	if errors.Is(err, os.ErrNotExist) && s.auth.BindingID == "" {
		return false
	}
	if _, err := gateway.RotateIdentity(s.identityPath); err != nil {
		s.identityStatus = "identity_error"
		s.status.Reason = fmt.Sprintf("停用复制的 Gateway 身份失败: %v", err)
		return false
	}
	s.identityStatus = "machine_changed"
	s.status.Registration = gateway.RegistrationPending
	s.status.Connected = false
	s.status.Reason = "检测到复制的 Gateway 身份不属于本机，已生成新身份，请重新登录"
	_ = s.persistLocked()
	return true
}

func (s *GatewayStateStore) persistIdentityMachineMarker() error {
	if s.machineCode == "" || s.identityMachinePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.identityMachinePath), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.identityMachinePath), ".gateway-identity-machine-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(s.machineCode + "\n"); err != nil {
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
	return os.Rename(name, s.identityMachinePath)
}

func gatewayDeviceID(machineCode string) string {
	value := strings.TrimPrefix(strings.TrimSpace(machineCode), "sha256:")
	if len(value) > 24 {
		value = value[:24]
	}
	if value == "" {
		return ""
	}
	return "device-" + value
}

func (s *GatewayStateStore) gatewayRegisterRequest(capabilities []string) gateway.RegisterRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	displayName := strings.TrimSpace(s.deviceDisplayName)
	if displayName == "" {
		displayName, _ = os.Hostname()
	}
	return gateway.RegisterRequest{
		NodeID: s.status.NodeID, DisplayName: displayName, Role: "device", ProtocolVersion: "v2",
		Capabilities: append([]string(nil), capabilities...), MachineCode: s.machineCode,
		FingerprintVersion: s.fingerprintVersion, Platform: runtime.GOOS, Architecture: runtime.GOARCH,
		RuntimeVersion: strings.TrimSpace(os.Getenv("WORKMESH_VERSION")),
	}
}

func (s *GatewayStateStore) machineIdentityError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := s.machineIdentityAvailableErrorLocked(); err != nil {
		return err
	}
	if s.identityStatus == "machine_changed" {
		return errors.New("机器身份已变化，必须重新登录 Gateway")
	}
	return nil
}

func (s *GatewayStateStore) machineIdentityAvailableError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.machineIdentityAvailableErrorLocked()
}

func (s *GatewayStateStore) machineIdentityAvailableErrorLocked() error {
	if s.machineCode == "" || s.fingerprintVersion <= 0 {
		return errors.New("本机没有可用的稳定机器身份")
	}
	if s.identityStatus == "identity_error" {
		return errors.New("Gateway 节点签名身份不可用")
	}
	return nil
}

func maskedMachineCode(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return value
	}
	return value[:7] + "…" + value[len(value)-8:]
}

// statusHandler 返回脱敏的 Gateway 绑定、连接和授权过期状态。
func (s *GatewayStateStore) statusHandler(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	status := s.status
	auth := s.auth
	account := s.account
	machineCode := s.machineCode
	fingerprintVersion := s.fingerprintVersion
	identityStatus := s.identityStatus
	previousBindingRecorded := s.previousBindingID != ""
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
		"machineCode": maskedMachineCode(machineCode), "fingerprintVersion": fingerprintVersion, "identityStatus": identityStatus,
		"previousBindingRecorded": previousBindingRecorded,
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
	if err := s.machineIdentityError(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	if s.client != nil {
		if err := s.sendGatewayHeartbeat(r.Context()); err != nil {
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
