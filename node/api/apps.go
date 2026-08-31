// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/service"
)

// appRecord 保存一个已安装应用及其运行参数；应用目录记录使用相同结构以减少常驻内存。
type appRecord struct {
	ID        string         `json:"id"`
	Key       string         `json:"key"`
	Name      string         `json:"name"`
	Version   string         `json:"version"`
	Status    string         `json:"status"`
	Config    map[string]any `json:"config,omitempty"`
	SortOrder int            `json:"sortOrder,omitempty"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

// appCatalogDocument 描述本地应用目录文件，允许目录同时携带版本和同步元数据。
type appCatalogDocument struct {
	Apps         []appRecord `json:"apps"`
	Catalog      []appRecord `json:"catalog"`
	Version      string      `json:"version"`
	LastModified int64       `json:"lastModified"`
	IsSyncing    bool        `json:"isSyncing"`
}

type appStoreState struct {
	Apps                []appRecord      `json:"apps"`
	Catalog             []appRecord      `json:"catalog"`
	Ignored             []map[string]any `json:"ignored"`
	StoreConfig         map[string]any   `json:"storeConfig"`
	CatalogVersion      string           `json:"catalogVersion,omitempty"`
	CatalogLastModified int64            `json:"catalogLastModified,omitempty"`
	CatalogSyncing      bool             `json:"catalogSyncing,omitempty"`
	CatalogSyncedAt     time.Time        `json:"catalogSyncedAt,omitempty"`
}

type appStore struct {
	mu             sync.RWMutex
	path           string
	state          appStoreState
	catalogPath    string
	catalogModTime time.Time
	catalogSize    int64
}

var appStoreMu sync.Mutex
var appStoreInstance *appStore

func getAppStore() *appStore {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = "./data"
	}
	path := filepath.Join(dir, "apps.json")
	appStoreMu.Lock()
	defer appStoreMu.Unlock()
	if appStoreInstance != nil && appStoreInstance.path == path {
		return appStoreInstance
	}
	state := appStoreState{StoreConfig: map[string]any{"uninstallDeleteImage": "false", "uninstallDeleteBackup": "false", "upgradeBackup": "true", "upgradeDeleteImage": "false", "installAllowPort": "true"}}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &state)
	}
	if state.StoreConfig == nil {
		state.StoreConfig = map[string]any{}
	}
	if state.Catalog == nil {
		state.Catalog = append([]appRecord(nil), state.Apps...)
	}
	if state.Ignored == nil {
		state.Ignored = make([]map[string]any, 0)
	}
	appStoreInstance = &appStore{path: path, state: state}
	return appStoreInstance
}

func (s *appStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	b, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err = os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func appBody(r *http.Request) map[string]any {
	var value map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&value)
	}
	if value == nil {
		value = map[string]any{}
	}
	return value
}

func appValue(v map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := v[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		if value, ok := v[key].(float64); ok {
			return strconv.FormatInt(int64(value), 10)
		}
	}
	return ""
}

func appOK(w http.ResponseWriter, d any) { runtimeOK(w, d) }

func appRecordData(a appRecord) map[string]any {
	id := a.ID
	// 默认目录记录没有已安装版本上下文，调用方会在有目录时覆写该字段。
	canUpdate := false
	item := map[string]any{"id": id, "key": a.Key, "name": a.Name, "version": a.Version, "status": a.Status, "appKey": a.Key, "appName": a.Name, "appStatus": a.Status, "ready": 1, "total": 1, "canUpdate": canUpdate, "favorite": false, "sortOrder": a.SortOrder, "updatedAt": a.UpdatedAt, "config": a.Config}
	if n, err := strconv.ParseInt(id, 10, 64); err == nil {
		item["id"] = n
	}
	return item
}

// appRecordDataWithCatalog 在输出已安装应用时根据目录中的最高版本计算升级状态。
func appRecordDataWithCatalog(a appRecord, catalog []appRecord) map[string]any {
	item := appRecordData(a)
	_, latest, ok := latestCatalogVersion(a, catalog)
	item["canUpdate"] = ok && compareAppVersion(latest.Version, a.Version) > 0
	return item
}

func findApp(items []appRecord, id string) (int, appRecord) {
	for index, item := range items {
		if item.ID == id || item.Key == id || item.Name == id {
			return index, item
		}
	}
	return -1, appRecord{}
}

func appCatalogFromEnv() []appRecord {
	if file := strings.TrimSpace(os.Getenv("WORKMESH_APP_CATALOG")); file != "" {
		result, _, _, err := loadAppCatalogFile(file)
		if err == nil {
			return result
		}
	}
	return nil
}

// compareAppVersion 比较常见的语义化版本，返回值大于零表示 a 更新。
// 预发布版本低于同一主版本的正式版本；无法解析时使用不区分大小写的字典序。
func compareAppVersion(a, b string) int {
	parse := func(value string) (core []int, pre []string, valid bool) {
		value = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(value, "v"), "V"))
		if value == "" {
			return nil, nil, false
		}
		value = strings.SplitN(value, "+", 2)[0]
		parts := strings.SplitN(value, "-", 2)
		for _, part := range strings.Split(parts[0], ".") {
			if part == "" {
				return nil, nil, false
			}
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 {
				return nil, nil, false
			}
			core = append(core, n)
		}
		if len(parts) == 2 {
			for _, id := range strings.Split(parts[1], ".") {
				if id == "" {
					return nil, nil, false
				}
				pre = append(pre, id)
			}
		}
		return core, pre, true
	}
	ac, ap, av := parse(a)
	bc, bp, bv := parse(b)
	if !av || !bv {
		return strings.Compare(strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b)))
	}
	for i := 0; i < len(ac) || i < len(bc); i++ {
		an, bn := 0, 0
		if i < len(ac) {
			an = ac[i]
		}
		if i < len(bc) {
			bn = bc[i]
		}
		if an != bn {
			if an > bn {
				return 1
			}
			return -1
		}
	}
	if len(ap) == 0 && len(bp) == 0 {
		return 0
	}
	if len(ap) == 0 {
		return 1
	}
	if len(bp) == 0 {
		return -1
	}
	for i := 0; i < len(ap) && i < len(bp); i++ {
		ai, aerr := strconv.Atoi(ap[i])
		bi, berr := strconv.Atoi(bp[i])
		if aerr == nil && berr == nil && ai != bi {
			if ai > bi {
				return 1
			}
			return -1
		}
		if aerr == nil && berr != nil {
			return -1
		}
		if aerr != nil && berr == nil {
			return 1
		}
		if cmp := strings.Compare(ap[i], bp[i]); cmp != 0 {
			return cmp
		}
	}
	if len(ap) > len(bp) {
		return 1
	}
	if len(ap) < len(bp) {
		return -1
	}
	return 0
}

// versionGreater 保留内部调用兼容性，使用语义化版本比较结果。
func versionGreater(latest, current string) bool { return compareAppVersion(latest, current) > 0 }

func appIdentityEqual(a, b appRecord) bool {
	for _, left := range []string{a.Key, a.ID, a.Name} {
		if strings.TrimSpace(left) == "" {
			continue
		}
		for _, right := range []string{b.Key, b.ID, b.Name} {
			if strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right)) {
				return true
			}
		}
	}
	return false
}

func latestCatalogVersion(app appRecord, catalog []appRecord) (appRecord, appRecord, bool) {
	var latest appRecord
	found := false
	for _, candidate := range catalog {
		if !appIdentityEqual(app, candidate) || strings.TrimSpace(candidate.Version) == "" {
			continue
		}
		if !found || compareAppVersion(candidate.Version, latest.Version) > 0 {
			latest = candidate
			found = true
		}
	}
	return app, latest, found
}

const maxAppCatalogBytes = 8 << 20

// loadAppCatalogFile 读取有界应用目录，避免配置文件异常增长导致内存占用失控。
func loadAppCatalogFile(path string) ([]appRecord, appCatalogDocument, os.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, appCatalogDocument{}, nil, fmt.Errorf("打开应用目录 %q 失败: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, appCatalogDocument{}, nil, fmt.Errorf("读取应用目录 %q 属性失败: %w", path, err)
	}
	if info.Size() > maxAppCatalogBytes {
		return nil, appCatalogDocument{}, nil, fmt.Errorf("应用目录 %q 超过 %d 字节限制", path, maxAppCatalogBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxAppCatalogBytes+1))
	if err != nil {
		return nil, appCatalogDocument{}, nil, fmt.Errorf("读取应用目录 %q 失败: %w", path, err)
	}
	var records []appRecord
	if err := json.Unmarshal(data, &records); err == nil {
		return records, appCatalogDocument{Apps: records}, info, nil
	}
	var doc appCatalogDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, appCatalogDocument{}, nil, fmt.Errorf("解析应用目录 %q 失败: %w", path, err)
	}
	if len(doc.Catalog) > 0 {
		records = doc.Catalog
	} else {
		records = doc.Apps
	}
	return records, doc, info, nil
}

// refreshCatalogLocked 在目录文件发生变化时刷新缓存，并记录同步元数据。
// 调用方必须持有 s.mu 写锁；没有配置目录时保留已持久化的 catalog。
func (s *appStore) refreshCatalogLocked() (bool, error) {
	path := strings.TrimSpace(os.Getenv("WORKMESH_APP_CATALOG"))
	if path == "" {
		if len(s.state.Catalog) == 0 && len(s.state.Apps) > 0 {
			s.state.Catalog = append([]appRecord(nil), s.state.Apps...)
			return true, nil
		}
		return false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("应用目录不可用 %q: %w", path, err)
	}
	if s.catalogPath == path && s.catalogSize == info.Size() && s.catalogModTime.Equal(info.ModTime()) {
		return false, nil
	}
	records, doc, _, err := loadAppCatalogFile(path)
	if err != nil {
		return false, err
	}
	s.state.Catalog = records
	s.state.CatalogVersion = strings.TrimSpace(doc.Version)
	s.state.CatalogLastModified = doc.LastModified
	if s.state.CatalogLastModified == 0 {
		s.state.CatalogLastModified = info.ModTime().Unix()
	}
	s.state.CatalogSyncing = doc.IsSyncing
	s.state.CatalogSyncedAt = time.Now().UTC()
	s.catalogPath, s.catalogSize, s.catalogModTime = path, info.Size(), info.ModTime()
	return true, nil
}

// RegisterAppRoutes 注册应用目录与已安装应用接口，所有写操作都会原子持久化到 apps.json。
func RegisterAppRoutes(mux *http.ServeMux) {
	s := getAppStore()
	listInstalled := func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		catalog := append([]appRecord(nil), s.state.Catalog...)
		items := make([]map[string]any, 0, len(s.state.Apps))
		for _, app := range s.state.Apps {
			items = append(items, appRecordDataWithCatalog(app, catalog))
		}
		s.mu.RUnlock()
		if r.Method == http.MethodGet {
			appOK(w, items)
			return
		}
		appOK(w, map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": 50})
	}
	searchCatalog := func(w http.ResponseWriter, r *http.Request) {
		body := appBody(r)
		name := strings.ToLower(appValue(body, "name", "key"))
		s.mu.Lock()
		if len(s.state.Catalog) == 0 {
			s.state.Catalog = appCatalogFromEnv()
			if len(s.state.Catalog) == 0 {
				s.state.Catalog = append([]appRecord(nil), s.state.Apps...)
			}
		}
		items := make([]map[string]any, 0, len(s.state.Catalog))
		for _, app := range s.state.Catalog {
			if name != "" && !strings.Contains(strings.ToLower(app.Name+" "+app.Key), name) {
				continue
			}
			items = append(items, appRecordData(app))
		}
		_ = s.saveLocked()
		s.mu.Unlock()
		appOK(w, map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": 50})
	}
	for _, path := range []string{"/api/v2/apps/installed/list", "/api/v2/apps/installed/search"} {
		mux.HandleFunc("GET "+path, listInstalled)
		mux.HandleFunc("POST "+path, listInstalled)
	}
	for _, path := range []string{"/api/v2/apps/search", "/api/v2/apps/sync/local", "/api/v2/apps/sync/remote"} {
		mux.HandleFunc("POST "+path, searchCatalog)
	}
	mux.HandleFunc("GET /api/v2/apps/checkupdate", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		changed, err := s.refreshCatalogLocked()
		if err != nil {
			s.mu.Unlock()
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		catalog := append([]appRecord(nil), s.state.Catalog...)
		installed := append([]appRecord(nil), s.state.Apps...)
		meta := struct {
			version      string
			lastModified int64
			syncing      bool
			syncedAt     time.Time
		}{s.state.CatalogVersion, s.state.CatalogLastModified, s.state.CatalogSyncing, s.state.CatalogSyncedAt}
		if changed {
			if err := s.saveLocked(); err != nil {
				s.mu.Unlock()
				runtimeErr(w, http.StatusInternalServerError, fmt.Sprintf("保存应用目录元数据失败: %v", err))
				return
			}
		}
		s.mu.Unlock()

		updates := make([]map[string]any, 0)
		for _, current := range installed {
			_, latest, ok := latestCatalogVersion(current, catalog)
			if !ok || compareAppVersion(latest.Version, current.Version) <= 0 {
				continue
			}
			updates = append(updates, map[string]any{
				"id":             current.ID,
				"key":            current.Key,
				"name":           current.Name,
				"currentVersion": current.Version,
				"latestVersion":  latest.Version,
				"status":         current.Status,
				"updatedAt":      latest.UpdatedAt,
			})
		}
		lastSyncAt := any(nil)
		if !meta.syncedAt.IsZero() {
			lastSyncAt = meta.syncedAt
		}
		appOK(w, map[string]any{
			"canUpdate":            len(updates) > 0,
			"updates":              updates,
			"total":                len(updates),
			"isSyncing":            meta.syncing,
			"appStoreVersion":      meta.version,
			"appStoreLastModified": meta.lastModified,
			"lastSyncAt":           lastSyncAt,
		})
	})
	mux.HandleFunc("GET /api/v2/apps/tags", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		seen := map[string]bool{}
		for _, item := range s.state.Catalog {
			if item.Key != "" {
				seen[item.Key] = true
			}
		}
		tags := make([]string, 0, len(seen))
		for tag := range seen {
			tags = append(tags, tag)
		}
		s.mu.RUnlock()
		appOK(w, tags)
	})
	mux.HandleFunc("GET /api/v2/apps/ignored/detail", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		items := append([]map[string]any(nil), s.state.Ignored...)
		s.mu.RUnlock()
		appOK(w, items)
	})
	mux.HandleFunc("GET /api/v2/apps/{key}", func(w http.ResponseWriter, r *http.Request) { appCatalogGet(w, s, r.PathValue("key")) })
	mux.HandleFunc("GET /api/v2/apps/detail/{appId}/{version}/{type}", func(w http.ResponseWriter, r *http.Request) { appCatalogGet(w, s, r.PathValue("appId")) })
	mux.HandleFunc("GET /api/v2/apps/detail/node/{appKey}/{version}", func(w http.ResponseWriter, r *http.Request) { appCatalogGet(w, s, r.PathValue("appKey")) })
	mux.HandleFunc("GET /api/v2/apps/details/{id}", func(w http.ResponseWriter, r *http.Request) { appCatalogGet(w, s, r.PathValue("id")) })
	mux.HandleFunc("GET /api/v2/apps/services/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		s.mu.RLock()
		_, item := findApp(s.state.Apps, key)
		s.mu.RUnlock()
		services := make([]map[string]any, 0)
		if item.ID != "" {
			if raw, ok := item.Config["services"].([]any); ok {
				for _, service := range raw {
					if value, ok := service.(map[string]any); ok {
						services = append(services, value)
					}
				}
			}
			if len(services) == 0 {
				services = append(services, map[string]any{"label": item.Name, "value": item.Key, "status": item.Status, "appKey": item.Key})
			}
		}
		appOK(w, services)
	})
	mux.HandleFunc("GET /api/v2/apps/icon/{key}", appIcon)
	mux.HandleFunc("GET /api/v2/apps/installed/info/{appInstallId}", func(w http.ResponseWriter, r *http.Request) { appInstalledGet(w, s, r.PathValue("appInstallId")) })
	mux.HandleFunc("GET /api/v2/apps/installed/params/{appInstallId}", func(w http.ResponseWriter, r *http.Request) { appInstalledGet(w, s, r.PathValue("appInstallId")) })
	mux.HandleFunc("GET /api/v2/apps/installed/delete/check/{appInstallId}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("appInstallId")
		s.mu.RLock()
		_, item := findApp(s.state.Apps, id)
		s.mu.RUnlock()
		resources := make([]map[string]any, 0)
		if item.ID != "" {
			resources = append(resources, map[string]any{"type": "app", "id": item.ID, "name": item.Name, "status": item.Status})
			if container := appValue(item.Config, "containerName"); container != "" {
				resources = append(resources, map[string]any{"type": "container", "name": container})
			}
		}
		appOK(w, map[string]any{"appInstallId": id, "exists": item.ID != "", "canDelete": item.ID != "", "resources": resources})
	})
	for _, path := range []string{"/api/v2/apps/install", "/api/v2/apps/installed/check", "/api/v2/apps/installed/conf", "/api/v2/apps/installed/config/update", "/api/v2/apps/installed/conninfo", "/api/v2/apps/installed/ignore", "/api/v2/apps/installed/loadport", "/api/v2/apps/installed/op", "/api/v2/apps/installed/params/update", "/api/v2/apps/installed/port/change", "/api/v2/apps/installed/sort/update", "/api/v2/apps/installed/sync", "/api/v2/apps/installed/update/versions", "/api/v2/apps/ignored/cancel"} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			handleAppPost(w, s, strings.TrimPrefix(r.URL.Path, "/api/v2/apps/"), appBody(r))
		})
	}
	// 自定义应用商店和跨节点安装是前端真实调用的扩展接口。
	mux.HandleFunc("POST /api/v2/custom/app/sync", func(w http.ResponseWriter, r *http.Request) {
		appOK(w, map[string]any{"accepted": true, "taskID": appValue(appBody(r), "taskID")})
	})
	mux.HandleFunc("GET /api/v2/custom/app/config", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		config := s.state.StoreConfig
		s.mu.RUnlock()
		appOK(w, config)
	})
	mux.HandleFunc("POST /api/v2/core/xpack/sync/app/install", func(w http.ResponseWriter, r *http.Request) { handleAppPost(w, s, "install", appBody(r)) })
}

func appCatalogGet(w http.ResponseWriter, s *appStore, id string) {
	s.mu.RLock()
	index, item := findApp(s.state.Catalog, id)
	if index < 0 {
		index, item = findApp(s.state.Apps, id)
	}
	s.mu.RUnlock()
	if index < 0 {
		appOK(w, map[string]any{"id": id, "key": id, "available": false})
		return
	}
	data := appRecordData(item)
	data["available"] = true
	params := any(map[string]any{})
	if item.Config != nil {
		if configured, ok := item.Config["params"]; ok {
			params = configured
		}
	}
	data["details"] = map[string]any{"version": item.Version, "type": "runtime", "params": params, "dockerCompose": appValue(item.Config, "dockerCompose", "compose")}
	appOK(w, data)
}

func appInstalledGet(w http.ResponseWriter, s *appStore, id string) {
	s.mu.RLock()
	_, item := findApp(s.state.Apps, id)
	s.mu.RUnlock()
	if item.ID == "" {
		appOK(w, map[string]any{"id": id, "status": "not_installed", "env": map[string]any{}})
		return
	}
	data := appRecordData(item)
	data["env"] = item.Config
	data["container"] = item.Config["containerName"]
	data["httpPort"] = item.Config["port"]
	appOK(w, data)
}

func handleAppPost(w http.ResponseWriter, s *appStore, path string, body map[string]any) {
	switch path {
	case "install":
		id := appValue(body, "appInstallId", "id", "appId", "key", "name")
		if id == "" {
			runtimeErr(w, http.StatusBadRequest, "应用标识不能为空")
			return
		}
		item := appRecord{ID: id, Key: appValue(body, "key", "appKey", "id"), Name: appValue(body, "name", "appName", "key"), Version: appValue(body, "version"), Status: "running", Config: body, UpdatedAt: time.Now().UTC()}
		if item.Key == "" {
			item.Key = id
		}
		s.mu.Lock()
		index, _ := findApp(s.state.Apps, id)
		if index >= 0 {
			s.state.Apps[index] = item
		} else {
			s.state.Apps = append(s.state.Apps, item)
		}
		_ = s.saveLocked()
		s.mu.Unlock()
		appOK(w, appRecordData(item))
	case "installed/check":
		id := appValue(body, "name", "key", "appInstallId")
		s.mu.RLock()
		_, item := findApp(s.state.Apps, id)
		s.mu.RUnlock()
		// 对核心运行环境使用本机探测器，避免仅依赖历史登记记录导致“已安装”被误报。
		probe := service.ProbeApplication(context.Background(), appValue(body, "key", "app", "type"), id)
		if probe.App == "" {
			probe.App = id
		}
		// 未知应用（或容器化应用没有宿主二进制）仍保留已登记记录作为可信状态来源。
		if item.ID != "" && !probe.IsExist && (probe.Error == "未配置该应用的本机探测器" || appValue(item.Config, "containerName") != "") {
			probe.IsExist, probe.IsActive, probe.Status = true, item.Status == "running", item.Status
			probe.Version = item.Version
		}
		containerName := item.Config["containerName"]
		if containerName == nil && strings.HasPrefix(probe.Binary, "docker://") {
			containerName = strings.TrimPrefix(probe.Binary, "docker://")
		}
		data := map[string]any{"name": id, "version": probe.Version, "isExist": probe.IsExist, "isActive": probe.IsActive, "status": probe.Status, "app": probe.App, "appInstallId": item.ID, "containerName": containerName, "httpPort": 80, "httpsPort": 443, "websiteDir": "/www/wwwroot"}
		if probe.Error != "" {
			data["error"] = probe.Error
		}
		appOK(w, data)
	case "installed/loadport":
		id := appValue(body, "name", "key")
		s.mu.RLock()
		_, item := findApp(s.state.Apps, id)
		s.mu.RUnlock()
		appOK(w, item.Config["port"])
	case "installed/conninfo":
		appOK(w, map[string]any{"status": "unknown", "username": "", "password": "", "privilege": false, "containerName": appValue(body, "name"), "serviceName": appValue(body, "name"), "systemIP": "127.0.0.1", "port": 0})
	case "installed/conf":
		appOK(w, map[string]any{"type": appValue(body, "type"), "name": appValue(body, "name"), "params": make([]any, 0), "dockerCompose": ""})
	case "installed/op":
		handleAppOperation(w, s, body)
	case "installed/port/change", "installed/params/update", "installed/config/update":
		handleAppUpdate(w, s, body)
	case "installed/sort/update":
		items, _ := body["items"].([]any)
		s.mu.Lock()
		for _, value := range items {
			if item, ok := value.(map[string]any); ok {
				id := appValue(item, "installID", "id")
				if index, _ := findApp(s.state.Apps, id); index >= 0 {
					if order, ok := item["sortOrder"].(float64); ok {
						s.state.Apps[index].SortOrder = int(order)
					}
				}
			}
		}
		_ = s.saveLocked()
		s.mu.Unlock()
		appOK(w, map[string]any{"updated": true})
	case "installed/update/versions":
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
	case "installed/ignore":
		s.mu.Lock()
		s.state.Ignored = append(s.state.Ignored, body)
		_ = s.saveLocked()
		s.mu.Unlock()
		appOK(w, map[string]any{"ignored": true})
	case "ignored/cancel":
		id := appValue(body, "id", "appID", "appDetailID")
		s.mu.Lock()
		kept := s.state.Ignored[:0]
		for _, item := range s.state.Ignored {
			if appValue(item, "id", "appID", "appDetailID") != id {
				kept = append(kept, item)
			}
		}
		s.state.Ignored = kept
		_ = s.saveLocked()
		s.mu.Unlock()
		appOK(w, map[string]any{"cancelled": true})
	default:
		appOK(w, map[string]any{"accepted": true, "config": body})
	}
}

func handleAppOperation(w http.ResponseWriter, s *appStore, body map[string]any) {
	id := appValue(body, "installId", "appInstallId", "id")
	operation := strings.ToLower(appValue(body, "operate", "operation"))
	if id == "" {
		runtimeErr(w, http.StatusBadRequest, "应用安装标识不能为空")
		return
	}
	if operation == "" {
		runtimeErr(w, http.StatusBadRequest, "应用操作不能为空")
		return
	}
	s.mu.Lock()
	index, item := findApp(s.state.Apps, id)
	if index < 0 {
		s.mu.Unlock()
		runtimeErr(w, http.StatusNotFound, "应用不存在: "+id)
		return
	}
	remove := false
	switch operation {
	case "stop", "停止":
		item.Status = "stopped"
	case "start", "启动", "restart", "重启":
		item.Status = "running"
	case "uninstall", "delete", "卸载":
		remove = true
	default:
		s.mu.Unlock()
		runtimeErr(w, http.StatusBadRequest, "不支持的应用操作: "+operation)
		return
	}
	item.UpdatedAt = time.Now().UTC()
	if remove {
		s.state.Apps = append(s.state.Apps[:index], s.state.Apps[index+1:]...)
	} else {
		s.state.Apps[index] = item
	}
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		runtimeErr(w, http.StatusInternalServerError, "保存应用状态失败: "+err.Error())
		return
	}
	s.mu.Unlock()
	result := map[string]any{"id": id, "operate": operation, "status": item.Status, "accepted": true}
	if remove {
		result["status"] = "uninstalled"
	}
	appOK(w, result)
}

func handleAppUpdate(w http.ResponseWriter, s *appStore, body map[string]any) {
	id := appValue(body, "installID", "installId", "appInstallId", "id")
	s.mu.Lock()
	index, item := findApp(s.state.Apps, id)
	if index >= 0 {
		if port, ok := body["port"]; ok {
			if item.Config == nil {
				item.Config = map[string]any{}
			}
			item.Config["port"] = port
		}
		for key, value := range body {
			if item.Config == nil {
				item.Config = map[string]any{}
			}
			item.Config[key] = value
		}
		item.UpdatedAt = time.Now().UTC()
		s.state.Apps[index] = item
	}
	_ = s.saveLocked()
	s.mu.Unlock()
	appOK(w, map[string]any{"id": id, "updated": index >= 0})
}

func appIcon(w http.ResponseWriter, _ *http.Request) {
	// 透明 1x1 PNG：目录没有图标时也返回真实图片响应，避免浏览器将 JSON 当脚本解析。
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func isAppRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	p := pattern
	if len(parts) == 2 {
		p = parts[1]
	}
	return p == "/api/v2/apps" || strings.HasPrefix(p, "/api/v2/apps/")
}
