// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package i18n 提供服务端任务、日志和错误消息使用的多语言资源加载。
package i18n

import (
	"bufio"
	"bytes"
	"embed"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

//go:embed lang/*.yaml
var languageFS embed.FS

var keyLine = regexp.MustCompile(`^([A-Za-z0-9_.-]+):\s*(.*)$`)

// Load 返回指定语言的完整 YAML 资源；未知语言回退到中文。
func Load(locale string) ([]byte, error) {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		locale = "zh"
	}
	data, err := languageFS.ReadFile("lang/" + locale + ".yaml")
	if err != nil && locale != "zh" {
		data, err = languageFS.ReadFile("lang/zh.yaml")
	}
	if err != nil {
		return nil, fmt.Errorf("加载语言资源 %q 失败: %w", locale, err)
	}
	return data, nil
}

// Message 按键读取简单标量消息；复杂 YAML 值由调用方使用 Load 解析。
func Message(locale, key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", errors.New("语言键不能为空")
	}
	data, err := Load(locale)
	if err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		match := keyLine.FindStringSubmatch(strings.TrimSpace(scanner.Text()))
		if len(match) != 3 || match[1] != key {
			continue
		}
		value := strings.TrimSpace(match[2])
		value = strings.Trim(value, "\"'")
		return value, nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("读取语言资源 %q 失败: %w", locale, err)
	}
	return "", fmt.Errorf("语言键 %q 不存在", key)
}
