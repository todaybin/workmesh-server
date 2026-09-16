// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

// Create 创建网站并初始化对应 WAF 配置。
func (s *WebsiteService) Create(req model.WebsiteCreateRequest) (model.Website, error) {
	domain := strings.TrimSpace(req.PrimaryDomain)
	if domain == "" && len(req.Domains) > 0 {
		domain = strings.TrimSpace(req.Domains[0].Domain)
	}
	if domain == "" {
		domain = strings.TrimSpace(req.Name)
	}
	var domainErr error
	if domain, domainErr = normalizeWebsiteDomain(domain); domainErr != nil {
		return model.Website{}, fmt.Errorf("主域名格式无效: %w", domainErr)
	}
	if strings.ContainsRune(req.SiteDir, 0) || strings.Contains(filepath.Clean(req.SiteDir), "..") || len(req.SiteDir) > 4096 {
		return model.Website{}, errors.New("站点目录无效")
	}
	// 已能探测到 OpenResty 时，创建站点前必须通过真实配置语法检查；未安装时由预检接口返回明确状态。
	openRestyStatus := s.ProbeOpenResty(context.Background())
	if openRestyStatus.Available && !openRestyStatus.ConfigValid && !strings.Contains(openRestyStatus.Error, "无法进入") && !strings.Contains(openRestyStatus.Error, "无权限") {
		return model.Website{}, fmt.Errorf("OpenResty 配置语法检查失败: %s", openRestyStatus.Error)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, site := range s.websites {
		if strings.EqualFold(site.PrimaryDomain, domain) {
			return model.Website{}, errors.New("主域名已存在")
		}
	}
	// 主域名也必须避开其他站点的附加域名；否则 SQLite 可以写入两条
	// 不同站点的同名域名，最终生成冲突的 OpenResty server_name。
	for _, domains := range s.domains {
		for _, existing := range domains {
			if strings.EqualFold(strings.TrimSpace(existing.Domain), domain) {
				return model.Website{}, fmt.Errorf("网站域名已存在: %s", domain)
			}
		}
	}
	for _, rawDomain := range req.Domains {
		normalizedDomain, normalizeErr := normalizeWebsiteDomain(rawDomain.Domain)
		if normalizeErr != nil {
			return model.Website{}, fmt.Errorf("附加域名格式无效: %w", normalizeErr)
		}
		for websiteID := range s.domains {
			for _, existing := range s.domains[websiteID] {
				if strings.EqualFold(existing.Domain, normalizedDomain) {
					return model.Website{}, errors.New("网站域名已存在")
				}
			}
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
	typeName := strings.ToLower(strings.TrimSpace(req.Type))
	if typeName == "" {
		typeName = strings.ToLower(strings.TrimSpace(req.AppType))
	}
	if typeName == "" {
		typeName = "static"
	}
	if typeName == "php" {
		typeName = "runtime"
	}
	switch typeName {
	case "deployment", "runtime", "static", "proxy", "subsite", "stream":
	default:
		return model.Website{}, errors.New("网站类型无效")
	}
	if req.AppInstallID == 0 {
		req.AppInstallID = req.AppInstallIDCompat
	}
	if strings.TrimSpace(req.AppInstallRef) == "" {
		for _, nested := range []map[string]any{req.AppInstall, req.AppInstallLegacy} {
			for _, key := range []string{"id", "appInstallId", "appInstallID", "appId", "appID", "key", "appkey"} {
				if value, ok := nested[key]; ok {
					candidate := strings.TrimSpace(fmt.Sprint(value))
					if candidate != "" && candidate != "0" {
						req.AppInstallRef = candidate
						break
					}
				}
			}
			if req.AppInstallRef != "" {
				break
			}
		}
	}
	runtimeID := strings.TrimSpace(req.RuntimeID)
	if typeName != "runtime" {
		runtimeID = ""
	} else if runtimeID == "" {
		return model.Website{}, errors.New("运行时站点必须选择运行时")
	} else if repository, repositoryErr := s.sqliteRepository(); repositoryErr == nil {
		var exists int
		if err := repository.QueryRow(`SELECT 1 FROM runtime_records WHERE id=? LIMIT 1`, runtimeID).Scan(&exists); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return model.Website{}, errors.New("运行时不存在")
		} else if err == nil && exists != 1 {
			return model.Website{}, errors.New("运行时不存在")
		}
	}
	if typeName == "runtime" && strings.TrimSpace(req.Proxy) == "" {
		runtimeType, runtimePort := s.runtimeMetadata(runtimeID)
		if runtimePort > 0 {
			req.Proxy = fmt.Sprintf("127.0.0.1:%d", runtimePort)
		}
		if strings.EqualFold(runtimeType, "php") {
			req.ProxyType = "fpm"
		}
	}
	if typeName == "subsite" {
		if req.ParentWebsiteID == 0 {
			return model.Website{}, errors.New("子网站必须选择父网站")
		}
		parent, err := s.getWebsiteLocked(req.ParentWebsiteID)
		if err != nil {
			return model.Website{}, errors.New("父网站不存在")
		}
		// 1Panel stores the subsite run directory relative to the parent's
		// index/app root. Validate it before creating any child files.
		runDir, dirErr := validateWebsiteRelativeDir(req.SiteDir)
		if dirErr != nil {
			return model.Website{}, dirErr
		}
		parentRoot := s.SitePath(parent, "app")
		target := filepath.Join(parentRoot, filepath.FromSlash(runDir))
		if !withinPath(parentRoot, target) {
			return model.Website{}, errors.New("子网站运行目录越界")
		}
		if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
			return model.Website{}, errors.New("子网站运行目录不存在")
		}
	}
	if typeName == "stream" {
		ports, portErr := normalizeStreamPorts(req.StreamPorts)
		if portErr != nil {
			return model.Website{}, portErr
		}
		req.StreamPorts = strings.Join(ports, ",")
		if len(req.Servers) > 0 {
			servers, serverErr := normalizeStreamServers(req.Servers)
			if serverErr != nil {
				return model.Website{}, serverErr
			}
			req.Servers = make([]map[string]any, 0, len(servers))
			for _, server := range servers {
				req.Servers = append(req.Servers, map[string]any{
					"server": server.Server, "weight": server.Weight, "failTimeout": server.FailTimeout,
					"failTimeoutUnit": server.FailTimeoutUnit, "maxFails": server.MaxFails,
					"maxConns": server.MaxConns, "flag": server.Flag,
				})
			}
		}
	}
	if typeName == "proxy" && strings.TrimSpace(req.Proxy) == "" {
		return model.Website{}, errors.New("反向代理必须设置目标地址")
	}
	site := model.Website{ID: id, PrimaryDomain: domain, Alias: strings.TrimSpace(req.Alias), Type: typeName, Remark: strings.TrimSpace(req.Remark), SiteDir: strings.TrimSpace(req.SiteDir), Status: "running", Protocol: protocol, HttpConfig: strings.TrimSpace(req.HttpConfig), Proxy: strings.TrimSpace(req.Proxy), ProxyType: strings.TrimSpace(req.ProxyType), ErrorLog: errorLog, AccessLog: accessLog, DefaultServer: req.DefaultServer, IPV6: req.IPV6, Rewrite: strings.TrimSpace(req.Rewrite), WebsiteSSLID: sslID, RuntimeID: runtimeID, AppInstallID: req.AppInstallID, AppInstallRef: strings.TrimSpace(req.AppInstallRef), FtpID: req.FtpID, ParentWebsiteID: req.ParentWebsiteID, User: strings.TrimSpace(req.User), Group: strings.TrimSpace(req.Group), DbType: strings.TrimSpace(req.DbType), DbID: req.DbID, StreamPorts: strings.TrimSpace(req.StreamPorts), UDP: req.UDP, Algorithm: normalizeStreamAlgorithm(req.Algorithm), WebsiteGroupID: req.WebsiteGroupID, ExpireDate: time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC), CreatedAt: now, UpdatedAt: now}
	if typeName == "stream" && len(req.Servers) > 0 {
		site.Servers, _ = normalizeStreamServers(req.Servers)
	}
	if site.Alias == "" {
		site.Alias = domain
	}
	if strings.TrimSpace(req.SiteDir) != "" {
		// 新站点仍固定在域名目录，SiteDir 只接受兼容请求，不允许逃逸站点根。
		if typeName != "subsite" && (filepath.IsAbs(req.SiteDir) || strings.Contains(filepath.ToSlash(req.SiteDir), "..")) {
			return model.Website{}, errors.New("站点目录无效")
		}
	}
	site.SiteDir = s.siteDirPath(domain)
	if site.User == "" {
		site.User = "www"
	}
	if site.Group == "" {
		site.Group = "www"
	}
	if typeName == "runtime" && strings.EqualFold(site.ProxyType, "fpm") && strings.TrimSpace(req.User) == "" && strings.TrimSpace(req.Group) == "" {
		if runtimeType, container, fpmUser, fpmGroup := s.runtimeOwnerMetadata(site.RuntimeID); strings.EqualFold(runtimeType, "php") {
			if container {
				site.User, site.Group = "1000", "1000"
			} else {
				if fpmUser != "" {
					site.User = fpmUser
				}
				if fpmGroup != "" {
					site.Group = fpmGroup
				}
			}
		}
	}
	_, statErr := os.Stat(s.SitePath(site, "root"))
	siteDirExisted := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return model.Website{}, fmt.Errorf("检查站点目录失败: %w", statErr)
	}
	committed := false
	defer func() {
		if !committed && !siteDirExisted {
			_ = os.RemoveAll(s.SitePath(site, "root"))
		}
	}()
	if err := s.ensureSiteLayout(site); err != nil {
		return model.Website{}, fmt.Errorf("创建站点目录失败: %w", err)
	}
	if err := s.writeInitialSiteConfigWithRoot(site, req.SiteDir); err != nil {
		return model.Website{}, fmt.Errorf("生成站点配置失败: %w", err)
	}
	if typeName == "stream" && len(site.Servers) > 0 {
		ports, portErr := normalizeStreamPorts(site.StreamPorts)
		if portErr != nil {
			return model.Website{}, portErr
		}
		content := renderStreamConfig(site.ID, ports, site.UDP, site.Algorithm, site.Servers)
		if err := writeWebsiteAtomic(s.SitePath(site, "stream.conf"), []byte(content), 0o640); err != nil {
			return model.Website{}, fmt.Errorf("生成 TCP/UDP 配置失败: %w", err)
		}
	}
	if err := s.applyWebsiteDefaults(site); err != nil {
		return model.Website{}, fmt.Errorf("设置站点默认权限失败: %w", err)
	}
	if err := validateCreatedWebsiteConfig(context.Background(), openRestyStatus); err != nil {
		return model.Website{}, err
	}
	previousState, snapshotErr := captureWebsiteState(s)
	if snapshotErr != nil {
		return model.Website{}, fmt.Errorf("保存网站状态快照失败: %w", snapshotErr)
	}
	s.websites = append(s.websites, site)
	domains := append([]model.WebsiteDomain(nil), req.Domains...)
	primaryFound := false
	seenDomains := map[string]bool{}
	for i := range domains {
		normalizedDomain, normalizeErr := normalizeWebsiteDomain(domains[i].Domain)
		if normalizeErr != nil {
			return model.Website{}, fmt.Errorf("附加域名格式无效: %w", normalizeErr)
		}
		domains[i].Domain = normalizedDomain
		if seenDomains[strings.ToLower(normalizedDomain)] {
			return model.Website{}, errors.New("网站域名重复")
		}
		seenDomains[strings.ToLower(normalizedDomain)] = true
		domains[i].WebsiteID = id
		if domains[i].Port == 0 {
			if protocol == "HTTPS" || domains[i].SSL {
				domains[i].Port, domains[i].SSL = 443, true
			} else {
				domains[i].Port = 80
			}
		}
		if strings.EqualFold(domains[i].Domain, domain) {
			primaryFound = true
		}
	}
	// 原版创建网站会同时写入主域名记录，域名设置页直接读取该表。
	if !strings.EqualFold(site.Type, "stream") && !primaryFound {
		primary := model.WebsiteDomain{WebsiteID: id, Domain: domain, Port: 80}
		if protocol == "HTTPS" {
			primary.Port, primary.SSL = 443, true
		}
		domains = append([]model.WebsiteDomain{primary}, domains...)
	}
	s.domains[id] = domains
	if _, syncErr := s.syncWebsiteServerNamesLocked(site); syncErr != nil {
		return model.Website{}, rollbackCreatedWebsite(s, id, previousState, fmt.Errorf("同步网站域名配置失败: %w", syncErr))
	}
	s.wafSites[id] = model.WAFSite{WebsiteID: id, Alias: site.Alias, Enabled: true, Mode: "observe", DetectionLevel: 1, FrequencyEnabled: true, Rules: []model.WAFRule{}}
	if typeName == "subsite" && strings.TrimSpace(req.SiteDir) != "" {
		if s.configs[id] == nil {
			s.configs[id] = map[string]any{}
		}
		if runDir, dirErr := validateWebsiteRelativeDir(req.SiteDir); dirErr == nil {
			s.configs[id]["dir"] = map[string]any{"dir": runDir}
		}
	}
	if err := s.persist("websites", s.websites); err != nil {
		return model.Website{}, rollbackCreatedWebsite(s, id, previousState, err)
	}
	if typeName == "stream" && len(site.Servers) > 0 {
		if s.configs[id] == nil {
			s.configs[id] = map[string]any{}
		}
		streamConfig := map[string]any{"streamPorts": site.StreamPorts, "udp": site.UDP, "algorithm": site.Algorithm, "servers": site.Servers}
		s.configs[id]["stream"] = streamConfig
		if repository, repositoryErr := s.sqliteRepository(); repositoryErr == nil {
			if err := repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
				return persistStreamConfig(tx, id, streamConfig, now)
			}); err != nil {
				return model.Website{}, rollbackCreatedWebsite(s, id, previousState, err)
			}
		} else if s.db != nil {
			return model.Website{}, rollbackCreatedWebsite(s, id, previousState, repositoryErr)
		}
	}
	if err := s.persistWAFSites(); err != nil {
		return model.Website{}, rollbackCreatedWebsite(s, id, previousState, err)
	}
	if err := s.persist("website-domains", s.domains); err != nil {
		return model.Website{}, rollbackCreatedWebsite(s, id, previousState, err)
	}
	if typeName == "subsite" && strings.TrimSpace(req.SiteDir) != "" {
		if err := s.persist("website-configs", s.configs); err != nil {
			return model.Website{}, rollbackCreatedWebsite(s, id, previousState, err)
		}
	}
	site.Domains = append([]model.WebsiteDomain(nil), s.domains[id]...)
	committed = true
	return s.decorateWebsite(site), nil
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
		previousState, snapshotErr := captureWebsiteState(s)
		if snapshotErr != nil {
			return model.Website{}, fmt.Errorf("保存网站更新回滚快照失败: %w", snapshotErr)
		}
		wafBackups, wafSnapshotErr := s.snapshotWAFRuntime()
		if wafSnapshotErr != nil {
			return model.Website{}, fmt.Errorf("保存 WAF 更新回滚快照失败: %w", wafSnapshotErr)
		}
		oldSite := s.websites[i]
		oldDomain := oldSite.PrimaryDomain
		updated := oldSite
		candidateDomain := oldDomain
		domainChanged := false
		if strings.TrimSpace(req.PrimaryDomain) != "" {
			candidate := strings.TrimSpace(req.PrimaryDomain)
			normalizedCandidate, normalizeErr := normalizeWebsiteDomain(candidate)
			if normalizeErr != nil {
				return model.Website{}, fmt.Errorf("主域名格式无效: %w", normalizeErr)
			}
			candidate = normalizedCandidate
			for j, other := range s.websites {
				if j != i && strings.EqualFold(other.PrimaryDomain, candidate) {
					return model.Website{}, errors.New("主域名已存在")
				}
			}
			for websiteID, domains := range s.domains {
				for _, existing := range domains {
					if websiteID != req.ID || !strings.EqualFold(existing.Domain, oldDomain) {
						if strings.EqualFold(existing.Domain, candidate) {
							return model.Website{}, errors.New("网站域名已存在")
						}
					}
				}
			}
			candidateDomain = candidate
			domainChanged = !strings.EqualFold(oldDomain, candidate)
		}
		if strings.TrimSpace(req.Alias) != "" {
			updated.Alias = strings.TrimSpace(req.Alias)
		}
		if req.Remark != "" {
			updated.Remark = strings.TrimSpace(req.Remark)
		}
		if req.SiteDir != "" {
			if filepath.IsAbs(req.SiteDir) || strings.Contains(filepath.ToSlash(req.SiteDir), "..") {
				return model.Website{}, errors.New("站点目录无效")
			}
			// siteDir 是兼容字段，真实网站根始终由域名目录规则决定。
			updated.SiteDir = s.siteDirPath(candidateDomain)
		}
		if req.Type != "" {
			typeName := strings.ToLower(strings.TrimSpace(req.Type))
			updated.Type = typeName
			if typeName != "runtime" {
				updated.RuntimeID = ""
			}
		}
		updated.Favorite = req.Favorite
		if req.WebsiteGroupID != 0 {
			updated.WebsiteGroupID = req.WebsiteGroupID
		}
		if req.ExpireDate != nil {
			updated.ExpireDate = req.ExpireDate.UTC()
		}
		if req.IPV6 != nil {
			updated.IPV6 = *req.IPV6
		}
		if req.WebsiteSSLID != nil {
			updated.WebsiteSSLID = *req.WebsiteSSLID
		}
		if req.Protocol != "" {
			p := strings.ToUpper(strings.TrimSpace(req.Protocol))
			if p != "HTTP" && p != "HTTPS" {
				return model.Website{}, errors.New("协议类型无效")
			}
			updated.Protocol = p
		}
		if req.HttpConfig != "" {
			updated.HttpConfig = strings.TrimSpace(req.HttpConfig)
		}
		if req.Proxy != "" {
			updated.Proxy = strings.TrimSpace(req.Proxy)
		}
		if req.ProxyType != "" {
			updated.ProxyType = strings.TrimSpace(req.ProxyType)
		}
		if req.ErrorLog != nil {
			updated.ErrorLog = *req.ErrorLog
		}
		if req.AccessLog != nil {
			updated.AccessLog = *req.AccessLog
		}
		if req.DefaultServer != nil {
			updated.DefaultServer = *req.DefaultServer
		}
		if req.Rewrite != "" {
			updated.Rewrite = strings.TrimSpace(req.Rewrite)
		}
		if req.RuntimeID != nil {
			if strings.EqualFold(updated.Type, "runtime") {
				updated.RuntimeID = strings.TrimSpace(*req.RuntimeID)
			} else {
				updated.RuntimeID = ""
			}
		}
		if req.AppInstallID != nil {
			updated.AppInstallID = *req.AppInstallID
		}
		if req.AppInstallRef != nil {
			updated.AppInstallRef = strings.TrimSpace(*req.AppInstallRef)
		}
		if req.FtpID != nil {
			updated.FtpID = *req.FtpID
		}
		if req.ParentWebsiteID != nil {
			updated.ParentWebsiteID = *req.ParentWebsiteID
		}
		if req.User != "" {
			updated.User = strings.TrimSpace(req.User)
		}
		if req.Group != "" {
			updated.Group = strings.TrimSpace(req.Group)
		}
		if req.DbType != "" {
			updated.DbType = strings.TrimSpace(req.DbType)
		}
		if req.DbID != nil {
			updated.DbID = *req.DbID
		}
		if req.StreamPorts != "" {
			updated.StreamPorts = strings.TrimSpace(req.StreamPorts)
		}
		if req.UDP != nil {
			updated.UDP = *req.UDP
		}

		updatedDomains := append([]model.WebsiteDomain(nil), s.domains[req.ID]...)
		if domainChanged {
			updated.PrimaryDomain = candidateDomain
		}
		updated.UpdatedAt = time.Now().UTC()

		fsState := websiteUpdateFilesystemState{}
		if domainChanged {
			// Move the real site directory only after request validation has
			// completed. This prevents an invalid later field from leaving the
			// filesystem and SQLite metadata split-brained.
			oldPath := strings.TrimSpace(oldSite.SiteDir)
			if oldPath == "" {
				oldPath = s.siteDirPath(oldDomain)
			}
			newPath := s.siteDirPath(updated.PrimaryDomain)
			fsState.newPath = newPath
			if filepath.Clean(oldPath) != filepath.Clean(newPath) {
				if _, statErr := os.Stat(newPath); statErr == nil {
					return model.Website{}, errors.New("目标站点目录已存在")
				} else if !errors.Is(statErr, os.ErrNotExist) {
					return model.Website{}, statErr
				}
				if _, statErr := os.Stat(oldPath); statErr == nil {
					if err := os.MkdirAll(filepath.Dir(newPath), 0o750); err != nil {
						return model.Website{}, err
					}
					if err := os.Rename(oldPath, newPath); err != nil {
						return model.Website{}, err
					}
					fsState.moved = true
				} else if !errors.Is(statErr, os.ErrNotExist) {
					return model.Website{}, statErr
				}
			}
			updated.SiteDir = newPath
			if _, statErr := os.Stat(newPath); errors.Is(statErr, os.ErrNotExist) {
				fsState.createdNewPath = true
				if err := s.ensureSiteLayout(updated); err != nil {
					return model.Website{}, rollbackWebsiteUpdate(s, previousState, wafBackups, oldSite, updated, fsState, false, err)
				}
			} else if statErr != nil {
				return model.Website{}, rollbackWebsiteUpdate(s, previousState, wafBackups, oldSite, updated, fsState, false, statErr)
			}
			if err := s.rewriteRenamedSiteConfig(oldSite, updated, oldPath, newPath); err != nil {
				return model.Website{}, rollbackWebsiteUpdate(s, previousState, wafBackups, oldSite, updated, fsState, false, err)
			}
			// The primary domain is also a real domain row. Keep its stable ID
			// and update only rows that represented the previous primary name.
			for index := range updatedDomains {
				if strings.EqualFold(updatedDomains[index].Domain, oldDomain) {
					updatedDomains[index].Domain = updated.PrimaryDomain
				}
			}
		}
		if site, ok := s.wafSites[req.ID]; ok {
			site.Alias = updated.Alias
			s.wafSites[req.ID] = site
		}
		s.websites[i] = updated
		s.domains[req.ID] = updatedDomains
		if err := s.persist("websites", s.websites); err != nil {
			return model.Website{}, rollbackWebsiteUpdate(s, previousState, wafBackups, oldSite, updated, fsState, false, err)
		}
		if domainChanged {
			if err := s.persist("website-domains", s.domains); err != nil {
				return model.Website{}, rollbackWebsiteUpdate(s, previousState, wafBackups, oldSite, updated, fsState, false, err)
			}
		}
		wafPersistStarted := true
		if err := s.persistWAFSites(); err != nil {
			return model.Website{}, rollbackWebsiteUpdate(s, previousState, wafBackups, oldSite, updated, fsState, wafPersistStarted, err)
		}
		s.websites[i].Domains = append([]model.WebsiteDomain(nil), updatedDomains...)
		return s.decorateWebsite(s.websites[i]), nil
	}
	return model.Website{}, os.ErrNotExist
}

