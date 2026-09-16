// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/service"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// phpExtensionDefinitionForName 处理 PHP 运行时配置或扩展操作。
func phpExtensionDefinitionForName(name string) phpExtensionDefinition {
	for _, definition := range phpExtensionCatalog {
		if strings.EqualFold(definition.Name, name) {
			return definition
		}
	}
	return phpExtensionDefinition{Name: strings.ToLower(name), Check: strings.ToLower(name), File: strings.ToLower(name) + ".so"}
}

// updatedPHPExtensionNames 执行运行时相关处理并返回可观测错误。
func updatedPHPExtensionNames(current []string, definition phpExtensionDefinition, install bool) []string {
	values := make(map[string]string, len(current)+1)
	for _, value := range current {
		value = strings.TrimSpace(value)
		if value != "" {
			values[strings.ToLower(value)] = value
		}
	}
	delete(values, strings.ToLower(definition.Name))
	delete(values, strings.ToLower(definition.Check))
	if install {
		values[strings.ToLower(definition.Name)] = definition.Name
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i]) < strings.ToLower(result[j]) })
	return result
}

// installPHPExtension 执行运行时相关处理并返回可观测错误。
func installPHPExtension(executor runtimeCommandExecutor, item runtimeRecord, extension string) error {
	return installPHPExtensionWithOutput(executor, item, extension, nil)
}

// installPHPExtensionWithOutput 执行运行时相关处理并返回可观测错误。
func installPHPExtensionWithOutput(executor runtimeCommandExecutor, item runtimeRecord, extension string, output func(string, []byte)) error {
	if strings.TrimSpace(item.Container) == "" || strings.TrimSpace(item.Image) == "" {
		return errors.New("PHP 运行时缺少容器名或目标镜像")
	}
	if _, err := runtimeCommandWithOutput(executor, item, time.Hour, output, "exec", "-i", item.Container, "install-ext", extension); err != nil {
		return fmt.Errorf("安装 PHP 扩展失败: %w", err)
	}
	if _, err := runtimeCommand(executor, item, 15*time.Minute, "commit", item.Container, item.Image); err != nil {
		return fmt.Errorf("提交 PHP 扩展镜像失败: %w", err)
	}
	if err := restartRuntimeContainer(executor, item); err != nil {
		return fmt.Errorf("重启 PHP 运行时失败: %w", err)
	}
	return nil
}

// restartRuntimeContainer 执行运行时相关处理并返回可观测错误。
func restartRuntimeContainer(executor runtimeCommandExecutor, item runtimeRecord) error {
	if err := operateRuntimeContainer(executor, item, "down"); err != nil {
		return err
	}
	return operateRuntimeContainer(executor, item, "up")
}

// uninstallPHPExtension 执行运行时相关处理并返回可观测错误。
func uninstallPHPExtension(executor runtimeCommandExecutor, current, updated runtimeRecord, definition phpExtensionDefinition) error {
	extensionDir, err := phpRuntimeExtensionDirectory(current)
	if err != nil {
		return err
	}
	tx, err := service.PreparePHPExtensionRemoval(service.PHPExtensionRemoval{
		RuntimeDir: current.InstallPath, ExtensionDir: extensionDir, Name: definition.Name,
		ModuleFile: definition.File, Extensions: updated.Extensions,
	})
	if err != nil {
		return fmt.Errorf("准备卸载 PHP 扩展失败: %w", err)
	}
	if err := restartRuntimeContainer(executor, current); err != nil {
		rollbackErr := tx.Rollback()
		_ = restartRuntimeContainer(executor, current)
		if rollbackErr != nil {
			return fmt.Errorf("卸载 PHP 扩展后重启失败，且文件回滚失败: %v: %w", rollbackErr, err)
		}
		return fmt.Errorf("卸载 PHP 扩展后重启失败，已恢复旧文件: %w", err)
	}
	tx.Commit()
	return nil
}

