// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/runtime/gateway"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// registerHandler 处理显式令牌注册，并拒绝未配置真实 Gateway 的伪注册。
func (s *GatewayStateStore) registerHandler(w http.ResponseWriter, r *http.Request) {
	var request gatewayRegisterRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.machineIdentityAvailableError(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	s.mu.RLock()
	request.NodeID = s.status.NodeID
	s.mu.RUnlock()
	if request.NodeID == "" {
		writeError(w, http.StatusBadRequest, errNodeIDRequired)
		return
	}
	if s.writeExistingRegistration(w, request.NodeID) {
		return
	}
	client, status, err := s.prepareRegistrationClient(r.Context(), request)
	if err != nil {
		writeError(w, status, err)
		return
	}
	registerRequest := s.registrationRequest(request)
	auth, err := client.Register(r.Context(), registerRequest)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	gatewayURL, err := s.saveRegistration(client, request.GatewayURL, registerRequest, auth)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("保存 Gateway 绑定失败: %w", err))
		return
	}
	s.startGatewayLoopIfReady()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"registered": true, "gatewayUrl": gatewayURL, "nodeId": registerRequest.NodeID, "status": "running", "bindingId": auth.BindingID}})
}

// writeExistingRegistration 返回已绑定节点的幂等结果或冲突响应。
func (s *GatewayStateStore) writeExistingRegistration(w http.ResponseWriter, nodeID string) bool {
	s.mu.RLock()
	registered := s.status.Registration == gateway.RegistrationRegistered && s.auth.BindingID != ""
	boundNode, auth := s.status.NodeID, s.auth
	s.mu.RUnlock()
	if !registered {
		return false
	}
	if nodeID == boundNode {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{
			"registered": true,
			"gatewayUrl": s.gatewayURLValue(),
			"nodeId":     boundNode,
			"status":     "running",
			"bindingId":  auth.BindingID,
		}})
	} else {
		writeError(w, http.StatusConflict, errors.New("Gateway 已绑定其他节点，如需切换请先解绑"))
	}
	return true
}

// prepareRegistrationClient 根据请求令牌选择云端客户端并完成凭据校验。
func (s *GatewayStateStore) prepareRegistrationClient(ctx context.Context, request gatewayRegisterRequest) (gateway.ProtocolClient, int, error) {
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()
	if strings.TrimSpace(request.GatewayURL) != "" {
		baseURL := strings.TrimRight(strings.TrimSpace(request.GatewayURL), "/")
		if err := validateGatewayBaseURL(baseURL); err != nil {
			return nil, http.StatusBadRequest, err
		}
		if strings.TrimSpace(request.RegistrationToken) == "" {
			return nil, http.StatusBadRequest, errors.New("Gateway 注册令牌不能为空")
		}
		identity, err := gateway.LoadOrCreateIdentity(s.identityPath)
		if err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("加载 Gateway 节点身份失败: %w", err)
		}
		temporary := gateway.NewHTTPClientWithIdentity(baseURL, os.Getenv("WORKMESH_GATEWAY_ID"), os.Getenv("WORKMESH_GATEWAY_SECRET"), identity)
		temporary.AccessToken = strings.TrimSpace(request.RegistrationToken)
		client = temporary
	}
	if client == nil {
		return nil, http.StatusServiceUnavailable, errors.New("Gateway 未配置有效凭据，无法完成节点注册")
	}
	if strings.TrimSpace(request.RegistrationToken) == "" {
		if err := s.loginIfConfigured(ctx); err != nil {
			return nil, http.StatusBadGateway, err
		}
	}
	return client, http.StatusOK, nil
}

// registrationRequest 补齐前端可选字段，生成稳定的 v2 节点注册请求。
func (s *GatewayStateStore) registrationRequest(request gatewayRegisterRequest) gateway.RegisterRequest {
	result := s.gatewayRegisterRequest(request.Capabilities)
	result.Metadata = request.Metadata
	if result.DisplayName == "" {
		result.DisplayName = request.DisplayName
	}
	if len(result.Capabilities) == 0 {
		result.Capabilities = append([]string(nil), gatewayCapabilities...)
	}
	return result
}

// saveRegistration 原子更新本地绑定状态并返回实际持久化的 Gateway 地址。
func (s *GatewayStateStore) saveRegistration(client gateway.ProtocolClient, requestedURL string, request gateway.RegisterRequest, auth gateway.Authorization) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previousStatus, previousAuth := s.status, s.auth
	previousClient, previousURL := s.client, s.gatewayURL
	previousBoundMachineCode, previousIdentityStatus := s.boundMachineCode, s.identityStatus
	s.client = client
	s.gatewayURL = strings.TrimRight(strings.TrimSpace(requestedURL), "/")
	if s.gatewayURL == "" {
		s.gatewayURL = strings.TrimRight(strings.TrimSpace(os.Getenv("WORKMESH_GATEWAY_URL")), "/")
	}
	s.status.Registration = gateway.RegistrationRegistered
	s.status.NodeID = request.NodeID
	s.status.Role = request.Role
	s.status.Connected = true
	s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
	s.status.AuthorizationExpireAt = auth.ExpiresAt
	s.status.Reason = ""
	s.auth = auth
	s.boundMachineCode = s.machineCode
	s.identityStatus = "ready"
	if err := s.persistLocked(); err != nil {
		s.status, s.auth = previousStatus, previousAuth
		s.client, s.gatewayURL = previousClient, previousURL
		s.boundMachineCode, s.identityStatus = previousBoundMachineCode, previousIdentityStatus
		return "", err
	}
	if err := s.persistIdentityMachineMarker(); err != nil {
		s.status, s.auth = previousStatus, previousAuth
		s.client, s.gatewayURL = previousClient, previousURL
		s.boundMachineCode, s.identityStatus = previousBoundMachineCode, previousIdentityStatus
		_ = s.persistLocked()
		return "", fmt.Errorf("保存 Gateway 机器身份标记失败: %w", err)
	}
	return s.gatewayURL, nil
}
