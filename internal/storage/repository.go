// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package storage

import (
	"context"
	"database/sql"
	"errors"
)

// SQLExecutor 是业务用例访问 SQLite 的最小查询/写入接口。
// *sql.DB 和 *sql.Tx 都满足该接口，因此迁移期间可以逐步替换直接依赖。
type SQLExecutor interface {
	Exec(string, ...any) (sql.Result, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	Query(string, ...any) (*sql.Rows, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Transactional 在不暴露连接池和事务具体类型的前提下提供事务边界。
// 回调返回错误时自动回滚，只有回调成功才提交事务。
type Transactional interface {
	SQLExecutor
	WithTx(context.Context, func(SQLExecutor) error) error
}

// SQLiteRepository 将共享 SQLite 连接池适配为稳定的业务存储接口。
// 具体的 SQL 仍由各业务 repository 负责，避免 storage 包承载领域规则。
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository 创建一个共享 SQLite repository。
func NewSQLiteRepository(db *sql.DB) (*SQLiteRepository, error) {
	if db == nil {
		return nil, errors.New("SQLite 数据库不能为空")
	}
	return &SQLiteRepository{db: db}, nil
}

// DB 返回底层连接池，仅供启动组装、迁移和尚未迁移的兼容代码使用。
// 新业务代码应优先依赖 SQLExecutor 或 Transactional。
func (r *SQLiteRepository) DB() *sql.DB {
	if r == nil {
		return nil
	}
	return r.db
}

// ExecContext 执行一条带上下文的 SQL 写操作。
func (r *SQLiteRepository) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("SQLite repository 未初始化")
	}
	return r.db.ExecContext(ctx, query, args...)
}

// Exec 执行一条无上下文的 SQL 写操作，兼容尚未迁移的旧调用方。
func (r *SQLiteRepository) Exec(query string, args ...any) (sql.Result, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("SQLite repository 未初始化")
	}
	return r.db.Exec(query, args...)
}

// QueryContext 执行一条带上下文的 SQL 查询。
func (r *SQLiteRepository) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("SQLite repository 未初始化")
	}
	return r.db.QueryContext(ctx, query, args...)
}

// Query 执行一条无上下文的 SQL 查询，兼容尚未迁移的旧调用方。
func (r *SQLiteRepository) Query(query string, args ...any) (*sql.Rows, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("SQLite repository 未初始化")
	}
	return r.db.Query(query, args...)
}

// QueryRowContext 执行一条带上下文的单行查询。
func (r *SQLiteRepository) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if r == nil || r.db == nil {
		return &sql.Row{}
	}
	return r.db.QueryRowContext(ctx, query, args...)
}

// QueryRow 执行一条无上下文的单行查询，兼容尚未迁移的旧调用方。
func (r *SQLiteRepository) QueryRow(query string, args ...any) *sql.Row {
	if r == nil || r.db == nil {
		return &sql.Row{}
	}
	return r.db.QueryRow(query, args...)
}

// WithTx 在共享 SQLite 连接池上执行一个原子事务闭包。
func (r *SQLiteRepository) WithTx(ctx context.Context, fn func(SQLExecutor) error) (err error) {
	if r == nil || r.db == nil {
		return errors.New("SQLite repository 未初始化")
	}
	if fn == nil {
		return errors.New("SQLite 事务函数不能为空")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	err = tx.Commit()
	if err == nil {
		committed = true
	}
	return err
}

var _ SQLExecutor = (*SQLiteRepository)(nil)
var _ Transactional = (*SQLiteRepository)(nil)
