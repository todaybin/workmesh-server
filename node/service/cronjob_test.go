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
