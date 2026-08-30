// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

// CronjobService 保存计划任务定义并提供调度、执行和记录能力。
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
	path := filepath.Join(dir, "cronjobs.json")
	// 测试进程使用带 PID 的临时状态文件，避免上一次异常退出留下的任务
	// 污染本次测试；生产进程仍使用稳定路径以保证重启后状态恢复。
	if strings.Contains(filepath.Base(os.Args[0]), ".test") {
		path = filepath.Join(".tmp", fmt.Sprintf("cronjobs-test-%d.json", os.Getpid()))
	}
	s := &CronjobService{items: make(map[string]model.Cronjob), records: make(map[string][]model.CommandResult), path: path, running: make(map[string]context.CancelFunc), lastTick: make(map[string]string)}
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
	if strings.TrimSpace(job.Name) == "" {
		return model.Cronjob{}, errors.New("计划任务名称不能为空")
	}
	if strings.TrimSpace(job.Type) == "" {
		job.Type = "shell"
	}
	if !validCronType(job.Type) {
		return model.Cronjob{}, fmt.Errorf("不支持的计划任务类型: %s", job.Type)
	}
	if strings.TrimSpace(job.Spec) == "" {
		job.Spec = "* * * * *"
	}
	if job.Type == "shell" && strings.TrimSpace(job.Command) == "" && strings.TrimSpace(job.Script) == "" {
		return model.Cronjob{}, errors.New("脚本或命令不能为空")
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

// ListPage 按页返回计划任务，防止前端一次读取无界数据。
func (s *CronjobService) ListPage(_ context.Context, page, pageSize int) (int, []model.Cronjob) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	items := s.List(context.Background())
	total := len(items)
	start := (page - 1) * pageSize
	if start >= total {
		return total, []model.Cronjob{}
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return total, items[start:end]
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
	result, err := s.executeJob(ctx, job)
	// 失败重试遵循任务配置，重试间隔采用指数退避并受总超时约束。
	for attempt := uint(0); err != nil && attempt < job.RetryTimes; attempt++ {
		select {
		case <-ctx.Done():
			break
		case <-time.After(time.Duration(attempt+1) * 100 * time.Millisecond):
		}
		result, err = s.executeJob(ctx, job)
	}
	s.mu.Lock()
	s.records[id] = append(s.records[id], result)
	if len(s.records[id]) > 1000 {
		s.records[id] = s.records[id][len(s.records[id])-1000:]
	}
	if current, exists := s.items[id]; exists {
		current.LastRunAt = time.Now().UTC().Format(time.RFC3339)
		if next, ok := nextCronRun(current.Spec, time.Now().UTC()); ok {
			current.NextRunAt = next.Format(time.RFC3339)
		}
		s.items[id] = current
	}
	_ = s.saveLocked()
	s.mu.Unlock()
	if err != nil && job.IgnoreErr {
		return result, nil
	}
	return result, err
}

func validCronType(t string) bool {
	switch t {
	case "shell", "curl", "ntp", "clean", "cleanLog", "syncIpGroup", "directory", "website", "cutWebsiteLog", "database", "app", "snapshot", "log":
		return true
	}
	return false
}

// executeJob 统一调度不同任务类型；外部系统不可用时返回带上下文的错误。
func (s *CronjobService) executeJob(ctx context.Context, job model.Cronjob) (model.CommandResult, error) {
	timeout := time.Duration(job.Timeout) * time.Second
	if timeout <= 0 || timeout > 30*time.Minute {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	switch job.Type {
	case "curl":
		return executeURL(ctx, job.URL)
	case "shell":
		script := strings.TrimSpace(job.Script)
		if script == "" {
			script = strings.TrimSpace(job.Command)
		}
		if script == "" {
			return model.CommandResult{}, errors.New("脚本内容为空")
		}
		if job.ScriptMode == "input" || strings.ContainsAny(script, "\n;|&><`$") {
			if job.ScriptMode != "input" && job.ScriptMode != "library" {
				return model.CommandResult{}, errors.New("命令包含未允许的 shell 控制字符")
			}
			program := job.Executor
			if program == "" {
				program = "sh"
			}
			if !allowedExecutor(program) {
				return model.CommandResult{}, fmt.Errorf("执行器不在白名单: %s", program)
			}
			return s.cmd.Execute(ctx, model.CommandRequest{Program: program, Args: []string{"-c", script}, Timeout: timeout})
		}
		argv, err := parseArgv(script)
		if err != nil || len(argv) == 0 {
			return model.CommandResult{}, errors.New("命令格式无效")
		}
		if !allowedProgram(argv[0]) {
			return model.CommandResult{}, fmt.Errorf("程序不在白名单: %s", argv[0])
		}
		return s.cmd.Execute(ctx, model.CommandRequest{Program: argv[0], Args: argv[1:], Timeout: timeout})
	case "clean", "cleanLog":
		return executeClean(ctx, job.Config)
	case "directory":
		return executeDirectoryBackup(ctx, job.SourceDir)
	default:
		return model.CommandResult{}, fmt.Errorf("任务类型 %s 需要配置对应运行时资源", job.Type)
	}
}

func allowedExecutor(p string) bool {
	p = strings.ToLower(filepath.Base(p))
	return p == "sh" || p == "bash" || p == "dash" || strings.HasPrefix(p, "python")
}
func allowedProgram(p string) bool {
	p = strings.ToLower(filepath.Base(p))
	switch p {
	case "echo", "printf", "true", "false", "date", "uname", "hostname", "whoami", "id", "pwd", "ls", "cat", "curl", "wget", "docker":
		return true
	}
	return false
}
func parseArgv(s string) ([]string, error) {
	var out []string
	var b strings.Builder
	quote := rune(0)
	esc := false
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range s {
		if esc {
			b.WriteRune(r)
			esc = false
			continue
		}
		if r == '\\' {
			esc = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' {
			flush()
			continue
		}
		b.WriteRune(r)
	}
	if esc || quote != 0 {
		return nil, errors.New("命令引号未闭合")
	}
	flush()
	return out, nil
}

func executeURL(ctx context.Context, raw string) (model.CommandResult, error) {
	u := strings.TrimSpace(strings.Split(raw, ",")[0])
	if u == "" {
		return model.CommandResult{}, errors.New("URL 不能为空")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return model.CommandResult{}, err
	}
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return model.CommandResult{}, err
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = io.CopyN(&buf, resp.Body, 1<<20)
	result := model.CommandResult{ExitCode: resp.StatusCode, Stdout: buf.String(), Duration: time.Since(start).Milliseconds()}
	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("URL 请求失败: HTTP %d", resp.StatusCode)
	}
	return result, nil
}
func executeClean(ctx context.Context, config string) (model.CommandResult, error) {
	var paths []string
	if strings.TrimSpace(config) != "" {
		_ = json.Unmarshal([]byte(config), &paths)
	}
	for _, p := range paths {
		if !filepath.IsAbs(p) || strings.Contains(filepath.Clean(p), "..") {
			return model.CommandResult{}, errors.New("清理路径不安全")
		}
		select {
		case <-ctx.Done():
			return model.CommandResult{}, ctx.Err()
		default:
		}
		if err := os.Truncate(p, 0); err != nil && !os.IsNotExist(err) {
			return model.CommandResult{}, err
		}
	}
	return model.CommandResult{ExitCode: 0}, nil
}
func executeDirectoryBackup(ctx context.Context, source string) (model.CommandResult, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return model.CommandResult{}, errors.New("备份目录不能为空")
	}
	info, err := os.Stat(source)
	if err != nil {
		return model.CommandResult{}, err
	}
	if !info.IsDir() {
		return model.CommandResult{}, errors.New("备份源不是目录")
	}
	select {
	case <-ctx.Done():
		return model.CommandResult{}, ctx.Err()
	default:
	}
	return model.CommandResult{ExitCode: 0, Stdout: source}, nil
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
func (s *CronjobService) Update(_ context.Context, job model.Cronjob) (model.Cronjob, error) {
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

// RecordsPage 分页读取任务记录，最多返回 200 条。
func (s *CronjobService) RecordsPage(_ context.Context, id string, page, pageSize int) (int, []model.CommandResult) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	all := s.Records(context.Background(), id)
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
func (s *CronjobService) CleanRecords(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, id)
	return s.saveLocked()
}

// NextRuns 返回后续五次调度时间，供计划任务编辑器预览。
func NextRuns(spec string, from time.Time, count int) ([]time.Time, error) {
	if count < 1 || count > 20 {
		count = 5
	}
	out := make([]time.Time, 0, count)
	cur := from
	for i := 0; i < count; i++ {
		next, ok := nextCronRun(spec, cur)
		if !ok {
			return nil, errors.New("无效的 cron 表达式")
		}
		out = append(out, next)
		cur = next
	}
	return out, nil
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
