// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dockerGuardOperateTest(t *testing.T, operation string) (int, map[string]any) {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/hosts/firewall/docker/operate", bytes.NewBufferString(`{"operation":"`+operation+`"}`))
	handleDockerGuardOperate(response, request)
	var envelope struct {
		Code any            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s response: %v (%s)", operation, err, response.Body.String())
	}
	return response.Code, envelope.Data
}

func TestDockerGuardOperateBindsAndUnbindsDockerUser(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "iptables.log")
	path := filepath.Join(dir, "iptables")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + logPath + "\"\ncase \"$1\" in -C) exit 1;; *) exit 0;; esac\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WORKMESH_ALLOW_FIREWALL_MUTATION", "1")

	status, data := dockerGuardOperateTest(t, "initialize")
	if status != http.StatusOK || data["initialized"] != true || data["bound"] != false {
		t.Fatalf("initialize status=%d data=%#v", status, data)
	}
	status, data = dockerGuardOperateTest(t, "bind")
	if status != http.StatusOK || data["initialized"] != true || data["bound"] != true {
		t.Fatalf("bind status=%d data=%#v", status, data)
	}
	status, data = dockerGuardOperateTest(t, "unbind")
	if status != http.StatusOK || data["initialized"] != true || data["bound"] != false || data["changed"] != false {
		t.Fatalf("unbind status=%d data=%#v", status, data)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"-N WORKMESH_DOCKER", "-I DOCKER-USER 1 -j WORKMESH_DOCKER", "-L WORKMESH_DOCKER -n"} {
		if !strings.Contains(string(content), expected) {
			t.Fatalf("iptables invocation %q missing in %s", expected, content)
		}
	}
}

func TestDockerGuardOperateRequiresMutationGate(t *testing.T) {
	t.Setenv("WORKMESH_ALLOW_FIREWALL_MUTATION", "")
	status, _ := dockerGuardOperateTest(t, "bind")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want %d", status, http.StatusServiceUnavailable)
	}
}

func TestDockerGuardPolicyRejectsMismatchedAddressFamily(t *testing.T) {
	policy := dockerGuardPolicy{Family: "ipv4", HostIP: "192.0.2.10", HostPort: 8080, Protocol: "tcp"}
	if err := validateDockerGuardPolicy(policy, "deny_sources", []string{"2001:db8::/32"}); err == nil || !strings.Contains(err.Error(), "IPv4") {
		t.Fatalf("expected IPv4 source validation error, got %v", err)
	}
	policy.HostIP = "bad;address"
	if err := validateDockerGuardPolicy(policy, "deny_all", nil); err == nil {
		t.Fatal("invalid host address accepted")
	}
}

func TestDockerFirewallSyncUsesSQLitePoliciesAndRollsSourceForward(t *testing.T) {
	store := openLogsTestStore(t)
	state := map[string]dockerGuardPolicy{
		dockerGuardKey("ipv4", "0.0.0.0", 8443, "tcp"): {Family: "ipv4", HostIP: "0.0.0.0", HostPort: 8443, Protocol: "tcp", Mode: "deny_all", PolicyUUID: "policy-deny"},
		dockerGuardKey("ipv4", "0.0.0.0", 8080, "tcp"): {Family: "ipv4", HostIP: "0.0.0.0", HostPort: 8080, Protocol: "tcp", Mode: "allow_sources", Sources: []string{"192.0.2.0/24"}, PolicyUUID: "policy-allow"},
	}
	if err := saveDockerGuardState(state); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "iptables")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \""+filepath.Join(dir, "calls.log")+"\"\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ip6Path := filepath.Join(dir, "ip6tables")
	if err := os.WriteFile(ip6Path, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \""+filepath.Join(dir, "calls.log")+"\"\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WORKMESH_ALLOW_FIREWALL_MUTATION", "1")

	preview := httptest.NewRecorder()
	handleFirewallSyncPreview(preview, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/firewall/rules/sync/preview", bytes.NewBufferString(`{"subsystem":"docker","targetProvider":"iptables"}`)))
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), `"ready":2`) || !strings.Contains(preview.Body.String(), `"targetRules"`) {
		t.Fatalf("unexpected docker preview: %d %s", preview.Code, preview.Body.String())
	}
	sync := httptest.NewRecorder()
	handleFirewallSync(sync, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/firewall/rules/sync", bytes.NewBufferString(`{"subsystem":"docker","targetProvider":"iptables","resetSource":true}`)))
	if sync.Code != http.StatusOK || !strings.Contains(sync.Body.String(), `"succeeded":2`) || !strings.Contains(sync.Body.String(), `"failed":0`) {
		t.Fatalf("unexpected docker sync: %d %s", sync.Code, sync.Body.String())
	}
	remaining := dockerGuardState()
	if _, ok := remaining[dockerGuardKey("ipv4", "0.0.0.0", 8443, "tcp")]; ok {
		t.Fatal("successful policy should be removed when resetSource is enabled")
	}
	if _, ok := remaining[dockerGuardKey("ipv4", "0.0.0.0", 8080, "tcp")]; ok {
		t.Fatal("successful policy should be removed in resetSource mode")
	}
	if calls, err := os.ReadFile(filepath.Join(dir, "calls.log")); err != nil || !strings.Contains(string(calls), "-A WORKMESH_DOCKER") || !strings.Contains(string(calls), "-s 192.0.2.0/24") {
		t.Fatalf("iptables Docker rule was not written: err=%v calls=%s", err, calls)
	}
	_ = store
}
