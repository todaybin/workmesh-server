// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var firewallSyncTaskState struct {
	sync.Mutex
	id string
}

type firewallSyncRequest struct {
	Subsystem      string `json:"subsystem"`
	SourceProvider string `json:"sourceProvider"`
	TargetProvider string `json:"targetProvider"`
	ResetSource    bool   `json:"resetSource"`
	TaskID         string `json:"taskID"`
}

type firewallSyncPlan struct {
	Request firewallSyncRequest
	Items   []map[string]any
}

type firewallSyncError struct {
	status  int
	message string
}

func (e *firewallSyncError) Error() string { return e.message }

func decodeFirewallSyncRequest(r *http.Request) (firewallSyncRequest, error) {
	var request firewallSyncRequest
	if err := decodeJSON(r, &request); err != nil {
		return request, err
	}
	request.Subsystem = strings.ToLower(strings.TrimSpace(request.Subsystem))
	if request.Subsystem == "" {
		request.Subsystem = "system"
	}
	request.SourceProvider = firewallBackendName(request.SourceProvider)
	request.TargetProvider = firewallBackendName(request.TargetProvider)
	if request.TargetProvider == "" {
		request.TargetProvider = "iptables"
	}
	return request, nil
}

func validateFirewallSyncRequest(request firewallSyncRequest) error {
	if request.Subsystem != "system" && request.Subsystem != "docker" {
		return &firewallSyncError{status: http.StatusServiceUnavailable, message: "当前仅支持 system 与 docker 防火墙规则同步"}
	}
	if request.TargetProvider != "iptables" && request.TargetProvider != "nftables" {
		return &firewallSyncError{status: http.StatusServiceUnavailable, message: "当前仅支持 iptables 与 nftables 之间同步"}
	}
	if request.SourceProvider != "" && request.SourceProvider != "iptables" && request.SourceProvider != "nftables" {
		return &firewallSyncError{status: http.StatusServiceUnavailable, message: "源防火墙后端暂不支持同步"}
	}
	if request.SourceProvider != "" && request.SourceProvider == request.TargetProvider {
		return &firewallSyncError{status: http.StatusBadRequest, message: "源和目标防火墙后端不能相同"}
	}
	if request.Subsystem == "docker" && request.SourceProvider != "" && request.SourceProvider != "database" && request.SourceProvider != "sqlite" {
		return &firewallSyncError{status: http.StatusBadRequest, message: "docker 防护策略源必须是 database"}
	}
	if request.Subsystem == "docker" && request.TargetProvider != "iptables" {
		return &firewallSyncError{status: http.StatusServiceUnavailable, message: "Docker 防护同步当前仅支持 iptables 目标链"}
	}
	return nil
}

func firewallSyncManagedRule(rule map[string]any) bool {
	scope, _ := rule["scope"].(map[string]any)
	provider := firewallBackendName(firewallString(scope, "provider"))
	chain := strings.TrimSpace(firewallString(scope, "chain"))
	if provider == "iptables" {
		for _, managed := range firewallManagedChains() {
			if chain == managed {
				return true
			}
		}
		return false
	}
	return provider == "nftables" && strings.TrimSpace(firewallString(scope, "table")) == "workmesh" && chain == "input"
}

func firewallSyncInventory(ctx context.Context, provider string) ([]map[string]any, error) {
	items := make([]map[string]any, 0)
	if provider == "iptables" {
		for _, family := range []string{"ipv4", "ipv6"} {
			inventory, err := firewallInventory(ctx, map[string]any{"provider": provider, "family": family})
			if err != nil {
				return nil, err
			}
			for _, raw := range inventory["items"].([]map[string]any) {
				rule, _ := raw["rule"].(map[string]any)
				if firewallSyncManagedRule(rule) {
					items = append(items, rule)
				}
			}
		}
		return items, nil
	}
	inventory, err := firewallInventory(ctx, map[string]any{"provider": provider, "family": "inet", "table": "workmesh", "chain": "input"})
	if err != nil {
		return nil, err
	}
	for _, raw := range inventory["items"].([]map[string]any) {
		rule, _ := raw["rule"].(map[string]any)
		if firewallSyncManagedRule(rule) {
			items = append(items, rule)
		}
	}
	return items, nil
}

