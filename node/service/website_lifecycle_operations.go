// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

// Delete 删除网站及其 WAF 规则、持久化配置和隔离站点目录。
func (s *WebsiteService) Delete(id uint) error {
	if id == 0 {
		return errors.New("网站 ID 无效")
	}
	// 删除会改变当前 OpenResty 站点集合；配置语法异常时先拒绝操作，
	// 避免数据库已删除而运行配置仍无法加载的半完成状态。
	status := s.ProbeOpenResty(context.Background())
	if status.Available && !status.ConfigValid && !strings.Contains(status.Error, "无法进入") && !strings.Contains(status.Error, "无权限") {
		return fmt.Errorf("删除网站前 OpenResty 配置检查失败: %s", status.Error)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, site := range s.websites {
		if site.ID != id {
			continue
		}
		for _, candidate := range s.websites {
			if candidate.ParentWebsiteID == id {
				return fmt.Errorf("父网站仍有子网站: %s", candidate.PrimaryDomain)
			}
		}
		siteRoot := filepath.Clean(s.SitePath(site, "root"))
		websiteRoot := filepath.Clean(s.siteRootPath())
		if siteRoot == websiteRoot || !withinPath(websiteRoot, siteRoot) {
			return errors.New("网站目录超出允许的清理范围")
		}
		snapshot, snapshotErr := captureWebsiteState(s)
		if snapshotErr != nil {
			return fmt.Errorf("保存删除回滚快照失败: %w", snapshotErr)
		}
		backupPath := siteRoot + fmt.Sprintf(".delete-backup-%d", time.Now().UTC().UnixNano())
		hadSiteRoot := false
		if _, statErr := os.Stat(siteRoot); statErr == nil {
			if err := os.Rename(siteRoot, backupPath); err != nil {
				return fmt.Errorf("准备删除站点目录失败: %w", err)
			}
			hadSiteRoot = true
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("检查站点目录失败: %w", statErr)
		}
		restore := func(cause error) error {
			s.websites = snapshot.Websites
			s.wafSites = snapshot.WAFSites
			s.domains = snapshot.Domains
			s.configs = snapshot.Configs
			var restoreErr error
			if err := s.persist("websites", snapshot.Websites); err != nil {
				restoreErr = errors.Join(restoreErr, err)
			}
			if err := s.persist("website-domains", snapshot.Domains); err != nil {
				restoreErr = errors.Join(restoreErr, err)
			}
			if err := s.persist("website-configs", snapshot.Configs); err != nil {
				restoreErr = errors.Join(restoreErr, err)
			}
			if err := s.persistWAFSites(); err != nil {
				restoreErr = errors.Join(restoreErr, err)
			}
			if hadSiteRoot {
				if err := os.Rename(backupPath, siteRoot); err != nil {
					restoreErr = errors.Join(restoreErr, err)
				}
			}
			return errors.Join(cause, restoreErr)
		}
		s.websites = append(s.websites[:i], s.websites[i+1:]...)
		delete(s.wafSites, id)
		delete(s.domains, id)
		delete(s.configs, id)
		if err := s.persist("websites", s.websites); err != nil {
			return restore(err)
		}
		if err := s.persistWAFSites(); err != nil {
			return restore(err)
		}
		if err := s.persist("website-domains", s.domains); err != nil {
			return restore(err)
		}
		if err := s.persist("website-configs", s.configs); err != nil {
			return restore(err)
		}
		if repository, repositoryErr := s.sqliteRepository(); repositoryErr == nil {
			if err := repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
				_, err := tx.Exec(`DELETE FROM websites WHERE id=?`, id)
				return err
			}); err != nil {
				return restore(err)
			}
		} else if s.db != nil {
			return restore(repositoryErr)
		}
		if hadSiteRoot {
			if err := os.RemoveAll(backupPath); err != nil {
				return restore(fmt.Errorf("清理网站目录失败: %w", err))
			}
		}
		return nil
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
		previous := s.websites[i]
		if operation == "stop" {
			s.websites[i].Status = "stopped"
		} else {
			s.websites[i].Status = "running"
		}
		s.websites[i].UpdatedAt = time.Now().UTC()
		if err := s.persist("websites", s.websites); err != nil {
			s.websites[i] = previous
			return model.Website{}, err
		}
		return s.websites[i], nil
	}
	return model.Website{}, os.ErrNotExist
}
