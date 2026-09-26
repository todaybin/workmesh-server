// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
)

// databaseKeysMatch 判断两个应用键是否属于同一数据库产品族。
func databaseKeysMatch(left, right string) bool {
	leftFamily, rightFamily := databaseTypeFamily(left), databaseTypeFamily(right)
	if len(leftFamily) == 0 || len(rightFamily) == 0 {
		return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
	}
	for _, item := range leftFamily {
		for _, candidate := range rightFamily {
			if item == candidate {
				return true
			}
		}
	}
	return false
}

func matchDatabaseInstall(items []appRecord, key, name string) (appRecord, bool) {
	key = strings.TrimSpace(key)
	name = strings.TrimSpace(name)
	var matched appRecord
	found := false
	for _, item := range items {
		if canonicalDatabaseAppType(item.Key) == "" {
			continue
		}
		if name != "" && item.ID != name && !strings.EqualFold(item.Name, name) {
			continue
		}
		if key != "" && !databaseKeysMatch(item.Key, key) && !strings.EqualFold(item.Key, key) && item.ID != key {
			continue
		}
		if name == "" && key == "" {
			continue
		}
		// 同名安装可能同时存在失败记录和正在运行的容器，优先使用运行中的实例。
		if !found || databaseInstallPreferred(item, matched) {
			matched = item
			found = true
		}
	}
	return matched, found
}

func databaseInstallPreferred(next, current appRecord) bool {
	nextRunning := strings.EqualFold(strings.TrimSpace(next.Status), "Running")
	currentRunning := strings.EqualFold(strings.TrimSpace(current.Status), "Running")
	if nextRunning != currentRunning {
		return nextRunning
	}
	return appConfiguredContainerName(next) != "" && appConfiguredContainerName(current) == ""
}

// findDatabaseInstall 按安装 ID、应用键和实例名查找数据库应用，内存未命中时回退到 app_installs。
func findDatabaseInstall(store *appStore, key, name string) (appRecord, bool) {
	if store != nil {
		store.mu.RLock()
		item, ok := matchDatabaseInstall(store.state.Apps, key, name)
		store.mu.RUnlock()
		if ok {
			return item, true
		}
	}
	for _, item := range installedDatabaseAppRecords(context.Background()) {
		if matched, ok := matchDatabaseInstall([]appRecord{item}, key, name); ok {
			return matched, true
		}
	}
	return appRecord{}, false
}

func installedDatabaseAppRecords(ctx context.Context) []appRecord {
	repository, err := SharedRepository()
	if err != nil || repository == nil {
		return nil
	}
	rows, err := repository.QueryContext(ctx, `SELECT id, app_key, name, version, status, install_path, compose_path, container_names, config_json FROM app_installs`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]appRecord, 0)
	for rows.Next() {
		var item appRecord
		var installPath, composePath string
		var config []byte
		if err := rows.Scan(&item.ID, &item.Key, &item.Name, &item.Version, &item.Status, &installPath, &composePath, &item.ContainerName, &config); err != nil {
			continue
		}
		if len(config) > 0 {
			_ = json.Unmarshal(config, &item.Config)
		}
		if item.Config == nil {
			item.Config = map[string]any{}
		}
		if installPath != "" {
			item.Config["installPath"] = installPath
		}
		if composePath != "" {
			item.Config["composePath"] = composePath
		}
		out = append(out, item)
	}
	return out
}

func ensureDatabaseInstallLoaded(store *appStore, key, name string) (appRecord, bool) {
	if store == nil {
		return findDatabaseInstall(nil, key, name)
	}
	store.mu.RLock()
	if item, ok := matchDatabaseInstall(store.state.Apps, key, name); ok {
		store.mu.RUnlock()
		return item, true
	}
	store.mu.RUnlock()
	item, ok := findDatabaseInstall(nil, key, name)
	if !ok {
		return appRecord{}, false
	}
	store.mu.Lock()
	if _, exists := matchDatabaseInstall(store.state.Apps, item.Key, item.Name); !exists {
		store.state.Apps = append(store.state.Apps, item)
	}
	store.mu.Unlock()
	return item, true
}

func respondInstalledDatabase(w http.ResponseWriter, item appRecord) {
	status := normalizeAppStatus(item.Status)
	port := configuredDatabasePort(item, item.Key)
	appOK(w, map[string]any{
		"name": item.Name, "version": item.Version, "isExist": true,
		"isActive": status == "Running", "status": status, "app": item.Key,
		"appInstallId": item.ID, "containerName": appConfiguredContainerName(item),
		"httpPort": port, "httpsPort": 0, "installPath": appInstallPath(item),
	})
}

