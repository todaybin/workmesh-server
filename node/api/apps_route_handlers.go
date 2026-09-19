// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"fmt"
	workmeshi18n "github.com/todaybin/workmesh-server/i18n"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// appRouteHandlers 持有应用接口共享的持久化仓储。
type appRouteHandlers struct {
	store *appStore
}

// appCatalogSearchFilter 表示应用目录查询的已校验筛选条件。
type appCatalogSearchFilter struct {
	name            string
	typeName        string
	resource        string
	recommendOnly   bool
	showCurrentArch bool
	tags            []any
}

// appCatalogUpdateSnapshot 保存更新检查期间读取的一致性目录快照。
type appCatalogUpdateSnapshot struct {
	catalog      []appRecord
	installed    []appRecord
	version      string
	lastModified int64
	syncing      bool
	syncedAt     time.Time
}

// listInstalled 返回已安装应用，并按请求选择是否同步真实容器状态。
func (h appRouteHandlers) listInstalled(w http.ResponseWriter, r *http.Request) {
	body := appBody(r)
	if syncRequested, _ := body["sync"].(bool); syncRequested {
		h.syncInstalledStatuses(r)
	}
	// 已安装应用本身不依赖商店可用性；目录刷新失败时仍返回真实安装状态。
	_ = h.refreshCatalogForSearch("installed")
	h.store.mu.RLock()
	catalog := append([]appRecord(nil), h.store.state.Catalog...)
	items := make([]map[string]any, 0, len(h.store.state.Apps))
	for _, app := range h.store.state.Apps {
		item := appInstalledResponseData(app, workmeshi18n.LocaleFromRequest(r), h.store.state.CatalogTags)
		item["installed"] = appStatusCountsAsInstalled(app.Status)
		_, latest, ok := latestCatalogVersion(app, catalog)
		item["canUpdate"] = ok && compareAppVersion(latest.Version, app.Version) > 0
		item["status"] = normalizeAppStatus(app.Status)
		item["appStatus"] = normalizeAppStatus(app.Status)
		items = append(items, item)
	}
	h.store.mu.RUnlock()
	if r.Method == http.MethodGet {
		appOK(w, items)
		return
	}
	appOK(w, map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": 50})
}

// syncInstalledStatuses 逐个探测已安装应用的真实运行状态。
func (h appRouteHandlers) syncInstalledStatuses(r *http.Request) {
	h.store.mu.RLock()
	ids := make([]string, 0, len(h.store.state.Apps))
	for _, item := range h.store.state.Apps {
		ids = append(ids, item.ID)
	}
	h.store.mu.RUnlock()
	for _, id := range ids {
		_, _, _ = h.store.syncAppInstallStatus(r.Context(), id, false)
	}
}

// appInstalledResponseData 补齐 1Panel 已安装应用页面实际使用的运行时字段。
func appInstalledResponseData(app appRecord, locale string, metadata []appTagRecord) map[string]any {
	item := appRecordDataLocalized(app, locale, metadata)
	item["path"] = appInstallPath(app)
	item["container"] = appConfiguredContainerName(app)
	item["serviceName"] = appServiceName(app)
	item["httpPort"] = appConfiguredInt(app.Config, 0, "httpPort", "PANEL_APP_PORT_HTTP", "port")
	item["httpsPort"] = appConfiguredInt(app.Config, 0, "httpsPort", "PANEL_APP_PORT_HTTPS")
	item["env"] = app.Config
	return item
}

// appServiceName 返回 Compose 项目名；未显式配置时使用 Compose 文件所在目录名。
func appServiceName(app appRecord) string {
	if value := appValue(app.Config, "serviceName", "composeProject", "projectName", "SERVICE_NAME"); value != "" {
		return value
	}
	return filepath.Base(appInstallPath(app))
}

// searchCatalog 校验查询参数，并返回真实应用目录的分页结果。
func (h appRouteHandlers) searchCatalog(w http.ResponseWriter, r *http.Request) {
	body, err := decodeAppSearchBody(r)
	if err != nil {
		runtimeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/apps/")
	if err := h.refreshCatalogForSearch(path); err != nil {
		runtimeErr(w, http.StatusBadGateway, "应用商店不可用: "+err.Error())
		return
	}
	h.store.mu.Lock()
	result, err := h.searchCatalogLocked(body, workmeshi18n.LocaleFromRequest(r))
	h.store.mu.Unlock()
	if err != nil {
		runtimeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	appOK(w, result)
}

// refreshCatalogForSearch 按本地或远程同步路由刷新应用目录。
func (h appRouteHandlers) refreshCatalogForSearch(path string) error {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	if err := h.store.ensureCatalogLocked(); err != nil {
		return err
	}
	if strings.TrimSpace(os.Getenv("WORKMESH_APP_CATALOG")) != "" || path == "sync/local" {
		_, err := h.store.refreshCatalogLocked()
		return err
	}
	err := h.store.refreshRemoteLocked(path == "sync/remote")
	if err != nil && len(h.store.state.Catalog) == 0 {
		return err
	}
	return nil
}

// searchCatalogLocked 在仓储写锁内筛选目录并保存同步后的元数据。
func (h appRouteHandlers) searchCatalogLocked(body map[string]any, locale string) (map[string]any, error) {
	if len(h.store.state.Catalog) == 0 {
		h.store.state.Catalog = appCatalogFromEnv()
		if len(h.store.state.Catalog) == 0 {
			h.store.state.Catalog = append([]appRecord(nil), h.store.state.Apps...)
		}
	}
	page, pageSize, err := appSearchPage(body)
	if err != nil {
		return nil, err
	}
	filter, err := parseAppCatalogSearchFilter(body)
	if err != nil {
		return nil, err
	}
	filtered := filterAppCatalog(h.store.state.Catalog, filter)
	total := len(filtered)
	start, end := appCatalogPageBounds(total, page, pageSize)
	items := h.catalogSearchItems(filtered[start:end], locale)
	return map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize}, nil
}

// parseAppCatalogSearchFilter 校验并规范化目录查询筛选字段。
func parseAppCatalogSearchFilter(body map[string]any) (appCatalogSearchFilter, error) {
	filter := appCatalogSearchFilter{
		name:     strings.ToLower(appValue(body, "name", "key")),
		typeName: normalizeRuntimeTypeFilter(strings.ToLower(strings.TrimSpace(appValue(body, "type")))),
		resource: strings.ToLower(strings.TrimSpace(appValue(body, "resource"))),
	}
	if raw, exists := body["recommend"]; exists {
		value, ok := raw.(bool)
		if !ok {
			return filter, fmt.Errorf("recommend 必须是布尔值")
		}
		filter.recommendOnly = value
	}
	if raw, exists := body["showCurrentArch"]; exists {
		value, ok := raw.(bool)
		if !ok {
			return filter, fmt.Errorf("showCurrentArch 必须是布尔值")
		}
		filter.showCurrentArch = value
	}
	if raw, exists := body["tags"]; exists {
		value, ok := raw.([]any)
		if !ok {
			return filter, fmt.Errorf("tags 必须是字符串数组")
		}
		if len(value) > 0 {
			filter.tags = value
		}
	}
	return filter, nil
}

// filterAppCatalog 返回满足全部查询条件的目录记录。
func filterAppCatalog(catalog []appRecord, filter appCatalogSearchFilter) []appRecord {
	filtered := make([]appRecord, 0, len(catalog))
	for _, app := range catalog {
		if appMatchesCatalogFilter(app, filter) {
			filtered = append(filtered, app)
		}
	}
	return filtered
}

// appMatchesCatalogFilter 判断目录记录是否符合运行类型、来源、架构和标签条件。
func appMatchesCatalogFilter(app appRecord, filter appCatalogSearchFilter) bool {
	if filter.typeName != "" && filter.typeName != "all" && !appMatchesRuntimeCatalog(app, filter.typeName) {
		return false
	}
	resource := strings.ToLower(appValue(app.Config, "resource", "source"))
	if resource == "" {
		resource = "remote"
	}
	if filter.resource != "" && filter.resource != "all" && resource != filter.resource {
		return false
	}
	if filter.recommendOnly && app.Recommend == 0 {
		return false
	}
	if filter.showCurrentArch && len(app.Architectures) > 0 && !appSupportsCurrentArch(app.Architectures) {
		return false
	}
	if filter.name != "" && !strings.Contains(strings.ToLower(app.Name+" "+app.Key), filter.name) {
		return false
	}
	return len(filter.tags) == 0 || appMatchesRequestedTags(app.Tags, filter.tags)
}

// appSupportsCurrentArch 判断应用声明的架构是否包含当前系统架构。
func appSupportsCurrentArch(architectures []string) bool {
	for _, arch := range architectures {
		if strings.EqualFold(strings.TrimSpace(arch), runtime.GOARCH) ||
			(runtime.GOARCH == "amd64" && strings.EqualFold(strings.TrimSpace(arch), "x86_64")) {
			return true
		}
	}
	return false
}

// appMatchesRequestedTags 判断应用是否至少命中一个请求标签。
func appMatchesRequestedTags(candidates []string, requested []any) bool {
	for _, raw := range requested {
		tag, ok := raw.(string)
		if !ok {
			continue
		}
		for _, candidate := range candidates {
			if strings.EqualFold(tag, candidate) {
				return true
			}
		}
	}
	return false
}

// appCatalogPageBounds 计算不会越过结果集的分页边界。
func appCatalogPageBounds(total, page, pageSize int) (int, int) {
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return start, end
}

// catalogSearchItems 将目录记录转换为带安装状态的本地化响应。
func (h appRouteHandlers) catalogSearchItems(catalog []appRecord, locale string) []map[string]any {
	items := make([]map[string]any, 0, len(catalog))
	for _, app := range catalog {
		item := appRecordDataLocalized(app, locale, h.store.state.CatalogTags)
		for _, installed := range h.store.state.Apps {
			if appIdentityEqual(app, installed) && appStatusCountsAsInstalled(installed.Status) {
				item["installed"] = true
				break
			}
		}
		items = append(items, item)
	}
	return items
}

// checkUpdate 返回目录中高于当前安装版本的应用清单。
func (h appRouteHandlers) checkUpdate(w http.ResponseWriter, _ *http.Request) {
	snapshot, status, err := h.loadUpdateSnapshot()
	if err != nil {
		runtimeErr(w, status, err.Error())
		return
	}
	updates := appCatalogUpdates(snapshot.installed, snapshot.catalog)
	lastSyncAt := any(nil)
	if !snapshot.syncedAt.IsZero() {
		lastSyncAt = snapshot.syncedAt
	}
	appOK(w, map[string]any{
		"canUpdate":            len(updates) > 0,
		"updates":              updates,
		"total":                len(updates),
		"isSyncing":            snapshot.syncing,
		"appStoreVersion":      snapshot.version,
		"appStoreLastModified": snapshot.lastModified,
		"lastSyncAt":           lastSyncAt,
	})
}

// loadUpdateSnapshot 刷新目录并在同一写锁周期内生成更新检查快照。
func (h appRouteHandlers) loadUpdateSnapshot() (appCatalogUpdateSnapshot, int, error) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	if err := h.store.ensureCatalogLocked(); err != nil {
		return appCatalogUpdateSnapshot{}, http.StatusInternalServerError, err
	}
	if len(h.store.state.Catalog) == 0 && strings.TrimSpace(os.Getenv("WORKMESH_APP_CATALOG")) == "" {
		if err := h.store.refreshRemoteLocked(false); err != nil {
			return appCatalogUpdateSnapshot{}, http.StatusBadGateway, fmt.Errorf("应用商店不可用: %w", err)
		}
	}
	_, err := h.store.refreshCatalogLocked()
	if err != nil {
		return appCatalogUpdateSnapshot{}, http.StatusBadGateway, err
	}
	return appCatalogUpdateSnapshot{
		catalog: append([]appRecord(nil), h.store.state.Catalog...), installed: append([]appRecord(nil), h.store.state.Apps...),
		version: h.store.state.CatalogVersion, lastModified: h.store.state.CatalogLastModified,
		syncing: h.store.state.CatalogSyncing, syncedAt: h.store.state.CatalogSyncedAt,
	}, http.StatusOK, nil
}

// appCatalogUpdates 生成可更新应用的兼容响应字段。
func appCatalogUpdates(installed, catalog []appRecord) []map[string]any {
	updates := make([]map[string]any, 0)
	for _, current := range installed {
		_, latest, ok := latestCatalogVersion(current, catalog)
		if !ok || compareAppVersion(latest.Version, current.Version) <= 0 {
			continue
		}
		updates = append(updates, map[string]any{
			"id": current.ID, "key": current.Key, "name": current.Name,
			"currentVersion": current.Version, "latestVersion": latest.Version,
			"status": current.Status, "updatedAt": latest.UpdatedAt,
		})
	}
	return updates
}

// listTags 返回当前应用目录中实际存在的本地化标签。
func (h appRouteHandlers) listTags(w http.ResponseWriter, r *http.Request) {
	h.store.mu.Lock()
	if err := h.store.ensureCatalogLocked(); err != nil {
		h.store.mu.Unlock()
		runtimeErr(w, 500, err.Error())
		return
	}
	if len(h.store.state.Catalog) == 0 && strings.TrimSpace(os.Getenv("WORKMESH_APP_CATALOG")) == "" {
		_ = h.store.refreshRemoteLocked(false)
	}
	h.store.mu.Unlock()
	h.store.mu.RLock()
	tags := catalogTags(h.store.state.Catalog, h.store.state.CatalogTags, workmeshi18n.LocaleFromRequest(r))
	h.store.mu.RUnlock()
	appOK(w, tags)
}

// catalogTags 汇总、翻译并按目录定义顺序排列应用标签。
func catalogTags(catalog []appRecord, metadata []appTagRecord, locale string) []map[string]any {
	seen := map[string]bool{}
	names := map[string]appTagRecord{}
	for _, tag := range metadata {
		names[strings.ToLower(tag.Key)] = tag
	}
	for _, item := range catalog {
		for _, tag := range item.Tags {
			if strings.TrimSpace(tag) != "" {
				seen[tag] = true
			}
		}
	}
	tags := make([]map[string]any, 0, len(seen))
	for tag := range seen {
		details := names[strings.ToLower(tag)]
		name := localizedAppText(details.Locales, locale)
		if name == "" {
			name = strings.TrimSpace(details.Name)
		}
		if name == "" {
			name = tag
		}
		tags = append(tags, map[string]any{"key": tag, "name": name, "sort": details.Sort})
	}
	sort.Slice(tags, func(i, j int) bool {
		leftSort, _ := tags[i]["sort"].(int)
		rightSort, _ := tags[j]["sort"].(int)
		if leftSort != rightSort {
			return leftSort < rightSort
		}
		return strings.ToLower(tags[i]["key"].(string)) < strings.ToLower(tags[j]["key"].(string))
	})
	return tags
}

// ignoredDetail 返回 SQLite 仓储中的应用更新忽略项。
func (h appRouteHandlers) ignoredDetail(w http.ResponseWriter, _ *http.Request) {
	h.store.mu.RLock()
	items := append([]map[string]any(nil), h.store.state.Ignored...)
	h.store.mu.RUnlock()
	appOK(w, items)
}

// catalogByKey 按应用键返回目录详情。
func (h appRouteHandlers) catalogByKey(w http.ResponseWriter, r *http.Request) {
	appCatalogGet(w, h.store, r, r.PathValue("key"), "")
}

// catalogByID 按应用标识返回目录详情。
func (h appRouteHandlers) catalogByID(w http.ResponseWriter, r *http.Request) {
	appCatalogGet(w, h.store, r, r.PathValue("id"), "")
}

// catalogDetail 按路径参数名称返回指定版本的目录详情。
func (h appRouteHandlers) catalogDetail(w http.ResponseWriter, r *http.Request, idParam string) {
	appCatalogGet(w, h.store, r, r.PathValue(idParam), r.PathValue("version"))
}

// services 返回应用关联的真实容器服务状态。
func (h appRouteHandlers) services(w http.ResponseWriter, r *http.Request) {
	appOK(w, appServices(r.Context(), h.store, r.PathValue("key")))
}

// icon 返回应用目录中保存或远程缓存的图标资源。
func (h appRouteHandlers) icon(w http.ResponseWriter, r *http.Request) {
	appIcon(w, h.store, r.PathValue("key"))
}

// installedInfo 返回已安装应用详情或参数。
func (h appRouteHandlers) installedInfo(w http.ResponseWriter, r *http.Request) {
	appInstalledGet(w, h.store, r, r.PathValue("appInstallId"))
}

// deleteCheck 返回删除应用前需要处理的真实关联资源。
func (h appRouteHandlers) deleteCheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("appInstallId")
	h.store.mu.RLock()
	_, item := findApp(h.store.state.Apps, id)
	h.store.mu.RUnlock()
	resources := make([]map[string]any, 0)
	if item.ID != "" {
		resources = append(resources, map[string]any{"type": "app", "id": item.ID, "name": item.Name, "status": item.Status})
		if container := appValue(item.Config, "containerName"); container != "" {
			resources = append(resources, map[string]any{"type": "container", "name": container})
		}
	}
	appOK(w, map[string]any{"appInstallId": id, "exists": item.ID != "", "canDelete": item.ID != "", "resources": resources})
}

// post 根据实际请求路径分派应用安装与生命周期操作。
func (h appRouteHandlers) post(w http.ResponseWriter, r *http.Request) {
	handleAppPost(w, h.store, r, strings.TrimPrefix(r.URL.Path, "/api/v2/apps/"), appBody(r))
}

// customSync 从配置的真实来源同步自定义应用目录。
func (h appRouteHandlers) customSync(w http.ResponseWriter, r *http.Request) {
	handleCustomAppSync(w, r, h.store)
}

// customConfig 返回当前自定义应用商店配置。
func (h appRouteHandlers) customConfig(w http.ResponseWriter, _ *http.Request) {
	h.store.mu.RLock()
	config := h.store.state.StoreConfig
	h.store.mu.RUnlock()
	appOK(w, config)
}

// crossNodeInstall 复用应用安装逻辑处理跨节点安装请求。
func (h appRouteHandlers) crossNodeInstall(w http.ResponseWriter, r *http.Request) {
	handleAppPost(w, h.store, r, "install", appBody(r))
}
