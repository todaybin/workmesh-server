// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenConfiguresSQLiteAndCreatesSchemaIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workmesh.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.DB().Stats().MaxOpenConnections; got != maxOpenConnections {
		t.Fatalf("MaxOpenConnections = %d, want %d", got, maxOpenConnections)
	}
	assertPragma(t, store.DB(), "journal_mode", "wal")
	assertPragma(t, store.DB(), "synchronous", "1")
	assertPragma(t, store.DB(), "foreign_keys", "1")
	assertPragma(t, store.DB(), "busy_timeout", "5000")
	assertTables(t, store.DB(), "schema_migrations", "migration_runs", "legacy_imports")
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("重复打开应保持 schema 幂等: %v", err)
	}
	defer reopened.Close()
	var count int
	if err := reopened.DB().QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE id = ?", "0001-migration-metadata").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("迁移记录数量 = %d, want 1", count)
	}
	var runs int
	if err := reopened.DB().QueryRow("SELECT COUNT(*) FROM migration_runs WHERE status IN ('applied','noop')").Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs < 2 {
		t.Fatalf("迁移审计记录数量 = %d, want at least 2", runs)
	}
	var artifact string
	if err := reopened.DB().QueryRow("SELECT artifact_sha256 FROM migration_runs ORDER BY id DESC LIMIT 1").Scan(&artifact); err != nil {
		t.Fatal(err)
	}
	if len(artifact) != 64 {
		t.Fatalf("迁移审计制品 SHA-256 长度 = %d, want 64", len(artifact))
	}
}

func TestApplyMigrationsRejectsChecksumConflict(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := SQLMigration("test-checksum", "CREATE TABLE checksum_test (id INTEGER PRIMARY KEY)")
	if err := ApplyMigrations(context.Background(), store.DB(), []Migration{first}); err != nil {
		t.Fatal(err)
	}
	changed := SQLMigration("test-checksum", "CREATE TABLE checksum_test_changed (id INTEGER PRIMARY KEY)")
	err = ApplyMigrations(context.Background(), store.DB(), []Migration{changed})
	if !errors.Is(err, ErrMigrationChecksumMismatch) {
		t.Fatalf("checksum 冲突错误 = %v", err)
	}
}

func assertPragma(t *testing.T, db *sql.DB, name, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow("PRAGMA " + name).Scan(&got); err != nil {
		t.Fatalf("读取 PRAGMA %s 失败: %v", name, err)
	}
	if got != want {
		t.Fatalf("PRAGMA %s = %s, want %s", name, got, want)
	}
}

func assertTables(t *testing.T, db *sql.DB, names ...string) {
	t.Helper()
	for _, name := range names {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("表 %s 未创建", name)
		}
	}
}
