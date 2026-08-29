// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"sync"

	"github.com/todaybin/workmesh-server/node/model"
)

// CronjobService 保存计划任务定义并提供立即执行适配；持久化迁移将在下一批完成。
type CronjobService struct {
	mu    sync.RWMutex
	items map[string]model.Cronjob
	cmd   CommandService
}

// NewCronjobService 创建计划任务服务。
func NewCronjobService() *CronjobService {
	return &CronjobService{items: make(map[string]model.Cronjob)}
}

// Create 新增计划任务，初始状态为 disabled。
func (s *CronjobService) Create(_ context.Context, job model.Cronjob) (model.Cronjob, error) {
	if strings.TrimSpace(job.Name) == "" || strings.TrimSpace(job.Command) == "" {
		return model.Cronjob{}, errors.New("计划任务名称和命令不能为空")
	}
	job.ID = newID()
	job.Status = "disabled"
	s.mu.Lock()
	s.items[job.ID] = job
	s.mu.Unlock()
	return job, nil
}

// List 返回全部计划任务定义。
func (s *CronjobService) List(context.Context) []model.Cronjob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]model.Cronjob, 0, len(s.items))
	for _, item := range s.items {
		result = append(result, item)
	}
	return result
}

// Delete 删除指定计划任务。
func (s *CronjobService) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return errors.New("计划任务不存在")
	}
	delete(s.items, id)
	return nil
}

// HandleOnce 立即执行计划任务的命令字段。
func (s *CronjobService) HandleOnce(ctx context.Context, id string) (model.CommandResult, error) {
	s.mu.RLock()
	job, ok := s.items[id]
	s.mu.RUnlock()
	if !ok {
		return model.CommandResult{}, errors.New("计划任务不存在")
	}
	if runtime.GOOS == "windows" {
		return s.cmd.Execute(ctx, model.CommandRequest{Program: "cmd", Args: []string{"/C", job.Command}})
	}
	return s.cmd.Execute(ctx, model.CommandRequest{Program: "sh", Args: []string{"-c", job.Command}})
}

// Export 返回稳定 JSON，供旧版导入导出接口适配。
func (s *CronjobService) Export(context.Context) ([]byte, error) {
	return json.Marshal(map[string]any{"cronjobs": s.List(context.Background())})
}

// Import 导入 JSON 中的 cronjobs 数组。
func (s *CronjobService) Import(ctx context.Context, payload []byte) error {
	var envelope struct {
		Cronjobs []model.Cronjob `json:"cronjobs"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return err
	}
	for _, job := range envelope.Cronjobs {
		if _, err := s.Create(ctx, job); err != nil {
			return err
		}
	}
	return nil
}

func newID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "local-cronjob"
	}
	return hex.EncodeToString(raw[:])
}
