// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package taskruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func testFirecrackerConfig() FirecrackerRuntimeConfig {
	return FirecrackerRuntimeConfig{
		BinaryPath:    "/opt/workmesh/bin/firecracker",
		KernelPath:    "/opt/workmesh/runtime/vmlinux",
		RootFSPath:    "/opt/workmesh/runtime/rootfs.ext4",
		KVMPath:       "/dev/kvm",
		WorkspaceRoot: "/opt/workmesh/workspaces",
		CgroupRoot:    "/sys/fs/cgroup/workmesh",
		VMMemoryBytes: 512 << 20,
		VCPUCount:     2,
		DiskBytes:     1 << 30,
	}
}

func TestValidateFirecrackerConfigRequiresAssetsAndKVM(t *testing.T) {
	config := testFirecrackerConfig()
	exists := func(path string) (bool, error) { return path != config.KVMPath, nil }
	if err := ValidateFirecrackerConfig(config, exists, exists); err == nil || !strings.Contains(strings.ToLower(err.Error()), "kvm") {
		t.Fatalf("expected KVM failure, got %v", err)
	}
	if err := ValidateFirecrackerConfig(config, func(string) (bool, error) { return true, nil }, func(string) (bool, error) { return true, nil }); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}

func TestBuildFirecrackerBootPlanRejectsShellInput(t *testing.T) {
	config := testFirecrackerConfig()
	config.BinaryPath += ";touch /tmp/pwned"
	if _, err := BuildFirecrackerBootPlan(config, "/run/workmesh/task.sock", "/run/workmesh/task.json"); err == nil {
		t.Fatal("expected shell input rejection")
	}
}

func TestBuildFirecrackerBootPlanUsesPerTaskConfig(t *testing.T) {
	plan, err := BuildFirecrackerBootPlan(testFirecrackerConfig(), "/run/workmesh/task.sock", "/run/workmesh/task.json")
	if err != nil {
		t.Fatal(err)
	}
	if plan.BinaryPath != "/opt/workmesh/bin/firecracker" || len(plan.Args) != 4 || plan.Args[1] != "/run/workmesh/task.sock" || plan.Args[3] != "/run/workmesh/task.json" {
		t.Fatalf("unexpected Firecracker boot plan: %+v", plan)
	}
}

func TestBuildFirecrackerMachineConfigContainsIsolatedDrivesAndVsock(t *testing.T) {
	config := testFirecrackerConfig()
	config.NetworkDevice = "wm-tap0"
	config.VsockDevice = "vsock0"
	body, err := BuildFirecrackerMachineConfig(config, TaskSpec{TaskID: "task-1"}, "/opt/workmesh/workspaces/project-a/task-1/workspace.ext4", "/run/workmesh/task-1.vsock", 42)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, fragment := range []string{"\"drive_id\":\"rootfs\"", "\"drive_id\":\"workspace\"", "\"guest_cid\":42", "\"host_dev_name\":\"wm-tap0\""} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("machine-config missing %s: %s", fragment, text)
		}
	}
}

func TestBuildFirecrackerMachineConfigRejectsWorkspaceEscape(t *testing.T) {
	config := testFirecrackerConfig()
	if _, err := BuildFirecrackerMachineConfig(config, TaskSpec{TaskID: "task-1"}, "/opt/workmesh/other/workspace.ext4", "/run/workmesh/task-1.vsock", 42); err == nil {
		t.Fatal("expected workspace escape rejection")
	}
}

func TestFirecrackerBackendFailsClosedWithoutGuestExecutor(t *testing.T) {
	backend := &FirecrackerTaskBackend{config: testFirecrackerConfig()}
	if _, err := backend.Create(context.Background(), TaskSpec{}); err == nil {
		t.Fatal("expected create to fail closed")
	} else if !errors.As(err, new(*FirecrackerUnavailableError)) {
		t.Fatalf("expected FirecrackerUnavailableError, got %T: %v", err, err)
	}
	if _, err := backend.Capabilities(context.Background()); err == nil {
		t.Fatal("expected capabilities to fail closed")
	}
}

func TestFirecrackerGuestFrameRoundTripAndRejectsTrailingData(t *testing.T) {
	frame, err := EncodeFirecrackerGuestRequest(FirecrackerGuestRequest{RequestID: "req-1", Operation: "exec", Argv: []string{"go", "test"}})
	if err != nil {
		t.Fatal(err)
	}
	responseFrame := append([]byte{0, 0, 0, 0}, []byte(`{"requestId":"req-1","exitCode":0,"stdout":"ok"}`)...)
	responseFrame[0] = 0
	responseFrame[1] = 0
	responseFrame[2] = 0
	responseFrame[3] = byte(len(responseFrame) - 4)
	var response FirecrackerGuestResponse
	if err := DecodeFirecrackerGuestFrame(responseFrame, &response); err != nil {
		t.Fatal(err)
	}
	if response.RequestID != "req-1" || response.Stdout != "ok" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if err := DecodeFirecrackerGuestFrame(append(responseFrame, 0), &response); err == nil {
		t.Fatal("expected trailing frame data rejection")
	}
	trailingJSON := append([]byte(nil), responseFrame...)
	trailingJSON = append(trailingJSON, ' ', '{', '}')
	trailingJSON[0] = byte(len(trailingJSON) - 4)
	if err := DecodeFirecrackerGuestFrame(trailingJSON, &response); err == nil {
		t.Fatal("expected trailing JSON rejection")
	}
	if len(frame) <= 4 {
		t.Fatal("expected encoded request body")
	}
}

func TestFirecrackerGuestRequestRejectsUnsafeOperationAndArg(t *testing.T) {
	if _, err := EncodeFirecrackerGuestRequest(FirecrackerGuestRequest{RequestID: "r", Operation: "shell"}); err == nil {
		t.Fatal("expected operation rejection")
	}
	if _, err := EncodeFirecrackerGuestRequest(FirecrackerGuestRequest{RequestID: "r", Operation: "exec", Argv: []string{"bad\narg"}}); err == nil {
		t.Fatal("expected argv rejection")
	}
}
