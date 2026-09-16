// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/todaybin/workmesh-server/node/model"
)

var authUserFilePattern = regexp.MustCompile(`(?m)^\s*auth_basic_user_file\s+([^;\s]+)`)
var authLocationPattern = regexp.MustCompile(`(?m)^\s*location\s+([^\s{]+)\s*\{`)

// ListWebsiteAuths 从站点真实 auth_basic 配置和 users.htpasswd 返回 Basic Auth 用户。
// 密码只参与生成 htpasswd，不从接口回传，避免泄露凭据。
func (s *WebsiteService) ListWebsiteAuths(id uint) (map[string]any, error) {
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(s.SitePath(site, "auth_basic"), "managed.conf")
	content, readErr := os.ReadFile(configPath)
	if errors.Is(readErr, os.ErrNotExist) {
		return map[string]any{"enable": false, "items": []map[string]any{}}, nil
	}
	if readErr != nil {
		return nil, readErr
	}
	usersPath := firstRegexpValue(authUserFilePattern.String(), string(content))
	items, err := s.readAuthUsers(site, usersPath)
	if err != nil {
		return nil, err
	}
	return map[string]any{"enable": strings.Contains(string(content), "auth_basic ") && len(items) > 0, "items": items}, nil
}

// ListWebsitePathAuths 从 path_auth/managed.conf 读取路径级认证配置。
func (s *WebsiteService) ListWebsitePathAuths(id uint) ([]map[string]any, error) {
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(s.SitePath(site, "path_auth"), "managed.conf")
	content, readErr := os.ReadFile(configPath)
	if errors.Is(readErr, os.ErrNotExist) {
		return []map[string]any{}, nil
	}
	if readErr != nil {
		return nil, readErr
	}
	pathMatch := authLocationPattern.FindStringSubmatch(string(content))
	if len(pathMatch) < 2 {
		return []map[string]any{}, nil
	}
	usersPath := firstRegexpValue(authUserFilePattern.String(), string(content))
	users, err := s.readAuthUsers(site, usersPath)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(users))
	for _, user := range users {
		items = append(items, map[string]any{
			"websiteID": id, "operate": "update", "path": pathMatch[1],
			"username": user["username"], "name": filepath.Base(configPath),
		})
	}
	return items, nil
}

func (s *WebsiteService) readAuthUsers(site model.Website, rawPath string) ([]map[string]any, error) {
	if strings.TrimSpace(rawPath) == "" {
		return []map[string]any{}, nil
	}
	usersPath := filepath.Clean(rawPath)
	if !filepath.IsAbs(usersPath) {
		usersPath = filepath.Join(s.SitePath(site, "auth_basic"), usersPath)
	}
	if !withinPath(s.SitePath(site, "auth_basic"), usersPath) {
		return nil, errors.New("认证用户文件路径越界")
	}
	data, err := os.ReadFile(usersPath)
	if errors.Is(err, os.ErrNotExist) {
		return []map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0)
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		username, _, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(username) == "" {
			continue
		}
		items = append(items, map[string]any{"username": strings.TrimSpace(username), "remark": ""})
	}
	return items, nil
}
