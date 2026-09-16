// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// nftablesInventory reads the native JSON ruleset and preserves unsupported
// expressions as opaque observed rules instead of inventing filter fields.
func nftablesInventory(ctx context.Context, scope map[string]any) (map[string]any, error) {
	out, err := runFirewallCommand(ctx, "nft", "-j", "list", "ruleset")
	if err != nil {
		return nil, err
	}
	var document struct {
		Nftables []map[string]json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal(out, &document); err != nil {
		return nil, fmt.Errorf("解析 nftables JSON 失败: %w", err)
	}
	wantedFamily, _ := scope["family"].(string)
	wantedTable, _ := scope["table"].(string)
	wantedChain, _ := scope["chain"].(string)
	items := make([]map[string]any, 0)
	for _, entry := range document.Nftables {
		rawRule, ok := entry["rule"]
		if !ok {
			continue
		}
		var native struct {
			Family  string `json:"family"`
			Table   string `json:"table"`
			Chain   string `json:"chain"`
			Handle  uint64 `json:"handle"`
			Comment string `json:"comment"`
			Expr    []any  `json:"expr"`
		}
		if json.Unmarshal(rawRule, &native) != nil {
			continue
		}
		family := "inet"
		if native.Family == "ip" {
			family = "ipv4"
		} else if native.Family == "ip6" {
			family = "ipv6"
		}
		if wantedFamily != "" && wantedFamily != family && !(wantedFamily == "inet" && (family == "ipv4" || family == "ipv6")) {
			continue
		}
		if wantedTable != "" && wantedTable != native.Table || wantedChain != "" && wantedChain != native.Chain {
			continue
		}
		canonical := string(rawRule)
		rule := map[string]any{"scope": map[string]any{"provider": "nftables", "family": family, "table": native.Table, "chain": native.Chain, "direction": "input"}, "protocol": "all", "action": "accept", "nativeKind": "opaque", "description": native.Comment, "nativeHandle": native.Handle, "raw": canonical}
		parseNftExpressions(native.Expr, rule)
		parseStatus := "opaque"
		if rule["protocol"] != "all" || rule["action"] != "accept" || rule["destinationPort"] != nil || rule["sourceAddress"] != nil || rule["destinationAddress"] != nil {
			rule["nativeKind"] = "rule"
			parseStatus = "supported"
		}
		rule["uuid"] = firewallRuleUUID(canonical)
		observed := map[string]any{"rule": rule, "locator": map[string]any{"provider": "nftables", "scopeKey": native.Family + ":" + native.Table + ":" + native.Chain, "nativeId": fmt.Sprint(native.Handle), "canonical": canonical}, "parseStatus": parseStatus, "raw": canonical, "protected": false}
		items = append(items, map[string]any{"rule": rule, "observed": observed, "state": "external", "match": "opaque"})
	}
	return map[string]any{"items": items, "notices": []any{}}, nil
}

// parseNftExpressions extracts only the common expressions emitted by the
// WorkMesh adapter. Unknown expressions remain opaque and are never guessed.
func parseNftExpressions(expressions []any, rule map[string]any) {
	for _, expression := range expressions {
		object, ok := expression.(map[string]any)
		if !ok {
			continue
		}
		if match, ok := object["match"].(map[string]any); ok {
			left, _ := match["left"].(map[string]any)
			payload, _ := left["payload"].(map[string]any)
			protocol, _ := payload["protocol"].(string)
			field, _ := payload["field"].(string)
			right := match["right"]
			switch field {
			case "dport":
				if protocol == "tcp" || protocol == "udp" {
					rule["protocol"] = protocol
					rule["destinationPort"] = fmt.Sprint(right)
				}
			case "sport":
				if protocol == "tcp" || protocol == "udp" {
					rule["protocol"] = protocol
					rule["sourcePort"] = fmt.Sprint(right)
				}
			case "saddr":
				rule["sourceAddress"] = fmt.Sprint(right)
			case "daddr":
				rule["destinationAddress"] = fmt.Sprint(right)
			}
		}
		for _, action := range []string{"accept", "drop", "reject"} {
			if _, ok := object[action]; ok {
				rule["action"] = action
			}
		}
	}
	if strings.TrimSpace(fmt.Sprint(rule["protocol"])) == "" {
		rule["protocol"] = "all"
	}
}
