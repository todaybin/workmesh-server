// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var (
	postgresExecutor = service.NewPostgresExecutor()
	redisExecutor    = service.NewRedisExecutor()
)

func postgresTargetForRequest(ctx context.Context, name string) (service.PostgresTarget, bool) {
	name = strings.TrimSpace(name)
	for _, typ := range []string{"postgresql", "postgres", "postgresql-cluster"} {
		if item, ok := databaseService.FindByName(ctx, typ, name); ok {
			database := item.InitialDB
			if database == "" {
				database = "postgres"
			}
			host, containerName := normalizedDatabaseItemTarget(item)
			return service.PostgresTarget{Type: item.Type, Host: host, Port: item.Port, Username: item.Username, Password: item.Password, Database: database, ContainerName: containerName}, true
		}
	}
	return service.PostgresTarget{}, false
}

func redisTargetForRequest(ctx context.Context, name string) (service.RedisTarget, service.Database, bool) {
	name = strings.TrimSpace(name)
	for _, typ := range []string{"redis", "redis-cluster"} {
		if item, ok := databaseService.FindByName(ctx, typ, name); ok {
			host, containerName := normalizedDatabaseItemTarget(item)
			return service.RedisTarget{Host: host, Port: item.Port, Password: item.Password, ContainerName: containerName}, item, true
		}
	}
	return service.RedisTarget{}, service.Database{}, false
}

func postgresResource(item service.Database) map[string]any {
	result := map[string]any{
		"id": item.ID, "createdAt": item.CreatedAt, "name": item.Name,
		"postgresqlName": item.InitialDB, "from": item.From, "format": "",
		"username": item.Username, "password": item.Password, "superUser": false, "isDelete": false,
		"description": item.Description,
	}
	var cfg struct {
		SuperUser bool   `json:"superUser"`
		Format    string `json:"format"`
	}
	if loadDatabaseRuntimeConfig(strconv.FormatInt(item.ID, 10), "postgres", &cfg) == nil {
		result["superUser"] = cfg.SuperUser
		result["format"] = cfg.Format
	}
	return result
}

func redisResource(item service.Database) map[string]any {
	return map[string]any{"id": item.ID, "createdAt": item.CreatedAt, "name": item.Name, "from": item.From, "type": item.Type, "address": item.Host, "port": item.Port}
}

func writeDBError(w http.ResponseWriter, status int, err error) {
	if err == nil {
		err = errors.New("数据库操作失败")
	}
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
}

func boolField(b map[string]any, key string) bool {
	v, ok := b[key]
	if !ok {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(strings.TrimSpace(x), "true") || x == "1"
	case float64:
		return x != 0
	default:
		return false
	}
}

func databaseRuntimeTable(executor storage.SQLExecutor) error {
	if executor == nil {
		return errors.New("公共数据库未初始化")
	}
	_, err := executor.Exec(`CREATE TABLE IF NOT EXISTS database_runtime_configs (database_name TEXT NOT NULL, config_key TEXT NOT NULL, content BLOB NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(database_name,config_key))`)
	return err
}

