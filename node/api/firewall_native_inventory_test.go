// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFirewallFake(t *testing.T, name, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestFirewalldInventoryParsesRichRules(t *testing.T) {
	writeFirewallFake(t, "firewall-cmd", `
case "$*" in
  "--get-zones") printf '%s\n' 'public trusted' ;;
		"--zone=public --list-rich-rules") printf '%s\n' 'rule family="ipv4" source address="10.0.0.0/8" destination address="192.0.2.0/24" port port="443" protocol="tcp" accept' ;;
  "--zone=trusted --list-rich-rules") printf '%s\n' 'rule family="ipv6" service name="ssh" accept' ;;
esac`)
	result, err := firewalldInventory(context.Background(), map[string]any{"provider": "firewalld", "family": "ipv4"})
	if err != nil {
		t.Fatal(err)
	}
	items := result["items"].([]map[string]any)
	if len(items) != 1 {
		t.Fatalf("items=%d want 1", len(items))
	}
	rule := items[0]["rule"].(map[string]any)
	if rule["protocol"] != "tcp" || rule["destinationPort"] != "443" || rule["sourceAddress"] != "10.0.0.0/8" || rule["destinationAddress"] != "192.0.2.0/24" || rule["action"] != "accept" {
		t.Fatalf("unexpected rule: %#v", rule)
	}
	if items[0]["observed"].(map[string]any)["parseStatus"] != "supported" {
		t.Fatalf("supported rule was not marked supported")
	}
}

func TestUFWInventoryParsesNumberedRules(t *testing.T) {
	writeFirewallFake(t, "ufw", `printf '%s\n' 'Status: active' '[ 1] 22/tcp ALLOW IN 10.0.0.0/8' '[ 2] 53/udp DENY IN Anywhere' '[ 3] 80/tcp ALLOW IN Anywhere (v6)'`)
	result, err := ufwInventory(context.Background(), map[string]any{"provider": "ufw", "family": "inet"})
	if err != nil {
		t.Fatal(err)
	}
	items := result["items"].([]map[string]any)
	if len(items) != 3 {
		t.Fatalf("items=%d want 3", len(items))
	}
	first := items[0]["rule"].(map[string]any)
	if first["protocol"] != "tcp" || first["destinationPort"] != "22" || first["action"] != "accept" || first["sourceAddress"] != "10.0.0.0/8" {
		t.Fatalf("unexpected ufw rule: %#v", first)
	}
	second := items[1]["rule"].(map[string]any)
	if second["action"] != "drop" || second["protocol"] != "udp" {
		t.Fatalf("unexpected deny rule: %#v", second)
	}
	if strings.TrimSpace(first["uuid"].(string)) == "" {
		t.Fatal("uuid is empty")
	}
	args, _, err := firewallRuleArgs(first, "-D")
	if err != nil || strings.Join(args, " ") != "delete 1" {
		t.Fatalf("ufw delete should use native rule number, args=%v err=%v", args, err)
	}
}

func TestFirewalldInventoryPropagatesDependencyError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := firewalldInventory(context.Background(), map[string]any{"provider": "firewalld", "zone": "public"}); err == nil {
		t.Fatal("expected missing firewall-cmd error")
	}
}
