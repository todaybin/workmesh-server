// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var mysqlExecutor = service.NewMySQLExecutor()
var mongoExecutor = service.NewMongoDBExecutor()

var databaseContainerIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

func databaseSourceIsLocal(source string) bool {
	source = strings.TrimSpace(source)
	return source == "" || strings.EqualFold(source, "local")
}

func databaseSourceForTarget(source, containerName string) string {
	source = strings.TrimSpace(source)
	if source == "" && strings.TrimSpace(containerName) != "" {
		return "local"
	}
	return source
}

// legacyDatabaseContainerName keeps old local records usable while refusing
// to treat an arbitrary remote host or host:port value as a Docker name.
func legacyDatabaseContainerName(source, host string) string {
	if !databaseSourceIsLocal(source) {
		return ""
	}
	host = strings.TrimSpace(host)
	if host == "" || strings.EqualFold(host, "localhost") {
		return ""
	}
	if net.ParseIP(strings.Trim(host, "[]")) != nil || strings.ContainsAny(host, ":/\\") {
		return ""
	}
	if !databaseContainerIdentifier.MatchString(host) {
		return ""
	}
	return host
}

// normalizedDatabaseTarget separates the network address persisted in Host
// from the Docker identity used by docker exec.
func normalizedDatabaseTarget(source, host, containerName string) (string, string) {
	host = strings.TrimSpace(host)
	containerName = strings.TrimSpace(containerName)
	if containerName == "" {
		containerName = legacyDatabaseContainerName(source, host)
	}
	if containerName != "" {
		return "127.0.0.1", containerName
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return host, ""
}

func normalizedDatabaseItemTarget(item service.Database) (string, string) {
	return normalizedDatabaseTarget(item.From, item.Host, item.ContainerName)
}

// mysqlTargetForRequest 从 SQLite 登记信息解析真实 MySQL/MariaDB 管理连接。
func mysqlTargetForRequest(ctx context.Context, name, typ string) (service.MySQLTarget, bool) {
	name = strings.TrimSpace(name)
	typ = strings.ToLower(strings.TrimSpace(typ))
	if typ == "" {
		typ = "mysql"
	}
	item, ok := databaseService.FindByName(ctx, typ, name)
	if !ok && typ == "mysql" {
		item, ok = databaseService.FindByName(ctx, "mariadb", name)
	}
	if !ok || (item.Type != "mysql" && item.Type != "mariadb") {
		return service.MySQLTarget{}, false
	}
	host, containerName := normalizedDatabaseItemTarget(item)
	target := service.MySQLTarget{Type: item.Type, Host: host, Port: item.Port, Username: item.Username, Password: item.Password, ContainerName: containerName}
	return target, true
}

// databaseOperation 记录需要在目标数据库上执行的管理操作，避免返回固定成功值。
type databaseOperation struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Target    string    `json:"target"`
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

func appendDatabaseOperation(op databaseOperation) error {
	repository, err := SharedRepository()
	if err != nil {
		return err
	}
	if _, err := repository.Exec(`INSERT INTO database_operations(id,type,target,status,message,created_at) VALUES(?,?,?,?,?,?)`, op.ID, op.Type, op.Target, op.Status, op.Message, op.CreatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	_, err = repository.Exec(`DELETE FROM database_operations WHERE id NOT IN (SELECT id FROM database_operations ORDER BY created_at DESC LIMIT 1000)`)
	return err
}

func databasePort(typ string, port int) int {
	if port > 0 {
		return port
	}
	switch strings.ToLower(typ) {
	case "redis":
		return 6379
	case "postgres", "postgresql", "pg":
		return 5432
	case "mongodb", "mongo":
		return 27017
	default:
		return 3306
	}
}

func checkDatabaseEndpoint(host, typ string, port int, timeout time.Duration) (bool, string) {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	}
	port = databasePort(typ, port)
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 2 * time.Second
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), timeout)
	if err != nil {
		return false, err.Error()
	}
	_ = conn.Close()
	return true, ""
}

