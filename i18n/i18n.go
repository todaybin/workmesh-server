// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package i18n 提供控制面和执行面共用的多语言消息目录。
package i18n

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"text/template"
)

var defaultLocale = "zh"

type localeDefinition struct {
	Code    string   `json:"code"`
	File    string   `json:"file"`
	Aliases []string `json:"aliases"`
}

type localeManifestDefinition struct {
	Default string             `json:"default"`
	Locales []localeDefinition `json:"locales"`
}

var supportedLocales []string
var localeAliases map[string]string
var localeFiles map[string]string

//go:embed lang/*.yaml
var languageFS embed.FS

//go:embed locales.json
var localeManifest []byte

var (
	catalogOnce sync.Once
	catalogs    map[string]map[string]string
	catalogErr  error
)

// SupportedLocales 返回服务端内置的规范语言代码副本。
func SupportedLocales() []string {
	initLocaleManifest()
	result := make([]string, len(supportedLocales))
	copy(result, supportedLocales)
	return result
}

// NormalizeLocale 从 BCP 47 语言代码或 Accept-Language 请求头选择可用语言。
func NormalizeLocale(value string) string {
	initLocaleManifest()
	bestLocale, bestQuality := "", -1.0
	for order, item := range strings.Split(value, ",") {
		parts := strings.Split(item, ";")
		languageCode := strings.ToLower(strings.TrimSpace(parts[0]))
		quality := 1.0
		for _, parameter := range parts[1:] {
			parameter = strings.TrimSpace(parameter)
			if !strings.HasPrefix(strings.ToLower(parameter), "q=") {
				continue
			}
			parsed, err := strconv.ParseFloat(strings.TrimSpace(parameter[2:]), 64)
			if err != nil || parsed < 0 || parsed > 1 {
				quality = 0
			} else {
				quality = parsed
			}
		}
		if quality <= 0 {
			continue
		}
		locale, ok := localeAliases[languageCode]
		if !ok {
			if languageCode == "*" {
				locale = defaultLocale
				ok = true
			} else if separator := strings.IndexByte(languageCode, '-'); separator > 0 {
				locale, ok = localeAliases[languageCode[:separator]]
			}
		}
		if ok && (quality > bestQuality || (quality == bestQuality && bestLocale == "" && order == 0)) {
			bestLocale, bestQuality = locale, quality
		}
	}
	if bestLocale != "" {
		return bestLocale
	}
	return defaultLocale
}

// LocaleFromRequest 根据 Accept-Language 选择服务端支持的语言。
// 未提供请求或请求头时使用中文，避免把任意用户输入直接作为文件路径。
func LocaleFromRequest(r *http.Request) string {
	if r == nil {
		return defaultLocale
	}
	return NormalizeLocale(r.Header.Get("Accept-Language"))
}

// ErrorCode 从错误文本和 HTTP 状态推导稳定的业务错误码。
// 已经是大写下划线格式的错误码会原样保留，其他错误按状态归类，确保客户端可以可靠分支处理。
func ErrorCode(status int, fallback string) string {
	value := strings.TrimSpace(fallback)
	if isStableErrorCode(value) {
		return value
	}
	switch status {
	case http.StatusBadRequest:
		return "INVALID_PARAMS"
	case http.StatusUnauthorized:
		return "LOCAL_AUTH_REQUIRED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusBadGateway, http.StatusGatewayTimeout:
		return "UPSTREAM_ERROR"
	case http.StatusServiceUnavailable:
		return "SERVICE_UNAVAILABLE"
	case http.StatusRequestTimeout:
		return "REQUEST_TIMEOUT"
	default:
		return "REQUEST_FAILED"
	}
}

// isStableErrorCode 判断字符串是否符合可供客户端分支处理的稳定错误码格式。
func isStableErrorCode(value string) bool {
	if value == "" || len(value) > 96 {
		return false
	}
	for index, char := range value {
		if (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' {
			continue
		}
		// 错误码必须以大写字母或数字开头，避免把普通英文句子当成错误码。
		if index == 0 || char == ' ' || char == ':' || char == '.' || char == '-' {
			return false
		}
		return false
	}
	return strings.Contains(value, "_")
}