func saveDatabaseRuntimeConfig(name, key string, value any) error {
	repository, err := SharedRepository()
	if err != nil {
		return err
	}
	if err := databaseRuntimeTable(repository); err != nil {
		return err
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = repository.Exec(`INSERT INTO database_runtime_configs(database_name,config_key,content,updated_at) VALUES(?,?,?,?) ON CONFLICT(database_name,config_key) DO UPDATE SET content=excluded.content,updated_at=excluded.updated_at`, name, key, payload, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func loadDatabaseRuntimeConfig(name, key string, out any) error {
	repository, err := SharedRepository()
	if err != nil {
		return err
	}
	if err := databaseRuntimeTable(repository); err != nil {
		return err
	}
	var payload []byte
	if err := repository.QueryRow(`SELECT content FROM database_runtime_configs WHERE database_name=? AND config_key=?`, name, key).Scan(&payload); err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}

func handlePostgresCreate(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		writeDBError(w, http.StatusBadRequest, err)
		return
	}
	name, server := strField(b, "name"), strField(b, "database")
	username, password := strField(b, "username"), decodeDatabaseSecret(strField(b, "password"))
	if name == "" || server == "" || username == "" || password == "" {
		writeDBError(w, http.StatusBadRequest, errors.New("PostgreSQL 数据库、用户和密码不能为空"))
		return
	}
	target, ok := postgresTargetForRequest(r.Context(), server)
	if !ok {
		writeDBError(w, http.StatusServiceUnavailable, errors.New("目标 PostgreSQL 实例未登记"))
		return
	}
	if err = service.CreatePostgresDatabase(r.Context(), postgresExecutor, target, name, username, password, boolField(b, "superUser")); err != nil {
		writeDBError(w, http.StatusServiceUnavailable, err)
		return
	}
	dbType := target.Type
	if dbType == "" {
		dbType = "postgresql"
	}
	item, err := databaseService.Create(r.Context(), service.Database{Name: name, Type: dbType, From: databaseSourceForTarget(strField(b, "from"), target.ContainerName), Host: target.Host, Port: target.Port, ContainerName: target.ContainerName, InitialDB: server, Username: username, Password: password, Description: strField(b, "description")})
	if err != nil {
		_, _ = postgresExecutor.Exec(r.Context(), target, "DROP DATABASE IF EXISTS "+service.QuotePostgresIdentifier(name)+";")
		_, _ = postgresExecutor.Exec(r.Context(), target, "DROP ROLE IF EXISTS "+service.QuotePostgresIdentifier(username)+";")
		writeDBError(w, http.StatusInternalServerError, err)
		return
	}
	if err = saveDatabaseRuntimeConfig(strconv.FormatInt(item.ID, 10), "postgres", map[string]any{"superUser": boolField(b, "superUser"), "format": strField(b, "format")}); err != nil {
		_ = databaseService.Delete(r.Context(), item.ID)
		_, _ = postgresExecutor.Exec(r.Context(), target, "DROP DATABASE IF EXISTS "+service.QuotePostgresIdentifier(name)+";")
		_, _ = postgresExecutor.Exec(r.Context(), target, "DROP ROLE IF EXISTS "+service.QuotePostgresIdentifier(username)+";")
		writeDBError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": postgresResource(item)})
}

func handlePostgresBind(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		writeDBError(w, 400, err)
		return
	}
	target, ok := postgresTargetForRequest(r.Context(), strField(b, "database"))
	if !ok {
		writeDBError(w, 503, errors.New("目标 PostgreSQL 实例未登记"))
		return
	}
	databaseName, newUsername := strField(b, "name"), strField(b, "username")
	removeNewRole := func() {
		_ = service.RevokePostgresDatabase(r.Context(), postgresExecutor, target, databaseName, newUsername)
		_ = service.DropPostgresRole(r.Context(), postgresExecutor, target, newUsername)
	}
	if err = service.CreatePostgresRole(r.Context(), postgresExecutor, target, newUsername, decodeDatabaseSecret(strField(b, "password")), boolField(b, "superUser")); err != nil {
		writeDBError(w, 503, err)
		return
	}
	if err = service.GrantPostgresDatabase(r.Context(), postgresExecutor, target, databaseName, newUsername); err != nil {
		removeNewRole()
		writeDBError(w, 503, err)
		return
	}
	item, found := findPostgresResource(r.Context(), strField(b, "database"), strField(b, "name"))
	if !found {
		removeNewRole()
		writeDBError(w, 404, errors.New("PostgreSQL 数据库不存在"))
		return
	}
	oldUsername := item.Username
	item.Username = newUsername
	if _, err = databaseService.Update(r.Context(), item); err != nil {
		removeNewRole()
		writeDBError(w, 500, err)
		return
	}
	if err = saveDatabaseRuntimeConfig(strconv.FormatInt(item.ID, 10), "postgres", map[string]any{"superUser": boolField(b, "superUser")}); err != nil {
		item.Username = oldUsername
		_, _ = databaseService.Update(r.Context(), item)
		removeNewRole()
		writeDBError(w, 500, err)
		return
	}
	if oldUsername != "" && !strings.EqualFold(oldUsername, item.Username) {
		if err = service.RevokePostgresDatabase(r.Context(), postgresExecutor, target, databaseName, oldUsername); err == nil {
			err = service.DropPostgresRole(r.Context(), postgresExecutor, target, oldUsername)
		}
		if err != nil {
			_ = service.GrantPostgresDatabase(r.Context(), postgresExecutor, target, databaseName, oldUsername)
			item.Username = oldUsername
			_, _ = databaseService.Update(r.Context(), item)
			removeNewRole()
			writeDBError(w, 503, err)
			return
		}
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"username": strField(b, "username"), "database": strField(b, "database")}})
}

