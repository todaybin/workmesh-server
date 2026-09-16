// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestSQLiteRepositoryTransactionCommitAndRollback(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repo, err := store.Repository()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ExecContext(context.Background(), `CREATE TABLE repository_test (id INTEGER PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := repo.WithTx(context.Background(), func(tx SQLExecutor) error {
		_, err := tx.ExecContext(context.Background(), `INSERT INTO repository_test(id,value) VALUES(?,?)`, 1, "committed")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	rollbackErr := errors.New("controlled rollback")
	if err := repo.WithTx(context.Background(), func(tx SQLExecutor) error {
		if _, err := tx.ExecContext(context.Background(), `INSERT INTO repository_test(id,value) VALUES(?,?)`, 2, "rolled-back"); err != nil {
			return err
		}
		return rollbackErr
	}); !errors.Is(err, rollbackErr) {
		t.Fatalf("事务错误 = %v, want %v", err, rollbackErr)
	}
	var count int
	if err := repo.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM repository_test`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("事务回滚后行数 = %d, want 1", count)
	}
}

func TestSQLiteRepositoryRejectsInvalidConstruction(t *testing.T) {
	if _, err := NewSQLiteRepository(nil); err == nil {
		t.Fatal("nil 数据库应被拒绝")
	}
	var repo *SQLiteRepository
	if err := repo.WithTx(context.Background(), func(SQLExecutor) error { return nil }); err == nil {
		t.Fatal("nil repository 应被拒绝")
	}
	if err := (&SQLiteRepository{}).WithTx(context.Background(), nil); err == nil {
		t.Fatal("nil 事务函数应被拒绝")
	}
}

func TestSQLiteRepositoryRollsBackWhenTransactionCallbackPanics(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repo, err := store.Repository()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ExecContext(context.Background(), `CREATE TABLE repository_panic_test (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("事务回调 panic 应继续向调用方传播")
			}
		}()
		_ = repo.WithTx(context.Background(), func(tx SQLExecutor) error {
			if _, err := tx.ExecContext(context.Background(), `INSERT INTO repository_panic_test(id) VALUES(1)`); err != nil {
				return err
			}
			panic("controlled transaction panic")
		})
	}()
	var count int
	if err := repo.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM repository_panic_test`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("panic 后事务未回滚，行数 = %d", count)
	}
}
