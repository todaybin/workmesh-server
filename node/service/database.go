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

type Database struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Username    string    `json:"username"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}
type DatabaseRepository struct {
	mu    sync.RWMutex
	next  int64
	items map[int64]Database
	path  string
}

func NewDatabaseRepository() *DatabaseRepository {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = ".workmesh-data"
	}
	r := &DatabaseRepository{next: 1, items: make(map[int64]Database), path: filepath.Join(dir, "databases.json")}
	if b, err := os.ReadFile(r.path); err == nil {
		var payload struct {
			Next  int64              `json:"next"`
			Items map[int64]Database `json:"items"`
		}
		if json.Unmarshal(b, &payload) == nil {
			if payload.Next > 0 {
				r.next = payload.Next
			}
			if payload.Items != nil {
				r.items = payload.Items
			}
		}
	}
	return r
}
func (r *DatabaseRepository) List(_ context.Context, typ, name string) []Database {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Database, 0)
	for _, item := range r.items {
		if typ != "" && item.Type != typ {
			continue
		}
		if name != "" && !strings.Contains(strings.ToLower(item.Name), strings.ToLower(name)) {
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (r *DatabaseRepository) Create(_ context.Context, item Database) (Database, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item.Name == "" || item.Type == "" {
		return Database{}, errors.New("数据库名称和类型不能为空")
	}
	item.ID = r.next
	r.next++
	item.CreatedAt = time.Now().UTC()
	r.items[item.ID] = item
	if err := r.saveLocked(); err != nil {
		delete(r.items, item.ID)
		r.next--
		return Database{}, err
	}
	return item, nil
}
func (r *DatabaseRepository) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return errors.New("数据库不存在")
	}
	delete(r.items, id)
	return r.saveLocked()
}

// Update 修改已登记的数据库连接信息，不会覆盖创建时间。
func (r *DatabaseRepository) Update(_ context.Context, item Database) (Database, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.items[item.ID]
	if !ok {
		return Database{}, errors.New("数据库不存在")
	}
	if item.Name == "" || item.Type == "" {
		return Database{}, errors.New("数据库名称和类型不能为空")
	}
	item.CreatedAt = current.CreatedAt
	r.items[item.ID] = item
	if err := r.saveLocked(); err != nil {
		r.items[item.ID] = current
		return Database{}, err
	}
	return item, nil
}

func (r *DatabaseRepository) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o750); err != nil {
		return err
	}
	b, err := json.Marshal(struct {
		Next  int64              `json:"next"`
		Items map[int64]Database `json:"items"`
	}{r.next, r.items})
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}

type DatabaseService struct{ repo *DatabaseRepository }

func NewDatabaseService(repo *DatabaseRepository) *DatabaseService {
	if repo == nil {
		repo = NewDatabaseRepository()
	}
	return &DatabaseService{repo: repo}
}
func (s *DatabaseService) Search(ctx context.Context, typ, name string) []Database {
	return s.repo.List(ctx, typ, name)
}
func (s *DatabaseService) Create(ctx context.Context, item Database) (Database, error) {
	if item.Port < 0 || item.Port > 65535 {
		return Database{}, errors.New("数据库端口无效")
	}
	return s.repo.Create(ctx, item)
}
func (s *DatabaseService) Delete(ctx context.Context, id int64) error { return s.repo.Delete(ctx, id) }

// Update 修改数据库资源登记并保留原始创建时间。
func (s *DatabaseService) Update(ctx context.Context, item Database) (Database, error) {
	if item.Port < 0 || item.Port > 65535 {
		return Database{}, errors.New("数据库端口无效")
	}
	if strings.TrimSpace(item.Host) == "" {
		return Database{}, errors.New("数据库主机不能为空")
	}
	return s.repo.Update(ctx, item)
}
func (s *DatabaseService) Check(_ context.Context, item Database) bool {
	return strings.TrimSpace(item.Host) != "" && item.Port > 0 && item.Port <= 65535
}
