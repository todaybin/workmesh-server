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

type mockMongoExecutor struct {
	statements []string
	output     string
	err        error
}

func (m *mockMongoExecutor) Exec(_ context.Context, _ service.MongoDBTarget, statement string) (string, error) {
	m.statements = append(m.statements, statement)
	return m.output, m.err
}

func TestMongoPrivilegeAndDeleteCheckUseRealExecutor(t *testing.T) {
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
	if _, err = databaseService.Create(context.Background(), service.Database{Name: "mongo-main", Type: "mongodb", Host: "127.0.0.1", Port: 27017, Username: "root", Password: "rootpw", InitialDB: "admin"}); err != nil {
		t.Fatal(err)
	}
	if _, err = databaseService.Create(context.Background(), service.Database{Name: "appdb", Type: "mongodb", Host: "127.0.0.1", Port: 27017, InitialDB: "mongo-main", Username: "alice"}); err != nil {
		t.Fatal(err)
	}
	mock := &mockMongoExecutor{output: "readWrite\n"}
	previous := mongoExecutor
	mongoExecutor = mock
	t.Cleanup(func() { mongoExecutor = previous })
	mux := http.NewServeMux()
	registerDatabaseRoutes(mux)
	call := func(path, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		mux.ServeHTTP(res, req)
		return res
	}
	privileges := call("/api/v2/databases/mongodb/privileges", `{"database":"mongo-main","name":"appdb","username":"alice"}`)
	if privileges.Code != http.StatusOK || !strings.Contains(privileges.Body.String(), `"data":"readWrite"`) || len(mock.statements) != 1 {
		t.Fatalf("MongoDB 权限读取未执行真实脚本: status=%d body=%s statements=%v", privileges.Code, privileges.Body.String(), mock.statements)
	}
	check := call("/api/v2/databases/mongodb/del/check", `{"id":2,"database":"mongo-main"}`)
	if check.Code != http.StatusOK || !strings.Contains(check.Body.String(), `"data":[]`) || len(mock.statements) != 2 {
		t.Fatalf("MongoDB 删除检查未执行 ping: status=%d body=%s statements=%v", check.Code, check.Body.String(), mock.statements)
	}
	mock.output = `["appdb","remote_only"]`
	loaded := call("/api/v2/databases/mongodb/load", `{"from":"remote","type":"mongodb","database":"mongo-main"}`)
	if loaded.Code != http.StatusOK || !strings.Contains(loaded.Body.String(), `"created":1`) {
		t.Fatalf("MongoDB 远程同步失败: status=%d body=%s statements=%v", loaded.Code, loaded.Body.String(), mock.statements)
	}
	if item, found := databaseService.FindByName(context.Background(), "mongodb", "remote_only"); !found || item.InitialDB != "mongo-main" {
		t.Fatalf("MongoDB 同步记录未持久化: found=%v item=%+v", found, item)
	}
}