func requiresDatabaseConnectionCheck(typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "mysql", "mariadb", "postgres", "postgresql", "pg", "redis":
		return true
	default:
		return false
	}
}

func checkDatabaseConnection(ctx context.Context, req databaseRequest) error {
	if req.ID > 0 {
		if item, ok := databaseService.Find(ctx, req.ID); ok {
			req.Type = item.Type
			req.From = item.From
			req.Host = item.Host
			req.ContainerName = item.ContainerName
			req.Port = item.Port
			req.Username = item.Username
			req.InitialDB = item.InitialDB
			if req.Password == "" {
				req.Password = item.Password
			}
		}
	}
	typ := strings.ToLower(strings.TrimSpace(req.Type))
	password := decodeDatabaseSecret(req.Password)
	host, containerName := normalizedDatabaseTarget(req.From, req.Host, req.ContainerName)
	switch typ {
	case "mysql", "mariadb":
		target := service.MySQLTarget{Type: typ, Host: host, Port: req.Port, Username: req.Username, Password: password, ContainerName: containerName}
		return mysqlExecutor.Exec(ctx, target, "SELECT 1;")
	case "postgres", "postgresql", "pg":
		_, err := postgresExecutor.Exec(ctx, service.PostgresTarget{Type: typ, Host: host, Port: req.Port, Username: req.Username, Password: password, Database: req.InitialDB, ContainerName: containerName}, "SELECT 1;")
		return err
	case "redis":
		out, err := redisExecutor.Exec(ctx, service.RedisTarget{Host: host, Port: req.Port, Password: password, ContainerName: containerName}, "PING")
		if err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(out), "PONG") {
			return errors.New("Redis PING 响应无效")
		}
		return nil
	default:
		available, message := checkDatabaseEndpoint(req.Host, typ, req.Port, time.Duration(req.Timeout)*time.Millisecond)
		if !available {
			return errors.New(message)
		}
		return nil
	}
}

func registerDatabaseRoutes(mux *http.ServeMux) {
	registerDatabaseRuntimeRoutes(mux)
	registerDatabaseBackupRoutes(mux)
	// MySQL 前端使用 /databases 作为创建入口；该接口会在目标实例执行建库。
	mux.HandleFunc("POST /api/v2/databases", handleMySQLDatabaseCreate)
	// PostgreSQL 及 Redis 使用专用执行器，不能落入通用兼容处理器。
	mux.HandleFunc("POST /api/v2/databases/pg", handlePostgresCreate)
	mux.HandleFunc("POST /api/v2/databases/pg/bind", handlePostgresBind)
	mux.HandleFunc("POST /api/v2/databases/pg/privileges", handlePostgresPrivileges)
	mux.HandleFunc("POST /api/v2/databases/pg/password", handlePostgresPassword)
	mux.HandleFunc("POST /api/v2/databases/pg/search", handlePostgresSearch)
	mux.HandleFunc("POST /api/v2/databases/pg/description", handlePostgresDescription)
	mux.HandleFunc("POST /api/v2/databases/pg/{database}/load", handlePostgresLoadRemote)
	mux.HandleFunc("POST /api/v2/databases/load", handleMySQLLoadRemote)
	mux.HandleFunc("POST /api/v2/databases/pg/del", handlePostgresDelete)
	mux.HandleFunc("POST /api/v2/databases/pg/del/check", handleDatabaseDeleteCheck)
	mux.HandleFunc("POST /api/v2/databases/mongodb/del/check", handleMongoDeleteCheck)
	mux.HandleFunc("POST /api/v2/databases/mongodb", handleMongoCreate)
	mux.HandleFunc("POST /api/v2/databases/mongodb/search", handleMongoSearch)
	mux.HandleFunc("POST /api/v2/databases/mongodb/load", handleMongoLoadRemote)
	mux.HandleFunc("POST /api/v2/databases/mongodb/bind", handleMongoBind)
	mux.HandleFunc("POST /api/v2/databases/mongodb/password", handleMongoPassword)
	mux.HandleFunc("POST /api/v2/databases/mongodb/root/password", handleMongoRootPassword)
	mux.HandleFunc("POST /api/v2/databases/mongodb/description", handleMongoDescription)
	mux.HandleFunc("POST /api/v2/databases/mongodb/del", handleMongoDelete)
	mux.HandleFunc("POST /api/v2/databases/mongodb/privileges", handleMongoPrivileges)
	mux.HandleFunc("POST /api/v2/databases/mongodb/privileges/change", handleMongoPrivilegesChange)
	mux.HandleFunc("POST /api/v2/databases/redis/status", handleRedisStatus)
	mux.HandleFunc("POST /api/v2/databases/redis/conf", handleRedisConfig)
	mux.HandleFunc("POST /api/v2/databases/redis/conf/update", handleRedisConfigUpdate)
	mux.HandleFunc("POST /api/v2/databases/redis/persistence/conf", handleRedisPersistenceConfig)
	mux.HandleFunc("POST /api/v2/databases/redis/persistence/update", handleRedisConfigUpdate)
	mux.HandleFunc("POST /api/v2/databases/redis/password", handleRedisPassword)
	mux.HandleFunc("GET /api/v2/databases/redis/check", handleRedisCLICheck)
	mux.HandleFunc("POST /api/v2/databases/redis/install/cli", handleRedisInstallCLI)
	mux.HandleFunc("POST /api/v2/databases/db", handleDatabaseCreate)
	mux.HandleFunc("POST /api/v2/databases/db/check", handleDatabaseCheck)
	mux.HandleFunc("POST /api/v2/databases/db/del", handleDatabaseDelete)
	mux.HandleFunc("POST /api/v2/databases/db/del/check", handleDatabaseDeleteCheck)
	mux.HandleFunc("POST /api/v2/databases/db/search", handleDatabaseSearch)
	mux.HandleFunc("POST /api/v2/databases/db/update", handleDatabaseUpdate)
	mux.HandleFunc("/api/v2/databases/", databaseRoute)
}

