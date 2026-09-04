// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
)

func TestAIPersistenceLegacyImportAndSQLiteRestart(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	legacy := aiPersistentData{Accounts: []map[string]any{{"id": "acct-1", "provider": "openai", "name": "legacy"}}}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(dataDir, "ai.json")
	if err := os.WriteFile(legacyPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		resetSharedStoreForTest()
		_ = store.Close()
		aiState = executionState{}
	}()
	if err := SetSharedStore(store); err != nil {
		t.Fatal(err)
	}
	aiState = executionState{}
	loaded := getAIState()
	if len(loaded.data.Accounts) != 1 || aiString(loaded.data.Accounts[0], "id") != "acct-1" {
		t.Fatalf("AI 旧数据导入失败: %#v", loaded.data.Accounts)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("旧 ai.json 未归档: %v", err)
	}
	loaded.mu.Lock()
	loaded.data.Accounts[0]["name"] = "sqlite"
	if err := loaded.saveLocked(); err != nil {
		loaded.mu.Unlock()
		t.Fatal(err)
	}
	loaded.mu.Unlock()
	aiState = executionState{}
	restarted := getAIState()
	if len(restarted.data.Accounts) != 1 || aiString(restarted.data.Accounts[0], "name") != "sqlite" {
		t.Fatalf("AI SQLite 重启恢复失败: %#v", restarted.data.Accounts)
	}
	var rows int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM ai_state`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("ai_state 行数=%d", rows)
	}
}