func firewallSyncTargetRule(source map[string]any, target string) (map[string]any, error) {
	sourceScope, _ := source["scope"].(map[string]any)
	sourceProvider := firewallBackendName(firewallString(sourceScope, "provider"))
	if target == "iptables" && sourceProvider == "nftables" && firewallString(source, "nativeKind") == "opaque" {
		return nil, &firewallSyncError{status: http.StatusBadRequest, message: "nftables opaque 规则不能安全转换为 iptables"}
	}
	rule := make(map[string]any, len(source)+1)
	for key, value := range source {
		if key == "uuid" || key == "description" || key == "nativeHandle" || key == "raw" {
			continue
		}
		rule[key] = value
	}
	scope := map[string]any{"provider": target, "direction": "input"}
	if target == "nftables" {
		scope["family"], scope["table"], scope["chain"] = "inet", "workmesh", "input"
	} else {
		family := firewallString(sourceScope, "family")
		if strings.Contains(firewallString(source, "sourceAddress"), ":") || strings.Contains(firewallString(source, "destinationAddress"), ":") {
			family = "ipv6"
		} else if family == "" || family == "inet" {
			family = "ipv4"
		}
		scope["family"], scope["table"], scope["chain"] = family, "filter", "WORKMESH_BASIC"
	}
	rule["scope"] = scope
	return rule, nil
}

func buildFirewallSyncPlan(ctx context.Context, request firewallSyncRequest) (firewallSyncPlan, error) {
	if err := validateFirewallSyncRequest(request); err != nil {
		return firewallSyncPlan{}, err
	}
	if request.Subsystem == "docker" {
		return buildDockerFirewallSyncPlan(ctx, request)
	}
	if request.SourceProvider == "" {
		status := firewallStatus(ctx)
		request.SourceProvider, _ = status["provider"].(string)
		request.SourceProvider = firewallBackendName(request.SourceProvider)
	}
	if request.SourceProvider != "iptables" && request.SourceProvider != "nftables" {
		return firewallSyncPlan{}, &firewallSyncError{status: http.StatusServiceUnavailable, message: "没有可同步的 iptables/nftables 源后端"}
	}
	source, err := firewallSyncInventory(ctx, request.SourceProvider)
	if err != nil {
		return firewallSyncPlan{}, err
	}
	target := make([]map[string]any, 0)
	if request.TargetProvider == "iptables" {
		for _, family := range []string{"ipv4", "ipv6"} {
			inventory, inventoryErr := firewallInventory(ctx, map[string]any{"provider": "iptables", "family": family, "chain": "WORKMESH_DOCKER"})
			if inventoryErr != nil {
				return firewallSyncPlan{}, inventoryErr
			}
			for _, raw := range inventory["items"].([]map[string]any) {
				if rule, ok := raw["rule"].(map[string]any); ok {
					target = append(target, rule)
				}
			}
		}
	} else {
		inventory, inventoryErr := firewallInventory(ctx, map[string]any{"provider": "nftables", "family": "inet", "table": "workmesh", "chain": "docker"})
		if inventoryErr != nil {
			return firewallSyncPlan{}, inventoryErr
		}
		for _, raw := range inventory["items"].([]map[string]any) {
			if rule, ok := raw["rule"].(map[string]any); ok {
				target = append(target, rule)
			}
		}
	}
	targetKeys := make(map[string]bool, len(target))
	for _, rule := range target {
		targetKeys[firewallRuleKey(rule)] = true
	}
	items := make([]map[string]any, 0, len(source))
	for _, sourceRule := range source {
		sourceUUID := firewallString(sourceRule, "uuid")
		targetRule, convertErr := firewallSyncTargetRule(sourceRule, request.TargetProvider)
		item := map[string]any{"sourceUUID": sourceUUID, "rule": sourceRule}
		if convertErr != nil {
			item["status"], item["reasonCode"], item["reason"] = "blocked", "opaque_rule", convertErr.Error()
		} else if targetKeys[firewallRuleKey(targetRule)] {
			item["status"], item["reasonCode"], item["reason"] = "existing", "target_matches", "目标后端已存在相同托管规则"
		} else {
			item["status"], item["reasonCode"], item["reason"], item["targetRule"] = "ready", "adapter_supported", "规则可同步", targetRule
		}
		items = append(items, item)
	}
	return firewallSyncPlan{Request: request, Items: items}, nil
}

