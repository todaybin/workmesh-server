// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var databaseAdmin = service.NewDatabaseAdminStore()

// readDatabaseBody 读取并限制数据库管理接口的 JSON 请求体。
func readDatabaseBody(r *http.Request) (map[string]any, error) {
	var b map[string]any
	if r.Body == nil {
		return map[string]any{}, nil
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(&b); err != nil && err != io.EOF {
		return nil, err
	}
	if b == nil {
		b = map[string]any{}
	}
	return b, nil
}

// strField 读取并清理数据库接口请求中的字符串字段。
func strField(b map[string]any, k string) string {
	if v, ok := b[k].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// intField 兼容 JSON 数字、数字字符串和 json.Number 的整数解析。
func intField(b map[string]any, k string) int64 {
	switch v := b[k].(type) {
	case float64:
		return int64(v)
	case json.Number:
		n, _ := strconv.ParseInt(string(v), 10, 64)
		return n
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}

func stringSliceField(b map[string]any, key string) []string {
	raw, ok := b[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		item, ok := value.(string)
		item = strings.TrimSpace(item)
		if !ok || item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

// dbAdminError 使用数据库管理接口统一的错误 envelope 返回失败信息。
func dbAdminError(w http.ResponseWriter, status int, err error) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
}

// RegisterDatabaseAdminRoutes 注册数据库用户、授权、变量及配置接口。
func RegisterDatabaseAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/databases/users/search", dbUsersSearch)
	mux.HandleFunc("POST /api/v2/databases/users", dbUsersCreate)
	mux.HandleFunc("POST /api/v2/databases/users/del", dbUsersDelete)
	mux.HandleFunc("POST /api/v2/databases/users/update", dbUsersUpdate)
	mux.HandleFunc("POST /api/v2/databases/users/password", dbUsersPassword)
	mux.HandleFunc("POST /api/v2/databases/users/password/save", dbUsersPassword)
	mux.HandleFunc("POST /api/v2/databases/grants/search", dbGrantsSearch)
	mux.HandleFunc("POST /api/v2/databases/grants/summary", dbGrantsSummary)
	mux.HandleFunc("POST /api/v2/databases/grants", dbGrantsCreate)
	mux.HandleFunc("POST /api/v2/databases/grants/del", dbGrantsDelete)
	mux.HandleFunc("POST /api/v2/databases/description/update", dbDescription)
	mux.HandleFunc("POST /api/v2/databases/variables", dbVariables)
	mux.HandleFunc("POST /api/v2/databases/variables/update", dbVariablesUpdate)
	mux.HandleFunc("POST /api/v2/databases/change/access", dbChangeAccess)
	mux.HandleFunc("POST /api/v2/databases/common/info", dbCommonInfo)
	mux.HandleFunc("POST /api/v2/databases/common/load/file", dbCommonFile)
	mux.HandleFunc("POST /api/v2/databases/common/update/conf", dbCommonUpdate)
	mux.HandleFunc("POST /api/v2/databases/format/options", dbFormatOptions)
	mux.HandleFunc("POST /api/v2/databases/status", dbStatus)
	mux.HandleFunc("POST /api/v2/databases/remote", dbRemote)
}

// dbUsersSearch 分页查询数据库用户记录。
func dbUsersSearch(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	items := databaseAdmin.ListUsers(r.Context(), strField(b, "database"), strField(b, "username"))
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": len(items)}})
}

// dbUsersCreate 创建数据库用户并保存真实状态。
func dbUsersCreate(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	server := strField(b, "database")
	target, err := mysqlTarget(r.Context(), server, strField(b, "type"))
	if err != nil {
		// 离线工具仍可维护 SQLite 元数据；HTTP 进程注入共享库后必须经过真实实例。
		if sharedDB() == nil {
			u := service.DatabaseUser{Database: server, Type: strField(b, "type"), Username: strField(b, "username"), Host: databaseAdminHost(strField(b, "host")), Description: strField(b, "description"), DatabaseID: intField(b, "databaseId"), PasswordSet: strField(b, "password") != ""}
			item, storeErr := databaseAdmin.CreateUser(r.Context(), u)
			if storeErr != nil {
				dbAdminError(w, http.StatusBadRequest, storeErr)
				return
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
			return
		}
		dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(err))
		return
	}
	username, password, host := strField(b, "username"), decodeDatabaseSecret(strField(b, "password")), databaseAdminHost(strField(b, "host"))
	if username == "" || password == "" {
		dbAdminError(w, http.StatusBadRequest, errors.New("数据库用户名和密码不能为空"))
		return
	}
	dbs := stringSliceField(b, "dbs")
	if err = service.CreateMySQLUser(r.Context(), mysqlExecutor, target, username, password, host, dbs); err != nil {
		dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(err))
		return
	}
	u := service.DatabaseUser{Database: server, Type: mysqlTypeForTarget(target), Username: username, Host: host, Description: strField(b, "description"), DatabaseID: intField(b, "databaseId"), PasswordSet: true}
	item, e := databaseAdmin.CreateUser(r.Context(), u)
	if e != nil {
		_ = service.DropMySQLUser(r.Context(), mysqlExecutor, target, username, host)
		dbAdminError(w, 400, e)
		return
	}
	for _, dbName := range dbs {
		if _, e = databaseAdmin.UpsertGrant(r.Context(), service.DatabaseGrant{Server: server, Database: dbName, Username: username, Host: host, Privileges: []string{"ALL"}}); e != nil {
			rollbackUserMetadata(r.Context(), item)
			_ = service.DropMySQLUser(r.Context(), mysqlExecutor, target, username, host)
			dbAdminError(w, http.StatusInternalServerError, e)
			return
		}
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}

// dbUsersDelete 删除指定数据库用户记录。
func dbUsersDelete(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	id := intField(b, "id")
	if id == 0 {
		id = intField(b, "userId")
	}
	server, username, host := strField(b, "database"), strField(b, "username"), databaseAdminHost(strField(b, "host"))
	item, found := databaseUserByIDOrIdentity(r.Context(), id, server, username, host)
	if !found {
		dbAdminError(w, http.StatusNotFound, errors.New("数据库用户不存在"))
		return
	}
	target, e2 := mysqlTarget(r.Context(), item.Database, item.Type)
	if e2 != nil {
		dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(e2))
		return
	}
	if e2 = service.DropMySQLUser(r.Context(), mysqlExecutor, target, item.Username, item.Host); e2 != nil {
		dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(e2))
		return
	}
	if id > 0 {
		e = databaseAdmin.DeleteUser(r.Context(), item.ID)
	} else {
		e = databaseAdmin.DeleteUserByIdentity(r.Context(), item.Database, item.Username, item.Host)
	}
	if e != nil {
		// 删除元数据失败时尽力恢复用户和其授权，避免远程实例出现孤立删除。
		_ = service.CreateMySQLUser(r.Context(), mysqlExecutor, target, item.Username, "", item.Host, grantDatabaseNames(r.Context(), item.Database, item.Username, item.Host))
		dbAdminError(w, 404, e)
		return
	}
	for _, grant := range databaseAdmin.ListGrants(r.Context(), item.Database, item.Username) {
		_ = databaseAdmin.DeleteGrantByIdentity(r.Context(), item.Database, grant.Database, grant.Username, grant.Host)
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"deleted": id}})
}

