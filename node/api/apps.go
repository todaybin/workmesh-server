// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	workmeshi18n "github.com/todaybin/workmesh-server/i18n"
	"github.com/todaybin/workmesh-server/node/model"
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
	// 进程接入公共数据库后，数据库状态优先于旧版 JSON 文件。
	_ = loadJSONState("app_store_state", &state)
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
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	if err := saveJSONState("app_store_state", s.state); err != nil {
		return err
	}
	return persistAppRelational(s.state)
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
	return appRecordDataLocalized(a, workmeshi18n.NormalizeLocale(""), nil)
}

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

func appRecordDataLocalized(a appRecord, locale string, metadata []appTagRecord) map[string]any {
	id := a.ID
	// 默认目录记录没有已安装版本上下文，调用方会在有目录时覆写该字段。
	canUpdate := false
	status := strings.TrimSpace(a.Status)
	if status == "" {
		status = "Normal"
	}
	description := localizedDescription(a.Description, locale, a.ShortDescZh, a.ShortDescEn)
	item := map[string]any{"id": id, "key": a.Key, "name": a.Name, "version": a.Version, "status": status, "message": a.Message, "containerName": appConfiguredContainerName(a), "appKey": a.Key, "appName": a.Name, "appStatus": normalizeAppStatus(status), "ready": 1, "total": 1, "canUpdate": canUpdate, "favorite": false, "sortOrder": a.SortOrder, "updatedAt": a.UpdatedAt, "config": a.Config, "type": a.Type, "description": description, "shortDescZh": a.ShortDescZh, "shortDescEn": a.ShortDescEn, "tags": localizedAppTags(a.Tags, metadata, locale), "limit": a.Limit, "recommend": a.Recommend, "gpuSupport": a.GpuSupport, "batchInstallSupport": a.BatchInstallSupport, "architectures": a.Architectures, "memoryRequired": a.MemoryRequired, "website": a.Website, "github": a.Github, "readMe": a.ReadMe, "icon": a.IconURL}
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
const maxAppCatalogArchiveBytes = 16 << 20
const maxAppCatalogExtractedBytes = 32 << 20

func appStoreRemoteBase() string {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("WORKMESH_APP_REPO_URL")), "/")
	if base == "" {
		if strings.EqualFold(strings.TrimSpace(os.Getenv("WORKMESH_APP_REPO_EDITION")), "intl") {
			base = "https://apps.1panel.pro"
		} else {
			base = "https://apps-assets.fit2cloud.com"
		}
	}
	mode := strings.Trim(strings.TrimSpace(os.Getenv("WORKMESH_APP_REPO_MODE")), "/")
	if mode == "" {
		mode = "stable"
	}
	return base + "/" + mode + "/1panel"
}

func appStableID(value string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(value))
	return strconv.FormatUint(uint64(h.Sum32()), 10)
}

