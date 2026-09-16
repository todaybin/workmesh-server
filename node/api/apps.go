// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	workmeshi18n "github.com/todaybin/workmesh-server/i18n"
	"github.com/todaybin/workmesh-server/node/service"
)

// appRecord 保存一个已安装应用及其运行参数；应用目录记录使用相同结构以减少常驻内存。
type appRecord struct {
	ID                  string             `json:"id"`
	Key                 string             `json:"key"`
	Name                string             `json:"name"`
	Version             string             `json:"version"`
	Status              string             `json:"status"`
	Message             string             `json:"message,omitempty"`
	ContainerName       string             `json:"containerName,omitempty"`
	Config              map[string]any     `json:"config,omitempty"`
	SortOrder           int                `json:"sortOrder,omitempty"`
	UpdatedAt           time.Time          `json:"updatedAt"`
	Type                string             `json:"type,omitempty"`
	Description         any                `json:"description,omitempty"`
	ShortDescZh         string             `json:"shortDescZh,omitempty"`
	ShortDescEn         string             `json:"shortDescEn,omitempty"`
	Tags                []string           `json:"tags,omitempty"`
	Limit               int                `json:"limit,omitempty"`
	Recommend           int                `json:"recommend,omitempty"`
	GpuSupport          bool               `json:"gpuSupport,omitempty"`
	BatchInstallSupport bool               `json:"batchInstallSupport,omitempty"`
	Architectures       []string           `json:"architectures,omitempty"`
	MemoryRequired      int                `json:"memoryRequired,omitempty"`
	Website             string             `json:"website,omitempty"`
	Github              string             `json:"github,omitempty"`
	ReadMe              string             `json:"readMe,omitempty"`
	IconURL             string             `json:"iconUrl,omitempty"`
	Versions            []appVersionRecord `json:"versions,omitempty"`
}

type appVersionRecord struct {
	ID            string         `json:"id"`
	Version       string         `json:"version"`
	DownloadURL   string         `json:"downloadUrl,omitempty"`
	ComposeURL    string         `json:"composeUrl,omitempty"`
	DockerCompose string         `json:"dockerCompose,omitempty"`
	Params        map[string]any `json:"params,omitempty"`
	LastModified  int            `json:"lastModified,omitempty"`
}

// cloneAppRecord 深复制应用记录，避免后台安装任务与 HTTP 响应共享可变 map 或 slice。
func cloneAppRecord(source appRecord) appRecord {
	raw, err := json.Marshal(source)
	if err != nil {
		return source
	}
	var clone appRecord
	if err := json.Unmarshal(raw, &clone); err != nil {
		return source
	}
	return clone
}

type remoteAppList struct {
	Apps  []remoteAppDefine `json:"apps"`
	Extra struct {
		Version string `json:"version"`
		Tags    []struct {
			Key     string            `json:"key"`
			Name    string            `json:"name"`
			Sort    int               `json:"sort"`
			Locales map[string]string `json:"locales"`
		} `json:"tags"`
	} `json:"additionalProperties"`
	LastModified int `json:"lastModified"`
}

type remoteAppDefine struct {
	Icon         string `json:"icon"`
	Name         string `json:"name"`
	ReadMe       string `json:"readMe"`
	LastModified int    `json:"lastModified"`
	Property     struct {
		Name                string   `json:"name"`
		Type                string   `json:"type"`
		Tags                []string `json:"tags"`
		ShortDescZh         string   `json:"shortDescZh"`
		ShortDescEn         string   `json:"shortDescEn"`
		Description         any      `json:"description"`
		Key                 string   `json:"key"`
		Limit               int      `json:"limit"`
		Recommend           int      `json:"recommend"`
		Website             string   `json:"website"`
		Github              string   `json:"github"`
		Architectures       []string `json:"architectures"`
		MemoryRequired      int      `json:"memoryRequired"`
		GpuSupport          bool     `json:"gpuSupport"`
		BatchInstallSupport bool     `json:"batchInstallSupport"`
	} `json:"additionalProperties"`
	Versions []struct {
		Name         string         `json:"name"`
		LastModified int            `json:"lastModified"`
		DownloadURL  string         `json:"downloadUrl"`
		AppForm      map[string]any `json:"additionalProperties"`
	} `json:"versions"`
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
	CatalogTags         []appTagRecord   `json:"catalogTags,omitempty"`
}

type appTagRecord struct {
	Key     string            `json:"key"`
	Name    string            `json:"name"`
	Sort    int               `json:"sort"`
	Locales map[string]string `json:"locales,omitempty"`
}

type appStore struct {
	mu              sync.RWMutex
	path            string
	state           appStoreState
	catalogPath     string
	catalogModTime  time.Time
	catalogSize     int64
	containerStates func(context.Context, []string) (map[string]string, error)
}

var appStoreMu sync.Mutex
var appStoreInstance *appStore

