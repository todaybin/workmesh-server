// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"database/sql"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
	_ "modernc.org/sqlite"
)

func TestDatabaseRepositoryLifecycle(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createDatabaseTable(t, db)
	SetSharedDatabase(db)
	defer SetSharedDatabase(nil)
	svc := NewDatabaseService(nil)
	item, err := svc.Create(context.Background(), Database{Name: "local", Type: "mysql", Host: "127.0.0.1", Port: 3306})
	if err != nil || item.ID == 0 {
		t.Fatalf("create: %#v %v", item, err)
	}
	if got := len(svc.Search(context.Background(), "mysql", "loc")); got != 1 {
		t.Fatalf("search=%d", got)
	}
	if err := svc.Delete(context.Background(), item.ID); err != nil {
		t.Fatal(err)
	}
}

func createDatabaseTable(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`CREATE TABLE databases (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'local', app_install_id INTEGER NOT NULL DEFAULT 0, container_name TEXT NOT NULL DEFAULT '', address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 0, initial_db TEXT NOT NULL DEFAULT '', username TEXT NOT NULL DEFAULT '', password TEXT NOT NULL DEFAULT '', ssl INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseRepositoryPersistsAndUpdates(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createDatabaseTable(t, db)
	SetSharedDatabase(db)
	defer SetSharedDatabase(nil)
	repo := NewDatabaseRepository()
	created, err := repo.Create(context.Background(), Database{Name: "local", Type: "postgres", ContainerName: "workmesh-panel-postgres", Host: "localhost", Port: 5432, Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repo.Update(context.Background(), Database{ID: created.ID, Name: "primary", Type: "postgres", ContainerName: "workmesh-panel-postgres-v2", Host: "db", Port: 5432})
	if err != nil || updated.CreatedAt != created.CreatedAt {
		t.Fatalf("update: %#v %v", updated, err)
	}
	reloaded := NewDatabaseRepository()
	items := reloaded.List(context.Background(), "postgres", "primary")
	if len(items) != 1 || items[0].Host != "db" || items[0].ContainerName != "workmesh-panel-postgres-v2" {
		t.Fatalf("reloaded=%#v", items)
	}
	found := reloaded.findSQL(context.Background(), created.ID)
	if found.Password != "secret" || found.ContainerName != "workmesh-panel-postgres-v2" {
		t.Fatalf("reloaded=%#v", found)
	}
}

func TestDatabaseRepositoryKeepsLegacySchemaUsable(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createLegacyDatabaseTable(t, db)
	SetSharedDatabase(db)
	defer SetSharedDatabase(nil)

	repo := NewDatabaseRepository()
	created, err := repo.Create(context.Background(), Database{
		Name:          "legacy",
		Type:          "mysql",
		ContainerName: "workmesh-panel-mariadb",
		Host:          "127.0.0.1",
		Port:          3306,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ContainerName != "workmesh-panel-mariadb" {
		t.Fatalf("created=%#v", created)
	}
	found := repo.findSQL(context.Background(), created.ID)
	if found.ID == 0 || found.ContainerName != "" {
		t.Fatalf("legacy record=%#v", found)
	}
	if items := repo.List(context.Background(), "mysql", "legacy"); len(items) != 1 || items[0].ContainerName != "" {
		t.Fatalf("legacy list=%#v", items)
	}
}

func createLegacyDatabaseTable(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`CREATE TABLE databases (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'local', app_install_id INTEGER NOT NULL DEFAULT 0, address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 0, initial_db TEXT NOT NULL DEFAULT '', username TEXT NOT NULL DEFAULT '', password TEXT NOT NULL DEFAULT '', ssl INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseContainerNameMigrationIsIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createLegacyDatabaseTable(t, db)

	migration := DatabaseContainerNameMigration()
	if err := storage.ApplyMigrations(context.Background(), db, []storage.Migration{migration}); err != nil {
		t.Fatalf("first migration: %v", err)
	}
	if err := storage.ApplyMigrations(context.Background(), db, []storage.Migration{migration}); err != nil {
		t.Fatalf("second migration: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('databases') WHERE name='container_name'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("container_name columns=%d", count)
	}
}
