// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package link

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileSyncStorePersistsAndRestores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "link-sync.json")
	store, err := NewFileSyncStore(path)
	if err != nil {
		t.Fatal(err)
	}
	next, err := store.Push(context.Background(), SyncCursor{Stream: "settings", Version: 0}, []byte(`{"version":1}`))
	if err != nil || next.Version != 1 {
		t.Fatalf("写入同步快照失败: %+v, %v", next, err)
	}
	reloaded, err := NewFileSyncStore(path)
	if err != nil {
		t.Fatal(err)
	}
	payload, cursor, err := reloaded.Pull(context.Background(), SyncCursor{Stream: "settings", Version: 0})
	if err != nil || string(payload) != `{"version":1}` || cursor.Version != 1 {
		t.Fatalf("重启后同步快照不一致: %s %+v %v", payload, cursor, err)
	}
}

func TestFileSyncStoreRejectsStaleConflictAndOversizedPayload(t *testing.T) {
	store, err := NewFileSyncStore(filepath.Join(t.TempDir(), "sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Push(context.Background(), SyncCursor{Stream: "settings", Version: 0}, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Push(context.Background(), SyncCursor{Stream: "settings", Version: 0}, []byte("two")); !errors.Is(err, ErrSyncConflict) {
		t.Fatalf("过期游标应返回冲突: %v", err)
	}
	if _, err := store.Push(context.Background(), SyncCursor{Stream: "large", Version: 0}, make([]byte, maxRequestBytes+1)); err == nil {
		t.Fatal("超大同步数据应被拒绝")
	}
}

func TestFileSyncStoreRejectsCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sync.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileSyncStore(path); err == nil {
		t.Fatal("损坏同步文件应返回错误")
	}
}
