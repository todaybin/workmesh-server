// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DatabaseUser 保存数据库实例的登录用户元数据；密码只保存哈希或脱敏标记。
type DatabaseUser struct {
	ID          int64     `json:"id"`
	DatabaseID  int64     `json:"databaseId"`
	Database    string    `json:"database"`
	Type        string    `json:"type"`
	Username    string    `json:"username"`
	Host        string    `json:"host"`
	Description string    `json:"description,omitempty"`
	PasswordSet bool      `json:"passwordSet"`
	CreatedAt   time.Time `json:"createdAt"`
}

// DatabaseGrant 描述用户可访问的数据库和权限集合。
type DatabaseGrant struct {
	ID         int64    `json:"id"`
	Database   string   `json:"database"`
	Username   string   `json:"username"`
	Host       string   `json:"host"`
	Privileges []string `json:"privileges"`
}

// DatabaseVariable 保存经过校验的数据库运行参数。
type DatabaseVariable struct {
	Database  string    `json:"database"`
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// DatabaseAdminStore 是节点侧数据库管理元数据的持久化仓库。
type DatabaseAdminStore struct {
	mu                  sync.RWMutex
	nextUser, nextGrant int64
	users               map[int64]DatabaseUser
	grants              map[int64]DatabaseGrant
	variables           map[string]DatabaseVariable
	configs             map[string]string
	path                string
}

func NewDatabaseAdminStore() *DatabaseAdminStore {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = "./data"
	}
	s := &DatabaseAdminStore{nextUser: 1, nextGrant: 1, users: map[int64]DatabaseUser{}, grants: map[int64]DatabaseGrant{}, variables: map[string]DatabaseVariable{}, configs: map[string]string{}, path: filepath.Join(dir, "database-admin.json")}
	if b, err := os.ReadFile(s.path); err == nil {
		var p struct {
			NextUser, NextGrant int64
			Users               map[int64]DatabaseUser
			Grants              map[int64]DatabaseGrant
			Variables           map[string]DatabaseVariable
			Configs             map[string]string
		}
		if json.Unmarshal(b, &p) == nil {
			if p.NextUser > 0 {
				s.nextUser = p.NextUser
			}
			if p.NextGrant > 0 {
				s.nextGrant = p.NextGrant
			}
			if p.Users != nil {
				s.users = p.Users
			}
			if p.Grants != nil {
				s.grants = p.Grants
			}
			if p.Variables != nil {
				s.variables = p.Variables
			}
			if p.Configs != nil {
				s.configs = p.Configs
			}
		}
	}
	return s
}
func (s *DatabaseAdminStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0750); err != nil {
		return err
	}
	b, err := json.Marshal(struct {
		NextUser, NextGrant int64
		Users               map[int64]DatabaseUser
		Grants              map[int64]DatabaseGrant
		Variables           map[string]DatabaseVariable
		Configs             map[string]string
	}{s.nextUser, s.nextGrant, s.users, s.grants, s.variables, s.configs})
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
func (s *DatabaseAdminStore) ListUsers(_ context.Context, database, username string) []DatabaseUser {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []DatabaseUser{}
	for _, u := range s.users {
		if database != "" && !strings.EqualFold(u.Database, database) {
			continue
		}
		if username != "" && !strings.Contains(strings.ToLower(u.Username), strings.ToLower(username)) {
			continue
		}
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (s *DatabaseAdminStore) CreateUser(_ context.Context, u DatabaseUser) (DatabaseUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(u.Database) == "" || strings.TrimSpace(u.Username) == "" {
		return DatabaseUser{}, errors.New("数据库和用户名不能为空")
	}
	if strings.TrimSpace(u.Host) == "" {
		u.Host = "%"
	}
	for _, x := range s.users {
		if strings.EqualFold(x.Database, u.Database) && strings.EqualFold(x.Username, u.Username) && x.Host == u.Host {
			return DatabaseUser{}, errors.New("数据库用户已存在")
		}
	}
	u.ID = s.nextUser
	s.nextUser++
	u.CreatedAt = time.Now().UTC()
	s.users[u.ID] = u
	if err := s.saveLocked(); err != nil {
		delete(s.users, u.ID)
		return DatabaseUser{}, err
	}
	return u, nil
}
func (s *DatabaseAdminStore) UpdateUser(_ context.Context, u DatabaseUser) (DatabaseUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.users[u.ID]
	if !ok {
		return DatabaseUser{}, errors.New("数据库用户不存在")
	}
	if strings.TrimSpace(u.Username) == "" || strings.TrimSpace(u.Database) == "" {
		return DatabaseUser{}, errors.New("数据库和用户名不能为空")
	}
	u.CreatedAt = old.CreatedAt
	u.PasswordSet = old.PasswordSet
	s.users[u.ID] = u
	if err := s.saveLocked(); err != nil {
		s.users[u.ID] = old
		return DatabaseUser{}, err
	}
	return u, nil
}
func (s *DatabaseAdminStore) DeleteUser(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[id]; !ok {
		return errors.New("数据库用户不存在")
	}
	delete(s.users, id)
	if err := s.saveLocked(); err != nil {
		return err
	}
	return nil
}
func (s *DatabaseAdminStore) SetPassword(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return errors.New("数据库用户不存在")
	}
	u.PasswordSet = true
	s.users[id] = u
	return s.saveLocked()
}
func (s *DatabaseAdminStore) ListGrants(_ context.Context, database, username string) []DatabaseGrant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []DatabaseGrant{}
	for _, g := range s.grants {
		if database != "" && !strings.EqualFold(g.Database, database) {
			continue
		}
		if username != "" && !strings.EqualFold(g.Username, username) {
			continue
		}
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (s *DatabaseAdminStore) UpsertGrant(_ context.Context, g DatabaseGrant) (DatabaseGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if g.Database == "" || g.Username == "" {
		return DatabaseGrant{}, errors.New("数据库和用户名不能为空")
	}
	if g.Host == "" {
		g.Host = "%"
	}
	for id, x := range s.grants {
		if strings.EqualFold(x.Database, g.Database) && strings.EqualFold(x.Username, g.Username) && x.Host == g.Host {
			g.ID = id
			s.grants[id] = g
			return g, s.saveLocked()
		}
	}
	g.ID = s.nextGrant
	s.nextGrant++
	s.grants[g.ID] = g
	return g, s.saveLocked()
}
func (s *DatabaseAdminStore) DeleteGrant(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.grants[id]; !ok {
		return errors.New("授权记录不存在")
	}
	delete(s.grants, id)
	return s.saveLocked()
}
func (s *DatabaseAdminStore) Variables(_ context.Context, database string) []DatabaseVariable {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []DatabaseVariable{}
	for _, v := range s.variables {
		if database == "" || strings.EqualFold(v.Database, database) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func (s *DatabaseAdminStore) SetVariable(_ context.Context, v DatabaseVariable) (DatabaseVariable, error) {
	if strings.TrimSpace(v.Name) == "" || len(v.Name) > 128 {
		return DatabaseVariable{}, errors.New("变量名称无效")
	}
	if len(v.Value) > 4096 {
		return DatabaseVariable{}, errors.New("变量值过长")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v.UpdatedAt = time.Now().UTC()
	s.variables[v.Database+"\x00"+v.Name] = v
	return v, s.saveLocked()
}
func (s *DatabaseAdminStore) SetConfig(_ context.Context, database, content string) error {
	if len(content) > 1<<20 {
		return errors.New("配置文件超过 1 MiB")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configs[database] = content
	return s.saveLocked()
}
func (s *DatabaseAdminStore) Config(_ context.Context, database string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.configs[database]
}
