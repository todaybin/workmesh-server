// SPDX-License-Identifier: LicenseRef-WorkMesh-Pending
// Copyright (c) 2026 WorkMesh contributors

package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Store 是控制面与执行面共享的轻量键值持久化接口，后续迁移模型时保持调用边界稳定。
type Store struct {
	mu   sync.RWMutex
	path string
	data map[string]json.RawMessage
}

// Open 打开节点本地状态文件；目录不存在时自动创建。
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("存储路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	s := &Store{path: path, data: make(map[string]json.RawMessage)}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(content) > 0 {
		if err := json.Unmarshal(content, &s.data); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Get 将指定键反序列化到目标对象。
func (s *Store) Get(ctx context.Context, key string, target any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.RLock()
	value, ok := s.data[key]
	s.mu.RUnlock()
	if !ok {
		return os.ErrNotExist
	}
	return json.Unmarshal(value, target)
}

// Set 保存键值并原子替换状态文件。
func (s *Store) Set(ctx context.Context, key string, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.data[key] = encoded
	content, err := json.Marshal(s.data)
	if err == nil {
		tmp := s.path + ".tmp"
		err = os.WriteFile(tmp, content, 0o600)
		if err == nil {
			err = os.Rename(tmp, s.path)
		}
	}
	s.mu.Unlock()
	return err
}

// Close 释放存储资源；当前实现无持久连接，保留生命周期接口便于替换数据库。
func (s *Store) Close() error { return nil }
