// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestSQLiteDatabaseRuntimeStateStoreRoundTrip(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := SQLiteDatabaseRuntimeStateStore{DB: db}
	observedAt := time.Date(2026, 9, 10, 1, 2, 3, 0, time.UTC)
	want := []DatabaseRuntimeState{{
		ContainerName: "workmesh-panel-postgres",
		Status:        "running",
		Health:        "healthy",
		Image:         "postgres:18-alpine",
		Ports:         []DatabaseRuntimePort{{ContainerPort: 5432, Protocol: "tcp"}},
		ObservedAt:    observedAt,
	}}
	if err := store.UpsertDatabaseRuntimeStates(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err := store.ListDatabaseRuntimeStates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ContainerName != want[0].ContainerName || got[0].Image != want[0].Image {
		t.Fatalf("got=%#v want=%#v", got, want)
	}
	if !reflect.DeepEqual(got[0].Ports, want[0].Ports) || !got[0].ObservedAt.Equal(observedAt) {
		t.Fatalf("state fields got=%#v want=%#v", got[0], want[0])
	}
	want[0].Status = "exited"
	want[0].Error = "failed"
	if err := store.UpsertDatabaseRuntimeStates(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err = store.ListDatabaseRuntimeStates(context.Background())
	if err != nil || len(got) != 1 || got[0].Status != "exited" || got[0].Error != "failed" {
		t.Fatalf("upsert got=%#v err=%v", got, err)
	}
}
