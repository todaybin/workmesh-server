// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package role

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentManagerRestoresRoleEpoch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "role-state.json")
	manager, err := NewPersistent("node-1", Primary, path)
	if err != nil {
		t.Fatal(err)
	}
	state, err := manager.Switch(context.Background(), 1, Secondary)
	if err != nil || state.Role != Secondary || state.RoleEpoch != 2 {
		t.Fatalf("首次切换失败: state=%+v err=%v", state, err)
	}
	reloaded, err := NewPersistent("node-1", Primary, path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.State(context.Background()); got != state {
		t.Fatalf("重启后角色状态不一致: got=%+v want=%+v", got, state)
	}
	if _, err := reloaded.Switch(context.Background(), 1, Primary); err == nil {
		t.Fatal("重启后过期 epoch 必须被拒绝")
	}
}

func TestPersistentManagerRejectsCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "role-state.json")
	if err := os.WriteFile(path, []byte(`{"nodeId":"other","role":"primary","roleEpoch":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPersistent("node-1", Primary, path); err == nil {
		t.Fatal("节点身份不一致的角色状态必须拒绝")
	}
}
