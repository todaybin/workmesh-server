// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const createSchemaMigrationsSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    id TEXT PRIMARY KEY,
    checksum TEXT NOT NULL,
    applied_at TEXT NOT NULL
)`

const createMigrationMetadataSQL = `
CREATE TABLE IF NOT EXISTS migration_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_version TEXT NOT NULL DEFAULT '',
    to_version TEXT NOT NULL,
    artifact_sha256 TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    backup_path TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    finished_at TEXT
);
CREATE INDEX idx_migration_runs_target_status ON migration_runs(to_version, status);

CREATE TABLE IF NOT EXISTS legacy_imports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_type TEXT NOT NULL,
    source_path TEXT NOT NULL,
    source_sha256 TEXT NOT NULL,
    domain TEXT NOT NULL,
    status TEXT NOT NULL,
    imported_count INTEGER NOT NULL DEFAULT 0,
    report_json TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    finished_at TEXT,
    UNIQUE(source_type, source_path, source_sha256, domain)
);
CREATE INDEX idx_legacy_imports_status ON legacy_imports(status, domain)`

// ErrMigrationChecksumMismatch 表示已应用迁移的内容被修改，继续启动可能破坏数据库。
var ErrMigrationChecksumMismatch = errors.New("迁移 checksum 冲突")

// Migration 描述一个具有不可变 ID 和 checksum 的数据库迁移。
type Migration struct {
	ID       string
	Checksum string
	Up       func(context.Context, *sql.Tx) error
}

// SQLMigration 根据规范化 SQL 创建版本迁移。
func SQLMigration(id, statement string) Migration {
	canonical := strings.TrimSpace(statement)
	sum := sha256.Sum256([]byte(canonical))
	return Migration{
		ID:       id,
		Checksum: hex.EncodeToString(sum[:]),
		Up: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, canonical)
			return err
		},
	}
}

func builtinMigrations() []Migration {
	return []Migration{
		SQLMigration("0001-migration-metadata", createMigrationMetadataSQL),
	}
}

// ApplyMigrations 按给定顺序执行尚未应用的迁移，并拒绝已应用 ID 的 checksum 变化。
func ApplyMigrations(ctx context.Context, db *sql.DB, migrations []Migration) error {
	if db == nil {
		return errors.New("SQLite 连接不能为空")
	}
	if _, err := db.ExecContext(ctx, createSchemaMigrationsSQL); err != nil {
		return fmt.Errorf("初始化迁移账本失败: %w", err)
	}
	for _, migration := range migrations {
		if err := validateMigration(migration); err != nil {
			return err
		}
		var checksum string
		err := db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE id = ?", migration.ID).Scan(&checksum)
		switch {
		case err == nil:
			if checksum != migration.Checksum {
				return fmt.Errorf("%w: id=%s stored=%s current=%s", ErrMigrationChecksumMismatch, migration.ID, checksum, migration.Checksum)
			}
			continue
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("读取迁移账本失败: %w", err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("开始迁移事务失败: %w", err)
		}
		if err := migration.Up(ctx, tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("执行迁移 %s 失败: %w", migration.ID, err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations(id, checksum, applied_at) VALUES(?, ?, ?)",
			migration.ID, migration.Checksum, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("记录迁移 %s 失败: %w", migration.ID, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("提交迁移 %s 失败: %w", migration.ID, err)
		}
	}
	return nil
}

func validateMigration(migration Migration) error {
	if strings.TrimSpace(migration.ID) == "" {
		return errors.New("迁移 ID 不能为空")
	}
	if strings.TrimSpace(migration.Checksum) == "" {
		return fmt.Errorf("迁移 %s 的 checksum 不能为空", migration.ID)
	}
	if migration.Up == nil {
		return fmt.Errorf("迁移 %s 缺少执行函数", migration.ID)
	}
	return nil
}
