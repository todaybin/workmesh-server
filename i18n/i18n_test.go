// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package i18n

import (
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
