// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

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

// GetRewrite 返回站点实际 rewrite 文件或内置规则内容。
func (s *WebsiteService) GetRewrite(id uint, name string) (map[string]any, error) {
	s.mu.RLock()
	site, err := s.getWebsiteLocked(id)
	s.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	content := ""
	templates := map[string]string{
		"default":   "location / {\n    try_files $uri $uri/ =404;\n}",
		"wordpress": "location / {\n    try_files $uri $uri/ /index.php?$args;\n}",
		"wp2":       "location / {\n    try_files $uri $uri/ /index.php?$args;\n}",
		"typecho":   "location / {\n    try_files $uri $uri/ /index.php?$query_string;\n}",
		"typecho2":  "location / {\n    try_files $uri $uri/ /index.php?$query_string;\n}",
		"thinkphp":  "location / {\n    try_files $uri $uri/ /index.php?$query_string;\n}",
		"laravel5":  "location / {\n    try_files $uri $uri/ /index.php?$query_string;\n}",
		"yii2":      "location / {\n    try_files $uri $uri/ /index.php?$query_string;\n}",
	}
	switch name {
	case "current":
		data, readErr := os.ReadFile(s.SitePath(site, "rewrite"))
		if readErr == nil {
			content = string(data)
		}
	case "default", "wordpress", "wp2", "typecho", "typecho2", "thinkphp", "laravel5", "yii2":
		content = templates[name]
	default:
		if cfg, e := s.GetConfig(id, "rewrite-custom"); e == nil {
			content, _ = cfg["content"].(string)
		}
	}
	content = strings.ReplaceAll(content, `\n`, "\n")
	return map[string]any{"content": content}, nil
}

// UpdateRewrite 写入域名目录 rewrite 文件，并在失败时恢复原文件。
func (s *WebsiteService) UpdateRewrite(id uint, name, content string) error {
	if len(content) > 64<<10 || strings.IndexByte(content, 0) >= 0 {
		return errors.New("rewrite 内容无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	site, err := s.getWebsiteLocked(id)
	if err != nil {
		return err
	}
	target := s.SitePath(site, "rewrite")
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	oldFiles, err := snapshotWebsiteFiles(target, s.SitePath(site, "site.conf"))
	if err != nil {
		return err
	}
	oldWebsites := append([]model.Website(nil), s.websites...)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o640); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		return errors.Join(err, restoreWebsiteFiles(oldFiles))
	}
	nginxPath := s.SitePath(site, "site.conf")
	nginxOld := oldFiles[nginxPath]
	includeLine := "    include " + target + ";"
	if !strings.Contains(string(nginxOld), includeLine) {
		nginxNew := strings.TrimRight(string(nginxOld), "\n") + "\n" + includeLine + "\n"
		if err := os.WriteFile(nginxPath+".tmp", []byte(nginxNew), 0o640); err != nil {
			return errors.Join(err, restoreWebsiteFiles(oldFiles))
		}
		if err := os.Rename(nginxPath+".tmp", nginxPath); err != nil {
			return errors.Join(err, restoreWebsiteFiles(oldFiles))
		}
	}
	for i := range s.websites {
		if s.websites[i].ID == id {
			s.websites[i].Rewrite = name
			s.websites[i].UpdatedAt = time.Now().UTC()
		}
	}
	if err := s.persist("websites", s.websites); err != nil {
		s.websites = oldWebsites
		return errors.Join(err, restoreWebsiteFiles(oldFiles))
	}
	return nil
}
