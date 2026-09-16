// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// Groups 返回分组列表。
func (s *CoreService) Groups() []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]map[string]any, 0, len(s.groups))
	for _, group := range s.groups {
		result = append(result, group)
	}
	return result
}

// UpsertGroup 创建或更新分组；持久化失败时不修改内存快照。
func (s *CoreService) UpsertGroup(id, name, kind string) (map[string]any, error) {
	if id == "" {
		id = randomToken()[:12]
	}
	item := map[string]any{"id": id, "name": name, "type": kind}
	s.mu.Lock()
	if s.repository != nil {
		if _, err := s.repository.Exec(`INSERT INTO core_groups(id,name,type,updated_at) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,type=excluded.type,updated_at=excluded.updated_at`, id, name, kind, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			s.mu.Unlock()
			return nil, err
		}
	}
	s.groups[id] = item
	s.mu.Unlock()
	return item, nil
}

// DeleteGroup 删除分组。
func (s *CoreService) DeleteGroup(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.groups[id]; !ok {
		return errors.New("分组不存在")
	}
	if s.repository != nil {
		if _, err := s.repository.Exec(`DELETE FROM core_groups WHERE id=?`, id); err != nil {
			return err
		}
	}
	delete(s.groups, id)
	return nil
}

// Settings 返回设置快照。
func (s *CoreService) Settings() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]string, len(s.settings))
	for key, value := range s.settings {
		result[key] = value
	}
	return result
}

// UpdateSettings 合并设置字段；SQLite 可用时批量事务提交，失败不修改内存快照。
func (s *CoreService) UpdateSettings(values map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	updates := make(map[string]string)
	for key, value := range values {
		if strings.TrimSpace(key) != "" {
			updates[key] = value
		}
	}
	if s.repository != nil {
		if err := s.repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			for key, value := range updates {
				if _, err := tx.Exec(`INSERT INTO core_settings(setting_key,value,updated_at) VALUES(?,?,?) ON CONFLICT(setting_key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, key, value, now); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	for key, value := range updates {
		s.settings[key] = value
	}
	return nil
}

// loadGroupsSettingsFromDB 从 SQLite 恢复用户分组和核心设置快照。
func (s *CoreService) loadGroupsSettingsFromDB(db storage.SQLExecutor) {
	rows, err := db.Query(`SELECT id,name,type FROM core_groups`)
	if err == nil {
		defer rows.Close()
		s.mu.Lock()
		for rows.Next() {
			var id, name, kind string
			if rows.Scan(&id, &name, &kind) == nil {
				s.groups[id] = map[string]any{"id": id, "name": name, "type": kind}
			}
		}
		s.mu.Unlock()
	}
	settingsRows, err := db.Query(`SELECT setting_key,value FROM core_settings`)
	if err == nil {
		defer settingsRows.Close()
		s.mu.Lock()
		for settingsRows.Next() {
			var key, value string
			if settingsRows.Scan(&key, &value) == nil {
				s.settings[key] = value
			}
		}
		s.mu.Unlock()
	}
}
