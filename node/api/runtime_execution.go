// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
)

// deployRuntimeArchive 执行运行时相关处理并返回可观测错误。
func deployRuntimeArchive(archivePath, installDir, runtimeType, version string) error {
	parent := filepath.Dir(installDir)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".runtime-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := extractTarGz(archivePath, stage); err != nil {
		return err
	}
	source := filepath.Join(stage, strings.ToLower(runtimeType), version)
	info, err := os.Stat(source)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("归档缺少 %s/%s 版本目录", strings.ToLower(runtimeType), version)
	}
	for _, required := range []string{"docker-compose.yml"} {
		entry, statErr := os.Stat(filepath.Join(source, required))
		if statErr != nil || !entry.Mode().IsRegular() {
			return fmt.Errorf("归档缺少 %s", required)
		}
	}
	backup := installDir + ".previous-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	hadExisting := false
	if _, err := os.Stat(installDir); err == nil {
		if err := os.Rename(installDir, backup); err != nil {
			return fmt.Errorf("备份旧运行时目录失败: %w", err)
		}
		hadExisting = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(source, installDir); err != nil {
		if hadExisting {
			_ = os.Rename(backup, installDir)
		}
		return fmt.Errorf("部署运行时目录失败: %w", err)
	}
	return nil
}

// runtimeComposeServiceName 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeComposeServiceName(runtimeType string) string {
	switch normalizeRuntimeTypeFilter(runtimeType) {
	case "go":
		return "golang"
	case "php", "java", "node", "python", "dotnet":
		return normalizeRuntimeTypeFilter(runtimeType)
	default:
		return "runtime"
	}
}

// writeRuntimeComposeOverride 写入运行时配置或协议数据，失败时返回完整错误。
func writeRuntimeComposeOverride(installDir string, item runtimeRecord) error {
	serviceConfig := map[string]any{}
	ports := make([]string, 0, len(item.ExposedPorts))
	for _, raw := range item.ExposedPorts {
		entry, _ := raw.(map[string]any)
		hostPort, _ := runtimeNumberValue(entry["hostPort"])
		containerPort, _ := runtimeNumberValue(entry["containerPort"])
		hostIP := runtimeString(entry, "hostIP")
		if hostIP == "" {
			hostIP = "0.0.0.0"
		}
		protocol := strings.ToLower(runtimeString(entry, "protocol"))
		if protocol == "" {
			protocol = "tcp"
		}
		ports = append(ports, fmt.Sprintf("%s:%d:%d/%s", hostIP, hostPort, containerPort, protocol))
	}
	if len(ports) > 0 {
		serviceConfig["ports"] = ports
	}
	volumes := make([]string, 0, len(item.Volumes))
	for _, raw := range item.Volumes {
		entry, _ := raw.(map[string]any)
		mapping := runtimeString(entry, "source") + ":" + runtimeString(entry, "target")
		if mode := strings.ToLower(runtimeString(entry, "mode")); mode != "" {
			mapping += ":" + mode
		}
		volumes = append(volumes, mapping)
	}
	if len(volumes) > 0 {
		serviceConfig["volumes"] = volumes
	}
	extraHosts := make([]string, 0, len(item.ExtraHosts))
	for _, raw := range item.ExtraHosts {
		entry, _ := raw.(map[string]any)
		extraHosts = append(extraHosts, runtimeString(entry, "hostname", "host")+":"+runtimeString(entry, "ip"))
	}
	if len(extraHosts) > 0 {
		serviceConfig["extra_hosts"] = extraHosts
	}
	path := filepath.Join(installDir, "docker-compose.override.json")
	if len(serviceConfig) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	payload, err := json.MarshalIndent(map[string]any{"services": map[string]any{runtimeComposeServiceName(item.Type): serviceConfig}}, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicRuntimeFile(path, append(payload, '\n'))
}

// runtimeComposeArgs 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeComposeArgs(item runtimeRecord, operation ...string) []string {
	args := []string{"compose", "-f", item.ComposePath}
	if override := filepath.Join(filepath.Dir(item.ComposePath), "docker-compose.override.json"); func() bool { _, err := os.Stat(override); return err == nil }() {
		args = append(args, "-f", override)
	}
	return append(args, operation...)
}

// runtimeCommandEnv 返回 Compose 解析所需的统一环境变量，避免缺失变量形成非法挂载规格。
func runtimeCommandEnv(item runtimeRecord) (map[string]string, error) {
	values, err := runtimeEnvironment(item)
	if err != nil {
		return nil, err
	}
	env := make(map[string]string, len(values))
	for key, value := range values {
		if validEnvKey(key) {
			env[key] = fmt.Sprint(value)
		}
	}
	return env, nil
}

