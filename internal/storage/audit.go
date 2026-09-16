// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package storage

import (
	"context"
	"errors"
	"time"
)

// LoginAuditEntry 描述一次登录尝试，不包含密码或原始请求体。
type LoginAuditEntry struct {
	IP        string
	User      string
	Address   string
	Agent     string
	Status    string
	Message   string
	CreatedAt time.Time
}

// OperationAuditEntry 描述一次 HTTP 操作审计，不包含原始请求体。
type OperationAuditEntry struct {
	Source      string
	User        string
	IP          string
	Node        string
	Path        string
	Method      string
	UserAgent   string
	LatencyNsec int64
	Status      string
	Message     string
	DetailZH    string
	DetailEN    string
	CreatedAt   time.Time
}

// AuditLogWriter 是认证和 HTTP transport 共用的审计写入接口。
type AuditLogWriter interface {
	RecordLoginAttempt(context.Context, LoginAuditEntry) error
	RecordOperation(context.Context, OperationAuditEntry) error
}

// SQLiteAuditLogWriter 将审计记录写入已经由启动迁移创建的 SQLite 表。
type SQLiteAuditLogWriter struct {
	db SQLExecutor
}

// NewSQLiteAuditLogWriter 创建审计日志 writer。
func NewSQLiteAuditLogWriter(db SQLExecutor) (*SQLiteAuditLogWriter, error) {
	if db == nil {
		return nil, errors.New("审计日志数据库不能为空")
	}
	return &SQLiteAuditLogWriter{db: db}, nil
}

// RecordLoginAttempt 写入登录尝试记录。
func (w *SQLiteAuditLogWriter) RecordLoginAttempt(ctx context.Context, entry LoginAuditEntry) error {
	if w == nil || w.db == nil {
		return errors.New("审计日志 writer 未初始化")
	}
	createdAt := entry.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := w.db.ExecContext(ctx, `INSERT INTO login_logs(ip,user,address,agent,status,message,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`,
		entry.IP, entry.User, entry.Address, entry.Agent, entry.Status, entry.Message,
		createdAt.Format(time.RFC3339Nano), createdAt.Format(time.RFC3339Nano))
	return err
}

// RecordOperation 写入 HTTP 操作审计记录。
func (w *SQLiteAuditLogWriter) RecordOperation(ctx context.Context, entry OperationAuditEntry) error {
	if w == nil || w.db == nil {
		return errors.New("审计日志 writer 未初始化")
	}
	createdAt := entry.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := w.db.ExecContext(ctx, `INSERT INTO operation_logs(source,user,ip,node,path,method,user_agent,latency,status,message,detail_zh,detail_en,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		entry.Source, entry.User, entry.IP, entry.Node, entry.Path, entry.Method, entry.UserAgent,
		entry.LatencyNsec, entry.Status, entry.Message, entry.DetailZH, entry.DetailEN,
		createdAt.Format(time.RFC3339Nano), createdAt.Format(time.RFC3339Nano))
	return err
}

var _ AuditLogWriter = (*SQLiteAuditLogWriter)(nil)
