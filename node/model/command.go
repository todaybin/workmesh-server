// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package model 定义节点执行面共用的数据结构。
package model

import "time"

// CommandRequest 描述一次不经过 shell 的系统命令调用。
type CommandRequest struct {
	Program string            `json:"program"`
	Args    []string          `json:"args,omitempty"`
	Dir     string            `json:"dir,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Timeout time.Duration     `json:"timeout,omitempty"`
}

// CommandResult 保存命令退出状态及截断后的标准输出。
type CommandResult struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Duration int64  `json:"durationMs"`
}

// DockerOperationRequest 描述 Docker 容器操作。
type DockerOperationRequest struct {
	Container string `json:"container"`
	Operation string `json:"operation"`
}

// Cronjob 描述节点计划任务的最小持久化模型。
type Cronjob struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Spec      string `json:"spec"`
	Command   string `json:"command"`
	Status    string `json:"status"`
	GroupID   uint   `json:"groupID,omitempty"`
	Type      string `json:"type,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	LastRunAt string `json:"lastRunAt,omitempty"`
	NextRunAt string `json:"nextRunAt,omitempty"`
}
