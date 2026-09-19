// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"context"
	"database/sql"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
	_ "modernc.org/sqlite"
)

func TestUnifiedSchemaMigrationsKeepDatabaseOrderAndRestartIdempotence(t *testing.T) {
	migrations := unifiedSchemaMigrations()
	wantIDs := []string{
		"0002-legacy-payloads",
		"0004-node-terminal-control-plane",
		"0005-database-resources",
		"0006-database-admin-metadata",
		"0006-website-relational",
		"0007-database-container-name",
		"0008-database-backup-metadata",
		"0009-database-runtime-states",
		"0014-website-default-html",
		"0015-website-template-relational",
		"0016-gateway-machine-identity",
	}
	if len(migrations) != len(wantIDs) {
		t.Fatalf("migration count = %d, want %d", len(migrations), len(wantIDs))
	}
	for index, migration := range migrations {
		if migration.ID != wantIDs[index] {
			t.Fatalf("migration[%d] = %q, want %q", index, migration.ID, wantIDs[index])
		}
		if migration.Checksum == "" {
			t.Fatalf("migration[%d] %q has empty checksum", index, migration.ID)
		}
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	if err := storage.ApplyMigrations(ctx, db, migrations); err != nil {
		t.Fatalf("first unified migration: %v", err)
	}
	if err := storage.ApplyMigrations(ctx, db, unifiedSchemaMigrations()); err != nil {
		t.Fatalf("second unified migration must be idempotent: %v", err)
	}

	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != len(wantIDs) {
		t.Fatalf("schema migration rows = %d, want %d", applied, len(wantIDs))
	}
	for _, id := range []string{"0008-database-backup-metadata", "0009-database-runtime-states"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE id=?`, id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("migration %s rows = %d, want 1", id, count)
		}
	}
}
