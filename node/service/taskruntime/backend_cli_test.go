// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package taskruntime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLITaskBackendCapabilitiesUsesExplicitOperation(t *testing.T) {
	backend := &CLITaskBackend{
		command:     "/opt/workmesh/sandbox-cli",
		timeout:     0,
		outputLimit: 4096,
		run: func(_ context.Context, _ string, args []string, _ int) ([]byte, error) {
			if len(args) != 4 || args[0] != "--json" || args[1] != "task" || args[2] != "capabilities" {
				t.Fatalf("能力探测参数不符合契约: %#v", args)
			}
			if !strings.Contains(args[3], "{}") {
				t.Fatalf("能力探测 payload 不符合契约: %s", args[3])
			}
			return []byte(`{"ok":true,"data":{"protocolVersion":"workmesh.sandbox.v1","sandboxType":"forgevm","backend":"gvisor","isolation":"container","workspaceIsolation":true,"networkIsolation":true,"networkAllowlist":true,"preStartEnforcement":true,"processTreeContainment":true,"hardCpu":true,"hardMemory":true,"hardPids":true,"hardDisk":true,"maxResourceLimits":{"cpuQuotaMicros":100000,"memoryBytes":1073741824,"pidsMax":256,"diskBytes":4294967296}}}`), nil
		},
	}
	caps, err := backend.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if caps.ProtocolVersion != SandboxProtocolVersion || caps.SandboxType != "forgevm" || caps.Backend != "gvisor" || !caps.HardCPU || caps.MaxResourceLimits.MemoryBytes != 1<<30 {
		t.Fatalf("能力响应解析错误: %+v", caps)
	}
}

func TestCLITaskBackendCapabilitiesRejectsInvalidEnvelope(t *testing.T) {
	backend := &CLITaskBackend{
		command:     "/opt/workmesh/sandbox-cli",
		timeout:     0,
		outputLimit: 4096,
		run: func(context.Context, string, []string, int) ([]byte, error) {
			return []byte(`{"ok":true,"data":{"sandboxType":"forgevm"}} trailing`), nil
		},
	}
	if _, err := backend.Capabilities(context.Background()); err == nil {
		t.Fatal("无效 CLI envelope 必须拒绝")
	}
}

func TestCLITaskBackendInspectUsesReadOnlyOperation(t *testing.T) {
	backend := &CLITaskBackend{
		command:     "/opt/workmesh/sandbox-cli",
		timeout:     0,
		outputLimit: 4096,
		run: func(_ context.Context, _ string, args []string, _ int) ([]byte, error) {
			if len(args) != 4 || args[2] != "inspect" || !strings.Contains(args[3], "sandbox-1") {
				t.Fatalf("inspect 参数不符合只读契约: %#v", args)
			}
			return []byte(`{"ok":true,"data":{"found":true,"state":"running"}}`), nil
		},
	}
	inspection, err := backend.Inspect(context.Background(), "sandbox-1")
	if err != nil || !inspection.Found || inspection.State != TaskRunning {
		t.Fatalf("inspect = %+v, %v", inspection, err)
	}
}

func TestCLITaskBackendInspectRejectsUnknownState(t *testing.T) {
	backend := &CLITaskBackend{command: "/opt/workmesh/sandbox-cli", outputLimit: 4096, run: func(context.Context, string, []string, int) ([]byte, error) {
		return []byte(`{"ok":true,"data":{"found":true,"state":"running-again"}}`), nil
	}}
	if _, err := backend.Inspect(context.Background(), "sandbox-1"); err == nil {
		t.Fatal("inspect 返回未知状态必须拒绝")
	}
}

