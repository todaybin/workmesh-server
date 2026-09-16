// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteAuditAndTaskWritersPersistContractFields(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.DB().Exec(`CREATE TABLE login_logs (id INTEGER PRIMARY KEY AUTOINCREMENT, ip TEXT, user TEXT, address TEXT, agent TEXT, status TEXT, message TEXT, created_at TEXT, updated_at TEXT);
CREATE TABLE operation_logs (id INTEGER PRIMARY KEY AUTOINCREMENT, source TEXT, user TEXT, ip TEXT, node TEXT, path TEXT, method TEXT, user_agent TEXT, latency INTEGER, status TEXT, message TEXT, detail_zh TEXT, detail_en TEXT, created_at TEXT, updated_at TEXT);
CREATE TABLE runtime_task_logs (id INTEGER PRIMARY KEY AUTOINCREMENT, task_id TEXT, line TEXT, created_at TEXT);
CREATE TABLE app_install_tasks (id TEXT PRIMARY KEY, app_install_id TEXT, status TEXT, step TEXT, progress INTEGER, message TEXT, error TEXT, log_path TEXT, created_at TEXT, updated_at TEXT);
CREATE TABLE runtime_tasks (id TEXT PRIMARY KEY, runtime_id TEXT, status TEXT, step TEXT, progress INTEGER, message TEXT, error TEXT, created_at TEXT, updated_at TEXT)`)
	if err != nil {
		t.Fatal(err)
	}
	audit, err := NewSQLiteAuditLogWriter(store.DB())
	if err != nil {
		t.Fatal(err)
	}
	task, err := NewSQLiteTaskLogWriter(store.DB())
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewSQLiteTaskStateWriter(store.DB())
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, 9, 11, 1, 2, 3, 0, time.UTC)
	ctx := context.Background()
	if err := audit.RecordLoginAttempt(ctx, LoginAuditEntry{IP: "192.0.2.1", User: "admin", Address: "test", Agent: "browser", Status: "Success", Message: "ok", CreatedAt: createdAt}); err != nil {
		t.Fatal(err)
	}
	if err := audit.RecordOperation(ctx, OperationAuditEntry{Source: "websites", User: "admin", IP: "192.0.2.1", Node: "local", Path: "/websites", Method: "post", UserAgent: "browser", LatencyNsec: 123, Status: "Success", Message: "ok", DetailZH: "完成", DetailEN: "done", CreatedAt: createdAt}); err != nil {
		t.Fatal(err)
	}
	if err := task.Append(ctx, TaskLogEntry{TaskID: "task-1", Line: "[TASK-END]", CreatedAt: createdAt}); err != nil {
		t.Fatal(err)
	}
	if err := state.UpsertApp(ctx, AppTaskState{ID: "app-task-1", InstallID: "app-1", Status: "running", Step: "starting", Progress: 80, Message: "ok", CreatedAt: createdAt, UpdatedAt: createdAt}); err != nil {
		t.Fatal(err)
	}
	if err := state.UpsertRuntime(ctx, RuntimeTaskState{ID: "runtime-task-1", RuntimeID: "runtime-1", Status: "running", Step: "starting", Progress: 80, Message: "ok", CreatedAt: createdAt, UpdatedAt: createdAt}); err != nil {
		t.Fatal(err)
	}
	var loginCount, operationCount, taskCount, appTaskCount, runtimeTaskCount int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM login_logs`).Scan(&loginCount); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM operation_logs`).Scan(&operationCount); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM runtime_task_logs`).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM app_install_tasks`).Scan(&appTaskCount); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM runtime_tasks`).Scan(&runtimeTaskCount); err != nil {
		t.Fatal(err)
	}
	if loginCount != 1 || operationCount != 1 || taskCount != 1 || appTaskCount != 1 || runtimeTaskCount != 1 {
		t.Fatalf("写入数量异常: login=%d operation=%d task=%d appTask=%d runtimeTask=%d", loginCount, operationCount, taskCount, appTaskCount, runtimeTaskCount)
	}
}
