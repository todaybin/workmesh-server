// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package i18n

import "testing"

func TestLoadAllLocalesAndMessage(t *testing.T) {
	for _, locale := range []string{"zh", "en", "zh-Hant", "pt-BR", "ja", "ru", "ms", "ko", "lo", "tr", "fa", "es-ES"} {
		data, err := Load(locale)
		if err != nil || len(data) < 1000 {
			t.Fatalf("locale %s not loaded: %v", locale, err)
		}
	}
	if message, err := Message("zh", "ErrInvalidParams"); err != nil || message == "" {
		t.Fatalf("message lookup failed: %q %v", message, err)
	}
	if _, err := Load("unknown"); err != nil {
		t.Fatalf("unknown locale should fall back: %v", err)
	}
}
