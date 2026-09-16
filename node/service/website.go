// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

var domainPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)

const defaultWebsiteIndexHTML = `<!doctype html>
<html>
<head><meta charset="utf-8"><meta name="robots" content="noindex,nofollow"><title>站点创建成功</title></head>
<body><h1>恭喜，站点创建成功！</h1><p>这是系统自动生成的默认 index.html。</p></body>
</html>
`

const defaultWebsite404HTML = `<html>
<head><title>404 Not Found</title></head>
<body><center><h1>404 Not Found</h1></center><hr><center>nginx</center></body>
</html>
`

// OpenRestyStatus 描述节点上 OpenResty 的真实探测结果。
type OpenRestyStatus struct {
	Available    bool                    `json:"available"`
	Binary       string                  `json:"binary,omitempty"`
	Version      string                  `json:"version,omitempty"`
	ConfigValid  bool                    `json:"configValid"`
	Enabled      bool                    `json:"enabled"`
	DefaultHTTPS bool                    `json:"defaultHttps"`
	Modules      []model.OpenRestyModule `json:"modules"`
	ProcessID    int                     `json:"processId,omitempty"`
	Cgroup       string                  `json:"cgroup,omitempty"`
	ConfigPath   string                  `json:"configPath,omitempty"`
	Listening    []int                   `json:"listening,omitempty"`
	Active       int                     `json:"active"`
	Accepts      int64                   `json:"accepts"`
	Handled      int64                   `json:"handled"`
	Requests     int64                   `json:"requests"`
	Reading      int                     `json:"reading"`
	Writing      int                     `json:"writing"`
	Waiting      int                     `json:"waiting"`
	Error        string                  `json:"error,omitempty"`
}

// OpenRestyScopeParams 是按配置作用域读取或更新的指令集合。
type OpenRestyScopeParams struct {
	Scope  string            `json:"scope"`
	Params map[string]string `json:"params"`
}

// WebsiteService 提供网站、WAF 和 OpenResty 的轻量本地控制面。
// 文件采用原子替换保存，节点未配置数据库时重启仍能保留配置。
type WebsiteService struct {
	mu              sync.RWMutex
	root            string
	websiteRoot     string
	websites        []model.Website
	wafSites        map[uint]model.WAFSite
	wafDefaultRules []model.WAFRule
	wafCustomRules  []model.WAFRule
	global          model.WAFGlobalConfig
	lists           model.WAFAccessLists
	openresty       model.OpenRestyConfig
	domains         map[uint][]model.WebsiteDomain
	configs         map[uint]map[string]any
	db              *sql.DB
	repository      storage.Transactional
	owner           *storage.Store
}

var (
	websiteDBMu sync.RWMutex
	websiteDB   *sql.DB
)

// SetWebsiteDB 注入进程唯一的公共数据库连接。
func SetWebsiteDB(db *sql.DB) error {
	if db == nil {
		return errors.New("网站公共数据库连接不能为空")
	}
	if err := ensureWebsiteTables(db); err != nil {
		return err
	}
	websiteDBMu.Lock()
	websiteDB = db
	websiteDBMu.Unlock()
	return nil
}

func currentWebsiteDB() *sql.DB {
	websiteDBMu.RLock()
	defer websiteDBMu.RUnlock()
	return websiteDB
}

func NewWebsiteService(root string) *WebsiteService {
	if strings.TrimSpace(root) == "" {
		root = os.Getenv("WORKMESH_DATA_DIR")
	}
	if strings.TrimSpace(root) == "" {
		root = "./data"
	}
	db := currentWebsiteDB()
	var owner *storage.Store
	if db == nil {
		if opened, err := storage.Open(filepath.Join(root, "workmesh.db")); err == nil {
			owner = opened
			db = opened.DB()
		}
	}
	if db != nil {
		_ = ensureWebsiteTables(db)
	}
	websiteRoot := strings.TrimSpace(os.Getenv("WORKMESH_WEBSITE_ROOT"))
	if websiteRoot == "" {
		websiteRoot = "/www/wwwroot"
	}
	var repository storage.Transactional
	if db != nil {
		repository, _ = storage.NewSQLiteRepository(db)
	}
	s := &WebsiteService{root: root, websiteRoot: websiteRoot, db: db, repository: repository, owner: owner, wafSites: map[uint]model.WAFSite{}, domains: map[uint][]model.WebsiteDomain{}, configs: map[uint]map[string]any{}}
	s.load()
	return s
}