func buildDockerFirewallSyncPlan(ctx context.Context, request firewallSyncRequest) (firewallSyncPlan, error) {
	if request.SourceProvider == "" {
		request.SourceProvider = "database"
	}
	target := make([]map[string]any, 0)
	for _, family := range []string{"ipv4", "ipv6"} {
		inventory, inventoryErr := firewallInventory(ctx, map[string]any{"provider": "iptables", "family": family, "chain": "WORKMESH_DOCKER"})
		if inventoryErr != nil {
			return firewallSyncPlan{}, inventoryErr
		}
		for _, raw := range inventory["items"].([]map[string]any) {
			if rule, ok := raw["rule"].(map[string]any); ok {
				target = append(target, rule)
			}
		}
	}
	targetKeys := make(map[string]bool, len(target))
	for _, rule := range target {
		scope, _ := rule["scope"].(map[string]any)
		if strings.TrimSpace(firewallString(scope, "chain")) == "WORKMESH_DOCKER" {
			targetKeys[firewallRuleKey(rule)] = true
		}
	}
	dockerGuardMu.Lock()
	state := dockerGuardState()
	dockerGuardMu.Unlock()
	policies := make([]dockerGuardPolicy, 0, len(state))
	for _, policy := range state {
		policies = append(policies, policy)
	}
	sort.Slice(policies, func(i, j int) bool {
		return dockerGuardKey(policies[i].Family, policies[i].HostIP, policies[i].HostPort, policies[i].Protocol) < dockerGuardKey(policies[j].Family, policies[j].HostIP, policies[j].HostPort, policies[j].Protocol)
	})
	items := make([]map[string]any, 0, len(policies))
	for _, policy := range policies {
		base := map[string]any{"family": policy.Family, "hostIP": policy.HostIP, "hostPort": policy.HostPort, "protocol": policy.Protocol, "mode": policy.Mode, "sources": policy.Sources, "description": policy.Description, "policyUUID": policy.PolicyUUID}
		item := map[string]any{"sourceUUID": policy.PolicyUUID, "dockerRule": base}
		targetRules, targetErr := dockerGuardTargetRules(policy, request.TargetProvider)
		if targetErr != nil {
			item["status"], item["reasonCode"], item["reason"] = "blocked", "adapter_unsupported", targetErr.Error()
		} else if dockerTargetRulesExist(targetRules, targetKeys) {
			item["status"], item["reasonCode"], item["reason"] = "existing", "target_matches", "目标 Docker 防护链已存在相同规则"
		} else {
			item["status"], item["reasonCode"], item["reason"], item["targetRule"], item["targetRules"] = "ready", "adapter_supported", "Docker 防护策略可同步", targetRules[0], targetRules
		}
		items = append(items, item)
	}
	return firewallSyncPlan{Request: request, Items: items}, nil
}

func dockerTargetRulesExist(rules []map[string]any, targetKeys map[string]bool) bool {
	if len(rules) == 0 {
		return false
	}
	for _, rule := range rules {
		if !targetKeys[firewallRuleKey(rule)] {
			return false
		}
	}
	return true
}

func dockerGuardTargetRules(policy dockerGuardPolicy, target string) ([]map[string]any, error) {
	if target != "iptables" {
		return nil, &firewallSyncError{status: http.StatusServiceUnavailable, message: "Docker 防护同步当前仅支持 iptables 目标链"}
	}
	if policy.Mode != "deny_all" && policy.Mode != "deny_sources" && policy.Mode != "allow_sources" {
		return nil, &firewallSyncError{status: http.StatusBadRequest, message: "Docker 防护模式暂不支持安全转换: " + policy.Mode}
	}
	makeRule := func(action, source string) map[string]any {
		rule := map[string]any{
			"scope":    map[string]any{"provider": target, "family": policy.Family, "table": "filter", "chain": "WORKMESH_DOCKER", "direction": "input"},
			"protocol": policy.Protocol, "destinationPort": policy.HostPort, "action": action, "description": policy.Description,
		}
		if policy.HostIP != "0.0.0.0" && policy.HostIP != "::" && policy.HostIP != "0.0.0.0/0" && policy.HostIP != "::/0" {
			rule["destinationAddress"] = policy.HostIP
		}
		if source != "" {
			rule["sourceAddress"] = source
		}
		return rule
	}
	rules := make([]map[string]any, 0, len(policy.Sources)+1)
	switch policy.Mode {
	case "deny_all":
		rules = append(rules, makeRule("drop", ""))
	case "deny_sources":
		if len(policy.Sources) == 0 {
			return nil, &firewallSyncError{status: http.StatusBadRequest, message: "deny_sources 必须至少指定一个来源"}
		}
		for _, source := range policy.Sources {
			rules = append(rules, makeRule("drop", source))
		}
	case "allow_sources":
		if len(policy.Sources) == 0 {
			return nil, &firewallSyncError{status: http.StatusBadRequest, message: "allow_sources 必须至少指定一个来源"}
		}
		for _, source := range policy.Sources {
			rules = append(rules, makeRule("accept", source))
		}
		// Keep the catch-all drop after all source-specific accepts.
		rules = append(rules, makeRule("drop", ""))
	}
	return rules, nil
}