// validateRuntimeCompose 校验运行时参数和外部资源边界。
func validateRuntimeCompose(executor runtimeCommandExecutor, item runtimeRecord, env map[string]string) error {
	args := runtimeComposeArgs(item, "config")
	_, err := runtimeCommandWithEnv(executor, item, 2*time.Minute, env, nil, args...)
	if err != nil {
		return fmt.Errorf("Docker Compose 配置校验失败: %w", err)
	}
	return nil
}

// runtimeCommand 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeCommand(executor runtimeCommandExecutor, item runtimeRecord, timeout time.Duration, args ...string) (model.CommandResult, error) {
	return runtimeCommandWithOutput(executor, item, timeout, nil, args...)
}

// runtimeCommandWithOutput 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeCommandWithOutput(executor runtimeCommandExecutor, item runtimeRecord, timeout time.Duration, output func(string, []byte), args ...string) (model.CommandResult, error) {
	return runtimeCommandWithEnv(executor, item, timeout, nil, output, args...)
}

// runtimeCommandWithEnv 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeCommandWithEnv(executor runtimeCommandExecutor, item runtimeRecord, timeout time.Duration, env map[string]string, output func(string, []byte), args ...string) (model.CommandResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	dir := ""
	if strings.TrimSpace(item.ComposePath) != "" {
		candidate := filepath.Dir(item.ComposePath)
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			dir = candidate
		}
	}
	result, err := executor.Execute(ctx, model.CommandRequest{Program: service.DockerBinary(), Args: args, Dir: dir, Env: env, Timeout: timeout, Output: output})
	if err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = strings.TrimSpace(result.Stdout)
		}
		if message == "" && err != nil {
			message = err.Error()
		}
		if message == "" {
			message = fmt.Sprintf("命令退出码 %d", result.ExitCode)
		}
		return result, errors.New(message)
	}
	return result, nil
}

// runtimeContainerStatus 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeContainerStatus(executor runtimeCommandExecutor, item runtimeRecord) (string, error) {
	if strings.TrimSpace(item.Container) == "" {
		return "", errors.New("运行时缺少容器名")
	}
	result, err := runtimeCommand(executor, item, 30*time.Second, "inspect", "--format", "{{.State.Status}}", item.Container)
	if err != nil {
		return "", err
	}
	status := strings.ToLower(strings.TrimSpace(result.Stdout))
	if status == "" {
		return "", errors.New("Docker 未返回容器状态")
	}
	return status, nil
}

// ensureRuntimeNetwork 执行运行时相关处理并返回可观测错误。
func ensureRuntimeNetwork(executor runtimeCommandExecutor, name string) error {
	probe := runtimeRecord{ComposePath: filepath.Join(os.TempDir(), "runtime-network-probe.yml")}
	if _, err := runtimeCommand(executor, probe, 30*time.Second, "network", "inspect", name); err == nil {
		return nil
	}
	_, err := runtimeCommand(executor, probe, 30*time.Second, "network", "create", name)
	return err
}

// executeRuntimeInstall 执行运行时相关处理并返回可观测错误。
func executeRuntimeInstall(executor runtimeCommandExecutor, item runtimeRecord, update func(string, string)) error {
	return executeRuntimeInstallChecked(executor, item, func(status, message string) error {
		update(status, message)
		return nil
	})
}

