// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

func mongoResource(item service.Database) map[string]any {
	return map[string]any{"id": item.ID, "createdAt": item.CreatedAt, "name": item.Name, "mongodbName": item.Name, "from": item.From, "username": item.Username, "password": item.Password, "isDelete": false, "description": item.Description}
}

func mongoDatabaseForRequest(ctx context.Context, server, database string) (service.Database, bool) {
	for _, item := range databaseService.Search(ctx, "mongodb", "") {
		if strings.EqualFold(item.InitialDB, server) && strings.EqualFold(item.Name, database) {
			return item, true
		}
	}
	return service.Database{}, false
}

func mongoTargetForRequest(ctx context.Context, name string) (service.MongoDBTarget, bool) {
	name = strings.TrimSpace(name)
	for _, typ := range []string{"mongodb", "mongo"} {
		if item, ok := databaseService.FindByName(ctx, typ, name); ok {
			host, containerName := normalizedDatabaseItemTarget(item)
			target := service.MongoDBTarget{Type: item.Type, Host: host, Port: item.Port, Username: item.Username, Password: item.Password, AuthDatabase: item.InitialDB, ContainerName: containerName}
			if target.AuthDatabase == "" {
				target.AuthDatabase = "admin"
			}
			return target, true
		}
	}
	return service.MongoDBTarget{}, false
}

func handleMongoPrivileges(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		dbAdminError(w, http.StatusBadRequest, err)
		return
	}
	server := strField(b, "database")
	database := strField(b, "name")
	username := strField(b, "username")
	target, ok := mongoTargetForRequest(r.Context(), server)
	if !ok {
		dbAdminError(w, http.StatusServiceUnavailable, errors.New("目标 MongoDB 实例未登记"))
		return
	}
	script, err := service.MongoPrivilegesScript(database, username)
	if err != nil {
		dbAdminError(w, http.StatusBadRequest, err)
		return
	}
	output, err := mongoExecutor.Exec(r.Context(), target, script)
	if err != nil {
		dbAdminError(w, http.StatusServiceUnavailable, err)
		return
	}
	role := strings.TrimSpace(strings.Split(output, "\n")[0])
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": role})
}

func handleMongoDeleteCheck(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		dbAdminError(w, http.StatusBadRequest, err)
		return
	}
	server := strField(b, "database")
	target, ok := mongoTargetForRequest(r.Context(), server)
	if !ok {
		dbAdminError(w, http.StatusServiceUnavailable, errors.New("目标 MongoDB 实例未登记"))
		return
	}
	if _, err := mongoExecutor.Exec(r.Context(), target, "db.adminCommand({ping:1});"); err != nil {
		dbAdminError(w, http.StatusServiceUnavailable, err)
		return
	}
	dependencies := []map[string]any{}
	if repository, repositoryErr := SharedRepository(); repositoryErr == nil {
		rows, queryErr := repository.QueryContext(r.Context(), `SELECT id,primary_domain,type FROM websites WHERE db_id=? ORDER BY id`, intField(b, "id"))
		if queryErr == nil {
			defer rows.Close()
			for rows.Next() {
				var id int64
				var domain, typ string
				if rows.Scan(&id, &domain, &typ) == nil {
					dependencies = append(dependencies, map[string]any{"id": id, "name": domain, "type": typ, "resource": "website"})
				}
			}
		}
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dependencies})
}

func handleMongoPrivilegesChange(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		dbAdminError(w, http.StatusBadRequest, err)
		return
	}
	server, database, username := strField(b, "database"), strField(b, "name"), strField(b, "username")
	target, ok := mongoTargetForRequest(r.Context(), server)
	if !ok {
		dbAdminError(w, http.StatusServiceUnavailable, errors.New("目标 MongoDB 实例未登记"))
		return
	}
	if err = service.ChangeMongoPrivileges(r.Context(), mongoExecutor, target, database, username, strField(b, "permission")); err != nil {
		dbAdminError(w, http.StatusServiceUnavailable, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"username": username, "permission": strField(b, "permission")}})
}

