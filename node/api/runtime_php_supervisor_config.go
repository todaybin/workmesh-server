// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

type supervisorProcessConfig struct {
	Name        string                  `json:"name"`
	Command     string                  `json:"command"`
	User        string                  `json:"user"`
	Dir         string                  `json:"dir"`
	Numprocs    string                  `json:"numprocs"`
	Msg         string                  `json:"msg"`
	Status      []supervisorProcessItem `json:"status"`
	AutoRestart string                  `json:"autoRestart"`
	AutoStart   string                  `json:"autoStart"`
	Environment string                  `json:"environment"`
}

type supervisorProcessItem struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	PID    string `json:"PID"`
	Uptime string `json:"uptime"`
	Msg    string `json:"msg"`
}

// supervisorRuntime 执行运行时相关处理并返回可观测错误。
func supervisorRuntime(s *runtimeStore, id string) (runtimeRecord, error) {
	item, index := runtimeByID(s, id)
	if index < 0 || normalizeRuntimeTypeFilter(item.Type) != "php" {
		return runtimeRecord{}, errors.New("PHP 运行时不存在")
	}
	if strings.TrimSpace(item.InstallPath) == "" || strings.TrimSpace(item.Container) == "" {
		return runtimeRecord{}, errors.New("PHP 运行时尚未完成安装")
	}
	return item, nil
}

// supervisorPaths 执行运行时相关处理并返回可观测错误。
func supervisorPaths(item runtimeRecord, name string) (string, string, string, error) {
	if !supervisorProcessNamePattern.MatchString(name) {
		return "", "", "", errors.New("Supervisor 进程名称无效")
	}
	root, err := filepath.Abs(filepath.Join(item.InstallPath, "supervisor"))
	if err != nil {
		return "", "", "", err
	}
	config := filepath.Join(root, "supervisor.d", name+".ini")
	outLog := filepath.Join(root, "log", name+".out.log")
	errLog := filepath.Join(root, "log", name+".err.log")
	for _, target := range []string{config, outLog, errLog} {
		relative, relErr := filepath.Rel(root, target)
		if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", "", "", errors.New("Supervisor 文件路径越界")
		}
	}
	return config, outLog, errLog, nil
}

// parseSupervisorConfig 解析运行时输入并返回结构化结果。
func parseSupervisorConfig(content []byte, name string) (supervisorProcessConfig, error) {
	config := supervisorProcessConfig{Name: name, Numprocs: "1", AutoRestart: "true", AutoStart: "true", Status: []supervisorProcessItem{}}
	inSection := false
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			inSection = strings.EqualFold(section, "program:"+name)
			continue
		}
		if !inSection {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "command":
			config.Command = value
		case "directory":
			config.Dir = value
		case "user":
			config.User = value
		case "numprocs":
			config.Numprocs = value
		case "autorestart":
			config.AutoRestart = value
		case "autostart":
			config.AutoStart = value
		case "environment":
			config.Environment = value
		}
	}
	if config.Command == "" {
		return supervisorProcessConfig{}, errors.New("Supervisor 配置缺少 program 段或 command")
	}
	return config, nil
}

// supervisorConfigFromBody 执行运行时相关处理并返回可观测错误。
func supervisorConfigFromBody(body map[string]any) (supervisorProcessConfig, error) {
	config := supervisorProcessConfig{
		Name: strings.TrimSpace(runtimeString(body, "name")), Command: strings.TrimSpace(runtimeString(body, "command")),
		User: strings.TrimSpace(runtimeString(body, "user")), Dir: strings.TrimSpace(runtimeString(body, "dir", "directory")),
		Numprocs: strings.TrimSpace(runtimeString(body, "numprocs")), AutoRestart: strings.ToLower(strings.TrimSpace(runtimeString(body, "autoRestart"))),
		AutoStart: strings.ToLower(strings.TrimSpace(runtimeString(body, "autoStart"))), Environment: strings.TrimSpace(runtimeString(body, "environment")),
		Status: []supervisorProcessItem{},
	}
	if !supervisorProcessNamePattern.MatchString(config.Name) {
		return config, errors.New("Supervisor 进程名称无效")
	}
	if config.Command == "" || len(config.Command) > 2048 || strings.ContainsAny(config.Command, "\r\n\x00") {
		return config, errors.New("Supervisor 启动命令无效")
	}
	if !supervisorUserPattern.MatchString(config.User) {
		return config, errors.New("Supervisor 用户无效")
	}
	if !strings.HasPrefix(config.Dir, "/") || len(config.Dir) > 1024 || strings.ContainsAny(config.Dir, "\r\n\x00") {
		return config, errors.New("Supervisor 工作目录必须是容器内绝对路径")
	}
	numprocs, err := strconv.Atoi(config.Numprocs)
	if err != nil || numprocs < 1 || numprocs > 9999 {
		return config, errors.New("Supervisor 进程数必须在 1-9999 范围内")
	}
	if config.AutoRestart == "" {
		config.AutoRestart = "true"
	}
	if config.AutoStart == "" {
		config.AutoStart = "true"
	}
	if (config.AutoRestart != "true" && config.AutoRestart != "false") || (config.AutoStart != "true" && config.AutoStart != "false") {
		return config, errors.New("Supervisor 自动启动和自动重启参数无效")
	}
	if len(config.Environment) > 4096 || strings.ContainsAny(config.Environment, "\r\n\x00") {
		return config, errors.New("Supervisor 环境变量无效")
	}
	return config, nil
}

