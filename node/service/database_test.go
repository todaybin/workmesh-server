// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"testing"
)

func TestDatabaseRepositoryLifecycle(t *testing.T) {
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