func handleMongoCreate(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		dbAdminError(w, http.StatusBadRequest, err)
		return
	}
	server, database, username := strField(b, "database"), strField(b, "name"), strField(b, "username")
	password, role := decodeDatabaseSecret(strField(b, "password")), strField(b, "permission")
	target, ok := mongoTargetForRequest(r.Context(), server)
	if !ok {
		dbAdminError(w, http.StatusServiceUnavailable, errors.New("目标 MongoDB 实例未登记"))
		return
	}
	if err = service.CreateMongoDatabase(r.Context(), mongoExecutor, target, database, username, password, role); err != nil {
		dbAdminError(w, http.StatusServiceUnavailable, err)
		return
	}
	item, err := databaseService.Create(r.Context(), service.Database{Name: database, Type: "mongodb", From: strField(b, "from"), Host: target.Host, Port: target.Port, InitialDB: server, Username: username, Password: password, Description: strField(b, "description")})
	if err != nil {
		_ = service.DropMongoDatabase(r.Context(), mongoExecutor, target, database, "")
		dbAdminError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": mongoResource(item)})
}

func handleMongoSearch(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		dbAdminError(w, http.StatusBadRequest, err)
		return
	}
	server := strField(b, "database")
	live := syncMongoLiveDatabases(r.Context(), server)
	children := visibleChildDatabases(databasesForTypes(r.Context(), databaseTypeFamily("mongodb")), server, strField(b, "info"), live)
	paged, total, page, pageSize := pageDatabaseSlice(children, int(intField(b, "page")), int(intField(b, "pageSize")))
	items := make([]map[string]any, 0, len(paged))
	for _, item := range paged {
		items = append(items, mongoResource(item))
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize}})
}

// handleMongoLoadRemote 从真实实例同步非系统数据库；远端只读，新增的
// SQLite 记录在任一持久化步骤失败时逆序清理，避免留下半同步状态。
func handleMongoLoadRemote(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		dbAdminError(w, http.StatusBadRequest, err)
		return
	}
	server := strField(b, "database")
	if server == "" {
		dbAdminError(w, http.StatusBadRequest, errors.New("MongoDB 实例不能为空"))
		return
	}
	target, ok := mongoTargetForRequest(r.Context(), server)
	if !ok {
		dbAdminError(w, http.StatusServiceUnavailable, errors.New("目标 MongoDB 实例未登记"))
		return
	}
	names, err := service.ListMongoDatabases(r.Context(), mongoExecutor, target)
	if err != nil {
		dbAdminError(w, http.StatusServiceUnavailable, err)
		return
	}
	existing := make(map[string]struct{})
	for _, item := range databaseService.Search(r.Context(), "mongodb", "") {
		if strings.EqualFold(item.InitialDB, server) {
			existing[strings.ToLower(item.Name)] = struct{}{}
		}
	}
	created := make([]service.Database, 0)
	from := strField(b, "from")
	for _, name := range names {
		if _, exists := existing[strings.ToLower(name)]; exists {
			continue
		}
		item, createErr := databaseService.Create(r.Context(), service.Database{Name: name, Type: "mongodb", From: from, Host: target.Host, Port: target.Port, InitialDB: server})
		if createErr != nil {
			if _, exists := databaseService.FindByName(r.Context(), "mongodb", name); exists {
				continue
			}
			for index := len(created) - 1; index >= 0; index-- {
				_ = databaseService.Delete(r.Context(), created[index].ID)
			}
			dbAdminError(w, http.StatusInternalServerError, createErr)
			return
		}
		created = append(created, item)
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"synced": len(names), "created": len(created)}})
}

func handleMongoBind(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		dbAdminError(w, http.StatusBadRequest, err)
		return
	}
	server, database, username := strField(b, "database"), strField(b, "name"), strField(b, "username")
	item, ok := mongoDatabaseForRequest(r.Context(), server, database)
	if !ok {
		dbAdminError(w, http.StatusNotFound, errors.New("MongoDB 数据库不存在"))
		return
	}
	target, targetOK := mongoTargetForRequest(r.Context(), server)
	if !targetOK {
		dbAdminError(w, http.StatusServiceUnavailable, errors.New("目标 MongoDB 实例未登记"))
		return
	}
	password := decodeDatabaseSecret(strField(b, "password"))
	if err = service.BindMongoUser(r.Context(), mongoExecutor, target, database, username, password); err != nil {
		dbAdminError(w, http.StatusServiceUnavailable, err)
		return
	}
	item.Username, item.Password, item.UpdatedAt = username, password, time.Now().UTC()
	if _, err = databaseService.Update(r.Context(), item); err != nil {
		_ = service.DropMongoUser(r.Context(), mongoExecutor, target, database, username)
		dbAdminError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": mongoResource(item)})
}