func readRemoteAppList(client *http.Client) (remoteAppList, error) {
	url := appStoreRemoteBase() + ".json.zip"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return remoteAppList{}, fmt.Errorf("创建应用商店请求失败: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return remoteAppList{}, fmt.Errorf("下载应用商店清单失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return remoteAppList{}, fmt.Errorf("应用商店清单返回 HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAppCatalogArchiveBytes+1))
	if err != nil {
		return remoteAppList{}, fmt.Errorf("读取应用商店清单失败: %w", err)
	}
	if len(data) > maxAppCatalogArchiveBytes {
		return remoteAppList{}, fmt.Errorf("应用商店清单超过 %d 字节限制", maxAppCatalogArchiveBytes)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return remoteAppList{}, fmt.Errorf("解析应用商店清单压缩包失败: %w", err)
	}
	var payload []byte
	var extracted int64
	for _, file := range archive.File {
		name := filepath.ToSlash(file.Name)
		clean := filepath.Clean(filepath.FromSlash(name))
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.Base(clean) != "1panel.json" {
			continue
		}
		if file.FileInfo().IsDir() || file.UncompressedSize64 > maxAppCatalogBytes {
			return remoteAppList{}, fmt.Errorf("应用商店清单文件无效或超过大小限制")
		}
		reader, openErr := file.Open()
		if openErr != nil {
			return remoteAppList{}, fmt.Errorf("打开应用商店清单失败: %w", openErr)
		}
		part, readErr := io.ReadAll(io.LimitReader(reader, maxAppCatalogBytes+1))
		_ = reader.Close()
		if readErr != nil {
			return remoteAppList{}, fmt.Errorf("读取应用商店清单文件失败: %w", readErr)
		}
		extracted += int64(len(part))
		if extracted > maxAppCatalogExtractedBytes || len(part) > maxAppCatalogBytes {
			return remoteAppList{}, fmt.Errorf("应用商店压缩包解压内容超过大小限制")
		}
		payload = part
		break
	}
	if len(payload) == 0 {
		return remoteAppList{}, fmt.Errorf("应用商店压缩包缺少 1panel.json")
	}
	var list remoteAppList
	if err := json.Unmarshal(payload, &list); err != nil {
		return remoteAppList{}, fmt.Errorf("解析应用商店清单 JSON 失败: %w", err)
	}
	return list, nil
}

func normalizeRemoteApps(list remoteAppList) []appRecord {
	items := make([]appRecord, 0, len(list.Apps))
	for _, item := range list.Apps {
		key := strings.TrimSpace(item.Property.Key)
		if key == "" {
			continue
		}
		name := item.Name
		if name == "" {
			name = item.Property.Name
		}
		record := appRecord{
			ID: appStableID("app:" + key), Key: key, Name: name, Status: "Normal", Type: item.Property.Type,
			Description: item.Property.Description, ShortDescZh: item.Property.ShortDescZh, ShortDescEn: item.Property.ShortDescEn,
			Tags: item.Property.Tags, Limit: item.Property.Limit, Recommend: item.Property.Recommend, GpuSupport: item.Property.GpuSupport,
			BatchInstallSupport: item.Property.BatchInstallSupport, Architectures: item.Property.Architectures, MemoryRequired: item.Property.MemoryRequired,
			Website: item.Property.Website, Github: item.Property.Github, ReadMe: item.ReadMe,
			IconURL: item.Icon, UpdatedAt: time.Now().UTC(), Config: map[string]any{"resource": "remote", "remoteBase": appStoreRemoteBase()},
		}
		for _, version := range item.Versions {
			v := strings.TrimSpace(version.Name)
			if v == "" {
				continue
			}
			record.Versions = append(record.Versions, appVersionRecord{ID: appStableID("detail:" + key + ":" + v), Version: v, DownloadURL: version.DownloadURL, ComposeURL: strings.TrimRight(appStoreRemoteBase()+"/"+key+"/"+v, "/") + "/docker-compose.yml", Params: version.AppForm, LastModified: version.LastModified})
		}
		if len(record.Versions) > 0 {
			record.Version = record.Versions[0].Version
		}
		items = append(items, record)
	}
	return items
}

func (s *appStore) refreshRemoteLocked(force bool) error {
	if !force && len(s.state.Catalog) > 0 {
		return nil
	}
	list, err := readRemoteAppList(&http.Client{Timeout: 20 * time.Second})
	if err != nil {
		if len(s.state.Catalog) > 0 {
			return nil
		}
		return err
	}
	records := normalizeRemoteApps(list)
	if len(records) == 0 {
		return fmt.Errorf("应用商店清单为空")
	}
	s.state.Catalog = records
	s.state.CatalogVersion = strings.TrimSpace(list.Extra.Version)
	s.state.CatalogLastModified = int64(list.LastModified)
	s.state.CatalogTags = make([]appTagRecord, 0, len(list.Extra.Tags))
	for _, tag := range list.Extra.Tags {
		key := strings.TrimSpace(tag.Key)
		if key == "" {
			continue
		}
		name := strings.TrimSpace(tag.Name)
		if name == "" {
			name = key
		}
		s.state.CatalogTags = append(s.state.CatalogTags, appTagRecord{Key: key, Name: name, Sort: tag.Sort, Locales: tag.Locales})
	}
	s.state.CatalogSyncing = false
	s.state.CatalogSyncedAt = time.Now().UTC()
	return s.saveLocked()
}

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
		body := appBody(r)
		if syncRequested, _ := body["sync"].(bool); syncRequested {
			s.mu.RLock()
			ids := make([]string, 0, len(s.state.Apps))
			for _, item := range s.state.Apps {
				ids = append(ids, item.ID)
			}
			s.mu.RUnlock()
			for _, id := range ids {
				_, _, _ = s.syncAppInstallStatus(r.Context(), id, false)
			}
		}
		s.mu.RLock()
		catalog := append([]appRecord(nil), s.state.Catalog...)
		items := make([]map[string]any, 0, len(s.state.Apps))
		for _, app := range s.state.Apps {
			item := appRecordDataLocalized(app, workmeshi18n.LocaleFromRequest(r), s.state.CatalogTags)
			item["installed"] = appStatusCountsAsInstalled(app.Status)
			_, latest, ok := latestCatalogVersion(app, catalog)
			item["canUpdate"] = ok && compareAppVersion(latest.Version, app.Version) > 0
			item["status"] = normalizeAppStatus(app.Status)
			item["appStatus"] = normalizeAppStatus(app.Status)
			items = append(items, item)
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
		path := strings.TrimPrefix(r.URL.Path, "/api/v2/apps/")
		if strings.TrimSpace(os.Getenv("WORKMESH_APP_CATALOG")) == "" && path != "sync/local" {
			s.mu.Lock()
			force := path == "sync/remote"
			if err := s.refreshRemoteLocked(force); err != nil && len(s.state.Catalog) == 0 {
				s.mu.Unlock()
				runtimeErr(w, http.StatusBadGateway, "应用商店不可用: "+err.Error())
				return
			}
			s.mu.Unlock()
		}
		s.mu.Lock()
		if len(s.state.Catalog) == 0 {
			s.state.Catalog = appCatalogFromEnv()
			if len(s.state.Catalog) == 0 {
				s.state.Catalog = append([]appRecord(nil), s.state.Apps...)
			}
		}
		page := 1
		pageSize := 50
		if value, ok := body["page"].(float64); ok && value >= 1 {
			page = int(value)
		}
		if value, ok := body["pageSize"].(float64); ok && value >= 1 {
			pageSize = int(value)
		}
		if pageSize > 200 {
			pageSize = 200
		}
		filtered := make([]appRecord, 0, len(s.state.Catalog))
		for _, app := range s.state.Catalog {
			if name != "" && !strings.Contains(strings.ToLower(app.Name+" "+app.Key), name) {
				continue
			}
			if tags, ok := body["tags"].([]any); ok && len(tags) > 0 {
				matched := false
				for _, raw := range tags {
					if tag, ok := raw.(string); ok {
						for _, candidate := range app.Tags {
							if strings.EqualFold(tag, candidate) {
								matched = true
							}
						}
					}
				}
				if !matched {
					continue
				}
			}
			filtered = append(filtered, app)
		}
		total := len(filtered)
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		items := make([]map[string]any, 0, end-start)
		for _, app := range filtered[start:end] {
			item := appRecordDataLocalized(app, workmeshi18n.LocaleFromRequest(r), s.state.CatalogTags)
			for _, installed := range s.state.Apps {
				if appIdentityEqual(app, installed) && appStatusCountsAsInstalled(installed.Status) {
					item["installed"] = true
					break
				}
			}
			items = append(items, item)
		}
		_ = s.saveLocked()
		s.mu.Unlock()
		appOK(w, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
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
		if len(s.state.Catalog) == 0 && strings.TrimSpace(os.Getenv("WORKMESH_APP_CATALOG")) == "" {
			if err := s.refreshRemoteLocked(false); err != nil {
				s.mu.Unlock()
				runtimeErr(w, http.StatusBadGateway, "应用商店不可用: "+err.Error())
				return
			}
		}
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
	mux.HandleFunc("GET /api/v2/apps/tags", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		if len(s.state.Catalog) == 0 && strings.TrimSpace(os.Getenv("WORKMESH_APP_CATALOG")) == "" {
			_ = s.refreshRemoteLocked(false)
		}
		s.mu.Unlock()
		s.mu.RLock()
		seen := map[string]bool{}
		names := map[string]appTagRecord{}
		for _, tag := range s.state.CatalogTags {
			names[strings.ToLower(tag.Key)] = tag
		}
		for _, item := range s.state.Catalog {
			for _, tag := range item.Tags {
				if strings.TrimSpace(tag) != "" {
					seen[tag] = true
				}
			}
		}
		tags := make([]map[string]any, 0, len(seen))
		for tag := range seen {
			metadata := names[strings.ToLower(tag)]
			name := localizedAppText(metadata.Locales, workmeshi18n.LocaleFromRequest(r))
			if name == "" {
				name = strings.TrimSpace(metadata.Name)
			}
			if name == "" {
				name = tag
			}
			tags = append(tags, map[string]any{"key": tag, "name": name, "sort": metadata.Sort})
		}
		sort.Slice(tags, func(i, j int) bool {
			leftSort, _ := tags[i]["sort"].(int)
			rightSort, _ := tags[j]["sort"].(int)
			if leftSort != rightSort {
				return leftSort < rightSort
			}
			return strings.ToLower(tags[i]["key"].(string)) < strings.ToLower(tags[j]["key"].(string))
		})
		s.mu.RUnlock()
		appOK(w, tags)
	})
	mux.HandleFunc("GET /api/v2/apps/ignored/detail", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		items := append([]map[string]any(nil), s.state.Ignored...)
		s.mu.RUnlock()
		appOK(w, items)
	})
	mux.HandleFunc("GET /api/v2/apps/{key}", func(w http.ResponseWriter, r *http.Request) { appCatalogGet(w, s, r, r.PathValue("key"), "") })
	mux.HandleFunc("GET /api/v2/apps/detail/{appId}/{version}/{type}", func(w http.ResponseWriter, r *http.Request) {
		appCatalogGet(w, s, r, r.PathValue("appId"), r.PathValue("version"))
	})
	mux.HandleFunc("GET /api/v2/apps/detail/node/{appKey}/{version}", func(w http.ResponseWriter, r *http.Request) {
		appCatalogGet(w, s, r, r.PathValue("appKey"), r.PathValue("version"))
	})
	mux.HandleFunc("GET /api/v2/apps/details/{id}", func(w http.ResponseWriter, r *http.Request) { appCatalogGet(w, s, r, r.PathValue("id"), "") })
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
	mux.HandleFunc("GET /api/v2/apps/icon/{key}", func(w http.ResponseWriter, r *http.Request) { appIcon(w, s, r.PathValue("key")) })
	mux.HandleFunc("GET /api/v2/apps/installed/info/{appInstallId}", func(w http.ResponseWriter, r *http.Request) { appInstalledGet(w, s, r, r.PathValue("appInstallId")) })
	mux.HandleFunc("GET /api/v2/apps/installed/params/{appInstallId}", func(w http.ResponseWriter, r *http.Request) { appInstalledGet(w, s, r, r.PathValue("appInstallId")) })
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
			handleAppPost(w, s, r, strings.TrimPrefix(r.URL.Path, "/api/v2/apps/"), appBody(r))
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
	mux.HandleFunc("POST /api/v2/core/xpack/sync/app/install", func(w http.ResponseWriter, r *http.Request) { handleAppPost(w, s, r, "install", appBody(r)) })
}

func appCatalogGet(w http.ResponseWriter, s *appStore, r *http.Request, id, version string) {
	s.mu.Lock()
	if len(s.state.Catalog) == 0 && strings.TrimSpace(os.Getenv("WORKMESH_APP_CATALOG")) == "" {
		_ = s.refreshRemoteLocked(false)
	}
	s.mu.Unlock()
	s.mu.RLock()
	index, item := findApp(s.state.Catalog, id)
	if index < 0 {
		index, item = findApp(s.state.Apps, id)
	}
	metadata := append([]appTagRecord(nil), s.state.CatalogTags...)
	s.mu.RUnlock()
	if index < 0 {
		appOK(w, map[string]any{"id": id, "key": id, "available": false})
		return
	}
	data := appRecordDataLocalized(item, workmeshi18n.LocaleFromRequest(r), metadata)
	data["available"] = true
	versions := make([]string, 0, len(item.Versions))
	for _, version := range item.Versions {
		if strings.TrimSpace(version.Version) != "" {
			versions = append(versions, version.Version)
		}
	}
	data["versions"] = versions
	data["readMe"] = item.ReadMe
	params := any(map[string]any{})
	selected := appVersionRecord{}
	for _, candidate := range item.Versions {
		if version == "" || candidate.Version == version {
			selected = candidate
			break
		}
	}
	if selected.Version == "" && len(item.Versions) > 0 {
		selected = item.Versions[0]
	}
	if version != "" && selected.ID != "" {
		data["appId"] = data["id"]
		data["id"] = selected.ID
		if numericID, err := strconv.ParseInt(selected.ID, 10, 64); err == nil {
			data["id"] = numericID
		}
	}
	if selected.Version != "" {
		params = selected.Params
	} else if item.Config != nil {
		if configured, ok := item.Config["params"]; ok {
			params = configured
		}
	}
	compose := selected.DockerCompose
	if compose == "" && item.Config != nil {
		compose = appValue(item.Config, "dockerCompose", "compose")
	}
	if compose == "" && selected.ComposeURL != "" {
		compose = fetchRemoteCompose(selected.ComposeURL)
	}
	// 原版安装表单直接读取详情顶层字段；details 保留为兼容扩展字段。
	data["params"] = params
	data["dockerCompose"] = compose
	data["memoryRequired"] = item.MemoryRequired
	data["gpuSupport"] = item.GpuSupport
	data["architectures"] = item.Architectures
	data["hostMode"] = false
	data["details"] = map[string]any{"id": selected.ID, "version": selected.Version, "type": item.Type, "params": params, "dockerCompose": compose, "downloadUrl": selected.DownloadURL}
	appOK(w, data)
}

func fetchRemoteCompose(url string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil || len(body) == 0 {
		return ""
	}
	return string(body)
}

func appInstalledGet(w http.ResponseWriter, s *appStore, r *http.Request, id string) {
	s.mu.RLock()
	_, item := findApp(s.state.Apps, id)
	s.mu.RUnlock()
	if item.ID == "" {
		appOK(w, map[string]any{"id": id, "status": "not_installed", "env": map[string]any{}})
		return
	}
	data := appRecordDataLocalized(item, workmeshi18n.LocaleFromRequest(r), s.state.CatalogTags)
	data["env"] = item.Config
	data["container"] = appConfiguredContainerName(item)
	data["httpPort"] = item.Config["port"]
	appOK(w, data)
}

func handleAppPost(w http.ResponseWriter, s *appStore, r *http.Request, path string, body map[string]any) {
	switch path {
	case "install":
		id := appValue(body, "appInstallId", "id", "appId", "key", "name")
		if id == "" {
			runtimeErr(w, http.StatusBadRequest, "应用标识不能为空")
			return
		}
		taskID := appValue(body, "taskID", "taskId")
		if taskID == "" {
			taskID = idToken()
		}
		item := appRecord{ID: id, Key: appValue(body, "key", "appKey", "id"), Name: appValue(body, "name", "appName", "key"), Version: appValue(body, "version"), Status: "installing", ContainerName: appValue(body, "containerName", "CONTAINER_NAME"), Config: body, UpdatedAt: time.Now().UTC()}
		if detailID := appValue(body, "appDetailId", "appDetailID"); detailID != "" {
			s.mu.RLock()
			if catalogApp, catalogVersion, ok := findCatalogDetail(s.state.Catalog, detailID); ok {
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
			s.mu.RUnlock()
		}
		item.Config["taskID"] = taskID
		if item.Key == "" {
			item.Key = id
		}
		if item.Name == "" {
			item.Name = item.Key
		}
		if item.ContainerName == "" {
			item.ContainerName = "WorkMesh-" + item.Key + "-" + strings.ToLower(idToken()[:6])
		}
		item.Config["containerName"] = item.ContainerName
		// 没有应用包或 Compose 时兼容旧版“仅登记应用”场景；真实安装请求必须进入异步任务。
		downloadURL := appValue(body, "downloadUrl", "downloadURL")
		compose := appValue(body, "dockerCompose", "compose")
		if downloadURL == "" {
			s.mu.RLock()
			if catalogIndex, catalogItem := findApp(s.state.Catalog, item.Key); catalogIndex >= 0 {
				for _, version := range catalogItem.Versions {
					if item.Version == "" || version.Version == item.Version {
						downloadURL, compose = version.DownloadURL, version.DockerCompose
						break
					}
				}
			}
			s.mu.RUnlock()
		}
		if compose == "" {
			compose = appValue(body, "docker-compose", "composeContent")
		}
		if compose != "" {
			item.Config["dockerCompose"] = compose
		}
		downloadURL = selectAppDownloadURL(downloadURL, body)
		s.mu.Lock()
		index, _ := findApp(s.state.Apps, id)
		if index >= 0 {
			s.state.Apps[index] = item
		} else {
			s.state.Apps = append(s.state.Apps, item)
		}
		_ = s.saveLocked()
		s.mu.Unlock()
		persistAppInstallRecord(item, "")
		ensureAppTaskLog(taskID, item.ID, item.Name, "installing", "开始安装应用")
		if downloadURL != "" || compose != "" {
			go runAppInstallTask(s, item, downloadURL, compose)
		} else {
			item.Status = "running"
			s.mu.Lock()
			if index, _ := findApp(s.state.Apps, id); index >= 0 {
				s.state.Apps[index] = item
				_ = s.saveLocked()
			}
			s.mu.Unlock()
			persistAppInstallRecord(item, "")
		}
		result := appRecordDataLocalized(item, workmeshi18n.LocaleFromRequest(r), s.state.CatalogTags)
		result["status"] = item.Status
		result["taskStatus"] = item.Status
		result["taskID"] = taskID
		appOK(w, result)
	case "installed/check":
		id := appValue(body, "name", "key", "appInstallId")
		s.mu.RLock()
		_, item := findApp(s.state.Apps, id)
		s.mu.RUnlock()
		if item.ID != "" && (strings.EqualFold(item.Key, "openresty") || appConfiguredContainerName(item) != "") {
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
			httpPort := appConfiguredInt(item.Config, 80, "httpPort", "PANEL_APP_PORT_HTTP", "port")
			httpsPort := appConfiguredInt(item.Config, 443, "httpsPort", "PANEL_APP_PORT_HTTPS")
			websiteDir := appValue(item.Config, "websiteDir", "WEBSITE_DIR")
			if websiteDir == "" {
				websiteDir = "/www/wwwroot"
			}
			data := map[string]any{"name": item.Name, "version": item.Version, "isExist": true, "isActive": status == "Running", "status": status, "app": item.Key, "appInstallId": item.ID, "containerName": appConfiguredContainerName(item), "httpPort": httpPort, "httpsPort": httpsPort, "websiteDir": websiteDir}
			if item.Message != "" {
				data["error"] = item.Message
			}
			appOK(w, data)
			return
		}
		probe := service.ProbeApplication(context.Background(), appValue(body, "key", "app", "type"), id)
		if probe.App == "" {
			probe.App = id
		}
		if item.ID != "" && !probe.IsExist && probe.Error == "未配置该应用的本机探测器" {
			probe.IsExist, probe.IsActive, probe.Status = true, normalizeAppStatus(item.Status) == "Running", normalizeAppStatus(item.Status)
			probe.Version = item.Version
		}
		appOK(w, map[string]any{"name": id, "version": probe.Version, "isExist": probe.IsExist, "isActive": probe.IsActive, "status": probe.Status, "app": probe.App, "appInstallId": item.ID, "containerName": appConfiguredContainerName(item), "httpPort": 80, "httpsPort": 443, "websiteDir": "/www/wwwroot"})
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

func appStatusCountsAsInstalled(status string) bool {
	switch normalizeAppStatus(status) {
	case "Running", "Stopped", "Paused", "ReStarting", "UnHealthy":
		return true
	default:
		return false
	}
}

func findCatalogDetail(catalog []appRecord, detailID string) (appRecord, appVersionRecord, bool) {
	for _, app := range catalog {
		for _, version := range app.Versions {
			if strings.EqualFold(strings.TrimSpace(version.ID), strings.TrimSpace(detailID)) {
				return app, version, true
			}
		}
	}
	return appRecord{}, appVersionRecord{}, false
}

func selectAppDownloadURL(original string, body map[string]any) string {
	original = strings.TrimSpace(original)
	if original == "" {
		return original
	}
	mirror := appValue(body, "mirror", "mirrorUrl", "accelerateUrl", "downloadMirror")
	if mirror == "" {
		mirror = strings.TrimSpace(os.Getenv("WORKMESH_APP_MIRROR"))
	}
	if mirror == "" {
		return original
	}
	mirror = strings.TrimRight(mirror, "/")
	if strings.Contains(mirror, "{url}") {
		return strings.ReplaceAll(mirror, "{url}", original)
	}
	name := filepath.Base(strings.TrimSpace(strings.SplitN(original, "?", 2)[0]))
	if name == "." || name == "" {
		return original
	}
	return mirror + "/" + name
}

func persistAppInstallRecord(item appRecord, composePath string) {
	db := sharedDB()
	if db == nil {
		return
	}
	config, _ := json.Marshal(item.Config)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, _ = db.Exec(`INSERT INTO app_installs(id,app_key,name,version,status,install_path,compose_path,compose_project,container_names,config_json,message,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET app_key=excluded.app_key,name=excluded.name,version=excluded.version,status=excluded.status,install_path=excluded.install_path,compose_path=excluded.compose_path,container_names=excluded.container_names,config_json=excluded.config_json,message=excluded.message,updated_at=excluded.updated_at`,
		item.ID, item.Key, item.Name, item.Version, item.Status, appInstallPath(item), composePath, appValue(item.Config, "composeProject", "projectName"), item.ContainerName, config, item.Message, now, now)
}

func appInstallPath(item appRecord) string {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	name := item.Name
	if !validDockerIdentifier(name) {
		name = item.ID
	}
	return filepath.Join(root, "apps", item.Key, name)
}

func appTaskLogPath(taskID string) string {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	return filepath.Join(root, "logs", "tasks", "app", taskID+".log")
}

func ensureAppTaskLog(taskID, installID, name, status, message string) {
	path := appTaskLogPath(taskID)
	_ = os.MkdirAll(filepath.Dir(path), 0o750)
	store := getDomainStore()
	store.mu.Lock()
	found := false
	for i := range store.state.Logs {
		if store.state.Logs[i].ID == taskID {
			store.state.Logs[i].Level = appTaskLevel(status)
			store.state.Logs[i].Message = message
			found = true
			break
		}
	}
	if !found {
		store.state.Logs = append(store.state.Logs, logItem{ID: taskID, Type: "task", Level: appTaskLevel(status), Message: name, Meta: map[string]any{"path": path, "scope": "app", "name": name}, CreatedAt: time.Now().UTC()})
	}
	_ = store.saveLocked()
	store.mu.Unlock()
	appendAppTaskLog(taskID, message)
	db := sharedDB()
	if db != nil {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, _ = db.Exec(`INSERT INTO app_install_tasks(id,app_install_id,status,step,progress,message,error,log_path,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET app_install_id=excluded.app_install_id,status=excluded.status,step=excluded.step,progress=excluded.progress,message=excluded.message,error=excluded.error,log_path=excluded.log_path,updated_at=excluded.updated_at`, taskID, installID, status, status, appTaskProgress(status), message, appTaskError(status, message), path, now, now)
	}
}

func appendAppTaskLog(taskID, message string) {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(message) == "" {
		return
	}
	path := appTaskLogPath(taskID)
	_ = os.MkdirAll(filepath.Dir(path), 0o750)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = fmt.Fprintf(file, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), strings.TrimSpace(message))
}

func appTaskLevel(status string) string {
	switch strings.ToLower(status) {
	case "running":
		return "Success"
	case "failed", "error":
		return "Failed"
	default:
		return "Executing"
	}
}

func appTaskProgress(status string) int {
	switch strings.ToLower(status) {
	case "downloading":
		return 20
	case "installing":
		return 40
	case "pulling":
		return 65
	case "starting":
		return 85
	case "running", "failed":
		return 100
	default:
		return 0
	}
}

func appTaskError(status, message string) string {
	if strings.EqualFold(status, "failed") {
		return message
	}
	return ""
}

func runAppInstallTask(store *appStore, item appRecord, downloadURL, compose string) {
	taskID := appValue(item.Config, "taskID", "taskId")
	update := func(status, message string) {
		store.mu.Lock()
		if index, _ := findApp(store.state.Apps, item.ID); index >= 0 {
			current := store.state.Apps[index]
			current.Status, current.Message, current.UpdatedAt = status, message, time.Now().UTC()
			if compose != "" {
				current.Config["dockerCompose"] = compose
			}
			store.state.Apps[index] = current
			item = current
			_ = store.saveLocked()
		}
		store.mu.Unlock()
		persistAppInstallRecord(item, appComposePath(item))
		ensureAppTaskLog(taskID, item.ID, item.Name, status, message)
	}
	update("installing", "准备安装")
	installDir := appInstallPath(item)
	if err := os.MkdirAll(installDir, 0o750); err != nil {
		update("failed", err.Error())
		return
	}
	if downloadURL != "" {
		update("downloading", "正在下载应用包")
		archivePath := filepath.Join(installDir, "package.tar.gz")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		err := downloadAppArchive(ctx, downloadURL, archivePath)
		cancel()
		if err != nil {
			update("failed", "下载应用包失败: "+err.Error())
			return
		}
		update("installing", "正在解压应用包")
		if err := extractTarGz(archivePath, installDir); err != nil {
			update("failed", "解压应用包失败: "+err.Error())
			return
		}
		if compose == "" {
			compose = findComposeContent(installDir)
		}
	}
	if compose == "" {
		update("failed", "应用包未提供 docker-compose.yml")
		return
	}
	composePath := appComposePath(item)
	if err := writeAtomicFile(composePath, []byte(compose)); err != nil {
		update("failed", "写入 Compose 文件失败: "+err.Error())
		return
	}
	params := map[string]any{}
	if raw, ok := item.Config["params"].(map[string]any); ok {
		params = raw
	}
	params["CONTAINER_NAME"] = item.ContainerName
	if params["PANEL_APP_PORT_HTTP"] == nil {
		if port := appConfiguredInt(item.Config, 0, "PANEL_APP_PORT_HTTP", "httpPort", "port"); port > 0 {
			params["PANEL_APP_PORT_HTTP"] = port
		}
	}
	if err := writeComposeEnv(filepath.Join(filepath.Dir(composePath), ".env"), params); err != nil {
		update("failed", "写入 Compose 环境文件失败: "+err.Error())
		return
	}
	if strings.Contains(compose, "external: true") && strings.Contains(compose, "1panel-network") {
		if err := ensureDockerNetwork("1panel-network"); err != nil {
			update("failed", "创建应用默认网络失败: "+err.Error())
			return
		}
	}
	composeStore := getContainerStore()
	composeStore.mu.Lock()
	foundCompose := false
	for i := range composeStore.state.Composes {
		if composeStore.state.Composes[i].Path == composePath {
			composeStore.state.Composes[i].Name = filepath.Base(filepath.Dir(composePath))
			composeStore.state.Composes[i].AppInstallID = item.ID
			composeStore.state.Composes[i].UpdatedAt = time.Now().UTC()
			foundCompose = true
			break
		}
	}
	if !foundCompose {
		now := time.Now().UTC()
		composeStore.state.Composes = append(composeStore.state.Composes, composeRecord{ID: idToken(), Name: filepath.Base(filepath.Dir(composePath)), Path: composePath, AppInstallID: item.ID, CreatedAt: now, UpdatedAt: now})
	}
	_ = composeStore.saveLocked()
	composeStore.mu.Unlock()
	update("pulling", "正在拉取应用镜像")
	cmd := service.CommandService{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	pullImage := true
	if value, ok := item.Config["pullImage"].(bool); ok {
		pullImage = value
	}
	if result, err := cmd.Execute(ctx, model.CommandRequest{Program: "docker", Args: composePullArgs(composePath, pullImage), Dir: filepath.Dir(composePath), Timeout: 30 * time.Minute}); err != nil || result.ExitCode != 0 {
		cancel()
		message := result.Stderr
		if message == "" && err != nil {
			message = err.Error()
		}
		update("failed", "拉取镜像失败: "+strings.TrimSpace(message))
		return
	}
	appendAppTaskLog(taskID, "应用镜像准备完成")
	update("starting", "正在启动应用容器")
	result, err := cmd.Execute(ctx, model.CommandRequest{Program: "docker", Args: []string{"compose", "-f", composePath, "up", "-d"}, Dir: filepath.Dir(composePath), Timeout: 30 * time.Minute})
	cancel()
	if err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" && err != nil {
			message = err.Error()
		}
		update("failed", "启动容器失败: "+message)
		return
	}
	if containerNames := composeServiceContainers(composePath); containerNames != "" {
		item.ContainerName = containerNames
	}
	item.Config["composePath"] = composePath
	item.Config["composeProject"] = filepath.Base(filepath.Dir(composePath))
	update("running", "安装完成")
	appendAppTaskLog(taskID, "[TASK-END]")
}

func composePullArgs(composePath string, pull bool) []string {
	if !pull {
		return []string{"compose", "-f", composePath, "config", "--quiet"}
	}
	return []string{"compose", "-f", composePath, "pull"}
}

func ensureDockerNetwork(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	commands := service.CommandService{}
	if result, err := commands.Execute(ctx, model.CommandRequest{Program: "docker", Args: []string{"network", "inspect", name}, Timeout: 30 * time.Second}); err == nil && result.ExitCode == 0 {
		return nil
	}
	result, err := commands.Execute(ctx, model.CommandRequest{Program: "docker", Args: []string{"network", "create", name}, Timeout: 30 * time.Second})
	if err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" && err != nil {
			message = err.Error()
		}
		return errors.New(message)
	}
	return nil
}

