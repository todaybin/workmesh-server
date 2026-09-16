// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// runtimeRecordFromRequest 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeRecordFromRequest(v map[string]any) (runtimeRecord, error) {
	name := runtimeString(v, "name")
	if name == "" {
		return runtimeRecord{}, errors.New("运行时名称不能为空")
	}
	if strings.ContainsAny(name, "\r\n\x00/\\") {
		return runtimeRecord{}, errors.New("运行时名称无效")
	}
	item := runtimeRecord{
		ID: runtimeString(v, "id", "runtimeId"), Name: name, Type: runtimeString(v, "type"),
		Mode:    runtimeString(v, "mode", "runtimeMode"),
		Version: runtimeString(v, "version"), CodeDir: runtimeString(v, "codeDir", "path"),
		WorkDir: runtimeString(v, "workDir"), Resource: runtimeString(v, "resource"), Source: runtimeString(v, "source"),
		Image: runtimeString(v, "image"), DockerCompose: runtimeString(v, "dockerCompose", "compose"),
		DownloadURL: runtimeString(v, "downloadUrl", "downloadURL"),
		Env:         runtimeString(v, "env"), AppDetailID: runtimeString(v, "appDetailID", "appDetailId"), AppID: runtimeString(v, "appID", "appId"),
		Remark: runtimeString(v, "remark"), TaskID: runtimeString(v, "taskID", "taskId"), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		Params: map[string]any{},
	}
	if item.Mode == "" {
		if rawMode := runtimeString(v, "runtime_mode"); rawMode != "" {
			item.Mode = rawMode
		} else if rawMode := runtimeString(item.Params, "RUNTIME_MODE", "MODE"); rawMode != "" {
			item.Mode = rawMode
		}
	}
	item.Mode = normalizeRuntimeMode(item.Mode)
	if item.Mode == "host" {
		item.Params["RUNTIME_MODE"] = "host"
	}
	if raw, ok := v["params"].(map[string]any); ok {
		for key, value := range raw {
			item.Params[key] = value
		}
	}
	if normalizeRuntimeTypeFilter(item.Type) == "node" {
		if item.Source != "" {
			item.Params["CONTAINER_PACKAGE_URL"] = item.Source
		}
		if install, ok := v["install"].(bool); ok {
			if install {
				item.Params["RUN_INSTALL"] = "1"
			} else {
				item.Params["RUN_INSTALL"] = "0"
			}
		}
	}
	if normalizeRuntimeTypeFilter(item.Type) == "php" {
		if item.Source != "" {
			item.Params["CONTAINER_PACKAGE_URL"] = item.Source
		}
		if raw, exists := item.Params["PHP_EXTENSIONS"]; exists {
			extensions, extensionErr := normalizePHPExtensions(raw)
			if extensionErr != nil {
				return runtimeRecord{}, extensionErr
			}
			item.Extensions = strings.Split(extensions, ",")
			if extensions == "" {
				item.Extensions = []string{}
			}
			item.Params["PHP_EXTENSIONS"] = extensions
		}
	}
	item.Container = runtimeString(v, "container", "containerName")
	if item.Container == "" {
		item.Container = runtimeString(item.Params, "CONTAINER_NAME", "containerName")
	}
	if item.Container == "" && runtimeInstallRequested(v, item) {
		item.Container = item.Name
	}
	if item.Mode == "host" && item.Container == "" {
		item.Container = item.Name
	}
	if item.Container != "" && !validDockerIdentifier(item.Container) {
		if runtimeInstallRequested(v, item) {
			return runtimeRecord{}, errors.New("容器名无效")
		}
		item.Container = ""
	}
	if rawPort, exists := v["port"]; exists {
		value, ok := runtimeNumberValue(rawPort)
		if !ok || value < 1 || value > 65535 {
			return runtimeRecord{}, errors.New("端口必须在 1-65535 范围内")
		}
		item.Port = value
	}
	for _, key := range []string{"exposedPorts", "environments", "volumes", "extraHosts"} {
		if raw, ok := v[key].([]any); ok {
			switch key {
			case "exposedPorts":
				item.ExposedPorts = raw
			case "environments":
				item.Environments = raw
			case "volumes":
				item.Volumes = raw
			case "extraHosts":
				item.ExtraHosts = raw
			}
		}
	}
	if item.Port == 0 && len(item.ExposedPorts) > 0 {
		if portMap, ok := item.ExposedPorts[0].(map[string]any); ok {
			item.Port, _ = runtimeNumberValue(portMap["hostPort"])
		}
	}
	if item.Port < 0 || item.Port > 65535 {
		return runtimeRecord{}, errors.New("端口必须在 1-65535 范围内")
	}
	if err := normalizeRuntimePorts(&item, runtimeInstallRequested(v, item)); err != nil {
		return runtimeRecord{}, err
	}
	if item.CodeDir != "" {
		clean, err := filepath.Abs(filepath.Clean(item.CodeDir))
		if err != nil {
			return runtimeRecord{}, errors.New("代码目录无效")
		}
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return runtimeRecord{}, errors.New("代码目录无效")
		}
		item.CodeDir = clean
	}
	if item.Mode == "host" {
		if strings.TrimSpace(runtimeString(item.Params, "EXEC_SCRIPT")) == "" {
			return runtimeRecord{}, errors.New("宿主机运行时启动命令不能为空")
		}
		if item.CodeDir == "" {
			return runtimeRecord{}, errors.New("宿主机运行时工作目录不能为空")
		}
		item.InstallPath = item.CodeDir
		item.ComposePath = ""
	}
	hydrateRuntimePaths(&item)
	if err := validateRuntimeCollections(item); err != nil {
		return runtimeRecord{}, err
	}
	if err := validateRuntimeCreateLocked(nil, item); err != nil {
		return runtimeRecord{}, err
	}
	return item, nil
}

