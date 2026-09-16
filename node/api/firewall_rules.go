// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

func firewallRuleToMap(value any) map[string]any {
	if item, ok := value.(map[string]any); ok {
		return item
	}
	return nil
}

func firewallRuleKey(rule map[string]any) string {
	args, chain, err := firewallRuleArgs(rule, "-A")
	if err != nil {
		return ""
	}
	return strings.Join(append([]string{chain}, args[2:]...), " ")
}

func handleFirewallRuleCheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []struct {
			UUID string         `json:"uuid"`
			Rule map[string]any `json:"rule"`
		} `json:"items"`
	}
	if err := decodeJSON(r, &req); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	items := make([]map[string]any, 0, len(req.Items))
	for _, item := range req.Items {
		result := map[string]any{"decision": "blocked", "classification": "unsupported", "requestedRule": item.Rule, "requestedRuleKey": firewallRuleKey(item.Rule), "checkFlag": ""}
		if err := validateFirewallRule(item.Rule); err != nil {
			result["reason"] = err.Error()
			items = append(items, result)
			continue
		}
		result["decision"], result["classification"], result["reason"], result["allowedActions"] = "ready", "none", "规则可应用", []string{"create"}
		items = append(items, result)
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": items}})
}

func handleFirewallRuleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []struct {
			Rule   map[string]any `json:"rule"`
			Action string         `json:"action"`
		} `json:"items"`
	}
	if err := decodeJSON(r, &req); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if !requireFirewallMutation(w) {
		return
	}
	response := map[string]any{"succeeded": 0, "failed": 0, "skipped": 0, "errors": []any{}}
	for index, item := range req.Items {
		action := item.Action
		if action == "" {
			action = "create"
		}
		if action != "create" {
			response["skipped"] = response["skipped"].(int) + 1
			continue
		}
		args, _, err := firewallRuleArgs(item.Rule, "-A")
		if err != nil {
			response["failed"] = response["failed"].(int) + 1
			response["errors"] = append(response["errors"].([]any), map[string]any{"index": index, "error": err.Error()})
			continue
		}
		command, err := firewallProviderCommand(item.Rule)
		if err == nil {
			_, err = runFirewallCommand(r.Context(), command, args...)
		}
		if err != nil {
			response["failed"] = response["failed"].(int) + 1
			response["errors"] = append(response["errors"].([]any), map[string]any{"index": index, "error": err.Error()})
			continue
		}
		response["succeeded"] = response["succeeded"].(int) + 1
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": response})
}

func handleFirewallRuleDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UUIDs []string `json:"uuids"`
	}
	if err := decodeJSON(r, &req); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if !requireFirewallMutation(w) {
		return
	}
	// UUIDs are derived from canonical lines; resolve current inventory before deletion.
	all := make([]map[string]any, 0)
	for _, family := range []string{"ipv4", "ipv6"} {
		inv, err := firewallInventory(r.Context(), map[string]any{"provider": "iptables", "family": family})
		if err == nil {
			all = append(all, inv["items"].([]map[string]any)...)
		}
	}
	if inv, err := firewallInventory(r.Context(), map[string]any{"provider": "nftables", "family": "inet"}); err == nil {
		all = append(all, inv["items"].([]map[string]any)...)
	}
	for _, provider := range []string{"firewalld", "ufw"} {
		if inv, err := firewallInventory(r.Context(), map[string]any{"provider": provider, "family": "inet"}); err == nil {
			all = append(all, inv["items"].([]map[string]any)...)
		}
	}
	response := map[string]any{"succeeded": 0, "failed": 0, "errors": []any{}}
	for index, uuid := range req.UUIDs {
		var found map[string]any
		for _, item := range all {
			rule, _ := item["rule"].(map[string]any)
			if firewallString(rule, "uuid") == uuid || firewallRuleUUID(fmt.Sprint(rule["description"])) == uuid {
				found = rule
				break
			}
		}
		if found == nil {
			response["failed"] = response["failed"].(int) + 1
			response["errors"] = append(response["errors"].([]any), map[string]any{"index": index, "uuid": uuid, "error": "规则不存在或不是可管理规则"})
			continue
		}
		args, _, err := firewallRuleArgs(found, "-D")
		command, cmdErr := firewallProviderCommand(found)
		if err == nil {
			err = cmdErr
		}
		if err == nil {
			_, err = runFirewallCommand(r.Context(), command, args...)
		}
		if err != nil {
			response["failed"] = response["failed"].(int) + 1
			response["errors"] = append(response["errors"].([]any), map[string]any{"index": index, "uuid": uuid, "error": err.Error()})
			continue
		}
		response["succeeded"] = response["succeeded"].(int) + 1
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": response})
}

func handleFirewallRuleReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
	}
	if err := decodeJSON(r, &req); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	provider := req.Provider
	if provider == "" {
		provider = "iptables"
	}
	if provider != "iptables" && provider != "nftables" {
		wmhttp.JSON(w, 503, map[string]any{"code": "ERR", "message": "仅支持重置 iptables 或 nftables 托管规则"})
		return
	}
	if !requireFirewallMutation(w) {
		return
	}
	if provider == "nftables" {
		if _, err := runFirewallCommand(r.Context(), "nft", "flush", "table", "inet", "workmesh"); err != nil {
			wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": "清理 nftables 托管表失败: " + err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"removed": 1, "disabled": false, "provider": provider}})
		return
	}
	removed := 0
	for _, chain := range []string{"WORKMESH_BASIC_BEFORE", "WORKMESH_BASIC", "WORKMESH_BASIC_AFTER"} {
		if _, err := runFirewallCommand(r.Context(), "iptables", "-F", chain); err == nil {
			removed++
		}
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"removed": removed, "disabled": false}})
}

func handleFirewallRuleUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UUID string         `json:"uuid"`
		Rule map[string]any `json:"rule"`
	}
	if err := decodeJSON(r, &req); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if !requireFirewallMutation(w) {
		return
	}
	if strings.TrimSpace(req.UUID) == "" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "规则 UUID 不能为空"})
		return
	}
	// Update is deliberately implemented as delete-old/create-new only when the old
	// canonical rule can be resolved from the live firewall inventory.
	var old map[string]any
	for _, family := range []string{"ipv4", "ipv6"} {
		inv, err := firewallInventory(r.Context(), map[string]any{"provider": "iptables", "family": family})
		if err != nil {
			continue
		}
		for _, raw := range inv["items"].([]map[string]any) {
			rule, _ := raw["rule"].(map[string]any)
			if firewallString(rule, "uuid") == req.UUID || firewallRuleUUID(firewallString(rule, "description")) == req.UUID {
				old = rule
				break
			}
		}
	}
	if inv, err := firewallInventory(r.Context(), map[string]any{"provider": "nftables", "family": "inet"}); err == nil {
		for _, raw := range inv["items"].([]map[string]any) {
			rule, _ := raw["rule"].(map[string]any)
			if firewallRuleUUID(firewallString(rule, "description")) == req.UUID {
				old = rule
				break
			}
		}
	}
	for _, provider := range []string{"firewalld", "ufw"} {
		if inv, err := firewallInventory(r.Context(), map[string]any{"provider": provider, "family": "inet"}); err == nil {
			for _, raw := range inv["items"].([]map[string]any) {
				rule, _ := raw["rule"].(map[string]any)
				if firewallString(rule, "uuid") == req.UUID || firewallRuleUUID(firewallString(rule, "description")) == req.UUID {
					old = rule
					break
				}
			}
		}
		if old != nil {
			break
		}
	}
	if old == nil {
		wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": "规则不存在或不是可管理规则"})
		return
	}
	oldArgs, _, oldErr := firewallRuleArgs(old, "-D")
	oldCommand, oldCmdErr := firewallProviderCommand(old)
	newArgs, _, newErr := firewallRuleArgs(req.Rule, "-A")
	newCommand, newCmdErr := firewallProviderCommand(req.Rule)
	if oldErr != nil || oldCmdErr != nil || newErr != nil || newCmdErr != nil {
		for _, err := range []error{oldErr, oldCmdErr, newErr, newCmdErr} {
			if err != nil {
				wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
				return
			}
		}
	}
	if _, err := runFirewallCommand(r.Context(), oldCommand, oldArgs...); err != nil {
		wmhttp.JSON(w, 502, map[string]any{"code": "ERR", "message": "删除旧规则失败: " + err.Error()})
		return
	}
	if _, err := runFirewallCommand(r.Context(), newCommand, newArgs...); err != nil {
		// Best-effort compensation restores the old rule; the original update error is retained.
		_, _ = runFirewallCommand(r.Context(), oldCommand, func() []string { a, _, _ := firewallRuleArgs(old, "-A"); return a }()...)
		wmhttp.JSON(w, 502, map[string]any{"code": "ERR", "message": "创建新规则失败，已尝试恢复旧规则: " + err.Error()})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"updated": true}})
}