func downloadAppArchive(ctx context.Context, source, target string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	tmp := target + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, io.LimitReader(resp.Body, 2<<30))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, target)
}

func extractTarGz(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(header.Name)
		if name == "." || name == ".." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("压缩包包含非法路径: %s", header.Name)
		}
		target := filepath.Join(destination, name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, io.LimitReader(reader, 512<<20))
			_ = out.Close()
			if copyErr != nil {
				return copyErr
			}
		}
	}
}

func findComposeContent(root string) string {
	var result string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || result != "" || info == nil || info.IsDir() {
			return nil
		}
		base := strings.ToLower(info.Name())
		if base == "docker-compose.yml" || base == "docker-compose.yaml" || base == "compose.yml" || base == "compose.yaml" {
			if data, readErr := os.ReadFile(path); readErr == nil {
				result = string(data)
			}
		}
		return nil
	})
	return result
}

func appComposePath(item appRecord) string {
	return filepath.Join(appInstallPath(item), "docker-compose.yml")
}

func composeServiceContainers(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := (service.CommandService{}).Execute(ctx, model.CommandRequest{Program: "docker", Args: []string{"compose", "-f", path, "ps", "--format", "{{.Name}}"}, Dir: filepath.Dir(path), Timeout: 30 * time.Second})
	if err != nil || result.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

func writeComposeEnv(path string, params map[string]any) error {
	lines := make([]string, 0, len(params))
	for key, value := range params {
		if !validEnvKey(key) {
			continue
		}
		text := strings.ReplaceAll(fmt.Sprint(value), "\n", "")
		lines = append(lines, key+"="+text)
	}
	sort.Strings(lines)
	return writeAtomicFile(path, []byte(strings.Join(lines, "\n")+"\n"))
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
	composePath := appValue(item.Config, "composePath")
	if composePath == "" {
		composePath = appComposePath(item)
	}
	if composePath != "" {
		if _, statErr := os.Stat(composePath); statErr != nil {
			composePath = ""
		}
	}
	if composePath != "" {
		op := operation
		if op == "uninstall" || op == "delete" || op == "鍗歌浇" {
			op = "down"
		}
		if op == "start" || op == "stop" || op == "restart" || op == "down" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			result, execErr := (service.CommandService{}).Execute(ctx, model.CommandRequest{Program: "docker", Args: []string{"compose", "-f", composePath, op}, Dir: filepath.Dir(composePath), Timeout: 5 * time.Minute})
			cancel()
			if execErr != nil || result.ExitCode != 0 {
				s.mu.Unlock()
				message := strings.TrimSpace(result.Stderr)
				if message == "" && execErr != nil {
					message = execErr.Error()
				}
				runtimeErr(w, http.StatusBadGateway, "Docker Compose 操作失败: "+message)
				return
			}
		}
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

func normalizeAppStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running":
		return "Running"
	case "stopped", "exited":
		return "Stopped"
	case "restarting":
		return "ReStarting"
	case "paused":
		return "Paused"
	case "error":
		return "Error"
	case "unhealthy":
		return "UnHealthy"
	default:
		return strings.TrimSpace(status)
	}
}