func databaseInstallDir(item appRecord) string {
	if path := appInstallValue(item, "installPath"); path != "" {
		return path
	}
	return appInstallPath(item)
}

func databaseComposeFile(item appRecord) string {
	if path := appInstallValue(item, "composePath"); path != "" {
		return path
	}
	return filepath.Join(databaseInstallDir(item), "docker-compose.yml")
}

func knownDatabaseConf(typ string) bool {
	switch strings.TrimSuffix(strings.ToLower(strings.TrimSpace(typ)), "-conf") {
	case "postgresql", "postgresql-cluster", "mysql", "mariadb", "mysql-cluster", "redis", "redis-cluster":
		return true
	default:
		return false
	}
}

// databaseConfPath 按 1Panel 的安装目录约定定位数据库配置文件。
func databaseConfPath(typ, name, version string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name != filepath.Base(name) || strings.Contains(name, "..") {
		return "", errors.New("数据库名称无效")
	}
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	key := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(typ)), "-conf")
	var relatives []string
	switch key {
	case "postgresql":
		if strings.HasPrefix(version, "18.") || version == "18" {
			relatives = append(relatives, filepath.Join("data", "18", "docker", "postgresql.conf"))
		}
		relatives = append(relatives, filepath.Join("data", "postgresql.conf"))
	case "postgresql-cluster":
		relatives = append(relatives, filepath.Join("data", "postgresql.conf"))
	case "mysql", "mariadb", "mysql-cluster":
		relatives = append(relatives, filepath.Join("conf", "my.cnf"))
	case "redis", "redis-cluster":
		relatives = append(relatives, filepath.Join("conf", "redis.conf"))
	default:
		return "", errors.New("不支持该数据库配置")
	}
	for _, relative := range relatives {
		path := filepath.Join(root, "apps", key, name, relative)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path, nil
		}
	}
	if len(relatives) == 0 {
		return "", os.ErrNotExist
	}
	return filepath.Join(root, "apps", key, name, relatives[len(relatives)-1]), os.ErrNotExist
}

func databaseConfigText(body map[string]any) string {
	if content := strField(body, "content"); content != "" {
		return content
	}
	file := strField(body, "file")
	if file == "" {
		return ""
	}
	if strings.Contains(file, "\n") || strings.Contains(file, "#") {
		return file
	}
	if raw, err := base64.StdEncoding.DecodeString(file); err == nil && len(raw) > 0 {
		return string(raw)
	}
	return file
}

func restartDatabaseCompose(item appRecord) error {
	compose := databaseComposeFile(item)
	if _, err := os.Stat(compose); err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := (service.CommandService{}).Execute(ctx, model.CommandRequest{
		Program: service.DockerBinary(), Args: []string{"compose", "-f", compose, "up", "-d"},
		Dir: filepath.Dir(compose), Timeout: 2 * time.Minute,
	})
	if err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" && err != nil {
			message = err.Error()
		}
		if message == "" {
			message = "重启数据库容器失败"
		}
		return errors.New(message)
	}
	return nil
}

func updateDatabaseInstancePort(ctx context.Context, key, name string, port int) {
	rows := databasesForTypes(ctx, databaseTypeFamily(key))
	for _, row := range databaseInstances(rows) {
		if !strings.EqualFold(row.Name, name) {
			continue
		}
		row.Port = port
		if strings.TrimSpace(row.Host) == "" {
			row.Host = "127.0.0.1"
		}
		_, _ = databaseService.Update(ctx, row)
	}
}

