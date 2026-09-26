// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// canonicalDatabaseAppType 把应用 key 归一成数据库实例类型；非数据库应用返回空字符串。
func canonicalDatabaseAppType(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "postgres", "postgresql":
		return "postgresql"
	case "postgresql-cluster":
		return "postgresql-cluster"
	case "redis":
		return "redis"
	case "redis-cluster":
		return "redis-cluster"
	case "mysql":
		return "mysql"
	case "mariadb":
		return "mariadb"
	case "mysql-cluster":
		return "mysql-cluster"
	case "mongo", "mongodb":
		return "mongodb"
	default:
		return ""
	}
}

// databaseTypeFamily 返回同一产品族的类型别名，列表请求里的逗号类型会展开到这里。
func databaseTypeFamily(key string) []string {
	switch canonicalDatabaseAppType(key) {
	case "postgresql", "postgresql-cluster":
		return []string{"postgresql", "postgres", "postgresql-cluster"}
	case "redis", "redis-cluster":
		return []string{"redis", "redis-cluster"}
	case "mysql", "mariadb", "mysql-cluster":
		return []string{"mysql", "mariadb", "mysql-cluster"}
	case "mongodb":
		return []string{"mongodb", "mongo"}
	default:
		return nil
	}
}

// expandDatabaseListTypes 按逗号拆开列表类型，并补上同族别名。
func expandDatabaseListTypes(raw string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	add := func(typ string) {
		typ = strings.ToLower(strings.TrimSpace(typ))
		if typ == "" {
			return
		}
		if _, ok := seen[typ]; ok {
			return
		}
		seen[typ] = struct{}{}
		out = append(out, typ)
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if family := databaseTypeFamily(part); len(family) > 0 {
			for _, typ := range family {
				add(typ)
			}
			continue
		}
		add(part)
	}
	return out
}

func databasesForTypes(ctx context.Context, types []string) []service.Database {
	rows := make([]service.Database, 0)
	for _, typ := range types {
		rows = append(rows, databaseService.Search(ctx, typ, "")...)
	}
	return rows
}

// databaseRowIsInstance 判断登记行是连接实例还是实例内的库。
// 实例内库的 initial_db 指向同族另一条记录的名称；远程实例自己的 initial_db 只是连接库名。
func databaseRowIsInstance(row service.Database, rows []service.Database) bool {
	parent := strings.ToLower(strings.TrimSpace(row.InitialDB))
	if parent == "" || parent == strings.ToLower(row.Name) {
		return true
	}
	for _, candidate := range rows {
		if candidate.ID == row.ID {
			continue
		}
		if strings.EqualFold(candidate.Name, parent) {
			return false
		}
	}
	return true
}

func databaseInstances(rows []service.Database) []service.Database {
	out := make([]service.Database, 0)
	for _, row := range rows {
		if databaseRowIsInstance(row, rows) {
			out = append(out, row)
		}
	}
	return out
}

func databaseChildren(rows []service.Database, server, info string) []service.Database {
	server = strings.TrimSpace(server)
	info = strings.ToLower(strings.TrimSpace(info))
	out := make([]service.Database, 0)
	for _, row := range rows {
		if databaseRowIsInstance(row, rows) {
			continue
		}
		if server != "" && !strings.EqualFold(row.InitialDB, server) {
			continue
		}
		if info != "" && !strings.Contains(strings.ToLower(row.Name), info) {
			continue
		}
		out = append(out, row)
	}
	return out
}

func pageDatabaseSlice[T any](items []T, page, pageSize int) ([]T, int, int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}
	start := (page - 1) * pageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], len(items), page, pageSize
}

