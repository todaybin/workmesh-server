// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/model"
	"strings"
	"time"
)

func (s *CronjobService) Update(ctx context.Context, job model.Cronjob) (model.Cronjob, error) {
	if err := s.ensureDatabase(ctx); err != nil {
		return model.Cronjob{}, err
	}
	if job.ID == "" || strings.TrimSpace(job.Name) == "" {
		return model.Cronjob{}, errors.New("计划任务 ID 和名称不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.items[job.ID]
	if !ok {
		return model.Cronjob{}, errors.New("cronjob not found")
	}
	if strings.TrimSpace(job.Type) == "" {
		job.Type = old.Type
	}
	if !validCronType(job.Type) {
		return model.Cronjob{}, fmt.Errorf("不支持的计划任务类型: %s", job.Type)
	}
	if job.Type == "shell" && strings.TrimSpace(job.Command) == "" && strings.TrimSpace(job.Script) == "" {
		return model.Cronjob{}, errors.New("脚本或命令不能为空")
	}
	if job.Type == "curl" && strings.TrimSpace(job.URL) == "" {
		return model.Cronjob{}, errors.New("curl 任务 URL 不能为空")
	}
	if strings.TrimSpace(job.Spec) == "" {
		job.Spec = old.Spec
	}
	if job.Status == "" {
		job.Status = old.Status
	}
	job.CreatedAt = old.CreatedAt
	job.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.items[job.ID] = job
	if err := s.persistLocked(ctx, job.ID); err != nil {
		s.items[job.ID] = old
		return model.Cronjob{}, err
	}
	s.wakeScheduler()
	return job, nil
}

// SetStatus 切换计划任务启停状态。
func (s *CronjobService) SetStatus(ctx context.Context, id, status string) error {
	if err := s.ensureDatabase(ctx); err != nil {
		return err
	}
	if status != "enabled" && status != "disabled" {
		return errors.New("invalid status")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.items[id]
	if !ok {
		return errors.New("cronjob not found")
	}
	old := job
	job.Status, job.UpdatedAt = status, time.Now().UTC().Format(time.RFC3339)
	s.items[id] = job
	if err := s.persistLocked(ctx, id); err != nil {
		s.items[id] = old
		return err
	}
	s.wakeScheduler()
	return nil
}

// Records 返回任务执行记录，调用方可按需分页。
func (s *CronjobService) Records(ctx context.Context, id string) []model.CommandResult {
	if err := s.ensureDatabase(ctx); err != nil {
		return []model.CommandResult{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]model.CommandResult(nil), s.records[id]...)
}

// RecordsPage 分页读取任务记录，最多返回 200 条。
func (s *CronjobService) RecordsPage(ctx context.Context, id string, page, pageSize int) (int, []model.CommandResult) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	all := s.Records(ctx, id)
	total := len(all)
	start := (page - 1) * pageSize
	if start >= total {
		return total, []model.CommandResult{}
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return total, all[start:end]
}
func (s *CronjobService) CleanRecords(ctx context.Context, id string) error {
	if err := s.ensureDatabase(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := append([]model.CommandResult(nil), s.records[id]...)
	delete(s.records, id)
	if err := s.persistLocked(ctx, id); err != nil {
		s.records[id] = previous
		return err
	}
	return nil
}

// NextRuns 返回后续五次调度时间，供计划任务编辑器预览。
