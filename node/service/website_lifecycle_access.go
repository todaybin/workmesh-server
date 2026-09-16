// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

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
		base = s.siteDirPath(site.PrimaryDomain)
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