// getAppStore 获取按数据目录隔离的应用状态仓库，并加载 SQLite 快照或兼容文件。
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
	// 生产进程始终由共享 SQLite 提供状态；旧 apps.json 只允许在未接入
	// SQLite 的迁移/单元测试场景读取，避免陈旧文件覆盖真实数据库状态。
	if sharedDB() == nil {
		if data, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(data, &state)
		}
	} else {
		_ = loadJSONState("app_store_state", &state)
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
	store := &appStore{path: path, state: state, containerStates: service.NewDockerService().ContainerStates}
	migrated := false
	for index := range store.state.Apps {
		item := &store.state.Apps[index]
		if item.ContainerName == "" {
			item.ContainerName = appConfiguredContainerName(*item)
			migrated = migrated || item.ContainerName != ""
		}
		normalized := normalizeAppStatus(item.Status)
		if normalized != item.Status {
			item.Status = normalized
			migrated = true
		}
	}
	if migrated {
		_ = store.saveLocked()
	}
	appStoreInstance = store
	return appStoreInstance
}

// saveLocked 在持有仓库锁时持久化应用状态，并同步关系表中的应用记录。
func (s *appStore) saveLocked() error {
	// 公共 SQLite 初始化后只写数据库，旧 apps.json 仅作为一次性迁移输入。
	if sharedDB() == nil {
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
		if err := os.Rename(tmp, s.path); err != nil {
			return err
		}
		return nil
	}
	if err := saveJSONState("app_store_state", s.state); err != nil {
		return err
	}
	return persistAppRelational(s.state)
}

// appBody 解码应用接口请求体；空请求体按兼容约定返回空对象。
// 应用写接口必须拒绝畸形 JSON 和尾随 JSON；调用方收到 nil 后会按缺少应用标识返回 400，
// 保持现有 handler 签名和 v2 错误 envelope，同时避免把无效请求当成空对象执行。
func appBody(r *http.Request) map[string]any {
	var value map[string]any
	if r == nil || r.Body == nil {
		return map[string]any{}
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	if err := decoder.Decode(&value); err != nil {
		if errors.Is(err, io.EOF) {
			return map[string]any{}
		}
		return nil
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil
	}
	if value == nil {
		return nil
	}
	return value
}

// decodeAppSearchBody parses the search contract strictly.  The legacy
// handlers intentionally accept an empty body, but search requests must not
// silently turn malformed JSON into an unfiltered query.
// decodeAppSearchBody 严格解析应用搜索请求，拒绝多余 JSON 或非对象内容。
func decodeAppSearchBody(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return map[string]any{}, nil
	}
	var value map[string]any
	decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	if err := decoder.Decode(&value); err != nil {
		if errors.Is(err, io.EOF) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("请求 JSON 无效: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("请求 JSON 只能包含一个对象")
	}
	if value == nil {
		return nil, errors.New("请求 JSON 必须是对象")
	}
	return value, nil
}

// appSearchPage 校验搜索分页参数并限制单页最大数量，避免无界读取。
func appSearchPage(body map[string]any) (int, int, error) {
	page, pageSize := 1, 50
	parse := func(key string, fallback int) (int, error) {
		raw, ok := body[key]
		if !ok || raw == nil {
			return fallback, nil
		}
		value, ok := raw.(float64)
		if !ok || value < 1 || value != float64(int(value)) || value > 10000 {
			return 0, fmt.Errorf("%s 必须是正整数", key)
		}
		return int(value), nil
	}
	var err error
	if page, err = parse("page", page); err != nil {
		return 0, 0, err
	}
	if pageSize, err = parse("pageSize", pageSize); err != nil {
		return 0, 0, err
	}
	if pageSize > 200 {
		return 0, 0, errors.New("pageSize 不能超过 200")
	}
	return page, pageSize, nil
}

// appValue 按候选字段顺序读取字符串或数字形式的应用属性。
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

// normalizeRuntimeTypeFilter 统一运行时类型别名，供应用筛选和目录匹配复用。
func normalizeRuntimeTypeFilter(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case ".net", "dot-net", "dotnet":
		return "dotnet"
	case "nodejs", "node.js":
		return "node"
	default:
		return value
	}
}

// appMatchesType 判断应用类型、键名或标签是否匹配请求的运行时类型。
func appMatchesType(app appRecord, wanted string) bool {
	wanted = normalizeRuntimeTypeFilter(wanted)
	actual := normalizeRuntimeTypeFilter(app.Type)
	if actual == wanted {
		return true
	}
	if actual != "" && actual != "app" && strings.Contains(actual, wanted) {
		return true
	}
	for _, candidate := range []string{strings.ToLower(strings.TrimSpace(app.Key)), strings.ToLower(strings.TrimSpace(app.Name))} {
		if candidate == wanted || strings.HasPrefix(candidate, wanted+"-") || strings.HasPrefix(candidate, wanted+" ") {
			return true
		}
	}
	/*
		Do not infer a language from arbitrary application names such as
		phpmyadmin; only an explicit runtime type/tag or a language-prefixed key
		is a match.
	*/
	for _, tag := range app.Tags {
		if normalizeRuntimeTypeFilter(tag) == wanted {
			return true
		}
	}
	return false
}

