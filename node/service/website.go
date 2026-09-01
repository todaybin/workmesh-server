// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

var domainPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)

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
	mu        sync.RWMutex
	root      string
	websites  []model.Website
	wafSites  map[uint]model.WAFSite
	global    model.WAFGlobalConfig
	lists     model.WAFAccessLists
	openresty model.OpenRestyConfig
	domains   map[uint][]model.WebsiteDomain
	configs   map[uint]map[string]any
	db        *sql.DB
	owner     *storage.Store
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

func ensureWebsiteTables(db *sql.DB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS website_state (state_key TEXT PRIMARY KEY, payload BLOB NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS websites (id INTEGER PRIMARY KEY, primary_domain TEXT NOT NULL UNIQUE, payload BLOB NOT NULL, status TEXT NOT NULL, group_id INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_websites_group_status ON websites(group_id, status, id)`,
		`CREATE TABLE IF NOT EXISTS website_domains (id TEXT PRIMARY KEY, website_id INTEGER NOT NULL REFERENCES websites(id) ON DELETE CASCADE, domain TEXT NOT NULL, port INTEGER NOT NULL DEFAULT 80, ssl INTEGER NOT NULL DEFAULT 0, payload BLOB NOT NULL, UNIQUE(website_id, domain))`,
		`CREATE INDEX IF NOT EXISTS idx_website_domains_website ON website_domains(website_id, domain)`,
		`CREATE TABLE IF NOT EXISTS website_configs (website_id INTEGER NOT NULL REFERENCES websites(id) ON DELETE CASCADE, config_type TEXT NOT NULL, payload BLOB NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(website_id, config_type))`,
		`CREATE TABLE IF NOT EXISTS website_dns_accounts (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, provider TEXT NOT NULL, credentials BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("初始化网站数据库表失败: %w", err)
		}
	}
	return nil
}

// DNSAccount 是站点管理使用的 DNS provider 账户登记，不包含明文凭据返回值。
type DNSAccount struct {
	ID          uint           `json:"id"`
	Name        string         `json:"name"`
	Provider    string         `json:"provider"`
	Credentials map[string]any `json:"-"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

func (s *WebsiteService) ListDNSAccounts(keyword string, page, pageSize int) (int, []DNSAccount) {
	if s.db == nil {
		return 0, []DNSAccount{}
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	pattern := "%" + strings.ToLower(strings.TrimSpace(keyword)) + "%"
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM website_dns_accounts WHERE lower(name) LIKE ? OR lower(provider) LIKE ?`, pattern, pattern).Scan(&total)
	rows, err := s.db.Query(`SELECT id,name,provider,created_at,updated_at FROM website_dns_accounts WHERE lower(name) LIKE ? OR lower(provider) LIKE ? ORDER BY id DESC LIMIT ? OFFSET ?`, pattern, pattern, pageSize, (page-1)*pageSize)
	if err != nil {
		return total, []DNSAccount{}
	}
	defer rows.Close()
	items := []DNSAccount{}
	for rows.Next() {
		var item DNSAccount
		var created, updated string
		if rows.Scan(&item.ID, &item.Name, &item.Provider, &created, &updated) == nil {
			item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
			item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
			items = append(items, item)
		}
	}
	return total, items
}

func (s *WebsiteService) UpsertDNSAccount(item DNSAccount) (DNSAccount, error) {
	if s.db == nil {
		return DNSAccount{}, errors.New("网站公共数据库未初始化")
	}
	item.Name, item.Provider = strings.TrimSpace(item.Name), strings.TrimSpace(item.Provider)
	if item.Name == "" || item.Provider == "" {
		return DNSAccount{}, errors.New("DNS 账户名称和提供商不能为空")
	}
	cred, _ := json.Marshal(item.Credentials)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if item.ID == 0 {
		res, err := s.db.Exec(`INSERT INTO website_dns_accounts(name,provider,credentials,created_at,updated_at) VALUES(?,?,?,?,?)`, item.Name, item.Provider, cred, now, now)
		if err != nil {
			return DNSAccount{}, err
		}
		id, _ := res.LastInsertId()
		item.ID = uint(id)
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	} else {
		if _, err := s.db.Exec(`UPDATE website_dns_accounts SET name=?,provider=?,credentials=?,updated_at=? WHERE id=?`, item.Name, item.Provider, cred, now, item.ID); err != nil {
			return DNSAccount{}, err
		}
	}
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, now)
	item.Credentials = nil
	return item, nil
}

