// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package storage 提供 WorkMesh Server 单应用共享的 SQLite 存储和版本迁移能力。
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	maxOpenConnections = 4
	maxIdleConnections = 1
	busyTimeoutMillis  = 5000
)

// Store 是全进程共享的 SQLite 存储。调用方应在应用退出时调用 Close。
type Store struct {
	db   *sql.DB
	path string
}

// Open 打开指定 SQLite 文件，配置连接池、PRAGMA，并执行内置版本迁移。
func Open(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("SQLite 路径不能为空")
	}
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("解析 SQLite 路径失败: %w", err)
	}
	if filepath.Dir(absPath) == absPath {
		return nil, errors.New("SQLite 路径不能是文件系统根目录")
	}
	if info, statErr := os.Stat(absPath); statErr == nil && info.IsDir() {
		return nil, fmt.Errorf("SQLite 路径指向目录: %s", absPath)
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("检查 SQLite 路径失败: %w", statErr)
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o750); err != nil {
		return nil, fmt.Errorf("创建 SQLite 数据目录失败: %w", err)
	}

	db, err := sql.Open("sqlite", sqliteDSN(absPath))
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 失败: %w", err)
	}
	db.SetMaxOpenConns(maxOpenConnections)
	db.SetMaxIdleConns(maxIdleConnections)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	store := &Store{db: db, path: absPath}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接 SQLite 失败: %w", err)
	}
	if err := configureDatabase(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ApplyMigrations(ctx, db, builtinMigrations()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("执行 SQLite 迁移失败: %w", err)
	}
	return store, nil
}

func sqliteDSN(path string) string {
	normalized := filepath.ToSlash(path)
	// modernc 的 URI 解析器把 file:///E:/... 误判为 authority=E:；
	// Windows 卷标使用 file:E:/...，Unix 绝对路径仍使用 file:/...。
	if volume := filepath.VolumeName(path); volume != "" {
		normalized = strings.TrimPrefix(normalized, "/")
	}
	query := url.Values{}
	// modernc SQLite 会在每条新连接上执行 _pragma，避免 foreign_keys 只对首条连接生效。
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "synchronous(NORMAL)")
	query.Add("_pragma", "foreign_keys(ON)")
	query.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeoutMillis))
	return "file:" + normalized + "?" + query.Encode()
}

func configureDatabase(ctx context.Context, db *sql.DB) error {
	statements := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
		fmt.Sprintf("PRAGMA busy_timeout = %d", busyTimeoutMillis),
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("设置 SQLite 参数失败 (%s): %w", statement, err)
		}
	}
	return nil
}

// DB 返回共享的底层连接池，供 typed repository 和事务边界使用。
func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// Path 返回 SQLite 文件的绝对路径。
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Close 关闭共享 SQLite 连接池。
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
