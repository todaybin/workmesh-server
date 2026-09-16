// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"errors"
	"os"
	"regexp"
	"strings"

	"github.com/todaybin/workmesh-server/node/model"
)

var websiteServerNamePattern = regexp.MustCompile(`(?m)^(\s*)server_name\s+[^;]+;\s*$`)

// syncWebsiteServerNamesLocked 将域名表同步到 HTTP 站点配置，保证新增或删除
// 域名后 OpenResty 实际 server_name 与 SQLite 记录保持一致。
func (s *WebsiteService) syncWebsiteServerNamesLocked(site model.Website) ([]byte, error) {
	if strings.EqualFold(strings.TrimSpace(site.Type), "stream") {
		return nil, nil
	}
	path := s.SitePath(site, "site.conf")
	old, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	names := []string{site.PrimaryDomain}
	seen := map[string]bool{strings.ToLower(site.PrimaryDomain): true}
	for _, item := range s.domains[site.ID] {
		domain := strings.TrimSpace(item.Domain)
		key := strings.ToLower(domain)
		if domain != "" && !seen[key] {
			names = append(names, domain)
			seen[key] = true
		}
	}
	updated := websiteServerNamePattern.ReplaceAllString(string(old), `${1}server_name `+strings.Join(names, " ")+`;`)
	if updated == string(old) && !websiteServerNamePattern.Match(old) {
		return nil, errors.New("网站配置缺少 server_name 指令")
	}
	if err := writeWebsiteAtomic(path, []byte(updated), 0o640); err != nil {
		return nil, err
	}
	return old, nil
}

// restoreWebsiteConfigLocked 在域名持久化失败时恢复原始站点配置。
func (s *WebsiteService) restoreWebsiteConfigLocked(websiteID uint, content []byte) error {
	if content == nil {
		return nil
	}
	site, err := s.getWebsiteLocked(websiteID)
	if err != nil {
		return err
	}
	return writeWebsiteAtomic(s.SitePath(site, "site.conf"), content, 0o640)
}