type websiteUpdateFilesystemState struct {
	newPath        string
	moved          bool
	createdNewPath bool
}

// rollbackWebsiteUpdate restores all state changed by Update. Filesystem
// compensation runs before the in-memory snapshot is restored because it
// needs the pre-update and post-update site paths.
func rollbackWebsiteUpdate(s *WebsiteService, snapshot websiteStateSnapshot, wafBackups []wafFileBackup, oldSite, updated model.Website, fsState websiteUpdateFilesystemState, reloadWAF bool, cause error) error {
	var rollbackErr error
	if fsState.moved {
		if err := s.restoreRenamedSite(oldSite, updated); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	} else if fsState.createdNewPath && strings.TrimSpace(fsState.newPath) != "" {
		if err := os.RemoveAll(fsState.newPath); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}

	s.websites = snapshot.Websites
	s.wafSites = snapshot.WAFSites
	s.domains = snapshot.Domains
	s.configs = snapshot.Configs

	if err := s.persist("websites", snapshot.Websites); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if err := s.persist("website-domains", snapshot.Domains); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if err := s.persist("website-configs", snapshot.Configs); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if err := restoreWAFRuntime(wafBackups); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if reloadWAF && s.wafRuntimeRequired() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		if err := s.reloadWAFRuntime(ctx); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
		cancel()
	}
	return errors.Join(cause, rollbackErr)
}

