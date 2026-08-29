// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"strings"

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
