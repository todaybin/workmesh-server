// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

// DockerService 通过 Docker CLI 提供跨平台的最小容器适配。
type DockerService struct{ commands CommandService }

// NewDockerService 创建 Docker 服务。
func NewDockerService() DockerService { return DockerService{commands: CommandService{}} }

// Status 返回 Docker daemon 的版本信息。
func (s DockerService) Status(ctx context.Context) (model.CommandResult, error) {
	return s.commands.Execute(ctx, model.CommandRequest{Program: "docker", Args: []string{"version", "--format", "{{json .}}"}})
}

// StatusInfo 探测 Docker CLI 和 daemon，返回稳定的前端状态契约。
// 探测使用短超时，daemon 不可用时仍返回 200 和明确的 false 状态，避免前端把命令结果误当 DTO。
func (s DockerService) StatusInfo(ctx context.Context) (model.DockerStatus, error) {
	status := model.DockerStatus{}
	if _, err := exec.LookPath("docker"); err != nil {
		status.Error = "docker CLI 未安装"
		return status, nil
	}
	status.IsExist = true
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := s.commands.Execute(probeCtx, model.CommandRequest{
		Program: "docker",
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
	return s.commands.Execute(ctx, model.CommandRequest{Program: "docker", Args: []string{"ps", "-a", "--no-trunc"}})
}

// Operate 执行白名单中的容器生命周期操作。
func (s DockerService) Operate(ctx context.Context, req model.DockerOperationRequest) (model.CommandResult, error) {
	if strings.TrimSpace(req.Container) == "" {
		return model.CommandResult{}, errors.New("容器名称不能为空")
	}
	allowed := map[string]bool{"start": true, "stop": true, "restart": true, "pause": true, "unpause": true, "remove": true}
	if !allowed[req.Operation] {
		return model.CommandResult{}, errors.New("不支持的容器操作")
	}
	operation := req.Operation
	if operation == "remove" {
		operation = "rm"
	}
	return s.commands.Execute(ctx, model.CommandRequest{Program: "docker", Args: []string{operation, req.Container}})
}
