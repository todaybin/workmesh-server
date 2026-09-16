// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/node/service"
	_ "modernc.org/sqlite"
)

type mockRedisExecutor struct {
	calls   [][]string
	setCall int
}

func (m *mockRedisExecutor) Exec(_ context.Context, _ service.RedisTarget, args ...string) (string, error) {
	m.calls = append(m.calls, append([]string(nil), args...))
	if len(args) >= 3 && args[0] == "CONFIG" && args[1] == "GET" {
		return args[2] + "\nold-" + args[2], nil
	}
	if len(args) >= 3 && args[0] == "CONFIG" && args[1] == "SET" {
		m.setCall++
		if m.setCall == 2 {
			return "", errors.New("injected redis failure")
		}
	}
	return "OK", nil
}

func TestDatabaseRedisCheckUsesRedisPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("当前沙箱禁止 loopback 监听: %v", err)
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

func TestPostgresAndRedisRoutesRejectUnregisteredTargets(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	for _, tc := range []struct {
		path string
		body string
	}{
		{path: "/api/v2/databases/pg", body: `{"name":"app","database":"pg-main","username":"u","password":"c2VjcmV0"}`},
		{path: "/api/v2/databases/redis/status", body: `{"name":"redis-main","type":"redis"}`},
		{path: "/api/v2/databases/redis/conf", body: `{"name":"redis-main","type":"redis"}`},
	} {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewBufferString(tc.body))
		mux.ServeHTTP(res, req)
		if res.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status=%d body=%s", tc.path, res.Code, res.Body.String())
		}
		if !strings.Contains(res.Body.String(), `"code":"ERR"`) {
			t.Fatalf("%s missing ERR envelope: %s", tc.path, res.Body.String())
		}
	}
}

