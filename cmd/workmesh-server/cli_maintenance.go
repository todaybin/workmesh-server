// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

var historyTables = []string{
	"operation_logs", "login_logs", "app_install_tasks", "runtime_tasks", "runtime_task_logs",
	"database_operations", "database_backups", "migration_runs",
}

func handleCLIMaintenance(args []string, dataDir string) error {
	if len(args) != 2 || args[0] != "reset-history" || args[1] != "--confirm-delete-history" {
		return errors.New("拒绝删除历史数据：必须使用 maintenance reset-history --confirm-delete-history")
	}
	return resetHistory(dataDir)
}

func resetHistory(dataDir string) error {
	root, err := filepath.Abs(filepath.Clean(dataDir))
	if err != nil || root == string(filepath.Separator) || filepath.Dir(root) == root {
		return errors.New("数据目录无效，拒绝执行历史清理")
	}
	dbPath := filepath.Join(root, "workmesh.db")
	if info, statErr := os.Stat(dbPath); statErr != nil || !info.Mode().IsRegular() {
		if statErr != nil {
			return fmt.Errorf("数据库不可用: %w", statErr)
		}
		return errors.New("数据库路径不是普通文件")
	}
	store, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	defer store.Close()
	db := store.DB()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	before, err := historyCounts(ctx, db)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	for _, table := range historyTables {
		if _, exists := before[table]; !exists {
			continue
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("清空 %s 失败: %w", table, err)
		}
	}
	if err := removeJSONFields(ctx, tx, "functional_domain_state", "logs"); err != nil {
		return err
	}
	if err := removeJSONFields(ctx, tx, "app_store_state", "catalog", "catalogVersion", "catalogLastModified", "catalogSyncing", "catalogSyncedAt", "catalogTags"); err != nil {
		return err
	}
	if exists, err := sqliteTableExists(ctx, tx, "app_catalog_cache"); err != nil {
		return err
	} else if exists {
		if _, err := tx.ExecContext(ctx, `DELETE FROM app_catalog_cache`); err != nil {
			return err
		}
	}
	if exists, err := sqliteTableExists(ctx, tx, "cronjobs"); err != nil {
		return err
	} else if exists {
		if _, err := tx.ExecContext(ctx, `UPDATE cronjobs SET records='[]', updated_at=?`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false

	taskLogs := filepath.Join(root, "logs", "tasks")
	if !pathWithin(root, taskLogs) {
		return errors.New("任务日志路径越界")
	}
	if err := os.RemoveAll(taskLogs); err != nil {
		return fmt.Errorf("删除任务日志失败: %w", err)
	}
	if err := os.MkdirAll(taskLogs, 0o750); err != nil {
		return err
	}
	serverLog := filepath.Join(root, "logs", "server.log")
	if !pathWithin(root, serverLog) {
		return errors.New("服务日志路径越界")
	}
	if err := os.MkdirAll(filepath.Dir(serverLog), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(serverLog, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("截断服务日志失败: %w", err)
	}
	if err := file.Close(); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("WAL checkpoint 失败: %w", err)
	}
	if _, err := db.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("VACUUM 失败: %w", err)
	}
	after, err := historyCounts(ctx, db)
	if err != nil {
		return err
	}
	for _, table := range historyTables {
		if oldCount, ok := before[table]; ok {
			fmt.Printf("%s: %d -> %d\n", table, oldCount, after[table])
		}
	}
	return nil
}

func removeJSONFields(ctx context.Context, tx *sql.Tx, table string, fields ...string) error {
	exists, err := sqliteTableExists(ctx, tx, table)
	if err != nil || !exists {
		return err
	}
	var payload []byte
	err = tx.QueryRowContext(ctx, "SELECT payload FROM "+table+" WHERE id=1").Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		return fmt.Errorf("解析 %s 失败: %w", table, err)
	}
	for _, field := range fields {
		delete(document, field)
	}
	updated, err := json.Marshal(document)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE "+table+" SET payload=?,updated_at=? WHERE id=1", updated, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

type sqlQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func sqliteTableExists(ctx context.Context, db sqlQueryer, table string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count)
	return count > 0, err
}

func historyCounts(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	result := make(map[string]int64)
	for _, table := range historyTables {
		exists, err := sqliteTableExists(ctx, db, table)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		var count int64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return nil, err
		}
		result[table] = count
	}
	return result, nil
}

func pathWithin(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
