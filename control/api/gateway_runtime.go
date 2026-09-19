// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/todaybin/workmesh-server/runtime/gateway"
)

// Start 启动节点自动注册和周期心跳；未配置云端客户端时不创建后台任务。
func (s *GatewayStateStore) Start(ctx context.Context, capabilities []string) {
	s.startMu.Lock()
	s.runCtx = ctx
	s.runCapabilities = append([]string(nil), capabilities...)
	s.startMu.Unlock()
	s.startGatewayLoopIfReady()
}

func (s *GatewayStateStore) startGatewayLoopIfReady() {
	s.startMu.Lock()
	if s.started || s.runCtx == nil {
		s.startMu.Unlock()
		return
	}
	s.mu.RLock()
	clientReady := s.client != nil
	s.mu.RUnlock()
	if !clientReady || s.machineIdentityError() != nil {
		s.startMu.Unlock()
		return
	}
	s.started = true
	ctx := s.runCtx
	capabilities := append([]string(nil), s.runCapabilities...)
	s.startMu.Unlock()
	go func() {
		defer func() {
			s.startMu.Lock()
			s.started = false
			s.startMu.Unlock()
		}()
		s.runGatewayLoop(ctx, capabilities)
	}()
}

// runGatewayLoop 执行首次连接和周期心跳，直到节点上下文被取消。
func (s *GatewayStateStore) runGatewayLoop(ctx context.Context, capabilities []string) {
	s.connectGateway(ctx, capabilities)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.gatewayHeartbeat(ctx)
		}
	}
}

// connectGateway 恢复已有绑定，或使用配置凭据完成首次注册。
func (s *GatewayStateStore) connectGateway(ctx context.Context, capabilities []string) {
	s.mu.RLock()
	bound := s.status.Registration == gateway.RegistrationRegistered && s.auth.BindingID != ""
	s.mu.RUnlock()
	// 已绑定节点优先恢复心跳，不再次要求账号登录或创建新绑定。
	if bound {
		if err := s.sendGatewayHeartbeat(ctx); err == nil {
			s.mu.Lock()
			previous := s.status
			s.status.Connected = true
			s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
			s.status.Reason = ""
			if persistErr := s.persistLocked(); persistErr != nil {
				s.status = previous
				s.mu.Unlock()
				log.Printf("gateway: persist recovered heartbeat state failed: %v", persistErr)
				return
			}
			s.mu.Unlock()
			return
		}
		if persistErr := s.markGatewayOffline("Gateway 绑定已保存，但心跳恢复失败"); persistErr != nil {
			log.Printf("gateway: persist offline state failed: %v", persistErr)
		}
		return
	}
	if err := s.loginIfConfigured(ctx); err != nil {
		if persistErr := s.markGatewayPending(err.Error()); persistErr != nil {
			log.Printf("gateway: persist pending state failed: %v", persistErr)
		}
		return
	}
	request := s.gatewayRegisterRequest(capabilities)
	auth, err := s.client.Register(ctx, request)
	s.mu.Lock()
	previousStatus, previousAuth := s.status, s.auth
	previousBoundMachineCode, previousIdentityStatus := s.boundMachineCode, s.identityStatus
	if err != nil {
		s.status.Registration = gateway.RegistrationPending
		s.status.Connected = false
		s.status.Reason = err.Error()
		if persistErr := s.persistLocked(); persistErr != nil {
			s.status, s.auth = previousStatus, previousAuth
			s.mu.Unlock()
			log.Printf("gateway: persist registration failure state failed: %v", persistErr)
			return
		}
		s.mu.Unlock()
		return
	}
	s.status.Registration = gateway.RegistrationRegistered
	s.status.Connected = true
	s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
	s.status.Reason = ""
	s.auth = auth
	s.boundMachineCode = s.machineCode
	s.identityStatus = "ready"
	if persistErr := s.persistLocked(); persistErr != nil {
		s.status = previousStatus
		s.auth = previousAuth
		s.boundMachineCode, s.identityStatus = previousBoundMachineCode, previousIdentityStatus
		s.mu.Unlock()
		log.Printf("gateway: persist registration state failed: %v", persistErr)
		return
	}
	s.mu.Unlock()
}

