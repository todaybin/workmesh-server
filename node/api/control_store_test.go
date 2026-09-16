// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"path/filepath"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
)

func TestSaveJSONStateReturnsClosedDatabaseError(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	defer resetSharedStoreForTest()

	if err := saveJSONState("functional_domain_state", map[string]any{"language": "en"}); err == nil {
		t.Fatal("共享 SQLite 已关闭时，saveJSONState 不应返回成功")
	}
}