func handleMongoPassword(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		dbAdminError(w, http.StatusBadRequest, err)
		return
	}
	server, database, username := strField(b, "database"), strField(b, "name"), strField(b, "username")
	item, ok := mongoDatabaseForRequest(r.Context(), server, database)
	if !ok {
		dbAdminError(w, http.StatusNotFound, errors.New("MongoDB 数据库不存在"))
		return
	}
	target, targetOK := mongoTargetForRequest(r.Context(), server)
	if !targetOK {
		dbAdminError(w, http.StatusServiceUnavailable, errors.New("目标 MongoDB 实例未登记"))
		return
	}
	password := decodeDatabaseSecret(strField(b, "password"))
	oldPassword := item.Password
	if err = service.ChangeMongoPassword(r.Context(), mongoExecutor, target, database, username, password); err != nil {
		dbAdminError(w, http.StatusServiceUnavailable, err)
		return
	}
	item.Password, item.UpdatedAt = password, time.Now().UTC()
	if _, err = databaseService.Update(r.Context(), item); err != nil {
		_ = service.ChangeMongoPassword(r.Context(), mongoExecutor, target, database, username, oldPassword)
		dbAdminError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"updated": true}})
}

func handleMongoRootPassword(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		dbAdminError(w, http.StatusBadRequest, err)
		return
	}
	server := strField(b, "database")
	serverItem, serverFound := databaseService.FindByName(r.Context(), "mongodb", server)
	if !serverFound {
		serverItem, serverFound = databaseService.FindByName(r.Context(), "mongo", server)
	}
	target, ok := mongoTargetForRequest(r.Context(), server)
	if !ok || !serverFound {
		dbAdminError(w, http.StatusServiceUnavailable, errors.New("目标 MongoDB 实例未登记"))
		return
	}
	username := strField(b, "username")
	if username == "" {
		username = target.Username
	}
	newPassword := decodeDatabaseSecret(strField(b, "value"))
	if err = service.ChangeMongoPassword(r.Context(), mongoExecutor, target, target.AuthDatabase, username, newPassword); err != nil {
		dbAdminError(w, http.StatusServiceUnavailable, err)
		return
	}
	serverItem.Password, serverItem.UpdatedAt = newPassword, time.Now().UTC()
	if _, err = databaseService.Update(r.Context(), serverItem); err != nil {
		rollbackTarget := target
		rollbackTarget.Password = newPassword
		_ = service.ChangeMongoPassword(r.Context(), mongoExecutor, rollbackTarget, target.AuthDatabase, username, target.Password)
		dbAdminError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"updated": true}})
}

func handleMongoDescription(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		dbAdminError(w, http.StatusBadRequest, err)
		return
	}
	item, ok := mongoDatabaseForRequest(r.Context(), strField(b, "database"), strField(b, "name"))
	if !ok {
		dbAdminError(w, http.StatusNotFound, errors.New("MongoDB 数据库不存在"))
		return
	}
	item.Description, item.UpdatedAt = strField(b, "description"), time.Now().UTC()
	updated, err := databaseService.Update(r.Context(), item)
	if err != nil {
		dbAdminError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": mongoResource(updated)})
}

func handleMongoDelete(w http.ResponseWriter, r *http.Request) {
	b, err := readDatabaseBody(r)
	if err != nil {
		dbAdminError(w, http.StatusBadRequest, err)
		return
	}
	server, database := strField(b, "database"), strField(b, "name")
	item, ok := mongoDatabaseForRequest(r.Context(), server, database)
	if !ok {
		dbAdminError(w, http.StatusNotFound, errors.New("MongoDB 数据库不存在"))
		return
	}
	target, targetOK := mongoTargetForRequest(r.Context(), server)
	if !targetOK {
		dbAdminError(w, http.StatusServiceUnavailable, errors.New("目标 MongoDB 实例未登记"))
		return
	}
	if err = service.DropMongoDatabase(r.Context(), mongoExecutor, target, database, item.Username); err != nil {
		dbAdminError(w, http.StatusServiceUnavailable, err)
		return
	}
	if err = databaseService.Delete(r.Context(), item.ID); err != nil {
		dbAdminError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"deleted": item.ID}})
}
