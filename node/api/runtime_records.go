// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// validateRuntimeCodeDirectory 校验运行时参数和外部资源边界。
func validateRuntimeCodeDirectory(item runtimeRecord) error {
	if normalizeRuntimeTypeFilter(item.Type) == "php" {
		return nil
	}
	if strings.TrimSpace(item.CodeDir) == "" {
		return errors.New("代码目录不能为空")
	}
	realCode, err := filepath.EvalSymlinks(item.CodeDir)
	if err != nil {
		return errors.New("代码目录不存在或无法访问")
	}
	info, err := os.Stat(realCode)
	if err != nil || !info.IsDir() {
		return errors.New("代码目录必须是目录")
	}
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	runtimeRoot, err := filepath.Abs(filepath.Join(root, "runtimes"))
	if err != nil {
		return err
	}
	realRoot, err := filepath.EvalSymlinks(filepath.Dir(runtimeRoot))
	if err == nil {
		runtimeRoot = filepath.Join(realRoot, filepath.Base(runtimeRoot))
	}
	relative, err := filepath.Rel(runtimeRoot, realCode)
	if err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))) {
		return errors.New("代码目录不能位于运行时目录内")
	}
	return nil
}

// runtimeWebsiteReferences 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeWebsiteReferences(ctx context.Context, runtimeID string) ([]map[string]any, error) {
	resources := []map[string]any{}
	if strings.TrimSpace(runtimeID) == "" {
		return resources, nil
	}
	repository, err := sharedRuntimeRepository()
	if err != nil {
		return resources, nil
	}
	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, err := repository.QueryContext(queryCtx, `SELECT id,primary_domain FROM websites WHERE runtime_id=? ORDER BY id LIMIT 201`, runtimeID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return resources, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		if len(resources) >= 200 {
			return nil, errors.New("运行时网站引用超过 200 条上限")
		}
		var id int64
		var domain string
		if err := rows.Scan(&id, &domain); err != nil {
			return nil, err
		}
		resources = append(resources, map[string]any{"id": id, "name": domain, "type": "website"})
	}
	return resources, rows.Err()
}

// runtimeByID 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeByID(s *runtimeStore, id string) (runtimeRecord, int) {
	if strings.TrimSpace(id) == "" {
		return runtimeRecord{}, -1
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for index, item := range s.state.Runtimes {
		if item.ID == id {
			// Older SQLite records predate persisted Compose/container fields.
			// Hydrate them at every operational entry point so stop/restart and
			// secondary PHP extension installs use the same canonical paths/env.
			hydrateRuntimePaths(&item)
			if item.Container == "" {
				item.Container = runtimeString(item.Params, "CONTAINER_NAME", "containerName")
			}
			if normalizeRuntimeTypeFilter(item.Type) == "php" && item.Image == "" {
				if version := runtimeString(item.Params, "PHP_VERSION"); version != "" {
					item.Image = "1panel-php-fpm:" + version
				}
			}
			return item, index
		}
	}
	return runtimeRecord{}, -1
}

// runtimeByIDLocked resolves a runtime while the caller holds s.mu. Keeping
// this helper separate prevents delete operations from using a stale slice
// index after releasing the lock for Docker/filesystem cleanup.
// runtimeByIDLocked 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeByIDLocked(items []runtimeRecord, id string) (runtimeRecord, int) {
	if strings.TrimSpace(id) == "" {
		return runtimeRecord{}, -1
	}
	for index, item := range items {
		if item.ID == id {
			hydrateRuntimePaths(&item)
			if item.Container == "" {
				item.Container = runtimeString(item.Params, "CONTAINER_NAME", "containerName")
			}
			if normalizeRuntimeTypeFilter(item.Type) == "php" && item.Image == "" {
				if version := runtimeString(item.Params, "PHP_VERSION"); version != "" {
					item.Image = "1panel-php-fpm:" + version
				}
			}
			return item, index
		}
	}
	return runtimeRecord{}, -1
}

func removeRuntimeByID(items []runtimeRecord, id string) []runtimeRecord {
	for index, item := range items {
		if item.ID == id {
			return append(items[:index], items[index+1:]...)
		}
	}
	return items
}

