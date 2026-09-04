// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"database/sql"
	"testing"

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
	_, err := db.Exec(`CREATE TABLE databases (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'local', app_install_id INTEGER NOT NULL DEFAULT 0, address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 0, initial_db TEXT NOT NULL DEFAULT '', username TEXT NOT NULL DEFAULT '', password TEXT NOT NULL DEFAULT '', ssl INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`)
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
	created, err := repo.Create(context.Background(), Database{Name: "local", Type: "postgres", Host: "localhost", Port: 5432, Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repo.Update(context.Background(), Database{ID: created.ID, Name: "primary", Type: "postgres", Host: "db", Port: 5432})
	if err != nil || updated.CreatedAt != created.CreatedAt {
		t.Fatalf("update: %#v %v", updated, err)
	}
	reloaded := NewDatabaseRepository()
	items := reloaded.List(context.Background(), "postgres", "primary")
	if len(items) != 1 || items[0].Host != "db" {
		t.Fatalf("reloaded=%#v", items)
	}
	if got := reloaded.findSQL(context.Background(), created.ID).Password; got != "secret" {
		t.Fatalf("password=%q", got)
	}
}