func handlePostgresPrivileges(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		writeDBError(w, 400, err)
		return
	}
	target, ok := postgresTargetForRequest(r.Context(), strField(b, "database"))
	if !ok {
		writeDBError(w, 503, errors.New("目标 PostgreSQL 实例未登记"))
		return
	}
	if err = service.ChangePostgresPrivileges(r.Context(), postgresExecutor, target, strField(b, "username"), boolField(b, "superUser")); err != nil {
		writeDBError(w, 503, err)
		return
	}
	if item, found := findPostgresResource(r.Context(), strField(b, "database"), strField(b, "name")); found {
		if err = saveDatabaseRuntimeConfig(strconv.FormatInt(item.ID, 10), "postgres", map[string]any{"superUser": boolField(b, "superUser")}); err != nil {
			_ = service.ChangePostgresPrivileges(r.Context(), postgresExecutor, target, strField(b, "username"), !boolField(b, "superUser"))
			writeDBError(w, 500, err)
			return
		}
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"username": strField(b, "username"), "superUser": boolField(b, "superUser")}})
}

func findPostgresResource(ctx context.Context, server, name string) (service.Database, bool) {
	for _, item := range databaseService.Search(ctx, "postgresql", name) {
		if strings.EqualFold(item.Name, name) && strings.EqualFold(item.InitialDB, server) {
			return item, true
		}
	}
	return service.Database{}, false
}

func handlePostgresPassword(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		writeDBError(w, 400, err)
		return
	}
	server := strField(b, "database")
	target, ok := postgresTargetForRequest(r.Context(), server)
	if !ok {
		writeDBError(w, 503, errors.New("目标 PostgreSQL 实例未登记"))
		return
	}
	username := strField(b, "username")
	if username == "" {
		if item, found := databaseService.Find(r.Context(), intField(b, "id")); found {
			username = item.Username
		}
	}
	if err = service.ChangePostgresPassword(r.Context(), postgresExecutor, target, username, decodeDatabaseSecret(strField(b, "value"))); err != nil {
		writeDBError(w, 503, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"updated": true}})
}

func handlePostgresSearch(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		writeDBError(w, 400, err)
		return
	}
	server := strField(b, "database")
	live := syncPostgresLiveDatabases(r.Context(), server)
	children := visibleChildDatabases(databasesForTypes(r.Context(), databaseTypeFamily("postgresql")), server, strField(b, "info"), live)
	paged, total, page, pageSize := pageDatabaseSlice(children, int(intField(b, "page")), int(intField(b, "pageSize")))
	out := make([]map[string]any, 0, len(paged))
	for _, item := range paged {
		out = append(out, postgresResource(item))
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": out, "total": total, "page": page, "pageSize": pageSize}})
}

func handlePostgresDescription(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		writeDBError(w, 400, err)
		return
	}
	id := intField(b, "id")
	item, ok := databaseService.Find(r.Context(), id)
	if !ok || item.Type != "postgresql" {
		writeDBError(w, 404, errors.New("PostgreSQL 数据库不存在"))
		return
	}
	item.Description = strField(b, "description")
	updated, err := databaseService.Update(r.Context(), item)
	if err != nil {
		writeDBError(w, 500, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": postgresResource(updated)})
}

func handlePostgresLoadRemote(w http.ResponseWriter, r *http.Request) {
	server := strings.TrimSpace(r.PathValue("database"))
	if server == "" {
		writeDBError(w, http.StatusBadRequest, errors.New("PostgreSQL 实例不能为空"))
		return
	}
	target, ok := postgresTargetForRequest(r.Context(), server)
	if !ok {
		writeDBError(w, http.StatusServiceUnavailable, errors.New("目标 PostgreSQL 实例未登记"))
		return
	}
	names, err := service.ListPostgresDatabases(r.Context(), postgresExecutor, target)
	if err != nil {
		writeDBError(w, http.StatusServiceUnavailable, err)
		return
	}
	existing := make(map[string]struct{})
	source := "remote"
	if target.ContainerName != "" {
		source = "local"
	}
	dbType := target.Type
	if dbType == "" {
		dbType = "postgresql"
	}
	for _, item := range databasesForTypes(r.Context(), databaseTypeFamily(dbType)) {
		if strings.EqualFold(item.InitialDB, server) {
			existing[strings.ToLower(item.Name)] = struct{}{}
		}
	}
	created := make([]service.Database, 0)
	for _, name := range names {
		if _, found := existing[strings.ToLower(name)]; found {
			continue
		}
		item, createErr := databaseService.Create(r.Context(), service.Database{Name: name, Type: dbType, From: source, Host: target.Host, Port: target.Port, ContainerName: target.ContainerName, InitialDB: server})
		if createErr != nil {
			if _, exists := databaseService.FindByName(r.Context(), dbType, name); exists {
				continue
			}
			for index := len(created) - 1; index >= 0; index-- {
				_ = databaseService.Delete(r.Context(), created[index].ID)
			}
			writeDBError(w, http.StatusInternalServerError, createErr)
			return
		}
		created = append(created, item)
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"synced": len(names), "created": len(created)}})
}

