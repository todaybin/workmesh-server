// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

// DockerService 通过 Docker CLI 提供跨平台的最小容器适配。
type DockerService struct{ commands CommandService }

var dockerLookPath = exec.LookPath
var dockerStandardPaths = func() []string {
	return []string{"/usr/bin/docker", "/usr/local/bin/docker", "/snap/bin/docker", filepath.Join(os.Getenv("ProgramFiles"), "Docker", "Docker", "resources", "bin", "docker.exe")}
}

// NewDockerService 创建 Docker 服务。
func NewDockerService() DockerService { return DockerService{commands: CommandService{}} }

// Status 返回 Docker daemon 的版本信息。
func (s DockerService) Status(ctx context.Context) (model.CommandResult, error) {
	return s.commands.Execute(ctx, model.CommandRequest{Program: dockerBinaryOrName(), Args: []string{"version", "--format", "{{json .}}"}})
}

// StatusInfo 探测 Docker CLI 和 daemon，返回稳定的前端状态契约。
// 探测使用短超时，daemon 不可用时仍返回 200 和明确的 false 状态，避免前端把命令结果误当 DTO。
func (s DockerService) StatusInfo(ctx context.Context) (model.DockerStatus, error) {
	status := model.DockerStatus{}
	binary := dockerBinary()
	if binary == "" {
		status.Error = "docker CLI 未安装"
		return status, nil
	}
	status.IsExist = true
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := s.commands.Execute(probeCtx, model.CommandRequest{
		Program: binary,
		Args:    []string{"version", "--format", "{{json .}}"},
		Timeout: 10 * time.Second,
	})
	if err == nil && result.ExitCode == 0 {
		status.IsActive = true
		status.Version = strings.TrimSpace(result.Stdout)
		return status, nil
	}
	if strings.TrimSpace(result.Stderr) != "" {
		status.Error = strings.TrimSpace(result.Stderr)
	} else if err != nil {
		status.Error = err.Error()
	} else {
		status.Error = "docker daemon 不可用"
	}
	return status, nil
}

// List 返回容器列表文本，保留 Docker CLI 原始字段避免迁移期间丢失信息。
func (s DockerService) List(ctx context.Context) (model.CommandResult, error) {
	return s.commands.Execute(ctx, model.CommandRequest{Program: dockerBinaryOrName(), Args: []string{"ps", "-a", "--no-trunc"}})
}

// ContainerStates 按应用安装记录中的容器名读取 Docker 状态。
// Docker 的 name 过滤允许模糊匹配，因此返回前再次进行完整名称匹配。
func (s DockerService) ContainerStates(ctx context.Context, names []string) (map[string]string, error) {
	wanted := make(map[string]struct{}, len(names))
	args := []string{"ps", "-a", "--format", "{{.Names}}\t{{.State}}"}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, exists := wanted[name]; exists {
			continue
		}
		wanted[name] = struct{}{}
		args = append(args, "--filter", "name="+name)
	}
	if len(wanted) == 0 {
		return map[string]string{}, nil
	}
	result, err := s.commands.Execute(ctx, model.CommandRequest{Program: dockerBinaryOrName(), Args: args, Timeout: 10 * time.Second})
	if err != nil {
		detail := strings.TrimSpace(result.Stderr)
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("查询 Docker 容器状态失败: %s", detail)
	}
	return parseContainerStates(result.Stdout, wanted), nil
}

func parseContainerStates(output string, wanted map[string]struct{}) map[string]string {
	states := make(map[string]string, len(wanted))
	for _, line := range strings.Split(output, "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimSpace(fields[0])
		if _, ok := wanted[name]; !ok {
			continue
		}
		states[name] = strings.ToLower(strings.TrimSpace(fields[1]))
	}
	return states
}

// Operate 执行白名单中的容器生命周期操作。
func (s DockerService) Operate(ctx context.Context, req model.DockerOperationRequest) (model.CommandResult, error) {
	operation := strings.ToLower(strings.TrimSpace(req.Operation))
	if operation == "up" {
		// 原 Agent 将 up 视为启动容器；保持 v2 操作语义一致。
		operation = "start"
	}
	allowed := map[string]bool{"start": true, "stop": true, "restart": true, "kill": true, "pause": true, "unpause": true, "remove": true}
	if !allowed[operation] {
		return model.CommandResult{}, errors.New("不支持的容器操作")
	}
	names := make([]string, 0, len(req.Names)+1)
	for _, name := range req.Names {
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 && strings.TrimSpace(req.Container) != "" {
		names = append(names, strings.TrimSpace(req.Container))
	}
	if len(names) == 0 {
		return model.CommandResult{}, errors.New("容器名称不能为空")
	}
	for _, name := range names {
		if !validDockerName(name) {
			return model.CommandResult{}, fmt.Errorf("容器名称无效: %s", name)
		}
	}
	command := operation
	if command == "remove" {
		command = "rm"
	}
	var aggregate model.CommandResult
	for _, name := range names {
		result, err := s.commands.Execute(ctx, model.CommandRequest{Program: dockerBinaryOrName(), Args: []string{command, name}, Timeout: 5 * time.Minute})
		aggregate.ExitCode = result.ExitCode
		aggregate.Duration += result.Duration
		aggregate.Stdout += result.Stdout
		aggregate.Stderr += result.Stderr
		if err != nil || result.ExitCode != 0 {
			if err == nil {
				err = fmt.Errorf("docker %s %s 失败: %s", operation, name, strings.TrimSpace(result.Stderr))
			}
			return aggregate, err
		}
	}
	return aggregate, nil
}

// validDockerName 限制生命周期操作只接受 Docker 名称或 ID，避免将输入解释为额外参数。
func validDockerName(value string) bool {
	if value == "" || len(value) > 255 || strings.ContainsAny(value, " \t\r\n\x00") {
		return false
	}
	for _, ch := range value {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || strings.ContainsRune("._-", ch)) {
			return false
		}
	}
	return true
}

// dockerBinary 在服务管理器精简 PATH 时补查 Docker 的标准安装位置。
func dockerBinary() string {
	if value, err := dockerLookPath("docker"); err == nil {
		return value
	}
	for _, candidate := range dockerStandardPaths() {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func dockerBinaryOrName() string {
	if value := dockerBinary(); value != "" {
		return value
	}
	return "docker"
}

// DockerBinary 返回当前系统可用的 Docker CLI 路径；服务管理器精简 PATH 时仍能定位标准安装位置。
func DockerBinary() string { return dockerBinaryOrName() }
