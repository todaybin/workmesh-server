// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package link

import (
	"bytes"
	"context"
	"errors"
	"sync"
)

// MemorySyncStore 是无外部依赖的同步存储，适合单节点部署和测试。
type MemorySyncStore struct {
	mu      sync.RWMutex
	streams map[string]syncRecord
}

// syncRecord 保存一个同步流的最新快照及其单调递增版本。
type syncRecord struct {
	payload []byte
	version uint64
}

// NewMemorySyncStore 创建内存同步存储。
func NewMemorySyncStore() *MemorySyncStore {
	return &MemorySyncStore{streams: make(map[string]syncRecord)}
}

// Pull 返回指定游标之后的最新快照；没有更新时 payload 为空。
func (s *MemorySyncStore) Pull(ctx context.Context, cursor SyncCursor) ([]byte, SyncCursor, error) {
	if err := ctx.Err(); err != nil {
		return nil, SyncCursor{}, err
	}
	if err := validateStream(cursor.Stream); err != nil {
		return nil, SyncCursor{}, err
	}
	s.mu.RLock()
	record := s.streams[cursor.Stream]
	s.mu.RUnlock()
	current := SyncCursor{Stream: cursor.Stream, Version: record.version}
	if cursor.Version >= record.version {
		return nil, current, nil
	}
	return append([]byte(nil), record.payload...), current, nil
}

// Push 以 compare-and-set 方式写入同步快照，拒绝旧游标覆盖新数据。
func (s *MemorySyncStore) Push(ctx context.Context, cursor SyncCursor, payload []byte) (SyncCursor, error) {
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
	record := s.streams[cursor.Stream]
	if cursor.Version < record.version {
		// 网络重试可能重复提交同一请求；相同内容视为幂等成功。
		if bytes.Equal(record.payload, payload) {
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
	return SyncCursor{Stream: cursor.Stream, Version: record.version}, nil
}

var _ SyncStore = (*MemorySyncStore)(nil)