func handlePostgresDelete(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		writeDBError(w, 400, err)
		return
	}
	id := intField(b, "id")
	item, ok := databaseService.Find(r.Context(), id)
	if !ok || item.Type != "postgresql" {
		writeDBError(w, 404, errors.New("PostgreSQL 数据库不存在"))
		return
	}
	target, targetOK := postgresTargetForRequest(r.Context(), item.InitialDB)
	if !targetOK {
		writeDBError(w, 503, errors.New("目标 PostgreSQL 实例未登记"))
		return
	}
	if err = service.DropPostgresDatabase(r.Context(), postgresExecutor, target, item.Name, ""); err != nil {
		writeDBError(w, 503, err)
		return
	}
	if err = databaseService.Delete(r.Context(), id); err != nil {
		if restoreErr := service.CreatePostgresDatabaseOnly(r.Context(), postgresExecutor, target, item.Name); restoreErr == nil {
			_ = service.GrantPostgresDatabase(r.Context(), postgresExecutor, target, item.Name, item.Username)
		}
		writeDBError(w, 500, err)
		return
	}
	if repository, repositoryErr := SharedRepository(); repositoryErr == nil {
		_, _ = repository.ExecContext(r.Context(), `DELETE FROM database_runtime_configs WHERE database_name=?`, strconv.FormatInt(id, 10))
	}
	if item.Username != "" {
		_ = service.DropPostgresRole(r.Context(), postgresExecutor, target, item.Username)
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"deleted": id}})
}

func handleRedisStatus(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		writeDBError(w, 400, err)
		return
	}
	target, _, ok := redisTargetForRequest(r.Context(), strField(b, "name"))
	if !ok {
		writeDBError(w, 503, errors.New("目标 Redis 实例未登记"))
		return
	}
	out, err := redisExecutor.Exec(r.Context(), target, "INFO")
	if err != nil {
		writeDBError(w, 503, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": parseRedisInfo(out)})
}

func parseRedisInfo(raw string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			out[parts[0]] = parts[1]
		}
	}
	return out
}

func redisConfigValue(ctx context.Context, target service.RedisTarget, key string) (string, error) {
	if err := service.ValidateRedisConfigKey(key); err != nil {
		return "", err
	}
	out, err := redisExecutor.Exec(ctx, target, "CONFIG", "GET", key)
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return "", errors.New("Redis 配置项不存在")
	}
	return strings.TrimSpace(lines[1]), nil
}

func handleRedisConfig(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		writeDBError(w, 400, err)
		return
	}
	name := strField(b, "name")
	target, item, ok := redisTargetForRequest(r.Context(), name)
	if !ok {
		writeDBError(w, 503, errors.New("目标 Redis 实例未登记"))
		return
	}
	keys := []string{"timeout", "maxclients", "maxmemory"}
	data := map[string]any{"name": item.Name, "port": item.Port}
	for _, key := range keys {
		value, e := redisConfigValue(r.Context(), target, key)
		if e != nil {
			writeDBError(w, 503, e)
			return
		}
		data[key] = value
	}
	// 保持前端字符串契约，但绝不回传 Redis 密码。
	data["requirepass"] = ""
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": data})
}

func handleRedisPersistenceConfig(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		writeDBError(w, 400, err)
		return
	}
	name := strField(b, "name")
	target, item, ok := redisTargetForRequest(r.Context(), name)
	if !ok {
		writeDBError(w, 503, errors.New("目标 Redis 实例未登记"))
		return
	}
	data := map[string]any{"database": item.Name}
	for _, key := range []string{"appendonly", "appendfsync", "save"} {
		value, e := redisConfigValue(r.Context(), target, key)
		if e != nil {
			writeDBError(w, 503, e)
			return
		}
		data[key] = value
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": data})
}