// gatewayHeartbeat 发送周期心跳并持久化最新连接状态。
func (s *GatewayStateStore) gatewayHeartbeat(ctx context.Context) {
	if err := s.sendGatewayHeartbeat(ctx); err != nil {
		s.mu.Lock()
		previous := s.status
		s.status.Connected = false
		s.status.Reason = err.Error()
		persistErr := s.persistLocked()
		if persistErr != nil {
			s.status = previous
		}
		s.mu.Unlock()
		if persistErr != nil {
			log.Printf("gateway: persist heartbeat failure state failed: %v", persistErr)
		}
		return
	}
	s.mu.Lock()
	previous := s.status
	if s.auth.BindingID != "" {
		s.status.Registration = gateway.RegistrationRegistered
	}
	s.status.Connected = true
	s.status.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
	persistErr := s.persistLocked()
	if persistErr != nil {
		s.status = previous
	}
	s.mu.Unlock()
	if persistErr != nil {
		log.Printf("gateway: persist heartbeat state failed: %v", persistErr)
	}
}

func (s *GatewayStateStore) registrationSnapshot(bindingID string, registered bool) gateway.Registration {
	s.mu.RLock()
	provider := s.resourceSnapshot
	registration := gateway.Registration{
		NodeID: s.status.NodeID, BindingID: bindingID, Role: "device", Registered: registered,
		Capabilities: append([]string(nil), gatewayCapabilities...), MachineCode: s.machineCode,
		FingerprintVersion: s.fingerprintVersion,
	}
	s.mu.RUnlock()
	if provider != nil {
		registration.ResourceSnapshot = provider()
	}
	return registration
}

func (s *GatewayStateStore) heartbeatClient(ctx context.Context, client gateway.ProtocolClient, registration gateway.Registration) error {
	if extended, ok := client.(gateway.PolicyProtocolClient); ok {
		result, err := extended.HeartbeatWithResult(ctx, registration)
		if err != nil {
			return err
		}
		s.mu.RLock()
		consumer := s.policyConsumer
		s.mu.RUnlock()
		if consumer != nil {
			return consumer(result.ResourcePolicy, result.PolicyRevision)
		}
		return nil
	}
	return client.Heartbeat(ctx, registration)
}

func (s *GatewayStateStore) sendGatewayHeartbeat(ctx context.Context) error {
	if err := s.machineIdentityError(); err != nil {
		return err
	}
	s.mu.RLock()
	client, bindingID := s.client, s.auth.BindingID
	registered := s.status.Registration == gateway.RegistrationRegistered
	s.mu.RUnlock()
	if client == nil {
		return errors.New("Gateway 未配置")
	}
	return s.heartbeatClient(ctx, client, s.registrationSnapshot(bindingID, registered))
}

// markGatewayOffline 标记已绑定节点的心跳恢复失败并保存状态。
func (s *GatewayStateStore) markGatewayOffline(reason string) error {
	s.mu.Lock()
	previous := s.status
	s.status.Connected = false
	s.status.Registration = gateway.RegistrationRegistered
	s.status.Reason = reason
	err := s.persistLocked()
	if err != nil {
		s.status = previous
	}
	s.mu.Unlock()
	return err
}

// markGatewayPending 标记自动登录失败，等待下一次人工或周期连接。
func (s *GatewayStateStore) markGatewayPending(reason string) error {
	s.mu.Lock()
	previous := s.status
	s.status.Registration = gateway.RegistrationPending
	s.status.Connected = false
	s.status.Reason = reason
	err := s.persistLocked()
	if err != nil {
		s.status = previous
	}
	s.mu.Unlock()
	return err
}

// loginIfConfigured 使用环境变量中的 Gateway 账号换取短期授权，并持久化授权摘要。
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
	previous := s.auth
	s.auth = auth
	err = s.persistLocked()
	if err != nil {
		s.auth = previous
	}
	s.mu.Unlock()
	return err
}
