// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/model"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BuildOpenResty 执行只读配置检查并返回真实探测结果，避免伪造构建成功。
func (s *WebsiteService) BuildOpenResty(ctx context.Context, modules []string) (OpenRestyStatus, error) {
	status := s.ProbeOpenResty(ctx)
	if !status.Available {
		if status.Error == "" {
			status.Error = "OpenResty 不可用"
		}
		return status, fmt.Errorf("OpenResty 构建前检查失败: %s", status.Error)
	}
	if !status.ConfigValid {
		return status, errors.New("OpenResty 配置检查未通过")
	}
	if len(modules) > 100 {
		return status, errors.New("OpenResty 模块数量超出限制")
	}
	selected := make(map[string]struct{}, len(modules))
	for _, name := range modules {
		name = strings.TrimSpace(name)
		if name == "" || len(name) > 120 || strings.ContainsAny(name, " /\\") {
			return status, errors.New("OpenResty 模块名称无效")
		}
		selected[name] = struct{}{}
	}
	return status, nil
}

// OperateOpenResty 对宿主机或容器中的 OpenResty 执行受控信号操作。
func (s *WebsiteService) OperateOpenResty(ctx context.Context, operation string) (OpenRestyStatus, error) {
	operation = strings.ToLower(strings.TrimSpace(operation))
	if operation != "reload" && operation != "restart" && operation != "stop" && operation != "start" {
		return OpenRestyStatus{}, errors.New("OpenResty 操作无效")
	}
	status := s.ProbeOpenResty(ctx)
	if status.Binary == "" {
		return status, errors.New("OpenResty 未安装或不可用")
	}
	if strings.HasPrefix(status.Binary, "proc://") {
		return status, errors.New("OpenResty 位于独立命名空间，当前节点无法执行 reload；请配置 WORKMESH_OPENRESTY_BIN 或等效 reload 命令")
	}
	var cmd *exec.Cmd
	if strings.HasPrefix(status.Binary, "docker://") {
		name := strings.TrimPrefix(status.Binary, "docker://")
		if name == "" {
			return status, errors.New("OpenResty 容器标识无效")
		}
		docker := dockerBinaryOrName()
		switch operation {
		case "start":
			cmd = exec.CommandContext(ctx, docker, "start", name)
		case "restart":
			cmd = exec.CommandContext(ctx, docker, "restart", name)
		case "stop":
			cmd = exec.CommandContext(ctx, docker, "stop", name)
		default:
			output, err := runContainerOpenRestyCommand(ctx, docker, name, "-s", "reload")
			if err != nil {
				return status, fmt.Errorf("OpenResty reload 失败: %s: %w", strings.TrimSpace(string(output)), err)
			}
			return s.ProbeOpenResty(ctx), nil
		}
	} else {
		switch operation {
		case "start":
			cmd = exec.CommandContext(ctx, status.Binary)
		case "restart":
			// nginx 没有 restart signal，使用 stop 后重新启动，确保返回真实错误。
			stop := exec.CommandContext(ctx, status.Binary, "-s", "stop")
			if output, err := stop.CombinedOutput(); err != nil {
				return status, fmt.Errorf("OpenResty restart 停止阶段失败: %s: %w", strings.TrimSpace(string(output)), err)
			}
			cmd = exec.CommandContext(ctx, status.Binary)
		default:
			cmd = exec.CommandContext(ctx, status.Binary, "-s", operation)
		}
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return status, fmt.Errorf("OpenResty %s 失败: %s: %w", operation, strings.TrimSpace(string(output)), err)
	}
	return s.ProbeOpenResty(ctx), nil
}

// runContainerOpenRestyCommand 兼容自定义镜像中仅提供 openresty 命令的情况。
// 只尝试固定的可执行文件名，不接受请求传入命令，避免把容器操作变成任意命令执行。
func runContainerOpenRestyCommand(ctx context.Context, docker, name string, args ...string) ([]byte, error) {
	var lastOutput []byte
	var lastErr error
	for _, binary := range []string{"nginx", "openresty", "/usr/bin/openresty", "/usr/local/openresty/nginx/sbin/nginx"} {
		commandArgs := append([]string{"exec", name, binary}, args...)
		output, err := exec.CommandContext(ctx, docker, commandArgs...).CombinedOutput()
		if err == nil {
			return output, nil
		}
		lastOutput, lastErr = output, err
	}
	return lastOutput, lastErr
}

