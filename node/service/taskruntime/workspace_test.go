// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package taskruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceLayoutCleansOnlyTaskTransientDirectory(t *testing.T) {
	root := testWorkspaceRoot(t)
	layout, err := NewWorkspaceLayout(root, "project-a", "task-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{layout.Worktree, layout.Temp, layout.Artifacts} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(layout.Cache, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(layout.Worktree, "source.go"), filepath.Join(layout.Temp, "test.log"), filepath.Join(layout.Cache, "module.zip"), filepath.Join(layout.Artifacts, "report.json")} {
		if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := layout.CleanupTransient(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(layout.Temp); !os.IsNotExist(err) {
		t.Fatalf("任务 tmp 未清理: %v", err)
	}
	for _, path := range []string{layout.Worktree, layout.Cache, layout.Artifacts} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("非临时目录不应清理 %s: %v", path, err)
		}
	}
}

func TestWorkspaceLayoutRejectsSymlinkProject(t *testing.T) {
	root := testWorkspaceRoot(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "project-link")); err != nil {
		t.Skipf("当前平台不支持符号链接测试: %v", err)
	}
	if _, err := NewWorkspaceLayout(root, "project-link", "task-1"); err == nil {
		t.Fatal("项目目录符号链接必须拒绝")
	}
}

func TestWorkspaceLayoutEnsureCreatesTaskDirectories(t *testing.T) {
	root := testWorkspaceRoot(t)
	layout, err := NewWorkspaceLayout(root, "project-a", "task-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{layout.Worktree, layout.Temp, layout.Cache, layout.Artifacts} {
		info, statErr := os.Stat(path)
		if statErr != nil || !info.IsDir() {
			t.Fatalf("workspace 目录未创建: %s (%v)", path, statErr)
		}
	}
}

func TestWorkspaceLayoutEnsureRejectsSymlinkDuringCreation(t *testing.T) {
	root := testWorkspaceRoot(t)
	layout, err := NewWorkspaceLayout(root, "project-link", "task-a")
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.MkdirAll(layout.Project, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(layout.Project, "worktrees")); err != nil {
		t.Skipf("当前平台不支持符号链接测试: %v", err)
	}
	if err := layout.Ensure(context.Background()); err == nil {
		t.Fatal("物化过程遇到符号链接时必须拒绝")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "task-a")); !os.IsNotExist(statErr) {
		t.Fatalf("不得在符号链接目标外写入任务目录: %v", statErr)
	}
}

func TestWorkspaceLayoutRejectsForgedCleanupPath(t *testing.T) {
	root := testWorkspaceRoot(t)
	forged := WorkspaceLayout{Root: root, Project: root, Temp: filepath.Join(root, "tmp", "task-1")}
	if err := forged.CleanupTransient(context.Background()); err == nil {
		t.Fatal("伪造的项目 tmp 路径必须拒绝")
	}
}

func TestProviderDestroyCleansProjectTaskTransientDirectory(t *testing.T) {
	root := testWorkspaceRoot(t)
	t.Setenv("WORKMESH_AGENT_WORKSPACE_ROOT", root)
	layout, err := NewWorkspaceLayout(root, "project-a", "task-clean")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.Temp, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.Temp, "compile.tmp"), []byte("temporary"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &backendStub{}
	provider, err := NewTaskProvider(backend)
	if err != nil {
		t.Fatal(err)
	}
	spec := validSpec()
	spec.TaskID = "task-clean"
	spec.Worktree = filepath.Join(root, "project-a", "worktrees", "task-clean")
	spec.RuntimePolicy.WorkspaceRef = "project-a"
	if _, err := provider.Create(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if err := provider.Destroy(context.Background(), spec.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(layout.Temp); !os.IsNotExist(err) {
		t.Fatalf("Destroy 未清理任务 tmp: %v", err)
	}
}
