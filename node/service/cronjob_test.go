// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

func TestCronjobCreateListDelete(t *testing.T) {
	service := NewCronjobService()
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
	s := NewCronjobService()
	job, err := s.Create(context.Background(), model.Cronjob{Name: "unsafe", Type: "shell", Command: "echo ok; rm -rf /"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.HandleOnce(context.Background(), job.ID); err == nil {
		t.Fatal("包含 shell 控制字符的命令必须拒绝")
	}
}