// dbUsersUpdate 更新数据库用户的连接和说明信息。
func dbUsersUpdate(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	u := service.DatabaseUser{ID: intField(b, "id"), DatabaseID: intField(b, "databaseId"), Database: strField(b, "database"), Type: strField(b, "type"), Username: strField(b, "username"), Host: strField(b, "host"), Description: strField(b, "description")}
	oldItem, found := databaseUserByIDOrIdentity(r.Context(), u.ID, u.Database, u.Username, u.Host)
	if !found {
		dbAdminError(w, http.StatusNotFound, errors.New("数据库用户不存在"))
		return
	}
	var item service.DatabaseUser
	if u.ID > 0 {
		target, targetErr := mysqlTarget(r.Context(), oldItem.Database, oldItem.Type)
		if targetErr != nil {
			dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(targetErr))
			return
		}
		newHost := databaseAdminHost(u.Host)
		if newHost != oldItem.Host {
			if targetErr = service.AlterMySQLUserHost(r.Context(), mysqlExecutor, target, oldItem.Username, oldItem.Host, newHost); targetErr != nil {
				dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(targetErr))
				return
			}
		}
		item, e = databaseAdmin.UpdateUser(r.Context(), u)
		if e != nil && newHost != oldItem.Host {
			_ = service.AlterMySQLUserHost(r.Context(), mysqlExecutor, target, oldItem.Username, newHost, oldItem.Host)
		}
	} else {
		target, targetErr := mysqlTarget(r.Context(), oldItem.Database, oldItem.Type)
		if targetErr != nil {
			dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(targetErr))
			return
		}
		newHost := databaseAdminHost(strField(b, "newHost"))
		if targetErr = service.AlterMySQLUserHost(r.Context(), mysqlExecutor, target, oldItem.Username, oldItem.Host, newHost); targetErr != nil {
			dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(targetErr))
			return
		}
		item, e = databaseAdmin.UpdateUserByIdentity(r.Context(), u, strField(b, "newHost"))
		if e != nil {
			_ = service.AlterMySQLUserHost(r.Context(), mysqlExecutor, target, oldItem.Username, newHost, oldItem.Host)
		}
	}
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}

