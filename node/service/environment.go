// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
		configured := strings.TrimSpace(os.Getenv(definition.env))
		if definition.env != "" && configured != "" {
			candidate = configured
		}
		if path, err := lookupApplicationBinary(candidate, key, configured != ""); err == nil {
			status.IsExist, status.Binary = true, path
			break
		}
	}
	if !status.IsExist {
		// OpenResty 常以独立容器运行，宿主机命名空间可能看不到容器内的 nginx 二进制。
		// 通过固定参数读取运行中容器清单，避免将容器化安装误报为未安装。
		if key == "openresty" || key == "nginx" {
			if container, ok := probeOpenRestyContainer(ctx); ok {
				return container
			}
		}
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

// lookupApplicationBinary 同时支持服务管理器常见的精简 PATH 和应用专属安装目录。
func lookupApplicationBinary(name, key string, explicit bool) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	if explicit {
		return "", os.ErrNotExist
	}
	candidates := map[string][]string{
		"openresty": {"/usr/local/openresty/nginx/sbin/nginx", "/usr/local/openresty/bin/openresty", "/www/server/openresty/nginx/sbin/nginx", "/www/server/openresty/bin/openresty", "/www/server/nginx/sbin/nginx", "/usr/local/nginx/sbin/nginx", "/opt/openresty/nginx/sbin/nginx", "/opt/openresty/bin/openresty", "/usr/sbin/nginx"},
		"nginx":     {"/usr/local/openresty/nginx/sbin/nginx", "/usr/local/openresty/bin/openresty", "/www/server/openresty/nginx/sbin/nginx", "/www/server/openresty/bin/openresty", "/www/server/nginx/sbin/nginx", "/usr/local/nginx/sbin/nginx", "/opt/openresty/nginx/sbin/nginx", "/opt/openresty/bin/openresty", "/usr/sbin/nginx"},
		"mysql":     {"/usr/sbin/mysqld", "/usr/libexec/mysqld", "/usr/sbin/mariadbd", "/usr/libexec/mariadbd"},
		"mariadb":   {"/usr/sbin/mariadbd", "/usr/libexec/mariadbd"},
		"postgres":  {"/usr/lib/postgresql/bin/postgres", "/usr/lib/postgresql/16/bin/postgres", "/usr/lib/postgresql/15/bin/postgres"},
		"postgresql": {"/usr/lib/postgresql/bin/postgres", "/usr/lib/postgresql/16/bin/postgres", "/usr/lib/postgresql/15/bin/postgres"},
		"redis":     {"/usr/bin/redis-server", "/usr/local/bin/redis-server"},
	}
	for _, path := range candidates[key] {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return filepath.Clean(path), nil
		}
	}
	return "", os.ErrNotExist
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

// probeOpenRestyContainer 识别运行中的 OpenResty/Nginx 容器。
// 只读取 docker ps 的固定输出，不接受请求参数，不执行容器内命令。
func probeOpenRestyContainer(ctx context.Context) (ApplicationStatus, bool) {
	binary := dockerBinary()
	if binary == "" {
		return ApplicationStatus{}, false
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(probeCtx, binary, "ps", "--format", "{{.Names}}\t{{.Image}}\t{{.Status}}").Output()
	if err != nil {
		return ApplicationStatus{}, false
	}
	return parseOpenRestyContainerList(string(out))
}

func parseOpenRestyContainerList(output string) (ApplicationStatus, bool) {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), "\t", 3)
		if len(fields) < 2 {
			continue
		}
		name, image := strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1])
		joined := strings.ToLower(name + " " + image)
		if !strings.Contains(joined, "openresty") && !strings.Contains(joined, "nginx") {
			continue
		}
		version := containerImageVersion(image)
		return ApplicationStatus{
			Name: name, App: "openresty", Version: version, IsExist: true, IsActive: true,
			Status: "Running", Binary: "docker://" + name,
		}, true
	}
	return ApplicationStatus{}, false
}

func containerImageVersion(image string) string {
	image = strings.TrimSpace(image)
	if at := strings.LastIndex(image, "@"); at >= 0 {
		return image[at+1:]
	}
	if colon := strings.LastIndex(image, ":"); colon >= 0 && colon+1 < len(image) {
		return image[colon+1:]
	}
	return ""
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
