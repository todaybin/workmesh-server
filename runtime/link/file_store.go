// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package link

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const maxPersistedStreams = 128

// FileSyncStore 将节点增量同步游标和最新快照持久化到受保护的数据目录。
// 每个流只保留最新快照，避免离线期间无限增长；调用方应在业务层处理历史审计。
type FileSyncStore struct {
	mu      sync.RWMutex
	path    string
	streams map[string]syncRecord
}

type fileSyncRecord struct {
	Payload []byte `json:"payload,omitempty"`
	Version uint64 `json:"version"`
}

// NewFileSyncStore 创建并加载文件同步存储；损坏文件会返回错误而不会静默清空。
func NewFileSyncStore(path string) (*FileSyncStore, error) {
	path = filepath.Clean(path)
	if path == "." || !filepath.IsAbs(path) || filepath.Base(path) == "" {
		return nil, errors.New("同步存储路径必须是绝对文件路径")
	}
	store := &FileSyncStore{path: path, streams: make(map[string]syncRecord)}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取同步存储失败: %w", err)
	}
	var saved map[string]fileSyncRecord
	if err := json.Unmarshal(raw, &saved); err != nil {
		return nil, fmt.Errorf("解析同步存储失败: %w", err)
	}
	if len(saved) > maxPersistedStreams {
		return nil, errors.New("同步存储流数量超过限制")
	}
	for stream, record := range saved {
		if err := validateStream(stream); err != nil || record.Version == 0 || len(record.Payload) > maxRequestBytes {
			return nil, errors.New("同步存储内容无效")
		}
		store.streams[stream] = syncRecord{payload: append([]byte(nil), record.Payload...), version: record.Version}
	}
	return store, nil
}

// Pull 返回指定流游标之后的最新快照。
func (s *FileSyncStore) Pull(ctx context.Context, cursor SyncCursor) ([]byte, SyncCursor, error) {
	if err := ctx.Err(); err != nil {
		return nil, SyncCursor{}, err
	}
	if err := validateStream(cursor.Stream); err != nil {
		return nil, SyncCursor{}, err
	}
	s.mu.RLock()
	record := s.streams[cursor.Stream]
	s.mu.RUnlock()
	next := SyncCursor{Stream: cursor.Stream, Version: record.version}
	if cursor.Version >= record.version {
		return nil, next, nil
	}
	return append([]byte(nil), record.payload...), next, nil
}

// Push 通过 compare-and-set 写入快照，并使用原子替换持久化。
func (s *FileSyncStore) Push(ctx context.Context, cursor SyncCursor, payload []byte) (SyncCursor, error) {
	if err := ctx.Err(); err != nil {
		return SyncCursor{}, err
	}
	if err := validateStream(cursor.Stream); err != nil {
		return SyncCursor{}, err
	}
	if len(payload) > maxRequestBytes {
		return SyncCursor{}, errors.New("同步数据超过 8 MiB 限制")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, exists := s.streams[cursor.Stream]
	if !exists && len(s.streams) >= maxPersistedStreams {
		return SyncCursor{}, errors.New("同步流数量超过限制")
	}
	if cursor.Version < record.version {
		if exists && equalBytes(record.payload, payload) {
			return SyncCursor{Stream: cursor.Stream, Version: record.version}, nil
		}
		return SyncCursor{Stream: cursor.Stream, Version: record.version}, ErrSyncConflict
	}
	if cursor.Version > record.version {
		return SyncCursor{Stream: cursor.Stream, Version: record.version}, ErrSyncConflict
	}
	record.version++
	record.payload = append([]byte(nil), payload...)
	s.streams[cursor.Stream] = record
	if err := s.persistLocked(); err != nil {
		delete(s.streams, cursor.Stream)
		return SyncCursor{}, fmt.Errorf("保存同步快照失败: %w", err)
	}
	return SyncCursor{Stream: cursor.Stream, Version: record.version}, nil
}

// equalBytes 比较两段同步快照内容，供旧游标重试的幂等判断使用。
func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// persistLocked 在持有写锁时以临时文件原子替换同步快照。
func (s *FileSyncStore) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	saved := make(map[string]fileSyncRecord, len(s.streams))
	for stream, record := range s.streams {
		saved[stream] = fileSyncRecord{Payload: append([]byte(nil), record.payload...), Version: record.version}
	}
	raw, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".link-sync-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

var _ SyncStore = (*FileSyncStore)(nil)
