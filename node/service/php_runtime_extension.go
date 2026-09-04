// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxPHPRuntimeConfigBytes int64 = 2 << 20

var (
	phpExtensionNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
	phpModuleFilePattern    = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}\.so$`)
)

// PHPExtensionRemoval 描述一次 PHP 扩展文件卸载。
type PHPExtensionRemoval struct {
	RuntimeDir   string
	ExtensionDir string
	Name         string
	ModuleFile   string
	Extensions   []string
}

// PHPExtensionRemovalTransaction 保存可回滚的扩展文件变更。
// 调用方只有在容器成功重启后才应 Commit；失败路径必须调用 Rollback。
type PHPExtensionRemovalTransaction struct {
	phpINIPath string
	phpINI     []byte
	envPath    string
	env        []byte
	moved      []phpExtensionMovedFile
	finished   bool
}

type phpExtensionMovedFile struct {
	original string
	backup   string
}

// PreparePHPExtensionRemoval 原子更新 PHP 配置并暂存待删除文件。
// 所有目标都必须位于已解析符号链接的运行时目录内，避免扩展名影响宿主机文件。
func PreparePHPExtensionRemoval(spec PHPExtensionRemoval) (*PHPExtensionRemovalTransaction, error) {
	if !phpExtensionNamePattern.MatchString(spec.Name) {
		return nil, errors.New("PHP 扩展名称无效")
	}
	if !phpModuleFilePattern.MatchString(spec.ModuleFile) || filepath.Base(spec.ModuleFile) != spec.ModuleFile {
		return nil, errors.New("PHP 扩展模块文件名无效")
	}

	root, err := resolvedDirectory(spec.RuntimeDir)
	if err != nil {
		return nil, fmt.Errorf("解析 PHP 运行时目录失败: %w", err)
	}
	extensionDir, err := resolvedDirectory(spec.ExtensionDir)
	if err != nil {
		return nil, fmt.Errorf("解析 PHP 扩展目录失败: %w", err)
	}
	if !pathWithin(root, extensionDir) {
		return nil, errors.New("PHP 扩展目录越界")
	}

	modulePath, err := checkedRuntimePath(root, filepath.Join(extensionDir, spec.ModuleFile))
	if err != nil {
		return nil, err
	}
	iniPath, err := checkedRuntimePath(root, filepath.Join(root, "conf", "conf.d", "docker-php-ext-"+spec.Name+".ini"))
	if err != nil {
		return nil, err
	}
	phpINIPath, err := checkedRuntimePath(root, filepath.Join(root, "conf", "php.ini"))
	if err != nil {
		return nil, err
	}
	envPath, err := checkedRuntimePath(root, filepath.Join(root, ".env"))
	if err != nil {
		return nil, err
	}

	phpINI, err := readRegularFileLimited(phpINIPath, maxPHPRuntimeConfigBytes)
	if err != nil {
		return nil, fmt.Errorf("读取 php.ini 失败: %w", err)
	}
	env, err := readRegularFileLimited(envPath, maxPHPRuntimeConfigBytes)
	if err != nil {
		return nil, fmt.Errorf("读取运行时环境变量失败: %w", err)
	}

	tx := &PHPExtensionRemovalTransaction{phpINIPath: phpINIPath, phpINI: phpINI, envPath: envPath, env: env}
	for _, target := range []string{modulePath, iniPath} {
		if err := tx.stageFile(target); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	if err := writeServiceFileAtomic(phpINIPath, removePHPExtensionReference(phpINI, spec.ModuleFile), 0o600); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("更新 php.ini 失败: %w", err)
	}
	if err := writeServiceFileAtomic(envPath, updateDotEnvValue(env, "PHP_EXTENSIONS", strings.Join(spec.Extensions, ",")), 0o600); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("更新 PHP_EXTENSIONS 失败: %w", err)
	}
	return tx, nil
}

func (tx *PHPExtensionRemovalTransaction) stageFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("PHP 扩展目标不是普通文件: %s", filepath.Base(path))
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".workmesh-php-extension-*")
	if err != nil {
		return err
	}
	backup := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(backup)
		return err
	}
	if err := os.Remove(backup); err != nil {
		return err
	}
	if err := os.Rename(path, backup); err != nil {
		return err
	}
	tx.moved = append(tx.moved, phpExtensionMovedFile{original: path, backup: backup})
	return nil
}

// Commit 完成卸载并清理同目录暂存文件。
func (tx *PHPExtensionRemovalTransaction) Commit() {
	if tx == nil || tx.finished {
		return
	}
	for _, file := range tx.moved {
		_ = os.Remove(file.backup)
	}
	tx.finished = true
}

// Rollback 恢复卸载前的配置和扩展文件。
func (tx *PHPExtensionRemovalTransaction) Rollback() error {
	if tx == nil || tx.finished {
		return nil
	}
	var rollbackErrors []string
	if err := writeServiceFileAtomic(tx.phpINIPath, tx.phpINI, 0o600); err != nil {
		rollbackErrors = append(rollbackErrors, "恢复 php.ini 失败: "+err.Error())
	}
	if err := writeServiceFileAtomic(tx.envPath, tx.env, 0o600); err != nil {
		rollbackErrors = append(rollbackErrors, "恢复 .env 失败: "+err.Error())
	}
	for index := len(tx.moved) - 1; index >= 0; index-- {
		file := tx.moved[index]
		if err := os.Rename(file.backup, file.original); err != nil {
			rollbackErrors = append(rollbackErrors, "恢复 "+filepath.Base(file.original)+" 失败: "+err.Error())
		}
	}
	tx.finished = true
	if len(rollbackErrors) > 0 {
		return errors.New(strings.Join(rollbackErrors, "; "))
	}
	return nil
}

func resolvedDirectory(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("不是目录")
		}
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func checkedRuntimePath(root, path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil || !pathWithin(root, abs) {
		return "", errors.New("PHP 扩展文件路径越界")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", fmt.Errorf("解析 PHP 扩展文件父目录失败: %w", err)
	}
	if !pathWithin(root, parent) {
		return "", errors.New("PHP 扩展文件父目录越界")
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func readRegularFileLimited(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("文件不是普通文件或超过大小限制")
	}
	return os.ReadFile(path)
}

func removePHPExtensionReference(content []byte, moduleFile string) []byte {
	lines := strings.Split(string(content), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, ";") && !strings.HasPrefix(trimmed, "#") {
			key, value, found := strings.Cut(trimmed, "=")
			key = strings.ToLower(strings.TrimSpace(key))
			value = strings.Trim(strings.TrimSpace(value), "\"'")
			if found && (key == "extension" || key == "zend_extension") && filepath.Base(value) == moduleFile {
				continue
			}
		}
		kept = append(kept, line)
	}
	return []byte(strings.Join(kept, "\n"))
}

func updateDotEnvValue(content []byte, key, value string) []byte {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	lines := make([]string, 0)
	written := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		name, _, found := strings.Cut(trimmed, "=")
		if found && strings.TrimSpace(name) == key {
			if !written {
				lines = append(lines, key+"="+value)
				written = true
			}
			continue
		}
		lines = append(lines, line)
	}
	if !written {
		lines = append(lines, key+"="+value)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func writeServiceFileAtomic(path string, content []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".workmesh-runtime-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
