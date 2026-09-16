// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"regexp"
	"strings"
)

var (
	firewalldAttrPattern = regexp.MustCompile(`([a-zA-Z]+)="([^"]*)"`)
	firewalldSourceAddr  = regexp.MustCompile(`\bsource\s+address="([^"]+)"`)
	firewalldDestAddr    = regexp.MustCompile(`\bdestination\s+address="([^"]+)"`)
	ufwRulePattern       = regexp.MustCompile(`^\[\s*([0-9]+)\]\s+(.+?)\s+(ALLOW|DENY|REJECT)\s+(?:IN|OUT)?\s*(.*)$`)
	ufwPortPattern       = regexp.MustCompile(`^([^/\s]+)/(tcp|udp)(?:\s+\(v6\))?$`)
)

// firewalldInventory reads rich rules from the live firewalld daemon. The
// daemon is queried directly so the result reflects runtime state rather than
// a cached compatibility file.
func firewalldInventory(ctx context.Context, scope map[string]any) (map[string]any, error) {
	zone := strings.TrimSpace(firewallString(scope, "zone"))
	if zone == "" {
		zone = strings.TrimSpace(firewallString(scope, "chain"))
	}
	zones := []string{zone}
	if zone == "" {
		out, err := runFirewallCommand(ctx, "firewall-cmd", "--get-zones")
		if err != nil {
			return nil, err
		}
		zones = strings.Fields(string(out))
	}
	items := make([]map[string]any, 0)
	for _, currentZone := range zones {
		if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`).MatchString(currentZone) {
			continue
		}
		out, err := runFirewallCommand(ctx, "firewall-cmd", "--zone="+currentZone, "--list-rich-rules")
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			rule, parseStatus := parseFirewalldRichRule(line, currentZone)
			if !firewallFamilyMatches(rule, scope) {
				continue
			}
			items = append(items, map[string]any{
				"rule": rule,
				"observed": map[string]any{
					"rule":        rule,
					"locator":     map[string]any{"provider": "firewalld", "scopeKey": "firewalld:" + currentZone, "canonical": line},
					"parseStatus": parseStatus, "raw": line, "protected": false,
				},
				"state": "external", "match": "none",
			})
		}
	}
	return map[string]any{"items": items, "notices": []any{}}, nil
}

func parseFirewalldRichRule(line, zone string) (map[string]any, string) {
	family, protocol, action := "ipv4", "all", "accept"
	rule := map[string]any{
		"scope":      map[string]any{"provider": "firewalld", "family": family, "zone": zone, "chain": zone, "direction": "input"},
		"nativeKind": "rich_rule", "protocol": protocol, "action": action, "description": line,
		"raw": line,
	}
	attrs := map[string]string{}
	for _, match := range firewalldAttrPattern.FindAllStringSubmatch(line, -1) {
		attrs[strings.ToLower(match[1])] = match[2]
	}
	if value := attrs["family"]; value == "ipv6" {
		family = "ipv6"
	} else if value == "ipv4" {
		family = "ipv4"
	}
	rule["scope"].(map[string]any)["family"] = family
	if match := firewalldSourceAddr.FindStringSubmatch(line); match != nil {
		rule["sourceAddress"] = match[1]
	}
	if match := firewalldDestAddr.FindStringSubmatch(line); match != nil {
		rule["destinationAddress"] = match[1]
	}
	if value := attrs["protocol"]; value == "tcp" || value == "udp" {
		protocol = value
		rule["protocol"] = value
		if port := attrs["port"]; port != "" {
			rule["destinationPort"] = port
		}
	}
	for _, candidate := range []string{"accept", "drop", "reject"} {
		if strings.Contains(line, " "+candidate) || strings.HasSuffix(line, candidate) {
			action = candidate
			rule["action"] = candidate
		}
	}
	parseStatus := "supported"
	if strings.Contains(line, " service ") || strings.Contains(line, " log ") || strings.Contains(line, " masquerade ") {
		rule["nativeKind"] = "opaque"
		parseStatus = "opaque"
	}
	rule["uuid"] = firewallRuleUUID(line)
	return rule, parseStatus
}

// ufwInventory parses the stable, human-readable numbered rule output. UFW
// has no machine-readable equivalent on older distributions.
func ufwInventory(ctx context.Context, scope map[string]any) (map[string]any, error) {
	out, err := runFirewallCommand(ctx, "ufw", "status", "numbered")
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		match := ufwRulePattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		destination, actionName, source := strings.TrimSpace(match[2]), strings.ToLower(match[3]), strings.TrimSpace(match[4])
		protocol, port := "all", ""
		if portMatch := ufwPortPattern.FindStringSubmatch(destination); portMatch != nil {
			port, protocol = portMatch[1], portMatch[2]
		} else if strings.Contains(destination, "/") {
			// UFW may print a service name (for example OpenSSH); retain it as opaque.
			protocol = "all"
		}
		action := map[string]string{"allow": "accept", "deny": "drop", "reject": "reject"}[actionName]
		if action == "" {
			continue
		}
		family := "ipv4"
		if strings.Contains(source, ":") || strings.Contains(destination, ":") || strings.Contains(source, "(v6)") || strings.Contains(destination, "(v6)") {
			family = "ipv6"
		}
		rule := map[string]any{
			"scope":      map[string]any{"provider": "ufw", "family": family, "chain": "INPUT", "direction": "input"},
			"nativeKind": "ufw_rule", "protocol": protocol, "action": action, "description": line,
			"orderIndex": match[1], "nativeId": match[1], "uuid": firewallRuleUUID(line),
		}
		if port != "" {
			rule["destinationPort"] = port
		}
		if source != "" && source != "Anywhere" && source != "Anywhere (v6)" {
			rule["sourceAddress"] = strings.Fields(source)[0]
		}
		if !firewallFamilyMatches(rule, scope) {
			continue
		}
		items = append(items, map[string]any{
			"rule":     rule,
			"observed": map[string]any{"rule": rule, "locator": map[string]any{"provider": "ufw", "scopeKey": "ufw:INPUT", "nativeId": match[1], "canonical": line}, "parseStatus": "supported", "raw": line, "protected": false},
			"state":    "external", "match": "none",
		})
	}
	return map[string]any{"items": items, "notices": []any{}}, nil
}

func firewallFamilyMatches(rule, scope map[string]any) bool {
	wanted := strings.TrimSpace(firewallString(scope, "family"))
	if wanted == "" || wanted == "inet" {
		return true
	}
	actual, _ := rule["scope"].(map[string]any)
	return wanted == firewallString(actual, "family")
}
