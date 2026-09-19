// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/runtime/gateway"
)

func TestReconcileMachineIdentityInvalidatesCopiedBinding(t *testing.T) {
	store := &GatewayStateStore{
		status:             gateway.Status{Registration: gateway.RegistrationRegistered, Connected: true},
		auth:               gateway.Authorization{BindingID: "binding-source", AccessToken: "copied-token"},
		machineCode:        "sha256:" + strings.Repeat("b", 64),
		boundMachineCode:   "sha256:" + strings.Repeat("a", 64),
		fingerprintVersion: 1,
		identityStatus:     "ready",
		statePath:          filepath.Join(t.TempDir(), "gateway-binding.json"),
	}
	if !store.reconcileMachineIdentity() {
		t.Fatal("复制到不同机器后必须轮换 Ed25519 身份")
	}
	if store.auth.BindingID != "" || store.auth.AccessToken != "" || store.status.Connected {
		t.Fatalf("复制的绑定和 Token 未停用: status=%+v auth=%+v", store.status, store.auth)
	}
	if store.previousBindingID != "binding-source" || store.identityStatus != "machine_changed" {
		t.Fatalf("旧绑定审计或身份状态缺失: %+v", store)
	}
}

func TestGatewayDeviceIDDependsOnlyOnMachineCode(t *testing.T) {
	code := "sha256:" + strings.Repeat("c", 64)
	first := gatewayDeviceID(code)
	if first != gatewayDeviceID(code) || !strings.HasPrefix(first, "device-") {
		t.Fatalf("Gateway 设备 ID 不稳定: %q", first)
	}
	if first == gatewayDeviceID("sha256:"+strings.Repeat("d", 64)) {
		t.Fatal("不同机器码不得生成相同 Gateway 设备 ID")
	}
}

func TestIdentityMachineMarkerRejectsCopiedIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gateway-identity.ed25519")
	if _, err := gateway.LoadOrCreateIdentity(path); err != nil {
		t.Fatal(err)
	}
	store := &GatewayStateStore{identityPath: path, identityMachinePath: filepath.Join(dir, "gateway-identity.machine"), machineCode: "sha256:" + strings.Repeat("b", 64), auth: gateway.Authorization{BindingID: "binding"}, status: gateway.Status{Registration: gateway.RegistrationRegistered, Connected: true}, identityStatus: "ready", statePath: filepath.Join(dir, "binding.json")}
	if err := os.WriteFile(store.identityMachinePath, []byte("sha256:"+strings.Repeat("a", 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	if !store.reconcileIdentityMachineMarker() || store.identityStatus != "machine_changed" || store.status.Connected {
		t.Fatal("复制的身份标记未触发隔离")
	}
}