// mergeRuntimeUpdate 执行运行时相关处理并返回可观测错误。
func mergeRuntimeUpdate(current runtimeRecord, body map[string]any) (runtimeRecord, error) {
	updated := current
	// 参数映射属于共享快照，更新前复制以避免校验失败污染原记录及并发读写。
	updated.Params = cloneRuntimeMap(current.Params)
	if err := mergeRuntimeUpdateFields(&updated, body); err != nil {
		return runtimeRecord{}, err
	}
	if normalizeRuntimeTypeFilter(updated.Type) == "php" {
		extensions, err := normalizePHPExtensions(updated.Params["PHP_EXTENSIONS"])
		if err != nil {
			return runtimeRecord{}, err
		}
		updated.Params["PHP_EXTENSIONS"] = extensions
		updated.Extensions = []string{}
		if extensions != "" {
			updated.Extensions = strings.Split(extensions, ",")
		}
		phpVersion := runtimeString(updated.Params, "PHP_VERSION")
		if phpVersion == "" {
			return runtimeRecord{}, errors.New("PHP_VERSION 不能为空")
		}
		updated.Image = "1panel-php-fpm:" + phpVersion
	}
	if err := normalizeRuntimePorts(&updated, runtimeInstallRequested(body, updated)); err != nil {
		return runtimeRecord{}, err
	}
	if err := validateRuntimeCollections(updated); err != nil {
		return runtimeRecord{}, err
	}
	if err := validateRuntimeCreateLocked(nil, updated); err != nil {
		return runtimeRecord{}, err
	}
	return updated, nil
}

// mergeRuntimeUpdateFields 应用通用字段更新，集中校验数组、端口、容器名和备注。
func mergeRuntimeUpdateFields(updated *runtimeRecord, body map[string]any) error {
	if mode := runtimeString(body, "mode", "runtimeMode", "runtime_mode"); mode != "" {
		updated.Mode = normalizeRuntimeMode(mode)
	}
	if params, exists := body["params"]; exists {
		values, ok := params.(map[string]any)
		if !ok {
			return errors.New("params 必须是对象")
		}
		updated.Params = cloneRuntimeMap(values)
	}
	if codeDir, exists := body["codeDir"]; exists {
		value, ok := codeDir.(string)
		if !ok || strings.TrimSpace(value) == "" {
			return errors.New("codeDir 必须是非空字符串")
		}
		updated.CodeDir = filepath.Clean(value)
	}
	if workDir, exists := body["workDir"]; exists {
		value, ok := workDir.(string)
		if !ok {
			return errors.New("workDir 必须是字符串")
		}
		updated.WorkDir = strings.TrimSpace(value)
	}
	if source, exists := body["source"]; exists {
		value, ok := source.(string)
		if !ok {
			return errors.New("source 必须是字符串")
		}
		updated.Source = strings.TrimSpace(value)
		if updated.Source != "" {
			updated.Params["CONTAINER_PACKAGE_URL"] = updated.Source
		}
	}
	if install, ok := body["install"].(bool); ok && normalizeRuntimeTypeFilter(updated.Type) == "node" {
		if install {
			updated.Params["RUN_INSTALL"] = "1"
		} else {
			updated.Params["RUN_INSTALL"] = "0"
		}
	}
	if raw, exists := body["port"]; exists {
		port, ok := runtimeNumberValue(raw)
		if !ok || port < 1 || port > 65535 {
			return errors.New("端口必须在 1-65535 范围内")
		}
		updated.Port = port
	}
	for _, field := range []string{"exposedPorts", "environments", "volumes", "extraHosts"} {
		raw, exists := body[field]
		if !exists {
			continue
		}
		values, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("%s 必须是数组", field)
		}
		switch field {
		case "exposedPorts":
			updated.ExposedPorts = values
		case "environments":
			updated.Environments = values
		case "volumes":
			updated.Volumes = values
		case "extraHosts":
			updated.ExtraHosts = values
		}
	}
	if value := runtimeString(updated.Params, "CONTAINER_NAME"); value != "" {
		if !validDockerIdentifier(value) {
			return errors.New("容器名无效")
		}
		updated.Container = value
	}
	updated.Mode = normalizeRuntimeMode(updated.Mode)
	if updated.Mode == "host" {
		updated.Params["RUNTIME_MODE"] = "host"
		if strings.TrimSpace(runtimeString(updated.Params, "EXEC_SCRIPT")) == "" {
			return errors.New("宿主机运行时启动命令不能为空")
		}
		if strings.TrimSpace(updated.CodeDir) == "" {
			return errors.New("宿主机运行时工作目录不能为空")
		}
		updated.InstallPath = updated.CodeDir
		updated.ComposePath = ""
	}
	if remark, exists := body["remark"]; exists {
		value, ok := remark.(string)
		if !ok {
			return errors.New("remark 必须是字符串")
		}
		updated.Remark = strings.TrimSpace(value)
	}
	return nil
}