// handleMySQLLoadRemote 从真实 MySQL/MariaDB 实例同步非系统 schema。
func handleMySQLLoadRemote(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	server := strField(b, "database")
	if server == "" {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "MySQL 实例不能为空"})
		return
	}
	serverItem, found := databaseService.FindByName(r.Context(), "mysql", server)
	if !found {
		serverItem, found = databaseService.FindByName(r.Context(), "mariadb", server)
	}
	if !found {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": "目标 MySQL/MariaDB 实例未登记"})
		return
	}
	target, found := mysqlTargetForRequest(r.Context(), server, serverItem.Type)
	if !found {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": "目标 MySQL/MariaDB 实例未登记"})
		return
	}
	queryExecutor, ok := mysqlExecutor.(service.MySQLQueryExecutor)
	if !ok {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": "MySQL 查询执行器不可用"})
		return
	}
	names, err := service.ListMySQLDatabases(r.Context(), queryExecutor, target)
	if err != nil {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	existing := make(map[string]struct{})
	for _, item := range databaseService.Search(r.Context(), serverItem.Type, "") {
		if strings.EqualFold(item.InitialDB, server) {
			existing[strings.ToLower(item.Name)] = struct{}{}
		}
	}
	created := make([]service.Database, 0)
	for _, name := range names {
		if _, exists := existing[strings.ToLower(name)]; exists {
			continue
		}
		item, createErr := databaseService.Create(r.Context(), service.Database{Name: name, Type: serverItem.Type, From: databaseSourceForTarget(serverItem.From, target.ContainerName), Host: target.Host, Port: target.Port, ContainerName: target.ContainerName, InitialDB: server})
		if createErr != nil {
			for index := len(created) - 1; index >= 0; index-- {
				_ = databaseService.Delete(r.Context(), created[index].ID)
			}
			wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": createErr.Error()})
			return
		}
		created = append(created, item)
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"synced": len(names), "created": len(created)}})
}