func appConfiguredContainerName(item appRecord) string {
	if value := strings.TrimSpace(item.ContainerName); value != "" {
		return value
	}
	if value := appValue(item.Config, "containerName", "CONTAINER_NAME", "container"); value != "" {
		return value
	}
	if params, ok := item.Config["params"].(map[string]any); ok {
		return appValue(params, "containerName", "CONTAINER_NAME")
	}
	return ""
}

func appContainerNames(item appRecord) []string {
	seen := map[string]bool{}
	names := make([]string, 0)
	for _, name := range strings.Split(appConfiguredContainerName(item), ",") {
		name = strings.TrimSpace(name)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

func appConfiguredInt(config map[string]any, fallback int, keys ...string) int {
	for _, key := range keys {
		switch value := config[key].(type) {
		case float64:
			if value > 0 {
				return int(value)
			}
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	if params, ok := config["params"].(map[string]any); ok {
		return appConfiguredInt(params, fallback, keys...)
	}
	return fallback
}

func appStatusIsTransient(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "installing", "rebuilding", "upgrading", "uninstalling", "starting", "restarting", "waiting", "syncing":
		return true
	default:
		return false
	}
}

func (s *appStore) syncAppInstallStatus(ctx context.Context, id string, force bool) (appRecord, bool, error) {
	s.mu.RLock()
	_, item := findApp(s.state.Apps, id)
	loader := s.containerStates
	s.mu.RUnlock()
	if item.ID == "" {
		return appRecord{}, false, nil
	}
	if appStatusIsTransient(item.Status) && !force {
		return item, true, nil
	}
	names := appContainerNames(item)
	states := map[string]string{}
	var err error
	if len(names) > 0 {
		states, err = loader(ctx, names)
		if err != nil {
			return item, true, err
		}
	}
	applyAppContainerStates(&item, names, states, force)
	item.ContainerName = strings.Join(names, ",")
	item.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	index, _ := findApp(s.state.Apps, item.ID)
	if index >= 0 {
		s.state.Apps[index] = item
		err = s.saveLocked()
	}
	s.mu.Unlock()
	if err != nil {
		return item, true, fmt.Errorf("保存应用状态失败: %w", err)
	}
	return item, true, nil
}

func applyAppContainerStates(item *appRecord, names []string, states map[string]string, force bool) {
	if len(names) == 0 || len(states) == 0 {
		if normalizeAppStatus(item.Status) == "UpErr" && !force {
			return
		}
		item.Status = "Error"
		item.Message = "未找到应用容器: " + strings.Join(names, ",")
		return
	}
	counts := map[string]int{}
	missing := make([]string, 0)
	exited := make([]string, 0)
	for _, name := range names {
		state, ok := states[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		state = strings.ToLower(strings.TrimSpace(state))
		counts[state]++
		if state == "exited" {
			exited = append(exited, name)
		}
	}
	total := len(names)
	item.Message = ""
	switch {
	case counts["exited"] == total:
		item.Status = "Stopped"
	case counts["running"] == total:
		item.Status = "Running"
	case counts["restarting"] == total:
		item.Status = "ReStarting"
	case counts["paused"] == total:
		item.Status = "Paused"
	case len(missing) == total:
		item.Status = "Error"
		item.Message = "未找到应用容器: " + strings.Join(missing, ",")
	default:
		item.Status = "UnHealthy"
		parts := make([]string, 0, 2)
		if len(exited) > 0 {
			parts = append(parts, "容器已停止: "+strings.Join(exited, ","))
		}
		if len(missing) > 0 {
			parts = append(parts, "未找到应用容器: "+strings.Join(missing, ","))
		}
		if len(parts) == 0 {
			parts = append(parts, "应用容器状态不一致")
		}
		item.Message = strings.Join(parts, "; ")
	}
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

func appIcon(w http.ResponseWriter, s *appStore, key string) {
	var source, cacheKey string
	s.mu.RLock()
	_, item := findApp(s.state.Catalog, key)
	if item.ID == "" {
		_, item = findApp(s.state.Apps, key)
	}
	s.mu.RUnlock()
	if item.ID != "" && item.IconURL != "" {
		source = item.IconURL
		cacheKey = item.Key
	}
	if source != "" {
		cachePath := filepath.Join(filepath.Dir(s.path), "app-icons", appStableID(cacheKey)+".png")
		if cached, err := os.ReadFile(cachePath); err == nil && len(cached) > 0 {
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "public, max-age=2592000")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(cached)
			return
		}
		if !strings.HasPrefix(source, "http://") && !strings.HasPrefix(source, "https://") {
			source = strings.TrimRight(appStoreRemoteBase()+"/"+item.Key, "/") + "/" + strings.TrimLeft(source, "/")
		}
		if data := fetchRemoteAsset(source); len(data) > 0 {
			if cacheKey != "" {
				cacheRemoteIcon(s, cacheKey, data)
			}
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "public, max-age=2592000")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
	}
	// 透明 1x1 PNG：目录没有图标时也返回真实图片响应，避免浏览器将 JSON 当脚本解析。
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func fetchRemoteAsset(url string) []byte {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || len(data) == 0 {
		return nil
	}
	return data
}

func cacheRemoteIcon(s *appStore, key string, data []byte) {
	if len(data) == 0 {
		return
	}
	path := filepath.Join(filepath.Dir(s.path), "app-icons", appStableID(key)+".png")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err == nil {
		_ = os.Rename(tmp, path)
	}
}

func isAppRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	p := pattern
	if len(parts) == 2 {
		p = parts[1]
	}
	return p == "/api/v2/apps" || strings.HasPrefix(p, "/api/v2/apps/")
}
