// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package log

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotatingWriterRotatesAndRetainsBoundedFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	w, err := Open(Config{Path: path, MaxBytes: 8, MaxBackups: 2})
	if err != nil {
		t.Fatalf("打开轮转日志失败: %v", err)
	}
	for i := 0; i < 4; i++ {
		if _, err := w.Write([]byte("12345678")); err != nil {
			t.Fatalf("写入轮转日志失败: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("关闭轮转日志失败: %v", err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("首个历史日志不存在: %v", err)
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("历史日志超过保留上限: %v", err)
	}
}
