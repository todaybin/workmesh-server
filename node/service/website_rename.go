// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/todaybin/workmesh-server/node/model"
)

// rewriteRenamedSiteConfig keeps the runtime configuration valid after a
// primary-domain rename. Both server_name and managed absolute paths may
// contain the previous domain and must move with the actual site directory.
func (s *WebsiteService) rewriteRenamedSiteConfig(oldSite, newSite model.Website, oldPath, newPath string) error {
	path := s.SitePath(newSite, "site.conf")
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	updated := strings.ReplaceAll(string(content), oldPath, newPath)
	lines := strings.Split(updated, "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "server_name ") || !strings.HasSuffix(trimmed, ";") {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		fields := strings.Fields(strings.TrimSuffix(strings.TrimSpace(trimmed[len("server_name "):]), ";"))
		changed := false
		for field := range fields {
			if strings.EqualFold(fields[field], oldSite.PrimaryDomain) {
				fields[field] = newSite.PrimaryDomain
				changed = true
			}
		}
		if changed {
			lines[index] = indent + "server_name " + strings.Join(fields, " ") + ";"
		}
	}
	return writeWebsiteAtomic(path, []byte(strings.Join(lines, "\n")), 0o640)
}

// restoreRenamedSite rolls back a completed directory move after a later
// SQLite write fails. It refuses to overwrite an unrelated existing path.
func (s *WebsiteService) restoreRenamedSite(oldSite, newSite model.Website) error {
	oldPath := strings.TrimSpace(oldSite.SiteDir)
	newPath := strings.TrimSpace(newSite.SiteDir)
	if oldPath == "" {
		oldPath = s.siteDirPath(oldSite.PrimaryDomain)
	}
	if newPath == "" {
		newPath = s.siteDirPath(newSite.PrimaryDomain)
	}
	if oldPath == newPath {
		return nil
	}
	if _, err := os.Stat(oldPath); err == nil {
		return errors.New("回滚站点目录时目标目录已存在")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(newPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := os.Rename(newPath, oldPath); err != nil {
		return err
	}
	restoredSite := oldSite
	restoredSite.SiteDir = oldPath
	if err := s.rewriteRenamedSiteConfig(newSite, restoredSite, newPath, oldPath); err != nil {
		return fmt.Errorf("恢复站点配置文件失败: %w", err)
	}
	return nil
}