func databaseListOptions(ctx context.Context, typeParam string) []map[string]any {
	syncInstalledDatabaseServers(ctx)
	types := expandDatabaseListTypes(typeParam)
	options := make([]map[string]any, 0)
	for _, item := range databaseInstances(databasesForTypes(ctx, types)) {
		from := item.From
		if from == "" {
			from = "local"
		}
		options = append(options, map[string]any{
			"id":       item.ID,
			"from":     from,
			"type":     item.Type,
			"database": item.Name,
			"version":  item.Version,
			"address":  item.Host,
		})
	}
	return options
}

func databaseItemOptions(ctx context.Context, typeParam string) []map[string]any {
	syncInstalledDatabaseServers(ctx)
	types := expandDatabaseListTypes(typeParam)
	syncLiveDatabaseItems(ctx, databasesForTypes(ctx, types))
	items := make([]map[string]any, 0)
	for _, item := range databaseChildren(databasesForTypes(ctx, types), "", "") {
		from := item.From
		if from == "" {
			from = "local"
		}
		items = append(items, map[string]any{
			"id":       item.ID,
			"from":     from,
			"database": item.InitialDB,
			"name":     item.Name,
		})
	}
	return items
}

// syncLiveDatabaseItems 把每个已登记实例里的真实库补进面板，计划任务才能看到未手工登记的库。
func syncLiveDatabaseItems(ctx context.Context, rows []service.Database) {
	for _, instance := range databaseInstances(rows) {
		switch {
		case strings.Contains(instance.Type, "postgres"):
			syncPostgresLiveDatabases(ctx, instance.Name)
		case strings.Contains(instance.Type, "mysql") || instance.Type == "mariadb":
			syncMySQLLiveDatabases(ctx, instance.Name)
		case strings.Contains(instance.Type, "mongo"):
			syncMongoLiveDatabases(ctx, instance.Name)
		}
	}
}

func handleDatabaseServerList(w http.ResponseWriter, r *http.Request, typeParam string) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": databaseListOptions(r.Context(), typeParam)})
}

func handleDatabaseItemList(w http.ResponseWriter, r *http.Request, typeParam string) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": databaseItemOptions(r.Context(), typeParam)})
}

func handleMySQLDatabaseSearch(w http.ResponseWriter, r *http.Request) {
	body, err := readDatabaseBody(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	server := strField(body, "database")
	types := databaseTypeFamily("mysql")
	if typ := strField(body, "type"); typ != "" {
		if family := databaseTypeFamily(typ); len(family) > 0 {
			types = family
		}
	}
	live := syncMySQLLiveDatabases(r.Context(), server)
	children := visibleChildDatabases(databasesForTypes(r.Context(), types), server, strField(body, "info"), live)
	page := int(intField(body, "page"))
	pageSize := int(intField(body, "pageSize"))
	paged, total, page, pageSize := pageDatabaseSlice(children, page, pageSize)
	items := make([]map[string]any, 0, len(paged))
	for _, item := range paged {
		items = append(items, map[string]any{
			"id": item.ID, "createdAt": item.CreatedAt, "name": item.Name, "mysqlName": item.InitialDB,
			"from": item.From, "format": "", "username": item.Username, "password": item.Password,
			"permission": "", "isDelete": false, "description": item.Description,
		})
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize}})
}

func syncInstalledDatabaseServers(ctx context.Context) {
	for _, install := range installedDatabaseApps(ctx) {
		registerInstalledDatabaseServer(ctx, install)
	}
	discoverInstalledDatabaseContainers(ctx)
}

func registerInstalledDatabaseServer(ctx context.Context, install appRecord) {
	typ := canonicalDatabaseAppType(install.Key)
	name := strings.TrimSpace(install.Name)
	if typ == "" || name == "" || !appStatusCountsAsInstalled(install.Status) {
		return
	}
	rows := databasesForTypes(ctx, databaseTypeFamily(install.Key))
	for _, row := range rows {
		if strings.EqualFold(row.Name, name) && databaseRowIsInstance(row, rows) {
			return
		}
	}
	container := ""
	if names := appContainerNames(install); len(names) > 0 {
		container = names[0]
	}
	host, containerName := normalizedDatabaseTarget("local", "", container)
	username, password := configuredDatabaseAuth(install, typ)
	_, _ = databaseService.Create(ctx, service.Database{
		Name: name, Type: typ, Version: install.Version, From: "local",
		Host: host, Port: configuredDatabasePort(install, typ), ContainerName: containerName,
		Username: username, Password: password,
	})
}

