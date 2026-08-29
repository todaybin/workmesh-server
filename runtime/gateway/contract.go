// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package gateway 定义 WorkMesh Server 与云端 Gateway 的协议边界。
package gateway

import "context"

// RegistrationState 表示当前节点在 Gateway 上的注册状态。
type RegistrationState string

const (
	RegistrationUnregistered RegistrationState = "unregistered"
	RegistrationPending      RegistrationState = "pending"
	RegistrationRegistered   RegistrationState = "registered"
	RegistrationRevoked      RegistrationState = "revoked"
)

// RouteMode 表示能力请求的执行路由。
type RouteMode string

const (
	RouteLocal   RouteMode = "local"
	RouteGateway RouteMode = "gateway"
	RouteAuto    RouteMode = "auto"
)

// Status 是本机保存并对前端公开的 Gateway 状态摘要。
type Status struct {
	Registration          RegistrationState `json:"registration"`
	NodeID                string            `json:"nodeId"`
	GatewayID             string            `json:"gatewayId,omitempty"`
	Role                  string            `json:"role"`
	Connected             bool              `json:"connected"`
	AuthorizationExpireAt string            `json:"authorizationExpiresAt,omitempty"`
	LastSeenAt            string            `json:"lastSeenAt,omitempty"`
	Reason                string            `json:"reason,omitempty"`
}

// LoginRequest 是 Gateway 账号授权请求。密码只允许通过 TLS 传输，不得写入日志。
type LoginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	GatewayURL string `json:"gatewayUrl,omitempty"`
}

// RegisterRequest 是节点首次注册请求。
type RegisterRequest struct {
	NodeID          string            `json:"nodeId"`
	DisplayName     string            `json:"displayName"`
	Role            string            `json:"role"`
	ProtocolVersion string            `json:"protocolVersion"`
	Capabilities    []string          `json:"capabilities"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// Authorization 包含注册后可缓存的非敏感授权摘要。
type Authorization struct {
	BindingID   string   `json:"bindingId"`
	Scopes      []string `json:"scopes"`
	ExpiresAt   string   `json:"expiresAt"`
	Refreshable bool     `json:"refreshable"`
}

// ProtocolClient 是完整 Gateway 访问适配器，具体 HTTP、签名和重试策略由 runtime 实现。
// 现有 Client 接口保留用于最小注册/心跳适配；实现完整协议时使用本接口。
type ProtocolClient interface {
	Login(context.Context, LoginRequest) (Authorization, error)
	Register(context.Context, RegisterRequest) (Authorization, error)
	Heartbeat(context.Context, Registration) error
	Status(context.Context) (Status, error)
	Refresh(context.Context) (Authorization, error)
	Revoke(context.Context) error
}

// CapabilityRouter 统一本机调用和 Gateway 授权调用的能力入口。
// 实现必须在执行前校验注册状态、项目权限、资源策略和幂等键。
type CapabilityRouter interface {
	Resolve(context.Context, RouteMode, string) (RouteMode, error)
}
