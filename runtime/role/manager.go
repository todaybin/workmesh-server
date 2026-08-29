// SPDX-License-Identifier: LicenseRef-WorkMesh-Pending
// Copyright (c) 2026 WorkMesh contributors

package role

import (
	"context"
	"errors"
	"sync"
)

const (
	// Primary 表示主节点。
	Primary = "primary"
	// Secondary 表示次节点。
	Secondary = "secondary"
)

// State 是节点角色及 fencing 版本。
type State struct {
	NodeID    string `json:"node_id"`
	Role      string `json:"role"`
	RoleEpoch uint64 `json:"role_epoch"`
}

// Manager 在进程内提供角色读取和安全切换的最小抽象。
type Manager struct {
	mu    sync.RWMutex
	state State
}

// New 创建角色管理器并校验角色值。
func New(nodeID, initialRole string) (*Manager, error) {
	if initialRole != Primary && initialRole != Secondary {
		return nil, errors.New("节点角色必须为 primary 或 secondary")
	}
	return &Manager{state: State{NodeID: nodeID, Role: initialRole, RoleEpoch: 1}}, nil
}

// State 返回当前角色快照。
func (m *Manager) State(context.Context) State { m.mu.RLock(); defer m.mu.RUnlock(); return m.state }

// Switch 在期望 epoch 匹配时切换角色，防止并发请求造成双主。
func (m *Manager) Switch(ctx context.Context, expectedEpoch uint64, target string) (State, error) {
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	if target != Primary && target != Secondary {
		return State{}, errors.New("无效的目标角色")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if expectedEpoch != m.state.RoleEpoch {
		return State{}, errors.New("角色版本冲突")
	}
	if target == m.state.Role {
		return m.state, nil
	}
	m.state.Role = target
	m.state.RoleEpoch++
	return m.state, nil
}
