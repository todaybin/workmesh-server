// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/todaybin/workmesh-server/node/service"
)

// handleAppPost 按 v2 路径分派应用安装和已安装应用操作。
func handleAppPost(w http.ResponseWriter, s *appStore, r *http.Request, path string, body map[string]any) {
	mergeAppPostQuery(r, body)
	switch path {
	case "install":
		handleAppInstall(w, s, r, body)
	case "installed/check":
		handleAppInstalledCheck(w, s, body)
	case "installed/loadport":
		handleAppLoadPort(w, s, body)
	case "installed/conninfo":
		appOK(w, appConnectionInfo(r.Context(), s, body))
	case "installed/conf":
		handleAppInstalledConfig(w, s, body)
	case "installed/op":
		handleAppOperation(w, s, body)
	case "installed/port/change", "installed/params/update", "installed/config/update":
		handleAppUpdate(w, s, body)
	case "installed/sort/update":
		handleAppSortUpdate(w, s, body)
	case "installed/update/versions":
		handleAppUpdateVersions(w, s, body)
	case "installed/sync":
		handleAppInstalledSync(w, s, r)
	case "installed/ignore":
		handleAppIgnore(w, s, body)
	case "ignored/cancel":
		handleAppIgnoreCancel(w, s, body)
	default:
		notImplementedError(w, "该应用操作尚未接入真实业务")
	}
}

// mergeAppPostQuery 将旧客户端查询参数补入统一请求对象。
func mergeAppPostQuery(r *http.Request, body map[string]any) {
	if strings.TrimSpace(appValue(body, "installId", "installID", "appInstallId", "id")) == "" {
		for _, key := range []string{"installId", "installID", "appInstallId", "id"} {
			if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" {
				body[key] = value
				break
			}
		}
	}
	if strings.TrimSpace(appValue(body, "operate", "operation")) == "" {
		if value := strings.TrimSpace(r.URL.Query().Get("operate")); value != "" {
			body["operate"] = value
		}
	}
}

// appStatusCountsAsInstalled 判断应用状态是否计入已安装服务。
func appStatusCountsAsInstalled(status string) bool {
	switch normalizeAppStatus(status) {
	case "Running", "Stopped", "Paused", "ReStarting", "UnHealthy":
		return true
	default:
		return false
	}
}

// appServices 按正式版规则从数据库资源和已安装应用记录生成服务列表，禁止伪造默认服务。
func appServices(ctx context.Context, s *appStore, key string) []map[string]any {
	key = strings.ToLower(strings.TrimSpace(key))
	types := []string{key}
	switch key {
	case "mysql":
		types = []string{"mysql", "mysql-cluster", "mariadb"}
	case "postgres", "postgresql":
		types = []string{"postgres", "postgresql", "postgresql-cluster"}
	case "redis":
		types = []string{"redis", "redis-cluster"}
	}
	dbs := make([]service.Database, 0)
	for _, typ := range types {
		dbs = append(dbs, databaseService.Search(ctx, typ, "")...)
	}
	services := make([]map[string]any, 0, len(dbs))
	for _, db := range dbs {
		config := map[string]any{}
		from := db.From
		status := "Running"
		if db.AppInstallID > 0 {
			from = "local"
			s.mu.RLock()
			_, install := findApp(s.state.Apps, strconv.FormatInt(db.AppInstallID, 10))
			if install.ID == "" {
				for _, candidate := range s.state.Apps {
					if appValue(candidate.Config, "databaseID", "dbID") == strconv.FormatInt(db.AppInstallID, 10) {
						install = candidate
						break
					}
				}
			}
			s.mu.RUnlock()
			if install.ID != "" {
				status = normalizeAppStatus(install.Status)
				for k, v := range install.Config {
					config[k] = v
				}
			}
		} else {
			from = "remote"
			if db.Username != "" {
				config["PANEL_DB_ROOT_USER"] = db.Username
			}
			if db.Password != "" {
				config["PANEL_DB_ROOT_PASSWORD"] = db.Password
			}
		}
		if from == "" {
			from = "local"
		}
		services = append(services, map[string]any{"label": db.Name, "value": db.Name, "config": config, "from": from, "status": status})
	}
	if len(dbs) > 0 {
		return services
	}
	// 非数据库应用返回实际运行中的安装实例，不再返回固定占位项。
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, install := range s.state.Apps {
		if !strings.EqualFold(install.Key, key) || !appStatusCountsAsInstalled(install.Status) {
			continue
		}
		config := map[string]any{}
		for k, v := range install.Config {
			config[k] = v
		}
		value := appValue(install.Config, "serviceName", "SERVICE_NAME")
		if value == "" {
			value = install.Name
		}
		services = append(services, map[string]any{"label": install.Name, "value": value, "config": config, "from": "local", "status": strings.ToLower(normalizeAppStatus(install.Status))})
	}
	return services
}

// appConnectionInfo 返回真实安装或数据库资源的连接信息，敏感字段仅在仓储中存在时返回。
func appConnectionInfo(ctx context.Context, s *appStore, body map[string]any) map[string]any {
	typ := appValue(body, "type", "key", "appKey")
	name := appValue(body, "name", "serviceName", "database")
	if item, ok := databaseService.FindConnection(ctx, typ, name); ok {
		return map[string]any{"status": "Running", "username": item.Username, "password": item.Password, "privilege": true, "containerName": "", "serviceName": item.Name, "systemIP": item.Host, "port": item.Port}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, item := findApp(s.state.Apps, appValue(body, "appInstallId", "installId", "id", "name"))
	if item.ID == "" && typ != "" {
		for _, candidate := range s.state.Apps {
			if strings.EqualFold(candidate.Key, typ) && (name == "" || strings.EqualFold(candidate.Name, name)) {
				item = candidate
				break
			}
		}
	}
	if item.ID == "" {
		return map[string]any{"status": "", "username": "", "password": "", "privilege": false, "containerName": "", "serviceName": "", "systemIP": "", "port": 0}
	}
	port := appConfiguredInt(item.Config, 0, "port", "servicePort", "PANEL_APP_PORT")
	return map[string]any{"status": normalizeAppStatus(item.Status), "username": appValue(item.Config, "username", "user", "PANEL_DB_ROOT_USER"), "password": appValue(item.Config, "password", "PANEL_DB_ROOT_PASSWORD"), "privilege": true, "containerName": appConfiguredContainerName(item), "serviceName": appValue(item.Config, "serviceName", "SERVICE_NAME"), "systemIP": appValue(item.Config, "host", "systemIP"), "port": port}
}