// runtimeEnvironment 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeEnvironment(item runtimeRecord) (map[string]any, error) {
	if err := normalizeRuntimePorts(&item, normalizeRuntimeTypeFilter(item.Type) != "php"); err != nil {
		return nil, err
	}
	values := cloneRuntimeMap(item.Params)
	for _, raw := range item.Environments {
		entry, _ := raw.(map[string]any)
		if key := runtimeString(entry, "key"); validEnvKey(key) {
			values[key] = fmt.Sprint(entry["value"])
		}
	}
	if appPort, ok := item.Params["APP_PORT"]; ok {
		values["APP_PORT"] = appPort
	}
	values["CONTAINER_NAME"] = item.Container
	if item.Port > 0 {
		values["PANEL_APP_PORT_HTTP"] = item.Port
	}
	values["CODE_DIR"] = item.CodeDir
	// Compose 使用这两个变量构造 PHP 及通用运行时挂载/时区；缺失时 Compose
	// 会把变量替换为空字符串，生成非法的 `:/www/` 挂载。
	websiteDirValue, websiteDirSet := values["PANEL_WEBSITE_DIR"]
	if !websiteDirSet || strings.TrimSpace(fmt.Sprint(websiteDirValue)) == "" {
		websiteDir := strings.TrimSpace(item.CodeDir)
		if websiteDir == "" {
			websiteDir = "/www/wwwroot"
		}
		values["PANEL_WEBSITE_DIR"] = websiteDir
	}
	tzValue, tzSet := values["TZ"]
	if !tzSet || strings.TrimSpace(fmt.Sprint(tzValue)) == "" {
		values["TZ"] = "Asia/Shanghai"
	}
	switch normalizeRuntimeTypeFilter(item.Type) {
	case "php":
		phpVersion := runtimeString(item.Params, "PHP_VERSION")
		if phpVersion == "" {
			phpVersion = strings.TrimSpace(item.Version)
		}
		if phpVersion != "" {
			values["PHP_VERSION"] = phpVersion
			values["IMAGE_NAME"] = "1panel-php-fpm:" + phpVersion
		}
	case "java":
		values["JAVA_VERSION"] = item.Version
	case "node":
		values["NODE_VERSION"] = item.Version
	case "go":
		values["GO_VERSION"] = item.Version
	case "python":
		values["PYTHON_VERSION"] = item.Version
	case "dotnet":
		values["DOTNET_VERSION"] = item.Version
	}
	return values, nil
}

// applyRuntimeConfiguration 执行运行时相关处理并返回可观测错误。
func applyRuntimeConfiguration(executor runtimeCommandExecutor, current, updated runtimeRecord) error {
	if strings.TrimSpace(updated.ComposePath) == "" {
		return nil
	}
	installDir := filepath.Dir(updated.ComposePath)
	envPath := filepath.Join(installDir, ".env")
	overridePath := filepath.Join(installDir, "docker-compose.override.json")
	oldEnv, envErr := os.ReadFile(envPath)
	oldOverride, overrideErr := os.ReadFile(overridePath)
	rollbackFiles := func() {
		if envErr == nil {
			_ = writeAtomicRuntimeFile(envPath, oldEnv)
		}
		if overrideErr == nil {
			_ = writeAtomicRuntimeFile(overridePath, oldOverride)
		} else {
			_ = os.Remove(overridePath)
		}
	}
	values, err := runtimeEnvironment(updated)
	if err != nil {
		return err
	}
	if err := writeRuntimeEnv(envPath, values); err != nil {
		return fmt.Errorf("写入运行时环境变量失败: %w", err)
	}
	if err := writeRuntimeComposeOverride(installDir, updated); err != nil {
		rollbackFiles()
		return fmt.Errorf("写入运行时 Compose 覆盖配置失败: %w", err)
	}
	if normalizeRuntimeTypeFilter(updated.Type) == "php" {
		if err := executeRuntimeInstall(executor, updated, func(string, string) {}); err != nil {
			rollbackFiles()
			_ = operateRuntimeContainer(executor, current, "down")
			_ = operateRuntimeContainer(executor, current, "up")
			return fmt.Errorf("重建 PHP 运行时失败: %w", err)
		}
		return nil
	}
	if err := operateRuntimeContainer(executor, current, "down"); err != nil {
		rollbackFiles()
		return err
	}
	if err := operateRuntimeContainer(executor, updated, "up"); err != nil {
		rollbackFiles()
		_ = operateRuntimeContainer(executor, current, "up")
		return err
	}
	return nil
}