// ClearOpenRestyCache 清理受控的反向代理缓存目录，不接受请求直接传入的任意路径。
func (s *WebsiteService) ClearOpenRestyCache() error {
	cacheRoot := strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_CACHE_DIR"))
	if cacheRoot == "" {
		cacheRoot = filepath.Join(s.root, "openresty-cache")
	}
	cacheRoot = filepath.Clean(cacheRoot)
	if cacheRoot == "." || cacheRoot == string(filepath.Separator) || strings.Contains(cacheRoot, ".."+string(filepath.Separator)) {
		return errors.New("OpenResty 缓存目录无效")
	}
	if err := os.MkdirAll(cacheRoot, 0o750); err != nil {
		return err
	}
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(cacheRoot, entry.Name())); err != nil {
			return fmt.Errorf("清理 OpenResty 缓存失败: %w", err)
		}
	}
	return nil
}

func (s *WebsiteService) openRestyConfigPath() string {
	if configured := strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_CONFIG")); configured != "" {
		return filepath.Clean(configured)
	}
	return filepath.Join(s.root, "openresty.conf")
}

func openRestyScopeKeys(scope string) ([]string, bool) {
	switch scope {
	case "index":
		return []string{"index"}, true
	case "limit-conn":
		return []string{"limit_conn", "limit_rate", "limit_conn_zone"}, true
	case "ssl":
		return []string{"ssl_certificate", "ssl_certificate_key"}, true
	case "http-per":
		return []string{"server_names_hash_bucket_size", "client_header_buffer_size", "client_max_body_size", "keepalive_timeout", "gzip", "gzip_min_length", "gzip_comp_level"}, true
	default:
		return nil, false
	}
}

func parseOpenRestyDirectives(content string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(strings.TrimSuffix(line, ";"))
		if len(fields) >= 2 {
			result[fields[0]] = strings.Join(fields[1:], " ")
		}
	}
	return result
}

