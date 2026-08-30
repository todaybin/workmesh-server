// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDatabaseRepositoryLifecycle(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
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

func TestDatabaseRepositoryPersistsAndUpdates(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	repo := NewDatabaseRepository()
	created, err := repo.Create(context.Background(), Database{Name: "local", Type: "postgres", Host: "localhost", Port: 5432})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repo.Update(context.Background(), Database{ID: created.ID, Name: "primary", Type: "postgres", Host: "db", Port: 5432})
	if err != nil || updated.CreatedAt != created.CreatedAt {
		t.Fatalf("update: %#v %v", updated, err)
	}
	path := filepath.Join(os.Getenv("WORKMESH_DATA_DIR"), "databases.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database state not persisted: %v", err)
	}
	reloaded := NewDatabaseRepository()
	items := reloaded.List(context.Background(), "postgres", "primary")
	if len(items) != 1 || items[0].Host != "db" {
		t.Fatalf("reloaded=%#v", items)
	}
}