// sqliteRepository 返回网站服务使用的事务边界；兼容手工构造的旧测试实例。
func (s *WebsiteService) sqliteRepository() (storage.Transactional, error) {
	if s == nil {
		return nil, errors.New("网站服务未初始化")
	}
	if s.repository != nil {
		return s.repository, nil
	}
	if s.db == nil {
		return nil, errors.New("网站公共数据库未初始化")
	}
	repository, err := storage.NewSQLiteRepository(s.db)
	if err != nil {
		return nil, err
	}
	s.repository = repository
	return repository, nil
}

// List 返回网站列表；limit 始终有界，避免管理端请求消耗无界内存。
func (s *WebsiteService) List(name string, offset, limit int) []model.Website {
	s.mu.RLock()
	defer s.mu.RUnlock()
	name = strings.ToLower(strings.TrimSpace(name))
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	result := make([]model.Website, 0, len(s.websites))
	for _, site := range s.websites {
		if name != "" && !strings.Contains(strings.ToLower(site.PrimaryDomain+" "+site.Alias), name) {
			continue
		}
		item := s.decorateWebsite(site)
		item.Domains = append([]model.WebsiteDomain(nil), s.domains[site.ID]...)
		result = append(result, item)
	}
	if offset >= len(result) {
		return []model.Website{}
	}
	end := offset + limit
	if end > len(result) {
		end = len(result)
	}
	return append([]model.Website(nil), result[offset:end]...)
}

// Count 返回匹配站点总数，用于分页响应的 total 字段。
func (s *WebsiteService) Count(name string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	name = strings.ToLower(strings.TrimSpace(name))
	count := 0
	for _, site := range s.websites {
		if name == "" || strings.Contains(strings.ToLower(site.PrimaryDomain+" "+site.Alias), name) {
			count++
		}
	}
	return count
}

// Get 根据 ID 查询网站。
func (s *WebsiteService) Get(id uint) (model.Website, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, site := range s.websites {
		if site.ID == id {
			site.Domains = append([]model.WebsiteDomain(nil), s.domains[id]...)
			return s.decorateWebsite(site), nil
		}
	}
	return model.Website{}, os.ErrNotExist
}

func (s *WebsiteService) decorateWebsite(site model.Website) model.Website {
	base := strings.TrimSpace(site.SiteDir)
	if base == "" {
		base = s.siteDirPath(site.PrimaryDomain)
	}
	site.SiteDir = filepath.Clean(base)
	site.SitePath = site.SiteDir
	site.Root = filepath.Join(site.SiteDir, "app")
	if strings.EqualFold(site.Type, "subsite") && site.ParentWebsiteID != 0 {
		for _, parent := range s.websites {
			if parent.ID == site.ParentWebsiteID {
				parentBase := strings.TrimSpace(parent.SiteDir)
				if parentBase == "" {
					parentBase = s.siteDirPath(parent.PrimaryDomain)
				}
				site.Root = filepath.Join(parentBase, "app")
				break
			}
		}
	}
	if cfg, ok := s.configs[site.ID]["dir"].(map[string]any); ok {
		if rel, ok := cfg["dir"].(string); ok {
			rootBase := filepath.Join(site.SiteDir, "app")
			if strings.EqualFold(site.Type, "subsite") && site.ParentWebsiteID != 0 {
				rootBase = site.Root
			}
			candidate := filepath.Join(rootBase, filepath.FromSlash(rel))
			if withinPath(rootBase, candidate) {
				site.Root = candidate
			}
		}
	}
	// Stream 页面与创建页面共用 Website 响应；从 SQLite-backed 配置恢复
	// 算法和真实上游，避免前端读到空配置后覆盖现有 stream.conf。
	if cfg, ok := s.configs[site.ID]["stream"].(map[string]any); ok {
		if algorithm := strings.TrimSpace(fmt.Sprint(cfg["algorithm"])); algorithm != "" {
			site.Algorithm = algorithm
		}
		if raw, ok := cfg["servers"]; ok {
			if encoded, err := json.Marshal(raw); err == nil {
				_ = json.Unmarshal(encoded, &site.Servers)
			}
		}
	}
	if repository, err := s.sqliteRepository(); site.WebsiteSSLID != 0 && err == nil {
		var raw string
		if repository.QueryRow(`SELECT expire_date FROM website_ssls WHERE id=?`, site.WebsiteSSLID).Scan(&raw) == nil {
			if value, err := time.Parse(time.RFC3339Nano, raw); err == nil {
				site.SSLExpireDate = &value
			}
		}
	}
	return site
}

