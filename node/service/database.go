// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
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
}

func NewDatabaseRepository() *DatabaseRepository {
	return &DatabaseRepository{next: 1, items: make(map[int64]Database)}
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
	return item, nil
}
func (r *DatabaseRepository) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return errors.New("数据库不存在")
	}
	delete(r.items, id)
	return nil
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
func (s *DatabaseService) Check(_ context.Context, item Database) bool {
	return strings.TrimSpace(item.Host) != "" && item.Port > 0 && item.Port <= 65535
}
