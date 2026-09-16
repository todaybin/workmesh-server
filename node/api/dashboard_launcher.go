// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// handleDashboardLauncher 返回首页应用启动器的真实目录和安装状态。
func handleDashboardLauncher(w http.ResponseWriter, _ *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardAppLaunchers()})
}

// dashboardAppLaunchers 按应用目录聚合安装实例，保持首页应用卡片契约。
func dashboardAppLaunchers() []map[string]any {
	store := getAppStore()
	store.mu.RLock()
	defer store.mu.RUnlock()
	catalog := store.state.Catalog
	if len(catalog) == 0 {
		catalog = store.state.Apps
	}
	installedByKey := make(map[string][]map[string]any)
	for _, install := range store.state.Apps {
		if strings.TrimSpace(install.ID) == "" || strings.TrimSpace(install.Key) == "" || strings.EqualFold(install.Status, "failed") {
			continue
		}
		installedByKey[install.Key] = append(installedByKey[install.Key], map[string]any{
			"installID": install.ID,
			"detailID":  install.ID,
			"name":      install.Name,
			"version":   install.Version,
			"status":    normalizeAppStatus(install.Status),
			"path":      appInstallPath(install),
			"webUI":     appValue(install.Config, "webUI", "WebUI", "url"),
			"httpPort":  appConfiguredInt(install.Config, 0, "httpPort", "port", "PANEL_APP_PORT_HTTP"),
			"httpsPort": appConfiguredInt(install.Config, 0, "httpsPort", "PANEL_APP_PORT_HTTPS"),
		})
	}
	visibility := dashboardLauncherVisibility()
	result := make([]map[string]any, 0, len(catalog))
	seen := make(map[string]bool)
	for _, app := range catalog {
		key := strings.TrimSpace(app.Key)
		if key == "" || seen[key] {
			continue
		}
		details := installedByKey[key]
		if visible, ok := visibility[key]; ok && !visible {
			continue
		}
		if len(details) == 0 && app.Recommend <= 0 {
			continue
		}
		item := appRecordDataLocalized(app, "", store.state.CatalogTags)
		item["key"] = key
		item["type"] = app.Type
		item["appType"] = app.Type
		item["isInstall"] = len(details) > 0
		item["detail"] = details
		result = append(result, item)
		seen[key] = true
	}
	// 保留目录缺失但已安装的应用，便于目录同步异常时仍可管理实例。
	for key, details := range installedByKey {
		if seen[key] {
			continue
		}
		if visible, ok := visibility[key]; ok && !visible {
			continue
		}
		install := appRecord{Key: key, Name: key}
		for _, candidate := range store.state.Apps {
			if candidate.Key == key {
				install = candidate
				break
			}
		}
		item := appRecordDataLocalized(install, "", nil)
		item["appType"] = install.Type
		item["isInstall"] = true
		item["detail"] = details
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		installedI, _ := result[i]["isInstall"].(bool)
		installedJ, _ := result[j]["isInstall"].(bool)
		if installedI != installedJ {
			return installedI
		}
		return fmt.Sprint(result[i]["recommend"]) < fmt.Sprint(result[j]["recommend"])
	})
	return result
}

// handleDashboardLauncherOption 返回应用启动器设置页的可选项目。
func handleDashboardLauncherOption(w http.ResponseWriter, r *http.Request) {
	request, err := requestMap(r)
	if err != nil {
		domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	filter := strings.ToLower(valueString(request, "filter", "name", "key"))
	visibility := dashboardLauncherVisibility()
	options := make([]map[string]any, 0)
	store := getAppStore()
	store.mu.RLock()
	keys := make(map[string]struct{})
	for _, item := range store.state.Catalog {
		if key := strings.TrimSpace(item.Key); key != "" {
			keys[key] = struct{}{}
		}
	}
	for _, item := range store.state.Apps {
		if key := strings.TrimSpace(item.Key); key != "" {
			keys[key] = struct{}{}
		}
	}
	store.mu.RUnlock()
	for key := range keys {
		if key == "" || (filter != "" && !strings.Contains(strings.ToLower(key), filter)) {
			continue
		}
		isShow := true
		if value, ok := visibility[key]; ok {
			isShow = value
		}
		options = append(options, map[string]any{"key": key, "isShow": isShow})
	}
	// 保留当前不可用但已保存的选择，避免安装/卸载后丢失用户配置。
	seen := make(map[string]bool, len(options))
	for _, item := range options {
		seen[fmt.Sprint(item["key"])] = true
	}
	for key, isShow := range visibility {
		if key == "" || seen[key] || (filter != "" && !strings.Contains(strings.ToLower(key), filter)) {
			continue
		}
		options = append(options, map[string]any{"key": key, "isShow": isShow})
	}
	sort.Slice(options, func(i, j int) bool { return fmt.Sprint(options[i]["key"]) < fmt.Sprint(options[j]["key"]) })
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": options})
}