// phpRuntimeExtensionDirectory 处理 PHP 运行时配置或扩展操作。
func phpRuntimeExtensionDirectory(item runtimeRecord) (string, error) {
	if strings.TrimSpace(item.InstallPath) == "" {
		return "", errors.New("PHP 运行时尚未完成安装")
	}
	if info, err := os.Stat(item.InstallPath); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("不是目录")
		}
		return "", fmt.Errorf("PHP 运行时目录不存在: %w", err)
	}
	root := filepath.Join(item.InstallPath, "extensions")
	if value := strings.TrimSpace(runtimeString(item.Params, "EXTENSION_DIR")); value != "" {
		name := filepath.Base(filepath.Clean(value))
		if strings.HasPrefix(name, "no-debug-non-zts-") {
			candidate := filepath.Join(root, name)
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate, nil
			}
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("读取 PHP 扩展目录失败: %w", err)
	}
	candidates := make([]string, 0, 1)
	for _, entry := range entries {
		if entry.IsDir() && entry.Type()&os.ModeSymlink == 0 && strings.HasPrefix(entry.Name(), "no-debug-non-zts-") {
			candidates = append(candidates, filepath.Join(root, entry.Name()))
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) == 0 {
		return root, nil
	}
	return "", errors.New("PHP 扩展目录不唯一，请检查 EXTENSION_DIR")
}

// phpRuntimeConfigPath 处理 PHP 运行时配置或扩展操作。
func phpRuntimeConfigPath(item runtimeRecord, kind string) (string, error) {
	if normalizeRuntimeTypeFilter(item.Type) != "php" || strings.TrimSpace(item.InstallPath) == "" {
		return "", errors.New("PHP 运行时尚未完成安装")
	}
	var name string
	switch kind {
	case "php", "config":
		name = "php.ini"
	case "fpm":
		name = "php-fpm.conf"
	default:
		return "", errors.New("PHP 配置文件类型无效")
	}
	path := filepath.Join(item.InstallPath, "conf", name)
	root, _ := filepath.Abs(item.InstallPath)
	target, _ := filepath.Abs(path)
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("PHP 配置文件路径越界")
	}
	return target, nil
}

// readRuntimeConfigFile 读取运行时配置或外部状态，并限制资源使用。
func readRuntimeConfigFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("PHP 配置文件不存在")
	}
	if info.Size() > 2<<20 {
		return nil, errors.New("PHP 配置文件超过 2 MiB 限制")
	}
	return os.ReadFile(path)
}

// parseRuntimeINI 解析运行时输入并返回结构化结果。
func parseRuntimeINI(content string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if found && strings.TrimSpace(key) != "" {
			values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"")
		}
	}
	return values
}

var phpINIKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,127}$`)

// updateRuntimeINI 执行运行时相关处理并返回可观测错误。
func updateRuntimeINI(content string, updates map[string]string) (string, error) {
	for key, value := range updates {
		if !phpINIKeyPattern.MatchString(key) || strings.ContainsAny(value, "\r\n\x00") {
			return "", fmt.Errorf("PHP 配置项无效: %s", key)
		}
	}
	seen := map[string]bool{}
	lines := strings.Split(content, "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, _, found := strings.Cut(trimmed, "=")
		key = strings.TrimSpace(key)
		value, exists := updates[key]
		if found && exists {
			lines[index] = key + " = " + value
			seen[key] = true
		}
	}
	keys := make([]string, 0, len(updates))
	for key := range updates {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, key+" = "+updates[key])
	}
	return strings.Join(lines, "\n"), nil
}

// updatePHPFileAndRestart 执行运行时相关处理并返回可观测错误。
func updatePHPFileAndRestart(executor runtimeCommandExecutor, item runtimeRecord, path string, content []byte) error {
	old, err := readRuntimeConfigFile(path)
	if err != nil {
		return err
	}
	if len(content) > 2<<20 || bytes.IndexByte(content, 0) >= 0 {
		return errors.New("PHP 配置内容无效或超过 2 MiB 限制")
	}
	if err := writeAtomicRuntimeFile(path, content); err != nil {
		return err
	}
	if err := operateRuntimeContainer(executor, item, "restart"); err != nil {
		_ = writeAtomicRuntimeFile(path, old)
		_ = operateRuntimeContainer(executor, item, "restart")
		return fmt.Errorf("应用 PHP 配置失败，已恢复旧文件: %w", err)
	}
	return nil
}
