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
	"io"
	"os"
	"path/filepath"
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

// createMigrationMetadataBootstrapSQL 只预创建审计表，不创建索引。
// 首个 0001 迁移随后会按原始不可变 SQL 创建索引，避免改变其 checksum。
const createMigrationMetadataBootstrapSQL = `
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
)`

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
	// 迁移审计表先行创建，确保首次安装的元数据迁移也能记录状态。
	if _, err := db.ExecContext(ctx, createMigrationMetadataBootstrapSQL); err != nil {
		return fmt.Errorf("初始化迁移审计表失败: %w", err)
	}
	previous := ""
	changed := false
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
			previous = migration.ID
			continue
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("读取迁移账本失败: %w", err)
		}

		runID, err := startMigrationRun(ctx, db, previous, migration.ID)
		if err != nil {
			return fmt.Errorf("记录迁移 %s 启动状态失败: %w", migration.ID, err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			_ = finishMigrationRun(ctx, db, runID, "failed", err.Error())
			return fmt.Errorf("开始迁移事务失败: %w", err)
		}
		if err := migration.Up(ctx, tx); err != nil {
			_ = tx.Rollback()
			_ = finishMigrationRun(ctx, db, runID, "failed", err.Error())
			return fmt.Errorf("执行迁移 %s 失败: %w", migration.ID, err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations(id, checksum, applied_at) VALUES(?, ?, ?)",
			migration.ID, migration.Checksum, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			_ = finishMigrationRun(ctx, db, runID, "failed", err.Error())
			return fmt.Errorf("记录迁移 %s 失败: %w", migration.ID, err)
		}
		if err := tx.Commit(); err != nil {
			_ = finishMigrationRun(ctx, db, runID, "failed", err.Error())
			return fmt.Errorf("提交迁移 %s 失败: %w", migration.ID, err)
		}
		if err := finishMigrationRun(ctx, db, runID, "applied", ""); err != nil {
			return fmt.Errorf("记录迁移 %s 完成状态失败: %w", migration.ID, err)
		}
		previous = migration.ID
		changed = true
	}
	if len(migrations) > 0 && !changed {
		// 已全部应用时也记录启动审计，明确本次没有待应用迁移。
		runID, err := startMigrationRun(ctx, db, previous, previous)
		if err != nil {
			return fmt.Errorf("记录迁移 noop 状态失败: %w", err)
		}
		if err := finishMigrationRun(ctx, db, runID, "noop", ""); err != nil {
			return fmt.Errorf("记录迁移 noop 完成状态失败: %w", err)
		}
	}
	return nil
}

// startMigrationRun 写入迁移开始审计，制品和备份信息由部署流程通过环境变量传入。
func startMigrationRun(ctx context.Context, db *sql.DB, fromVersion, toVersion string) (int64, error) {
	result, err := db.ExecContext(ctx, `INSERT INTO migration_runs(from_version,to_version,artifact_sha256,status,backup_path,error,started_at) VALUES(?,?,?,?,?,?,?)`, fromVersion, toVersion, migrationArtifactSHA256(), "running", migrationBackupPath(), "", time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// migrationArtifactSHA256 返回发布流程传入的制品哈希；未传入时读取当前
// 可执行文件，避免升级审计留下空的制品身份。
func migrationArtifactSHA256() string {
	if value := strings.TrimSpace(os.Getenv("WORKMESH_ARTIFACT_SHA256")); value != "" {
		return value
	}
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	file, err := os.Open(executable)
	if err != nil {
		return ""
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ""
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// migrationBackupPath 返回部署流程配置的备份路径；未配置时使用服务安装目录
// 下的 backups 目录，保证升级审计至少指向受控的回滚根目录。
func migrationBackupPath() string {
	if value := strings.TrimSpace(os.Getenv("WORKMESH_MIGRATION_BACKUP_PATH")); value != "" {
		return value
	}
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "backups"))
}

// finishMigrationRun 结束迁移审计并保留失败根因，供升级复盘使用。
func finishMigrationRun(ctx context.Context, db *sql.DB, id int64, status, runErr string) error {
	_, err := db.ExecContext(ctx, `UPDATE migration_runs SET status=?,error=?,finished_at=? WHERE id=?`, status, runErr, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
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