func executeRuntimeInstallChecked(executor runtimeCommandExecutor, item runtimeRecord, update func(string, string) error) error {
	run := func(status, message string, timeout time.Duration, args ...string) error {
		if err := update(status, message); err != nil {
			return err
		}
		appendRuntimeBuildLog(item, "$ "+service.DockerBinary()+" "+strings.Join(args, " "))
		output := func(stream string, chunk []byte) {
			appendRuntimeBuildLog(item, "["+stream+"] "+string(chunk))
			if item.TaskID != "" {
				appendAppTaskLog(item.TaskID, string(chunk))
			}
		}
		result, err := runtimeCommandWithOutput(executor, item, timeout, output, args...)
		appendRuntimeBuildLog(item, result.Stdout)
		appendRuntimeBuildLog(item, result.Stderr)
		if strings.TrimSpace(result.Stdout) != "" {
			if err := update(status, result.Stdout); err != nil {
				return err
			}
		}
		if err != nil {
			appendRuntimeBuildLog(item, "命令失败: "+err.Error())
			return fmt.Errorf("%s: %w", message, err)
		}
		return nil
	}
	if normalizeRuntimeTypeFilter(item.Type) != "php" {
		if err := run("pulling", "正在拉取运行时镜像", 30*time.Minute, runtimeComposeArgs(item, "pull")...); err != nil {
			return err
		}
		return run("starting", "正在启动运行时容器", 30*time.Minute, runtimeComposeArgs(item, "up", "-d")...)
	}
	if err := run("Building", "正在构建 PHP 运行时镜像", time.Hour, runtimeComposeArgs(item, "build")...); err != nil {
		return err
	}
	if err := run("Creating", "正在启动 PHP 构建容器", 30*time.Minute, runtimeComposeArgs(item, "up", "-d")...); err != nil {
		return err
	}
	extensions := strings.TrimSpace(runtimeString(item.Params, "PHP_EXTENSIONS"))
	if extensions != "" {
		if err := run("Building", "正在安装 PHP 扩展", time.Hour, "exec", "-i", item.Container, "install-ext", extensions); err != nil {
			return err
		}
		if err := run("Building", "正在提交 PHP 运行时镜像", 15*time.Minute, "commit", item.Container, item.Image); err != nil {
			return err
		}
	}
	if err := run("ReCreating", "正在停止 PHP 构建容器", 10*time.Minute, runtimeComposeArgs(item, "down")...); err != nil {
		return err
	}
	return run("ReCreating", "正在使用最终镜像启动 PHP 运行时", 30*time.Minute, runtimeComposeArgs(item, "up", "-d")...)
}

// appendRuntimeBuildLog 执行运行时相关处理并返回可观测错误。
func appendRuntimeBuildLog(item runtimeRecord, output string) {
	if strings.TrimSpace(item.InstallPath) == "" || strings.TrimSpace(output) == "" {
		return
	}
	path := filepath.Join(item.InstallPath, "build.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	if info, statErr := file.Stat(); statErr == nil && info.Size() >= 8<<20 {
		return
	}
	_, _ = fmt.Fprintln(file, strings.TrimRight(output, "\r\n"))
}

// formatRuntimeBytes 执行运行时相关处理并返回可观测错误。
func formatRuntimeBytes(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	if value < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(value)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(value)/(1024*1024))
}

// writeAtomicRuntimeFile 写入运行时配置或协议数据，失败时返回完整错误。
func writeAtomicRuntimeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// writeRuntimeEnv 写入运行时配置或协议数据，失败时返回完整错误。
func writeRuntimeEnv(path string, values map[string]any) error {
	lines := make([]string, 0, len(values))
	for key, value := range values {
		if !validEnvKey(key) {
			continue
		}
		lines = append(lines, key+"="+strings.ReplaceAll(fmt.Sprint(value), "\n", ""))
	}
	sort.Strings(lines)
	return writeAtomicRuntimeFile(path, []byte(strings.Join(lines, "\n")+"\n"))
}

// runtimeStatusForOperation 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeStatusForOperation(operation string) string {
	op := strings.ToLower(strings.TrimSpace(operation))
	switch op {
	case "停止":
		op = "stop"
	case "启动":
		op = "start"
	case "重启":
		op = "restart"
	}
	switch op {
	case "up", "start":
		return "Running"
	case "down", "stop":
		return "Stopped"
	case "restart":
		return "Running"
	default:
		return "Normal"
	}
}

type runtimeContainerSnapshot struct {
	status  string
	err     string
	version time.Time
}

// inspectRuntimeContainer 读取单个真实容器状态，并将不存在视为已停止。
func inspectRuntimeContainer(executor runtimeCommandExecutor, item runtimeRecord) runtimeContainerSnapshot {
	if hostRuntime(item) {
		return inspectHostRuntime(item)
	}
	value := runtimeContainerSnapshot{status: "Stopped", version: item.UpdatedAt}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	result, err := executor.Execute(ctx, model.CommandRequest{Program: service.DockerBinary(), Args: []string{"inspect", "--format", "{{.State.Status}}", item.Container}, Timeout: 20 * time.Second})
	cancel()
	if err == nil && result.ExitCode == 0 {
		switch strings.ToLower(strings.TrimSpace(result.Stdout)) {
		case "running":
			value.status = "Running"
		case "restarting":
			value.status = "Restarting"
		case "created":
			value.status = "Creating"
		}
		return value
	}
	value.err = strings.TrimSpace(result.Stderr)
	if value.err == "" && err != nil {
		value.err = err.Error()
	}
	// 容器已被外部删除属于已停止状态，不应让同步接口整体失败。
	missing := strings.Contains(strings.ToLower(value.err), "no such object") || strings.Contains(strings.ToLower(value.err), "not found")
	if !missing {
		value.status = "Error"
	}
	if missing {
		value.err = ""
	}
	return value
}