// normalizeRuntimeMode keeps the persisted mode deliberately small and
// backwards compatible with the panel's optional runtimeMode field.
func normalizeRuntimeMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "host", "native", "local", "supervisor":
		return "host"
	case "", "docker", "compose":
		return "docker"
	default:
		return "docker"
	}
}

// validateRuntimeCollections 校验运行时参数和外部资源边界。
func validateRuntimeCollections(item runtimeRecord) error {
	for _, raw := range item.Environments {
		entry, ok := raw.(map[string]any)
		if !ok || !validEnvKey(runtimeString(entry, "key")) {
			return errors.New("环境变量参数无效")
		}
		if strings.ContainsRune(fmt.Sprint(entry["value"]), '\x00') {
			return errors.New("环境变量值无效")
		}
	}
	for _, raw := range item.Volumes {
		entry, ok := raw.(map[string]any)
		source, target := runtimeString(entry, "source"), runtimeString(entry, "target")
		mode := strings.ToLower(runtimeString(entry, "mode"))
		if !ok || source == "" || !filepath.IsAbs(target) || strings.ContainsAny(source+target, "\r\n\x00") || (mode != "" && mode != "ro" && mode != "rw") {
			return errors.New("卷映射参数无效")
		}
	}
	for _, raw := range item.ExtraHosts {
		entry, ok := raw.(map[string]any)
		hostname, ip := runtimeString(entry, "hostname", "host"), runtimeString(entry, "ip")
		if !ok || hostname == "" || len(hostname) > 253 || strings.ContainsAny(hostname, " /\\\r\n\x00") || net.ParseIP(ip) == nil {
			return errors.New("extra_hosts 参数无效")
		}
	}
	return nil
}

// runtimeNumberValue 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeNumberValue(raw any) (int, bool) {
	switch value := raw.(type) {
	case float64:
		return int(value), value == float64(int(value))
	case int:
		return value, true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(value))
		return n, err == nil
	default:
		return 0, false
	}
}

// normalizeRuntimePorts keeps the host and container port variables in sync
// with the 1Panel runtime contract. All language runtime templates expose
// ${APP_PORT} inside the container and ${PANEL_APP_PORT_HTTP} on the host.
// normalizeRuntimePorts 规范化运行时输入，保持原前端字段语义。
func normalizeRuntimePorts(item *runtimeRecord, required bool) error {
	if item == nil || normalizeRuntimeTypeFilter(item.Type) == "php" {
		return nil
	}
	if item.Params == nil {
		item.Params = map[string]any{}
	}
	containerPort := 0
	if raw, exists := item.Params["APP_PORT"]; exists {
		value, ok := runtimeNumberValue(raw)
		if !ok || value < 1 || value > 65535 {
			return errors.New("APP_PORT 必须在 1-65535 范围内")
		}
		containerPort = value
	}
	if containerPort == 0 && len(item.ExposedPorts) > 0 {
		if value, ok := runtimeNumberValue(runtimeMapValue(item.ExposedPorts[0], "containerPort")); ok {
			containerPort = value
		}
	}
	if containerPort == 0 {
		containerPort = item.Port
	}
	if containerPort == 0 {
		if required {
			return errors.New("运行时容器端口不能为空，请设置 params.APP_PORT、exposedPorts.containerPort 或 port")
		}
		return nil
	}
	if containerPort < 1 || containerPort > 65535 {
		return errors.New("APP_PORT 必须在 1-65535 范围内")
	}
	item.Params["APP_PORT"] = containerPort
	if item.Port == 0 && len(item.ExposedPorts) > 0 {
		item.Port, _ = runtimeNumberValue(runtimeMapValue(item.ExposedPorts[0], "hostPort"))
	}
	if item.Port == 0 {
		item.Port = containerPort
	}
	if item.Port < 1 || item.Port > 65535 {
		return errors.New("端口必须在 1-65535 范围内")
	}
	return nil
}

