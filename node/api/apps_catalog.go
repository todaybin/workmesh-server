// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ensureCatalogLocked 按需加载目录，并在空闲 TTL 后释放所有大对象引用。
// 调用方必须持有 s.mu 写锁。
func (s *appStore) ensureCatalogLocked() error {
	if !s.catalogLoaded {
		if repository, err := SharedRepository(); err == nil {
			var payload []byte
			if err := repository.QueryRow(`SELECT payload FROM app_catalog_cache WHERE id=1`).Scan(&payload); err == nil {
				var cache appCatalogCache
				if err := json.Unmarshal(payload, &cache); err != nil {
					return fmt.Errorf("解析应用目录缓存失败: %w", err)
				}
				s.state.Catalog, s.state.CatalogVersion = cache.Catalog, cache.Version
				s.state.CatalogLastModified, s.state.CatalogSyncing = cache.LastModified, cache.Syncing
				s.state.CatalogSyncedAt, s.state.CatalogTags = cache.SyncedAt, cache.Tags
			}
		} else if payload, err := os.ReadFile(filepath.Join(filepath.Dir(s.path), "app-catalog.json")); err == nil {
			var cache appCatalogCache
			if err := json.Unmarshal(payload, &cache); err != nil {
				return fmt.Errorf("解析应用目录缓存失败: %w", err)
			}
			s.state.Catalog, s.state.CatalogVersion = cache.Catalog, cache.Version
			s.state.CatalogLastModified, s.state.CatalogSyncing = cache.LastModified, cache.Syncing
			s.state.CatalogSyncedAt, s.state.CatalogTags = cache.SyncedAt, cache.Tags
		}
		s.catalogLoaded = true
	}
	s.touchCatalogExpiryLocked()
	return nil
}

func (s *appStore) touchCatalogExpiryLocked() {
	s.catalogEpoch++
	epoch := s.catalogEpoch
	if s.catalogExpiry != nil {
		s.catalogExpiry.Stop()
	}
	s.catalogExpiry = time.AfterFunc(runtimeCatalogTTL(), func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.catalogEpoch != epoch {
			return
		}
		s.state.Catalog = nil
		s.state.CatalogTags = nil
		s.state.CatalogVersion = ""
		s.state.CatalogLastModified = 0
		s.state.CatalogSyncing = false
		s.state.CatalogSyncedAt = time.Time{}
		s.catalogLoaded = false
		s.catalogPath, s.catalogSize, s.catalogModTime = "", 0, time.Time{}
	})
}

// saveCatalogLocked 只持久化目录缓存，不触碰已安装应用状态。
func (s *appStore) saveCatalogLocked() error {
	s.catalogLoaded = true
	s.touchCatalogExpiryLocked()
	cache := appCatalogCache{Catalog: s.state.Catalog, Version: s.state.CatalogVersion, LastModified: s.state.CatalogLastModified, Syncing: s.state.CatalogSyncing, SyncedAt: s.state.CatalogSyncedAt, Tags: s.state.CatalogTags}
	if sharedDB() != nil {
		return saveJSONState("app_catalog_cache", cache)
	}
	payload, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(s.path), "app-catalog.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(path+".tmp", payload, 0o600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

// appCatalogFromEnv 从受控环境变量加载本地应用目录。
func appCatalogFromEnv() []appRecord {
	if file := strings.TrimSpace(os.Getenv("WORKMESH_APP_CATALOG")); file != "" {
		result, _, _, err := loadAppCatalogFile(file)
		if err == nil {
			return result
		}
	}
	return nil
}

type appVersionParts struct {
	core []int
	pre  []string
}

// parseAppVersion 解析语义化版本的主版本和预发布标识。
func parseAppVersion(value string) (appVersionParts, bool) {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(value, "v"), "V"))
	if value == "" {
		return appVersionParts{}, false
	}
	parts := strings.SplitN(strings.SplitN(value, "+", 2)[0], "-", 2)
	parsed := appVersionParts{}
	for _, part := range strings.Split(parts[0], ".") {
		if part == "" {
			return appVersionParts{}, false
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return appVersionParts{}, false
		}
		parsed.core = append(parsed.core, n)
	}
	if len(parts) == 2 {
		for _, id := range strings.Split(parts[1], ".") {
			if id == "" {
				return appVersionParts{}, false
			}
			parsed.pre = append(parsed.pre, id)
		}
	}
	return parsed, true
}

// compareAppVersion 比较常见的语义化版本，返回值大于零表示 a 更新。
// 预发布版本低于同一主版本的正式版本；无法解析时使用不区分大小写的字典序。
func compareAppVersion(a, b string) int {
	avParts, av := parseAppVersion(a)
	bvParts, bv := parseAppVersion(b)
	if !av || !bv {
		return strings.Compare(strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b)))
	}
	for i := 0; i < len(avParts.core) || i < len(bvParts.core); i++ {
		an, bn := 0, 0
		if i < len(avParts.core) {
			an = avParts.core[i]
		}
		if i < len(bvParts.core) {
			bn = bvParts.core[i]
		}
		if an != bn {
			if an > bn {
				return 1
			}
			return -1
		}
	}
	if len(avParts.pre) == 0 && len(bvParts.pre) == 0 {
		return 0
	}
	if len(avParts.pre) == 0 {
		return 1
	}
	if len(bvParts.pre) == 0 {
		return -1
	}
	for i := 0; i < len(avParts.pre) && i < len(bvParts.pre); i++ {
		ai, aerr := strconv.Atoi(avParts.pre[i])
		bi, berr := strconv.Atoi(bvParts.pre[i])
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
		if cmp := strings.Compare(avParts.pre[i], bvParts.pre[i]); cmp != 0 {
			return cmp
		}
	}
	if len(avParts.pre) > len(bvParts.pre) {
		return 1
	}
	if len(avParts.pre) < len(bvParts.pre) {
		return -1
	}
	return 0
}

// versionGreater 保留内部调用兼容性，使用语义化版本比较结果。
func versionGreater(latest, current string) bool { return compareAppVersion(latest, current) > 0 }

// appIdentityEqual 判断两个应用记录是否具有相同的稳定标识。
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

// latestCatalogVersion 在远程目录中查找指定应用的最高版本。
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

// appStoreRemoteBase 返回应用商店远程目录的受控地址。
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

// appStableID 为应用目录记录生成稳定的本地标识。
func appStableID(value string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(value))
	return strconv.FormatUint(uint64(h.Sum32()), 10)
}

// readRemoteAppList 下载并解析有界的远程应用目录压缩包。
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

// normalizeRemoteApps 将远程目录转换为本地应用记录结构。
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

// refreshRemoteLocked 在持有应用商店锁时刷新远程目录并持久化结果。
func (s *appStore) refreshRemoteLocked(force bool) error {
	if err := s.ensureCatalogLocked(); err != nil {
		return err
	}
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
	return s.saveCatalogLocked()
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
	if err := s.ensureCatalogLocked(); err != nil {
		return false, err
	}
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
	if err := s.saveCatalogLocked(); err != nil {
		return false, err
	}
	return true, nil
}

// RegisterAppRoutes 注册应用目录与已安装应用接口；运行时状态统一持久化到公共 SQLite。
