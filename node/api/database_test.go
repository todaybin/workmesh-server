// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/todaybin/workmesh-server/node/service"
	_ "modernc.org/sqlite"
)

func TestDatabaseRedisCheckUsesRedisPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/databases/redis/check", bytes.NewBufferString(fmt.Sprintf(`{"host":"127.0.0.1","port":%d}`, port)))
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("check status=%d body=%s", res.Code, res.Body.String())
	}
	var envelope struct {
		Data struct {
			Port      int  `json:"port"`
			Available bool `json:"available"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Port != port || !envelope.Data.Available {
		t.Fatalf("redis check should honor explicit port and report available: %#v", envelope.Data)
	}
}

func TestDatabaseGenericOperationDoesNotClaimSuccessWhenUnavailable(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/databases/redis/password", bytes.NewBufferString(`{"host":"127.0.0.1","port":1}`))
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unavailable status, got=%d body=%s", res.Code, res.Body.String())
	}
}

func TestDatabaseCreateAndSearch(t *testing.T) {
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
	defer service.SetSharedDatabase(nil)
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/databases/db", bytes.NewBufferString(`{"name":"local","type":"sqlite","host":"127.0.0.1","port":1}`))
	mux.ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatalf("create status=%d body=%s", res.Code, res.Body.String())
	}
	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v2/databases/db/search", bytes.NewBufferString(`{"type":"sqlite"}`))
	mux.ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatalf("search status=%d", res.Code)
	}
}