// dbUsersPassword 标记数据库用户密码已设置并返回更新结果。
func dbUsersPassword(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	id := intField(b, "id")
	server, username, host := strField(b, "database"), strField(b, "username"), databaseAdminHost(strField(b, "host"))
	item, found := databaseUserByIDOrIdentity(r.Context(), id, server, username, host)
	if !found {
		dbAdminError(w, http.StatusNotFound, errors.New("数据库用户不存在"))
		return
	}
	target, targetErr := mysqlTarget(r.Context(), item.Database, item.Type)
	if targetErr != nil {
		dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(targetErr))
		return
	}
	if targetErr = service.ChangeMySQLUserPassword(r.Context(), mysqlExecutor, target, item.Username, item.Host, decodeDatabaseSecret(strField(b, "password"))); targetErr != nil {
		dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(targetErr))
		return
	}
	if id > 0 {
		e = databaseAdmin.SetPassword(r.Context(), item.ID)
	} else {
		e = databaseAdmin.SetPasswordByIdentity(r.Context(), item.Database, item.Username, item.Host)
	}
	if e != nil {
		dbAdminError(w, 404, e)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"id": id, "passwordSet": true}})
}

// dbGrantsSearch 查询数据库用户授权记录。
func dbGrantsSearch(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	items := databaseAdmin.ListGrants(r.Context(), strField(b, "database"), strField(b, "username"))
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
}

// dbGrantsSummary 按数据库名称汇总授权记录。
func dbGrantsSummary(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	items := databaseAdmin.ListGrants(r.Context(), strField(b, "database"), "")
	summary := map[string][]service.DatabaseGrant{}
	for _, g := range items {
		summary[g.Database] = append(summary[g.Database], g)
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": summary})
}

// dbGrantsCreate 新增或更新数据库授权记录。
func dbGrantsCreate(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	database := strField(b, "database")
	if database == "" {
		database = strField(b, "db")
	}
	target, targetErr := mysqlTarget(r.Context(), database, strField(b, "type"))
	if targetErr != nil {
		if sharedDB() == nil {
			g := service.DatabaseGrant{Server: database, Database: func() string {
				if v := strField(b, "db"); v != "" {
					return v
				}
				return database
			}(), Username: strField(b, "username"), Host: strField(b, "host"), Privileges: []string{"SELECT"}}
			if item, storeErr := databaseAdmin.UpsertGrant(r.Context(), g); storeErr == nil {
				wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
				return
			}
		}
		dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(targetErr))
		return
	}
	grantDatabase := strField(b, "db")
	if grantDatabase == "" {
		grantDatabase = database
	}
	g := service.DatabaseGrant{ID: intField(b, "id"), Server: database, Database: grantDatabase, Username: strField(b, "username"), Host: strField(b, "host")}
	if p, ok := b["privileges"].([]any); ok {
		for _, v := range p {
			if s, ok := v.(string); ok {
				g.Privileges = append(g.Privileges, s)
			}
		}
	}
	if len(g.Privileges) == 0 {
		g.Privileges = []string{"SELECT"}
	}
	if e = service.GrantMySQLUser(r.Context(), mysqlExecutor, target, g.Database, g.Username, g.Host); e != nil {
		dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(e))
		return
	}
	item, e := databaseAdmin.UpsertGrant(r.Context(), g)
	if e != nil {
		_ = service.RevokeMySQLGrant(r.Context(), mysqlExecutor, target, g.Database, g.Username, g.Host)
		dbAdminError(w, 400, e)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}