func writeDatabaseEnvPort(item appRecord, port int) error {
	compose := databaseComposeFile(item)
	envPath := filepath.Join(filepath.Dir(compose), ".env")
	content, err := os.ReadFile(envPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	line := "PANEL_APP_PORT_HTTP=" + strconv.Itoa(port)
	lines := strings.Split(string(content), "\n")
	found := false
	for index, current := range lines {
		if strings.HasPrefix(strings.TrimSpace(current), "PANEL_APP_PORT_HTTP=") {
			lines[index] = line
			found = true
		}
	}
	if !found {
		if len(lines) == 1 && lines[0] == "" {
			lines = []string{line}
		} else {
			lines = append(lines, line)
		}
	}
	if err := os.MkdirAll(filepath.Dir(envPath), 0o750); err != nil {
		return err
	}
	return os.WriteFile(envPath, []byte(strings.Join(lines, "\n")), 0o600)
}

func databasePublicInfo(item service.Database) map[string]any {
	from := item.From
	if from == "" {
		from = "local"
	}
	return map[string]any{
		"id": item.ID, "createdAt": item.CreatedAt, "name": item.Name, "type": item.Type,
		"version": item.Version, "from": from, "address": item.Host, "host": item.Host,
		"port": item.Port, "username": item.Username, "password": item.Password,
		"initialDB": item.InitialDB, "description": item.Description, "ssl": item.SSL,
		"containerName": item.ContainerName,
	}
}

func remoteDatabaseServers(ctx context.Context, typ, filter string) []service.Database {
	types := expandDatabaseListTypes(typ)
	rows := databasesForTypes(ctx, types)
	filter = strings.ToLower(strings.TrimSpace(filter))
	out := make([]service.Database, 0)
	for _, item := range databaseInstances(rows) {
		if databaseSourceIsLocal(item.From) {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(item.Name), filter) {
			continue
		}
		if full, ok := databaseService.Find(ctx, item.ID); ok {
			item = full
		}
		out = append(out, item)
	}
	return out
}

func findDatabaseByName(ctx context.Context, name string) (service.Database, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return service.Database{}, false
	}
	for _, item := range databaseService.Search(ctx, "", name) {
		if strings.EqualFold(item.Name, name) {
			if full, ok := databaseService.Find(ctx, item.ID); ok {
				return full, true
			}
			return item, true
		}
	}
	return service.Database{}, false
}

func loadDatabaseBaseInfo(ctx context.Context, typ, name string) map[string]any {
	rows := databasesForTypes(ctx, databaseTypeFamily(typ))
	for _, item := range databaseInstances(rows) {
		if !strings.EqualFold(item.Name, name) {
			continue
		}
		container := item.ContainerName
		if container == "" {
			if install, ok := findDatabaseInstall(nil, typ, name); ok {
				container = appConfiguredContainerName(install)
			}
		}
		return map[string]any{
			"name": item.Name, "port": item.Port, "password": item.Password,
			"remoteConn": strings.EqualFold(item.From, "remote"), "containerName": container, "mysqlKey": item.Type,
		}
	}
	if install, ok := findDatabaseInstall(nil, typ, name); ok {
		_, password := configuredDatabaseAuth(install, install.Key)
		return map[string]any{
			"name": install.Name, "port": configuredDatabasePort(install, install.Key), "password": password,
			"remoteConn": false, "containerName": appConfiguredContainerName(install), "mysqlKey": install.Key,
		}
	}
	return map[string]any{"name": name, "port": databasePort(typ, 0), "password": "", "remoteConn": false, "containerName": "", "mysqlKey": typ}
}

func findDatabaseServerConnection(ctx context.Context, store *appStore, typ, name string) map[string]any {
	rows := databasesForTypes(ctx, databaseTypeFamily(typ))
	var matched service.Database
	found := false
	for _, item := range databaseInstances(rows) {
		if strings.EqualFold(item.Name, name) {
			matched, found = item, true
			break
		}
	}
	install, hasInstall := findDatabaseInstall(store, typ, name)
	if !found && !hasInstall {
		return map[string]any{"status": "", "username": "", "password": "", "privilege": false, "containerName": "", "serviceName": "", "systemIP": "", "port": 0}
	}
	status := "Running"
	container, username, password, host := "", "", "", ""
	port := 0
	serviceName := name
	if found {
		if full, ok := databaseService.Find(ctx, matched.ID); ok {
			matched = full
		}
		username, password, host, port = matched.Username, matched.Password, matched.Host, matched.Port
		container = matched.ContainerName
		serviceName = matched.Name
	}
	if hasInstall {
		status = normalizeAppStatus(install.Status)
		if container == "" {
			container = appConfiguredContainerName(install)
		}
		if username == "" || password == "" {
			installUser, installPassword := configuredDatabaseAuth(install, install.Key)
			if username == "" {
				username = installUser
			}
			if password == "" {
				password = installPassword
			}
		}
		if port == 0 {
			port = configuredDatabasePort(install, install.Key)
		}
	}
	return map[string]any{"status": status, "username": username, "password": password, "privilege": true, "containerName": container, "serviceName": serviceName, "systemIP": host, "port": port}
}
