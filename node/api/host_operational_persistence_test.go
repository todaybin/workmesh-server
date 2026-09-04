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

func TestHostOperationalLegacyImportAndSQLitePersistence(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	legacyPath := filepath.Join(dataDir, "host-operational.json")
	legacy := hostOperationalState{Monitor: map[string]any{"enabled": false, "interval": 42}}
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		resetSharedStoreForTest()
		_ = store.Close()
	}()
	if err := SetSharedStore(store); err != nil {
		t.Fatal(err)
	}
	hostOperationalMu.Lock()
	loaded := loadHostOperationalStateLocked()
	hostOperationalMu.Unlock()
	if got, _ := loaded.Monitor["interval"].(float64); got != 42 {
		if gotInt, ok := loaded.Monitor["interval"].(int); !ok || gotInt != 42 {
			t.Fatalf("导入监控间隔不匹配: %#v", loaded.Monitor["interval"])
		}
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("旧主机监控 JSON 未归档: %v", err)
	}
	var payloadDB []byte
	if err := store.DB().QueryRow(`SELECT payload FROM node_settings WHERE setting_key='host_operational'`).Scan(&payloadDB); err != nil {
		t.Fatal(err)
	}
	if len(payloadDB) == 0 {
		t.Fatal("SQLite 未保存主机监控设置")
	}
	hostOperationalMu.Lock()
	err = saveHostOperationalStateLocked(hostOperationalState{Monitor: map[string]any{"enabled": true, "interval": 60}})
	hostOperationalMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	hostOperationalMu.Lock()
	reloaded := loadHostOperationalStateLocked()
	hostOperationalMu.Unlock()
	if got, _ := reloaded.Monitor["interval"].(float64); got != 60 {
		if gotInt, ok := reloaded.Monitor["interval"].(int); !ok || gotInt != 60 {
			t.Fatalf("SQLite 重载监控间隔不匹配: %#v", reloaded.Monitor["interval"])
		}
	}
}
