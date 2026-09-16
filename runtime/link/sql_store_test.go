// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package link

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
)

func TestSQLiteSyncStoreUsesRepositoryTransactionBoundary(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repo, err := store.Repository()
	if err != nil {
		t.Fatal(err)
	}
	syncStore, err := NewSQLiteSyncStoreWithRepository(repo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	first := []byte(`{"status":"created"}`)
	cursor, err := syncStore.Push(ctx, SyncCursor{Stream: "tasks.repository", RoleEpoch: 3}, first)
	if err != nil || cursor.Version != 1 {
		t.Fatalf("首次写入失败: cursor=%+v err=%v", cursor, err)
	}
	conflict, err := syncStore.Push(ctx, SyncCursor{Stream: "tasks.repository", Version: 0, RoleEpoch: 3}, []byte(`{"status":"changed"}`))
	if !errors.Is(err, ErrSyncConflict) || conflict.Version != 1 {
		t.Fatalf("冲突未保留当前游标: cursor=%+v err=%v", conflict, err)
	}
	got, next, err := syncStore.Pull(ctx, SyncCursor{Stream: "tasks.repository", Version: 0})
	if err != nil || next.Version != 1 || string(got) != string(first) {
		t.Fatalf("读取同步快照失败: payload=%q cursor=%+v err=%v", got, next, err)
	}
}