// removeInstalledDatabaseServer 卸载数据库应用时删除本机实例及其库内登记，不删除同名远程连接。
func removeInstalledDatabaseServer(ctx context.Context, install appRecord) {
	typ := canonicalDatabaseAppType(install.Key)
	name := strings.TrimSpace(install.Name)
	if typ == "" || name == "" {
		return
	}
	rows := databasesForTypes(ctx, databaseTypeFamily(install.Key))
	var instanceID int64
	for _, row := range rows {
		if strings.EqualFold(row.Name, name) && strings.EqualFold(row.From, "local") && databaseRowIsInstance(row, rows) {
			instanceID = row.ID
			break
		}
	}
	for _, row := range rows {
		if row.ID == instanceID {
			continue
		}
		if strings.EqualFold(row.InitialDB, name) {
			_ = databaseService.Delete(ctx, row.ID)
		}
	}
	if instanceID > 0 {
		_ = databaseService.Delete(ctx, instanceID)
	}
	if install.ID != "" {
		if repository, err := SharedRepository(); err == nil && repository != nil {
			_, _ = repository.ExecContext(ctx, `DELETE FROM app_installs WHERE id=?`, install.ID)
		}
	}
}

func installedDatabaseApps(ctx context.Context) []appRecord {
	repository, err := SharedRepository()
	if err != nil || repository == nil {
		return nil
	}
	rows, err := repository.QueryContext(ctx, `SELECT id, app_key, name, version, status, container_names, config_json FROM app_installs`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	seen := map[string]struct{}{}
	out := make([]appRecord, 0)
	for rows.Next() {
		var item appRecord
		var config []byte
		if err := rows.Scan(&item.ID, &item.Key, &item.Name, &item.Version, &item.Status, &item.ContainerName, &config); err != nil {
			continue
		}
		if len(config) > 0 {
			_ = json.Unmarshal(config, &item.Config)
		}
		key := strings.ToLower(item.Key) + "\x00" + strings.ToLower(item.Name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func configuredDatabasePort(install appRecord, typ string) int {
	port := appConfiguredInt(install.Config, 0, "PANEL_APP_PORT_HTTP", "httpPort", "port")
	if port == 0 {
		if params, ok := install.Config["params"].(map[string]any); ok {
			port = appConfiguredInt(params, 0, "PANEL_APP_PORT_HTTP", "port")
		}
	}
	if port == 0 {
		port = databasePort(typ, 0)
	}
	return port
}

func configuredDatabaseAuth(install appRecord, typ string) (string, string) {
	username := appInstallValue(install, "PANEL_DB_ROOT_USER", "username", "user")
	password := appInstallValue(install, "PANEL_DB_ROOT_PASSWORD", "password")
	if canonicalDatabaseAppType(typ) == "redis" || canonicalDatabaseAppType(typ) == "redis-cluster" {
		if value := appInstallValue(install, "PANEL_REDIS_ROOT_PASSWORD"); value != "" {
			password = value
		}
		return "", password
	}
	if username == "" {
		switch canonicalDatabaseAppType(typ) {
		case "postgresql", "postgresql-cluster":
			username = "postgres"
		default:
			username = "root"
		}
	}
	return username, password
}

func appInstallValue(install appRecord, keys ...string) string {
	if value := appValue(install.Config, keys...); value != "" {
		return value
	}
	if params, ok := install.Config["params"].(map[string]any); ok {
		return appValue(params, keys...)
	}
	return ""
}