// renderSupervisorConfig 执行运行时相关处理并返回可观测错误。
func renderSupervisorConfig(config supervisorProcessConfig) []byte {
	lines := []string{
		"[program:" + config.Name + "]", "command=" + config.Command, "directory=" + config.Dir,
		"autorestart=" + config.AutoRestart, "autostart=" + config.AutoStart, "startsecs=3",
		"stdout_logfile=/var/log/supervisor/" + config.Name + ".out.log", "stderr_logfile=/var/log/supervisor/" + config.Name + ".err.log",
		"stdout_logfile_maxbytes=2MB", "stderr_logfile_maxbytes=2MB", "user=" + config.User,
		"priority=999", "numprocs=" + config.Numprocs, "process_name=%(program_name)s_%(process_num)02d",
	}
	if config.Environment != "" {
		lines = append(lines, "environment="+config.Environment)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

// supervisorCtl 执行运行时相关处理并返回可观测错误。
func supervisorCtl(executor runtimeCommandExecutor, item runtimeRecord, args ...string) (model.CommandResult, error) {
	command := append([]string{"exec", "-i", item.Container, "supervisorctl"}, args...)
	return runtimeCommand(executor, item, 30*time.Second, command...)
}

// restoreSupervisorFile 执行运行时相关处理并返回可观测错误。
func restoreSupervisorFile(path string, old []byte, existed bool) {
	if existed {
		_ = writeAtomicRuntimeFile(path, old)
	} else {
		_ = os.Remove(path)
	}
}

// applySupervisorConfig 执行运行时相关处理并返回可观测错误。
func applySupervisorConfig(executor runtimeCommandExecutor, item runtimeRecord, operation string, config supervisorProcessConfig) error {
	configPath, outLog, _, err := supervisorPaths(item, config.Name)
	if err != nil {
		return err
	}
	old, readErr := os.ReadFile(configPath)
	existed := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if operation == "create" && existed {
		return errors.New("Supervisor 进程配置已存在")
	}
	if operation == "update" && !existed {
		return errors.New("Supervisor 进程配置不存在")
	}
	if operation != "create" && operation != "update" {
		return errors.New("Supervisor 配置操作无效")
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outLog), 0o750); err != nil {
		return err
	}
	if err := writeAtomicRuntimeFile(configPath, renderSupervisorConfig(config)); err != nil {
		return err
	}
	if _, err := supervisorCtl(executor, item, "reread"); err != nil {
		restoreSupervisorFile(configPath, old, existed)
		return fmt.Errorf("Supervisor 重新读取配置失败，已恢复旧文件: %w", err)
	}
	if _, err := supervisorCtl(executor, item, "update", config.Name); err != nil {
		restoreSupervisorFile(configPath, old, existed)
		_, _ = supervisorCtl(executor, item, "reread")
		_, _ = supervisorCtl(executor, item, "update", config.Name)
		return fmt.Errorf("Supervisor 应用配置失败，已恢复旧文件: %w", err)
	}
	return nil
}

// parseSupervisorStatus 解析运行时输入并返回结构化结果。
func parseSupervisorStatus(output string) map[string][]supervisorProcessItem {
	items := map[string][]supervisorProcessItem{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "stdout:")))
		if len(fields) < 2 {
			continue
		}
		name, status := fields[0], fields[1]
		group := strings.SplitN(name, ":", 2)[0]
		entry := supervisorProcessItem{Name: name, Status: status}
		if status == "RUNNING" && len(fields) >= 4 && fields[2] == "pid" {
			entry.PID = strings.TrimSuffix(fields[3], ",")
			for index := 4; index+1 < len(fields); index++ {
				if strings.TrimSuffix(fields[index], ",") == "uptime" {
					entry.Uptime = strings.Join(fields[index+1:], " ")
					break
				}
			}
		} else if len(fields) > 2 {
			entry.Msg = strings.Join(fields[2:], " ")
		}
		items[group] = append(items[group], entry)
	}
	return items
}

// listSupervisorProcesses 执行运行时相关处理并返回可观测错误。
func listSupervisorProcesses(executor runtimeCommandExecutor, item runtimeRecord) ([]supervisorProcessConfig, error) {
	configDir := filepath.Join(item.InstallPath, "supervisor", "supervisor.d")
	entries, err := os.ReadDir(configDir)
	if errors.Is(err, os.ErrNotExist) {
		return []supervisorProcessConfig{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(entries) > 200 {
		return nil, errors.New("Supervisor 配置超过 200 个上限")
	}
	statusResult, err := supervisorCtl(executor, item, "status")
	if err != nil {
		return nil, err
	}
	statuses := parseSupervisorStatus(statusResult.Stdout)
	result := make([]supervisorProcessConfig, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ini") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".ini")
		if !supervisorProcessNamePattern.MatchString(name) {
			continue
		}
		path, _, _, _ := supervisorPaths(item, name)
		content, readErr := readRuntimeConfigFile(path)
		if readErr != nil {
			continue
		}
		config, parseErr := parseSupervisorConfig(content, name)
		if parseErr != nil {
			continue
		}
		config.Status = statuses[name]
		if config.Status == nil {
			config.Status = []supervisorProcessItem{}
		}
		result = append(result, config)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
