// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	workmeshi18n "github.com/todaybin/workmesh-server/i18n"
	"github.com/todaybin/workmesh-server/node/service"
)

// handleAppInstall 创建安装记录并按应用资源类型启动真实安装任务。
func handleAppInstall(w http.ResponseWriter, s *appStore, r *http.Request, body map[string]any) {
	id := appValue(body, "appInstallId", "id", "appId", "key", "name")
	if id == "" {
		runtimeErr(w, http.StatusBadRequest, "应用标识不能为空")
		return
	}
	taskID := appValue(body, "taskID", "taskId")
	if taskID == "" {
		taskID = idToken()
	}
	item := newAppInstallRecord(id, body)
	fillAppInstallCatalogDetail(s, &item, body)
	normalizeAppInstallRecord(&item, taskID)
	downloadURL, compose := resolveAppInstallSource(s, item, body)
	if err := storeAppInstallRecord(s, item); err != nil {
		runtimeErr(w, http.StatusServiceUnavailable, "应用安装状态存储不可用: "+err.Error())
		return
	}
	if err := persistAppInstallRecord(item, ""); err != nil {
		_ = rollbackAppInstallRecord(s, item.ID)
		runtimeErr(w, http.StatusServiceUnavailable, "应用安装状态存储不可用: "+err.Error())
		return
	}
	if err := ensureAppTaskLogChecked(taskID, item.ID, item.Name, "installing", "开始安装应用"); err != nil {
		item.Status = "error"
		item.Message = "应用任务存储失败: " + err.Error()
		if saveErr := storeAppInstallRecord(s, item); saveErr == nil {
			_ = persistAppInstallRecord(item, "")
		}
		runtimeErr(w, http.StatusServiceUnavailable, "应用任务存储不可用: "+err.Error())
		return
	}
	if downloadURL != "" || compose != "" {
		go runAppInstallTask(s, item, downloadURL, compose)
	} else {
		var completeErr error
		item, completeErr = completeRegisteredAppInstall(s, item)
		releaseManagedSlot(managedRuntimeSlots.tasks, taskID)
		if completeErr != nil {
			runtimeErr(w, http.StatusServiceUnavailable, "应用安装状态存储不可用: "+completeErr.Error())
			return
		}
	}
	result := appRecordDataLocalized(item, workmeshi18n.LocaleFromRequest(r), appCatalogTagsSnapshot(s))
	result["status"] = item.Status
	result["taskStatus"] = item.Status
	result["taskID"] = taskID
	appOK(w, result)
}

// newAppInstallRecord 从兼容字段构造待安装应用记录。
func newAppInstallRecord(id string, body map[string]any) appRecord {
	return appRecord{
		ID:            id,
		Key:           appValue(body, "key", "appKey", "id"),
		Name:          appValue(body, "name", "appName", "key"),
		Version:       appValue(body, "version"),
		Status:        "installing",
		ContainerName: appValue(body, "containerName", "CONTAINER_NAME"),
		Config:        body,
		UpdatedAt:     time.Now().UTC(),
	}
}

// fillAppInstallCatalogDetail 使用应用详情标识补全目录元数据和下载地址。
func fillAppInstallCatalogDetail(s *appStore, item *appRecord, body map[string]any) {
	detailID := appValue(body, "appDetailId", "appDetailID")
	if detailID == "" {
		return
	}
	s.mu.Lock()
	_ = s.ensureCatalogLocked()
	catalogApp, catalogVersion, ok := findCatalogDetail(s.state.Catalog, detailID)
	s.mu.Unlock()
	if !ok {
		return
	}
	item.Key = catalogApp.Key
	if item.Name == "" {
		item.Name = catalogApp.Name
	}
	if item.Version == "" {
		item.Version = catalogVersion.Version
	}
	if appValue(body, "downloadUrl", "downloadURL") == "" {
		body["downloadUrl"] = catalogVersion.DownloadURL
	}
}

