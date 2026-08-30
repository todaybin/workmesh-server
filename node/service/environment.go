// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ApplicationStatus 描述本机应用运行环境的探测结果。
// isExist 表示已安装可执行文件，status 仅在可执行文件存在时反映服务状态。
type ApplicationStatus struct {
	Name     string `json:"name"`
	App      string `json:"app"`
	Version  string `json:"version,omitempty"`
	IsExist  bool   `json:"isExist"`
	IsActive bool   `json:"isActive"`
	Status   string `json:"status"`
	Binary   string `json:"binary,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ProbeApplication 通过受限命令和本机 TCP 探测应用安装及服务状态。
// 探测不接受用户提供的命令参数，所有外部操作都使用短超时，避免阻塞请求。
func ProbeApplication(ctx context.Context, key, name string) ApplicationStatus {
	key = strings.ToLower(strings.TrimSpace(key))
	name = strings.TrimSpace(name)
	if name == "" {
		name = key
	}
	status := ApplicationStatus{Name: name, App: key, Status: "NotInstalled"}
	if key == "" {
		status.Error = "应用标识不能为空"
		return status
	}

	// Docker 由专用服务读取 CLI 和 daemon 状态，保证与容器页面使用同一数据源。
	if key == "docker" {
		info, err := NewDockerService().StatusInfo(ctx)
		if err != nil {
			status.Error = err.Error()
			return status
		}
		status.IsExist, status.IsActive = info.IsExist, info.IsActive
		status.Status = "Stopped"
		if info.IsActive {
			status.Status = "Running"
		}
		status.Version = info.Version
		status.Error = info.Error
		return status
	}

	definition, ok := applicationDefinitions[key]
	if !ok {
		status.Error = "未配置该应用的本机探测器"
		return status
	}
	for _, candidate := range definition.binaries {
		if configured := strings.TrimSpace(os.Getenv(definition.env)); definition.env != "" && configured != "" {
			candidate = configured
		}
		if path, err := exec.LookPath(candidate); err == nil {
			status.IsExist, status.Binary = true, path
			break
		}
	}
	if !status.IsExist {
		status.Error = fmt.Sprintf("未找到 %s 可执行文件", key)
		return status
	}

	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if definition.versionArg != "" {
		out, err := exec.CommandContext(probeCtx, status.Binary, definition.versionArg).CombinedOutput()
		if err != nil {
			status.Error = strings.TrimSpace(string(out))
			if status.Error == "" {
				status.Error = err.Error()
			}
			status.Status = "Stopped"
			return status
		}
		status.Version = parseVersionText(string(out))
	}
	if definition.port > 0 {
		conn, err := (&net.Dialer{}).DialContext(probeCtx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(definition.port)))
		if err == nil {
			_ = conn.Close()
			status.IsActive = true
			status.Status = "Running"
			return status
		}
		status.Error = err.Error()
		status.Status = "Stopped"
		return status
	}
	status.IsActive = true
	status.Status = "Running"
	return status
}

type applicationDefinition struct {
	binaries   []string
	env        string
	versionArg string
	port       int
}

var applicationDefinitions = map[string]applicationDefinition{
	"openresty":  {binaries: []string{"openresty", "nginx"}, env: "WORKMESH_OPENRESTY_BIN", versionArg: "-v", port: 80},
	"nginx":      {binaries: []string{"openresty", "nginx"}, env: "WORKMESH_OPENRESTY_BIN", versionArg: "-v", port: 80},
	"mysql":      {binaries: []string{"mysqld", "mariadbd"}, versionArg: "--version", port: 3306},
	"mariadb":    {binaries: []string{"mariadbd", "mysqld"}, versionArg: "--version", port: 3306},
	"postgres":   {binaries: []string{"postgres", "pg_ctl"}, versionArg: "--version", port: 5432},
	"postgresql": {binaries: []string{"postgres", "pg_ctl"}, versionArg: "--version", port: 5432},
	"redis":      {binaries: []string{"redis-server"}, versionArg: "--version", port: 6379},
}

func parseVersionText(output string) string {
	for _, token := range strings.Fields(output) {
		token = strings.Trim(token, "()[];,:")
		if len(token) > 0 && token[0] >= '0' && token[0] <= '9' {
			return token
		}
	}
	return ""
}
