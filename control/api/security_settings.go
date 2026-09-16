// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// securitySettingsCache 缓存低频变化的安全设置，并保留数据库优先策略。
type securitySettingsCache struct {
	mu      sync.RWMutex
	path    string
	modTime time.Time
	size    int64
	loaded  bool
	value   SecuritySettings
	loadDB  func() map[string]any
}

// newSecuritySettingsProvider 创建安全设置提供器并确定兼容配置文件位置。
func newSecuritySettingsProvider(dataDir string, loadDB func() map[string]any) *securitySettingsCache {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		dataDir = strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	}
	if dataDir == "" {
		dataDir = "./data"
	}
	return &securitySettingsCache{path: filepath.Join(dataDir, "domains.json"), loadDB: loadDB}
}

// load 读取环境变量、SQLite 和兼容文件中的安全设置，并返回当前快照。
func (p *securitySettingsCache) load() SecuritySettings {
	value := SecuritySettings{BindDomain: strings.TrimSpace(os.Getenv("WORKMESH_BIND_DOMAIN")), SecurityEntrance: strings.Trim(strings.TrimSpace(os.Getenv("WORKMESH_SECURITY_ENTRANCE")), "/")}
	if p.loadDB != nil {
		if settings := p.loadDB(); settings != nil {
			return mergeSecuritySettings(value, parseSecuritySettings(settings))
		}
	}
	info, err := os.Stat(p.path)
	if err == nil {
		p.mu.RLock()
		cached := p.loaded && p.modTime.Equal(info.ModTime()) && p.size == info.Size()
		cachedValue := p.value
		p.mu.RUnlock()
		if cached {
			return mergeSecuritySettings(value, cachedValue)
		}
		if raw, readErr := os.ReadFile(p.path); readErr == nil {
			var document struct {
				Settings map[string]any `json:"settings"`
			}
			if json.Unmarshal(raw, &document) == nil {
				loaded := parseSecuritySettings(document.Settings)
				p.mu.Lock()
				p.modTime, p.size, p.loaded, p.value = info.ModTime(), info.Size(), true, loaded
				p.mu.Unlock()
				return mergeSecuritySettings(value, loaded)
			}
		}
	}
	return value
}

// mergeSecuritySettings 合并配置来源，并让非空的兼容文件字段补充基础设置。
func mergeSecuritySettings(base, file SecuritySettings) SecuritySettings {
	if base.BindDomain == "" {
		base.BindDomain = file.BindDomain
	}
	if base.SecurityEntrance == "" {
		base.SecurityEntrance = file.SecurityEntrance
	}
	if file.ExpirationDays != 0 {
		base.ExpirationDays = file.ExpirationDays
	}
	if !file.ExpirationTime.IsZero() {
		base.ExpirationTime = file.ExpirationTime
	}
	if !file.PasswordChangedAt.IsZero() {
		base.PasswordChangedAt = file.PasswordChangedAt
	}
	return base
}

// parseSecuritySettings 将统一存储中的动态值转换为安全设置结构。
func parseSecuritySettings(values map[string]any) SecuritySettings {
	result := SecuritySettings{}
	for key, value := range values {
		switch strings.ToLower(strings.ReplaceAll(key, "_", "")) {
		case "binddomain":
			result.BindDomain = strings.TrimSpace(asString(value))
		case "securityentrance":
			result.SecurityEntrance = strings.Trim(strings.TrimSpace(asString(value)), "/")
		case "expirationdays":
			result.ExpirationDays = asInt(value)
		case "expirationtime", "passwordexpirationtime":
			result.ExpirationTime = parseTime(asString(value))
		case "passwordchangedat", "passwordupdatedat":
			result.PasswordChangedAt = parseTime(asString(value))
		}
	}
	return result
}

// asString 将 SQLite 或 JSON 解码后的基础类型统一转换为字符串。
func asString(value any) string {
	switch item := value.(type) {
	case string:
		return item
	case json.Number:
		return item.String()
	case float64:
		return strconv.FormatFloat(item, 'f', -1, 64)
	default:
		return ""
	}
}

// asInt 将安全设置中的数值转换为整数，无法转换时返回零。
func asInt(value any) int {
	if number, err := strconv.Atoi(strings.TrimSpace(asString(value))); err == nil {
		return number
	}
	return 0
}

// parseTime 按兼容格式解析密码过期相关时间，失败时返回零时间。
func parseTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
