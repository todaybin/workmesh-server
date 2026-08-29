// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package role 定义本机主节点与次节点的角色切换契约。
package role

import "context"

// Node 是角色管理需要的持久化摘要。
type Node struct {
	NodeID        string `json:"nodeId"`
	Role          string `json:"role"`
	RoleEpoch     uint64 `json:"roleEpoch"`
	DisplayName   string `json:"displayName"`
	Endpoint      string `json:"endpoint"`
	CredentialRef string `json:"credentialRef"`
	Status        string `json:"status"`
	LastSeenAt    string `json:"lastSeenAt,omitempty"`
	ChangedBy     string `json:"changedBy,omitempty"`
	ChangedAt     string `json:"changedAt,omitempty"`
}

// Transition 是一次幂等角色切换操作。
type Transition struct {
	OperationID   string `json:"operationId"`
	NodeID        string `json:"nodeId"`
	From          string `json:"from"`
	To            string `json:"to"`
	ExpectedEpoch uint64 `json:"expectedEpoch"`
}

// Store 是角色状态的持久化接口。
type Store interface {
	Current(context.Context) (Node, error)
	CompareAndSet(context.Context, uint64, Node) error
}

// Coordinator 定义 prepare/commit/abort 三阶段切换，不允许直接写角色字段。
type Coordinator interface {
	Prepare(context.Context, Transition) error
	Commit(context.Context, Transition) error
	Abort(context.Context, Transition) error
}
