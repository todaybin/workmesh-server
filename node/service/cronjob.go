// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

// CronjobService 保存计划任务定义并提供立即执行适配；持久化迁移将在下一批完成。
type CronjobService struct {
	mu       sync.RWMutex
	items    map[string]model.Cronjob
	cmd      CommandService
	records  map[string][]model.CommandResult
	path     string
	running  map[string]context.CancelFunc
	lastTick map[string]string
}

// NewCronjobService 创建计划任务服务。
func NewCronjobService() *CronjobService {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = ".workmesh-data"
	}
	s := &CronjobService{items: make(map[string]model.Cronjob), records: make(map[string][]model.CommandResult), path: filepath.Join(dir, "cronjobs.json"), running: make(map[string]context.CancelFunc), lastTick: make(map[string]string)}
	if b, err := os.ReadFile(s.path); err == nil {
		var payload struct {
			Items   map[string]model.Cronjob         `json:"items"`
			Records map[string][]model.CommandResult `json:"records"`
		}
		if json.Unmarshal(b, &payload) == nil {
			if payload.Items != nil {
				s.items = payload.Items
			}
			if payload.Records != nil {
				s.records = payload.Records
			}
		}
	}
	return s
}

// Create 新增计划任务，初始状态为 disabled。
func (s *CronjobService) Create(_ context.Context, job model.Cronjob) (model.Cronjob, error) {
	if strings.TrimSpace(job.Name) == "" || strings.TrimSpace(job.Command) == "" {
		return model.Cronjob{}, errors.New("计划任务名称和命令不能为空")
	}
	job.ID = newID()
	job.Status = "disabled"
	now := time.Now().UTC().Format(time.RFC3339)
	job.CreatedAt, job.UpdatedAt = now, now
	s.mu.Lock()
	s.items[job.ID] = job
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return model.Cronjob{}, err
	}
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
	delete(s.records, id)
	return s.saveLocked()
}

// HandleOnce 立即执行计划任务的命令字段。
func (s *CronjobService) HandleOnce(ctx context.Context, id string) (model.CommandResult, error) {
	s.mu.RLock()
	job, ok := s.items[id]
	s.mu.RUnlock()
	if !ok {
		return model.CommandResult{}, errors.New("计划任务不存在")
	}
	if err := ctx.Err(); err != nil {
		return model.CommandResult{}, err
	}
	var result model.CommandResult
	var err error
	if runtime.GOOS == "windows" {
		result, err = s.cmd.Execute(ctx, model.CommandRequest{Program: "cmd", Args: []string{"/C", job.Command}})
	} else {
		result, err = s.cmd.Execute(ctx, model.CommandRequest{Program: "sh", Args: []string{"-c", job.Command}})
	}
	s.mu.Lock()
	s.records[id] = append(s.records[id], result)
	if current, exists := s.items[id]; exists {
		current.LastRunAt = time.Now().UTC().Format(time.RFC3339)
		if next, ok := nextCronRun(current.Spec, time.Now().UTC()); ok {
			current.NextRunAt = next.Format(time.RFC3339)
		}
		s.items[id] = current
	}
	_ = s.saveLocked()
	s.mu.Unlock()
	return result, err
}

// Start 启动单实例计划任务轮询器，服务重启后会从持久化任务状态继续运行。
func (s *CronjobService) Start(parent context.Context) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		s.runDue(parent)
		for {
			select {
			case <-parent.Done():
				return
			case <-ticker.C:
				s.runDue(parent)
			}
		}
	}()
}

// runDue 执行当前分钟到期且尚未执行的启用任务，避免同一分钟重复调度。
func (s *CronjobService) runDue(parent context.Context) {
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
	if spec == "@hourly" {
		return from.Truncate(time.Hour).Add(time.Hour), true
	}
	if spec == "@daily" {
		n := from.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
		return n, true
	}
	parts := strings.Fields(spec)
	if len(parts) != 5 {
		return time.Time{}, false
	}
	start := from.UTC().Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		candidate := start.Add(time.Duration(i) * time.Minute)
		if cronField(parts[0], candidate.Minute(), 0, 59) && cronField(parts[1], candidate.Hour(), 0, 23) && cronField(parts[2], candidate.Day(), 1, 31) && cronField(parts[3], int(candidate.Month()), 1, 12) && cronField(parts[4], int(candidate.Weekday()), 0, 6) {
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
func (s *CronjobService) Update(_ context.Context, job model.Cronjob) (model.Cronjob, error) {
	if job.ID == "" || strings.TrimSpace(job.Name) == "" || strings.TrimSpace(job.Command) == "" {
		return model.Cronjob{}, errors.New("invalid cronjob")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.items[job.ID]
	if !ok {
		return model.Cronjob{}, errors.New("cronjob not found")
	}
	if job.Status == "" {
		job.Status = old.Status
	}
	job.CreatedAt = old.CreatedAt
	job.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.items[job.ID] = job
	if err := s.saveLocked(); err != nil {
		return model.Cronjob{}, err
	}
	return job, nil
}

// SetStatus 切换计划任务启停状态。
func (s *CronjobService) SetStatus(_ context.Context, id, status string) error {
	if status != "enabled" && status != "disabled" {
		return errors.New("invalid status")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.items[id]
	if !ok {
		return errors.New("cronjob not found")
	}
	job.Status, job.UpdatedAt = status, time.Now().UTC().Format(time.RFC3339)
	s.items[id] = job
	return s.saveLocked()
}

// Records 返回任务执行记录，调用方可按需分页。
func (s *CronjobService) Records(_ context.Context, id string) []model.CommandResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]model.CommandResult(nil), s.records[id]...)
}
func (s *CronjobService) CleanRecords(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, id)
	return s.saveLocked()
}
func (s *CronjobService) Get(_ context.Context, id string) (model.Cronjob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.items[id]
	return v, ok
}

func (s *CronjobService) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	b, err := json.Marshal(struct {
		Items   map[string]model.Cronjob         `json:"items"`
		Records map[string][]model.CommandResult `json:"records"`
	}{s.items, s.records})
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
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
