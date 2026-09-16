// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package testenv

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWebsiteIsolationRestoresEnvironment 验证隔离目录可写、用例失败透传及原环境恢复。
func TestWebsiteIsolationRestoresEnvironment(t *testing.T) {
	original := filepath.Join(t.TempDir(), "existing-sites")
	t.Setenv("WORKMESH_WEBSITE_ROOT", original)
	var isolated string
	result := RunWebsiteTests(func() int {
		isolated = os.Getenv("WORKMESH_WEBSITE_ROOT")
		if isolated == original || isolated == "" {
			t.Error("测试目录未隔离")
			return 1
		}
		if err := os.MkdirAll(isolated, 0o700); err != nil {
			t.Error(err)
		}
		if err := os.WriteFile(filepath.Join(isolated, "evidence"), []byte("test"), 0o600); err != nil {
			t.Error(err)
		}
		return 7
	})
	if result != 7 {
		t.Fatalf("用例状态未透传: %d", result)
	}
	if os.Getenv("WORKMESH_WEBSITE_ROOT") != original {
		t.Fatal("原环境未恢复")
	}
	if _, err := os.Stat(isolated); !os.IsNotExist(err) {
		t.Fatalf("测试目录未清理: %v", err)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("原目录被修改: %v", err)
	}
}
