// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
)

// databaseTypeFromImage 根据镜像名识别数据库类型，忽略代理和工具镜像。
func databaseTypeFromImage(image string) string {
	image = strings.ToLower(strings.TrimSpace(image))
	if image == "" {
		return ""
	}
	name := image
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		name = name[slash+1:]
	}
	base := strings.SplitN(name, ":", 2)[0]
	switch {
	case base == "redis" || strings.HasPrefix(base, "redis-"):
		return "redis"
	case base == "postgres" || strings.HasPrefix(base, "postgres"):
		return "postgresql"
	case strings.Contains(base, "mariadb"):
		return "mariadb"
	case base == "mysql" || strings.HasPrefix(base, "mysql"):
		return "mysql"
	case base == "mongo" || strings.HasPrefix(base, "mongo"):
		return "mongodb"
	default:
		return ""
	}
}

func imageVersion(image string) string {
	image = strings.TrimSpace(image)
	if slash := strings.LastIndex(image, "/"); slash >= 0 {
		image = image[slash+1:]
	}
	parts := strings.SplitN(image, ":", 2)
	if len(parts) == 2 && parts[1] != "" && parts[1] != "latest" {
		return parts[1]
	}
	return ""
}

func dockerComposeLabel(labels, key string) string {
	for _, item := range strings.Split(labels, ",") {
		name, value, ok := strings.Cut(item, "=")
		if ok && strings.TrimSpace(name) == key {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func hostPortFromDockerPorts(ports string) int {
	for _, part := range strings.Split(ports, ",") {
		part = strings.TrimSpace(part)
		arrow := strings.Index(part, "->")
		if arrow < 0 {
			continue
		}
		left := part[:arrow]
		if colon := strings.LastIndex(left, ":"); colon >= 0 {
			left = left[colon+1:]
		}
		port := 0
		for _, ch := range left {
			if ch < '0' || ch > '9' {
				port = 0
				break
			}
			port = port*10 + int(ch-'0')
		}
		if port > 0 && port <= 65535 {
			return port
		}
	}
	return 0
}

func discoverInstalledDatabaseContainers(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	result, err := (service.CommandService{}).Execute(probeCtx, model.CommandRequest{
		Program: service.DockerBinary(), Args: []string{"ps", "-a", "--format", "{{json .}}"}, Timeout: 8 * time.Second,
	})
	if err != nil || result.ExitCode != 0 {
		return
	}
	rows := databasesForTypes(ctx, []string{"postgresql", "postgres", "postgresql-cluster", "redis", "redis-cluster", "mysql", "mariadb", "mysql-cluster", "mongodb", "mongo"})
	seen := 0
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if seen >= 40 {
			break
		}
		var item struct {
			Names  string `json:"Names"`
			Image  string `json:"Image"`
			State  string `json:"State"`
			Ports  string `json:"Ports"`
			Labels string `json:"Labels"`
			Status string `json:"Status"`
		}
		if json.Unmarshal([]byte(line), &item) != nil {
			continue
		}
		typ := databaseTypeFromImage(item.Image)
		container := strings.TrimSpace(item.Names)
		if typ == "" || !databaseContainerIdentifier.MatchString(container) {
			continue
		}
		if strings.EqualFold(dockerComposeLabel(item.Labels, "com.docker.compose.oneoff"), "True") {
			continue
		}
		seen++
		if databaseContainerRegistered(rows, container) {
			continue
		}
		env := dockerContainerEnv(ctx, container)
		name := discoveredDatabaseName(dockerComposeLabel(item.Labels, "com.docker.compose.project"), container, rows)
		if name == "" {
			continue
		}
		port := hostPortFromDockerPorts(item.Ports)
		if port == 0 {
			port = databasePort(typ, 0)
		}
		username, password := credentialsFromContainerEnv(typ, env)
		version := imageVersion(item.Image)
		created, createErr := databaseService.Create(ctx, service.Database{
			Name: name, Type: typ, Version: version, From: "local", Host: "127.0.0.1", Port: port,
			ContainerName: container, Username: username, Password: password,
		})
		if createErr != nil {
			continue
		}
		rows = append(rows, created)
		rememberDiscoveredInstall(ctx, created, dockerComposeLabel(item.Labels, "com.docker.compose.project.config_files"), item.State)
	}
}

func databaseContainerRegistered(rows []service.Database, container string) bool {
	for _, row := range rows {
		if strings.EqualFold(row.ContainerName, container) && databaseRowIsInstance(row, rows) {
			return true
		}
	}
	return false
}

func discoveredDatabaseName(project, container string, rows []service.Database) string {
	for _, candidate := range []string{strings.TrimSpace(project), container} {
		if candidate == "" || !databaseContainerIdentifier.MatchString(candidate) {
			continue
		}
		conflict := false
		for _, row := range rows {
			if !strings.EqualFold(row.Name, candidate) {
				continue
			}
			if row.ContainerName == "" || strings.EqualFold(row.ContainerName, container) {
				return ""
			}
			conflict = true
		}
		if !conflict {
			return candidate
		}
	}
	return ""
}

func dockerContainerEnv(ctx context.Context, container string) map[string]string {
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := (service.CommandService{}).Execute(probeCtx, model.CommandRequest{
		Program: service.DockerBinary(),
		Args:    []string{"inspect", "--format", "{{range .Config.Env}}{{println .}}{{end}}", container},
		Timeout: 5 * time.Second,
	})
	if err != nil || result.ExitCode != 0 {
		return map[string]string{}
	}
	env := map[string]string{}
	for _, line := range strings.Split(result.Stdout, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok && key != "" {
			env[key] = value
		}
	}
	return env
}

func credentialsFromContainerEnv(typ string, env map[string]string) (string, string) {
	switch canonicalDatabaseAppType(typ) {
	case "postgresql", "postgresql-cluster":
		user := env["POSTGRES_USER"]
		if user == "" {
			user = "postgres"
		}
		return user, env["POSTGRES_PASSWORD"]
	case "mysql", "mysql-cluster":
		password := env["MYSQL_ROOT_PASSWORD"]
		if password == "" {
			password = env["MYSQL_PASSWORD"]
		}
		return "root", password
	case "mariadb":
		password := env["MARIADB_ROOT_PASSWORD"]
		if password == "" {
			password = env["MYSQL_ROOT_PASSWORD"]
		}
		return "root", password
	case "mongodb":
		user := env["MONGO_INITDB_ROOT_USERNAME"]
		if user == "" {
			user = "root"
		}
		return user, env["MONGO_INITDB_ROOT_PASSWORD"]
	default:
		return "", env["REDIS_PASSWORD"]
	}
}

func rememberDiscoveredInstall(ctx context.Context, item service.Database, composePath, state string) {
	repository, err := SharedRepository()
	if err != nil || repository == nil {
		return
	}
	status := "Stopped"
	if strings.EqualFold(strings.TrimSpace(state), "running") {
		status = "Running"
	}
	id := "container:" + item.ContainerName
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, _ = repository.ExecContext(ctx, `INSERT INTO app_installs(id,app_key,name,version,status,install_path,compose_path,compose_project,container_names,config_json,message,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET status=excluded.status,container_names=excluded.container_names,updated_at=excluded.updated_at`,
		id, item.Type, item.Name, item.Version, status, "", composePath, item.Name, item.ContainerName, "{}", "", now, now)
}

func visibleChildDatabases(rows []service.Database, server, info string, live []string) []service.Database {
	children := databaseChildren(rows, server, info)
	if len(live) == 0 {
		return children
	}
	seen := map[string]struct{}{}
	for _, item := range children {
		seen[strings.ToLower(item.Name)] = struct{}{}
	}
	byName := map[string]service.Database{}
	for _, row := range rows {
		byName[strings.ToLower(row.Name)] = row
	}
	info = strings.ToLower(strings.TrimSpace(info))
	for _, name := range live {
		name = strings.TrimSpace(name)
		key := strings.ToLower(name)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		if info != "" && !strings.Contains(key, info) {
			continue
		}
		// 已登记行优先复用；实例里新出现、尚未写入 SQLite 的库也必须返回。
		if row, ok := byName[key]; ok {
			children = append(children, row)
		} else {
			children = append(children, service.Database{Name: name, Type: "postgresql", From: "local", InitialDB: server})
		}
		seen[key] = struct{}{}
	}
	return children
}

func syncPostgresLiveDatabases(ctx context.Context, server string) []string {
	target, ok := postgresTargetForRequest(ctx, server)
	if !ok {
		return nil
	}
	names, err := service.ListPostgresDatabases(ctx, postgresExecutor, target)
	if err != nil {
		return nil
	}
	rows := databasesForTypes(ctx, databaseTypeFamily("postgresql"))
	from := "remote"
	if target.ContainerName != "" {
		from = "local"
	}
	typ := target.Type
	if typ == "" {
		typ = "postgresql"
	}
	for _, name := range names {
		rememberLiveChildDatabase(ctx, server, typ, from, target.Host, target.ContainerName, target.Port, name, rows)
	}
	return names
}

func syncMySQLLiveDatabases(ctx context.Context, server string) []string {
	target, ok := mysqlTargetForRequest(ctx, server, "")
	if !ok {
		return nil
	}
	queryExecutor, ok := mysqlExecutor.(service.MySQLQueryExecutor)
	if !ok {
		return nil
	}
	names, err := service.ListMySQLDatabases(ctx, queryExecutor, target)
	if err != nil {
		return nil
	}
	rows := databasesForTypes(ctx, databaseTypeFamily("mysql"))
	from := "remote"
	if target.ContainerName != "" {
		from = "local"
	}
	typ := target.Type
	if typ == "" {
		typ = "mysql"
	}
	for _, name := range names {
		rememberLiveChildDatabase(ctx, server, typ, from, target.Host, target.ContainerName, target.Port, name, rows)
	}
	return names
}

func syncMongoLiveDatabases(ctx context.Context, server string) []string {
	target, ok := mongoTargetForRequest(ctx, server)
	if !ok {
		return nil
	}
	names, err := service.ListMongoDatabases(ctx, mongoExecutor, target)
	if err != nil {
		return nil
	}
	rows := databasesForTypes(ctx, databaseTypeFamily("mongodb"))
	from := "remote"
	if target.ContainerName != "" {
		from = "local"
	}
	for _, name := range names {
		rememberLiveChildDatabase(ctx, server, "mongodb", from, target.Host, target.ContainerName, target.Port, name, rows)
	}
	return names
}

func rememberLiveChildDatabase(ctx context.Context, server string, targetType, from, host, container string, port int, name string, rows []service.Database) {
	for _, row := range rows {
		if strings.EqualFold(row.Name, name) {
			return
		}
	}
	if strings.TrimSpace(host) == "" {
		host = "127.0.0.1"
	}
	created, err := databaseService.Create(ctx, service.Database{
		Name: name, Type: targetType, From: from, Host: host, Port: port, ContainerName: container, InitialDB: server,
	})
	if err == nil {
		_ = created
	}
}
