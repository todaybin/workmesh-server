// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/service"
)

func TestExternalDatabaseLifecycles(t *testing.T) {
	if os.Getenv("WORKMESH_DATABASE_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_DATABASE_EXTERNAL_TEST=1 to run Docker database acceptance")
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	mariaName := "workmesh-acceptance-mariadb-" + suffix
	pgName := "workmesh-acceptance-postgres-" + suffix
	redisName := "workmesh-acceptance-redis-" + suffix
	mongoName := "workmesh-acceptance-mongodb-" + suffix
	redisConfigDir := t.TempDir()
	if err := os.Chmod(redisConfigDir, 0o777); err != nil {
		t.Fatal(err)
	}
	redisConfig := filepath.Join(redisConfigDir, "redis.conf")
	if err := os.WriteFile(redisConfig, []byte("bind 0.0.0.0\nprotected-mode no\nappendonly no\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(redisConfig, 0o666); err != nil {
		t.Fatal(err)
	}
	for _, spec := range [][]string{
		{"run", "-d", "--rm", "--name", mariaName, "-e", "MARIADB_ROOT_PASSWORD=acceptance-root", "mariadb:11"},
		{"run", "-d", "--rm", "--name", pgName, "-e", "POSTGRES_PASSWORD=acceptance-root", "-p", "127.0.0.1::5432", "postgres:18-alpine"},
		{"run", "-d", "--rm", "--name", redisName, "-p", "127.0.0.1::6379", "-v", redisConfigDir + ":/usr/local/etc/redis", "redis:8-alpine", "redis-server", "/usr/local/etc/redis/redis.conf"},
		{"run", "-d", "--rm", "--name", mongoName, "-e", "MONGO_INITDB_ROOT_USERNAME=root", "-e", "MONGO_INITDB_ROOT_PASSWORD=acceptance-root", "mongo:8-noble"},
	} {
		if out, err := exec.Command("docker", spec...).CombinedOutput(); err != nil {
			t.Fatalf("docker %v: %v: %s", spec, err, out)
		}
	}
	t.Cleanup(func() {
		for _, name := range []string{mariaName, pgName, redisName, mongoName} {
			_ = exec.Command("docker", "rm", "-f", name).Run()
		}
	})
	waitContainerCommand(t, mariaName, "mariadb-admin", "ping", "-h", "127.0.0.1", "-uroot", "-pacceptance-root", "--silent")
	waitContainerCommand(t, pgName, "pg_isready", "-U", "postgres")
	waitContainerCommand(t, redisName, "redis-cli", "PING")
	waitContainerCommand(t, mongoName, "mongosh", "--quiet", "--username", "root", "--password", "acceptance-root", "--authenticationDatabase", "admin", "--eval", "db.adminCommand({ping:1}).ok")
	pgPort := dockerPort(t, pgName, "5432/tcp")
	redisPort := dockerPort(t, redisName, "6379/tcp")

	store, err := storage.Open(filepath.Join(t.TempDir(), "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = SetSharedStore(store); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resetSharedStoreForTest(); _ = store.Close() })
	for _, item := range []service.Database{
		{Name: "acceptance-mariadb", Type: "mariadb", From: "local", Host: mariaName, Port: 3306, Username: "root", Password: "acceptance-root"},
		{Name: "acceptance-postgres", Type: "postgresql", From: "remote", Host: "127.0.0.1", Port: pgPort, InitialDB: "postgres", Username: "postgres", Password: "acceptance-root"},
		{Name: "acceptance-redis", Type: "redis", From: "remote", Host: "127.0.0.1", Port: redisPort},
		{Name: "acceptance-mongodb", Type: "mongodb", From: "local", Host: mongoName, Port: 27017, InitialDB: "admin", Username: "root", Password: "acceptance-root"},
	} {
		if _, err = databaseService.Create(t.Context(), item); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	registerDatabaseRoutes(mux)
	RegisterDatabaseAdminRoutes(mux)
	requireMongoOutput(t, mongoName, "1", "acceptance-root", `db.getSiblingDB("wm_synced_mongo").createCollection("_init").ok`)
	postDatabaseJSON(t, mux, "/api/v2/databases/mongodb/load", map[string]any{"from": "local", "type": "mongodb", "database": "acceptance-mongodb"})
	syncedMongo, syncedFound := databaseService.FindByName(t.Context(), "mongodb", "wm_synced_mongo")
	if !syncedFound || syncedMongo.InitialDB != "acceptance-mongodb" {
		t.Fatalf("MongoDB 远程同步未写入 SQLite: found=%v item=%+v", syncedFound, syncedMongo)
	}

	maria := postDatabaseJSON(t, mux, "/api/v2/databases", map[string]any{"name": "wm_accept_mysql", "database": "acceptance-mariadb", "username": "wm_user", "password": "YWNjZXB0YW5jZS11c2Vy", "format": "utf8mb4", "collation": "utf8mb4_general_ci"})
	mariaID := responseDatabaseID(t, maria)
	requireContainerOutput(t, mariaName, "", "mariadb", "-uroot", "-pacceptance-root", "-Nse", "CREATE DATABASE wm_synced_mysql")
	postDatabaseJSON(t, mux, "/api/v2/databases/load", map[string]any{"from": "local", "type": "mariadb", "database": "acceptance-mariadb"})
	var syncedMaria service.Database
	var syncedMariaFound bool
	for _, item := range databaseService.Search(t.Context(), "mariadb", "wm_synced_mysql") {
		if strings.EqualFold(item.InitialDB, "acceptance-mariadb") {
			syncedMaria, syncedMariaFound = item, true
			break
		}
	}
	if !syncedMariaFound {
		t.Fatal("MySQL 远程同步未写入 SQLite")
	}
	postDatabaseJSON(t, mux, "/api/v2/databases/del", map[string]any{"id": syncedMaria.ID, "database": "acceptance-mariadb"})
	variables := postDatabaseJSON(t, mux, "/api/v2/databases/variables", map[string]any{"type": "mariadb", "name": "acceptance-mariadb", "database": "acceptance-mariadb"})
	if !strings.Contains(string(variables), "max_connections") {
		t.Fatalf("MySQL 变量查询未返回真实变量: %s", variables)
	}
	postDatabaseJSON(t, mux, "/api/v2/databases/variables/update", map[string]any{"type": "mariadb", "database": "acceptance-mariadb", "variables": []map[string]any{{"param": "max_connections", "value": "512"}}})
	requireContainerOutput(t, mariaName, "512", "mariadb", "-uroot", "-pacceptance-root", "-Nse", "SELECT @@GLOBAL.max_connections")
	postDatabaseJSON(t, mux, "/api/v2/databases/change/access", map[string]any{"type": "mariadb", "database": "acceptance-mariadb", "from": "local", "value": "%"})
	requireContainerOutput(t, mariaName, "%", "mariadb", "-uroot", "-pacceptance-root", "-Nse", "SELECT Host FROM mysql.user WHERE User='root' AND Host='%'")
	requireSQLiteCount(t, store.DB(), 1, `SELECT COUNT(*) FROM database_users WHERE database_name='acceptance-mariadb' AND username='wm_user'`)
	requireSQLiteCount(t, store.DB(), 1, `SELECT COUNT(*) FROM database_grants WHERE server_name='acceptance-mariadb' AND database_name='wm_accept_mysql'`)
	requireContainerOutput(t, mariaName, "wm_accept_mysql", "mariadb", "-uroot", "-pacceptance-root", "-Nse", "SELECT SCHEMA_NAME FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME='wm_accept_mysql'")
	postDatabaseJSON(t, mux, "/api/v2/databases/users/password", map[string]any{"database": "acceptance-mariadb", "username": "wm_user", "host": "%", "password": "bmV3LW1hcmlhZGItcGFzcw=="})
	postDatabaseJSON(t, mux, "/api/v2/databases/grants/del", map[string]any{"database": "acceptance-mariadb", "db": "wm_accept_mysql", "username": "wm_user", "host": "%"})
	requireSQLiteCount(t, store.DB(), 0, `SELECT COUNT(*) FROM database_grants WHERE server_name='acceptance-mariadb' AND database_name='wm_accept_mysql'`)
	postDatabaseJSON(t, mux, "/api/v2/databases/grants", map[string]any{"database": "acceptance-mariadb", "db": "wm_accept_mysql", "username": "wm_user", "host": "%"})
	postDatabaseJSON(t, mux, "/api/v2/databases/del", map[string]any{"id": mariaID, "database": "acceptance-mariadb"})
	requireContainerOutput(t, mariaName, "", "mariadb", "-uroot", "-pacceptance-root", "-Nse", "SELECT SCHEMA_NAME FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME='wm_accept_mysql'")
	requireContainerOutput(t, mariaName, "", "mariadb", "-uroot", "-pacceptance-root", "-Nse", "SELECT User FROM mysql.user WHERE User='wm_user'")
	requireSQLiteCount(t, store.DB(), 0, `SELECT COUNT(*) FROM database_users WHERE database_name='acceptance-mariadb' AND username='wm_user'`)
	requireSQLiteCount(t, store.DB(), 0, `SELECT COUNT(*) FROM database_grants WHERE server_name='acceptance-mariadb' AND database_name='wm_accept_mysql'`)

	pg := postDatabaseJSON(t, mux, "/api/v2/databases/pg", map[string]any{"name": "wm_accept_pg", "database": "acceptance-postgres", "username": "wm_pg_user", "password": "YWNjZXB0YW5jZS11c2Vy", "from": "remote"})
	pgID := responseDatabaseID(t, pg)
	requireContainerOutput(t, pgName, "wm_accept_pg", "psql", "-U", "postgres", "-Atc", "SELECT datname FROM pg_database WHERE datname='wm_accept_pg'")
	requireSQLiteCount(t, store.DB(), 1, `SELECT COUNT(*) FROM database_runtime_configs WHERE database_name='`+strconv.FormatInt(pgID, 10)+`' AND config_key='postgres'`)
	if out, err := exec.Command("docker", "exec", pgName, "psql", "-U", "postgres", "-c", "CREATE DATABASE wm_synced_pg;").CombinedOutput(); err != nil {
		t.Fatalf("创建 PostgreSQL 同步样本失败: %v: %s", err, out)
	}
	postDatabaseJSON(t, mux, "/api/v2/databases/pg/acceptance-postgres/load", map[string]any{"from": "remote", "type": "postgresql", "database": "acceptance-postgres"})
	var syncedPG service.Database
	var syncedPGFound bool
	for _, item := range databaseService.Search(t.Context(), "postgresql", "wm_synced_pg") {
		if strings.EqualFold(item.InitialDB, "acceptance-postgres") {
			syncedPG, syncedPGFound = item, true
			break
		}
	}
	if !syncedPGFound {
		t.Fatal("PostgreSQL 远程同步未写入 SQLite")
	}
	postDatabaseJSON(t, mux, "/api/v2/databases/pg/del", map[string]any{"id": syncedPG.ID, "database": "acceptance-postgres"})
	postDatabaseJSON(t, mux, "/api/v2/databases/pg/bind", map[string]any{"name": "wm_accept_pg", "database": "acceptance-postgres", "username": "wm_pg_bound", "password": "Ym91bmQtcGFzc3dvcmQ=", "superUser": false})
	postDatabaseJSON(t, mux, "/api/v2/databases/pg/privileges", map[string]any{"name": "wm_accept_pg", "database": "acceptance-postgres", "username": "wm_pg_bound", "superUser": false})
	postDatabaseJSON(t, mux, "/api/v2/databases/pg/password", map[string]any{"id": pgID, "database": "acceptance-postgres", "value": "bmV3LXBhc3N3b3Jk"})
	postDatabaseJSON(t, mux, "/api/v2/databases/pg/del", map[string]any{"id": pgID, "database": "acceptance-postgres"})
	requireContainerOutput(t, pgName, "", "psql", "-U", "postgres", "-Atc", "SELECT datname FROM pg_database WHERE datname='wm_accept_pg'")
	requireContainerOutput(t, pgName, "", "psql", "-U", "postgres", "-Atc", "SELECT rolname FROM pg_roles WHERE rolname IN ('wm_pg_user','wm_pg_bound') ORDER BY rolname")
	requireSQLiteCount(t, store.DB(), 0, `SELECT COUNT(*) FROM database_runtime_configs WHERE database_name='`+strconv.FormatInt(pgID, 10)+`'`)

	postDatabaseJSON(t, mux, "/api/v2/databases/redis/status", map[string]any{"name": "acceptance-redis", "type": "redis"})
	postDatabaseJSON(t, mux, "/api/v2/databases/redis/conf", map[string]any{"name": "acceptance-redis", "type": "redis"})
	postDatabaseJSON(t, mux, "/api/v2/databases/redis/conf/update", map[string]any{"database": "acceptance-redis", "dbType": "redis", "timeout": "61", "maxclients": "512", "maxmemory": "16mb"})
	postDatabaseJSON(t, mux, "/api/v2/databases/redis/persistence/update", map[string]any{"database": "acceptance-redis", "dbType": "redis", "appendonly": "yes", "appendfsync": "everysec", "save": "60 1"})
	requireSQLiteCount(t, store.DB(), 1, `SELECT COUNT(*) FROM database_runtime_configs WHERE database_name='acceptance-redis' AND config_key='redis'`)
	postDatabaseJSON(t, mux, "/api/v2/databases/redis/password", map[string]any{"database": "acceptance-redis", "value": "cmVkaXMtcGFzc3dvcmQ="})
	postDatabaseJSON(t, mux, "/api/v2/databases/redis/status", map[string]any{"name": "acceptance-redis", "type": "redis"})
	if out, err := exec.Command("docker", "stop", redisName).CombinedOutput(); err != nil {
		t.Fatalf("stop redis: %v: %s", err, out)
	}
	failed := callDatabaseJSON(t, mux, "/api/v2/databases/redis/status", map[string]any{"name": "acceptance-redis", "type": "redis"})
	if failed.Code != http.StatusServiceUnavailable || !strings.Contains(failed.Body.String(), `"code":"ERR"`) {
		t.Fatalf("redis dependency failure status=%d body=%s", failed.Code, failed.Body.String())
	}
	var redisServerID int64
	for _, item := range databaseService.Search(t.Context(), "redis", "acceptance-redis") {
		redisServerID = item.ID
	}
	postDatabaseJSON(t, mux, "/api/v2/databases/db/del", map[string]any{"id": redisServerID})
	requireSQLiteCount(t, store.DB(), 0, `SELECT COUNT(*) FROM database_runtime_configs WHERE database_name='acceptance-redis'`)

	mongo := postDatabaseJSON(t, mux, "/api/v2/databases/mongodb", map[string]any{"name": "wm_accept_mongo", "database": "acceptance-mongodb", "username": "wm_mongo_user", "password": "YWNjZXB0YW5jZS11c2Vy", "permission": "readWrite", "from": "local"})
	mongoID := responseDatabaseID(t, mongo)
	requireMongoOutput(t, mongoName, "true", "acceptance-root", `db.getMongo().getDBNames().includes("wm_accept_mongo")`)
	requireMongoOutput(t, mongoName, "wm_mongo_user", "acceptance-root", `db.getSiblingDB("wm_accept_mongo").getUser("wm_mongo_user").user`)
	privileges := postDatabaseJSON(t, mux, "/api/v2/databases/mongodb/privileges", map[string]any{"database": "acceptance-mongodb", "name": "wm_accept_mongo", "username": "wm_mongo_user"})
	if !strings.Contains(string(privileges), `"data":"readWrite"`) {
		t.Fatalf("MongoDB 权限读取错误: %s", privileges)
	}
	postDatabaseJSON(t, mux, "/api/v2/databases/mongodb/bind", map[string]any{"database": "acceptance-mongodb", "name": "wm_accept_mongo", "username": "wm_mongo_bound", "password": "Ym91bmQtcGFzc3dvcmQ="})
	postDatabaseJSON(t, mux, "/api/v2/databases/mongodb/password", map[string]any{"database": "acceptance-mongodb", "name": "wm_accept_mongo", "username": "wm_mongo_bound", "password": "bmV3LW1vbmdvLXBhc3M="})
	postDatabaseJSON(t, mux, "/api/v2/databases/mongodb/privileges/change", map[string]any{"database": "acceptance-mongodb", "name": "wm_accept_mongo", "username": "wm_mongo_bound", "permission": "read"})
	postDatabaseJSON(t, mux, "/api/v2/databases/mongodb/root/password", map[string]any{"database": "acceptance-mongodb", "username": "root", "value": "bmV3LW1vbmdvLXJvb3Q="})
	privileges = postDatabaseJSON(t, mux, "/api/v2/databases/mongodb/privileges", map[string]any{"database": "acceptance-mongodb", "name": "wm_accept_mongo", "username": "wm_mongo_bound"})
	if !strings.Contains(string(privileges), `"data":"read"`) {
		t.Fatalf("MongoDB 改权或 root 密码持久化失败: %s", privileges)
	}
	postDatabaseJSON(t, mux, "/api/v2/databases/mongodb/del", map[string]any{"id": mongoID, "database": "acceptance-mongodb", "name": "wm_accept_mongo"})
	postDatabaseJSON(t, mux, "/api/v2/databases/mongodb/del", map[string]any{"id": syncedMongo.ID, "database": "acceptance-mongodb", "name": "wm_synced_mongo"})
	requireMongoOutput(t, mongoName, "false", "new-mongo-root", `db.getMongo().getDBNames().includes("wm_accept_mongo")`)
	requireSQLiteCount(t, store.DB(), 0, `SELECT COUNT(*) FROM databases WHERE id=`+strconv.FormatInt(mongoID, 10))
}

func requireMongoOutput(t *testing.T, container, expected, password, script string) {
	t.Helper()
	out, err := exec.Command("docker", "exec", container, "mongosh", "--quiet", "--username", "root", "--password", password, "--authenticationDatabase", "admin", "--eval", script).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != expected {
		t.Fatalf("MongoDB 验证输出=%q expected=%q err=%v", out, expected, err)
	}
}

func requireSQLiteCount(t *testing.T, db interface{ QueryRow(string, ...any) *sql.Row }, expected int, query string) {
	t.Helper()
	var count int
	if err := db.QueryRow(query).Scan(&count); err != nil || count != expected {
		t.Fatalf("sqlite count=%d expected=%d err=%v query=%s", count, expected, err, query)
	}
}

func requireContainerOutput(t *testing.T, container, expected string, command ...string) {
	t.Helper()
	args := append([]string{"exec", container}, command...)
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != expected {
		t.Fatalf("docker %v output=%q err=%v", args, out, err)
	}
}

func waitContainerCommand(t *testing.T, container string, command ...string) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		args := append([]string{"exec", container}, command...)
		if exec.Command("docker", args...).Run() == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("container %s did not become ready", container)
}

func dockerPort(t *testing.T, container, port string) int {
	t.Helper()
	out, err := exec.Command("docker", "port", container, port).Output()
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	i := strings.LastIndex(line, ":")
	value, err := strconv.Atoi(line[i+1:])
	if err != nil {
		t.Fatalf("docker port %q: %v", line, err)
	}
	return value
}

func postDatabaseJSON(t *testing.T, mux http.Handler, path string, body map[string]any) []byte {
	t.Helper()
	payload, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body.String())
	}
	return w.Body.Bytes()
}

func responseDatabaseID(t *testing.T, body []byte) int64 {
	t.Helper()
	var envelope struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Data.ID <= 0 {
		t.Fatalf("invalid database response: %s (%v)", body, err)
	}
	return envelope.Data.ID
}

func callDatabaseJSON(t *testing.T, mux http.Handler, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	mux.ServeHTTP(res, req)
	return res
}