func handleRedisConfigUpdate(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		writeDBError(w, 400, err)
		return
	}
	name := strField(b, "database")
	target, item, ok := redisTargetForRequest(r.Context(), name)
	if !ok {
		writeDBError(w, 503, errors.New("目标 Redis 实例未登记"))
		return
	}
	updates := map[string]string{}
	for _, key := range []string{"timeout", "maxclients", "maxmemory", "appendonly", "appendfsync", "save"} {
		if value := strField(b, key); value != "" {
			updates[key] = value
		}
	}
	if len(updates) == 0 {
		writeDBError(w, 400, errors.New("Redis 配置不能为空"))
		return
	}
	old := map[string]string{}
	for key := range updates {
		old[key], err = redisConfigValue(r.Context(), target, key)
		if err != nil {
			writeDBError(w, 503, err)
			return
		}
	}
	applied := make([]string, 0, len(updates))
	for _, key := range sortedConfigKeys(updates) {
		value := updates[key]
		if err = service.ValidateRedisConfigKey(key); err != nil {
			writeDBError(w, 400, err)
			return
		}
		if _, err = redisExecutor.Exec(r.Context(), target, "CONFIG", "SET", key, value); err != nil {
			for i := len(applied) - 1; i >= 0; i-- {
				rollbackKey := applied[i]
				_, _ = redisExecutor.Exec(r.Context(), target, "CONFIG", "SET", rollbackKey, old[rollbackKey])
			}
			writeDBError(w, 503, err)
			return
		}
		applied = append(applied, key)
	}
	if _, err = redisExecutor.Exec(r.Context(), target, "CONFIG", "REWRITE"); err != nil {
		for i := len(applied) - 1; i >= 0; i-- {
			rollbackKey := applied[i]
			_, _ = redisExecutor.Exec(r.Context(), target, "CONFIG", "SET", rollbackKey, old[rollbackKey])
		}
		writeDBError(w, 503, err)
		return
	}
	if err = saveDatabaseRuntimeConfig(name, "redis", updates); err != nil {
		for rollbackKey, rollbackValue := range old {
			_, _ = redisExecutor.Exec(r.Context(), target, "CONFIG", "SET", rollbackKey, rollbackValue)
		}
		writeDBError(w, 500, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"updated": true, "database": item.Name}})
}

func sortedConfigKeys(values map[string]string) []string {
	order := []string{"timeout", "maxclients", "maxmemory", "appendonly", "appendfsync", "save"}
	out := make([]string, 0, len(values))
	for _, key := range order {
		if _, ok := values[key]; ok {
			out = append(out, key)
		}
	}
	return out
}

func handleRedisPassword(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		writeDBError(w, 400, err)
		return
	}
	name := strField(b, "database")
	target, item, ok := redisTargetForRequest(r.Context(), name)
	if !ok {
		writeDBError(w, 503, errors.New("目标 Redis 实例未登记"))
		return
	}
	password := decodeDatabaseSecret(strField(b, "value"))
	if _, err = redisExecutor.Exec(r.Context(), target, "CONFIG", "SET", "requirepass", password); err != nil {
		writeDBError(w, 503, err)
		return
	}
	item.Password = password
	if _, err = databaseService.Update(r.Context(), item); err != nil {
		_, _ = redisExecutor.Exec(r.Context(), service.RedisTarget{Host: target.Host, Port: target.Port, Password: password, ContainerName: target.ContainerName}, "CONFIG", "SET", "requirepass", target.Password)
		writeDBError(w, 500, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"updated": true}})
}

func handleRedisCLICheck(w http.ResponseWriter, r *http.Request) {
	_ = r
	bin := strings.TrimSpace(execEnv("WORKMESH_REDIS_CLI_BIN"))
	if bin == "" {
		bin = "redis-cli"
	}
	_, err := exec.LookPath(bin)
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": err == nil})
}

func handleRedisInstallCLI(w http.ResponseWriter, r *http.Request) {
	writeDBError(w, http.StatusServiceUnavailable, errors.New("不支持自动安装 redis-cli，请预先安装 CLI"))
}

func execEnv(key string) string { return strings.TrimSpace(os.Getenv(key)) }
