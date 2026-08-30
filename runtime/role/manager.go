// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package role

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	mu        sync.RWMutex
	state     State
	statePath string
}

// New 创建角色管理器并校验角色值。
func New(nodeID, initialRole string) (*Manager, error) {
	if initialRole != Primary && initialRole != Secondary {
		return nil, errors.New("节点角色必须为 primary 或 secondary")
	}
	return &Manager{state: State{NodeID: nodeID, Role: initialRole, RoleEpoch: 1}}, nil
}

// NewPersistent 创建带磁盘状态的角色管理器；状态文件损坏时返回错误，避免静默回退造成双主。
func NewPersistent(nodeID, initialRole, statePath string) (*Manager, error) {
	manager, err := New(nodeID, initialRole)
	if err != nil {
		return nil, err
	}
	statePath = strings.TrimSpace(statePath)
	if statePath == "" {
		return manager, nil
	}
	if !filepath.IsAbs(statePath) {
		return nil, errors.New("角色状态路径必须是绝对路径")
	}
	manager.statePath = statePath
	if err := manager.load(); err != nil {
		return nil, err
	}
	return manager, nil
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
	previous := m.state
	m.state.Role = target
	m.state.RoleEpoch++
	if m.statePath != "" {
		if err := m.persistLocked(); err != nil {
			m.state = previous
			return State{}, fmt.Errorf("保存角色状态失败: %w", err)
		}
	}
	return m.state, nil
}

// load 从原子状态文件恢复角色与 fencing epoch，并校验节点身份。
func (m *Manager) load() error {
	raw, err := os.ReadFile(m.statePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取角色状态失败: %w", err)
	}
	var saved State
	if err := json.Unmarshal(raw, &saved); err != nil {
		return fmt.Errorf("解析角色状态失败: %w", err)
	}
	if saved.NodeID != m.state.NodeID || (saved.Role != Primary && saved.Role != Secondary) || saved.RoleEpoch == 0 {
		return errors.New("角色状态文件内容无效")
	}
	m.state = saved
	return nil
}

func (m *Manager) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(m.statePath), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(m.statePath), ".role-state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
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
	return os.Rename(tmpName, m.statePath)
}

// Persist 将当前角色快照写入磁盘，供启动恢复或运维工具显式调用。
func (m *Manager) Persist() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.statePath == "" {
		return errors.New("角色状态未配置持久化路径")
	}
	return m.persistLocked()
}

// StatePath 返回角色状态文件路径；未启用持久化时返回空字符串。
func (m *Manager) StatePath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.statePath
}
