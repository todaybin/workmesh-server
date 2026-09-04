// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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

func supervisorCtl(executor runtimeCommandExecutor, item runtimeRecord, args ...string) (model.CommandResult, error) {
	command := append([]string{"exec", "-i", item.Container, "supervisorctl"}, args...)
	return runtimeCommand(executor, item, 30*time.Second, command...)
}

func restoreSupervisorFile(path string, old []byte, existed bool) {
	if existed {
		_ = writeAtomicRuntimeFile(path, old)
	} else {
		_ = os.Remove(path)
	}
}

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

func readSupervisorFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("Supervisor 文件不是普通文件")
	}
	if info.Size() > 2<<20 {
		return "", errors.New("Supervisor 文件超过 2 MiB 限制")
	}
	content, err := os.ReadFile(path)
	return string(content), err
}

func registerPHPSupervisorRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("GET /api/v2/runtimes/supervisor/process/{id}", func(w http.ResponseWriter, r *http.Request) {
		item, err := supervisorRuntime(s, r.PathValue("id"))
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		processes, err := listSupervisorProcesses(s.commandExecutor(), item)
		if err != nil {
			runtimeErr(w, http.StatusBadGateway, "读取 Supervisor 进程失败: "+err.Error())
			return
		}
		runtimeOK(w, processes)
	})
	mux.HandleFunc("POST /api/v2/runtimes/supervisor/process", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		item, err := supervisorRuntime(s, runtimeRequestID(body))
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		operation := strings.ToLower(runtimeString(body, "operate", "operation"))
		name := runtimeString(body, "name")
		if !supervisorProcessNamePattern.MatchString(name) || name == "php-fpm" && operation == "delete" {
			runtimeErr(w, http.StatusBadRequest, "Supervisor 进程名称或操作无效")
			return
		}
		if operation == "create" || operation == "update" {
			config, configErr := supervisorConfigFromBody(body)
			if configErr != nil {
				runtimeErr(w, http.StatusBadRequest, configErr.Error())
				return
			}
			if err := applySupervisorConfig(s.commandExecutor(), item, operation, config); err != nil {
				status := http.StatusBadGateway
				if strings.Contains(err.Error(), "已存在") || strings.Contains(err.Error(), "不存在") {
					status = http.StatusConflict
				}
				runtimeErr(w, status, err.Error())
				return
			}
			runtimeOK(w, nil)
			return
		}
		if operation != "start" && operation != "stop" && operation != "restart" && operation != "delete" {
			runtimeErr(w, http.StatusBadRequest, "Supervisor 操作只允许 create、update、start、stop、restart 或 delete")
			return
		}
		configPath, outLog, errLog, pathErr := supervisorPaths(item, name)
		if pathErr != nil {
			runtimeErr(w, http.StatusBadRequest, pathErr.Error())
			return
		}
		if _, statErr := os.Stat(configPath); statErr != nil {
			runtimeErr(w, http.StatusNotFound, "Supervisor 进程配置不存在")
			return
		}
		if operation != "delete" {
			if _, err := supervisorCtl(s.commandExecutor(), item, operation, name+":*"); err != nil {
				runtimeErr(w, http.StatusBadGateway, "Supervisor 进程操作失败: "+err.Error())
				return
			}
			runtimeOK(w, nil)
			return
		}
		old, err := os.ReadFile(configPath)
		if err != nil {
			runtimeErr(w, http.StatusNotFound, "Supervisor 进程配置不存在")
			return
		}
		_, _ = supervisorCtl(s.commandExecutor(), item, "stop", name+":*")
		if err := os.Remove(configPath); err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if _, err := supervisorCtl(s.commandExecutor(), item, "reread"); err != nil {
			_ = writeAtomicRuntimeFile(configPath, old)
			runtimeErr(w, http.StatusBadGateway, "Supervisor 删除失败，已恢复配置: "+err.Error())
			return
		}
		if _, err := supervisorCtl(s.commandExecutor(), item, "update"); err != nil {
			_ = writeAtomicRuntimeFile(configPath, old)
			_, _ = supervisorCtl(s.commandExecutor(), item, "reread")
			_, _ = supervisorCtl(s.commandExecutor(), item, "update", name)
			runtimeErr(w, http.StatusBadGateway, "Supervisor 删除失败，已恢复配置: "+err.Error())
			return
		}
		_ = os.Remove(outLog)
		_ = os.Remove(errLog)
		runtimeOK(w, nil)
	})
	mux.HandleFunc("POST /api/v2/runtimes/supervisor/process/file", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		item, err := supervisorRuntime(s, runtimeRequestID(body))
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		name := runtimeString(body, "name")
		operation := strings.ToLower(runtimeString(body, "operate", "operation"))
		kind := strings.ToLower(runtimeString(body, "file"))
		configPath, outLog, errLog, err := supervisorPaths(item, name)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		path := configPath
		if kind == "out.log" {
			path = outLog
		} else if kind == "err.log" {
			path = errLog
		} else if kind != "config" {
			runtimeErr(w, http.StatusBadRequest, "Supervisor 文件类型无效")
			return
		}
		switch operation {
		case "get":
			content, readErr := readSupervisorFile(path)
			if readErr != nil {
				runtimeErr(w, http.StatusNotFound, readErr.Error())
				return
			}
			runtimeOK(w, content)
		case "clear":
			if kind == "config" {
				runtimeErr(w, http.StatusBadRequest, "Supervisor 配置文件不能清空")
				return
			}
			if err := writeAtomicRuntimeFile(path, []byte{}); err != nil {
				runtimeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			runtimeOK(w, "")
		case "update":
			if kind != "config" {
				runtimeErr(w, http.StatusBadRequest, "只允许更新 Supervisor 配置文件")
				return
			}
			content, ok := body["content"].(string)
			if !ok || content == "" || len(content) > 2<<20 || strings.IndexByte(content, 0) >= 0 {
				runtimeErr(w, http.StatusBadRequest, "Supervisor 配置内容无效或超过 2 MiB 限制")
				return
			}
			if _, err := parseSupervisorConfig([]byte(content), name); err != nil {
				runtimeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			old, err := os.ReadFile(configPath)
			if err != nil {
				runtimeErr(w, http.StatusNotFound, "Supervisor 配置文件不存在")
				return
			}
			if err := writeAtomicRuntimeFile(configPath, []byte(content)); err != nil {
				runtimeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			_, applyErr := supervisorCtl(s.commandExecutor(), item, "reread")
			if applyErr == nil {
				_, applyErr = supervisorCtl(s.commandExecutor(), item, "update", name)
			}
			if applyErr != nil {
				_ = writeAtomicRuntimeFile(configPath, old)
				_, _ = supervisorCtl(s.commandExecutor(), item, "reread")
				_, _ = supervisorCtl(s.commandExecutor(), item, "update", name)
				runtimeErr(w, http.StatusBadGateway, "Supervisor 配置应用失败，已恢复旧文件: "+applyErr.Error())
				return
			}
			runtimeOK(w, "")
		default:
			runtimeErr(w, http.StatusBadRequest, "Supervisor 文件操作只允许 get、clear 或 update")
		}
	})
}

const (
	fastCGIVersion      = 1
	fastCGIBeginRequest = 1
	fastCGIEndRequest   = 3
	fastCGIParams       = 4
	fastCGIStdin        = 5
	fastCGIStdout       = 6
	fastCGIStderr       = 7
)

func writeFastCGIRecord(writer io.Writer, recordType byte, requestID uint16, content []byte) error {
	if len(content) > 65535 {
		return errors.New("FastCGI 记录过大")
	}
	padding := byte((8 - len(content)%8) % 8)
	header := []byte{fastCGIVersion, recordType, byte(requestID >> 8), byte(requestID), byte(len(content) >> 8), byte(len(content)), padding, 0}
	if _, err := writer.Write(header); err != nil {
		return err
	}
	if _, err := writer.Write(content); err != nil {
		return err
	}
	if padding > 0 {
		_, err := writer.Write(make([]byte, int(padding)))
		return err
	}
	return nil
}

func appendFastCGILength(target []byte, length int) []byte {
	if length < 128 {
		return append(target, byte(length))
	}
	return binary.BigEndian.AppendUint32(target, uint32(length)|1<<31)
}

func encodeFastCGIParams(values map[string]string) []byte {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	content := make([]byte, 0, 512)
	for _, key := range keys {
		value := values[key]
		content = appendFastCGILength(content, len(key))
		content = appendFastCGILength(content, len(value))
		content = append(content, key...)
		content = append(content, value...)
	}
	return content
}

func readFastCGIStatus(address string, timeout time.Duration) ([]map[string]any, error) {
	connection, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(timeout))
	if err := writeFastCGIRecord(connection, fastCGIBeginRequest, 1, []byte{0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return nil, err
	}
	_, port, splitErr := net.SplitHostPort(address)
	if splitErr != nil {
		return nil, splitErr
	}
	params := encodeFastCGIParams(map[string]string{
		"REQUEST_METHOD": "GET", "REQUEST_URI": "/status", "SCRIPT_FILENAME": "/status", "SCRIPT_NAME": "/status",
		"QUERY_STRING": "", "CONTENT_TYPE": "", "CONTENT_LENGTH": "0", "SERVER_NAME": "localhost",
		"SERVER_PORT": port, "REMOTE_ADDR": "127.0.0.1", "GATEWAY_INTERFACE": "CGI/1.1",
	})
	for _, record := range []struct {
		typ     byte
		content []byte
	}{{fastCGIParams, params}, {fastCGIParams, nil}, {fastCGIStdin, nil}} {
		if err := writeFastCGIRecord(connection, record.typ, 1, record.content); err != nil {
			return nil, err
		}
	}
	stdout, stderr := bytes.Buffer{}, bytes.Buffer{}
	for {
		header := make([]byte, 8)
		if _, err := io.ReadFull(connection, header); err != nil {
			return nil, err
		}
		if header[0] != fastCGIVersion || binary.BigEndian.Uint16(header[2:4]) != 1 {
			return nil, errors.New("FastCGI 响应头无效")
		}
		length, padding := int(binary.BigEndian.Uint16(header[4:6])), int(header[6])
		content := make([]byte, length)
		if _, err := io.ReadFull(connection, content); err != nil {
			return nil, err
		}
		if padding > 0 {
			if _, err := io.CopyN(io.Discard, connection, int64(padding)); err != nil {
				return nil, err
			}
		}
		switch header[1] {
		case fastCGIStdout:
			if stdout.Len()+len(content) > 1<<20 {
				return nil, errors.New("FastCGI 状态响应超过 1 MiB 限制")
			}
			stdout.Write(content)
		case fastCGIStderr:
			if stderr.Len()+len(content) <= 64<<10 {
				stderr.Write(content)
			}
		case fastCGIEndRequest:
			if message := strings.TrimSpace(stderr.String()); message != "" {
				return nil, errors.New(message)
			}
			return parseFastCGIStatusPayload(stdout.String())
		}
	}
}

func parseFastCGIStatusPayload(payload string) ([]map[string]any, error) {
	if _, body, found := strings.Cut(payload, "\r\n\r\n"); found {
		payload = body
	} else if _, body, found := strings.Cut(payload, "\n\n"); found {
		payload = body
	}
	status := []map[string]any{}
	scanner := bufio.NewScanner(strings.NewReader(payload))
	for scanner.Scan() {
		key, value, found := strings.Cut(strings.TrimSpace(scanner.Text()), ":")
		if found && strings.TrimSpace(key) != "" {
			status = append(status, map[string]any{"key": strings.TrimSpace(key), "value": strings.TrimSpace(value)})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(status) == 0 {
		return nil, errors.New("FastCGI 状态响应为空")
	}
	return status, nil
}
