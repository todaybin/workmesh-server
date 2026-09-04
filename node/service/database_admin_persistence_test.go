// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDatabaseAdminLegacyImportArchivesJSON(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	legacyPath := filepath.Join(dataDir, "database-admin.json")
	payload := map[string]any{
		"users":  map[string]any{"1": map[string]any{"id": 1, "database": "app", "username": "admin", "host": "%"}},
		"grants": map[string]any{}, "variables": map[string]any{}, "configs": map[string]string{},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewDatabaseAdminStore()
	db, err := s.database(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		fallbackSQLiteMu.Lock()
		delete(fallbackSQLiteDB, s.fallbackPath)
		fallbackSQLiteMu.Unlock()
		_ = db.Close()
	}()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM database_users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("导入数据库用户数=%d", count)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("旧数据库管理 JSON 未归档: %v", err)
	}
	var archives int
	entries, err := os.ReadDir(filepath.Join(dataDir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			archives++
		}
	}
	if archives != 1 {
		t.Fatalf("归档文件数=%d", archives)
	}
	if _, err := s.database(nil); err != nil {
		t.Fatal(err)
	}
	var countAfter int
	if err := db.QueryRow(`SELECT COUNT(*) FROM database_users`).Scan(&countAfter); err != nil {
		t.Fatal(err)
	}
	if countAfter != 1 {
		t.Fatalf("重复初始化改变用户数=%d", countAfter)
	}
}
