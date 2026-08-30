// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCLIListenIPPersistsSettings(t *testing.T) {
	dir := t.TempDir()
	handled, err := runCLI([]string{"listen-ip", "ipv6"}, dir)
	if !handled || err != nil {
		t.Fatalf("listen-ip 执行失败: %v", err)
	}
	settings, err := loadCLISettings(dir)
	if err != nil || settings["bindAddress"] != "::" {
		t.Fatalf("监听地址未持久化: %#v, %v", settings, err)
	}
}

func TestCLIAppInitCreatesDataDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	handled, err := runCLI([]string{"app", "init"}, dir)
	if !handled || err != nil {
		t.Fatalf("app init 执行失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "apps")); err != nil {
		t.Fatalf("应用目录未创建: %v", err)
	}
}