// runtimeMapValue 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeMapValue(raw any, key string) any {
	if value, ok := raw.(map[string]any); ok {
		return value[key]
	}
	return nil
}

// validateRuntimeCreateLocked 校验运行时参数和外部资源边界。
func validateRuntimeCreateLocked(items []runtimeRecord, candidate runtimeRecord) error {
	seenHost := map[string]bool{}
	seenContainer := map[string]bool{}
	for _, raw := range candidate.ExposedPorts {
		portMap, ok := raw.(map[string]any)
		if !ok {
			return errors.New("exposedPorts 参数无效")
		}
		host, hostOK := runtimeNumberValue(portMap["hostPort"])
		container, containerOK := runtimeNumberValue(portMap["containerPort"])
		if !hostOK || !containerOK || host < 1 || host > 65535 || container < 1 || container > 65535 {
			return errors.New("映射端口必须在 1-65535 范围内")
		}
		protocol := strings.ToLower(runtimeString(portMap, "protocol"))
		if protocol == "" {
			protocol = "tcp"
		}
		if protocol != "tcp" && protocol != "udp" {
			return errors.New("端口协议只允许 tcp 或 udp")
		}
		hostKey, containerKey := fmt.Sprintf("%d/%s", host, protocol), fmt.Sprintf("%d/%s", container, protocol)
		if seenHost[hostKey] || seenContainer[containerKey] {
			return errors.New("映射端口重复")
		}
		seenHost[hostKey], seenContainer[containerKey] = true, true
	}
	for _, item := range items {
		if strings.EqualFold(item.Name, candidate.Name) || (candidate.Container != "" && strings.EqualFold(item.Container, candidate.Container)) {
			return errors.New("运行时名称或容器名已存在")
		}
		if candidate.CodeDir != "" && strings.EqualFold(item.CodeDir, candidate.CodeDir) {
			return errors.New("代码目录已被其他运行时使用")
		}
		if candidate.Image != "" && item.Image != "" && strings.EqualFold(item.Image, candidate.Image) {
			return errors.New("运行时镜像已被使用")
		}
		if candidate.Port > 0 && item.Port == candidate.Port || runtimePortsConflict(item.ExposedPorts, candidate.ExposedPorts) {
			return errors.New("运行时端口已被占用")
		}
	}
	return nil
}

// runtimePortsConflict 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimePortsConflict(left, right []any) bool {
	seen := map[string]struct{}{}
	for _, raw := range left {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		port, valid := runtimeNumberValue(entry["hostPort"])
		if !valid {
			continue
		}
		protocol := strings.ToLower(runtimeString(entry, "protocol"))
		if protocol == "" {
			protocol = "tcp"
		}
		seen[fmt.Sprintf("%d/%s", port, protocol)] = struct{}{}
	}
	for _, raw := range right {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		port, valid := runtimeNumberValue(entry["hostPort"])
		if !valid {
			continue
		}
		protocol := strings.ToLower(runtimeString(entry, "protocol"))
		if protocol == "" {
			protocol = "tcp"
		}
		if _, exists := seen[fmt.Sprintf("%d/%s", port, protocol)]; exists {
			return true
		}
	}
	return false
}

// runtimeInstallRequested 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeInstallRequested(v map[string]any, item runtimeRecord) bool {
	if normalizeRuntimeMode(item.Mode) == "host" {
		return false
	}
	if value, ok := v["install"].(bool); ok {
		return value
	}
	return strings.EqualFold(item.Resource, "appstore") || item.Image != "" || item.DockerCompose != ""
}