// dbGrantsDelete 删除指定数据库授权记录。
func dbGrantsDelete(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	id := intField(b, "id")
	server := strField(b, "database")
	database := strField(b, "database")
	if strField(b, "db") != "" {
		database = strField(b, "db")
	}
	target, targetErr := mysqlTarget(r.Context(), server, strField(b, "type"))
	if targetErr != nil {
		dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(targetErr))
		return
	}
	if targetErr = service.RevokeMySQLGrant(r.Context(), mysqlExecutor, target, database, strField(b, "username"), strField(b, "host")); targetErr != nil {
		dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(targetErr))
		return
	}
	if id > 0 {
		e = databaseAdmin.DeleteGrant(r.Context(), id)
	} else {
		e = databaseAdmin.DeleteGrantByIdentity(r.Context(), server, database, strField(b, "username"), strField(b, "host"))
	}
	if e != nil {
		_ = service.GrantMySQLUser(r.Context(), mysqlExecutor, target, database, strField(b, "username"), strField(b, "host"))
		dbAdminError(w, 404, e)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"deleted": id}})
}

// dbDescription 更新数据库实例的描述和连接信息。
func dbDescription(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	id := intField(b, "id")
	description := strField(b, "description")
	typ := strField(b, "type")
	if typ == "" {
		typ = "mysql"
	}
	port := int(intField(b, "port"))
	if port == 0 {
		port = databasePort(typ, 0)
	}
	current, ok := databaseService.Find(r.Context(), id)
	if !ok {
		dbAdminError(w, 404, errors.New("数据库不存在"))
		return
	}
	name := strField(b, "name")
	if name == "" {
		name = current.Name
	}
	host := strField(b, "host")
	if host == "" {
		host = current.Host
	}
	host, containerName := normalizedDatabaseTarget(current.From, host, current.ContainerName)
	item, e := databaseService.Update(r.Context(), service.Database{ID: id, Name: name, Type: typ, Version: current.Version, From: databaseSourceForTarget(current.From, containerName), Host: host, Port: port, ContainerName: containerName, InitialDB: current.InitialDB, Username: current.Username, SSL: current.SSL, Description: description})
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}

// dbVariables 查询数据库变量配置。
func dbVariables(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	database := strField(b, "database")
	if target, err := mysqlTarget(r.Context(), database, strField(b, "type")); err == nil {
		if queryExecutor, ok := mysqlExecutor.(service.MySQLQueryExecutor); ok {
			if items, queryErr := service.ListMySQLVariables(r.Context(), queryExecutor, target); queryErr == nil {
				wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
				return
			}
		}
	}
	items := databaseAdmin.Variables(r.Context(), database)
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
}