func balancedConfig(content string) bool {
	depth := 0
	for _, r := range content {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

// ProbeOpenResty 使用受限外部命令探测 OpenResty 安装及配置状态；命令均设置超时且不接受用户参数。
func (s *WebsiteService) ProbeOpenResty(ctx context.Context) OpenRestyStatus {
	cfg := s.GetOpenResty()
	return s.probeOpenResty(ctx, cfg)
}

func (s *WebsiteService) probeOpenResty(ctx context.Context, cfg model.OpenRestyConfig) OpenRestyStatus {
	status := OpenRestyStatus{Enabled: cfg.Enabled, DefaultHTTPS: cfg.DefaultHTTPS, Modules: append([]model.OpenRestyModule{}, cfg.Modules...)}
	configuredContainer := strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_CONTAINER"))
	configuredBin := strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_BIN"))
	// An explicit binary is the narrowest isolation boundary and takes
	// precedence over a configured container. This lets callers deliberately
	// model an unavailable host binary without probing Docker.
	if configuredBin == "" && configuredContainer != "" {
		if container, ok := probeOpenRestyContainer(ctx); ok {
			containerStatus := OpenRestyStatus{
				Available: true, ConfigValid: false, Enabled: container.IsActive,
				Version: container.Version, Binary: container.Binary,
				DefaultHTTPS: cfg.DefaultHTTPS, Modules: append([]model.OpenRestyModule{}, cfg.Modules...),
			}
			name := strings.TrimPrefix(container.Binary, "docker://")
			if output, err := runContainerOpenRestyCommand(ctx, dockerBinaryOrName(), name, "-t"); err == nil {
				containerStatus.ConfigValid = true
			} else {
				containerStatus.Error = strings.TrimSpace(string(output))
				if containerStatus.Error == "" {
					containerStatus.Error = err.Error()
				}
			}
			return s.probeStubStatus(ctx, containerStatus)
		}
		status.Error = "未找到配置的 OpenResty 容器"
		return status
	}
	bin := configuredBin
	if bin == "" {
		for _, candidate := range []string{"openresty", "nginx"} {
			if found, err := lookupApplicationBinary(candidate, "openresty", false); err == nil {
				bin = found
				break
			}
		}
	}
	if bin == "" {
		// An explicitly configured binary is an isolation boundary. Do not
		// silently fall back to an unrelated Docker container or host process
		// when that binary is missing or unusable.
		if configuredBin != "" {
			status.Binary = configuredBin
			status.ConfigPath = s.openRestyConfigPath()
			status.Error = "配置的 OpenResty 可执行文件不可用"
			return status
		}
		if container, ok := probeOpenRestyContainer(ctx); ok {
			containerStatus := OpenRestyStatus{
				Available: true, ConfigValid: false, Enabled: container.IsActive,
				Version: container.Version, Binary: container.Binary,
				DefaultHTTPS: cfg.DefaultHTTPS, Modules: append([]model.OpenRestyModule{}, cfg.Modules...),
			}
			name := strings.TrimPrefix(container.Binary, "docker://")
			if output, err := runContainerOpenRestyCommand(ctx, dockerBinaryOrName(), name, "-t"); err == nil {
				containerStatus.ConfigValid = true
			} else {
				containerStatus.Error = strings.TrimSpace(string(output))
				if containerStatus.Error == "" {
					containerStatus.Error = err.Error()
				}
			}
			return s.probeStubStatus(ctx, containerStatus)
		}
		if process, ok := s.probeOpenRestyProcessNamespace(); ok {
			process.Enabled = cfg.Enabled
			process.DefaultHTTPS = cfg.DefaultHTTPS
			process.Modules = append([]model.OpenRestyModule{}, cfg.Modules...)
			return process
		}
		status.Error = "未找到 OpenResty 可执行文件或运行进程"
		return status
	}
	status.Binary = bin
	status.ConfigPath = s.openRestyConfigPath()
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	versionOut, err := exec.CommandContext(probeCtx, bin, "-v").CombinedOutput()
	if err != nil {
		status.Error = strings.TrimSpace(string(versionOut))
		if status.Error == "" {
			status.Error = err.Error()
		}
		return status
	}
	status.Available = true
	status.Version = parseOpenRestyVersion(string(versionOut))
	if status.Version == "" {
		status.Version = cfg.Version
	}
	configCtx, cancelConfig := context.WithTimeout(ctx, 2*time.Second)
	defer cancelConfig()
	configOut, configErr := exec.CommandContext(configCtx, bin, "-t").CombinedOutput()
	status.ConfigValid = configErr == nil
	if configErr != nil && status.Error == "" {
		status.Error = strings.TrimSpace(string(configOut))
	}
	status = s.probeStubStatus(ctx, status)
	return status
}

// probeStubStatus 读取 nginx_stub_status 的实时计数；接口不可用时保留零值并返回探测状态。
func (s *WebsiteService) probeStubStatus(ctx context.Context, status OpenRestyStatus) OpenRestyStatus {
	endpoint := strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_STATUS_URL"))
	if endpoint == "" {
		endpoint = "http://127.0.0.1/status"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return status
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	response, err := client.Do(request)
	if err != nil {
		return status
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return status
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return status
	}
	metrics := parseStubStatus(string(body))
	status.Active, status.Accepts, status.Handled, status.Requests, status.Reading, status.Writing, status.Waiting = metrics.active, metrics.accepts, metrics.handled, metrics.requests, metrics.reading, metrics.writing, metrics.waiting
	return status
}

type stubMetrics struct {
	active, reading, writing, waiting int
	accepts, handled, requests        int64
}

func parseStubStatus(content string) stubMetrics {
	var m stubMetrics
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		f := strings.Fields(line)
		if len(f) >= 3 && strings.EqualFold(f[0], "Active") {
			m.active, _ = strconv.Atoi(f[len(f)-1])
		}
		if len(f) >= 4 && f[0] == "server" && i+1 < len(lines) {
			v := strings.Fields(lines[i+1])
			if len(v) >= 3 {
				m.accepts, _ = strconv.ParseInt(v[0], 10, 64)
				m.handled, _ = strconv.ParseInt(v[1], 10, 64)
				m.requests, _ = strconv.ParseInt(v[2], 10, 64)
			}
		}
		if len(f) >= 6 && f[0] == "Reading:" {
			m.reading, _ = strconv.Atoi(strings.TrimSuffix(f[1], ","))
			m.writing, _ = strconv.Atoi(strings.TrimSuffix(f[3], ","))
			m.waiting, _ = strconv.Atoi(strings.TrimSuffix(f[5], ","))
		}
	}
	return m
}

// probeOpenRestyProcessNamespace 从 procfs 识别独立挂载命名空间中的 Nginx/OpenResty 主进程。
// 无法进入命名空间时只报告真实进程、cgroup、配置和监听信息，不虚构 -t 校验结果。
func (s *WebsiteService) probeOpenRestyProcessNamespace() (OpenRestyStatus, bool) {
	if runtime.GOOS == "windows" {
		return OpenRestyStatus{}, false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return OpenRestyStatus{}, false
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		base := filepath.Join("/proc", entry.Name())
		exe, _ := os.Readlink(filepath.Join(base, "exe"))
		cmdline, _ := os.ReadFile(filepath.Join(base, "cmdline"))
		identity := strings.ToLower(filepath.Base(exe) + " " + strings.ReplaceAll(string(cmdline), "\x00", " "))
		if !strings.Contains(identity, "openresty") && !strings.Contains(identity, "nginx") {
			continue
		}
		cgroup, _ := os.ReadFile(filepath.Join(base, "cgroup"))
		status := OpenRestyStatus{Available: true, Binary: "proc://" + entry.Name() + "/exe", ProcessID: pid, Cgroup: strings.TrimSpace(string(cgroup)), Listening: processListeningPorts(base)}
		for _, configured := range []string{s.openRestyConfigPath(), "/etc/nginx/nginx.conf", "/usr/local/openresty/nginx/conf/nginx.conf"} {
			path := filepath.Join(base, "root", strings.TrimPrefix(filepath.Clean(configured), string(filepath.Separator)))
			if content, readErr := os.ReadFile(path); readErr == nil {
				status.ConfigPath = configured
				if balancedConfig(string(content)) {
					status.Error = "检测到独立命名空间中的 OpenResty，当前服务无法进入该命名空间执行配置语法检查"
				} else {
					status.Error = "检测到独立命名空间中的 OpenResty，但读取到的配置括号不匹配"
				}
				return status, true
			}
		}
		status.Error = "检测到独立命名空间中的 OpenResty，但无权限读取其配置文件"
		return status, true
	}
	return OpenRestyStatus{}, false
}

func processListeningPorts(procRoot string) []int {
	ports := make([]int, 0, 2)
	seen := map[int]bool{}
	for _, name := range []string{"tcp", "tcp6"} {
		content, err := os.ReadFile(filepath.Join(procRoot, "net", name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(content), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 || fields[3] != "0A" {
				continue
			}
			parts := strings.Split(fields[1], ":")
			if len(parts) != 2 {
				continue
			}
			port, err := strconv.ParseInt(parts[1], 16, 32)
			if err == nil && port > 0 && !seen[int(port)] {
				seen[int(port)] = true
				ports = append(ports, int(port))
			}
		}
	}
	sort.Ints(ports)
	return ports
}

func parseOpenRestyVersion(output string) string {
	for _, token := range strings.Fields(output) {
		if strings.HasPrefix(token, "openresty/") {
			return strings.TrimPrefix(token, "openresty/")
		}
		if strings.HasPrefix(token, "nginx/") {
			return strings.TrimPrefix(token, "nginx/")
		}
	}
	return ""
}
