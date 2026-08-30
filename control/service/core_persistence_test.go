// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import "testing"

func TestCoreUserPasswordPersistsAfterReload(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	first := NewCoreService()
	_, session, err := first.Login("admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.UpdateCurrentUser(session.ID, "admin", "admin", "new-password-123"); err != nil {
		t.Fatal(err)
	}
	second := NewCoreService()
	if _, _, err := second.Login("admin", "new-password-123"); err != nil {
		t.Fatalf("服务重载后新密码不可用: %v", err)
	}
	if _, _, err := second.Login("admin", "admin"); err == nil {
		t.Fatal("服务重载后旧密码仍可用")
	}
}
