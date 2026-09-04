// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Group 与原 Agent 的分组 DTO 保持兼容，网站和主机等资源通过 Type 区分。
type Group struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	IsDefault bool   `json:"isDefault"`
	IsDelete  bool   `json:"isDelete"`
}

// GroupService 使用进程统一 SQLite 保存分组；legacyPath 只用于一次性迁移。
type GroupService struct {
	db         *sql.DB
	legacyPath string
	initErr    error
}

// NewGroupService 创建分组服务，并在空表时导入旧 groups.json。
func NewGroupService(root string) *GroupService {
	if strings.TrimSpace(root) == "" {
		root = os.Getenv("WORKMESH_DATA_DIR")
	}
	if strings.TrimSpace(root) == "" {
		root = "./data"
	}
	s := &GroupService{db: SharedDatabase(), legacyPath: filepath.Join(root, "groups.json")}
	if s.db == nil {
		s.initErr = errors.New("公共数据库未初始化")
		return s
	}
	s.initErr = s.initialize()
	return s
}

func (s *GroupService) initialize() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS resource_groups (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL, is_default INTEGER NOT NULL DEFAULT 0, is_delete INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, UNIQUE(type,name))`); err != nil {
		return fmt.Errorf("初始化分组表失败: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_resource_groups_type_default ON resource_groups(type,is_default DESC,id)`); err != nil {
		return fmt.Errorf("初始化分组索引失败: %w", err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM resource_groups`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		var legacy []Group
		if payload, err := os.ReadFile(s.legacyPath); err == nil && json.Unmarshal(payload, &legacy) == nil {
			for _, group := range legacy {
				if _, err := s.insertLegacy(group); err != nil {
					return err
				}
			}
		}
	}
	// 默认分组属于业务不变量，每种内置资源至少保留一个默认分组。
	for _, kind := range []string{"website", "host"} {
		var exists int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM resource_groups WHERE type=?`, kind).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if _, err := s.db.Exec(`INSERT INTO resource_groups(name,type,is_default,is_delete,created_at,updated_at) VALUES('Default',?,1,0,?,?)`, kind, now, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *GroupService) insertLegacy(group Group) (sql.Result, error) {
	if strings.TrimSpace(group.Name) == "" || strings.TrimSpace(group.Type) == "" {
		return nil, errors.New("旧分组数据缺少名称或类型")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.db.Exec(`INSERT OR IGNORE INTO resource_groups(id,name,type,is_default,is_delete,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, group.ID, group.Name, group.Type, databaseBoolInt(group.IsDefault), databaseBoolInt(group.IsDelete), now, now)
}

// List 按类型返回有界分组列表，默认分组始终排在前面。
func (s *GroupService) List(kind string) []Group {
	if s.initErr != nil || s.db == nil {
		return []Group{}
	}
	rows, err := s.db.Query(`SELECT id,name,type,is_default,is_delete FROM resource_groups WHERE (?='' OR type=?) ORDER BY is_default DESC,id LIMIT 1000`, strings.TrimSpace(kind), strings.TrimSpace(kind))
	if err != nil {
		return []Group{}
	}
	defer rows.Close()
	result := make([]Group, 0)
	for rows.Next() {
		var group Group
		var isDefault, isDelete int
		if err := rows.Scan(&group.ID, &group.Name, &group.Type, &isDefault, &isDelete); err != nil {
			continue
		}
		group.IsDefault, group.IsDelete = isDefault != 0, isDelete != 0
		result = append(result, group)
	}
	return result
}

// Upsert 在事务中新增或更新分组，并原子维护同类型唯一默认组。
func (s *GroupService) Upsert(id uint, name, kind string, isDefault bool) (Group, error) {
	if s.initErr != nil {
		return Group{}, s.initErr
	}
	name, kind = strings.TrimSpace(name), strings.TrimSpace(kind)
	if name == "" || kind == "" || len(name) > 64 {
		return Group{}, errors.New("分组参数无效")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Group{}, err
	}
	defer tx.Rollback()
	if isDefault {
		if _, err := tx.Exec(`UPDATE resource_groups SET is_default=0,updated_at=? WHERE type=?`, time.Now().UTC().Format(time.RFC3339Nano), kind); err != nil {
			return Group{}, err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if id == 0 {
		res, err := tx.Exec(`INSERT INTO resource_groups(name,type,is_default,is_delete,created_at,updated_at) VALUES(?,?,?,?,?,?)`, name, kind, databaseBoolInt(isDefault), 0, now, now)
		if err != nil {
			return Group{}, fmt.Errorf("保存分组失败: %w", err)
		}
		lastID, err := res.LastInsertId()
		if err != nil {
			return Group{}, err
		}
		id = uint(lastID)
	} else {
		res, err := tx.Exec(`UPDATE resource_groups SET name=?,type=?,is_default=?,updated_at=? WHERE id=?`, name, kind, databaseBoolInt(isDefault), now, id)
		if err != nil {
			return Group{}, fmt.Errorf("更新分组失败: %w", err)
		}
		if affected, _ := res.RowsAffected(); affected == 0 {
			return Group{}, errors.New("分组不存在")
		}
	}
	if err := tx.Commit(); err != nil {
		return Group{}, err
	}
	return Group{ID: id, Name: name, Type: kind, IsDefault: isDefault}, nil
}

// Delete 删除非默认且未被业务资源引用的分组。
func (s *GroupService) Delete(id uint, inUse func(uint) bool) error {
	if s.initErr != nil {
		return s.initErr
	}
	var isDefault int
	if err := s.db.QueryRow(`SELECT is_default FROM resource_groups WHERE id=?`, id).Scan(&isDefault); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("分组不存在")
		}
		return err
	}
	if isDefault != 0 {
		return errors.New("默认分组不可删除")
	}
	if inUse != nil && inUse(id) {
		return errors.New("分组正在被网站使用")
	}
	_, err := s.db.Exec(`DELETE FROM resource_groups WHERE id=?`, id)
	return err
}

// Touch 保留更新时间语义，便于审计和迁移时判断数据是否刷新。
func (s *GroupService) Touch() time.Time { return time.Now().UTC() }
