// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

func (s *WebsiteService) load() {
	repository, repositoryErr := s.sqliteRepository()
	// 正式数据从关系表加载；只有旧数据库没有任何关系记录时才读取旧 blob 迁移数据。
	if repositoryErr == nil {
		if rows, err := repository.Query(`SELECT id,protocol,primary_domain,type,alias,remark,status,http_config,expire_date,proxy,proxy_type,site_dir,error_log,access_log,default_server,ipv6,rewrite,website_group_id,website_ssl_id,runtime_id,app_install_id,app_install_ref,ftp_id,parent_website_id,user,"group",db_type,db_id,favorite,stream_ports,udp,created_at,updated_at FROM websites ORDER BY id`); err == nil {
			for rows.Next() {
				var item model.Website
				var id, groupID, sslID, appID, ftpID, parentID, dbID int64
				var runtimeID, appInstallRef string
				var errLog, accessLog, defaultServer, ipv6, favorite, udp int
				var expire, created, updated string
				if rows.Scan(&id, &item.Protocol, &item.PrimaryDomain, &item.Type, &item.Alias, &item.Remark, &item.Status, &item.HttpConfig, &expire, &item.Proxy, &item.ProxyType, &item.SiteDir, &errLog, &accessLog, &defaultServer, &ipv6, &item.Rewrite, &groupID, &sslID, &runtimeID, &appID, &appInstallRef, &ftpID, &parentID, &item.User, &item.Group, &item.DbType, &dbID, &favorite, &item.StreamPorts, &udp, &created, &updated) != nil {
					continue
				}
				item.ID = uint(id)
				item.WebsiteGroupID = uint(groupID)
				item.WebsiteSSLID = uint(sslID)
				item.RuntimeID = strings.TrimSpace(runtimeID)
				item.AppInstallID = uint(appID)
				item.AppInstallRef = strings.TrimSpace(appInstallRef)
				item.FtpID = uint(ftpID)
				item.ParentWebsiteID = uint(parentID)
				item.DbID = uint(dbID)
				item.ErrorLog = errLog != 0
				item.AccessLog = accessLog != 0
				item.DefaultServer = defaultServer != 0
				item.IPV6 = ipv6 != 0
				item.Favorite = favorite != 0
				item.UDP = udp != 0
				item.ExpireDate, _ = time.Parse(time.RFC3339Nano, expire)
				item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
				item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
				s.websites = append(s.websites, item)
			}
			rows.Close()
		}
	}
	// WAF 运行态始终来自 WorkMesh-owned JSON files. SQLite is not consulted.
	for _, site := range s.websites {
		cfg := model.WAFSite{WebsiteID: site.ID, Alias: site.Alias, Enabled: true, Mode: "observe", DetectionLevel: 1, FrequencyEnabled: true, Rules: []model.WAFRule{}}
		base := s.SitePath(site, "waf")
		var disk struct {
			WebsiteID        uint                               `json:"websiteID"`
			Alias            string                             `json:"alias"`
			Enabled          *bool                              `json:"enabled"`
			Mode             string                             `json:"mode"`
			DetectionLevel   *int                               `json:"detectionLevel"`
			FrequencyEnabled *bool                              `json:"frequencyEnabled"`
			RateLimits       map[string]model.WAFFrequencyLimit `json:"rateLimits"`
			Rules            []model.WAFRule                    `json:"rules"`
		}
		if err := readJSON(filepath.Join(base, "config.json"), &disk); err == nil {
			if disk.WebsiteID == 0 || disk.WebsiteID == site.ID {
				cfg.WebsiteID = site.ID
			}
			if disk.Alias != "" {
				cfg.Alias = disk.Alias
			}
			if disk.Enabled != nil {
				cfg.Enabled = *disk.Enabled
			}
			if disk.Mode == "observe" || disk.Mode == "block" {
				cfg.Mode = disk.Mode
			}
			if disk.DetectionLevel != nil && *disk.DetectionLevel >= 1 && *disk.DetectionLevel <= 4 {
				cfg.DetectionLevel = *disk.DetectionLevel
			}
			if disk.FrequencyEnabled != nil {
				cfg.FrequencyEnabled = *disk.FrequencyEnabled
			} else {
				cfg.FrequencyEnabled = true
			}
			cfg.RateLimits = disk.RateLimits
		}
		_ = readJSON(filepath.Join(base, "rules.json"), &cfg.Rules)
		if len(cfg.Rules) == 0 && len(disk.Rules) > 0 {
			cfg.Rules = disk.Rules
		}
		if cfg.Rules == nil {
			cfg.Rules = []model.WAFRule{}
		}
		s.wafSites[site.ID] = cfg
	}
	s.global = model.WAFGlobalConfig{Enabled: true, StandardRules: true, Mode: "observe", ParanoiaLevel: 1, InboundThreshold: 5, RequestBodyLimit: 1 << 20}
	s.lists = model.WAFAccessLists{Whitelist: []string{}, Blacklist: []string{}}
	_ = readJSON(filepath.Join(s.wafRoot(), "global.json"), &s.global)
	_ = readJSON(filepath.Join(s.wafRoot(), "access-lists.json"), &s.lists)
	if s.lists.Enabled == nil {
		s.lists.Enabled = map[string]bool{
			"blacklist": true, "whitelist": true,
			"urlBlacklist": true, "urlWhitelist": true,
			"uaBlacklist": true, "uaWhitelist": true, "ipGroups": true,
		}
	}
	_ = readJSON(filepath.Join(s.wafRoot(), "default-rules.json"), &s.wafDefaultRules)
	_ = readJSON(filepath.Join(s.wafRoot(), "custom-rules.json"), &s.wafCustomRules)
	if s.wafDefaultRules == nil {
		s.wafDefaultRules = []model.WAFRule{}
	}
	if s.wafCustomRules == nil {
		s.wafCustomRules = []model.WAFRule{}
	}
	// The production OpenResty WAF directory is bind-mounted over the image's
	// generated defaults. Recreate the switch file during startup so an older
	// host directory cannot silently omit CRS after an image upgrade.
	_ = s.writeWAFGeneratedConfig()
	s.openresty = model.OpenRestyConfig{Version: "1.27.1", Enabled: true, Modules: []model.OpenRestyModule{}}
	if repositoryErr == nil {
		var enabled, defaultHTTPS, reject int
		var updated string
		if repository.QueryRow(`SELECT version,enabled,default_https,ssl_reject_handshake,config_content,updated_at FROM website_openresty_config WHERE id=1`).Scan(&s.openresty.Version, &enabled, &defaultHTTPS, &reject, &s.openresty.ConfigContent, &updated) == nil {
			s.openresty.Enabled = enabled != 0
			s.openresty.DefaultHTTPS = defaultHTTPS != 0
			s.openresty.SSLRejectHandshake = reject != 0
			s.openresty.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		}
		rows, _ := repository.Query(`SELECT name,enabled,build_mode,load_order,custom,script,packages,params,provider,build_status,load_status,last_error FROM website_openresty_modules ORDER BY load_order,name`)
		if rows != nil {
			for rows.Next() {
				var m model.OpenRestyModule
				var enabled, custom int
				if rows.Scan(&m.Name, &enabled, &m.BuildMode, &m.LoadOrder, &custom, &m.Script, &m.Packages, &m.Params, &m.Provider, &m.BuildStatus, &m.LoadStatus, &m.LastError) == nil {
					m.Enabled = enabled != 0
					m.Custom = custom != 0
					s.openresty.Modules = append(s.openresty.Modules, m)
				}
			}
			rows.Close()
		}
	}
	if repositoryErr == nil {
		if rows, err := repository.Query(`SELECT id,website_id,domain,port,ssl FROM website_domains ORDER BY id`); err == nil {
			for rows.Next() {
				var idRaw string
				var websiteID, port int64
				var domain string
				var ssl int
				if rows.Scan(&idRaw, &websiteID, &domain, &port, &ssl) == nil {
					if _, err := strconv.ParseInt(idRaw, 10, 64); err != nil && idRaw == "" {
						idRaw = strconv.FormatInt(time.Now().UnixNano(), 10)
					}
					s.domains[uint(websiteID)] = append(s.domains[uint(websiteID)], model.WebsiteDomain{ID: idRaw, WebsiteID: uint(websiteID), Domain: domain, Port: int(port), SSL: ssl != 0})
				}
			}
			rows.Close()
		}
	}
	// 域名和配置只从当前关系表读取。
	if s.domains == nil {
		s.domains = map[uint][]model.WebsiteDomain{}
	}
	domainsBackfilled := false
	for _, site := range s.websites {
		if strings.EqualFold(site.Type, "stream") || strings.TrimSpace(site.PrimaryDomain) == "" {
			continue
		}
		found := false
		for _, item := range s.domains[site.ID] {
			if strings.EqualFold(strings.TrimSpace(item.Domain), strings.TrimSpace(site.PrimaryDomain)) {
				found = true
				break
			}
		}
		if !found {
			primary := model.WebsiteDomain{WebsiteID: site.ID, Domain: strings.TrimSpace(site.PrimaryDomain), Port: 80}
			if strings.EqualFold(site.Protocol, "HTTPS") {
				primary.Port, primary.SSL = 443, true
			}
			s.domains[site.ID] = append([]model.WebsiteDomain{primary}, s.domains[site.ID]...)
			domainsBackfilled = true
		}
	}
	if domainsBackfilled {
		_ = s.persist("website-domains", s.domains)
	}
	if s.configs == nil {
		s.configs = map[uint]map[string]any{}
	}
	if repositoryErr == nil {
		if rows, err := repository.Query(`SELECT website_id,config_type,content FROM website_settings`); err == nil {
			for rows.Next() {
				var id int64
				var typ, content string
				if rows.Scan(&id, &typ, &content) == nil {
					var value map[string]any
					if json.Unmarshal([]byte(content), &value) == nil {
						if s.configs[uint(id)] == nil {
							s.configs[uint(id)] = map[string]any{}
						}
						s.configs[uint(id)][typ] = value
					}
				}
			}
			rows.Close()
		}
	}
	if s.websites == nil {
		s.websites = []model.Website{}
	}
	// 旧版本反向站点可能只有 site.conf 中的 proxy_pass；先补齐菜单所需的 root.conf。
	s.reconcileReverseProxyFiles()
	// SQLite is authoritative after restart, but runtime files can be removed
	// by a package upgrade or an operator cleanup. Rebuild only missing/empty
	// files for running sites from the persisted typed settings; stopped sites
	// remain disabled and are never silently re-enabled.
	s.reconcileLoadedWebsiteFiles()
}