// appMatchesRuntimeCatalog 判断目录应用是否满足运行时筛选的额外约束。
func appMatchesRuntimeCatalog(app appRecord, wanted string) bool {
	if !appMatchesType(app, wanted) {
		return false
	}
	return normalizeRuntimeTypeFilter(wanted) != "php" || strings.EqualFold(strings.TrimSpace(app.Key), "php")
}

// appOK 使用运行时接口共用的成功响应 envelope 返回应用数据。
func appOK(w http.ResponseWriter, d any) { runtimeOK(w, d) }

// appRecordData 将内部应用记录转换为前端兼容的默认语言响应对象。
func appRecordData(a appRecord) map[string]any {
	return appRecordDataLocalized(a, workmeshi18n.NormalizeLocale(""), nil)
}

// localizedAppText 按区域回退顺序选择应用文案，确保中英文请求均有结果。
func localizedAppText(values map[string]string, locale string) string {
	if len(values) == 0 {
		return ""
	}
	for _, key := range []string{locale, strings.ToLower(locale)} {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
	}
	if separator := strings.IndexByte(locale, '-'); separator > 0 {
		if value := strings.TrimSpace(values[locale[:separator]]); value != "" {
			return value
		}
	}
	for _, key := range []string{"zh", "en"} {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
	}
	return ""
}

// localizedDescription 将多语言描述解析为请求区域可直接展示的文本。
func localizedDescription(value any, locale string, shortZh, shortEn string) any {
	if values, ok := value.(map[string]any); ok {
		translations := make(map[string]string, len(values))
		for key, raw := range values {
			if text, ok := raw.(string); ok {
				translations[key] = text
			}
		}
		if text := localizedAppText(translations, locale); text != "" {
			return text
		}
	}
	if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
		return text
	}
	if strings.TrimSpace(shortZh) != "" {
		return shortZh
	}
	return shortEn
}

// localizedAppTags 根据目录标签元数据输出请求区域的标签名称。
func localizedAppTags(tags []string, metadata []appTagRecord, locale string) []string {
	byKey := make(map[string]appTagRecord, len(metadata))
	for _, item := range metadata {
		byKey[strings.ToLower(strings.TrimSpace(item.Key))] = item
	}
	result := make([]string, 0, len(tags))
	for _, key := range tags {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		item := byKey[strings.ToLower(key)]
		name := localizedAppText(item.Locales, locale)
		if name == "" {
			name = strings.TrimSpace(item.Name)
		}
		if name == "" {
			name = key
		}
		result = append(result, name)
	}
	return result
}

// appRecordDataLocalized 将应用记录映射为包含本地化文案和能力字段的响应对象。
func appRecordDataLocalized(a appRecord, locale string, metadata []appTagRecord) map[string]any {
	id := a.ID
	// 默认目录记录没有已安装版本上下文，调用方会在有目录时覆写该字段。
	canUpdate := false
	status := strings.TrimSpace(a.Status)
	if status == "" {
		status = "Normal"
	}
	description := localizedDescription(a.Description, locale, a.ShortDescZh, a.ShortDescEn)
	resource := appValue(a.Config, "resource", "source")
	if resource == "" {
		resource = "remote"
	}
	item := map[string]any{"id": id, "key": a.Key, "name": a.Name, "version": a.Version, "status": status, "message": a.Message, "containerName": appConfiguredContainerName(a), "appKey": a.Key, "appName": a.Name, "appStatus": normalizeAppStatus(status), "ready": 1, "total": 1, "canUpdate": canUpdate, "favorite": false, "sortOrder": a.SortOrder, "updatedAt": a.UpdatedAt, "config": a.Config, "type": a.Type, "resource": resource, "source": resource, "description": description, "shortDescZh": a.ShortDescZh, "shortDescEn": a.ShortDescEn, "tags": localizedAppTags(a.Tags, metadata, locale), "limit": a.Limit, "recommend": a.Recommend, "gpuSupport": a.GpuSupport, "batchInstallSupport": a.BatchInstallSupport, "architectures": a.Architectures, "memoryRequired": a.MemoryRequired, "website": a.Website, "github": a.Github, "readMe": a.ReadMe, "icon": a.IconURL}
	item["installed"] = false
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

// findApp 按 ID、应用键或显示名称查找应用，并返回其索引和副本。
func findApp(items []appRecord, id string) (int, appRecord) {
	for index, item := range items {
		if item.ID == id || item.Key == id || item.Name == id {
			return index, item
		}
	}
	return -1, appRecord{}
}