func (s *WebsiteService) runtimeMetadata(runtimeID string) (string, int) {
	repository, err := s.sqliteRepository()
	if err != nil || strings.TrimSpace(runtimeID) == "" {
		return "", 0
	}
	var runtimeType string
	var payload []byte
	if err := repository.QueryRow(`SELECT type,payload FROM runtime_records WHERE id=?`, runtimeID).Scan(&runtimeType, &payload); err != nil {
		return runtimeType, 0
	}
	var data struct {
		Port int `json:"port"`
	}
	_ = json.Unmarshal(payload, &data)
	if data.Port == 0 {
		var raw []byte
		if err := repository.QueryRow(`SELECT attribute_value FROM runtime_attributes WHERE runtime_id=? AND attribute_key IN ('Port','port') ORDER BY attribute_key LIMIT 1`, runtimeID).Scan(&raw); err == nil {
			_ = json.Unmarshal(raw, &data.Port)
			if data.Port == 0 {
				var textValue string
				if json.Unmarshal(raw, &textValue) == nil {
					data.Port, _ = strconv.Atoi(strings.TrimSpace(textValue))
				}
			}
		}
	}
	return runtimeType, data.Port
}

func restoreWebsiteFiles(oldFiles map[string][]byte) error {
	var restoreErr error
	for path, old := range oldFiles {
		var err error
		if old == nil {
			err = os.Remove(path)
			if errors.Is(err, os.ErrNotExist) {
				err = nil
			}
		} else {
			err = writeWebsiteAtomic(path, old, 0o640)
		}
		if err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("恢复网站文件 %q 失败: %w", path, err))
		}
	}
	return restoreErr
}

func snapshotWebsiteFiles(paths ...string) (map[string][]byte, error) {
	files := make(map[string][]byte, len(paths))
	for _, path := range paths {
		old, err := os.ReadFile(path)
		if err == nil {
			files[path] = append([]byte(nil), old...)
			continue
		}
		if errors.Is(err, os.ErrNotExist) {
			files[path] = nil
			continue
		}
		return nil, fmt.Errorf("读取网站文件 %q 失败: %w", path, err)
	}
	return files, nil
}

func cloneWebsiteConfigs(configs map[uint]map[string]any) map[uint]map[string]any {
	clone := make(map[uint]map[string]any, len(configs))
	for websiteID, entries := range configs {
		if entries == nil {
			clone[websiteID] = nil
			continue
		}
		clone[websiteID] = make(map[string]any, len(entries))
		for key, value := range entries {
			clone[websiteID][key] = value
		}
	}
	return clone
}