func (s *WebsiteService) DeleteDNSAccount(id uint) error {
	if s.db == nil {
		return errors.New("网站公共数据库未初始化")
	}
	res, err := s.db.Exec(`DELETE FROM website_dns_accounts WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return os.ErrNotExist
	}
	return nil
}

// NewWebsiteService 创建服务并从数据目录加载已有状态。
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
	s := &WebsiteService{root: root, db: db, owner: owner, wafSites: map[uint]model.WAFSite{}, domains: map[uint][]model.WebsiteDomain{}, configs: map[uint]map[string]any{}}
	s.load()
	return s
}

func (s *WebsiteService) load() {
	read := func(key string, target any) bool {
		if s.db == nil {
			return false
		}
		var data []byte
		if err := s.db.QueryRow("SELECT payload FROM website_state WHERE state_key = ?", key).Scan(&data); err != nil {
			return false
		}
		return json.Unmarshal(data, target) == nil
	}
	_ = read("websites", &s.websites)
	if len(s.websites) == 0 && s.db != nil {
		if rows, err := s.db.Query(`SELECT payload FROM websites ORDER BY id`); err == nil {
			defer rows.Close()
			for rows.Next() {
				var payload []byte
				var item model.Website
				if rows.Scan(&payload) == nil && json.Unmarshal(payload, &item) == nil {
					s.websites = append(s.websites, item)
				}
			}
		}
	}
	var sites []model.WAFSite
	if read("waf-sites", &sites) {
		for _, site := range sites {
			if site.Rules == nil {
				site.Rules = []model.WAFRule{}
			}
			s.wafSites[site.WebsiteID] = site
		}
	}
	if !read("waf-global", &s.global) {
		s.global = model.WAFGlobalConfig{Enabled: true, StandardRules: true, Mode: "observe", ParanoiaLevel: 1, InboundThreshold: 5, RequestBodyLimit: 1 << 20}
	}
	if !read("waf-access-lists", &s.lists) {
		s.lists = model.WAFAccessLists{Whitelist: []string{}, Blacklist: []string{}}
	}
	if !read("openresty", &s.openresty) {
		s.openresty = model.OpenRestyConfig{Version: "1.27.1", Enabled: true, Modules: []model.OpenRestyModule{}}
	}
	_ = read("website-domains", &s.domains)
	_ = read("website-configs", &s.configs)
	if s.domains == nil {
		s.domains = map[uint][]model.WebsiteDomain{}
	}
	if s.configs == nil {
		s.configs = map[uint]map[string]any{}
	}
	if s.websites == nil {
		s.websites = []model.Website{}
	}
}

func (s *WebsiteService) persist(name string, value any) error {
	if s.db == nil {
		return errors.New("网站公共数据库未初始化")
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	key := strings.TrimSuffix(name, ".json")
	_, err = s.db.Exec(`INSERT INTO website_state(state_key,payload,updated_at) VALUES(?,?,?) ON CONFLICT(state_key) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`, key, data, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if key == "websites" {
		var items []model.Website
		if json.Unmarshal(data, &items) == nil {
			if _, err := s.db.Exec("DELETE FROM websites"); err != nil {
				return err
			}
			for _, item := range items {
				payload, _ := json.Marshal(item)
				if _, err := s.db.Exec(`INSERT INTO websites(id,primary_domain,payload,status,group_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET primary_domain=excluded.primary_domain,payload=excluded.payload,status=excluded.status,group_id=excluded.group_id,updated_at=excluded.updated_at`, item.ID, item.PrimaryDomain, payload, item.Status, item.WebsiteGroupID, item.CreatedAt.UTC().Format(time.RFC3339Nano), item.UpdatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
					return err
				}
			}
		}
	}
	return err
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
		base = filepath.Join(s.root, "websites", strconv.FormatUint(uint64(site.ID), 10))
	}
	site.SiteDir = filepath.Clean(base)
	site.SitePath = site.SiteDir
	site.Root = site.SiteDir
	return site
}

// Create 创建网站并初始化对应 WAF 配置。
func (s *WebsiteService) Create(req model.WebsiteCreateRequest) (model.Website, error) {
	domain := strings.TrimSpace(req.PrimaryDomain)
	if domain == "" && len(req.Domains) > 0 {
		domain = strings.TrimSpace(req.Domains[0].Domain)
	}
	if domain == "" {
		domain = strings.TrimSpace(req.Name)
	}
	if !domainPattern.MatchString(domain) || strings.Contains(domain, "..") {
		return model.Website{}, errors.New("主域名格式无效")
	}
	if strings.ContainsRune(req.SiteDir, 0) || strings.Contains(filepath.Clean(req.SiteDir), "..") || len(req.SiteDir) > 4096 {
		return model.Website{}, errors.New("站点目录无效")
	}
	if strings.TrimSpace(req.SiteDir) == "" && strings.EqualFold(domain, "znmp.sopvip.com") {
		req.SiteDir = "/www/wwwroot/znmp.sopvip.com"
	}
	// 已能探测到 OpenResty 时，创建站点前必须通过真实配置语法检查；未安装时由预检接口返回明确状态。
	if status := s.ProbeOpenResty(context.Background()); status.Available && !status.ConfigValid && !strings.Contains(status.Error, "无法进入") && !strings.Contains(status.Error, "无权限") {
		return model.Website{}, fmt.Errorf("OpenResty 配置语法检查失败: %s", status.Error)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, site := range s.websites {
		if strings.EqualFold(site.PrimaryDomain, domain) {
			return model.Website{}, errors.New("主域名已存在")
		}
	}
	var id uint = 1
	for _, site := range s.websites {
		if site.ID >= id {
			id = site.ID + 1
		}
	}
	now := time.Now().UTC()
	protocol := strings.ToUpper(strings.TrimSpace(req.Protocol))
	if protocol == "" {
		protocol = "HTTP"
	}
	if protocol != "HTTP" && protocol != "HTTPS" {
		return model.Website{}, errors.New("协议类型无效")
	}
	sslID := req.WebsiteSSLID
	if sslID == 0 {
		sslID = req.SSLID
	}
	errorLog, accessLog := true, true
	if req.ErrorLog != nil {
		errorLog = *req.ErrorLog
	}
	if req.AccessLog != nil {
		accessLog = *req.AccessLog
	}
	site := model.Website{ID: id, PrimaryDomain: domain, Alias: strings.TrimSpace(req.Alias), Type: strings.TrimSpace(req.Type), Remark: strings.TrimSpace(req.Remark), SiteDir: strings.TrimSpace(req.SiteDir), Status: "running", Protocol: protocol, HttpConfig: strings.TrimSpace(req.HttpConfig), Proxy: strings.TrimSpace(req.Proxy), ProxyType: strings.TrimSpace(req.ProxyType), ErrorLog: errorLog, AccessLog: accessLog, DefaultServer: req.DefaultServer, IPV6: req.IPV6, Rewrite: strings.TrimSpace(req.Rewrite), WebsiteSSLID: sslID, RuntimeID: req.RuntimeID, AppInstallID: req.AppInstallID, FtpID: req.FtpID, ParentWebsiteID: req.ParentWebsiteID, User: strings.TrimSpace(req.User), Group: strings.TrimSpace(req.Group), DbType: strings.TrimSpace(req.DbType), DbID: req.DbID, StreamPorts: strings.TrimSpace(req.StreamPorts), UDP: req.UDP, WebsiteGroupID: req.WebsiteGroupID, ExpireDate: time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC), CreatedAt: now, UpdatedAt: now}
	if site.Alias == "" {
		site.Alias = domain
	}
	if site.Type == "" {
		site.Type = strings.TrimSpace(req.AppType)
	}
	if site.Type == "" {
		site.Type = "static"
	}
	s.websites = append(s.websites, site)
	if len(req.Domains) > 0 {
		for i := range req.Domains {
			req.Domains[i].WebsiteID = id
			if req.Domains[i].ID == "" {
				req.Domains[i].ID = fmt.Sprintf("domain-%d", time.Now().UnixNano()+int64(i))
			}
			if req.Domains[i].Port == 0 {
				req.Domains[i].Port = 80
			}
		}
		s.domains[id] = append([]model.WebsiteDomain(nil), req.Domains...)
	}
	s.wafSites[id] = model.WAFSite{WebsiteID: id, Alias: site.Alias, Enabled: true, Mode: "observe", Rules: []model.WAFRule{}}
	if err := s.persist("websites.json", s.websites); err != nil {
		return model.Website{}, err
	}
	if err := s.persistWAFSites(); err != nil {
		return model.Website{}, err
	}
	if err := s.persist("website-domains.json", s.domains); err != nil {
		return model.Website{}, err
	}
	site.Domains = append([]model.WebsiteDomain(nil), s.domains[id]...)
	return site, nil
}

// OperateCrossSiteAccess 根据原系统约定创建或删除站点根目录 .user.ini。
func (s *WebsiteService) OperateCrossSiteAccess(id uint, operation string) error {
	if operation != "Enable" && operation != "Disable" && operation != "enable" && operation != "disable" {
		return errors.New("跨站访问操作无效")
	}
	s.mu.RLock()
	site, err := s.getWebsiteLocked(id)
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	base := strings.TrimSpace(site.SiteDir)
	if base == "" {
		base = filepath.Join(s.root, "websites", strconv.FormatUint(uint64(id), 10))
	}
	path := filepath.Join(base, ".user.ini")
	if strings.EqualFold(operation, "Enable") {
		if err := os.MkdirAll(base, 0o750); err != nil {
			return err
		}
		return os.WriteFile(path, []byte("open_basedir=\n"), 0o600)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Update 更新网站可编辑字段。
func (s *WebsiteService) Update(req model.WebsiteUpdateRequest) (model.Website, error) {
	if req.ID == 0 {
		return model.Website{}, errors.New("网站 ID 无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.websites {
		if s.websites[i].ID != req.ID {
			continue
		}
		if strings.TrimSpace(req.PrimaryDomain) != "" {
			candidate := strings.TrimSpace(req.PrimaryDomain)
			if !domainPattern.MatchString(candidate) || strings.Contains(candidate, "..") {
				return model.Website{}, errors.New("主域名格式无效")
			}
			for j, other := range s.websites {
				if j != i && strings.EqualFold(other.PrimaryDomain, candidate) {
					return model.Website{}, errors.New("主域名已存在")
				}
			}
			s.websites[i].PrimaryDomain = candidate
		}
		if strings.TrimSpace(req.Alias) != "" {
			s.websites[i].Alias = strings.TrimSpace(req.Alias)
		}
		if req.Remark != "" {
			s.websites[i].Remark = strings.TrimSpace(req.Remark)
		}
		if req.SiteDir != "" {
			s.websites[i].SiteDir = strings.TrimSpace(req.SiteDir)
		}
		s.websites[i].Favorite = req.Favorite
		if req.WebsiteGroupID != 0 {
			s.websites[i].WebsiteGroupID = req.WebsiteGroupID
		}
		if req.ExpireDate != nil {
			s.websites[i].ExpireDate = req.ExpireDate.UTC()
		}
		if req.IPV6 != nil {
			s.websites[i].IPV6 = *req.IPV6
		}
		if req.WebsiteSSLID != nil {
			s.websites[i].WebsiteSSLID = *req.WebsiteSSLID
		}
		if req.Protocol != "" {
			p := strings.ToUpper(strings.TrimSpace(req.Protocol))
			if p != "HTTP" && p != "HTTPS" {
				return model.Website{}, errors.New("协议类型无效")
			}
			s.websites[i].Protocol = p
		}
		if req.HttpConfig != "" {
			s.websites[i].HttpConfig = strings.TrimSpace(req.HttpConfig)
		}
		if req.Proxy != "" {
			s.websites[i].Proxy = strings.TrimSpace(req.Proxy)
		}
		if req.ProxyType != "" {
			s.websites[i].ProxyType = strings.TrimSpace(req.ProxyType)
		}
		if req.ErrorLog != nil {
			s.websites[i].ErrorLog = *req.ErrorLog
		}
		if req.AccessLog != nil {
			s.websites[i].AccessLog = *req.AccessLog
		}
		if req.DefaultServer != nil {
			s.websites[i].DefaultServer = *req.DefaultServer
		}
		if req.Rewrite != "" {
			s.websites[i].Rewrite = strings.TrimSpace(req.Rewrite)
		}
		if req.RuntimeID != nil {
			s.websites[i].RuntimeID = *req.RuntimeID
		}
		if req.AppInstallID != nil {
			s.websites[i].AppInstallID = *req.AppInstallID
		}
		if req.FtpID != nil {
			s.websites[i].FtpID = *req.FtpID
		}
		if req.ParentWebsiteID != nil {
			s.websites[i].ParentWebsiteID = *req.ParentWebsiteID
		}
		if req.User != "" {
			s.websites[i].User = strings.TrimSpace(req.User)
		}
		if req.Group != "" {
			s.websites[i].Group = strings.TrimSpace(req.Group)
		}
		if req.DbType != "" {
			s.websites[i].DbType = strings.TrimSpace(req.DbType)
		}
		if req.DbID != nil {
			s.websites[i].DbID = *req.DbID
		}
		if req.StreamPorts != "" {
			s.websites[i].StreamPorts = strings.TrimSpace(req.StreamPorts)
		}
		if req.UDP != nil {
			s.websites[i].UDP = *req.UDP
		}
		s.websites[i].UpdatedAt = time.Now().UTC()
		if site, ok := s.wafSites[req.ID]; ok {
			site.Alias = s.websites[i].Alias
			s.wafSites[req.ID] = site
		}
		if err := s.persist("websites.json", s.websites); err != nil {
			return model.Website{}, err
		}
		_ = s.persistWAFSites()
		s.websites[i].Domains = append([]model.WebsiteDomain(nil), s.domains[req.ID]...)
		return s.websites[i], nil
	}
	return model.Website{}, os.ErrNotExist
}

// Delete 删除网站及其 WAF 规则。
func (s *WebsiteService) Delete(id uint) error {
	if id == 0 {
		return errors.New("网站 ID 无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, site := range s.websites {
		if site.ID != id {
			continue
		}
		s.websites = append(s.websites[:i], s.websites[i+1:]...)
		delete(s.wafSites, id)
		delete(s.domains, id)
		delete(s.configs, id)
		if err := s.persist("websites.json", s.websites); err != nil {
			return err
		}
		if err := s.persistWAFSites(); err != nil {
			return err
		}
		if err := s.persist("website-domains.json", s.domains); err != nil {
			return err
		}
		return s.persist("website-configs.json", s.configs)
	}
	return os.ErrNotExist
}

// Operate 更新网站运行状态，支持启动、停止和重启。
func (s *WebsiteService) Operate(id uint, operation string) (model.Website, error) {
	operation = strings.ToLower(strings.TrimSpace(operation))
	if operation != "start" && operation != "stop" && operation != "restart" {
		return model.Website{}, errors.New("网站操作必须是 start、stop 或 restart")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.websites {
		if s.websites[i].ID != id {
			continue
		}
		if operation == "stop" {
			s.websites[i].Status = "stopped"
		} else {
			s.websites[i].Status = "running"
		}
		s.websites[i].UpdatedAt = time.Now().UTC()
		if err := s.persist("websites.json", s.websites); err != nil {
			return model.Website{}, err
		}
		return s.websites[i], nil
	}
	return model.Website{}, os.ErrNotExist
}

// UpdateHTTPS 保存站点 HTTPS 开关及证书关联，并同步站点协议字段。
func (s *WebsiteService) UpdateHTTPS(id uint, enabled bool, sslID uint, httpConfig string) (model.Website, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.websites {
		if s.websites[i].ID != id {
			continue
		}
		if enabled {
			s.websites[i].Protocol = "HTTPS"
		} else if s.websites[i].Protocol == "HTTPS" {
			s.websites[i].Protocol = "HTTP"
		}
		if sslID != 0 {
			s.websites[i].WebsiteSSLID = sslID
		}
		if strings.TrimSpace(httpConfig) != "" {
			s.websites[i].HttpConfig = strings.TrimSpace(httpConfig)
		}
		s.websites[i].UpdatedAt = time.Now().UTC()
		if err := s.persist("websites.json", s.websites); err != nil {
			return model.Website{}, err
		}
		return s.websites[i], nil
	}
	return model.Website{}, os.ErrNotExist
}

// SetGroups 批量更新网站分组并一次持久化。
func (s *WebsiteService) SetGroups(ids []uint, groupID uint) error {
	if len(ids) == 0 || groupID == 0 {
		return errors.New("网站分组参数无效")
	}
	wanted := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	updated := 0
	for i := range s.websites {
		if _, ok := wanted[s.websites[i].ID]; ok {
			s.websites[i].WebsiteGroupID = groupID
			s.websites[i].UpdatedAt = time.Now().UTC()
			updated++
		}
	}
	if updated != len(wanted) {
		return errors.New("部分网站不存在")
	}
	return s.persist("websites.json", s.websites)
}

// GroupInUse 判断分组是否仍被网站引用。
func (s *WebsiteService) GroupInUse(groupID uint) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, website := range s.websites {
		if website.WebsiteGroupID == groupID {
			return true
		}
	}
	return false
}

// WebsiteLog 返回站点 access.log/error.log 的真实内容，按页读取避免一次性加载大文件。
func (s *WebsiteService) WebsiteLog(id uint, logType string, page, pageSize int) (map[string]any, error) {
	if logType != "access.log" && logType != "error.log" {
		return nil, errors.New("日志类型无效")
	}
	s.mu.RLock()
	site, err := s.getWebsiteLocked(id)
	if err != nil {
		s.mu.RUnlock()
		return nil, err
	}
	enabled := site.AccessLog
	if logType == "error.log" {
		enabled = site.ErrorLog
	}
	path := s.websiteLogPath(site, logType)
	s.mu.RUnlock()
	result := map[string]any{"enable": enabled, "content": "", "end": true, "path": path}
	if !enabled {
		return result, nil
	}
	data, readErr := os.ReadFile(path)
	if errors.Is(readErr, os.ErrNotExist) {
		return result, nil
	}
	if readErr != nil {
		return nil, readErr
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 5000 {
		pageSize = 100
	}
	start := (page - 1) * pageSize
	if start >= len(lines) {
		result["end"] = true
		return result, nil
	}
	end := start + pageSize
	if end > len(lines) {
		end = len(lines)
	}
	result["content"] = strings.Join(lines[start:end], "\n")
	result["end"] = end >= len(lines)
	return result, nil
}

// OperateWebsiteLog 持久化日志开关并执行清理操作；配置文件由站点生成器后续同步。
func (s *WebsiteService) OperateWebsiteLog(id uint, logType, operation string) error {
	if logType != "access.log" && logType != "error.log" {
		return errors.New("日志类型无效")
	}
	if operation != "enable" && operation != "disable" && operation != "delete" {
		return errors.New("日志操作无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.websites {
		if s.websites[i].ID != id {
			continue
		}
		if operation == "delete" {
			if err := os.WriteFile(s.websiteLogPath(s.websites[i], logType), nil, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		}
		enabled := operation == "enable"
		if logType == "access.log" {
			s.websites[i].AccessLog = enabled
		} else {
			s.websites[i].ErrorLog = enabled
		}
		return s.persist("websites.json", s.websites)
	}
	return os.ErrNotExist
}

func (s *WebsiteService) websiteLogPath(site model.Website, logType string) string {
	base := strings.TrimSpace(site.SiteDir)
	if base == "" {
		base = filepath.Join(s.root, "websites", strconv.FormatUint(uint64(site.ID), 10))
	}
	return filepath.Join(base, "logs", logType)
}

// ListDomains 返回网站域名，并限制最大返回数量。
func (s *WebsiteService) ListDomains(websiteID uint) ([]model.WebsiteDomain, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.getWebsiteLocked(websiteID); err != nil {
		return nil, err
	}
	return append([]model.WebsiteDomain(nil), s.domains[websiteID]...), nil
}

// UpsertDomain 新增或更新域名记录，并校验端口和域名格式。
func (s *WebsiteService) UpsertDomain(domain model.WebsiteDomain) (model.WebsiteDomain, error) {
	if domain.WebsiteID == 0 || !domainPattern.MatchString(strings.TrimSpace(domain.Domain)) || strings.Contains(domain.Domain, "..") {
		return model.WebsiteDomain{}, errors.New("网站域名无效")
	}
	if domain.Port == 0 {
		domain.Port = 80
	}
	if domain.Port < 1 || domain.Port > 65535 {
		return model.WebsiteDomain{}, errors.New("网站端口超出范围")
	}
	domain.Domain = strings.TrimSpace(domain.Domain)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getWebsiteLocked(domain.WebsiteID); err != nil {
		return model.WebsiteDomain{}, err
	}
	if domain.ID == "" {
		domain.ID = fmt.Sprintf("domain-%d", time.Now().UnixNano())
	}
	items := s.domains[domain.WebsiteID]
	for _, site := range s.websites {
		if site.ID == domain.WebsiteID && strings.EqualFold(site.PrimaryDomain, domain.Domain) {
			return model.WebsiteDomain{}, errors.New("网站域名已被主域名占用")
		}
		for _, existing := range s.domains[site.ID] {
			if existing.ID != domain.ID && strings.EqualFold(existing.Domain, domain.Domain) {
				return model.WebsiteDomain{}, errors.New("网站域名已存在")
			}
		}
	}
	for i := range items {
		if items[i].ID == domain.ID {
			items[i] = domain
			s.domains[domain.WebsiteID] = items
			return domain, s.persist("website-domains.json", s.domains)
		}
	}
	s.domains[domain.WebsiteID] = append(items, domain)
	return domain, s.persist("website-domains.json", s.domains)
}

// DeleteDomain 删除指定网站域名。
func (s *WebsiteService) DeleteDomain(websiteID uint, domainID string) error {
	if websiteID == 0 || strings.TrimSpace(domainID) == "" {
		return errors.New("网站域名参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.domains[websiteID]
	for i, item := range items {
		if item.ID == domainID {
			s.domains[websiteID] = append(items[:i], items[i+1:]...)
			return s.persist("website-domains.json", s.domains)
		}
	}
	return os.ErrNotExist
}

// GetConfig 读取网站类型配置；未设置时返回空对象而非固定业务数据。
func (s *WebsiteService) GetConfig(websiteID uint, typ string) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.getWebsiteLocked(websiteID); err != nil {
		return nil, err
	}
	result := map[string]any{}
	for key, value := range s.configs[websiteID] {
		result[key] = value
	}
	if typ != "" {
		if value, ok := result[typ]; ok {
			if typed, ok := value.(map[string]any); ok {
				return typed, nil
			}
		}
	}
	return result, nil
}

// BasicConfig 返回站点 Basic 页面所需的真实目录、配置和日志状态。
func (s *WebsiteService) BasicConfig(id uint) (map[string]any, error) {
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	content, configErr := s.OpenRestyFile()
	defaultDocuments := []string{}
	if configErr == nil {
		for _, match := range regexp.MustCompile(`(?m)^\s*index\s+([^;]+);`).FindAllStringSubmatch(content, -1) {
			defaultDocuments = append(defaultDocuments, strings.Fields(match[1])...)
		}
	}
	if len(defaultDocuments) == 0 {
		defaultDocuments = []string{"index.html", "index.htm"}
	}
	accessPath, errorPath := s.websiteLogPath(site, "access.log"), s.websiteLogPath(site, "error.log")
	logStatus := func(path string, enabled bool) map[string]any {
		entry := map[string]any{"path": path, "enabled": enabled, "exists": false}
		if !enabled {
			entry["status"] = "disabled"
			return entry
		}
		if _, statErr := os.Stat(path); statErr == nil {
			entry["exists"], entry["status"] = true, "available"
		} else if errors.Is(statErr, os.ErrNotExist) {
			entry["status"] = "missing"
		} else {
			entry["status"], entry["error"] = "unavailable", statErr.Error()
		}
		return entry
	}
	traffic := map[string]any{"status": "missing", "requests": 0, "bytes": int64(0), "source": accessPath}
	if data, readErr := os.ReadFile(accessPath); readErr == nil {
		requests, bytes := accessLogTraffic(string(data))
		traffic = map[string]any{"status": "available", "requests": requests, "bytes": bytes, "source": accessPath}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		traffic["status"], traffic["error"] = "unavailable", readErr.Error()
	}
	configError := ""
	if configErr != nil {
		configError = configErr.Error()
	}
	return map[string]any{
		"website": site, "id": site.ID, "domains": append([]model.WebsiteDomain(nil), site.Domains...),
		"siteDir": site.SiteDir, "sitePath": site.SitePath, "root": site.Root,
		"defaultDocuments": defaultDocuments, "accessLog": logStatus(accessPath, site.AccessLog), "errorLog": logStatus(errorPath, site.ErrorLog),
		"traffic": traffic, "proxy": site.Proxy, "https": site.Protocol == "HTTPS", "rewrite": site.Rewrite,
		"configPath": s.openRestyConfigPath(), "configError": configError,
	}, nil
}

func accessLogTraffic(content string) (int, int64) {
	requests := 0
	var bytes int64
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		requests++
		if len(fields) >= 10 {
			if value, err := strconv.ParseInt(fields[9], 10, 64); err == nil && value > 0 {
				bytes += value
			}
		}
	}
	return requests, bytes
}

// UpdateConfig 保存网站类型配置，配置键由调用方明确指定。
func (s *WebsiteService) UpdateConfig(websiteID uint, typ string, value map[string]any) (map[string]any, error) {
	if websiteID == 0 || strings.TrimSpace(typ) == "" || len(typ) > 64 {
		return nil, errors.New("网站配置参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getWebsiteLocked(websiteID); err != nil {
		return nil, err
	}
	if s.configs[websiteID] == nil {
		s.configs[websiteID] = map[string]any{}
	}
	s.configs[websiteID][typ] = value
	if err := s.persist("website-configs.json", s.configs); err != nil {
		return nil, err
	}
	return value, nil
}

// ListCustomRewrites 返回已实际保存的站点级自定义 rewrite 资源，不生成固定演示条目。
func (s *WebsiteService) ListCustomRewrites() []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]map[string]any, 0)
	for _, site := range s.websites {
		value, ok := s.configs[site.ID]["rewrite-custom"]
		if !ok {
			continue
		}
		config, ok := value.(map[string]any)
		if !ok {
			continue
		}
		items = append(items, map[string]any{"websiteID": site.ID, "name": site.Alias, "domain": site.PrimaryDomain, "content": config["content"]})
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["websiteID"].(uint) < items[j]["websiteID"].(uint) })
	return items
}

func (s *WebsiteService) persistWAFSites() error {
	items := make([]model.WAFSite, 0, len(s.wafSites))
	for _, site := range s.wafSites {
		items = append(items, site)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].WebsiteID < items[j].WebsiteID })
	return s.persist("waf-sites.json", items)
}

// ListWAFSites 返回所有网站 WAF 配置；已存在网站但尚无配置时自动补齐默认项。
func (s *WebsiteService) ListWAFSites() []model.WAFSite {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, site := range s.websites {
		if _, ok := s.wafSites[site.ID]; !ok {
			s.wafSites[site.ID] = model.WAFSite{WebsiteID: site.ID, Alias: site.Alias, Enabled: true, Mode: "observe", Rules: []model.WAFRule{}}
		}
	}
	result := make([]model.WAFSite, 0, len(s.wafSites))
	for _, site := range s.wafSites {
		result = append(result, site)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].WebsiteID < result[j].WebsiteID })
	return result
}

// UpdateWAFSite 更新网站 WAF 开关和模式。
func (s *WebsiteService) UpdateWAFSite(id uint, enabled bool, mode string) (model.WAFSite, error) {
	if id == 0 || (mode != "observe" && mode != "block") {
		return model.WAFSite{}, errors.New("网站 WAF 参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	website, err := s.getWebsiteLocked(id)
	if err != nil {
		return model.WAFSite{}, err
	}
	site := s.wafSites[id]
	site.WebsiteID, site.Enabled, site.Mode = id, enabled, mode
	if site.Alias == "" {
		site.Alias = website.Alias
	}
	if site.Rules == nil {
		site.Rules = []model.WAFRule{}
	}
	s.wafSites[id] = site
	return site, s.persistWAFSites()
}

func (s *WebsiteService) getWebsiteLocked(id uint) (model.Website, error) {
	for _, site := range s.websites {
		if site.ID == id {
			return site, nil
		}
	}
	return model.Website{}, os.ErrNotExist
}

// ListRules 返回按优先级排序的规则。
func (s *WebsiteService) ListRules(id uint) ([]model.WAFRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.getWebsiteLocked(id); err != nil {
		return nil, err
	}
	rules := append([]model.WAFRule(nil), s.wafSites[id].Rules...)
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Priority < rules[j].Priority })
	return rules, nil
}

// UpsertRule 新增或更新结构化规则。
func (s *WebsiteService) UpsertRule(id uint, rule model.WAFRule) (model.WAFRule, error) {
	if id == 0 || strings.TrimSpace(rule.Name) == "" || len(rule.Name) > 120 || strings.TrimSpace(rule.Value) == "" || len(rule.Value) > 2048 {
		return model.WAFRule{}, errors.New("WAF 规则参数无效")
	}
	validLocation, validOperator, validAction := map[string]bool{"ip": true, "uri": true, "args": true, "header": true, "cookie": true, "method": true, "body": true}, map[string]bool{"contains": true, "regex": true, "equals": true, "ip-cidr": true}, map[string]bool{"allow": true, "log": true, "block": true}
	if !validLocation[rule.Location] || !validOperator[rule.Operator] || !validAction[rule.Action] {
		return model.WAFRule{}, errors.New("WAF 规则字段无效")
	}
	if rule.Priority <= 0 {
		rule.Priority = 100
	}
	if rule.Priority > 10000 {
		return model.WAFRule{}, errors.New("WAF 规则优先级无效")
	}
	if rule.Operator == "regex" {
		if len(rule.Value) > 256 {
			return model.WAFRule{}, errors.New("正则表达式长度不能超过 256")
		}
		if _, err := regexp.Compile(rule.Value); err != nil {
			return model.WAFRule{}, fmt.Errorf("正则表达式无效: %w", err)
		}
	}
	if rule.Operator == "ip-cidr" {
		if _, _, err := net.ParseCIDR(rule.Value); err != nil {
			return model.WAFRule{}, errors.New("IP 网段无效")
		}
	}
	if strings.TrimSpace(rule.ID) == "" {
		rule.ID = fmt.Sprintf("rule-%d", time.Now().UnixNano())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getWebsiteLocked(id); err != nil {
		return model.WAFRule{}, err
	}
	site := s.wafSites[id]
	site.WebsiteID, site.Rules = id, append([]model.WAFRule(nil), site.Rules...)
	found := false
	for i := range site.Rules {
		if site.Rules[i].ID == rule.ID {
			site.Rules[i], found = rule, true
			break
		}
	}
	if !found {
		site.Rules = append(site.Rules, rule)
	}
	s.wafSites[id] = site
	return rule, s.persistWAFSites()
}

// DeleteRule 删除指定网站规则。
func (s *WebsiteService) DeleteRule(id uint, ruleID string) error {
	if id == 0 || strings.TrimSpace(ruleID) == "" {
		return errors.New("WAF 规则参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getWebsiteLocked(id); err != nil {
		return err
	}
	site := s.wafSites[id]
	filtered := site.Rules[:0]
	for _, rule := range site.Rules {
		if rule.ID != ruleID {
			filtered = append(filtered, rule)
		}
	}
	site.Rules = filtered
	s.wafSites[id] = site
	return s.persistWAFSites()
}

// GetGlobal、UpdateGlobal 读取和更新节点级配置。
func (s *WebsiteService) GetGlobal() model.WAFGlobalConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.global
}
func (s *WebsiteService) UpdateGlobal(cfg model.WAFGlobalConfig) (model.WAFGlobalConfig, error) {
	if cfg.Mode != "observe" && cfg.Mode != "block" {
		return model.WAFGlobalConfig{}, errors.New("WAF 模式无效")
	}
	if cfg.ParanoiaLevel == 0 {
		cfg.ParanoiaLevel = 1
	}
	if cfg.ParanoiaLevel < 1 || cfg.ParanoiaLevel > 4 || cfg.InboundThreshold < 1 || cfg.InboundThreshold > 99 || cfg.RequestBodyLimit < 0 || cfg.RequestBodyLimit > 10485760 {
		return model.WAFGlobalConfig{}, errors.New("WAF 全局参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.global = cfg
	return cfg, s.persist("waf-global.json", cfg)
}
func (s *WebsiteService) GetLists() model.WAFAccessLists {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lists
}

// UpdateLists 校验 IP/CIDR、去重并持久化黑白名单。
func (s *WebsiteService) UpdateLists(lists model.WAFAccessLists) (model.WAFAccessLists, error) {
	clean := func(items []string) ([]string, error) {
		seen := map[string]bool{}
		out := []string{}
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if net.ParseIP(item) == nil {
				if _, _, err := net.ParseCIDR(item); err != nil {
					return nil, fmt.Errorf("IP 或网段无效: %s", item)
				}
			}
			if !seen[item] {
				seen[item] = true
				out = append(out, item)
			}
		}
		return out, nil
	}
	whitelist, err := clean(lists.Whitelist)
	if err != nil {
		return model.WAFAccessLists{}, err
	}
	blacklist, err := clean(lists.Blacklist)
	if err != nil {
		return model.WAFAccessLists{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lists = model.WAFAccessLists{Whitelist: whitelist, Blacklist: blacklist}
	return s.lists, s.persist("waf-access-lists.json", s.lists)
}

// StandardRules 返回离线可用的基础检测清单。
func (s *WebsiteService) StandardRules() []model.WAFStandardRule {
	return []model.WAFStandardRule{{ID: "CRS-942100", Category: "SQL 注入", Description: "检测联合查询和布尔盲注特征", Locations: []string{"uri", "args", "body", "header", "cookie"}}, {ID: "CRS-941100", Category: "跨站脚本", Description: "检测脚本标签和 javascript 协议", Locations: []string{"uri", "args", "body", "header", "cookie"}}, {ID: "CRS-930110", Category: "本地文件包含", Description: "检测目录穿越和敏感文件读取", Locations: []string{"uri", "args", "body"}}}
}

// GetOpenResty、UpdateOpenResty 管理 OpenResty 状态和配置摘要。
func (s *WebsiteService) GetOpenResty() model.OpenRestyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.openresty
}
func (s *WebsiteService) UpdateOpenResty(cfg model.OpenRestyConfig) (model.OpenRestyConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(cfg.Version) == "" {
		cfg.Version = s.openresty.Version
	}
	if cfg.Modules == nil {
		cfg.Modules = s.openresty.Modules
	}
	cfg.UpdatedAt = time.Now().UTC()
	s.openresty = cfg
	return cfg, s.persist("openresty.json", cfg)
}

// OpenRestyFile 返回持久化的 nginx.conf 内容；首次使用时从受控配置文件读取。
func (s *WebsiteService) OpenRestyFile() (string, error) {
	s.mu.RLock()
	content := s.openresty.ConfigContent
	s.mu.RUnlock()
	if content != "" {
		return content, nil
	}
	path := s.openRestyConfigPath()
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("读取 OpenResty 配置失败: %w", err)
	}
	if len(b) > 4<<20 {
		return "", errors.New("OpenResty 配置超过 4 MiB 限制")
	}
	return string(b), nil
}

// UpdateOpenRestyFile 原子写入 nginx.conf，并在需要时保留可回滚备份。
func (s *WebsiteService) UpdateOpenRestyFile(content string, backup bool) error {
	if strings.TrimSpace(content) == "" || strings.IndexByte(content, 0) >= 0 || len(content) > 4<<20 {
		return errors.New("OpenResty 配置内容无效")
	}
	if !balancedConfig(content) {
		return errors.New("OpenResty 配置括号不匹配")
	}
	path := s.openRestyConfigPath()
	old, readErr := os.ReadFile(path)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("读取 OpenResty 原配置失败: %w", readErr)
	}
	if backup && len(old) > 0 {
		if err := os.WriteFile(path+".bak", old, 0o600); err != nil {
			return fmt.Errorf("保存 OpenResty 配置备份失败: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("创建 OpenResty 配置目录失败: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return fmt.Errorf("写入 OpenResty 临时配置失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("替换 OpenResty 配置失败: %w", err)
	}
	s.mu.Lock()
	s.openresty.ConfigContent = content
	s.openresty.UpdatedAt = time.Now().UTC()
	err := s.persist("openresty.json", s.openresty)
	s.mu.Unlock()
	if err != nil {
		// 数据库状态写入失败时恢复原配置，避免文件与控制面状态分叉。
		if len(old) == 0 {
			_ = os.Remove(path)
		} else {
			_ = os.WriteFile(path, old, 0o600)
		}
		return fmt.Errorf("保存 OpenResty 配置状态失败: %w", err)
	}
	return nil
}

// OpenRestyScope 读取指定作用域下的白名单指令，防止任意字段写入配置。
func (s *WebsiteService) OpenRestyScope(scope string) (map[string]string, error) {
	keys, ok := openRestyScopeKeys(strings.TrimSpace(scope))
	if !ok {
		return nil, errors.New("OpenResty 配置作用域无效")
	}
	content, err := s.OpenRestyFile()
	if err != nil {
		return nil, err
	}
	values := parseOpenRestyDirectives(content)
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, exists := values[key]; exists {
			result[key] = value
		}
	}
	return result, nil
}

// UpdateOpenRestyScope 更新作用域白名单指令并复用原子配置写入流程。
func (s *WebsiteService) UpdateOpenRestyScope(scope string, params map[string]string, backup bool) error {
	keys, ok := openRestyScopeKeys(strings.TrimSpace(scope))
	if !ok || len(params) == 0 || len(params) > 32 {
		return errors.New("OpenResty 作用域参数无效")
	}
	allowed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		allowed[key] = struct{}{}
	}
	for key, value := range params {
		if _, exists := allowed[key]; !exists || strings.TrimSpace(value) == "" || len(value) > 256 || strings.ContainsAny(value, "{};\x00") {
			return fmt.Errorf("OpenResty 指令 %q 不允许或值无效", key)
		}
	}
	content, err := s.OpenRestyFile()
	if err != nil {
		return err
	}
	lines := strings.Split(content, "\n")
	seen := make(map[string]bool, len(params))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		for key, value := range params {
			if strings.HasPrefix(trimmed, key+" ") || strings.HasPrefix(trimmed, key+"\t") {
				indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				lines[i] = indent + key + " " + value + ";"
				seen[key] = true
			}
		}
	}
	for key, value := range params {
		if !seen[key] {
			lines = append(lines, key+" "+value+";")
		}
	}
	return s.UpdateOpenRestyFile(strings.Join(lines, "\n"), backup)
}

// BuildOpenResty 执行只读配置检查并返回真实探测结果，避免伪造构建成功。
func (s *WebsiteService) BuildOpenResty(ctx context.Context, modules []string) (OpenRestyStatus, error) {
	status := s.ProbeOpenResty(ctx)
	if !status.Available {
		if status.Error == "" {
			status.Error = "OpenResty 不可用"
		}
		return status, fmt.Errorf("OpenResty 构建前检查失败: %s", status.Error)
	}
	if !status.ConfigValid {
		return status, errors.New("OpenResty 配置检查未通过")
	}
	if len(modules) > 100 {
		return status, errors.New("OpenResty 模块数量超出限制")
	}
	selected := make(map[string]struct{}, len(modules))
	for _, name := range modules {
		name = strings.TrimSpace(name)
		if name == "" || len(name) > 120 || strings.ContainsAny(name, " /\\") {
			return status, errors.New("OpenResty 模块名称无效")
		}
		selected[name] = struct{}{}
	}
	return status, nil
}

// OperateOpenResty 对宿主机或容器中的 OpenResty 执行受控信号操作。
func (s *WebsiteService) OperateOpenResty(ctx context.Context, operation string) (OpenRestyStatus, error) {
	operation = strings.ToLower(strings.TrimSpace(operation))
	if operation != "reload" && operation != "restart" && operation != "stop" && operation != "start" {
		return OpenRestyStatus{}, errors.New("OpenResty 操作无效")
	}
	status := s.ProbeOpenResty(ctx)
	if status.Binary == "" {
		return status, errors.New("OpenResty 未安装或不可用")
	}
	if strings.HasPrefix(status.Binary, "proc://") {
		return status, errors.New("OpenResty 位于独立命名空间，当前节点无法执行 reload；请配置 WORKMESH_OPENRESTY_BIN 或等效 reload 命令")
	}
	var cmd *exec.Cmd
	if strings.HasPrefix(status.Binary, "docker://") {
		name := strings.TrimPrefix(status.Binary, "docker://")
		if name == "" {
			return status, errors.New("OpenResty 容器标识无效")
		}
		switch operation {
		case "start":
			cmd = exec.CommandContext(ctx, "docker", "start", name)
		case "restart":
			cmd = exec.CommandContext(ctx, "docker", "restart", name)
		case "stop":
			cmd = exec.CommandContext(ctx, "docker", "stop", name)
		default:
			cmd = exec.CommandContext(ctx, "docker", "exec", name, "nginx", "-s", "reload")
		}
	} else {
		switch operation {
		case "start":
			cmd = exec.CommandContext(ctx, status.Binary)
		case "restart":
			// nginx 没有 restart signal，使用 stop 后重新启动，确保返回真实错误。
			stop := exec.CommandContext(ctx, status.Binary, "-s", "stop")
			if output, err := stop.CombinedOutput(); err != nil {
				return status, fmt.Errorf("OpenResty restart 停止阶段失败: %s: %w", strings.TrimSpace(string(output)), err)
			}
			cmd = exec.CommandContext(ctx, status.Binary)
		default:
			cmd = exec.CommandContext(ctx, status.Binary, "-s", operation)
		}
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return status, fmt.Errorf("OpenResty %s 失败: %s: %w", operation, strings.TrimSpace(string(output)), err)
	}
	return s.ProbeOpenResty(ctx), nil
}

func (s *WebsiteService) openRestyConfigPath() string {
	if configured := strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_CONFIG")); configured != "" {
		return filepath.Clean(configured)
	}
	return filepath.Join(s.root, "openresty.conf")
}

func openRestyScopeKeys(scope string) ([]string, bool) {
	switch scope {
	case "index":
		return []string{"index"}, true
	case "limit-conn":
		return []string{"limit_conn", "limit_rate", "limit_conn_zone"}, true
	case "ssl":
		return []string{"ssl_certificate", "ssl_certificate_key"}, true
	case "http-per":
		return []string{"server_names_hash_bucket_size", "client_header_buffer_size", "client_max_body_size", "keepalive_timeout", "gzip", "gzip_min_length", "gzip_comp_level"}, true
	default:
		return nil, false
	}
}

func parseOpenRestyDirectives(content string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(strings.TrimSuffix(line, ";"))
		if len(fields) >= 2 {
			result[fields[0]] = strings.Join(fields[1:], " ")
		}
	}
	return result
}

func balancedConfig(content string) bool {
	depth := 0
	for _, r := range content {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

// ProbeOpenResty 使用受限外部命令探测 OpenResty 安装及配置状态；命令均设置超时且不接受用户参数。
func (s *WebsiteService) ProbeOpenResty(ctx context.Context) OpenRestyStatus {
	cfg := s.GetOpenResty()
	status := OpenRestyStatus{Enabled: cfg.Enabled, DefaultHTTPS: cfg.DefaultHTTPS, Modules: append([]model.OpenRestyModule{}, cfg.Modules...)}
	bin := strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_BIN"))
	if bin == "" {
		for _, candidate := range []string{"openresty", "nginx"} {
			if found, err := lookupApplicationBinary(candidate, "openresty", false); err == nil {
				bin = found
				break
			}
		}
	}
	if bin == "" {
		if container, ok := probeOpenRestyContainer(ctx); ok {
			return OpenRestyStatus{
				Available: true, ConfigValid: true, Enabled: container.IsActive,
				Version: container.Version, Binary: container.Binary,
				DefaultHTTPS: cfg.DefaultHTTPS, Modules: append([]model.OpenRestyModule{}, cfg.Modules...),
			}
		}
		if process, ok := s.probeOpenRestyProcessNamespace(); ok {
			process.Enabled = cfg.Enabled
			process.DefaultHTTPS = cfg.DefaultHTTPS
			process.Modules = append([]model.OpenRestyModule{}, cfg.Modules...)
			return process
		}
		status.Error = "未找到 OpenResty 可执行文件或运行进程"
		return status
	}
	status.Binary = bin
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	versionOut, err := exec.CommandContext(probeCtx, bin, "-v").CombinedOutput()
	if err != nil {
		status.Error = strings.TrimSpace(string(versionOut))
		if status.Error == "" {
			status.Error = err.Error()
		}
		return status
	}
	status.Available = true
	status.Version = parseOpenRestyVersion(string(versionOut))
	if status.Version == "" {
		status.Version = cfg.Version
	}
	configCtx, cancelConfig := context.WithTimeout(ctx, 2*time.Second)
	defer cancelConfig()
	configOut, configErr := exec.CommandContext(configCtx, bin, "-t").CombinedOutput()
	status.ConfigValid = configErr == nil
	if configErr != nil && status.Error == "" {
		status.Error = strings.TrimSpace(string(configOut))
	}
	return status
}

// probeOpenRestyProcessNamespace 从 procfs 识别独立挂载命名空间中的 Nginx/OpenResty 主进程。
// 无法进入命名空间时只报告真实进程、cgroup、配置和监听信息，不虚构 -t 校验结果。
func (s *WebsiteService) probeOpenRestyProcessNamespace() (OpenRestyStatus, bool) {
	if runtime.GOOS == "windows" {
		return OpenRestyStatus{}, false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return OpenRestyStatus{}, false
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		base := filepath.Join("/proc", entry.Name())
		exe, _ := os.Readlink(filepath.Join(base, "exe"))
		cmdline, _ := os.ReadFile(filepath.Join(base, "cmdline"))
		identity := strings.ToLower(filepath.Base(exe) + " " + strings.ReplaceAll(string(cmdline), "\x00", " "))
		if !strings.Contains(identity, "openresty") && !strings.Contains(identity, "nginx") {
			continue
		}
		cgroup, _ := os.ReadFile(filepath.Join(base, "cgroup"))
		status := OpenRestyStatus{Available: true, Binary: "proc://" + entry.Name() + "/exe", ProcessID: pid, Cgroup: strings.TrimSpace(string(cgroup)), Listening: processListeningPorts(base)}
		for _, configured := range []string{s.openRestyConfigPath(), "/etc/nginx/nginx.conf", "/usr/local/openresty/nginx/conf/nginx.conf"} {
			path := filepath.Join(base, "root", strings.TrimPrefix(filepath.Clean(configured), string(filepath.Separator)))
			if content, readErr := os.ReadFile(path); readErr == nil {
				status.ConfigPath = configured
				if balancedConfig(string(content)) {
					status.Error = "检测到独立命名空间中的 OpenResty，当前服务无法进入该命名空间执行配置语法检查"
				} else {
					status.Error = "检测到独立命名空间中的 OpenResty，但读取到的配置括号不匹配"
				}
				return status, true
			}
		}
		status.Error = "检测到独立命名空间中的 OpenResty，但无权限读取其配置文件"
		return status, true
	}
	return OpenRestyStatus{}, false
}

func processListeningPorts(procRoot string) []int {
	ports := make([]int, 0, 2)
	seen := map[int]bool{}
	for _, name := range []string{"tcp", "tcp6"} {
		content, err := os.ReadFile(filepath.Join(procRoot, "net", name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(content), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 || fields[3] != "0A" {
				continue
			}
			parts := strings.Split(fields[1], ":")
			if len(parts) != 2 {
				continue
			}
			port, err := strconv.ParseInt(parts[1], 16, 32)
			if err == nil && port > 0 && !seen[int(port)] {
				seen[int(port)] = true
				ports = append(ports, int(port))
			}
		}
	}
	sort.Ints(ports)
	return ports
}

func parseOpenRestyVersion(output string) string {
	for _, token := range strings.Fields(output) {
		if strings.HasPrefix(token, "openresty/") {
			return strings.TrimPrefix(token, "openresty/")
		}
		if strings.HasPrefix(token, "nginx/") {
			return strings.TrimPrefix(token, "nginx/")
		}
	}
	return ""
}
