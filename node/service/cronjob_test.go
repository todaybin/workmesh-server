// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

func newTestCronjobService(t *testing.T) *CronjobService {
	t.Helper()
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	return NewCronjobService()
}

func TestCronjobCreateListDelete(t *testing.T) {
	service := newTestCronjobService(t)
	job, err := service.Create(context.Background(), model.Cronjob{Name: "健康检查", Command: "echo ok"})
	if err != nil || job.ID == "" {
		t.Fatalf("创建计划任务失败: %+v, %v", job, err)
	}
	if len(service.List(context.Background())) != 1 {
		t.Fatal("计划任务列表数量错误")
	}
	if err := service.Delete(context.Background(), job.ID); err != nil {
		t.Fatalf("删除计划任务失败: %v", err)
	}
}

func TestNextRunCronExpression(t *testing.T) {
	from := time.Date(2026, 8, 30, 10, 1, 0, 0, time.UTC)
	next, ok := NextRun("*/5 * * * *", from)
	if !ok || !next.Equal(time.Date(2026, 8, 30, 10, 5, 0, 0, time.UTC)) {
		t.Fatalf("unexpected next run: %v %v", next, ok)
	}
	if _, ok := NextRun("invalid", from); ok {
		t.Fatal("invalid expression should fail")
	}
}

func TestNextRunsSupportsEveryAndMacros(t *testing.T) {
	from := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	runs, err := NextRuns("@every 2m", from, 3)
	if err != nil || len(runs) != 3 || !runs[0].Equal(from.Add(2*time.Minute)) {
		t.Fatalf("@every 解析异常: %v %+v", err, runs)
	}
	if _, err := NextRuns("@every invalid", from, 1); err == nil {
		t.Fatal("无效 @every 应返回错误")
	}
}

func TestCronjobRejectsUnsafeCommand(t *testing.T) {
	s := newTestCronjobService(t)
	job, err := s.Create(context.Background(), model.Cronjob{Name: "unsafe", Type: "shell", Command: "echo ok; rm -rf /"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.HandleOnce(context.Background(), job.ID); err == nil {
		t.Fatal("包含 shell 控制字符的命令必须拒绝")
	}
}

func TestCronjobStartIsIdempotent(t *testing.T) {
	s := newTestCronjobService(t)
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	s.Start(ctx)
	s.startMu.Lock()
	started := s.started
	s.startMu.Unlock()
	if !started {
		t.Fatal("调度器首次启动后应保持运行状态")
	}
	cancel()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.startMu.Lock()
		running := s.started
		s.startMu.Unlock()
		if !running {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("调度器取消后未释放运行标记")
}

func TestCronjobHandleOnceRollsBackWhenPersistenceFails(t *testing.T) {
	s := newTestCronjobService(t)
	job, err := s.Create(context.Background(), model.Cronjob{Name: "持久化失败", Command: "echo ok"})
	if err != nil {
		t.Fatal(err)
	}

	s.mu.RLock()
	db := s.db
	s.mu.RUnlock()
	if db == nil {
		t.Fatal("计划任务数据库未初始化")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		fallbackSQLiteMu.Lock()
		delete(fallbackSQLiteDB, s.fallbackPath)
		fallbackSQLiteMu.Unlock()
	})

	if _, err := s.HandleOnce(context.Background(), job.ID); err == nil {
		t.Fatal("计划任务执行结果持久化失败时必须返回错误")
	}
	if got := s.Records(context.Background(), job.ID); len(got) != 0 {
		t.Fatalf("持久化失败后执行记录未回滚: %+v", got)
	}
	restored, ok := s.Get(context.Background(), job.ID)
	if !ok || restored.LastRunAt != "" {
		t.Fatalf("持久化失败后任务状态未回滚: %+v exists=%v", restored, ok)
	}

}
