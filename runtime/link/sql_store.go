// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package link

import (
	"bytes"
	"context"
	"database/sql"
	"errors"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// SQLiteSyncStore 将链路同步游标和快照保存在共享 SQLite 中。
type SQLiteSyncStore struct{ repo storage.Transactional }

// NewSQLiteSyncStore 初始化 SQLite 同步表。
func NewSQLiteSyncStore(db *sql.DB) (*SQLiteSyncStore, error) {
	repo, err := storage.NewSQLiteRepository(db)
	if err != nil {
		return nil, err
	}
	return NewSQLiteSyncStoreWithRepository(repo)
}

// NewSQLiteSyncStoreWithRepository 使用稳定的 storage 接口创建链路同步存储。
func NewSQLiteSyncStoreWithRepository(repo storage.Transactional) (*SQLiteSyncStore, error) {
	if repo == nil {
		return nil, errors.New("SQLite repository 不能为空")
	}
	if _, err := repo.ExecContext(context.Background(), `CREATE TABLE IF NOT EXISTS link_sync (stream TEXT PRIMARY KEY, version INTEGER NOT NULL, payload BLOB NOT NULL)`); err != nil {
		return nil, err
	}
	return &SQLiteSyncStore{repo: repo}, nil
}

// Pull 从 SQLite 读取指定游标之后的最新同步快照。
func (s *SQLiteSyncStore) Pull(ctx context.Context, cursor SyncCursor) ([]byte, SyncCursor, error) {
	if err := ctx.Err(); err != nil {
		return nil, SyncCursor{}, err
	}
	if err := validateStream(cursor.Stream); err != nil {
		return nil, SyncCursor{}, err
	}
	var version uint64
	var payload []byte
	err := s.repo.QueryRowContext(ctx, `SELECT version,payload FROM link_sync WHERE stream=?`, cursor.Stream).Scan(&version, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, SyncCursor{Stream: cursor.Stream}, nil
	}
	if err != nil {
		return nil, SyncCursor{}, err
	}
	next := SyncCursor{Stream: cursor.Stream, Version: version}
	if cursor.Version >= version {
		return nil, next, nil
	}
	return append([]byte(nil), payload...), next, nil
}

// Push 在 SQLite 事务中执行 compare-and-set，防止旧游标覆盖新快照。
func (s *SQLiteSyncStore) Push(ctx context.Context, cursor SyncCursor, payload []byte) (SyncCursor, error) {
	if err := ctx.Err(); err != nil {
		return SyncCursor{}, err
	}
	if err := validateStream(cursor.Stream); err != nil {
		return SyncCursor{}, err
	}
	if len(payload) > maxRequestBytes {
		return SyncCursor{}, errors.New("同步数据超过 8 MiB 限制")
	}
	var result SyncCursor
	err := s.repo.WithTx(ctx, func(tx storage.SQLExecutor) error {
		var version uint64
		var current []byte
		err := tx.QueryRowContext(ctx, `SELECT version,payload FROM link_sync WHERE stream=?`, cursor.Stream).Scan(&version, &current)
		if errors.Is(err, sql.ErrNoRows) {
			version = 0
		} else if err != nil {
			return err
		}
		if cursor.Version != version {
			if cursor.Version < version && bytes.Equal(current, payload) {
				result = SyncCursor{Stream: cursor.Stream, Version: version}
				return nil
			}
			result = SyncCursor{Stream: cursor.Stream, Version: version}
			return ErrSyncConflict
		}
		version++
		if _, err := tx.ExecContext(ctx, `INSERT INTO link_sync(stream,version,payload) VALUES(?,?,?) ON CONFLICT(stream) DO UPDATE SET version=excluded.version,payload=excluded.payload`, cursor.Stream, version, payload); err != nil {
			return err
		}
		result = SyncCursor{Stream: cursor.Stream, Version: version}
		return nil
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

var _ SyncStore = (*SQLiteSyncStore)(nil)
