// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
	_ "modernc.org/sqlite"
)

func TestDatabaseBackupAndRuntimeMigrationsAreStableAndIdempotent(t *testing.T) {
	backup := DatabaseBackupSchemaMigration()
	runtime := DatabaseRuntimeStateSchemaMigration()
	if backup.ID != "0008-database-backup-metadata" {
		t.Fatalf("backup migration ID = %q", backup.ID)
	}
	if runtime.ID != "0009-database-runtime-states" {
		t.Fatalf("runtime migration ID = %q", runtime.ID)
	}
	if backup.Checksum != "3e5c02e412f0d0c79460711a823d739975f55fe687c684dfee6b510675490afc" {
		t.Fatalf("backup migration checksum = %q", backup.Checksum)
	}
	if runtime.Checksum != "2ffaead97666329560f128b8803757c6602dd3c42497aa2a8ed214e216f457fd" {
		t.Fatalf("runtime migration checksum = %q", runtime.Checksum)
	}
	if backup.Checksum != DatabaseBackupSchemaMigration().Checksum ||
		runtime.Checksum != DatabaseRuntimeStateSchemaMigration().Checksum {
		t.Fatal("migration checksum is not stable across construction")
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	migrations := []storage.Migration{backup, runtime}
	ctx := context.Background()
	if err := storage.ApplyMigrations(ctx, db, migrations); err != nil {
		t.Fatalf("first migration: %v", err)
	}
	if err := storage.ApplyMigrations(ctx, db, migrations); err != nil {
		t.Fatalf("second migration must be idempotent: %v", err)
	}

	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE id IN (?,?)`, backup.ID, runtime.ID).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 2 {
		t.Fatalf("applied database migration count = %d, want 2", applied)
	}
	for _, table := range []string{"database_backups", "database_runtime_states"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("table %s count = %d, want 1", table, count)
		}
	}

	tampered := storage.SQLMigration(backup.ID, "CREATE TABLE migration_checksum_tamper (id INTEGER)")
	err = storage.ApplyMigrations(ctx, db, []storage.Migration{tampered})
	if !errors.Is(err, storage.ErrMigrationChecksumMismatch) {
		t.Fatalf("tampered checksum error = %v, want ErrMigrationChecksumMismatch", err)
	}
}
