// SPDX-License-Identifier: LicenseRef-WorkMesh-Pending
// Copyright (c) 2026 WorkMesh contributors

package schedule

import (
	"context"
	"sync"
	"time"
)

// Task 是一个可停止的周期任务。
type Task struct {
	Name     string
	Interval time.Duration
	Run      func(context.Context) error
}

// Scheduler 管理全进程唯一的任务调度器。
type Scheduler struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New 创建尚未启动的调度器。
func New() *Scheduler { return &Scheduler{} }

// Start 启动任务；任务函数失败不会终止调度器。
func (s *Scheduler) Start(parent context.Context, tasks ...Task) {
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.mu.Unlock()
	for _, task := range tasks {
		if task.Interval <= 0 || task.Run == nil {
			continue
		}
		t := task
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(t.Interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = t.Run(ctx)
				}
			}
		}()
	}
}

// Stop 停止全部任务并等待退出。
func (s *Scheduler) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		s.wg.Wait()
	}
}
