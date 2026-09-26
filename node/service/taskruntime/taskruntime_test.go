// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package taskruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type backendStub struct {
	created   int
	spec      TaskSpec
	result    TaskExecResult
	startErr  error
	cancelErr error
}

type capabilityBackendStub struct {
	*backendStub
	caps SandboxCapabilities
	err  error
}

type inspectBackendStub struct {
	*backendStub
	inspection TaskInspection
	err        error
}

func (b *inspectBackendStub) Inspect(context.Context, string) (TaskInspection, error) {
	return b.inspection, b.err
}

func (b *capabilityBackendStub) Capabilities(context.Context) (SandboxCapabilities, error) {
	return b.caps, b.err
}

func testWorkspaceRoot(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("/opt", ".workmesh-taskruntime-test-")
	if err != nil {
		t.Fatalf("创建受控测试 workspace root 失败: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return root
}

func (b *backendStub) Create(_ context.Context, spec TaskSpec) (string, error) {
	b.created++
	b.spec = spec
	return "sandbox-1", nil
}
func (b *backendStub) Start(context.Context, string) error { return b.startErr }
func (b *backendStub) Exec(context.Context, string, []string) (TaskExecResult, error) {
	return b.result, nil
}
func (b *backendStub) Cancel(context.Context, string) error { return b.cancelErr }
func (b *backendStub) Collect(context.Context, string) (TaskExecResult, error) {
	return b.result, nil
}
func (b *backendStub) Destroy(context.Context, string) error { return nil }

func validSpec() TaskSpec {
	root := os.Getenv("WORKMESH_AGENT_WORKSPACE_ROOT")
	if root == "" {
		root = "/srv/workmesh"
	}
	return TaskSpec{TaskID: "task-1", ImageDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Worktree: filepath.Join(root, "project"), Entrypoint: []string{"/opt/workmesh/task-bootstrap"}}
}

func TestProviderLifecycleAndIdempotency(t *testing.T) {
	t.Setenv("WORKMESH_AGENT_WORKSPACE_ROOT", testWorkspaceRoot(t))
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

func TestProviderRestoreRequiresBackendInspection(t *testing.T) {
	backend := &backendStub{}
	provider, _ := NewTaskProvider(backend)
	restored, err := provider.RestoreAndInspect(context.Background(), []TaskHandle{{TaskID: "task-restored", SandboxID: "sandbox-1", State: TaskRunning}})
	if err != nil || len(restored) != 1 || restored[0].State != TaskUnknown {
		t.Fatalf("无 inspect 后端必须恢复为 unknown: %+v, %v", restored, err)
	}
	if err := provider.Destroy(context.Background(), "task-restored"); err == nil {
		t.Fatal("unknown 状态不得销毁后端句柄")
	}
	if _, err := provider.Exec(context.Background(), "task-restored", []string{"/opt/workmesh/test"}); err == nil {
		t.Fatal("unknown 状态不得执行任务")
	}
}

func TestProviderRecoverInspectUsesBackendTruth(t *testing.T) {
	backend := &inspectBackendStub{backendStub: &backendStub{}, inspection: TaskInspection{Found: true, State: TaskRunning}}
	provider, _ := NewTaskProvider(backend)
	if _, err := provider.RestoreAndInspect(context.Background(), []TaskHandle{{TaskID: "task-restored", SandboxID: "sandbox-1", State: TaskCompleted}}); err != nil {
		t.Fatal(err)
	}
	state, err := provider.RecoverInspect(context.Background(), "task-restored")
	if err != nil || state != TaskRunning {
		t.Fatalf("恢复核验未使用后端真实状态: %s %v", state, err)
	}
	backend.inspection = TaskInspection{Found: false}
	state, err = provider.RecoverInspect(context.Background(), "task-restored")
	if err != nil || state != TaskAwaitingHuman {
		t.Fatalf("句柄不存在应进入人工恢复: %s %v", state, err)
	}
}

func TestProviderRejectsUnsafeSpec(t *testing.T) {
	t.Setenv("WORKMESH_AGENT_WORKSPACE_ROOT", testWorkspaceRoot(t))
	provider, _ := NewTaskProvider(&backendStub{})
	cases := []struct {
		name string
		edit func(*TaskSpec)
	}{
		{"digest", func(s *TaskSpec) { s.ImageDigest = "latest" }},
		{"uppercase digest", func(s *TaskSpec) {
			s.ImageDigest = "sha256:0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef"
		}},
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

func TestProviderRequiresScopedWorkspaceAndRejectsSymlinkEscape(t *testing.T) {
	t.Setenv("WORKMESH_AGENT_WORKSPACE_ROOT", "")
	providerWithoutRoot, err := NewTaskProvider(&backendStub{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := providerWithoutRoot.Create(context.Background(), validSpec()); err == nil {
		t.Fatal("未配置 workspace root 必须拒绝")
	}

	root := testWorkspaceRoot(t)
	workspace := filepath.Join(root, "project", "task")
	if err := os.MkdirAll(filepath.Dir(workspace), 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_AGENT_WORKSPACE_ROOT", root)
	provider, err := NewTaskProvider(&backendStub{})
	if err != nil {
		t.Fatal(err)
	}
	spec := validSpec()
	spec.Worktree = workspace
	if _, err := provider.Create(context.Background(), spec); err != nil {
		t.Fatalf("scoped workspace should pass: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(filepath.Dir(workspace), link); err != nil {
		t.Skipf("当前平台不支持符号链接测试: %v", err)
	}
	escaped := spec
	escaped.TaskID = "task-link"
	escaped.Worktree = filepath.Join(link, "task")
	if _, err := provider.Create(context.Background(), escaped); err == nil {
		t.Fatal("符号链接工作区必须拒绝")
	}
}

func TestResourceProfileLimitsAreNormalizedAndBounded(t *testing.T) {
	limits, err := ResourceLimitsForProfile("medium")
	if err != nil || limits.MemoryBytes != 2<<30 || limits.PIDsMax != 512 {
		t.Fatalf("medium limits = %+v, %v", limits, err)
	}
	if _, err := ResourceLimitsForProfile("unbounded"); err == nil {
		t.Fatal("未知资源 profile 必须拒绝")
	}
	t.Setenv("WORKMESH_AGENT_WORKSPACE_ROOT", testWorkspaceRoot(t))
	backend := &backendStub{}
	provider, _ := NewTaskProvider(backend)
	valid := validSpec()
	valid.ResourceProfile = "medium"
	if _, err := provider.Create(context.Background(), valid); err != nil {
		t.Fatalf("合法 resource profile 创建失败: %v", err)
	}
	if backend.spec.ResourceProfile != "medium" || backend.spec.ResourceLimits.MemoryBytes != 2<<30 || backend.spec.ResourceLimits.PIDsMax != 512 {
		t.Fatalf("归一化资源限制未传入 backend: %+v", backend.spec)
	}

	provider, _ = NewTaskProvider(&backendStub{})
	spec := validSpec()
	spec.ResourceProfile = "small"
	spec.ResourceLimits = ResourceLimits{CPUQuotaMicros: 200_000, MemoryBytes: 1 << 30, PIDsMax: 256, DiskBytes: 4 << 30}
	if _, err := provider.Create(context.Background(), spec); err == nil {
		t.Fatal("超过 profile 的 CPU 限制必须拒绝")
	}
}

func TestProviderDestroyRejectsRunningTask(t *testing.T) {
	t.Setenv("WORKMESH_AGENT_WORKSPACE_ROOT", testWorkspaceRoot(t))
	backend := &backendStub{}
	provider, err := NewTaskProvider(backend)
	if err != nil {
		t.Fatal(err)
	}
	spec := validSpec()
	spec.TaskID = "task-running-destroy"
	spec.Worktree = filepath.Join(os.Getenv("WORKMESH_AGENT_WORKSPACE_ROOT"), "project-a", "worktrees", spec.TaskID)
	spec.RuntimePolicy.WorkspaceRef = "project-a"
	if _, err := provider.Create(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if err := provider.Start(context.Background(), spec.TaskID); err != nil {
		t.Fatal(err)
	}
	if err := provider.Destroy(context.Background(), spec.TaskID); err == nil {
		t.Fatal("运行中任务不得直接销毁")
	}
	if err := provider.Cancel(context.Background(), spec.TaskID); err != nil {
		t.Fatal(err)
	}
	if err := provider.Destroy(context.Background(), spec.TaskID); err != nil {
		t.Fatal(err)
	}
}

func TestProviderOperationFailurePreservesRetryableState(t *testing.T) {
	t.Setenv("WORKMESH_AGENT_WORKSPACE_ROOT", testWorkspaceRoot(t))
	backend := &backendStub{startErr: errors.New("暂时无法启动")}
	provider, err := NewTaskProvider(backend)
	if err != nil {
		t.Fatal(err)
	}
	spec := validSpec()
	spec.TaskID = "task-retryable-start"
	if _, err := provider.Create(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if err := provider.Start(context.Background(), spec.TaskID); err == nil {
		t.Fatal("后端启动失败必须返回错误")
	}
	state, err := provider.State(spec.TaskID)
	if err != nil || state != TaskCreated {
		t.Fatalf("启动失败必须保留 created 以便重试: state=%s err=%v", state, err)
	}
	backend.startErr = nil
	if err := provider.Start(context.Background(), spec.TaskID); err != nil {
		t.Fatalf("修复后启动重试失败: %v", err)
	}
	backend.cancelErr = errors.New("暂时无法取消")
	if err := provider.Cancel(context.Background(), spec.TaskID); err == nil {
		t.Fatal("后端取消失败必须返回错误")
	}
	state, err = provider.State(spec.TaskID)
	if err != nil || state != TaskRunning {
		t.Fatalf("取消失败必须保留 running 以便重试: state=%s err=%v", state, err)
	}
}

func TestProviderCollectPreservesCancelledState(t *testing.T) {
	t.Setenv("WORKMESH_AGENT_WORKSPACE_ROOT", testWorkspaceRoot(t))
	provider, err := NewTaskProvider(&backendStub{})
	if err != nil {
		t.Fatal(err)
	}
	spec := validSpec()
	spec.TaskID = "task-cancel-collect"
	if _, err := provider.Create(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if err := provider.Start(context.Background(), spec.TaskID); err != nil {
		t.Fatal(err)
	}
	if err := provider.Cancel(context.Background(), spec.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Collect(context.Background(), spec.TaskID); err != nil {
		t.Fatal(err)
	}
	state, err := provider.State(spec.TaskID)
	if err != nil || state != TaskCancelled {
		t.Fatalf("取消后的 collect 不得伪造成 completed: state=%s err=%v", state, err)
	}
}

func TestCapabilityCheckedProviderRequiresAndValidatesSandboxCapabilities(t *testing.T) {
	if _, err := NewCapabilityCheckedTaskProvider(&backendStub{}); err == nil {
		t.Fatal("缺少 Sandbox 能力证明的 backend 必须拒绝")
	}
	t.Setenv("WORKMESH_AGENT_WORKSPACE_ROOT", testWorkspaceRoot(t))
	limits, err := ResourceLimitsForProfile("small")
	if err != nil {
		t.Fatal(err)
	}
	backend := &capabilityBackendStub{backendStub: &backendStub{}, caps: SandboxCapabilities{
		ProtocolVersion: SandboxProtocolVersion, SandboxType: "forgevm", Backend: "gvisor", Isolation: "container",
		WorkspaceIsolation: true, NetworkIsolation: true, NetworkAllowlist: true,
		PreStartEnforcement: true, ProcessTreeContainment: true,
		HardCPU: true, HardMemory: true, HardPIDs: true, HardDisk: true,
		MaxResourceLimits: limits,
	}}
	provider, err := NewCapabilityCheckedTaskProvider(backend)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Create(context.Background(), validSpec()); err != nil {
		t.Fatalf("能力覆盖任务时不应拒绝: %v", err)
	}
	backend.caps.HardMemory = false
	second := validSpec()
	second.TaskID = "task-capability-rejected"
	if _, err := provider.Create(context.Background(), second); err == nil {
		t.Fatal("缺少内存硬限制时必须拒绝")
	}
	backend.caps.HardMemory = true
	backend.caps.ProtocolVersion = ""
	third := validSpec()
	third.TaskID = "task-legacy-protocol"
	if _, err := provider.Create(context.Background(), third); err == nil {
		t.Fatal("旧版或未声明协议版本的 CLI 必须拒绝")
	}
}
