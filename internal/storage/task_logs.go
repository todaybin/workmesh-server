// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package storage

import (
	"context"
	"errors"
	"strings"
	"time"
)

// TaskLogEntry 描述一行任务输出。
type TaskLogEntry struct {
	TaskID    string
	Line      string
	CreatedAt time.Time
}

// TaskLogWriter 是应用、运行时和容器任务共用的日志写入接口。
type TaskLogWriter interface {
	Append(context.Context, TaskLogEntry) error
}

// AppTaskState 描述应用安装任务的持久化状态。
type AppTaskState struct {
	ID        string
	InstallID string
	Status    string
	Step      string
	Progress  int
	Message   string
	Error     string
	LogPath   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// RuntimeTaskState 描述运行时任务的持久化状态。
type RuntimeTaskState struct {
	ID        string
	RuntimeID string
	Status    string
	Step      string
	Progress  int
	Message   string
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TaskStateWriter 是应用和运行时任务状态的统一持久化接口。
type TaskStateWriter interface {
	UpsertApp(context.Context, AppTaskState) error
	UpsertRuntime(context.Context, RuntimeTaskState) error
}

// SQLiteTaskLogWriter 将任务输出写入统一 runtime_task_logs 表。
type SQLiteTaskLogWriter struct {
	db SQLExecutor
}

// NewSQLiteTaskLogWriter 创建任务日志 writer。
func NewSQLiteTaskLogWriter(db SQLExecutor) (*SQLiteTaskLogWriter, error) {
	if db == nil {
		return nil, errors.New("任务日志数据库不能为空")
	}
	return &SQLiteTaskLogWriter{db: db}, nil
}

// Append 追加一行任务日志；空任务或空行不写入数据库。
func (w *SQLiteTaskLogWriter) Append(ctx context.Context, entry TaskLogEntry) error {
	if w == nil || w.db == nil {
		return errors.New("任务日志 writer 未初始化")
	}
	if strings.TrimSpace(entry.TaskID) == "" || strings.TrimSpace(entry.Line) == "" {
		return nil
	}
	createdAt := entry.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := w.db.ExecContext(ctx, `INSERT INTO runtime_task_logs(task_id,line,created_at) VALUES(?,?,?)`,
		strings.TrimSpace(entry.TaskID), strings.TrimSpace(entry.Line), createdAt.Format(time.RFC3339Nano))
	return err
}

var _ TaskLogWriter = (*SQLiteTaskLogWriter)(nil)

// SQLiteTaskStateWriter 将应用和运行时任务状态写入各自的 SQLite 表。
type SQLiteTaskStateWriter struct {
	db SQLExecutor
}

// NewSQLiteTaskStateWriter 创建任务状态 writer。
func NewSQLiteTaskStateWriter(db SQLExecutor) (*SQLiteTaskStateWriter, error) {
	if db == nil {
		return nil, errors.New("任务状态数据库不能为空")
	}
	return &SQLiteTaskStateWriter{db: db}, nil
}

// UpsertApp 保存应用安装任务的最新状态。
func (w *SQLiteTaskStateWriter) UpsertApp(ctx context.Context, state AppTaskState) error {
	if w == nil || w.db == nil {
		return errors.New("任务状态 writer 未初始化")
	}
	if strings.TrimSpace(state.ID) == "" {
		return errors.New("应用任务 ID 不能为空")
	}
	createdAt, updatedAt := taskTimes(state.CreatedAt, state.UpdatedAt)
	_, err := w.db.ExecContext(ctx, `INSERT INTO app_install_tasks(id,app_install_id,status,step,progress,message,error,log_path,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		app_install_id=excluded.app_install_id,status=excluded.status,step=excluded.step,
		progress=excluded.progress,message=excluded.message,error=excluded.error,
		log_path=excluded.log_path,updated_at=excluded.updated_at`,
		state.ID, state.InstallID, state.Status, state.Step, state.Progress, state.Message,
		state.Error, state.LogPath, createdAt.Format(time.RFC3339Nano), updatedAt.Format(time.RFC3339Nano))
	return err
}

// UpsertRuntime 保存运行时任务的最新状态。
func (w *SQLiteTaskStateWriter) UpsertRuntime(ctx context.Context, state RuntimeTaskState) error {
	if w == nil || w.db == nil {
		return errors.New("任务状态 writer 未初始化")
	}
	if strings.TrimSpace(state.ID) == "" {
		return errors.New("运行时任务 ID 不能为空")
	}
	createdAt, updatedAt := taskTimes(state.CreatedAt, state.UpdatedAt)
	_, err := w.db.ExecContext(ctx, `INSERT INTO runtime_tasks(id,runtime_id,status,step,progress,message,error,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		runtime_id=excluded.runtime_id,status=excluded.status,step=excluded.step,
		progress=excluded.progress,message=excluded.message,error=excluded.error,
		updated_at=excluded.updated_at`,
		state.ID, state.RuntimeID, state.Status, state.Step, state.Progress, state.Message,
		state.Error, createdAt.Format(time.RFC3339Nano), updatedAt.Format(time.RFC3339Nano))
	return err
}

func taskTimes(createdAt, updatedAt time.Time) (time.Time, time.Time) {
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	return createdAt.UTC(), updatedAt.UTC()
}

var _ TaskStateWriter = (*SQLiteTaskStateWriter)(nil)