// handleMySQLDatabaseCreate 在真实 MySQL/MariaDB 实例创建 schema，成功后才登记 SQLite 元数据。
func handleMySQLDatabaseCreate(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	name, server := strField(b, "name"), strField(b, "database")
	if server == "" {
		// 兼容离线登记工具：真实 HTTP 服务的前端始终提供已登记实例名。
		if sharedDB() == nil && name != "" {
			host, containerName := normalizedDatabaseTarget(strField(b, "from"), strField(b, "address"), strField(b, "containerName"))
			item, createErr := databaseService.Create(r.Context(), service.Database{Name: name, Type: "mysql", From: databaseSourceForTarget(strField(b, "from"), containerName), Host: host, Port: int(intField(b, "port")), ContainerName: containerName, Username: strField(b, "username"), Description: strField(b, "description")})
			if createErr != nil {
				wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": createErr.Error()})
				return
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
			return
		}
	}
	if name == "" || server == "" {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "数据库名称和目标实例不能为空"})
		return
	}
	target, ok := mysqlTargetForRequest(r.Context(), server, "")
	if !ok {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": "目标 MySQL/MariaDB 实例未登记"})
		return
	}
	password := decodeDatabaseSecret(strField(b, "password"))
	username := strField(b, "username")
	if username != "" && password == "" {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "创建数据库用户时密码不能为空"})
		return
	}
	if err = service.CreateMySQLDatabase(r.Context(), mysqlExecutor, target, name, strField(b, "format"), strField(b, "collation"), username, password, strField(b, "permission")); err != nil {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	host := databaseAdminHost(strField(b, "permission"))
	item, err := databaseService.Create(r.Context(), service.Database{Name: name, Type: mysqlTypeForTarget(target), From: databaseSourceForTarget(strField(b, "from"), target.ContainerName), Host: target.Host, Port: target.Port, ContainerName: target.ContainerName, InitialDB: server, Username: username, Password: password, Description: strField(b, "description")})
	if err != nil {
		// 外部建库已成功但登记失败，尽力回滚，避免产生孤立 schema。
		_ = service.DropMySQLDatabase(r.Context(), mysqlExecutor, target, name)
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if username != "" {
		user := service.DatabaseUser{DatabaseID: item.ID, Database: server, Type: item.Type, Username: username, Host: host, Description: strField(b, "description"), PasswordSet: true}
		if _, err = databaseAdmin.CreateUser(r.Context(), user); err != nil {
			_ = databaseService.Delete(r.Context(), item.ID)
			_ = service.DropMySQLUser(r.Context(), mysqlExecutor, target, username, host)
			_ = service.DropMySQLDatabase(r.Context(), mysqlExecutor, target, name)
			wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		if _, err = databaseAdmin.UpsertGrant(r.Context(), service.DatabaseGrant{Server: server, Database: name, Username: username, Host: host, Privileges: []string{"ALL"}}); err != nil {
			rollbackUserMetadata(r.Context(), user)
			_ = databaseService.Delete(r.Context(), item.ID)
			_ = service.DropMySQLUser(r.Context(), mysqlExecutor, target, username, host)
			_ = service.DropMySQLDatabase(r.Context(), mysqlExecutor, target, name)
			wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
}

// handleDatabaseDeleteCheck 校验登记资源是否存在，返回真实依赖列表而不是伪造连接成功。
func handleDatabaseDeleteCheck(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&payload); err != nil && err != io.EOF {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if _, ok := databaseService.Find(r.Context(), payload.ID); !ok {
		wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": "数据库不存在"})
		return
	}
	// 依赖关系由网站/应用域维护；当前无登记依赖时返回空数组，供前端安全确认删除。
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": []any{}})
}

// deleteRegisteredDatabase 删除真实 schema 后再删除 SQLite 登记，避免面板记录先于外部状态消失。
func deleteRegisteredDatabase(ctx context.Context, id int64) error {
	item, ok := databaseService.Find(ctx, id)
	if !ok {
		return errors.New("数据库不存在")
	}
	var mysqlTarget service.MySQLTarget
	var mysqlTargetOK bool
	userHost := "%"
	dropUser := false
	if (item.Type == "mysql" || item.Type == "mariadb") && strings.TrimSpace(item.InitialDB) != "" {
		target, targetOK := mysqlTargetForRequest(ctx, item.InitialDB, item.Type)
		if !targetOK {
			return errors.New("目标 MySQL/MariaDB 实例未登记")
		}
		if err := service.DropMySQLDatabase(ctx, mysqlExecutor, target, item.Name); err != nil {
			return err
		}
		mysqlTarget, mysqlTargetOK = target, targetOK
		if item.Username != "" {
			grants := databaseAdmin.ListGrants(ctx, item.InitialDB, item.Username)
			otherGrants := 0
			for _, grant := range grants {
				if strings.EqualFold(grant.Database, item.Name) {
					userHost = grant.Host
				} else {
					otherGrants++
				}
			}
			dropUser = otherGrants == 0
			if dropUser {
				if err := service.DropMySQLUser(ctx, mysqlExecutor, target, item.Username, userHost); err != nil {
					_ = service.CreateMySQLDatabase(ctx, mysqlExecutor, target, item.Name, "", "", "", "", "")
					return err
				}
			}
		}
	}
	if err := databaseService.Delete(ctx, id); err != nil {
		// SQLite 写入失败时尽力恢复外部 schema，调用方仍会收到错误而不会误报成功。
		if mysqlTargetOK {
			username, password, host := "", "", ""
			if dropUser {
				username, password, host = item.Username, item.Password, userHost
			}
			_ = service.CreateMySQLDatabase(ctx, mysqlExecutor, mysqlTarget, item.Name, "", "", username, password, host)
		}
		return err
	}
	if repository, repositoryErr := SharedRepository(); repositoryErr == nil {
		_, _ = repository.ExecContext(ctx, `DELETE FROM database_runtime_configs WHERE database_name IN (?,?)`, item.Name, strconv.FormatInt(item.ID, 10))
	}
	if mysqlTargetOK && item.Username != "" {
		_ = databaseAdmin.DeleteGrantByIdentity(ctx, item.InitialDB, item.Name, item.Username, userHost)
		if dropUser {
			_ = databaseAdmin.DeleteUserByIdentity(ctx, item.InitialDB, item.Username, userHost)
		}
	}
	return nil
}

func isDatabaseRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return path == "/api/v2/databases" || strings.HasPrefix(path, "/api/v2/databases/")
}

func databaseRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/databases/"), "/")
	if r.Method == http.MethodGet {
		typ, name := "", r.URL.Query().Get("name")
		segments := strings.Split(path, "/")
		if len(segments) == 2 && segments[0] == "db" {
			name = segments[1]
		}
		if len(segments) == 3 && segments[0] == "db" && segments[1] == "list" {
			typ = segments[2]
		}
		if strings.HasSuffix(path, "/check") {
			checkType := typ
			if checkType == "" {
				checkType = strings.TrimSuffix(strings.TrimPrefix(path, "db/"), "/check")
				if strings.Contains(checkType, "/") {
					checkType = strings.SplitN(checkType, "/", 2)[0]
				}
			}
			available, message := checkDatabaseEndpoint(r.URL.Query().Get("host"), checkType, 0, 2*time.Second)
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"available": available, "type": checkType, "error": message}})
			return
		}
		items := databaseService.Search(r.Context(), typ, name)
		if len(segments) == 2 && segments[0] == "db" {
			for _, item := range items {
				if strings.EqualFold(item.Name, name) {
					wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
					return
				}
			}
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": "database not found"})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": 50}})
		return
	}
	var payload struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		Type          string `json:"type"`
		Host          string `json:"host"`
		ContainerName string `json:"containerName"`
		Port          int    `json:"port"`
		Username      string `json:"username"`
		Description   string `json:"description"`
		Timeout       int    `json:"timeout"`
		Operate       string `json:"operate"`
		Operation     string `json:"operation"`
		Database      string `json:"database"`
		Password      string `json:"password"`
		From          string `json:"from"`
		InitialDB     string `json:"initialDB"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&payload); err != nil && err != io.EOF {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "INVALID_JSON"}, "message": err.Error()})
			return
		}
	}
	payload.Type = strings.ToLower(strings.TrimSpace(payload.Type))
	if payload.Type == "" {
		switch {
		case strings.HasPrefix(path, "redis"):
			payload.Type = "redis"
		case strings.HasPrefix(path, "pg") || strings.HasPrefix(path, "postgres"):
			payload.Type = "postgresql"
		case strings.HasPrefix(path, "mongodb"):
			payload.Type = "mongodb"
		case strings.HasPrefix(path, "mariadb"):
			payload.Type = "mariadb"
		default:
			payload.Type = "mysql"
		}
	}
	if strings.HasSuffix(path, "/check") || path == "redis/check" || path == "status" || path == "db/check" {
		if payload.ID > 0 || strings.TrimSpace(payload.ContainerName) != "" {
			checkReq := databaseRequest{
				ID: payload.ID, Type: payload.Type, Host: payload.Host, ContainerName: payload.ContainerName,
				Port: payload.Port, Username: payload.Username, Password: payload.Password,
				From: payload.From, InitialDB: payload.InitialDB, Timeout: payload.Timeout,
			}
			if err := checkDatabaseConnection(r.Context(), checkReq); err == nil {
				wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"host": defaultHost(payload.Host), "port": databasePort(payload.Type, payload.Port), "type": payload.Type, "available": true, "error": ""}})
			} else {
				wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"host": defaultHost(payload.Host), "port": databasePort(payload.Type, payload.Port), "type": payload.Type, "available": false, "error": err.Error()}})
			}
			return
		}
		available, message := checkDatabaseEndpoint(payload.Host, payload.Type, payload.Port, time.Duration(payload.Timeout)*time.Millisecond)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"host": defaultHost(payload.Host), "port": databasePort(payload.Type, payload.Port), "type": payload.Type, "available": available, "error": message}})
		return
	}
	// 统一处理旧版数据库管理入口：元数据操作写入本地仓库，远程管理操作写入审计记录并执行可验证的连接探测。
	if path == "" || path == "pg" || path == "mongodb" || path == "db" {
		host, containerName := normalizedDatabaseTarget(payload.From, payload.Host, payload.ContainerName)
		item, err := databaseService.Create(r.Context(), service.Database{Name: strings.TrimSpace(payload.Name), Type: payload.Type, From: databaseSourceForTarget(payload.From, containerName), Host: host, Port: databasePort(payload.Type, payload.Port), ContainerName: containerName, InitialDB: payload.InitialDB, Username: payload.Username, Password: payload.Password, Description: payload.Description})
		if err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
		return
	}
	if strings.HasSuffix(path, "/search") || path == "search" {
		items := databaseService.Search(r.Context(), payload.Type, payload.Name)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": 50}})
		return
	}
	if strings.HasSuffix(path, "/del") || path == "del" {
		if payload.ID <= 0 {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "database id is required"})
			return
		}
		if err := deleteRegisteredDatabase(r.Context(), payload.ID); err != nil {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"deleted": payload.ID}})
		return
	}
	if payload.Operate == "" {
		payload.Operate = payload.Operation
	}
	if payload.Operate == "" {
		payload.Operate = strings.Trim(path, "/")
	}
	host := payload.Host
	available, message := checkDatabaseEndpoint(host, payload.Type, payload.Port, 2*time.Second)
	status := "completed"
	if !available {
		status = "failed"
	}
	op := databaseOperation{ID: strconv.FormatInt(time.Now().UnixNano(), 10), Type: payload.Type, Target: payload.Database, Status: status, Message: message, CreatedAt: time.Now().UTC()}
	persistErr := appendDatabaseOperation(op)
	if !available {
		details := map[string]any{"errCode": "DATABASE_UNAVAILABLE", "operation": payload.Operate}
		if persistErr != nil {
			details["auditError"] = persistErr.Error()
		}
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "details": details, "message": message})
		return
	}
	if persistErr != nil {
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": persistErr.Error()})
		return
	}
	// 密码等敏感字段永不回显，仅返回操作审计状态。
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": op})
}

func defaultHost(host string) string {
	if strings.TrimSpace(host) == "" {
		return "127.0.0.1"
	}
	return strings.TrimSpace(host)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
