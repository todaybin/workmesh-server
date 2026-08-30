// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package i18n

import (
	"net/http"
	"strings"
	"testing"
)

func TestLoadAllLocalesAndMergedCatalog(t *testing.T) {
	for _, locale := range SupportedLocales() {
		data, err := Load(locale)
		if err != nil || len(data) < 50_000 {
			t.Fatalf("语言 %s 未完整加载: bytes=%d err=%v", locale, len(data), err)
		}
		for _, key := range []string{"ErrInvalidParams", "ErrOIDCNotEnabled", "ErrFileShareExpired", "TaskStart"} {
			if message, messageErr := Message(locale, key); messageErr != nil || message == "" {
				t.Fatalf("语言 %s 缺少合并键 %s: %q %v", locale, key, message, messageErr)
			}
		}
	}
}

func TestNormalizeLocaleAndFallback(t *testing.T) {
	cases := map[string]string{
		"zh-CN, en;q=0.8": "zh",
		"zh-TW":           "zh-Hant",
		"pt;q=0.9":        "pt-BR",
		"es-MX":           "es-ES",
		"unknown":         "zh",
	}
	for input, expected := range cases {
		if actual := NormalizeLocale(input); actual != expected {
			t.Fatalf("NormalizeLocale(%q)=%q，期望 %q", input, actual, expected)
		}
	}
	if actual := NormalizeLocale("fr-FR;q=0.9,en;q=0.8,zh;q=0.1"); actual != "en" {
		t.Fatalf("NormalizeLocale 应选择最高质量值，实际 %q", actual)
	}
	if actual := NormalizeLocale("en;q=0,zh;q=0.5"); actual != "zh" {
		t.Fatalf("q=0 语言应被忽略，实际 %q", actual)
	}
}

func TestFormatRendersTemplateData(t *testing.T) {
	message, err := Format("en", "ErrInvalidParams", map[string]any{"detail": "pageSize"})
	if err != nil {
		t.Fatalf("渲染消息失败: %v", err)
	}
	if !strings.Contains(message, "pageSize") || strings.Contains(message, "{{") {
		t.Fatalf("模板参数未正确渲染: %q", message)
	}
	if _, err := Message("en", "missing.key"); err == nil {
		t.Fatal("未知语言键应返回明确错误")
	}
}

func TestErrorCodeAndLocalization(t *testing.T) {
	if code := ErrorCode(http.StatusUnauthorized, "会话无效"); code != "LOCAL_AUTH_REQUIRED" {
		t.Fatalf("未登录状态码映射错误: %q", code)
	}
	if code := ErrorCode(http.StatusBadRequest, "CUSTOM_RULE_INVALID"); code != "CUSTOM_RULE_INVALID" {
		t.Fatalf("稳定错误码不应被覆盖: %q", code)
	}
	message := LocalizeError("en-US", "LOCAL_AUTH_REQUIRED", "需要有效的本地登录会话")
	if strings.Contains(message, "本地登录") || !strings.Contains(strings.ToLower(message), "not logged") {
		t.Fatalf("英文错误消息未按语言目录渲染: %q", message)
	}
	unknown := LocalizeError("en", "UNMAPPED_CUSTOM_CODE", "自定义失败: item-1")
	if unknown != "自定义失败: item-1" {
		t.Fatalf("未知错误码应保留原始上下文: %q", unknown)
	}
}