// dbVariablesUpdate 更新数据库变量并持久化修改。
func dbVariablesUpdate(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	database := strField(b, "database")
	if target, targetErr := mysqlTarget(r.Context(), database, strField(b, "type")); targetErr == nil {
		updates := b["variables"]
		entries, ok := updates.([]any)
		if !ok {
			entries = []any{b}
		}
		queryExecutor, queryOK := mysqlExecutor.(service.MySQLQueryExecutor)
		oldValues := map[string]string{}
		if queryOK {
			oldValues, _ = service.ListMySQLVariables(r.Context(), queryExecutor, target)
		}
		applied := make([]string, 0, len(entries))
		for _, entry := range entries {
			fields, fieldsOK := entry.(map[string]any)
			if !fieldsOK {
				dbAdminError(w, http.StatusBadRequest, errors.New("变量项格式无效"))
				return
			}
			name := strField(fields, "name")
			if name == "" {
				name = strField(fields, "param")
			}
			value := strField(fields, "value")
			if value == "" {
				value = fmt.Sprint(fields["value"])
			}
			if e = service.SetMySQLVariable(r.Context(), mysqlExecutor, target, name, value); e != nil {
				for index := len(applied) - 1; index >= 0; index-- {
					if old, exists := oldValues[applied[index]]; exists {
						_ = service.SetMySQLVariable(r.Context(), mysqlExecutor, target, applied[index], old)
					}
				}
				dbAdminError(w, http.StatusServiceUnavailable, e)
				return
			}
			applied = append(applied, name)
			_, _ = databaseAdmin.SetVariable(r.Context(), service.DatabaseVariable{Database: database, Name: name, Value: value})
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"updated": true}})
		return
	}
	// 原版变量页一次提交 variables[{param,value}]，逐项持久化并返回全部结果。
	if raw, ok := b["variables"].([]any); ok {
		items := make([]service.DatabaseVariable, 0, len(raw))
		for _, entry := range raw {
			fields, ok := entry.(map[string]any)
			if !ok {
				dbAdminError(w, 400, errors.New("变量项格式无效"))
				return
			}
			name := strField(fields, "name")
			if name == "" {
				name = strField(fields, "param")
			}
			value := strField(fields, "value")
			item, err := databaseAdmin.SetVariable(r.Context(), service.DatabaseVariable{Database: database, Name: name, Value: value})
			if err != nil {
				dbAdminError(w, 400, err)
				return
			}
			items = append(items, item)
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
		return
	}
	v := service.DatabaseVariable{Database: database, Name: strField(b, "name"), Value: strField(b, "value")}
	item, e := databaseAdmin.SetVariable(r.Context(), v)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}

// dbChangeAccess 修改真实 MySQL/MariaDB root 的远程访问权限。
func dbChangeAccess(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		dbAdminError(w, http.StatusBadRequest, err)
		return
	}
	database := strField(b, "database")
	target, targetErr := mysqlTarget(r.Context(), database, strField(b, "type"))
	if targetErr != nil {
		dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(targetErr))
		return
	}
	host := strField(b, "value")
	if err = service.ChangeMySQLRootAccess(r.Context(), mysqlExecutor, target, host); err != nil {
		dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(err))
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"database": database, "access": host}})
}

