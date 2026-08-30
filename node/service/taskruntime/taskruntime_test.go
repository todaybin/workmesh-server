// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package taskruntime

import (
	"context"
	"testing"
)

type backendStub struct {
	created int
	result  TaskExecResult
}

func (b *backendStub) Create(context.Context, TaskSpec) (string, error) {
	b.created++
	return "sandbox-1", nil
}
func (b *backendStub) Start(context.Context, string) error { return nil }
func (b *backendStub) Exec(context.Context, string, []string) (TaskExecResult, error) {
	return b.result, nil
}
func (b *backendStub) Cancel(context.Context, string) error { return nil }
func (b *backendStub) Collect(context.Context, string) (TaskExecResult, error) {
	return b.result, nil
}
func (b *backendStub) Destroy(context.Context, string) error { return nil }

func validSpec() TaskSpec {
	return TaskSpec{TaskID: "task-1", ImageDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Worktree: "/srv/workmesh/project", Entrypoint: []string{"/opt/workmesh/task-bootstrap"}}
}

func TestProviderLifecycleAndIdempotency(t *testing.T) {
	backend := &backendStub{result: TaskExecResult{ExitCode: 0, Stdout: "done"}}
	provider, err := NewTaskProvider(backend)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := provider.Create(context.Background(), validSpec())
	if err != nil || handle.State != TaskCreated || backend.created != 1 {
		t.Fatalf("create = %+v, %v", handle, err)
	}
	if _, err := provider.Create(context.Background(), validSpec()); err == nil {
		t.Fatal("重复 taskId 必须拒绝")
	}
	if _, err := provider.Exec(context.Background(), handle.TaskID, []string{"run"}); err == nil {
		t.Fatal("未启动任务不得执行")
	}
	if err := provider.Start(context.Background(), handle.TaskID); err != nil {
		t.Fatal(err)
	}
	result, err := provider.Exec(context.Background(), handle.TaskID, []string{"run"})
	if err != nil || result.Stdout != "done" {
		t.Fatalf("exec = %+v, %v", result, err)
	}
	if _, err := provider.Collect(context.Background(), handle.TaskID); err != nil {
		t.Fatal(err)
	}
	if err := provider.Destroy(context.Background(), handle.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Exec(context.Background(), handle.TaskID, []string{"run"}); err == nil {
		t.Fatal("已销毁任务不得执行")
	}
}

func TestProviderRejectsUnsafeSpec(t *testing.T) {
	provider, _ := NewTaskProvider(&backendStub{})
	cases := []struct {
		name string
		edit func(*TaskSpec)
	}{
		{"digest", func(s *TaskSpec) { s.ImageDigest = "latest" }},
		{"uppercase digest", func(s *TaskSpec) { s.ImageDigest = "sha256:0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef" }},
		{"relative worktree", func(s *TaskSpec) { s.Worktree = "relative" }},
		{"sensitive worktree", func(s *TaskSpec) { s.Worktree = "/etc" }},
		{"shell entrypoint", func(s *TaskSpec) { s.Entrypoint = []string{"/bin/sh"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := validSpec()
			tc.edit(&spec)
			if _, err := provider.Create(context.Background(), spec); err == nil {
				t.Fatal("不安全任务参数必须拒绝")
			}
		})
	}
	if _, err := provider.Create(context.Background(), validSpec()); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Exec(context.Background(), "task-1", []string{"bad\x00"}); err == nil {
		t.Fatal("NUL argv 必须拒绝")
	}
}
