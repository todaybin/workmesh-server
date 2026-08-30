// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

var domainPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)

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
}

// NewWebsiteService 创建服务并从数据目录加载已有状态。
func NewWebsiteService(root string) *WebsiteService {
	if strings.TrimSpace(root) == "" {
		root = os.Getenv("WORKMESH_DATA_DIR")
	}
	if strings.TrimSpace(root) == "" {
		root = "./data"
	}
	s := &WebsiteService{root: root, wafSites: map[uint]model.WAFSite{}, domains: map[uint][]model.WebsiteDomain{}, configs: map[uint]map[string]any{}}
	s.load()
	return s
}

func (s *WebsiteService) load() {
	_ = os.MkdirAll(s.root, 0o750)
	read := func(name string, target any) bool {
		data, err := os.ReadFile(filepath.Join(s.root, name))
		if err != nil || len(data) == 0 {
			return false
		}
		return json.Unmarshal(data, target) == nil
	}
	_ = read("websites.json", &s.websites)
	var sites []model.WAFSite
	if read("waf-sites.json", &sites) {
		for _, site := range sites {
			if site.Rules == nil {
				site.Rules = []model.WAFRule{}
			}
			s.wafSites[site.WebsiteID] = site
		}
	}
	if !read("waf-global.json", &s.global) {
		s.global = model.WAFGlobalConfig{Enabled: true, StandardRules: true, Mode: "observe", ParanoiaLevel: 1, InboundThreshold: 5, RequestBodyLimit: 1 << 20}
	}
	if !read("waf-access-lists.json", &s.lists) {
		s.lists = model.WAFAccessLists{Whitelist: []string{}, Blacklist: []string{}}
	}
	if !read("openresty.json", &s.openresty) {
		s.openresty = model.OpenRestyConfig{Version: "1.27.1", Enabled: true, Modules: []model.OpenRestyModule{}}
	}
	_ = read("website-domains.json", &s.domains)
	_ = read("website-configs.json", &s.configs)
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
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.root, 0o750); err != nil {
		return err
	}
	tmp := filepath.Join(s.root, name+".tmp")
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(s.root, name)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
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
		result = append(result, site)
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

// Get 根据 ID 查询网站。
func (s *WebsiteService) Get(id uint) (model.Website, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, site := range s.websites {
		if site.ID == id {
			return site, nil
		}
	}
	return model.Website{}, os.ErrNotExist
}

// Create 创建网站并初始化对应 WAF 配置。
func (s *WebsiteService) Create(req model.WebsiteCreateRequest) (model.Website, error) {
	domain := strings.TrimSpace(req.PrimaryDomain)
	if !domainPattern.MatchString(domain) || strings.Contains(domain, "..") {
		return model.Website{}, errors.New("主域名格式无效")
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
	site := model.Website{ID: id, PrimaryDomain: domain, Alias: strings.TrimSpace(req.Alias), Type: strings.TrimSpace(req.Type), Remark: strings.TrimSpace(req.Remark), SiteDir: strings.TrimSpace(req.SiteDir), Status: "running", CreatedAt: now, UpdatedAt: now}
	if site.Alias == "" {
		site.Alias = domain
	}
	if site.Type == "" {
		site.Type = "static"
	}
	s.websites = append(s.websites, site)
	s.wafSites[id] = model.WAFSite{WebsiteID: id, Alias: site.Alias, Enabled: true, Mode: "observe", Rules: []model.WAFRule{}}
	if err := s.persist("websites.json", s.websites); err != nil {
		return model.Website{}, err
	}
	if err := s.persistWAFSites(); err != nil {
		return model.Website{}, err
	}
	return site, nil
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
			if !domainPattern.MatchString(strings.TrimSpace(req.PrimaryDomain)) {
				return model.Website{}, errors.New("主域名格式无效")
			}
			s.websites[i].PrimaryDomain = strings.TrimSpace(req.PrimaryDomain)
		}
		if strings.TrimSpace(req.Alias) != "" {
			s.websites[i].Alias = strings.TrimSpace(req.Alias)
		}
		s.websites[i].Remark, s.websites[i].SiteDir, s.websites[i].Favorite = strings.TrimSpace(req.Remark), strings.TrimSpace(req.SiteDir), req.Favorite
		s.websites[i].UpdatedAt = time.Now().UTC()
		if site, ok := s.wafSites[req.ID]; ok {
			site.Alias = s.websites[i].Alias
			s.wafSites[req.ID] = site
		}
		if err := s.persist("websites.json", s.websites); err != nil {
			return model.Website{}, err
		}
		_ = s.persistWAFSites()
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
	for i := range items {
		if items[i].ID == domain.ID {
			items[i] = domain
			s.domains[domain.WebsiteID] = items
			return domain, s.persist("website-domains.json", s.domains)
		}
		if strings.EqualFold(items[i].Domain, domain.Domain) && items[i].ID != domain.ID {
			return model.WebsiteDomain{}, errors.New("网站域名已存在")
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
