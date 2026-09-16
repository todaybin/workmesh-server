// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

// defaultHTMLTypes 与 1Panel 默认页面选择器保持一致，避免任意路径写入站点目录。
var defaultHTMLTypes = map[string]string{
	"404":       "404.html",
	"domain404": "404.html",
	"index":     "index.html",
	"php":       "index.php",
	"stop":      "stop.html",
}

// WebsiteDefaultHTMLMigration 为默认页面关系表提供可审计、幂等的 SQLite 迁移。
func WebsiteDefaultHTMLMigration() storage.Migration {
	return storage.SQLMigration("0014-website-default-html", `CREATE TABLE IF NOT EXISTS website_default_html (
    type TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    sync INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL
)`)
}

// DefaultHTML 读取面板级默认页面的真实 SQLite 配置。
func (s *WebsiteService) DefaultHTML(typ string) (map[string]any, error) {
	typ = strings.ToLower(strings.TrimSpace(typ))
	if _, ok := defaultHTMLTypes[typ]; !ok {
		return nil, errors.New("默认页面类型无效")
	}
	repository, err := s.sqliteRepository()
	if err != nil {
		return nil, err
	}
	var content string
	var syncEnabled int
	var updatedAt string
	err = repository.QueryRow(`SELECT content,sync,updated_at FROM website_default_html WHERE type=?`, typ).Scan(&content, &syncEnabled, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		content = builtinDefaultHTML(typ)
		return map[string]any{"type": typ, "content": content, "sync": false, "source": "builtin"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取默认页面失败: %w", err)
	}
	return map[string]any{"type": typ, "content": content, "sync": syncEnabled != 0, "source": "sqlite", "updatedAt": updatedAt}, nil
}

// UpdateDefaultHTML 将默认页面写入 SQLite；sync=true 时再同步到已存在站点的真实文件。
func (s *WebsiteService) UpdateDefaultHTML(typ, content string, syncEnabled bool) (map[string]any, error) {
	typ = strings.ToLower(strings.TrimSpace(typ))
	fileName, ok := defaultHTMLTypes[typ]
	if !ok {
		return nil, errors.New("默认页面类型无效")
	}
	if strings.TrimSpace(content) == "" || len(content) > 2<<20 || strings.IndexByte(content, 0) >= 0 {
		return nil, errors.New("默认页面内容无效")
	}
	repository, err := s.sqliteRepository()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if syncEnabled {
		if err := s.syncDefaultHTMLToSites(fileName, []byte(content)); err != nil {
			return nil, err
		}
	}
	if _, err := repository.Exec(`INSERT INTO website_default_html(type,content,sync,updated_at) VALUES(?,?,?,?) ON CONFLICT(type) DO UPDATE SET content=excluded.content,sync=excluded.sync,updated_at=excluded.updated_at`, typ, content, boolInt(syncEnabled), now); err != nil {
		return nil, fmt.Errorf("保存默认页面失败: %w", err)
	}
	return map[string]any{"type": typ, "content": content, "sync": syncEnabled, "source": "sqlite", "updatedAt": now}, nil
}

// syncDefaultHTMLToSites 将页面同步到 SQLite 中登记的站点目录，失败时恢复已经写入的文件。
func (s *WebsiteService) syncDefaultHTMLToSites(fileName string, content []byte) error {
	s.mu.RLock()
	sites := append([]model.Website(nil), s.websites...)
	s.mu.RUnlock()
	type backup struct {
		path   string
		data   []byte
		mode   os.FileMode
		exists bool
	}
	backups := make([]backup, 0, len(sites))
	for _, site := range sites {
		path := filepath.Join(s.SitePath(site, "app"), fileName)
		old, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("读取默认页面 %s 失败: %w", path, err)
		}
		info, statErr := os.Stat(path)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("检查默认页面 %s 失败: %w", path, statErr)
		}
		entry := backup{path: path, data: old, exists: !errors.Is(err, os.ErrNotExist)}
		if info != nil {
			entry.mode = info.Mode().Perm()
		}
		backups = append(backups, entry)
	}
	for _, entry := range backups {
		if err := writeWebsiteAtomic(entry.path, content, 0o644); err != nil {
			var restoreErr error
			for _, previous := range backups {
				if previous.exists {
					if err := writeWebsiteAtomic(previous.path, previous.data, previous.mode); err != nil {
						restoreErr = errors.Join(restoreErr, err)
					}
				} else {
					if err := os.Remove(previous.path); err != nil && !errors.Is(err, os.ErrNotExist) {
						restoreErr = errors.Join(restoreErr, err)
					}
				}
			}
			return errors.Join(fmt.Errorf("同步默认页面失败: %w", err), restoreErr)
		}
	}
	return nil
}

// builtinDefaultHTML 仅用于首次读取尚未配置的页面，不伪造已保存的业务状态。
func builtinDefaultHTML(typ string) string {
	switch typ {
	case "index":
		return defaultWebsiteIndexHTML
	case "404", "domain404":
		return defaultWebsite404HTML
	case "php":
		return "<?php\nhttp_response_code(200);\n?>\n"
	default:
		return ""
	}
}
