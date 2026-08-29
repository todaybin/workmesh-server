// SPDX-License-Identifier: LicenseRef-WorkMesh-Pending
// Copyright (c) 2026 WorkMesh contributors

// Package link 定义 WorkMesh 节点之间的认证通信契约。
package link

import "context"

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
	Stream  string `json:"stream"`
	Version uint64 `json:"version"`
}

// Client 是节点到节点的认证传输接口。
type Client interface {
	Handshake(context.Context, Handshake) (Handshake, error)
	Heartbeat(context.Context, Heartbeat) error
	Pull(context.Context, SyncCursor) ([]byte, SyncCursor, error)
	Push(context.Context, SyncCursor, []byte) (SyncCursor, error)
}
