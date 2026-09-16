// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// TestRuntimePersistenceLegacyImportAndRestart 验证运行时相关功能、失败边界和持久化结果。
func TestRuntimePersistenceLegacyImportAndRestart(t *testing.T) {
	dataDir := t.TempDir()
	legacy := runtimeState{
		Runtimes: []runtimeRecord{{ID: "php-82", Name: "PHP 8.2", Type: "php", Version: "8.2", Status: "running", UpdatedAt: time.Now().UTC()}},
		Settings: map[string]any{"ssh": map[string]any{"host": "127.0.0.1", "password": "secret"}},
	}
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "runtime.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := initializeRuntimePersistence(store.DB(), dataDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "runtime.json")); !os.IsNotExist(err) {
		t.Fatalf("旧 runtime.json 未归档: %v", err)
	}
	loaded, err := (runtimeRepository{db: store.DB()}).load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Runtimes) != 1 || loaded.Runtimes[0].ID != "php-82" {
		t.Fatalf("导入运行时清单不匹配: %#v", loaded.Runtimes)
	}
	if loaded.Runtimes[0].CreatedAt.IsZero() || !loaded.Runtimes[0].CreatedAt.Equal(loaded.Runtimes[0].UpdatedAt) {
		t.Fatalf("旧运行时创建时间未从更新时间回填: %#v", loaded.Runtimes[0])
	}
	if loaded.Settings["ssh"] == nil {
		t.Fatalf("导入运行时设置缺失: %#v", loaded.Settings)
	}
	var imports int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM legacy_imports WHERE domain='runtime' AND status='imported'`).Scan(&imports); err != nil {
		t.Fatal(err)
	}
	if imports != 1 {
		t.Fatalf("旧运行时导入记录数=%d, want 1", imports)
	}
	if err := initializeRuntimePersistence(store.DB(), dataDir); err != nil {
		t.Fatal(err)
	}
	var importsAfter int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM legacy_imports WHERE domain='runtime' AND status='imported'`).Scan(&importsAfter); err != nil {
		t.Fatal(err)
	}
	if importsAfter != 1 {
		t.Fatalf("重复初始化改变导入记录数=%d", importsAfter)
	}
}

// TestRuntimePersistenceRequiresSQLite 验证运行时相关功能、失败边界和持久化结果。
func TestRuntimePersistenceRequiresSQLite(t *testing.T) {
	s := &runtimeStore{repository: runtimeRepository{}, state: runtimeState{Settings: map[string]any{}}}
	if err := s.saveLocked(); err == nil {
		t.Fatal("未初始化 SQLite 时保存应失败")
	}
}

func TestRuntimeInstallPathPersistenceFailureRollsBackState(t *testing.T) {
	dataDir := t.TempDir()
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	repository, err := storage.NewSQLiteRepository(store.DB())
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	s := &runtimeStore{
		repository: runtimeRepository{repository: repository},
		state: runtimeState{
			Runtimes: []runtimeRecord{{ID: "go-1", Name: "go-1", Type: "go"}},
			Settings: map[string]any{},
		},
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if err := persistRuntimeInstallPath(s, "go-1", filepath.Join(dataDir, "runtimes", "go", "go-1")); err == nil {
		t.Fatal("SQLite 关闭后保存运行时路径应失败")
	}
	if got := s.state.Runtimes[0].InstallPath; got != "" {
		t.Fatalf("保存失败后不应保留内存路径: %q", got)
	}
	if got := s.state.Runtimes[0].ComposePath; got != "" {
		t.Fatalf("保存失败后不应保留 Compose 路径: %q", got)
	}
}
