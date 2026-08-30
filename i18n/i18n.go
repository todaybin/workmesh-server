// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package i18n 提供控制面和执行面共用的多语言消息目录。
package i18n

import (
	"bufio"
	"bytes"
	"embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"text/template"
)

const defaultLocale = "zh"

var supportedLocales = []string{"zh", "zh-Hant", "en", "pt-BR", "ja", "ru", "ms", "ko", "lo", "tr", "es-ES", "fa"}

//go:embed lang/*.yaml
var languageFS embed.FS

var (
	catalogOnce sync.Once
	catalogs    map[string]map[string]string
	catalogErr  error
)

// SupportedLocales 返回服务端内置的规范语言代码副本。
func SupportedLocales() []string {
	result := make([]string, len(supportedLocales))
	copy(result, supportedLocales)
	return result
}

// NormalizeLocale 从 BCP 47 语言代码或 Accept-Language 请求头选择可用语言。
func NormalizeLocale(value string) string {
	aliases := map[string]string{
		"zh": "zh", "zh-cn": "zh", "zh-hans": "zh",
		"zh-hant": "zh-Hant", "zh-tw": "zh-Hant", "zh-hk": "zh-Hant",
		"en": "en", "pt": "pt-BR", "pt-br": "pt-BR", "ja": "ja",
		"ru": "ru", "ms": "ms", "ko": "ko", "lo": "lo", "tr": "tr",
		"es": "es-ES", "es-es": "es-ES", "fa": "fa",
	}
	for _, item := range strings.Split(value, ",") {
		languageCode := strings.ToLower(strings.TrimSpace(strings.SplitN(item, ";", 2)[0]))
		if locale, ok := aliases[languageCode]; ok {
			return locale
		}
		if separator := strings.IndexByte(languageCode, '-'); separator > 0 {
			if locale, ok := aliases[languageCode[:separator]]; ok {
				return locale
			}
		}
	}
	return defaultLocale
}

// Load 返回规范化语言对应的完整 YAML 资源。
func Load(locale string) ([]byte, error) {
	locale = NormalizeLocale(locale)
	data, err := languageFS.ReadFile("lang/" + locale + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("加载语言资源 %q 失败: %w", locale, err)
	}
	return data, nil
}

// Message 按键读取标量消息；未知键返回带上下文的错误。
func Message(locale, key string) (string, error) {
	return Format(locale, key, nil)
}

// Format 按语言和键渲染 Go template 风格的消息参数。
func Format(locale, key string, data map[string]any) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("语言键不能为空")
	}
	catalogOnce.Do(loadCatalogs)
	if catalogErr != nil {
		return "", catalogErr
	}
	normalized := NormalizeLocale(locale)
	message, ok := catalogs[normalized][key]
	if !ok && normalized != defaultLocale {
		message, ok = catalogs[defaultLocale][key]
	}
	if !ok {
		return "", fmt.Errorf("语言键 %q 不存在", key)
	}
	if !strings.Contains(message, "{{") {
		return message, nil
	}
	tmpl, err := template.New(key).Option("missingkey=zero").Parse(message)
	if err != nil {
		return "", fmt.Errorf("解析语言模板 %q 失败: %w", key, err)
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("渲染语言模板 %q 失败: %w", key, err)
	}
	return strings.ReplaceAll(rendered.String(), ": <no value>", ""), nil
}

func loadCatalogs() {
	catalogs = make(map[string]map[string]string, len(supportedLocales))
	for _, locale := range supportedLocales {
		content, err := languageFS.ReadFile("lang/" + locale + ".yaml")
		if err != nil {
			catalogErr = fmt.Errorf("加载语言目录 %q 失败: %w", locale, err)
			return
		}
		catalog, err := parseCatalog(content)
		if err != nil {
			catalogErr = fmt.Errorf("解析语言目录 %q 失败: %w", locale, err)
			return
		}
		catalogs[locale] = catalog
	}
}

// 语言文件采用单层标量 YAML；专用解析器避免每次请求读取文件。
func parseCatalog(content []byte) (map[string]string, error) {
	catalog := make(map[string]string, 1024)
	scanner := bufio.NewScanner(bytes.NewReader(bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		separator := strings.IndexByte(line, ':')
		if separator <= 0 {
			return nil, fmt.Errorf("第 %d 行不是键值标量", lineNumber)
		}
		key := strings.TrimSpace(line[:separator])
		if _, exists := catalog[key]; exists {
			return nil, fmt.Errorf("第 %d 行存在重复键 %q", lineNumber, key)
		}
		value, err := parseScalar(strings.TrimSpace(line[separator+1:]))
		if err != nil {
			return nil, fmt.Errorf("第 %d 行键 %q: %w", lineNumber, key, err)
		}
		catalog[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取语言目录失败: %w", err)
	}
	return catalog, nil
}

func parseScalar(value string) (string, error) {
	if len(value) < 2 {
		return value, nil
	}
	if value[0] == '"' && value[len(value)-1] == '"' {
		result, err := strconv.Unquote(value)
		if err != nil {
			return "", fmt.Errorf("双引号字符串无效: %w", err)
		}
		return result, nil
	}
	if value[0] == '\'' && value[len(value)-1] == '\'' {
		return strings.ReplaceAll(value[1:len(value)-1], "''", "'"), nil
	}
	return value, nil
}