func dockerGuardTargetRule(policy dockerGuardPolicy, target, source string) (map[string]any, error) {
	rules, err := dockerGuardTargetRules(policy, target)
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		if source == "" || firewallString(rule, "sourceAddress") == source {
			return rule, nil
		}
	}
	return nil, &firewallSyncError{status: http.StatusBadRequest, message: "Docker 防护来源不存在"}
}

func firewallSyncCounts(items []map[string]any) (total, ready, existing, removed, blocked int) {
	total = len(items)
	for _, item := range items {
		switch item["status"] {
		case "ready":
			ready++
		case "existing":
			existing++
		case "remove":
			removed++
		case "blocked":
			blocked++
		}
	}
	return
}

func handleFirewallSyncPreview(w http.ResponseWriter, r *http.Request) {
	request, err := decodeFirewallSyncRequest(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	plan, err := buildFirewallSyncPlan(r.Context(), request)
	if err != nil {
		status := http.StatusServiceUnavailable
		if syncErr, ok := err.(*firewallSyncError); ok {
			status = syncErr.status
		}
		wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	total, ready, existing, removed, blocked := firewallSyncCounts(plan.Items)
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"subsystem": plan.Request.Subsystem, "sourceProvider": plan.Request.SourceProvider, "targetProvider": plan.Request.TargetProvider, "total": total, "ready": ready, "existing": existing, "removed": removed, "blocked": blocked, "items": plan.Items}})
}

