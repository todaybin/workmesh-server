// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/node/service"
	_ "modernc.org/sqlite"
)

type postgresLoadMock struct {
	output string
}

func (m *postgresLoadMock) Exec(_ context.Context, _ service.PostgresTarget, _ string) (string, error) {
	return m.output, nil
}

func TestPostgresLoadRemotePersistsDiscoveredDatabases(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE databases (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'local', app_install_id INTEGER NOT NULL DEFAULT 0, address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 0, initial_db TEXT NOT NULL DEFAULT '', username TEXT NOT NULL DEFAULT '', password TEXT NOT NULL DEFAULT '', ssl INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	service.SetSharedDatabase(db)
	t.Cleanup(func() { service.SetSharedDatabase(nil) })
	if _, err = databaseService.Create(context.Background(), service.Database{Name: "pg-main", Type: "postgresql", Host: "127.0.0.1", Port: 5432, InitialDB: "postgres", Username: "postgres", Password: "secret"}); err != nil {
		t.Fatal(err)
	}
	previous := postgresExecutor
	postgresExecutor = &postgresLoadMock{output: "appdb\nmetrics\nappdb\n"}
	t.Cleanup(func() { postgresExecutor = previous })
	mux := http.NewServeMux()
	registerDatabaseRoutes(mux)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/databases/pg/pg-main/load", bytes.NewBufferString(`{"from":"remote","type":"postgresql","database":"pg-main"}`))
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"created":2`) {
		t.Fatalf("PostgreSQL 远程同步失败: %d %s", res.Code, res.Body.String())
	}
	if _, found := databaseService.FindByName(context.Background(), "postgresql", "appdb"); !found {
		t.Fatal("同步数据库未写入 SQLite")
	}
}
