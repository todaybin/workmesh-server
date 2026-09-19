// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/runtime/gateway"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// loginHandler 校验 Gateway 账号，完成节点注册/复用绑定并写入 SQLite 状态。
func (s *GatewayStateStore) loginHandler(w http.ResponseWriter, r *http.Request) {
	var request gateway.LoginRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	client, status, err := s.prepareLoginClient(&request)
	if err != nil {
		writeError(w, status, err)
		return
	}
	auth, nodeID, status, err := s.authenticateAndBind(r, client, request)
	if err != nil {
		writeError(w, status, err)
		return
	}
	if err := s.saveLoginState(auth, request.Username); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("保存 Gateway 授权失败: %w", err))
		return
	}
	s.startGatewayLoopIfReady()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"bound": true, "account": request.Username, "nodeId": nodeID, "status": "running"}})
}

// prepareLoginClient 校验登录地址和账号，并按请求切换 Gateway 客户端。
func (s *GatewayStateStore) prepareLoginClient(request *gateway.LoginRequest) (gateway.ProtocolClient, int, error) {
	if err := s.machineIdentityAvailableError(); err != nil {
		return nil, http.StatusConflict, err
	}
	if strings.TrimSpace(request.GatewayURL) != "" {
		baseURL := strings.TrimRight(strings.TrimSpace(request.GatewayURL), "/")
		if err := validateGatewayBaseURL(baseURL); err != nil {
			return nil, http.StatusBadRequest, err
		}
		identity, err := gateway.LoadOrCreateIdentity(s.identityPath)
		if err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("加载 Gateway 节点身份失败: %w", err)
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
		return nil, http.StatusServiceUnavailable, errors.New("Gateway 未配置")
	}
	request.Username = strings.TrimSpace(request.Username)
	if request.Username == "" || request.Password == "" {
		return nil, http.StatusBadRequest, errors.New("Gateway 用户名和密码不能为空")
	}
	return client, http.StatusOK, nil
}

// authenticateAndBind 使用新令牌登录并复用或创建节点绑定。
func (s *GatewayStateStore) authenticateAndBind(r *http.Request, client gateway.ProtocolClient, request gateway.LoginRequest) (gateway.Authorization, string, int, error) {
	auth, err := client.Login(r.Context(), request)
	if err != nil {
		return gateway.Authorization{}, "", http.StatusBadGateway, err
	}
	s.mu.RLock()
	nodeID, existingBindingID, identityStatus := s.status.NodeID, s.auth.BindingID, s.identityStatus
	s.mu.RUnlock()
	if strings.TrimSpace(nodeID) == "" {
		return gateway.Authorization{}, "", http.StatusInternalServerError, errNodeIDRequired
	}
	if existingBindingID != "" && identityStatus != "machine_changed" {
		registration := s.registrationSnapshot(existingBindingID, true)
		if s.heartbeatClient(r.Context(), client, registration) == nil {
			auth.BindingID = existingBindingID
			return auth, nodeID, http.StatusOK, nil
		}
	}
	auth, err = s.registerAfterLogin(r, client, auth)
	if err != nil {
		return gateway.Authorization{}, nodeID, http.StatusBadGateway, err
	}
	return auth, nodeID, http.StatusOK, nil
}

// registerAfterLogin 创建节点绑定并通过心跳验证云端状态。
func (s *GatewayStateStore) registerAfterLogin(r *http.Request, client gateway.ProtocolClient, auth gateway.Authorization) (gateway.Authorization, error) {
	request := s.gatewayRegisterRequest(gatewayCapabilities)
	registered, err := client.Register(r.Context(), request)
	if err != nil {
		return gateway.Authorization{}, fmt.Errorf("Gateway 节点注册失败: %w", err)
	}
	if registered.BindingID == "" {
		return gateway.Authorization{}, errors.New("Gateway 节点注册响应缺少绑定标识")
	}
	auth = mergeGatewayAuthorization(auth, registered)
	registration := s.registrationSnapshot(auth.BindingID, true)
	if err := s.heartbeatClient(r.Context(), client, registration); err != nil {
		return gateway.Authorization{}, fmt.Errorf("Gateway 节点注册后心跳验证失败: %w", err)
	}
	return auth, nil
}

// mergeGatewayAuthorization 合并登录和注册接口返回的授权字段。
func mergeGatewayAuthorization(auth, registered gateway.Authorization) gateway.Authorization {
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
	return auth
}

// saveLoginState 保存登录后的绑定状态，并更新授权过期和连接时间。
func (s *GatewayStateStore) saveLoginState(auth gateway.Authorization, account string) error {
	if auth.BindingID == "" {
		return errors.New("Gateway 账号登录成功，但节点尚未完成绑定")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previousStatus, previousAuth, previousAccount := s.status, s.auth, s.account
	previousURL := s.gatewayURL
	previousBoundMachineCode, previousIdentityStatus := s.boundMachineCode, s.identityStatus
	s.status.Registration = gateway.RegistrationRegistered
	s.status.Connected = true
	s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
	s.status.AuthorizationExpireAt = auth.ExpiresAt
	s.status.Reason = ""
	s.auth = auth
	s.account = account
	s.boundMachineCode = s.machineCode
	s.identityStatus = "ready"
	if s.gatewayURL == "" {
		s.gatewayURL = strings.TrimRight(strings.TrimSpace(os.Getenv("WORKMESH_GATEWAY_URL")), "/")
	}
	if err := s.persistLocked(); err != nil {
		s.status, s.auth, s.account, s.gatewayURL = previousStatus, previousAuth, previousAccount, previousURL
		s.boundMachineCode, s.identityStatus = previousBoundMachineCode, previousIdentityStatus
		return err
	}
	if err := s.persistIdentityMachineMarker(); err != nil {
		s.status, s.auth, s.account, s.gatewayURL = previousStatus, previousAuth, previousAccount, previousURL
		s.boundMachineCode, s.identityStatus = previousBoundMachineCode, previousIdentityStatus
		_ = s.persistLocked()
		return fmt.Errorf("保存 Gateway 机器身份标记失败: %w", err)
	}
	return nil
}