func handleFirewallSync(w http.ResponseWriter, r *http.Request) {
	request, err := decodeFirewallSyncRequest(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if !requireFirewallMutation(w) {
		return
	}
	if request.TaskID != "" {
		firewallSyncTaskState.Lock()
		if firewallSyncTaskState.id != "" {
			current := firewallSyncTaskState.id
			firewallSyncTaskState.Unlock()
			wmhttp.JSON(w, http.StatusConflict, map[string]any{"code": "ERR", "message": "已有防火墙同步任务正在执行", "data": map[string]any{"taskID": current, "executing": true}})
			return
		}
		firewallSyncTaskState.id = request.TaskID
		firewallSyncTaskState.Unlock()
		defer func() {
			firewallSyncTaskState.Lock()
			if firewallSyncTaskState.id == request.TaskID {
				firewallSyncTaskState.id = ""
			}
			firewallSyncTaskState.Unlock()
		}()
	}
	plan, err := buildFirewallSyncPlan(r.Context(), request)
	if err != nil {
		status := http.StatusServiceUnavailable
		if syncErr, ok := err.(*firewallSyncError); ok {
			status = syncErr.status
		}
		wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if plan.Request.Subsystem == "docker" {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": executeDockerFirewallSync(r.Context(), plan)})
		return
	}
	total, _, existing, _, blocked := firewallSyncCounts(plan.Items)
	result := map[string]any{"subsystem": plan.Request.Subsystem, "sourceProvider": plan.Request.SourceProvider, "targetProvider": plan.Request.TargetProvider, "total": total, "succeeded": 0, "skipped": existing, "removed": 0, "failed": blocked, "errors": []any{}, "taskID": plan.Request.TaskID, "queued": false}
	for index, item := range plan.Items {
		if item["status"] != "ready" {
			if item["status"] == "blocked" {
				result["errors"] = append(result["errors"].([]any), map[string]any{"index": index, "sourceUUID": item["sourceUUID"], "error": item["reason"]})
			}
			continue
		}
		targetRule, _ := item["targetRule"].(map[string]any)
		args, _, argsErr := firewallRuleArgs(targetRule, "-A")
		command, commandErr := firewallProviderCommand(targetRule)
		if argsErr == nil {
			argsErr = commandErr
		}
		if argsErr == nil {
			_, argsErr = runFirewallCommand(r.Context(), command, args...)
		}
		if argsErr != nil {
			result["failed"] = result["failed"].(int) + 1
			result["errors"] = append(result["errors"].([]any), map[string]any{"index": index, "sourceUUID": item["sourceUUID"], "error": argsErr.Error()})
			continue
		}
		result["succeeded"] = result["succeeded"].(int) + 1
		if plan.Request.ResetSource {
			sourceRule, _ := item["rule"].(map[string]any)
			deleteArgs, _, deleteErr := firewallRuleArgs(sourceRule, "-D")
			deleteCommand, deleteCommandErr := firewallProviderCommand(sourceRule)
			if deleteErr == nil {
				deleteErr = deleteCommandErr
			}
			if deleteErr == nil {
				_, deleteErr = runFirewallCommand(r.Context(), deleteCommand, deleteArgs...)
			}
			if deleteErr != nil {
				rollbackArgs, _, rollbackArgsErr := firewallRuleArgs(targetRule, "-D")
				if rollbackArgsErr == nil {
					_, _ = runFirewallCommand(r.Context(), command, rollbackArgs...)
				}
				result["succeeded"] = result["succeeded"].(int) - 1
				result["failed"] = result["failed"].(int) + 1
				result["errors"] = append(result["errors"].([]any), map[string]any{"index": index, "sourceUUID": item["sourceUUID"], "error": "删除源规则失败，已回滚目标规则: " + deleteErr.Error()})
			}
		}
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
}

func executeDockerFirewallSync(ctx context.Context, plan firewallSyncPlan) map[string]any {
	total, _, existing, _, blocked := firewallSyncCounts(plan.Items)
	result := map[string]any{"subsystem": "docker", "sourceProvider": "database", "targetProvider": plan.Request.TargetProvider, "total": total, "succeeded": 0, "skipped": existing, "removed": 0, "failed": blocked, "errors": []any{}, "taskID": plan.Request.TaskID, "queued": false}
	for index, item := range plan.Items {
		if item["status"] != "ready" {
			if item["status"] == "blocked" {
				result["errors"] = append(result["errors"].([]any), map[string]any{"index": index, "sourceUUID": item["sourceUUID"], "error": item["reason"]})
			}
			continue
		}
		rules, _ := item["targetRules"].([]map[string]any)
		if len(rules) == 0 {
			if targetRule, ok := item["targetRule"].(map[string]any); ok {
				rules = []map[string]any{targetRule}
			}
		}
		applied := make([]struct {
			command string
			rule    map[string]any
		}, 0, len(rules))
		var err error
		for _, targetRule := range rules {
			args, _, argsErr := firewallRuleArgs(targetRule, "-A")
			command, commandErr := firewallProviderCommand(targetRule)
			if argsErr == nil {
				argsErr = commandErr
			}
			if argsErr == nil {
				_, argsErr = runFirewallCommand(ctx, command, args...)
			}
			if argsErr != nil {
				err = argsErr
				break
			}
			applied = append(applied, struct {
				command string
				rule    map[string]any
			}{command: command, rule: targetRule})
		}
		if err != nil {
			for _, item := range applied {
				if rollbackArgs, _, rollbackErr := firewallRuleArgs(item.rule, "-D"); rollbackErr == nil {
					_, _ = runFirewallCommand(ctx, item.command, rollbackArgs...)
				}
			}
			result["failed"] = result["failed"].(int) + 1
			result["errors"] = append(result["errors"].([]any), map[string]any{"index": index, "sourceUUID": item["sourceUUID"], "error": err.Error()})
			continue
		}
		if plan.Request.ResetSource {
			if removeErr := removeDockerGuardPolicy(item["dockerRule"].(map[string]any)); removeErr != nil {
				for _, appliedRule := range applied {
					if rollbackArgs, _, rollbackArgsErr := firewallRuleArgs(appliedRule.rule, "-D"); rollbackArgsErr == nil {
						_, _ = runFirewallCommand(ctx, appliedRule.command, rollbackArgs...)
					}
				}
				result["failed"] = result["failed"].(int) + 1
				result["errors"] = append(result["errors"].([]any), map[string]any{"index": index, "sourceUUID": item["sourceUUID"], "error": "删除数据库策略失败，已回滚目标规则: " + removeErr.Error()})
				continue
			}
			result["removed"] = result["removed"].(int) + 1
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return result
}

func removeDockerGuardPolicy(raw map[string]any) error {
	family := firewallString(raw, "family")
	hostIP := firewallString(raw, "hostIP")
	protocol := firewallString(raw, "protocol")
	port := 0
	if value := firewallString(raw, "hostPort"); value != "" {
		if parsed, parseErr := strconv.Atoi(value); parseErr == nil {
			port = parsed
		}
	}
	key := dockerGuardKey(family, hostIP, port, protocol)
	dockerGuardMu.Lock()
	defer dockerGuardMu.Unlock()
	state := dockerGuardState()
	if _, exists := state[key]; !exists {
		return fmt.Errorf("Docker 防护策略不存在: %s", key)
	}
	delete(state, key)
	return saveDockerGuardState(state)
}

func handleFirewallSyncTask(w http.ResponseWriter, _ *http.Request) {
	firewallSyncTaskState.Lock()
	id := firewallSyncTaskState.id
	firewallSyncTaskState.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"taskID": id, "executing": id != ""}})
}
