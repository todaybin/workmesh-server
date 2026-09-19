// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

func (s *CronjobService) Start(parent context.Context) {
	s.startMu.Lock()
	if s.started {
		s.startMu.Unlock()
		return
	}
	s.started = true
	s.startMu.Unlock()
	go func() {
		defer func() {
			s.startMu.Lock()
			s.started = false
			s.startMu.Unlock()
		}()
		for {
			next, jobIDs, ok := s.nextEnabledRun(time.Now().UTC())
			if !ok {
				select {
				case <-parent.Done():
					return
				case <-s.wake:
					continue
				}
			}
			timer := time.NewTimer(time.Until(next))
			select {
			case <-parent.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-s.wake:
				if !timer.Stop() {
					<-timer.C
				}
			case <-timer.C:
				s.runScheduled(parent, jobIDs, next)
			}
		}
	}()
}

func (s *CronjobService) nextEnabledRun(from time.Time) (time.Time, []string, bool) {
	if err := s.ensureDatabase(context.Background()); err != nil {
		return time.Time{}, nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var earliest time.Time
	jobIDs := []string{}
	for _, job := range s.items {
		if job.Status != "enabled" {
			continue
		}
		next, ok := nextCronRun(job.Spec, from)
		if !ok {
			continue
		}
		if earliest.IsZero() || next.Before(earliest) {
			earliest = next
			jobIDs = []string{job.ID}
		} else if next.Equal(earliest) {
			jobIDs = append(jobIDs, job.ID)
		}
	}
	return earliest, jobIDs, !earliest.IsZero()
}

func (s *CronjobService) runScheduled(parent context.Context, jobIDs []string, scheduledAt time.Time) {
	tick := scheduledAt.UTC().Format(time.RFC3339Nano)
	for _, id := range jobIDs {
		s.mu.Lock()
		job, exists := s.items[id]
		if !exists || job.Status != "enabled" || s.lastTick[id] == tick {
			s.mu.Unlock()
			continue
		}
		s.lastTick[id] = tick
		ctx, cancel := context.WithCancel(parent)
		s.running[id] = cancel
		s.mu.Unlock()
		go func(jobID string) {
			defer func() {
				s.mu.Lock()
				delete(s.running, jobID)
				s.mu.Unlock()
			}()
			_, _ = s.HandleOnce(ctx, jobID)
		}(id)
	}
}

// runDue 执行当前分钟到期且尚未执行的启用任务，避免同一分钟重复调度。
func (s *CronjobService) runDue(parent context.Context) {
	// 调度器可能在主进程注入共享库前启动，因此每轮先懒加载一次数据库。
	if err := s.ensureDatabase(parent); err != nil {
		return
	}
	now := time.Now().UTC()
	minute := now.Truncate(time.Minute).Format(time.RFC3339)
	s.mu.RLock()
	jobs := make([]model.Cronjob, 0, len(s.items))
	for _, job := range s.items {
		if job.Status == "enabled" && cronMatches(job.Spec, now) && s.lastTick[job.ID] != minute {
			jobs = append(jobs, job)
		}
	}
	s.mu.RUnlock()
	for _, job := range jobs {
		s.mu.Lock()
		if s.lastTick[job.ID] == minute {
			s.mu.Unlock()
			continue
		}
		s.lastTick[job.ID] = minute
		s.mu.Unlock()
		ctx, cancel := context.WithCancel(parent)
		s.mu.Lock()
		s.running[job.ID] = cancel
		s.mu.Unlock()
		go func(id string) {
			defer func() {
				s.mu.Lock()
				delete(s.running, id)
				s.mu.Unlock()
			}()
			_, _ = s.HandleOnce(ctx, id)
		}(job.ID)
	}
}

// Stop 取消指定任务的正在执行实例；不存在时返回错误。
func (s *CronjobService) Stop(id string) error {
	s.mu.Lock()
	cancel, ok := s.running[id]
	s.mu.Unlock()
	if !ok {
		return errors.New("计划任务当前未运行")
	}
	cancel()
	return nil
}

// NextRun 根据标准五字段 cron 表达式计算未来一年内的下一次执行时间。
func NextRun(spec string, from time.Time) (time.Time, bool) { return nextCronRun(spec, from) }

func cronMatches(spec string, t time.Time) bool {
	next, ok := nextCronRun(spec, t.Add(-time.Second))
	return ok && next.Truncate(time.Minute).Equal(t.Truncate(time.Minute))
}

func nextCronRun(spec string, from time.Time) (time.Time, bool) {
	spec = strings.TrimSpace(spec)
	if strings.HasPrefix(spec, "@every ") {
		d, err := time.ParseDuration(strings.TrimSpace(strings.TrimPrefix(spec, "@every ")))
		if err != nil || d <= 0 {
			return time.Time{}, false
		}
		return from.Add(d), true
	}
	if spec == "@hourly" {
		return from.Truncate(time.Hour).Add(time.Hour), true
	}
	if spec == "@daily" {
		n := from.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
		return n, true
	}
	if spec == "@weekly" {
		n := from.UTC()
		days := int((7 - int(n.Weekday())) % 7)
		if days == 0 {
			days = 7
		}
		return time.Date(n.Year(), n.Month(), n.Day()+days, 0, 0, 0, 0, time.UTC), true
	}
	if spec == "@monthly" {
		n := from.UTC()
		return time.Date(n.Year(), n.Month()+1, 1, 0, 0, 0, 0, time.UTC), true
	}
	if spec == "@yearly" || spec == "@annually" {
		n := from.UTC()
		return time.Date(n.Year()+1, 1, 1, 0, 0, 0, 0, time.UTC), true
	}
	parts := strings.Fields(spec)
	if len(parts) != 5 {
		return time.Time{}, false
	}
	start := from.UTC().Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		candidate := start.Add(time.Duration(i) * time.Minute)
		domMatch := cronField(parts[2], candidate.Day(), 1, 31)
		dowMatch := cronField(parts[4], int(candidate.Weekday()), 0, 6)
		// 标准 cron 在日字段同时受限时采用 OR 语义。
		domWildcard := parts[2] == "*"
		dowWildcard := parts[4] == "*"
		dayMatch := (domWildcard && dowMatch) || (dowWildcard && domMatch) || (!domWildcard && !dowWildcard && (domMatch || dowMatch))
		if cronField(parts[0], candidate.Minute(), 0, 59) && cronField(parts[1], candidate.Hour(), 0, 23) && dayMatch && cronField(parts[3], int(candidate.Month()), 1, 12) {
			return candidate, true
		}
	}
	return time.Time{}, false
}

func cronField(expr string, value, min, max int) bool {
	for _, item := range strings.Split(expr, ",") {
		item = strings.TrimSpace(item)
		if item == "*" {
			return true
		}
		if strings.HasPrefix(item, "*/") {
			step, err := strconv.Atoi(strings.TrimPrefix(item, "*/"))
			if err == nil && step > 0 && (value-min)%step == 0 {
				return true
			}
			continue
		}
		if strings.Contains(item, "-") {
			bounds := strings.SplitN(item, "-", 2)
			if len(bounds) == 2 {
				lo, e1 := strconv.Atoi(bounds[0])
				hi, e2 := strconv.Atoi(bounds[1])
				if e1 == nil && e2 == nil && value >= lo && value <= hi {
					return true
				}
			}
			continue
		}
		n, err := strconv.Atoi(item)
		if err == nil && n >= min && n <= max && n == value {
			return true
		}
	}
	return false
}

// Update 修改计划任务定义并保留原有创建时间。
