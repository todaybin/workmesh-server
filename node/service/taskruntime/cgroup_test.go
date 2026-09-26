// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package taskruntime

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCgroupV2ControllerWritesLimitsAndRefusesResidualProcesses(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("仅在 Linux 验证 cgroup v2 文件契约")
	}
	root := t.TempDir()
	parent := filepath.Join(root, "workmesh-agent")
	if err := os.MkdirAll(parent, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpu memory pids"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "cgroup.subtree_control"), []byte(""), 0o640); err != nil {
		t.Fatal(err)
	}
	controller, err := NewCgroupV2Controller(root, "workmesh-agent")
	if err != nil {
		t.Fatal(err)
	}
	limits := ResourceLimits{CPUQuotaMicros: 100_000, MemoryBytes: 1 << 30, PIDsMax: 256, DiskBytes: 4 << 30}
	handle, err := controller.Create(context.Background(), "task-1", limits)
	if err != nil {
		t.Fatal(err)
	}
	assertFile := func(name, expected string) {
		t.Helper()
		content, readErr := os.ReadFile(filepath.Join(handle.Path, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(content) != expected {
			t.Fatalf("%s = %q, want %q", name, content, expected)
		}
	}
	assertFile("cpu.max", "100000 100000")
	assertFile("memory.max", "1073741824")
	assertFile("memory.swap.max", "0")
	assertFile("pids.max", "256")
	if !strings.Contains(string(mustReadFile(t, filepath.Join(parent, "cgroup.subtree_control"))), "+cpu") {
		t.Fatal("未启用 cpu controller")
	}
	if err := controller.Attach(context.Background(), handle, 123); err != nil {
		t.Fatal(err)
	}
	if err := controller.Destroy(context.Background(), handle); err == nil {
		t.Fatal("仍有进程时必须拒绝删除 cgroup")
	}
	if err := os.WriteFile(filepath.Join(handle.Path, "cgroup.procs"), nil, 0o640); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cpu.max", "memory.max", "memory.swap.max", "pids.max", "cgroup.procs"} {
		if err := os.Remove(filepath.Join(handle.Path, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := controller.Destroy(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
}

func TestCgroupV2ControllerRejectsMissingControllerAndDiskOnlyLimit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("仅在 Linux 验证 cgroup v2 文件契约")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpu memory"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCgroupV2Controller(root, "workmesh-agent"); err == nil {
		t.Fatal("缺少 pids controller 必须拒绝")
	}
	if _, err := NewCgroupV2Controller(root, "../escape"); err == nil {
		t.Fatal("parent 路径穿越必须拒绝")
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
