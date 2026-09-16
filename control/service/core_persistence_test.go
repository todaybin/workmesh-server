// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
)

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

func TestCoreAdminCredentialsFromEnvironment(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_ADMIN_USERNAME", "workmesh")
	t.Setenv("WORKMESH_ADMIN_PASSWORD", "test-password")
	s := NewCoreService()
	if _, _, err := s.Login("workmesh", "test-password"); err != nil {
		t.Fatalf("环境变量管理员凭据登录失败: %v", err)
	}
	if _, _, err := s.Login("admin", "admin"); err == nil {
		t.Fatal("配置自定义管理员后不应接受默认凭据")
	}
}

func TestCoreAPIKeyPersistsAfterReload(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	first := NewCoreService()
	_, session, err := first.Login("admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	key, err := first.GenerateAPIKey(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.UpdateAPIConfig(session.ID, APIConfig{Enabled: true, Key: key, IPWhiteList: "127.0.0.1", ValidityHours: 24}); err != nil {
		t.Fatal(err)
	}

	second := NewCoreService()
	if _, err := second.Current(key); err != nil {
		t.Fatalf("服务重载后 API Key 不可用: %v", err)
	}
	config, err := second.APIConfig(key)
	if err != nil {
		t.Fatal(err)
	}
	if !config.Enabled || config.IPWhiteList != "127.0.0.1" || config.ValidityHours != 24 {
		t.Fatalf("服务重载后 API 配置不完整: %#v", config)
	}
}

func TestCoreSQLiteIgnoresLegacyJSONAfterInitialization(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	legacy := map[string]persistedUser{
		"admin": {ID: "admin", Name: "admin", Role: "ADMIN", Password: hashPassword("legacy-password")},
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "users.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := NewCoreService()
	if err := first.SetDatabase(store.DB()); err != nil {
		t.Fatal(err)
	}
	legacy["admin"] = persistedUser{ID: "admin", Name: "admin", Role: "ADMIN", Password: hashPassword("stale-file-password")}
	raw, err = json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "users.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	second := NewCoreService()
	if err := second.SetDatabase(store.DB()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := second.Login("admin", "legacy-password"); err != nil {
		t.Fatalf("SQLite 用户未恢复: %v", err)
	}
	if _, _, err := second.Login("admin", "stale-file-password"); err == nil {
		t.Fatal("SQLite 已初始化后不应回读 users.json")
	}
}

func TestCoreGroupsAndSettingsPersistThroughRepository(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	first := NewCoreService()
	if err := first.SetDatabase(store.DB()); err != nil {
		t.Fatal(err)
	}
	if _, err := first.UpsertGroup("group-repository", "运维", "host"); err != nil {
		t.Fatal(err)
	}
	if err := first.UpdateSettings(map[string]string{"language": "en", "securityEntrance": "secure"}); err != nil {
		t.Fatal(err)
	}

	second := NewCoreService()
	if err := second.SetDatabase(store.DB()); err != nil {
		t.Fatal(err)
	}
	groups := second.Groups()
	if len(groups) != 1 || groups[0]["id"] != "group-repository" || groups[0]["name"] != "运维" {
		t.Fatalf("repository 未恢复核心分组: %#v", groups)
	}
	settings := second.Settings()
	if settings["language"] != "en" || settings["securityEntrance"] != "secure" {
		t.Fatalf("repository 未恢复核心设置: %#v", settings)
	}
}

func TestCoreGroupAndSettingsPersistenceFailureRollsBackMemory(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	first := NewCoreService()
	if err := first.SetDatabase(store.DB()); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if _, err := first.UpsertGroup("stable", "稳定", "host"); err != nil {
		store.Close()
		t.Fatal(err)
	}
	store.Close()

	if _, err := first.UpsertGroup("failed", "失败", "host"); err == nil {
		t.Fatal("数据库关闭后分组写入应失败")
	}
	if err := first.UpdateSettings(map[string]string{"language": "en"}); err == nil {
		t.Fatal("数据库关闭后设置写入应失败")
	}
	if err := first.DeleteGroup("stable"); err == nil {
		t.Fatal("数据库关闭后分组删除应失败")
	}
	groups := first.Groups()
	if len(groups) != 1 || groups[0]["id"] != "stable" {
		t.Fatalf("持久化失败后分组内存快照被错误修改: %#v", groups)
	}
	if first.Settings()["language"] != "zh" {
		t.Fatalf("持久化失败后设置内存快照被错误修改: %#v", first.Settings())
	}
}