func TestRedisConfigUpdateRollsBackAppliedValues(t *testing.T) {
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
	if _, err = databaseService.Create(context.Background(), service.Database{Name: "redis-test", Type: "redis", From: "remote", Host: "127.0.0.1", Port: 6379}); err != nil {
		t.Fatal(err)
	}
	mock := &mockRedisExecutor{}
	previous := redisExecutor
	redisExecutor = mock
	defer func() { redisExecutor = previous }()

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/databases/redis/conf/update", bytes.NewBufferString(`{"database":"redis-test","timeout":"60","maxclients":"512"}`))
	handleRedisConfigUpdate(res, req)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	foundRollback := false
	for _, call := range mock.calls {
		if len(call) == 4 && call[0] == "CONFIG" && call[1] == "SET" && call[2] == "timeout" && call[3] == "old-timeout" {
			foundRollback = true
		}
	}
	if !foundRollback {
		t.Fatalf("missing redis rollback: %#v", mock.calls)
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

func TestDatabaseMySQLContractRoutesUseSQLiteMetadata(t *testing.T) {
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
	registerDatabaseRoutes(mux)
	post := func(path, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		mux.ServeHTTP(res, req)
		return res
	}
	created := post("/api/v2/databases", `{"name":"mysql-local","address":"127.0.0.1","port":3306,"username":"root"}`)
	if created.Code != http.StatusOK {
		t.Fatalf("mysql create status=%d body=%s", created.Code, created.Body.String())
	}
	var envelope struct {
		Data service.Database `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil || envelope.Data.ID == 0 || envelope.Data.Type != "mysql" {
		t.Fatalf("mysql create response=%s", created.Body.String())
	}
	check := post("/api/v2/databases/db/del/check", fmt.Sprintf(`{"id":%d}`, envelope.Data.ID))
	if check.Code != http.StatusOK || !strings.Contains(check.Body.String(), `"data":[]`) {
		t.Fatalf("delete check status=%d body=%s", check.Code, check.Body.String())
	}
}

func TestDatabaseTargetsPreferContainerNameAndKeepHostSeparate(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE databases (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'local', app_install_id INTEGER NOT NULL DEFAULT 0, container_name TEXT NOT NULL DEFAULT '', address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 0, initial_db TEXT NOT NULL DEFAULT '', username TEXT NOT NULL DEFAULT '', password TEXT NOT NULL DEFAULT '', ssl INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	service.SetSharedDatabase(db)
	t.Cleanup(func() { service.SetSharedDatabase(nil) })

	created, err := databaseService.Create(context.Background(), service.Database{
		Name: "panel-postgres", Type: "postgresql", From: "local",
		Host: "127.0.0.1", ContainerName: "workmesh-panel-postgres",
		Port: 5432, Username: "postgres", Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	pg, ok := postgresTargetForRequest(context.Background(), created.Name)
	if !ok {
		t.Fatal("postgres target not found")
	}
	if pg.Host != "127.0.0.1" || pg.ContainerName != "workmesh-panel-postgres" {
		t.Fatalf("postgres target=%#v", pg)
	}
	mysql, ok := mysqlTargetForRequest(context.Background(), created.Name, "postgresql")
	if ok || mysql.ContainerName != "" {
		t.Fatalf("mysql target should reject a PostgreSQL resource: %#v", mysql)
	}

	redisItem, err := databaseService.Create(context.Background(), service.Database{
		Name: "panel-redis", Type: "redis", From: "local",
		Host: "127.0.0.1", ContainerName: "workmesh-panel-redis",
		Port: 6379, Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	redis, _, ok := redisTargetForRequest(context.Background(), redisItem.Name)
	if !ok || redis.Host != "127.0.0.1" || redis.ContainerName != "workmesh-panel-redis" {
		t.Fatalf("redis target=%#v ok=%v", redis, ok)
	}

	if _, err := db.Exec(`INSERT INTO databases(name,type,source,address,port,username,password,created_at,updated_at) VALUES('legacy-mysql','mysql','local','legacy-mariadb',3306,'root','secret','2026-09-10T00:00:00Z','2026-09-10T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	legacy, ok := mysqlTargetForRequest(context.Background(), "legacy-mysql", "mysql")
	if !ok || legacy.Host != "127.0.0.1" || legacy.ContainerName != "legacy-mariadb" {
		t.Fatalf("legacy mysql target=%#v ok=%v", legacy, ok)
	}
	if _, err := db.Exec(`INSERT INTO databases(name,type,source,address,port,username,password,created_at,updated_at) VALUES('remote-mysql','mysql','remote','db.example.internal',3306,'root','secret','2026-09-10T00:00:00Z','2026-09-10T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	remote, ok := mysqlTargetForRequest(context.Background(), "remote-mysql", "mysql")
	if !ok || remote.Host != "db.example.internal" || remote.ContainerName != "" {
		t.Fatalf("remote mysql target=%#v ok=%v", remote, ok)
	}
}

func TestDatabaseGenericCreatePersistsSeparatedContainerTarget(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE databases (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'local', app_install_id INTEGER NOT NULL DEFAULT 0, container_name TEXT NOT NULL DEFAULT '', address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 0, initial_db TEXT NOT NULL DEFAULT '', username TEXT NOT NULL DEFAULT '', password TEXT NOT NULL DEFAULT '', ssl INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	service.SetSharedDatabase(db)
	t.Cleanup(func() { service.SetSharedDatabase(nil) })

	mux := http.NewServeMux()
	registerDatabaseRoutes(mux)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/databases/db", bytes.NewBufferString(`{"name":"containerized-mysql","type":"mysql","from":"local","host":"127.0.0.1","containerName":"workmesh-panel-mariadb","port":3306}`))
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", res.Code, res.Body.String())
	}
	var envelope struct {
		Data service.Database `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Host != "127.0.0.1" || envelope.Data.ContainerName != "workmesh-panel-mariadb" || envelope.Data.From != "local" {
		t.Fatalf("persisted item=%#v", envelope.Data)
	}
}

func TestControlStoreDatabaseSchemaUpgradeIsIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE databases (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'local', app_install_id INTEGER NOT NULL DEFAULT 0, address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 0, initial_db TEXT NOT NULL DEFAULT '', username TEXT NOT NULL DEFAULT '', password TEXT NOT NULL DEFAULT '', ssl INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureControlTables(db); err != nil {
		t.Fatalf("first control-store bootstrap: %v", err)
	}
	if err := ensureControlTables(db); err != nil {
		t.Fatalf("second control-store bootstrap: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('databases') WHERE name='container_name'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("container_name columns=%d", count)
	}
}
