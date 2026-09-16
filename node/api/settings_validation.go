// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// normalizeSettingValue 将前端 key/value 请求转换为稳定的 SQLite JSON 类型并校验边界。
func normalizeSettingValue(key string, value any) (any, error) {
	key = settingJSONKey(strings.TrimSpace(key))
	switch key {
	case "sessionTimeout":
		return settingInteger(value, 300, 864000)
	case "expirationDays":
		return settingInteger(value, 0, 3650)
	case "serverPort":
		return settingInteger(value, 1, 65535)
	case "monitorInterval":
		return settingInteger(value, 1, 3600)
	case "monitorStoreDays":
		return settingInteger(value, 1, 3650)
	case "port":
		n, err := settingInteger(value, 1, 65535)
		if err != nil {
			return nil, err
		}
		return strconv.Itoa(n), nil
	case "proxyPort":
		if strings.TrimSpace(settingString(value)) == "" {
			return "", nil
		}
		n, err := settingInteger(value, 1, 65535)
		if err != nil {
			return nil, err
		}
		return strconv.Itoa(n), nil
	case "proxyType":
		v := strings.ToLower(strings.TrimSpace(settingString(value)))
		if v == "close" {
			v = ""
		}
		switch v {
		case "", "http", "https", "socks5":
			return v, nil
		default:
			return nil, fmt.Errorf("proxyType 不受支持: %s", v)
		}
	case "theme":
		v := strings.ToLower(strings.TrimSpace(settingString(value)))
		switch v {
		case "system", "light", "dark":
			return v, nil
		default:
			return nil, fmt.Errorf("theme 不受支持: %s", v)
		}
	case "edition":
		v := strings.ToLower(strings.TrimSpace(settingString(value)))
		if v != "cn" && v != "intl" {
			return nil, fmt.Errorf("edition 不受支持: %s", v)
		}
		return v, nil
	case "docSource":
		v := strings.TrimSpace(settingString(value))
		if v != "withByRegion" && v != "withByLang" {
			return nil, fmt.Errorf("docSource 不受支持: %s", v)
		}
		return v, nil
	case "ipv6", "ssl":
		v, err := normalizeSettingToggle(value)
		if err != nil {
			return nil, fmt.Errorf("%s 无效: %w", key, err)
		}
		return v, nil
	case "sslType":
		v := strings.ToLower(strings.TrimSpace(settingString(value)))
		if v != "self" && v != "custom" {
			return nil, fmt.Errorf("sslType 不受支持: %s", v)
		}
		return v, nil
	case "proxyUrl":
		v := strings.TrimSpace(settingString(value))
		if len(v) > 2048 || strings.ContainsAny(v, "\x00\r\n ") {
			return nil, fmt.Errorf("proxyUrl 格式无效")
		}
		if v != "" && strings.Contains(v, "://") {
			parsed, err := url.Parse(v)
			if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "socks5") {
				return nil, fmt.Errorf("proxyUrl 必须是无凭据 HTTP(S)/SOCKS5 地址")
			}
		}
		return v, nil
	case "bindDomain":
		v := strings.TrimSpace(settingString(value))
		if len(v) > 253 || strings.ContainsAny(v, "\x00\r\n /\\") {
			return nil, fmt.Errorf("bindDomain 格式无效")
		}
		return v, nil
	case "allowIPs":
		v := strings.TrimSpace(settingString(value))
		if err := validateAllowIPs(v); err != nil {
			return nil, err
		}
		return v, nil
	default:
		return value, nil
	}
}

func settingInteger(value any, min, max int) (int, error) {
	text := strings.TrimSpace(settingString(value))
	if text == "" {
		return 0, fmt.Errorf("数值不能为空")
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("数值必须在 %d 到 %d 之间", min, max)
	}
	return n, nil
}

func settingString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		if typed != float64(int(typed)) {
			return ""
		}
		return strconv.Itoa(int(typed))
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return ""
	}
}

func normalizeSettingToggle(value any) (string, error) {
	v := strings.ToLower(strings.TrimSpace(settingString(value)))
	switch v {
	case "enable", "enabled", "true", "on":
		return "enable", nil
	case "disable", "disabled", "false", "off":
		return "disable", nil
	default:
		return "", fmt.Errorf("必须是 enable 或 disable")
	}
}

func validateAllowIPs(value string) error {
	if value == "" {
		return nil
	}
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return fmt.Errorf("allowIPs 包含空地址")
		}
		if net.ParseIP(item) != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(item); err != nil {
			return fmt.Errorf("allowIPs 地址无效: %s", item)
		}
	}
	return nil
}