// normalizeAppInstallRecord 补齐安装记录的任务、名称和容器字段。
func normalizeAppInstallRecord(item *appRecord, taskID string) {
	item.Config["taskID"] = taskID
	if item.Key == "" {
		item.Key = item.ID
	}
	if item.Name == "" {
		item.Name = item.Key
	}
	if item.ContainerName == "" {
		item.ContainerName = "WorkMesh-" + item.Key + "-" + strings.ToLower(idToken()[:6])
	}
	item.Config["containerName"] = item.ContainerName
}

// resolveAppInstallSource 从请求或目录版本中解析真实安装资源。
func resolveAppInstallSource(s *appStore, item appRecord, body map[string]any) (string, string) {
	downloadURL := appValue(body, "downloadUrl", "downloadURL")
	compose := appValue(body, "dockerCompose", "compose")
	if downloadURL == "" {
		s.mu.Lock()
		_ = s.ensureCatalogLocked()
		if catalogIndex, catalogItem := findApp(s.state.Catalog, item.Key); catalogIndex >= 0 {
			for _, version := range catalogItem.Versions {
				if item.Version == "" || version.Version == item.Version {
					downloadURL, compose = version.DownloadURL, version.DockerCompose
					break
				}
			}
		}
		s.mu.Unlock()
	}
	if compose == "" {
		compose = appValue(body, "docker-compose", "composeContent")
	}
	if compose != "" {
		item.Config["dockerCompose"] = compose
	}
	return selectAppDownloadURL(downloadURL, body), compose
}

// storeAppInstallRecord 写入待安装应用的当前状态。
func storeAppInstallRecord(s *appStore, item appRecord) error {
	s.mu.Lock()
	previous := append([]appRecord(nil), s.state.Apps...)
	if index, _ := findApp(s.state.Apps, item.ID); index >= 0 {
		s.state.Apps[index] = item
	} else {
		s.state.Apps = append(s.state.Apps, item)
	}
	err := s.saveLocked()
	if err != nil {
		s.state.Apps = previous
	}
	s.mu.Unlock()
	return err
}

