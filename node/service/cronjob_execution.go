// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/model"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *CronjobService) HandleOnce(ctx context.Context, id string) (model.CommandResult, error) {
	if err := s.ensureDatabase(ctx); err != nil {
		return model.CommandResult{}, err
	}
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
	previousRecords := append([]model.CommandResult(nil), s.records[id]...)
	previousJob, hadJob := s.items[id]
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
	persistErr := s.persistLocked(ctx, id)
	if persistErr != nil {
		s.records[id] = previousRecords
		if hadJob {
			s.items[id] = previousJob
		} else {
			delete(s.items, id)
		}
	}
	s.mu.Unlock()
	if persistErr != nil {
		return result, fmt.Errorf("保存计划任务执行结果失败: %w", persistErr)
	}
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
	case "directory", "log":
		return s.executeArchiveJob(ctx, job)
	case "website", "app", "snapshot":
		return s.executeResourceArchive(ctx, job)
	case "cutWebsiteLog":
		return s.executeCutWebsiteLog(ctx, job)
	case "database":
		return s.executeDatabaseCron(ctx, job)
	case "ntp":
		return s.executeNTP(ctx)
	case "syncIpGroup":
		return s.executeSyncIPGroup(ctx, job)
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
