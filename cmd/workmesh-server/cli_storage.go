// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// saveJSONFile 以临时文件替换方式保存受控 JSON 配置。
func saveJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// randomSecurityEntrance 生成无法读取系统随机源时的安全入口后备值。
func randomSecurityEntrance() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	raw := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "workmesh-entry"
	}
	for i := range raw {
		raw[i] = alphabet[int(raw[i])%len(alphabet)]
	}
	return string(raw)
}

// initializeDataDir 创建单进程服务启动所需的数据目录。
func initializeDataDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		dir = "./data"
	}
	abs, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return fmt.Errorf("解析数据目录失败: %w", err)
	}
	if filepath.Dir(abs) == abs {
		return errors.New("数据目录不能使用文件系统根目录")
	}
	if info, statErr := os.Stat(abs); statErr == nil && !info.IsDir() {
		return fmt.Errorf("数据目录不是目录: %s", abs)
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("检查数据目录失败: %w", statErr)
	}
	// 仅创建启动必需目录；应用、备份、运行时和上传目录由首次使用的功能按需创建。
	dirs := []string{"logs", "releases"}
	for _, name := range append([]string{""}, dirs...) {
		path := filepath.Join(abs, name)
		if err := os.MkdirAll(path, 0o750); err != nil {
			return fmt.Errorf("创建数据目录 %s 失败: %w", name, err)
		}
	}
	return nil
}