func rollbackAppInstallRecord(s *appStore, id string) error {
	s.mu.Lock()
	previous := append([]appRecord(nil), s.state.Apps...)
	kept := make([]appRecord, 0, len(s.state.Apps))
	for _, item := range s.state.Apps {
		if item.ID != id {
			kept = append(kept, item)
		}
	}
	s.state.Apps = kept
	if err := s.saveLocked(); err != nil {
		s.state.Apps = previous
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	return nil
}

// completeRegisteredAppInstall 完成不含应用包或 Compose 的兼容登记流程。
func completeRegisteredAppInstall(s *appStore, item appRecord) (appRecord, error) {
	item.Status = "running"
	s.mu.Lock()
	previous := append([]appRecord(nil), s.state.Apps...)
	if index, _ := findApp(s.state.Apps, item.ID); index >= 0 {
		s.state.Apps[index] = item
		if err := s.saveLocked(); err != nil {
			s.state.Apps = previous
			s.mu.Unlock()
			return item, err
		}
	}
	s.mu.Unlock()
	if err := persistAppInstallRecord(item, ""); err != nil {
		s.mu.Lock()
		s.state.Apps = previous
		if saveErr := s.saveLocked(); saveErr != nil {
			s.mu.Unlock()
			return item, fmt.Errorf("%w; 回滚应用安装状态失败: %v", err, saveErr)
		}
		s.mu.Unlock()
		return item, err
	}
	return item, nil
}

// appCatalogTagsSnapshot 返回可脱离仓储锁使用的目录标签快照。
func appCatalogTagsSnapshot(s *appStore) []appTagRecord {
	s.mu.RLock()
	tags := append([]appTagRecord(nil), s.state.CatalogTags...)
	s.mu.RUnlock()
	return tags
}

// handleAppInstalledCheck 探测已安装应用及其真实运行状态。
func handleAppInstalledCheck(w http.ResponseWriter, s *appStore, body map[string]any) {
	id := appValue(body, "name", "key", "appInstallId")
	s.mu.RLock()
	_, item := findApp(s.state.Apps, id)
	s.mu.RUnlock()
	if item.ID != "" && (strings.EqualFold(item.Key, "openresty") || appConfiguredContainerName(item) != "") {
		handleStoredAppInstalledCheck(w, s, id)
		return
	}
	probe := serviceProbeApplication(body, id)
	if item.ID != "" && !probe.IsExist && probe.Error == "未配置该应用的本机探测器" {
		probe.IsExist = true
		probe.IsActive = normalizeAppStatus(item.Status) == "Running"
		probe.Status = normalizeAppStatus(item.Status)
		probe.Version = item.Version
	}
	containerName := appConfiguredContainerName(item)
	if containerName == "" && strings.HasPrefix(probe.Binary, "docker://") {
		containerName = strings.TrimPrefix(probe.Binary, "docker://")
	}
	appOK(w, map[string]any{"name": id, "version": probe.Version, "isExist": probe.IsExist, "isActive": probe.IsActive, "status": probe.Status, "app": probe.App, "appInstallId": item.ID, "containerName": containerName, "httpPort": 80, "httpsPort": 443, "websiteDir": "/www/wwwroot"})
}

// handleStoredAppInstalledCheck 同步并返回仓储应用的运行状态。
func handleStoredAppInstalledCheck(w http.ResponseWriter, s *appStore, id string) {
	item, exists, err := s.syncAppInstallStatus(context.Background(), id, false)
	if err != nil {
		runtimeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		appOK(w, map[string]any{"name": id, "app": id, "isExist": false, "isActive": false, "status": ""})
		return
	}
	status := normalizeAppStatus(item.Status)
	websiteDir := appValue(item.Config, "websiteDir", "WEBSITE_DIR")
	if websiteDir == "" {
		websiteDir = "/www/wwwroot"
	}
	data := map[string]any{
		"name": item.Name, "version": item.Version, "isExist": true,
		"isActive": status == "Running", "status": status, "app": item.Key,
		"appInstallId": item.ID, "containerName": appConfiguredContainerName(item),
		"httpPort":   appConfiguredInt(item.Config, 80, "httpPort", "PANEL_APP_PORT_HTTP", "port"),
		"httpsPort":  appConfiguredInt(item.Config, 443, "httpsPort", "PANEL_APP_PORT_HTTPS"),
		"websiteDir": websiteDir,
	}
	if item.Message != "" {
		data["error"] = item.Message
	}
	appOK(w, data)
}

// serviceProbeApplication 调用系统应用探测器并补齐应用名称。
func serviceProbeApplication(body map[string]any, id string) service.ApplicationStatus {
	probe := service.ProbeApplication(context.Background(), appValue(body, "key", "app", "type"), id)
	if probe.App == "" {
		probe.App = id
	}
	return probe
}

// handleAppLoadPort 返回已安装应用保存的端口配置。
func handleAppLoadPort(w http.ResponseWriter, s *appStore, body map[string]any) {
	id := appValue(body, "name", "key")
	s.mu.RLock()
	_, item := findApp(s.state.Apps, id)
	s.mu.RUnlock()
	appOK(w, item.Config["port"])
}

// handleAppInstalledConfig 返回已安装应用参数和 Compose 路径。
func handleAppInstalledConfig(w http.ResponseWriter, s *appStore, body map[string]any) {
	id := appValue(body, "appInstallId", "installId", "id", "name")
	s.mu.RLock()
	_, item := findApp(s.state.Apps, id)
	s.mu.RUnlock()
	if item.ID == "" {
		runtimeErr(w, http.StatusNotFound, "应用安装记录不存在")
		return
	}
	params := make(map[string]any, len(item.Config))
	for key, value := range item.Config {
		params[key] = value
	}
	appOK(w, map[string]any{"type": item.Key, "name": item.Name, "params": params, "dockerCompose": appComposePath(item)})
}

// handleAppSortUpdate 按客户端提交顺序更新已安装应用排序。
func handleAppSortUpdate(w http.ResponseWriter, s *appStore, body map[string]any) {
	items, _ := body["items"].([]any)
	s.mu.Lock()
	previous := append([]appRecord(nil), s.state.Apps...)
	for _, value := range items {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		id := appValue(item, "installID", "id")
		if index, _ := findApp(s.state.Apps, id); index >= 0 {
			if order, ok := item["sortOrder"].(float64); ok {
				s.state.Apps[index].SortOrder = int(order)
			}
		}
	}
	if err := s.saveLocked(); err != nil {
		s.state.Apps = previous
		s.mu.Unlock()
		runtimeErr(w, http.StatusServiceUnavailable, "应用排序保存失败: "+err.Error())
		return
	}
	s.mu.Unlock()
	appOK(w, map[string]any{"updated": true})
}

// handleAppUpdateVersions 返回指定目录应用可用的版本集合。
func handleAppUpdateVersions(w http.ResponseWriter, s *appStore, body map[string]any) {
	id := appValue(body, "appInstallId", "id", "key", "name")
	s.mu.RLock()
	versions := make([]map[string]any, 0)
	for _, app := range s.state.Catalog {
		if id == "" || app.Key == id || app.ID == id || app.Name == id {
			versions = append(versions, map[string]any{"version": app.Version, "appKey": app.Key, "name": app.Name})
		}
	}
	s.mu.RUnlock()
	appOK(w, versions)
}

// handleAppInstalledSync 同步全部已安装应用的容器状态。
func handleAppInstalledSync(w http.ResponseWriter, s *appStore, r *http.Request) {
	s.mu.RLock()
	ids := make([]string, 0, len(s.state.Apps))
	for _, item := range s.state.Apps {
		ids = append(ids, item.ID)
	}
	s.mu.RUnlock()
	failed := make([]map[string]any, 0)
	for _, id := range ids {
		_, _, err := s.syncAppInstallStatus(r.Context(), id, true)
		if err != nil {
			failed = append(failed, map[string]any{"id": id, "error": err.Error()})
		}
	}
	if len(failed) > 0 {
		runtimeErrData(w, http.StatusBadGateway, "同步部分应用状态失败", map[string]any{"failed": failed, "total": len(ids)})
		return
	}
	appOK(w, map[string]any{"synced": len(ids), "total": len(ids)})
}

// handleAppIgnore 将应用版本加入忽略列表。
func handleAppIgnore(w http.ResponseWriter, s *appStore, body map[string]any) {
	s.mu.Lock()
	previous := append([]map[string]any(nil), s.state.Ignored...)
	s.state.Ignored = append(s.state.Ignored, body)
	if err := s.saveLocked(); err != nil {
		s.state.Ignored = previous
		s.mu.Unlock()
		runtimeErr(w, http.StatusServiceUnavailable, "应用忽略列表保存失败: "+err.Error())
		return
	}
	s.mu.Unlock()
	appOK(w, map[string]any{"ignored": true})
}

// handleAppIgnoreCancel 从忽略列表移除指定应用版本。
func handleAppIgnoreCancel(w http.ResponseWriter, s *appStore, body map[string]any) {
	id := appValue(body, "id", "appID", "appDetailID")
	s.mu.Lock()
	previous := append([]map[string]any(nil), s.state.Ignored...)
	kept := make([]map[string]any, 0, len(s.state.Ignored))
	for _, item := range s.state.Ignored {
		if appValue(item, "id", "appID", "appDetailID") != id {
			kept = append(kept, item)
		}
	}
	s.state.Ignored = kept
	if err := s.saveLocked(); err != nil {
		s.state.Ignored = previous
		s.mu.Unlock()
		runtimeErr(w, http.StatusServiceUnavailable, "应用忽略列表保存失败: "+err.Error())
		return
	}
	s.mu.Unlock()
	appOK(w, map[string]any{"cancelled": true})
}