// hydrateRuntimeFromAppStore 执行运行时相关处理并返回可观测错误。
func hydrateRuntimeFromAppStore(item *runtimeRecord) error {
	if item == nil {
		return errors.New("运行时参数不能为空")
	}
	runtimeType := normalizeRuntimeTypeFilter(item.Type)
	if runtimeType != "php" && runtimeType != "go" && runtimeType != "java" && runtimeType != "node" && runtimeType != "python" && runtimeType != "dotnet" {
		return errors.New("运行时类型无效")
	}
	item.Type = runtimeType
	if item.AppDetailID == "" {
		if strings.EqualFold(item.Resource, "appstore") {
			return errors.New("应用详情 ID 不能为空")
		}
		return nil
	}
	store := getAppStore()
	store.mu.RLock()
	defer store.mu.RUnlock()
	for _, app := range store.state.Catalog {
		if !appMatchesType(app, runtimeType) {
			continue
		}
		if runtimeType == "php" && !strings.EqualFold(strings.TrimSpace(app.Key), "php") {
			continue
		}
		for _, version := range app.Versions {
			if !strings.EqualFold(strings.TrimSpace(version.ID), strings.TrimSpace(item.AppDetailID)) {
				continue
			}
			if item.Version == "" {
				item.Version = version.Version
			} else if !strings.EqualFold(item.Version, version.Version) {
				return errors.New("运行时版本与应用详情不匹配")
			}
			if item.DockerCompose == "" {
				item.DockerCompose = version.DockerCompose
				if item.DockerCompose == "" && version.ComposeURL != "" {
					item.DockerCompose = fetchRemoteCompose(version.ComposeURL)
				}
			}
			if item.Image == "" {
				if repository := appRuntimeImage(app, item.DockerCompose); repository != "" {
					item.Image = repository + ":" + item.Version
				}
			}
			if item.DownloadURL == "" {
				item.DownloadURL = version.DownloadURL
			}
			item.PackageModified = version.LastModified
			if item.AppID == "" {
				item.AppID = app.ID
			}
			if item.Resource == "" {
				item.Resource = "appstore"
			}
			return nil
		}
	}
	return errors.New("应用详情不存在或不属于所选运行时类型")
}

// removeRuntimeInstallPath 执行运行时相关处理并返回可观测错误。
func removeRuntimeInstallPath(path string) error {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	base, err := filepath.Abs(filepath.Join(root, "runtimes"))
	if err != nil {
		return err
	}
	target, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("运行时目录不在数据目录内")
	}
	return os.RemoveAll(target)
}

// normalizePHPExtensions 规范化运行时输入，保持原前端字段语义。
func normalizePHPExtensions(raw any) (string, error) {
	values := make([]string, 0)
	switch value := raw.(type) {
	case nil:
		return "", nil
	case string:
		values = strings.Split(value, ",")
	case []string:
		values = append(values, value...)
	case []any:
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				return "", errors.New("PHP 扩展必须是字符串数组")
			}
			values = append(values, text)
		}
	default:
		return "", errors.New("PHP 扩展格式无效")
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if !phpExtensionNamePattern.MatchString(value) {
			return "", fmt.Errorf("PHP 扩展名称无效: %s", value)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return strings.Join(result, ","), nil
}
