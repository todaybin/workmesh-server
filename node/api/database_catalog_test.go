// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/node/service"
	_ "modernc.org/sqlite"
)

func TestDatabaseListIncludesInstalledServersAndHidesInnerDatabases(t *testing.T) {
	db := openDatabaseCatalogDB(t)
	useDatabaseCatalogDB(t, db)
	if _, err := databaseService.Create(context.Background(), service.Database{Name: "pg-main", Type: "postgresql", From: "local", Host: "127.0.0.1", Port: 5432, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	if _, err := databaseService.Create(context.Background(), service.Database{Name: "appdb", Type: "postgresql", From: "local", Host: "127.0.0.1", Port: 5432, InitialDB: "pg-main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := databaseService.Create(context.Background(), service.Database{Name: "redis-main", Type: "redis-cluster", From: "local", Host: "127.0.0.1", Port: 6379}); err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(map[string]any{"params": map[string]any{"PANEL_DB_ROOT_PASSWORD": "secret", "PANEL_APP_PORT_HTTP": 5433}})
	if _, err := db.Exec(`INSERT INTO app_installs(id,app_key,name,version,status,container_names,config_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, "install-pg", "postgresql", "installed-pg", "16.4", "Running", "pg-container", config, "2026-09-22T00:00:00Z", "2026-09-22T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app_installs(id,app_key,name,version,status,container_names,config_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, "install-redis", "redis", "installed-redis", "7", "Stopped", "redis-container", `{}`, "2026-09-22T00:00:00Z", "2026-09-22T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app_installs(id,app_key,name,version,status,container_names,config_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, "install-maria", "mariadb", "installed-maria", "11", "Running", "maria-container", `{}`, "2026-09-22T00:00:00Z", "2026-09-22T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app_installs(id,app_key,name,version,status,container_names,config_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, "install-mongo", "mongodb", "installed-mongo", "7", "Running", "mongo-container", `{}`, "2026-09-22T00:00:00Z", "2026-09-22T00:00:00Z"); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	registerDatabaseRoutes(mux)
	postgres := serveDatabaseCatalog(t, mux, http.MethodGet, "/api/v2/databases/db/list/postgresql,postgresql-cluster", "")
	names := databaseOptionNames(t, postgres)
	if !containsString(names, "pg-main") || !containsString(names, "installed-pg") || containsString(names, "appdb") {
		t.Fatalf("postgresql options = %#v", names)
	}
	redis := serveDatabaseCatalog(t, mux, http.MethodGet, "/api/v2/databases/db/list/redis,redis-cluster", "")
	redisNames := databaseOptionNames(t, redis)
	if !containsString(redisNames, "redis-main") || !containsString(redisNames, "installed-redis") {
		t.Fatalf("redis options = %#v", redisNames)
	}
	mysql := serveDatabaseCatalog(t, mux, http.MethodGet, "/api/v2/databases/db/list/mysql,mariadb,mysql-cluster", "")
	if mysqlNames := databaseOptionNames(t, mysql); !containsString(mysqlNames, "installed-maria") {
		t.Fatalf("mysql options = %#v", mysqlNames)
	}
	mongo := serveDatabaseCatalog(t, mux, http.MethodGet, "/api/v2/databases/db/list/mongodb", "")
	if mongoNames := databaseOptionNames(t, mongo); !containsString(mongoNames, "installed-mongo") {
		t.Fatalf("mongodb options = %#v", mongoNames)
	}

	search := serveDatabaseCatalog(t, mux, http.MethodPost, "/api/v2/databases/pg/search", `{"database":"pg-main","page":1,"pageSize":20}`)
	var searched struct {
		Data struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(search.Body.Bytes(), &searched); err != nil {
		t.Fatal(err)
	}
	if searched.Data.Total != 1 || len(searched.Data.Items) != 1 || searched.Data.Items[0].Name != "appdb" {
		t.Fatalf("postgres inner databases = %s", search.Body.String())
	}
	installed, ok := databaseService.FindByName(context.Background(), "postgresql", "installed-pg")
	if !ok || installed.From != "local" || installed.Port != 5433 || installed.ContainerName != "pg-container" || installed.Username != "postgres" {
		t.Fatalf("installed postgres = %+v ok=%v", installed, ok)
	}
}

func TestMySQLSearchListsOnlyInnerDatabases(t *testing.T) {
	db := openDatabaseCatalogDB(t)
	useDatabaseCatalogDB(t, db)
	if _, err := databaseService.Create(context.Background(), service.Database{Name: "maria-main", Type: "mariadb", From: "local", Host: "127.0.0.1", Port: 3306}); err != nil {
		t.Fatal(err)
	}
	if _, err := databaseService.Create(context.Background(), service.Database{Name: "shop", Type: "mariadb", From: "local", Host: "127.0.0.1", Port: 3306, InitialDB: "maria-main", Username: "shop"}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerDatabaseRoutes(mux)
	response := serveDatabaseCatalog(t, mux, http.MethodPost, "/api/v2/databases/search", `{"database":"maria-main","info":"sh","page":1,"pageSize":10}`)
	var payload struct {
		Data struct {
			Items []struct {
				Name      string `json:"name"`
				MysqlName string `json:"mysqlName"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Items) != 1 || payload.Data.Items[0].Name != "shop" || payload.Data.Items[0].MysqlName != "maria-main" {
		t.Fatalf("mysql inner databases = %s", response.Body.String())
	}
}

func useDatabaseCatalogDB(t *testing.T, db *sql.DB) {
	t.Helper()
	controlStoreMu.Lock()
	previous := controlStoreDB
	controlStoreDB = db
	controlStoreMu.Unlock()
	service.SetSharedDatabase(db)
	t.Cleanup(func() {
		controlStoreMu.Lock()
		controlStoreDB = previous
		controlStoreMu.Unlock()
		service.SetSharedDatabase(nil)
	})
}

func TestDatabaseOperationsUseInstalledInstance(t *testing.T) {
	db := openDatabaseCatalogDB(t)
	useDatabaseCatalogDB(t, db)
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	confDir := filepath.Join(root, "apps", "postgresql", "installed-pg", "data")
	if err := os.MkdirAll(confDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "postgresql.conf"), []byte("max_connections = 100\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := databaseService.Create(context.Background(), service.Database{
		Name: "installed-pg", Type: "postgresql", From: "local", Host: "127.0.0.1", Port: 5433,
		ContainerName: "pg-container", Username: "postgres", Password: "secret",
	}); err != nil {
		t.Fatal(err)
	}
	store := &appStore{state: appStoreState{Apps: []appRecord{{
		ID: "pg-1", Key: "postgresql", Name: "installed-pg", Version: "16", Status: "Running", ContainerName: "pg-container",
	}}}}
	checked := httptest.NewRecorder()
	handleAppInstalledCheck(checked, store, map[string]any{"key": "postgresql", "name": "installed-pg"})
	if checked.Code != http.StatusOK || !strings.Contains(checked.Body.String(), `"isExist":true`) || !strings.Contains(checked.Body.String(), "pg-1") {
		t.Fatalf("check: %d %s", checked.Code, checked.Body.String())
	}
	info := findDatabaseServerConnection(context.Background(), store, "postgresql", "installed-pg")
	if info["password"] != "secret" || info["containerName"] != "pg-container" || info["port"] != 5433 {
		t.Fatalf("conninfo = %#v", info)
	}
	mux := http.NewServeMux()
	RegisterDatabaseAdminRoutes(mux)
	base := serveDatabaseCatalog(t, mux, http.MethodPost, "/api/v2/databases/common/info", `{"type":"postgresql","name":"installed-pg"}`)
	if !strings.Contains(base.Body.String(), `"containerName":"pg-container"`) || !strings.Contains(base.Body.String(), `"port":5433`) {
		t.Fatalf("base info = %s", base.Body.String())
	}
	file := serveDatabaseCatalog(t, mux, http.MethodPost, "/api/v2/databases/common/load/file", `{"type":"postgresql-conf","name":"installed-pg"}`)
	if !strings.Contains(file.Body.String(), "max_connections = 100") {
		t.Fatalf("conf = %s", file.Body.String())
	}
}

func TestDatabaseDiscoveryHelpers(t *testing.T) {
	if databaseTypeFromImage("redis:8-alpine") != "redis" || databaseTypeFromImage("postgres:18.6-alpine") != "postgresql" || databaseTypeFromImage("alpine/socat:latest") != "" {
		t.Fatal("image classification mismatch")
	}
	if hostPortFromDockerPorts("127.0.0.1:6379->6379/tcp") != 6379 {
		t.Fatal("port parse mismatch")
	}
	rows := []service.Database{
		{ID: 1, Name: "postgresql-sp", Type: "postgresql", InitialDB: ""},
		{ID: 2, Name: "sp_sopvip_com", Type: "postgresql", InitialDB: "sp-postgres"},
	}
	visible := visibleChildDatabases(rows, "postgresql-sp", "", []string{"sp_sopvip_com", "sp_admin"})
	names := map[string]bool{}
	for _, item := range visible {
		names[item.Name] = true
	}
	if !names["sp_sopvip_com"] || !names["sp_admin"] || names["postgresql-sp"] {
		t.Fatalf("visible = %#v", visible)
	}
	chosen, ok := matchDatabaseInstall([]appRecord{
		{ID: "postgresql", Key: "postgresql", Name: "postgresql", Status: "Error", ContainerName: "WorkMesh-postgresql-dd548e"},
		{ID: "container:WorkMesh-postgresql-ZNMP", Key: "postgresql", Name: "postgresql", Status: "Running", ContainerName: "WorkMesh-postgresql-ZNMP"},
	}, "postgresql", "postgresql")
	if !ok || chosen.ContainerName != "WorkMesh-postgresql-ZNMP" {
		t.Fatalf("running install not preferred: %+v ok=%v", chosen, ok)
	}
}

type routedPostgresExecutor struct{}

func (routedPostgresExecutor) Exec(_ context.Context, target service.PostgresTarget, _ string) (string, error) {
	if target.ContainerName == "WorkMesh-postgresql-ZNMP" {
		return "znmp_sopvip_com\n", nil
	}
	if target.ContainerName == "WorkMesh-postgresql-SP" {
		return "sp_sopvip_com\n", nil
	}
	return "", nil
}

func TestDatabaseItemListSyncsBothPostgresInstances(t *testing.T) {
	db := openDatabaseCatalogDB(t)
	useDatabaseCatalogDB(t, db)
	previous := postgresExecutor
	postgresExecutor = routedPostgresExecutor{}
	t.Cleanup(func() { postgresExecutor = previous })
	for _, item := range []service.Database{
		{Name: "postgresql", Type: "postgresql", From: "local", Host: "127.0.0.1", Port: 5432, ContainerName: "WorkMesh-postgresql-ZNMP", Username: "workmesh"},
		{Name: "postgresql-sp", Type: "postgresql", From: "local", Host: "127.0.0.1", Port: 55432, ContainerName: "WorkMesh-postgresql-SP", Username: "sp_admin"},
	} {
		if _, err := databaseService.Create(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	registerDatabaseRoutes(mux)
	response := serveDatabaseCatalog(t, mux, http.MethodGet, "/api/v2/databases/db/item/postgresql", "")
	body := response.Body.String()
	if !strings.Contains(body, "znmp_sopvip_com") || !strings.Contains(body, "sp_sopvip_com") {
		t.Fatalf("database items = %s", body)
	}
}

func TestRemoteDatabaseSearchAndTerminal(t *testing.T) {
	db := openDatabaseCatalogDB(t)
	useDatabaseCatalogDB(t, db)
	if _, err := databaseService.Create(context.Background(), service.Database{Name: "local-pg", Type: "postgresql", From: "local", Host: "127.0.0.1", Port: 5432, ContainerName: "pg-container", Username: "postgres", Password: "local-secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := databaseService.Create(context.Background(), service.Database{Name: "appdb", Type: "postgresql", From: "local", Host: "127.0.0.1", Port: 5432, InitialDB: "local-pg"}); err != nil {
		t.Fatal(err)
	}
	if _, err := databaseService.Create(context.Background(), service.Database{Name: "cloud-pg", Type: "postgresql", From: "remote", Host: "10.0.0.8", Port: 5432, Username: "postgres", Password: "remote-secret", InitialDB: "postgres"}); err != nil {
		t.Fatal(err)
	}
	if _, err := databaseService.Create(context.Background(), service.Database{Name: "cloud-redis", Type: "redis", From: "remote", Host: "10.0.0.9", Port: 6379, Password: "redis-secret"}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerDatabaseRoutes(mux)
	listed := serveDatabaseCatalog(t, mux, http.MethodPost, "/api/v2/databases/db/search", `{"type":"postgresql","info":"cloud","page":1,"pageSize":20}`)
	if !strings.Contains(listed.Body.String(), `"address":"10.0.0.8"`) || !strings.Contains(listed.Body.String(), "remote-secret") || strings.Contains(listed.Body.String(), "local-pg") || strings.Contains(listed.Body.String(), "appdb") {
		t.Fatalf("remote search = %s", listed.Body.String())
	}
	got := serveDatabaseCatalog(t, mux, http.MethodGet, "/api/v2/databases/db/cloud-pg", "")
	if !strings.Contains(got.Body.String(), `"address":"10.0.0.8"`) || !strings.Contains(got.Body.String(), "remote-secret") {
		t.Fatalf("remote detail = %s", got.Body.String())
	}
	command, err := databaseTerminalCommand(context.Background(), httptest.NewRequest(http.MethodGet, "/api/v2/hosts/terminal/container?source=database&databaseType=postgresql&database=local-pg", nil))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(command.Args, " ")
	if !strings.Contains(joined, "pg-container") || !strings.Contains(joined, "psql") {
		t.Fatalf("local terminal = %s", joined)
	}
	redisCommand, err := redisTerminalCommand(context.Background(), httptest.NewRequest(http.MethodGet, "/api/v2/hosts/terminal/container?source=redis&name=cloud-redis&from=remote", nil))
	if err != nil {
		t.Fatal(err)
	}
	if redisCommand.Args[0] != "sh" || !strings.Contains(strings.Join(redisCommand.Args, " "), "redis-cli --raw -h 10.0.0.9 -p 6379") {
		t.Fatalf("remote redis terminal = %#v", redisCommand.Args)
	}
}

func openDatabaseCatalogDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	statements := []string{
		`CREATE TABLE databases (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'local', app_install_id INTEGER NOT NULL DEFAULT 0, container_name TEXT NOT NULL DEFAULT '', address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 0, initial_db TEXT NOT NULL DEFAULT '', username TEXT NOT NULL DEFAULT '', password TEXT NOT NULL DEFAULT '', ssl INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE UNIQUE INDEX idx_databases_type_name ON databases(type, name)`,
		`CREATE TABLE app_installs (id TEXT PRIMARY KEY, app_key TEXT NOT NULL, name TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, install_path TEXT NOT NULL DEFAULT '', compose_path TEXT NOT NULL DEFAULT '', compose_project TEXT NOT NULL DEFAULT '', container_names TEXT NOT NULL DEFAULT '', config_json BLOB NOT NULL DEFAULT '{}', message TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func serveDatabaseCatalog(t *testing.T, mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("%s %s status=%d body=%s", method, path, response.Code, response.Body.String())
	}
	return response
}

func databaseOptionNames(t *testing.T, response *httptest.ResponseRecorder) []string {
	t.Helper()
	var payload struct {
		Data []struct {
			Database string `json:"database"`
			From     string `json:"from"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode list: %v body=%s", err, response.Body.String())
	}
	names := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		if item.From == "" || item.Database == "" {
			t.Fatalf("incomplete option %#v", item)
		}
		names = append(names, item.Database)
	}
	return names
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
