// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateForwardRule(t *testing.T) {
	item, err := validateForwardRule(map[string]any{"family": "ipv4", "protocol": "tcp", "port": "8080", "targetIP": "10.0.0.2", "targetPort": "80"})
	if err != nil || item.TargetIP != "10.0.0.2" {
		t.Fatalf("valid rule rejected: %+v %v", item, err)
	}
	for _, rule := range []map[string]any{
		{"protocol": "icmp", "port": "80", "targetIP": "10.0.0.2", "targetPort": "80"},
		{"protocol": "tcp", "port": "0", "targetIP": "10.0.0.2", "targetPort": "80"},
		{"protocol": "tcp", "port": "80", "targetIP": "10;reboot", "targetPort": "80"},
	} {
		if _, err := validateForwardRule(rule); err == nil {
			t.Fatalf("unsafe rule accepted: %+v", rule)
		}
	}
}

func TestForwardMutationRequiresExplicitGate(t *testing.T) {
	t.Setenv("WORKMESH_ALLOW_FIREWALL_MUTATION", "")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/hosts/firewall/forward/operate", handleForwardRuleOperate)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/firewall/forward/operate", bytes.NewBufferString(`{"rules":[]}`)))
	if res.Code != http.StatusServiceUnavailable || !strings.Contains(res.Body.String(), "WORKMESH_ALLOW_FIREWALL_MUTATION") {
		t.Fatalf("unexpected gate response: %d %s", res.Code, res.Body.String())
	}
}

func TestFirewallRuleMutationsRequireExplicitGate(t *testing.T) {
	t.Setenv("WORKMESH_ALLOW_FIREWALL_MUTATION", "")
	cases := []struct {
		name string
		h    http.HandlerFunc
		body string
	}{
		{"create", handleFirewallRuleCreate, `{"items":[]}`},
		{"delete", handleFirewallRuleDelete, `{"uuids":[]}`},
		{"reset", handleFirewallRuleReset, `{"provider":"iptables"}`},
		{"update", handleFirewallRuleUpdate, `{"uuid":"x","rule":{}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v2/hosts/firewall/rules", bytes.NewBufferString(tc.body))
			tc.h(res, req)
			if res.Code != http.StatusServiceUnavailable || !strings.Contains(res.Body.String(), "WORKMESH_ALLOW_FIREWALL_MUTATION") {
				t.Fatalf("unexpected gate response: %d %s", res.Code, res.Body.String())
			}
		})
	}
}

func TestFirewallNativeDetailValidation(t *testing.T) {
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/hosts/firewall/rules/native/detail", bytes.NewBufferString(`{"provider":"ufw","nativeKind":"ufw_application","name":"bad;command"}`))
	handleFirewallNativeDetail(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestFirewallNftablesBackendAccepted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nft")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WORKMESH_ALLOW_FIREWALL_MUTATION", "1")
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/hosts/firewall/backend", bytes.NewBufferString(`{"backend":"nftables","operation":"initialize"}`))
	handleFirewallBackendOperate(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"backend":"nftables"`) {
		t.Fatalf("unexpected nftables response: %d %s", res.Code, res.Body.String())
	}
}

func TestFirewallNativeBackendDoesNotFakeManagedChain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "firewall-cmd")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WORKMESH_ALLOW_FIREWALL_MUTATION", "1")
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/hosts/firewall/backend", bytes.NewBufferString(`{"backend":"firewalld","operation":"initialize"}`))
	handleFirewallBackendOperate(res, req)
	if res.Code != http.StatusServiceUnavailable || !strings.Contains(res.Body.String(), "没有可安全托管") {
		t.Fatalf("unexpected native backend response: %d %s", res.Code, res.Body.String())
	}
}

func TestFirewallProviderRuleArgs(t *testing.T) {
	rule := map[string]any{"scope": map[string]any{"provider": "firewalld", "family": "ipv4", "chain": "public"}, "protocol": "tcp", "sourceAddress": "10.0.0.0/8", "destinationPort": "443", "action": "accept"}
	args, _, err := firewallRuleArgs(rule, "-A")
	if err != nil || len(args) < 3 || args[1] != "--add-rich-rule" || !strings.Contains(strings.Join(args, " "), `source address="10.0.0.0/8"`) {
		t.Fatalf("firewalld args=%v err=%v", args, err)
	}
	rule["scope"].(map[string]any)["provider"] = "ufw"
	rule["scope"].(map[string]any)["chain"] = "INPUT"
	args, _, err = firewallRuleArgs(rule, "-D")
	if err != nil || len(args) < 2 || args[0] != "delete" || args[1] != "allow" {
		t.Fatalf("ufw args=%v err=%v", args, err)
	}
}

func TestFirewallRuleReorderUsesDeleteAndInsert(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	script := filepath.Join(dir, "iptables")
	content := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + logPath + "\"\nif [ \"$1\" = \"-S\" ]; then printf '%s\\n' '-A WORKMESH_BASIC -p tcp --dport 80 -j ACCEPT'; fi\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WORKMESH_ALLOW_FIREWALL_MUTATION", "1")
	ruleLine := "-A WORKMESH_BASIC -p tcp --dport 80 -j ACCEPT"
	// UUIDs are derived from the canonical iptables line returned by -S.
	uuid := firewallRuleUUID(ruleLine)
	position := int64(1)
	body := fmt.Sprintf(`{"uuid":%q,"targetPosition":%d}`, uuid, position)
	res := httptest.NewRecorder()
	httpReq := httptest.NewRequest(http.MethodPost, "/api/v2/hosts/firewall/rules/reorder", bytes.NewBufferString(body))
	handleFirewallRuleReorder(res, httpReq)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"reordered":true`) {
		t.Fatalf("unexpected reorder response: %d %s", res.Code, res.Body.String())
	}
	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "-D WORKMESH_BASIC") || !strings.Contains(string(calls), "-I WORKMESH_BASIC 1") {
		t.Fatalf("expected delete and insert calls, got %q", string(calls))
	}
}

func TestNftablesInventoryUsesNativeJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nft")
	fixture := `{"nftables":[{"rule":{"family":"ip","table":"workmesh","chain":"input","handle":7,"expr":[{"accept":null}]}}]}`
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s' '"+fixture+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	result, err := nftablesInventory(context.Background(), map[string]any{"provider": "nftables", "family": "ipv4"})
	if err != nil || len(result["items"].([]map[string]any)) != 1 {
		t.Fatalf("nft inventory=%v err=%v", result, err)
	}
}
