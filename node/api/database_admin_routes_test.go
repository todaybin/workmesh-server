// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/todaybin/workmesh-server/node/service"
)

func TestDatabaseAdminUserGrantVariablePersistence(t *testing.T) {
	dir := t.TempDir()
	old := os.Getenv("WORKMESH_DATA_DIR")
	_ = os.Setenv("WORKMESH_DATA_DIR", dir)
	defer os.Setenv("WORKMESH_DATA_DIR", old)
	databaseAdmin = service.NewDatabaseAdminStore()
	mux := http.NewServeMux()
	RegisterDatabaseAdminRoutes(mux)
	post := func(path string, body any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	if w := post("/api/v2/databases/users", map[string]any{"database": "app", "username": "alice", "host": "localhost", "password": "secret"}); w.Code != 200 {
		t.Fatalf("create user status=%d", w.Code)
	}
	var search struct {
		Code int `json:"code"`
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	w := post("/api/v2/databases/users/search", map[string]any{"database": "app"})
	_ = json.Unmarshal(w.Body.Bytes(), &search)
	if search.Code != 200 || len(search.Data.Items) != 1 || search.Data.Items[0]["passwordSet"] != true {
		t.Fatalf("unexpected users: %s", w.Body.String())
	}
	if w = post("/api/v2/databases/grants", map[string]any{"database": "app", "username": "alice", "privileges": []string{"SELECT", "UPDATE"}}); w.Code != 200 {
		t.Fatalf("grant status=%d", w.Code)
	}
	if w = post("/api/v2/databases/variables/update", map[string]any{"database": "app", "name": "max_connections", "value": "100"}); w.Code != 200 {
		t.Fatalf("variable status=%d", w.Code)
	}
	if w = post("/api/v2/databases/common/update/conf", map[string]any{"database": "app", "content": "max_connections=100"}); w.Code != 200 {
		t.Fatalf("config status=%d", w.Code)
	}
	reloaded := service.NewDatabaseAdminStore()
	if len(reloaded.ListUsers(nil, "app", "")) != 1 || len(reloaded.ListGrants(nil, "app", "")) != 1 || len(reloaded.Variables(nil, "app")) != 1 {
		t.Fatal("database admin state was not persisted")
	}
}