// handleFirewallRuleReorder moves a managed iptables rule to an explicit
// position. Native firewalld/ufw rules do not expose a portable ordering
// primitive, and nftables ordering is expression-specific, so those providers
// return a structured unsupported error instead of claiming success.
func handleFirewallRuleReorder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UUID           string `json:"uuid"`
		TargetPosition *int64 `json:"targetPosition"`
		Priority       *int   `json:"priority"`
	}
	if err := decodeJSON(r, &req); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if !requireFirewallMutation(w) {
		return
	}
	if strings.TrimSpace(req.UUID) == "" {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "规则 UUID 不能为空"})
		return
	}
	if req.TargetPosition == nil && req.Priority == nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "targetPosition 或 priority 必须提供"})
		return
	}
	if req.Priority != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "FW_SCOPE_UNSUPPORTED", "message": "当前防火墙后端不支持 priority 重排"})
		return
	}
	if *req.TargetPosition < 1 || *req.TargetPosition > 10000 {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "targetPosition 必须在 1 到 10000 之间"})
		return
	}

	var found map[string]any
	var providerCommand string
	var family string
	for _, candidateFamily := range []string{"ipv4", "ipv6"} {
		inventory, err := firewallInventory(r.Context(), map[string]any{"provider": "iptables", "family": candidateFamily})
		if err != nil {
			continue
		}
		for _, raw := range inventory["items"].([]map[string]any) {
			rule, _ := raw["rule"].(map[string]any)
			if firewallString(rule, "uuid") == req.UUID {
				found = rule
				family = candidateFamily
				providerCommand, _ = firewallProviderCommand(rule)
				break
			}
		}
		if found != nil {
			break
		}
	}
	if found == nil {
		wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": "规则不存在或不是可重排的 iptables 规则"})
		return
	}
	if providerCommand != "iptables" && providerCommand != "ip6tables" {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "FW_SCOPE_UNSUPPORTED", "message": "当前防火墙后端不支持规则重排"})
		return
	}
	deleteArgs, chain, err := firewallRuleArgs(found, "-D")
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if _, err := runFirewallCommand(r.Context(), providerCommand, deleteArgs...); err != nil {
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": "删除旧规则失败: " + err.Error()})
		return
	}
	insertArgs, _, err := firewallRuleArgs(found, "-A")
	if err != nil {
		// The old rule was already removed; best effort restoration at the end.
		_, _, _ = firewallRuleArgs(found, "-A")
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	insertArgs[0] = "-I"
	insertArgs = append(insertArgs[:1], append([]string{chain, fmt.Sprint(*req.TargetPosition)}, insertArgs[2:]...)...)
	if _, err := runFirewallCommand(r.Context(), providerCommand, insertArgs...); err != nil {
		// Restore at the chain tail when insertion fails; retain the original error.
		if restoreArgs, _, restoreErr := firewallRuleArgs(found, "-A"); restoreErr == nil {
			_, _ = runFirewallCommand(r.Context(), providerCommand, restoreArgs...)
		}
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": "插入新规则失败，已尝试恢复旧规则: " + err.Error()})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"uuid": req.UUID, "targetPosition": *req.TargetPosition, "family": family, "reordered": true}})
}

var firewallChainPattern = regexp.MustCompile(`^WORKMESH_(?:BASIC(?:_(?:BEFORE|AFTER))?|DOCKER)$`)
var firewallPortPattern = regexp.MustCompile(`^[0-9]{1,5}(?:[:-][0-9]{1,5})?(?:,[0-9]{1,5}(?:[:-][0-9]{1,5})?)*$`)

func firewallProviderCommand(rule map[string]any) (string, error) {
	scope, _ := rule["scope"].(map[string]any)
	provider, _ := scope["provider"].(string)
	if provider != "iptables" && provider != "nftables" && provider != "firewalld" && provider != "ufw" {
		return "", fmt.Errorf("防火墙后端 %q 暂不支持写入", provider)
	}
	if provider == "nftables" {
		return "nft", nil
	}
	if provider == "firewalld" {
		return "firewall-cmd", nil
	}
	if provider == "ufw" {
		return "ufw", nil
	}
	family, _ := scope["family"].(string)
	if family == "ipv6" {
		return "ip6tables", nil
	}
	return "iptables", nil
}

func firewallString(rule map[string]any, key string) string {
	value, _ := rule[key].(string)
	if value == "" {
		if raw, ok := rule[key]; ok {
			value = fmt.Sprint(raw)
		}
	}
	return strings.TrimSpace(value)
}

func validateFirewallRule(rule map[string]any) error {
	scope, _ := rule["scope"].(map[string]any)
	provider, _ := scope["provider"].(string)
	chain, _ := scope["chain"].(string)
	if provider == "nftables" {
		if !regexp.MustCompile(`^[A-Za-z0-9_.-]{1,32}$`).MatchString(strings.TrimSpace(chain)) {
			return fmt.Errorf("nftables chain 无效: %s", chain)
		}
	} else if provider == "firewalld" {
		if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`).MatchString(strings.TrimSpace(chain)) {
			return fmt.Errorf("firewalld zone 无效: %s", chain)
		}
	} else if provider == "ufw" {
		if strings.TrimSpace(chain) != "" && strings.TrimSpace(chain) != "INPUT" {
			return fmt.Errorf("ufw 链无效: %s", chain)
		}
	} else if !firewallChainPattern.MatchString(strings.TrimSpace(chain)) {
		return fmt.Errorf("防火墙链无效: %s", chain)
	}
	protocol := strings.ToLower(firewallString(rule, "protocol"))
	if protocol == "" {
		protocol = "all"
	}
	allowedProtocol := map[string]bool{"all": true, "tcp": true, "udp": true, "icmp": true, "icmpv6": true}
	if !allowedProtocol[protocol] {
		return fmt.Errorf("防火墙协议无效: %s", protocol)
	}
	action := strings.ToLower(firewallString(rule, "action"))
	if action != "accept" && action != "drop" && action != "reject" {
		return fmt.Errorf("防火墙动作无效: %s", action)
	}
	for _, key := range []string{"sourceAddress", "destinationAddress"} {
		address := firewallString(rule, key)
		if address == "" || address == "0.0.0.0/0" || address == "::/0" {
			continue
		}
		if _, _, err := net.ParseCIDR(address); err != nil {
			if net.ParseIP(address) == nil {
				return fmt.Errorf("防火墙地址无效: %s", address)
			}
		}
	}
	if port := firewallString(rule, "destinationPort"); port != "" && port != "*" && !firewallPortPattern.MatchString(port) {
		return fmt.Errorf("防火墙端口无效: %s", port)
	}
	return nil
}

func firewallRuleArgs(rule map[string]any, operation string) ([]string, string, error) {
	if err := validateFirewallRule(rule); err != nil {
		return nil, "", err
	}
	scope, _ := rule["scope"].(map[string]any)
	provider, _ := scope["provider"].(string)
	chain, _ := scope["chain"].(string)
	protocol := strings.ToLower(firewallString(rule, "protocol"))
	if protocol == "" {
		protocol = "all"
	}
	action := strings.ToUpper(firewallString(rule, "action"))
	if provider == "nftables" {
		table, _ := scope["table"].(string)
		if table == "" {
			table = "workmesh"
		}
		family, _ := scope["family"].(string)
		nftFamily := "inet"
		if family == "ipv4" {
			nftFamily = "ip"
		} else if family == "ipv6" {
			nftFamily = "ip6"
		}
		if operation == "-D" {
			if handle := firewallString(rule, "nativeHandle"); handle != "" {
				return []string{"delete", "rule", nftFamily, table, chain, "handle", handle}, chain, nil
			}
		}
		nftOp := "add"
		if operation == "-D" {
			nftOp = "delete"
		}
		args := []string{nftOp, "rule", nftFamily, table, chain}
		addressFamily := "ip"
		if nftFamily == "ip6" || strings.Contains(firewallString(rule, "sourceAddress"), ":") || strings.Contains(firewallString(rule, "destinationAddress"), ":") || protocol == "icmpv6" {
			addressFamily = "ip6"
		}
		if value := firewallString(rule, "sourceAddress"); value != "" && value != "0.0.0.0/0" && value != "::/0" {
			args = append(args, addressFamily, "saddr", value)
		}
		if value := firewallString(rule, "destinationAddress"); value != "" && value != "0.0.0.0/0" && value != "::/0" {
			args = append(args, addressFamily, "daddr", value)
		}
		if protocol != "all" {
			args = append(args, protocol)
		}
		if port := firewallString(rule, "destinationPort"); port != "" && port != "*" {
			args = append(args, "dport", port)
		}
		args = append(args, strings.ToLower(action))
		return args, chain, nil
	}
	if provider == "firewalld" {
		zone := chain
		if zone == "" {
			zone = "public"
		}
		family, _ := scope["family"].(string)
		if family == "" || family == "inet" {
			family = "ipv4"
		}
		rich := `rule family="` + family + `"`
		if address := firewallString(rule, "sourceAddress"); address != "" && address != "0.0.0.0/0" && address != "::/0" {
			rich += ` source address="` + address + `"`
		}
		if protocol == "all" && firewallString(rule, "destinationPort") != "" && firewallString(rule, "destinationPort") != "*" {
			return nil, "", fmt.Errorf("firewalld 目标端口规则必须指定 tcp 或 udp")
		}
		if protocol != "all" {
			rich += ` port port="` + firewallString(rule, "destinationPort") + `" protocol="` + protocol + `"`
		}
		rich += ` ` + strings.ToLower(action)
		flag := "--add-rich-rule"
		if operation == "-D" {
			flag = "--remove-rich-rule"
			if raw := firewallString(rule, "raw"); raw != "" {
				rich = raw
			}
		}
		return []string{"--zone=" + zone, flag, rich}, zone, nil
	}
	if provider == "ufw" {
		if operation == "-D" {
			if nativeID := firewallString(rule, "nativeId"); nativeID != "" && regexp.MustCompile(`^[0-9]+$`).MatchString(nativeID) {
				return []string{"delete", nativeID}, "INPUT", nil
			}
		}
		verb := "allow"
		if action == "DROP" {
			verb = "deny"
		} else if action == "REJECT" {
			verb = "reject"
		}
		args := []string{verb}
		if address := firewallString(rule, "sourceAddress"); address != "" && address != "0.0.0.0/0" && address != "::/0" {
			args = append(args, "from", address)
		}
		if port := firewallString(rule, "destinationPort"); port != "" && port != "*" {
			args = append(args, "to", "any", "port", port)
		}
		if protocol != "all" {
			args = append(args, "proto", protocol)
		}
		if operation == "-D" {
			args = append([]string{"delete"}, args...)
		}
		return args, "INPUT", nil
	}
	args := []string{operation, chain}
	if protocol != "all" {
		args = append(args, "-p", protocol)
	}
	for _, pair := range [][2]string{{"sourceAddress", "-s"}, {"destinationAddress", "-d"}} {
		value := firewallString(rule, pair[0])
		if value != "" && value != "0.0.0.0/0" && value != "::/0" {
			args = append(args, pair[1], value)
		}
	}
	if port := firewallString(rule, "destinationPort"); port != "" && port != "*" {
		if protocol != "tcp" && protocol != "udp" {
			return nil, "", fmt.Errorf("仅 tcp/udp 规则支持目标端口")
		}
		args = append(args, "--dport", port)
	}
	args = append(args, "-j", action)
	return args, chain, nil
}

func firewallRuleUUID(line string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(line)))
	return hex.EncodeToString(digest[:16])
}

func runFirewallCommand(ctx context.Context, command string, args ...string) ([]byte, error) {
	path, err := exec.LookPath(command)
	if err != nil {
		return nil, fmt.Errorf("%s 不可用: %w", command, err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return output, fmt.Errorf("%s: %s", command, message)
	}
	return output, nil
}

// requireFirewallMutation prevents accidental host firewall changes during
// normal API use. Production or isolated acceptance environments must opt in
// explicitly before any handler executes a mutating command.
func requireFirewallMutation(w http.ResponseWriter) bool {
	if os.Getenv("WORKMESH_ALLOW_FIREWALL_MUTATION") == "1" {
		return true
	}
	wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{
		"code":    "ERR",
		"message": "防火墙写操作需要 WORKMESH_ALLOW_FIREWALL_MUTATION=1",
	})
	return false
}

func firewallBackendName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return value
}

func firewallManagedChains() []string {
	return []string{"WORKMESH_BASIC_BEFORE", "WORKMESH_BASIC", "WORKMESH_BASIC_AFTER"}
}

func handleFirewallBackendOperate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subsystem string
		Backend   string
		Operation string
		Name      string
		Operate   string
	}
	if err := decodeJSON(r, &req); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	backend := firewallBackendName(req.Backend)
	operation := strings.ToLower(strings.TrimSpace(req.Operation))
	if operation == "" {
		operation = strings.ToLower(strings.TrimSpace(req.Operate))
	}
	if backend == "" {
		backend = strings.ToLower(strings.TrimSpace(req.Name))
	}
	if backend != "iptables" && backend != "nftables" && backend != "firewalld" && backend != "ufw" {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": "当前防火墙后端暂不支持托管链操作: " + backend})
		return
	}
	if operation == "select" {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"selected": backend, "subsystem": req.Subsystem}})
		return
	}
	if os.Getenv("WORKMESH_ALLOW_FIREWALL_MUTATION") != "1" {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": "防火墙后端写操作需要 WORKMESH_ALLOW_FIREWALL_MUTATION=1"})
		return
	}
	if backend == "firewalld" || backend == "ufw" {
		binary := backend
		if backend == "firewalld" {
			binary = "firewall-cmd"
		}
		args := []string{"--state"}
		if backend == "ufw" {
			args = []string{"status"}
			binary = "ufw"
		}
		if _, err := runFirewallCommand(r.Context(), binary, args...); err != nil {
			wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": "检查防火墙后端失败: " + err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": backend + " 没有可安全托管的独立链，初始化/绑定/清理需通过规则级接口执行"})
		return
	}
	if backend == "nftables" {
		if _, err := runFirewallCommand(r.Context(), "nft", "list", "tables"); err != nil {
			wmhttp.JSON(w, 502, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		switch operation {
		case "initialize", "init":
			if _, err := runFirewallCommand(r.Context(), "nft", "add", "table", "inet", "workmesh"); err != nil && !strings.Contains(strings.ToLower(err.Error()), "exists") {
				wmhttp.JSON(w, 502, map[string]any{"code": "ERR", "message": err.Error()})
				return
			}
			chainDef := `{ type filter hook input priority 0; policy accept; }`
			if _, err := runFirewallCommand(r.Context(), "nft", "add", "chain", "inet", "workmesh", "input", chainDef); err != nil && !strings.Contains(strings.ToLower(err.Error()), "exists") {
				wmhttp.JSON(w, 502, map[string]any{"code": "ERR", "message": err.Error()})
				return
			}
		case "cleanup", "clean":
			_, _ = runFirewallCommand(r.Context(), "nft", "delete", "table", "inet", "workmesh")
		default:
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "不支持的 nftables 后端操作: " + operation})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"backend": backend, "operation": operation, "initialized": operation != "cleanup" && operation != "clean"}})
		return
	}
	if _, err := exec.LookPath("iptables"); err != nil {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": "iptables 不可用: " + err.Error()})
		return
	}
	chains := firewallManagedChains()
	switch operation {
	case "initialize", "init":
		for _, chain := range chains {
			if _, err := runFirewallCommand(r.Context(), "iptables", "-N", chain); err != nil && !strings.Contains(strings.ToLower(err.Error()), "chain already exists") {
				wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": "创建托管链失败: " + err.Error()})
				return
			}
		}
		if _, err := runFirewallCommand(r.Context(), "iptables", "-C", "INPUT", "-j", chains[0]); err != nil {
			if _, insertErr := runFirewallCommand(r.Context(), "iptables", "-I", "INPUT", "1", "-j", chains[0]); insertErr != nil {
				wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": "绑定托管链失败: " + insertErr.Error()})
				return
			}
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"initialized": true, "bound": true, "backend": backend}})
	case "cleanup", "clean":
		for _, chain := range chains {
			_, _ = runFirewallCommand(r.Context(), "iptables", "-D", "INPUT", "-j", chain)
			if _, err := runFirewallCommand(r.Context(), "iptables", "-F", chain); err == nil {
				_, _ = runFirewallCommand(r.Context(), "iptables", "-X", chain)
			}
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"cleaned": true, "backend": backend}})
	default:
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "不支持的防火墙后端操作: " + operation})
	}
}

func handleFirewallOperate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Operation string
	}
	if err := decodeJSON(r, &req); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	operation := strings.ToLower(strings.TrimSpace(req.Operation))
	if operation != "enablebanping" && operation != "disablebanping" {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "不支持的防火墙操作: " + req.Operation})
		return
	}
	status := firewallStatus(r.Context())
	provider, _ := status["provider"].(string)
	if provider == "" {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": "没有可用的防火墙后端"})
		return
	}
	if provider != "iptables" {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": "当前后端不支持安全修改 ping 策略"})
		return
	}
	if !requireFirewallMutation(w) {
		return
	}
	action := "-D"
	if operation == "enablebanping" {
		action = "-A"
	}
	_, err := runFirewallCommand(r.Context(), "iptables", action, "INPUT", "-p", "icmp", "--icmp-type", "echo-request", "-j", "DROP")
	if err != nil && operation == "disablebanping" && strings.Contains(strings.ToLower(err.Error()), "matching rule") {
		err = nil
	}
	if err != nil {
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": "更新 ping 策略失败: " + err.Error()})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"operation": req.Operation, "updated": true}})
}