// dbCommonInfo 查询数据库通用配置文件信息。
func dbCommonInfo(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	typ := strField(b, "type")
	if typ == "" {
		typ = "mysql"
	}
	name := strField(b, "name")
	if name == "" {
		name = strField(b, "database")
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": loadDatabaseBaseInfo(r.Context(), typ, name)})
}

// dbCommonFile 读取数据库通用配置文件内容。
func dbCommonFile(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	typ := strField(b, "type")
	name := strField(b, "name")
	if name == "" {
		name = strField(b, "database")
	}
	if !knownDatabaseConf(typ) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": databaseAdmin.Config(r.Context(), name)})
		return
	}
	version := ""
	if install, ok := findDatabaseInstall(nil, strings.TrimSuffix(typ, "-conf"), name); ok {
		version = install.Version
	}
	path, err := databaseConfPath(typ, name, version)
	if err != nil {
		dbAdminError(w, http.StatusNotFound, errors.New("数据库配置文件不存在"))
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		dbAdminError(w, http.StatusNotFound, errors.New("数据库配置文件不存在"))
		return
	}
	if len(content) > 1<<20 {
		dbAdminError(w, http.StatusBadRequest, errors.New("数据库配置文件超过 1MB"))
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": string(content)})
}

// dbCommonUpdate 写入数据库通用配置文件内容。
func dbCommonUpdate(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	content := databaseConfigText(b)
	if strings.TrimSpace(content) == "" {
		dbAdminError(w, http.StatusBadRequest, errors.New("数据库配置不能为空"))
		return
	}
	if len(content) > 1<<20 {
		dbAdminError(w, http.StatusBadRequest, errors.New("数据库配置文件超过 1MB"))
		return
	}
	typ := strField(b, "type")
	name := strField(b, "database")
	if name == "" {
		name = strField(b, "name")
	}
	if !knownDatabaseConf(typ) {
		if e = databaseAdmin.SetConfig(r.Context(), name, content); e != nil {
			dbAdminError(w, 400, e)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"saved": true}})
		return
	}
	version := ""
	var install appRecord
	if found, ok := findDatabaseInstall(nil, typ, name); ok {
		install, version = found, found.Version
	}
	path, pathErr := databaseConfPath(typ, name, version)
	if pathErr != nil && !errors.Is(pathErr, os.ErrNotExist) {
		dbAdminError(w, http.StatusBadRequest, pathErr)
		return
	}
	if path == "" {
		dbAdminError(w, http.StatusNotFound, errors.New("数据库配置文件不存在"))
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		dbAdminError(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		dbAdminError(w, http.StatusInternalServerError, err)
		return
	}
	if install.ID != "" {
		if err := restartDatabaseCompose(install); err != nil {
			dbAdminError(w, http.StatusBadGateway, err)
			return
		}
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"saved": true}})
}

// dbFormatOptions 返回数据库类型对应的格式化选项。
func dbFormatOptions(w http.ResponseWriter, r *http.Request) {
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": []map[string]string{{"charset": "utf8mb4", "collation": "utf8mb4_unicode_ci"}, {"charset": "utf8mb4", "collation": "utf8mb4_general_ci"}, {"charset": "latin1", "collation": "latin1_swedish_ci"}}})
}

// dbStatus 查询数据库实例当前状态。
func dbStatus(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	typ, name := strField(b, "type"), strField(b, "name")
	host, port := strField(b, "host"), int(intField(b, "port"))
	if host == "" && name != "" {
		if item, found := databaseService.FindByName(r.Context(), typ, name); found {
			host, port = item.Host, item.Port
		}
	}
	if typ == "mysql" || typ == "mariadb" || typ == "" {
		if name != "" {
			if target, targetErr := mysqlTarget(r.Context(), name, typ); targetErr == nil {
				err := mysqlExecutor.Exec(r.Context(), target, "SELECT 1;")
				wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"type": mysqlTypeForTarget(target), "name": name, "available": err == nil, "message": errorString(err), "Run": err == nil}})
				return
			}
		}
	}
	ok, msg := checkDatabaseEndpoint(host, typ, port, 0)
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"type": typ, "name": name, "available": ok, "message": msg, "Run": ok}})
}

// dbRemote 返回远程数据库连接配置和可用性信息。
func dbRemote(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	typ, name := strField(b, "type"), strField(b, "name")
	host, port := strField(b, "host"), int(intField(b, "port"))
	if host == "" && name != "" {
		if item, found := databaseService.FindByName(r.Context(), typ, name); found {
			host, port = item.Host, item.Port
		}
	}
	if typ == "mysql" || typ == "mariadb" || typ == "" {
		if name == "" {
			dbAdminError(w, http.StatusBadRequest, errors.New("数据库实例不能为空"))
			return
		}
		target, targetErr := mysqlTarget(r.Context(), name, typ)
		if targetErr != nil {
			dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(targetErr))
			return
		}
		if targetErr = mysqlExecutor.Exec(r.Context(), target, "SELECT 1;"); targetErr != nil {
			dbAdminError(w, http.StatusServiceUnavailable, formatMySQLAdminError(targetErr))
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": true})
		return
	}
	ok, msg := checkDatabaseEndpoint(host, typ, port, 0)
	if !ok {
		wmhttp.JSON(w, 503, map[string]any{"code": "ERR", "message": msg})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"remote": true}})
}