// UpdateConfig 保存网站类型配置，配置键由调用方明确指定。
func (s *WebsiteService) UpdateConfig(websiteID uint, typ string, value map[string]any) (map[string]any, error) {
	if websiteID == 0 || strings.TrimSpace(typ) == "" || len(typ) > 64 {
		return nil, errors.New("网站配置参数无效")
	}
	// 在获取写锁前探测一次 OpenResty；校验助手不能在持有 s.mu 时调用
	// ProbeOpenResty，否则会产生读写锁自等待。
	openRestyStatus := s.ProbeOpenResty(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	website, err := s.getWebsiteLocked(websiteID)
	if err != nil {
		return nil, err
	}
	if value == nil {
		value = map[string]any{}
	}
	persistedValue := normalizeWebsiteSetting(typ, value)
	managedType := strings.ToLower(strings.TrimSpace(typ))
	if managedType == "auths" || managedType == "auth" || managedType == "path-auth" {
		persistedValue = make(map[string]any, len(value))
		for key, item := range value {
			if !strings.EqualFold(key, "password") {
				persistedValue[key] = item
			}
		}
		_, hasPassword := value["password"]
		persistedValue["hasPassword"] = hasPassword && websiteSettingString(value, "password") != ""
	}
	oldValue, hadOldValue := s.configs[websiteID][typ]
	oldFiles := map[string][]byte{}
	newFiles := map[string][]byte{}
	rememberFile := func(path string, content []byte) {
		if _, seen := oldFiles[path]; !seen {
			old, readErr := os.ReadFile(path)
			if readErr == nil {
				oldFiles[path] = append([]byte(nil), old...)
			} else if errors.Is(readErr, os.ErrNotExist) {
				oldFiles[path] = nil
			}
		}
		newFiles[path] = append([]byte(nil), content...)
	}
	if content, ok := value["content"].(string); ok {
		var target string
		switch strings.ToLower(strings.TrimSpace(typ)) {
		case "nginx":
			target = s.SitePath(website, "site.conf")
		case "stream":
			target = s.SitePath(website, "stream.conf")
		case "rewrite":
			target = s.SitePath(website, "rewrite")
		}
		if target != "" {
			updated := content
			if strings.EqualFold(typ, "nginx") {
				updated = s.syncManagedWebsiteIncludes(website, content)
			}
			rememberFile(target, []byte(updated))
		}
		if typ == "rewrite-custom" {
			dir := strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_REWRITE_DIR"))
			if dir == "" {
				dir = filepath.Join(s.root, "openresty", "rewrite")
			}
			name := website.Alias
			if name == "" {
				name = website.PrimaryDomain
			}
			name = regexp.MustCompile(`[^A-Za-z0-9_.-]+`).ReplaceAllString(name, "_")
			rememberFile(filepath.Join(dir, name+".conf"), []byte(content))
		}
	}
	renderValue := persistedValue
	if managedType == "auths" || managedType == "auth" || managedType == "path-auth" {
		renderValue = value
	}
	if rendered, renderErr := s.renderWebsiteSetting(website, typ, renderValue); renderErr != nil {
		return nil, renderErr
	} else {
		for path, content := range rendered {
			rememberFile(path, content)
		}
	}
	// Updating a managed setting changes the include graph as well as its
	// fragment. Preserve the previous site.conf for atomic rollback.
	if _, managed := map[string]bool{"proxy": true, "lbs": true, "cors": true, "realip": true, "leech": true, "hotlink": true, "redirect": true, "auths": true, "auth": true, "path-auth": true, "php": true, "nginx": true}[strings.ToLower(strings.TrimSpace(typ))]; managed {
		path := s.SitePath(website, "site.conf")
		current, readErr := os.ReadFile(path)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return nil, readErr
		}
		rememberFile(path, []byte(s.syncManagedWebsiteIncludesPending(website, string(current), newFiles)))
	}
	for path, content := range newFiles {
		if content == nil {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, errors.Join(err, restoreWebsiteFiles(oldFiles))
			}
			continue
		}
		mode := os.FileMode(0o640)
		if filepath.Base(path) == "users.htpasswd" {
			// OpenResty worker 运行在容器内的非宿主用户，必须能读取密码哈希；
			// 文件只保存不可逆 SHA 摘要，明文不会写入磁盘或 SQLite。
			mode = 0o644
		}
		if err := writeWebsiteAtomic(path, content, mode); err != nil {
			return nil, errors.Join(err, restoreWebsiteFiles(oldFiles))
		}
	}
	if managedType == "proxy" || managedType == "lbs" || managedType == "redirect" || managedType == "nginx" {
		if err := validateCreatedWebsiteConfig(context.Background(), openRestyStatus); err != nil {
			return nil, errors.Join(err, restoreWebsiteFiles(oldFiles))
		}
	}
	if s.configs[websiteID] == nil {
		s.configs[websiteID] = map[string]any{}
	}
	s.configs[websiteID][typ] = persistedValue
	if err := s.persist("website-configs", s.configs); err != nil {
		restoreErr := restoreWebsiteFiles(oldFiles)
		if hadOldValue {
			s.configs[websiteID][typ] = oldValue
		} else {
			delete(s.configs[websiteID], typ)
		}
		return nil, errors.Join(err, restoreErr)
	}
	return persistedValue, nil
}