// applyRuntimeContainerSnapshots 在锁内应用容器快照，避免覆盖并发更新和后台任务状态。
func applyRuntimeContainerSnapshots(s *runtimeStore, statuses map[string]runtimeContainerSnapshot) (bool, error) {
	changed := false
	activeTask := map[string]bool{"creating": true, "building": true, "recreating": true, "starting": true, "downloading": true, "installing": true, "pulling": true}
	for index := range s.state.Runtimes {
		value, exists := statuses[s.state.Runtimes[index].ID]
		if !exists {
			continue
		}
		current := &s.state.Runtimes[index]
		if !current.UpdatedAt.Equal(value.version) {
			continue
		}
		taskStatus := strings.ToLower(strings.TrimSpace(current.TaskStatus))
		if taskStatus == "failed" || taskStatus == "error" {
			continue
		}
		if (current.TaskID != "" || current.TaskStatus != "") && (activeTask[strings.ToLower(strings.TrimSpace(current.Status))] || activeTask[taskStatus]) {
			continue
		}
		if current.Status != value.status || current.Error != value.err || current.Message != value.err {
			current.Status, current.Message, current.Error = value.status, value.err, value.err
			current.UpdatedAt = time.Now().UTC()
			changed = true
		}
	}
	if changed {
		if err := s.saveLocked(); err != nil {
			return false, err
		}
	}
	return changed, nil
}

// syncRuntimeContainerStatus 同步所有运行时容器状态，并返回非缺失容器错误。
func syncRuntimeContainerStatus(s *runtimeStore) error {
	s.mu.RLock()
	items := append([]runtimeRecord(nil), s.state.Runtimes...)
	s.mu.RUnlock()
	statuses := make(map[string]runtimeContainerSnapshot, len(items))
	failures := make([]string, 0)
	for _, item := range items {
		if strings.TrimSpace(item.Container) == "" {
			continue
		}
		value := inspectRuntimeContainer(s.commandExecutor(), item)
		if value.status == "Error" {
			failures = append(failures, item.Name+": "+value.err)
		}
		statuses[item.ID] = value
	}
	s.mu.Lock()
	_, err := applyRuntimeContainerSnapshots(s, statuses)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

// operateRuntimeContainer 执行运行时相关处理并返回可观测错误。
func operateRuntimeContainer(executor runtimeCommandExecutor, item runtimeRecord, operation string) error {
	if hostRuntime(item) {
		return operateHostRuntime(item, operation)
	}
	op := strings.ToLower(strings.TrimSpace(operation))
	switch op {
	case "停止":
		op = "stop"
	case "启动":
		op = "start"
	case "重启":
		op = "restart"
	case "删除", "卸载":
		op = "down"
	}
	if op == "" {
		return errors.New("运行时操作不能为空")
	}
	if item.ComposePath != "" {
		if _, err := os.Stat(item.ComposePath); err == nil {
			env, err := runtimeCommandEnv(item)
			if err != nil {
				return err
			}
			if err := validateRuntimeCompose(executor, item, env); err != nil {
				return err
			}
			if op == "up" || op == "start" {
				op = "up"
			}
			args := runtimeComposeArgs(item, op)
			if op == "up" {
				args = append(args, "-d")
			}
			if _, err := runtimeCommandWithEnv(executor, item, 10*time.Minute, env, nil, args...); err != nil {
				return fmt.Errorf("Docker Compose 操作失败: %w", err)
			}
			return nil
		}
	}
	if strings.TrimSpace(item.Container) == "" {
		return errors.New("运行时缺少 Compose 文件和容器名")
	}
	dockerOp := op
	if dockerOp == "up" {
		dockerOp = "start"
	}
	if dockerOp == "down" {
		dockerOp = "stop"
	}
	_, err := runtimeCommand(executor, item, 10*time.Minute, dockerOp, item.Container)
	return err
}

// removeRuntimeContainer stops and removes a directly managed container. Compose
// projects are removed by `compose down`; plain container runtimes need an
// explicit `rm -f` so deleting a runtime cannot leave an orphan behind.
func removeRuntimeContainer(executor runtimeCommandExecutor, item runtimeRecord) error {
	if strings.TrimSpace(item.ComposePath) != "" {
		if _, err := os.Stat(item.ComposePath); err == nil {
			return operateRuntimeContainer(executor, item, "down")
		}
	}
	if strings.TrimSpace(item.Container) == "" {
		return errors.New("运行时缺少容器名")
	}
	if _, err := runtimeCommand(executor, item, 10*time.Minute, "rm", "-f", item.Container); err != nil {
		return fmt.Errorf("删除运行时容器失败: %w", err)
	}
	return nil
}