func TestCapabilityCheckedProviderRunsCompleteCLILifecycle(t *testing.T) {
	root := testWorkspaceRoot(t)
	t.Setenv("WORKMESH_AGENT_WORKSPACE_ROOT", root)
	completedWorktree := filepath.Join(root, "project-a", "worktrees", "task-completed")
	cancelledWorktree := filepath.Join(root, "project-a", "worktrees", "task-cancelled")
	for _, worktree := range []string{completedWorktree, cancelledWorktree} {
		if err := os.MkdirAll(worktree, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	operations := make([]string, 0, 16)
	backend := &CLITaskBackend{
		command:     "/opt/workmesh/sandbox-cli",
		timeout:     0,
		outputLimit: 4096,
		run: func(_ context.Context, _ string, args []string, _ int) ([]byte, error) {
			if len(args) != 4 || args[0] != "--json" || args[1] != "task" {
				t.Fatalf("CLI 参数不符合固定契约: %#v", args)
			}
			operation := args[2]
			operations = append(operations, operation)
			switch operation {
			case "capabilities":
				return []byte(`{"ok":true,"data":{"protocolVersion":"workmesh.sandbox.v1","sandboxType":"forgevm","backend":"gvisor","isolation":"container","workspaceIsolation":true,"networkIsolation":true,"networkAllowlist":true,"preStartEnforcement":true,"processTreeContainment":true,"hardCpu":true,"hardMemory":true,"hardPids":true,"hardDisk":true,"maxResourceLimits":{"cpuQuotaMicros":200000,"memoryBytes":2147483648,"pidsMax":512,"diskBytes":8589934592}}}`), nil
			case "create":
				var spec TaskSpec
				if err := json.Unmarshal([]byte(args[3]), &spec); err != nil {
					t.Fatalf("create payload 无效: %v", err)
				}
				if spec.ResourceProfile != "medium" || spec.ResourceLimits.MemoryBytes != 2<<30 || spec.RuntimePolicy.WorkspaceRef != "project-a" {
					t.Fatalf("create 未传递任务隔离上下文: %+v", spec)
				}
				receipt := enforcementReceiptForSpec(spec)
				response, err := json.Marshal(map[string]any{"ok": true, "data": map[string]any{"sandboxId": "sandbox-1", "enforcement": receipt}})
				if err != nil {
					t.Fatalf("create response 编码失败: %v", err)
				}
				return response, nil
			case "exec":
				return []byte(`{"ok":true,"data":{"exitCode":0,"stdout":"通过","stderr":""}}`), nil
			case "collect":
				return []byte(`{"ok":true,"data":{"exitCode":0,"stdout":"完成","stderr":""}}`), nil
			default:
				return []byte(`{"ok":true}`), nil
			}
		},
	}
	provider, err := NewCapabilityCheckedTaskProvider(backend)
	if err != nil {
		t.Fatal(err)
	}
	image := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	baseSpec := func(taskID, worktree string) TaskSpec {
		return TaskSpec{
			TaskID:          taskID,
			ImageDigest:     image,
			Worktree:        worktree,
			Entrypoint:      []string{"/opt/workmesh/task-bootstrap"},
			ResourceProfile: "medium",
			RuntimePolicy: RuntimePolicy{
				SandboxType:       "forgevm",
				Backend:           "gvisor",
				IsolationRequired: "container",
				RiskClass:         "untrusted",
				Environment:       "development",
				WorkspaceRef:      "project-a",
			},
		}
	}
	completed, err := provider.Create(context.Background(), baseSpec("task-completed", completedWorktree))
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Start(context.Background(), completed.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Exec(context.Background(), completed.TaskID, []string{"/opt/workmesh/test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Collect(context.Background(), completed.TaskID); err != nil {
		t.Fatal(err)
	}
	if err := provider.Destroy(context.Background(), completed.TaskID); err != nil {
		t.Fatal(err)
	}
	cancelled, err := provider.Create(context.Background(), baseSpec("task-cancelled", cancelledWorktree))
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Start(context.Background(), cancelled.TaskID); err != nil {
		t.Fatal(err)
	}
	if err := provider.Cancel(context.Background(), cancelled.TaskID); err != nil {
		t.Fatal(err)
	}
	if err := provider.Destroy(context.Background(), cancelled.TaskID); err != nil {
		t.Fatal(err)
	}
	if strings.Join(operations, ",") != "capabilities,create,start,exec,collect,destroy,capabilities,create,start,cancel,destroy" {
		t.Fatalf("CLI 生命周期顺序错误: %v", operations)
	}
}

func TestCLITaskBackendCreateRejectsMissingOrMismatchedEnforcementReceipt(t *testing.T) {
	spec := validSpec()
	spec.RuntimePolicy = RuntimePolicy{SandboxType: "forgevm", Backend: "gvisor", IsolationRequired: "container", WorkspaceRef: "project-a", AllowedHosts: []string{"api.example.test"}}
	spec.ResourceProfile = "small"
	spec.ResourceLimits, _ = ResourceLimitsForProfile("small")
	for _, tc := range []struct {
		name   string
		mutate func(*SandboxEnforcementReceipt)
	}{
		{name: "missing pre-start enforcement", mutate: func(r *SandboxEnforcementReceipt) { r.PreStartEnforcement = false }},
		{name: "resource mismatch", mutate: func(r *SandboxEnforcementReceipt) { r.ResourceLimits.MemoryBytes /= 2 }},
		{name: "workspace mismatch", mutate: func(r *SandboxEnforcementReceipt) { r.WorkspaceRef = "project-b" }},
		{name: "allowlist not enforced", mutate: func(r *SandboxEnforcementReceipt) { r.NetworkAllowlistEnforced = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			receipt := enforcementReceiptForSpec(spec)
			tc.mutate(&receipt)
			backend := &CLITaskBackend{command: "/opt/workmesh/sandbox-cli", outputLimit: 4096, run: func(context.Context, string, []string, int) ([]byte, error) {
				response, err := json.Marshal(map[string]any{"ok": true, "data": map[string]any{"sandboxId": "sandbox-1", "enforcement": receipt}})
				return response, err
			}}
			if _, err := backend.Create(context.Background(), spec); err == nil {
				t.Fatal("缺失或不匹配的逐任务隔离回执必须拒绝")
			}
		})
	}
}

func enforcementReceiptForSpec(spec TaskSpec) SandboxEnforcementReceipt {
	policy := spec.RuntimePolicy.normalized()
	return SandboxEnforcementReceipt{
		ProtocolVersion: SandboxProtocolVersion, SandboxType: policy.SandboxType, Backend: policy.Backend,
		Isolation: policy.IsolationRequired, WorkspaceRef: policy.WorkspaceRef, ResourceLimits: spec.ResourceLimits,
		WorkspaceIsolation: true, NetworkIsolation: true, NetworkAllowlistEnforced: true, AllowedHosts: policy.AllowedHosts,
		PreStartEnforcement: true, ProcessTreeContainment: true, HardCPU: true, HardMemory: true,
		HardPIDs: true, HardDisk: true,
	}
}