// LocalizeError 将稳定错误码映射到语言目录中的通用消息。
// 未知业务码保留原始上下文；这比伪造成功或吞掉上游错误更安全。
func LocalizeError(locale, code, fallback string) string {
	code = strings.TrimSpace(code)
	fallback = strings.TrimSpace(fallback)
	key := map[string]string{
		"INVALID_REQUEST":            "ErrInvalidParams",
		"INVALID_PARAMS":             "ErrInvalidParams",
		"LOCAL_AUTH_REQUIRED":        "ErrNotLogin",
		"SESSION_REQUIRED":           "ErrNotLogin",
		"SESSION_NOT_FOUND":          "ErrNotLogin",
		"FORBIDDEN":                  "ErrApiConfigDisable",
		"CSRF_INVALID":               "ErrApiConfigKeyInvalid",
		"DOMAIN_MISMATCH":            "ErrApiConfigDisable",
		"SECURITY_ENTRANCE_REQUIRED": "ErrApiConfigDisable",
		"NOT_FOUND":                  "ErrRecordNotFound",
		"CONFLICT":                   "ErrRecordExist",
		"UPSTREAM_ERROR":             "ErrHttpReqFailed",
		"REQUEST_TIMEOUT":            "ErrHttpReqTimeOut",
		"SERVICE_UNAVAILABLE":        "ErrInternalServer",
		"REQUEST_FAILED":             "ErrProxy",
		"PASSWORD_EXPIRED":           "ErrPasswordExpired",
		"AUTH_REQUIRED":              "ErrNotLogin",
		"STREAM_AUTH_REQUIRED":       "ErrNotLogin",
	}
	keyName := key[code]
	if keyName == "" {
		upper := strings.ToUpper(code)
		switch {
		case strings.Contains(upper, "NOT_FOUND") || strings.HasSuffix(upper, "_MISSING"):
			keyName = "ErrRecordNotFound"
		case strings.Contains(upper, "UNAUTHORIZED") || strings.Contains(upper, "AUTH_REQUIRED"):
			keyName = "ErrNotLogin"
		case strings.Contains(upper, "FORBIDDEN") || strings.Contains(upper, "CSRF"):
			keyName = "ErrApiConfigDisable"
		case strings.Contains(upper, "TIMEOUT"):
			keyName = "ErrHttpReqTimeOut"
		case strings.Contains(upper, "INVALID") || strings.HasSuffix(upper, "_REQUIRED") || strings.HasSuffix(upper, "_EMPTY"):
			keyName = "ErrInvalidParams"
		case strings.Contains(upper, "FAILED") || strings.Contains(upper, "UNAVAILABLE") || strings.Contains(upper, "ERROR"):
			keyName = "ErrInternalServer"
		}
	}
	if keyName == "" {
		return fallback
	}
	// 仅把可展示的上下文传给模板，错误码本身不重复拼入 detail。
	detail := fallback
	if strings.EqualFold(detail, code) {
		detail = ""
	}
	if code == "CSRF_INVALID" {
		// CSRF 失败原因可能包含客户端提交的原文；统一使用语言目录中的 Token 文案。
		detail = ""
	}
	// 通用错误通常来自中文业务错误；在非中文界面不拼接原文，避免出现中英混排。
	// 已知错误码仍保留 ASCII 资源标识、上游错误等上下文。
	if NormalizeLocale(locale) != defaultLocale && containsNonASCII(detail) {
		detail = ""
	}
	message, err := Format(locale, keyName, map[string]any{"detail": detail, "err": detail, "name": detail, "id": detail})
	if err != nil || strings.TrimSpace(message) == "" {
		return fallback
	}
	return message
}

// containsNonASCII 检测错误上下文是否包含非 ASCII 字符，以控制跨语言展示混排。
func containsNonASCII(value string) bool {
	for _, char := range value {
		if char > 127 {
			return true
		}
	}
	return false
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

// loadCatalogs 一次性读取并解析所有内置语言目录，供后续消息查询复用。
func loadCatalogs() {
	initLocaleManifest()
	catalogs = make(map[string]map[string]string, len(supportedLocales))
	for _, locale := range supportedLocales {
		content, err := languageFS.ReadFile("lang/" + localeFiles[locale])
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

var localeManifestOnce sync.Once

// initLocaleManifest 一次性解析语言清单并建立代码、别名和文件映射。
func initLocaleManifest() {
	localeManifestOnce.Do(func() {
		var manifest localeManifestDefinition
		if err := json.Unmarshal(localeManifest, &manifest); err != nil {
			panic(fmt.Sprintf("解析语言清单失败: %v", err))
		}
		if value := strings.TrimSpace(manifest.Default); value != "" {
			defaultLocale = value
		}
		localeAliases = make(map[string]string, len(manifest.Locales)*2)
		localeFiles = make(map[string]string, len(manifest.Locales))
		for _, item := range manifest.Locales {
			code := strings.TrimSpace(item.Code)
			if code == "" || strings.TrimSpace(item.File) == "" {
				continue
			}
			supportedLocales = append(supportedLocales, code)
			localeFiles[code] = item.File
			localeAliases[strings.ToLower(code)] = code
			for _, alias := range item.Aliases {
				localeAliases[strings.ToLower(strings.TrimSpace(alias))] = code
			}
		}
	})
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

// parseScalar 解析单层 YAML 标量，支持未引用、单引号和双引号值。
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