type websiteStateSnapshot struct {
	Websites []model.Website                `json:"websites"`
	WAFSites map[uint]model.WAFSite         `json:"wafSites"`
	Domains  map[uint][]model.WebsiteDomain `json:"domains"`
	Configs  map[uint]map[string]any        `json:"configs"`
}

func captureWebsiteState(s *WebsiteService) (websiteStateSnapshot, error) {
	payload, err := json.Marshal(websiteStateSnapshot{
		Websites: s.websites,
		WAFSites: s.wafSites,
		Domains:  s.domains,
		Configs:  s.configs,
	})
	if err != nil {
		return websiteStateSnapshot{}, err
	}
	var snapshot websiteStateSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return websiteStateSnapshot{}, err
	}
	if snapshot.WAFSites == nil {
		snapshot.WAFSites = map[uint]model.WAFSite{}
	}
	if snapshot.Domains == nil {
		snapshot.Domains = map[uint][]model.WebsiteDomain{}
	}
	if snapshot.Configs == nil {
		snapshot.Configs = map[uint]map[string]any{}
	}
	return snapshot, nil
}

// rollbackCreatedWebsite restores all in-memory and relational state after a
// post-layout creation failure. The original error remains the primary cause;
// compensation errors are joined for operator visibility.
func rollbackCreatedWebsite(s *WebsiteService, id uint, snapshot websiteStateSnapshot, cause error) error {
	s.websites = snapshot.Websites
	s.wafSites = snapshot.WAFSites
	s.domains = snapshot.Domains
	s.configs = snapshot.Configs
	var rollbackErr error
	if repository, err := s.sqliteRepository(); err == nil {
		if _, err := repository.Exec(`DELETE FROM websites WHERE id=?`, id); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	} else if s.db != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if err := s.persist("websites", snapshot.Websites); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if err := s.persist("website-domains", snapshot.Domains); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if err := s.persist("website-configs", snapshot.Configs); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if err := s.persistWAFSites(); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	return errors.Join(cause, rollbackErr)
}
