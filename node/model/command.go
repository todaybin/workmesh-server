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

// DockerStatus 描述 Docker 客户端和守护进程的可用状态。
// IsExist 表示本机是否存在 docker CLI，IsActive 表示 CLI 能否连接守护进程。
type DockerStatus struct {
	IsActive bool   `json:"isActive"`
	IsExist  bool   `json:"isExist"`
	Version  string `json:"version,omitempty"`
	Error    string `json:"error,omitempty"`
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
	// 以下字段覆盖文件、网站、数据库、应用和快照任务所需配置。
	SpecCustom        bool   `json:"specCustom,omitempty"`
	Executor          string `json:"executor,omitempty"`
	ScriptMode        string `json:"scriptMode,omitempty"`
	Script            string `json:"script,omitempty"`
	ContainerName     string `json:"containerName,omitempty"`
	User              string `json:"user,omitempty"`
	ScriptID          string `json:"scriptID,omitempty"`
	Website           string `json:"website,omitempty"`
	AppID             string `json:"appID,omitempty"`
	DBType            string `json:"dbType,omitempty"`
	DBName            string `json:"dbName,omitempty"`
	URL               string `json:"url,omitempty"`
	IsDir             bool   `json:"isDir,omitempty"`
	SourceDir         string `json:"sourceDir,omitempty"`
	SnapshotRule      string `json:"snapshotRule,omitempty"`
	ExclusionRules    string `json:"exclusionRules,omitempty"`
	SourceAccountIDs  string `json:"sourceAccountIDs,omitempty"`
	DownloadAccountID string `json:"downloadAccountID,omitempty"`
	RetryTimes        uint   `json:"retryTimes,omitempty"`
	Timeout           uint   `json:"timeout,omitempty"`
	IgnoreErr         bool   `json:"ignoreErr,omitempty"`
	RetainCopies      uint64 `json:"retainCopies,omitempty"`
	Args              string `json:"args,omitempty"`
	Secret            string `json:"secret,omitempty"`
	Config            string `json:"config,omitempty"`
}

// ScriptLibrary 描述可审核执行的脚本库条目。
type ScriptLibrary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Script      string `json:"script"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	Approved    bool   `json:"approved"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}
