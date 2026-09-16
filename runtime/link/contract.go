// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package link 定义 WorkMesh 节点之间的认证通信契约。
package link

import (
	"context"
	"net/http"
	"time"
)

// Handshake 是节点建立连接时交换的身份和能力信息。
type Handshake struct {
	NodeID          string   `json:"nodeId"`
	Role            string   `json:"role"`
	RoleEpoch       uint64   `json:"roleEpoch"`
	ProtocolVersion string   `json:"protocolVersion"`
	Capabilities    []string `json:"capabilities"`
	Nonce           string   `json:"nonce"`
}

// Heartbeat 是节点在线状态上报。
type Heartbeat struct {
	NodeID    string `json:"nodeId"`
	RoleEpoch uint64 `json:"roleEpoch"`
	Version   string `json:"version"`
	SentAt    string `json:"sentAt"`
}

// SyncCursor 用于控制面增量同步和断点恢复。
type SyncCursor struct {
	Stream    string `json:"stream"`
	Version   uint64 `json:"version"`
	RoleEpoch uint64 `json:"roleEpoch,omitempty"`
}

// Client 是节点到节点的认证传输接口。
type Client interface {
	Handshake(context.Context, Handshake) (Handshake, error)
	Heartbeat(context.Context, Heartbeat) error
	Pull(context.Context, SyncCursor) ([]byte, SyncCursor, error)
	Push(context.Context, SyncCursor, []byte) (SyncCursor, error)
}

// HTTPClientOptions 配置节点链路 HTTP 客户端。Secret 为空时不附加签名，适合本地开发。
type HTTPClientOptions struct {
	HTTP         HTTPDoer
	Secret       []byte
	NodeID       string
	Timeout      time.Duration
	MaxRetries   int
	RetryBackoff time.Duration
	Clock        Clock
	Nonce        NonceGenerator
}

// Clock 允许测试注入确定性的时间源。
type Clock func() time.Time

// NonceGenerator 允许测试注入确定性的随机数源。
type NonceGenerator func() (string, error)

// HTTPDoer 是 http.Client 的最小接口，方便单元测试注入传输层。
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// 签名协议使用的请求头名称。
const (
	HeaderNodeID    = "X-WorkMesh-Node-ID"
	HeaderTimestamp = "X-WorkMesh-Timestamp"
	HeaderNonce     = "X-WorkMesh-Nonce"
	HeaderSignature = "X-WorkMesh-Signature"
	HeaderRoleEpoch = "X-WorkMesh-Role-Epoch"
)
