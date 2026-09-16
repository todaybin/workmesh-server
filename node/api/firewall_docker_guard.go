// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/todaybin/workmesh-server/node/model"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

type dockerGuardPolicy struct {
	Family      string   `json:"family"`
	HostIP      string   `json:"hostIP"`
	HostPort    int      `json:"hostPort"`
	Protocol    string   `json:"protocol"`
	Mode        string   `json:"mode"`
	Sources     []string `json:"sources"`
	Description string   `json:"description"`
	PolicyUUID  string   `json:"policyUUID"`
}

var dockerGuardMu sync.Mutex

func dockerGuardState() map[string]dockerGuardPolicy {
	state := map[string]dockerGuardPolicy{}
	_ = loadJSONState("docker_port_guard_state", &state)
	return state
}

func saveDockerGuardState(state map[string]dockerGuardPolicy) error {
	return saveJSONState("docker_port_guard_state", state)
}

func dockerGuardKey(family, hostIP string, hostPort int, protocol string) string {
	return fmt.Sprintf("%s|%s|%d|%s", family, hostIP, hostPort, strings.ToLower(protocol))
}

func dockerGuardEndpointMap(r *http.Request) ([]map[string]any, error) {
	result, err := runDocker(r, "ps", "-aq", "--no-trunc")
	if err != nil || result.ExitCode != 0 {
		return nil, dockerCommandError(result, err, "查询 Docker 容器失败")
	}
	ids := strings.Fields(result.Stdout)
	endpoints := make([]map[string]any, 0)
	for _, id := range ids {
		inspect, inspectErr := runDocker(r, "inspect", id)
		if inspectErr != nil || inspect.ExitCode != 0 {
			return nil, dockerCommandError(inspect, inspectErr, "读取 Docker 端口映射失败")
		}
		var records []struct {
			ID     string `json:"Id"`
			Name   string `json:"Name"`
			Config struct {
				Labels map[string]string `json:"Labels"`
			} `json:"Config"`
			NetworkSettings struct {
				Ports map[string][]struct {
					HostIP   string `json:"HostIp"`
					HostPort string `json:"HostPort"`
				} `json:"Ports"`
			} `json:"NetworkSettings"`
		}
		if err := json.Unmarshal([]byte(inspect.Stdout), &records); err != nil {
			return nil, fmt.Errorf("解析 Docker inspect 失败: %w", err)
		}
		for _, record := range records {
			for mapping, bindings := range record.NetworkSettings.Ports {
				parts := strings.SplitN(mapping, "/", 2)
				if len(parts) != 2 {
					continue
				}
				protocol := strings.ToLower(parts[1])
				containerPort := parts[0]
				for _, binding := range bindings {
					port := 0
					_, _ = fmt.Sscanf(binding.HostPort, "%d", &port)
					if port < 1 || port > 65535 {
						continue
					}
					hostIP := binding.HostIP
					if hostIP == "" {
						hostIP = "0.0.0.0"
					}
					family := "ipv4"
					if strings.Contains(hostIP, ":") {
						family = "ipv6"
					}
					key := dockerGuardKey(family, hostIP, port, protocol)
					endpoints = append(endpoints, map[string]any{"key": key, "family": family, "hostIP": hostIP, "hostPort": port, "protocol": protocol, "containerID": record.ID, "containerName": strings.TrimPrefix(record.Name, "/"), "containerPort": containerPort, "compose": record.Config.Labels["com.docker.compose.project"], "application": record.Config.Labels["com.docker.compose.service"]})
				}
			}
		}
	}
	return endpoints, nil
}

func dockerCommandError(result model.CommandResult, err error, prefix string) error {
	detail := strings.TrimSpace(result.Stderr)
	if detail == "" && err != nil {
		detail = err.Error()
	}
	if detail == "" {
		detail = "命令执行失败"
	}
	return fmt.Errorf("%s: %s", prefix, detail)
}

func handleDockerGuardPorts(w http.ResponseWriter, r *http.Request) {
	endpoints, err := dockerGuardEndpointMap(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	dockerGuardMu.Lock()
	policies := dockerGuardState()
	dockerGuardMu.Unlock()
	matchedPolicies := map[string]bool{}
	containers := map[string]map[string]any{}
	for _, endpoint := range endpoints {
		key := endpoint["containerID"].(string)
		container := containers[key]
		if container == nil {
			container = map[string]any{"key": key, "name": endpoint["containerName"], "compose": endpoint["compose"], "application": endpoint["application"], "endpoints": []map[string]any{}, "portGroups": []map[string]any{}}
			containers[key] = container
		}
		if policy, ok := policies[endpoint["key"].(string)]; ok {
			matchedPolicies[endpoint["key"].(string)] = true
			for field, value := range map[string]any{"policyUUID": policy.PolicyUUID, "mode": policy.Mode, "sources": policy.Sources, "description": policy.Description} {
				endpoint[field] = value
			}
			// Effective is determined after the live Docker chain binding is
			// inspected below; a persisted policy alone is not enforcement.
			endpoint["effective"] = false
		} else {
			endpoint["effective"] = false
		}
		container["endpoints"] = append(container["endpoints"].([]map[string]any), endpoint)
	}
	items := make([]map[string]any, 0, len(containers))
	for _, container := range containers {
		items = append(items, container)
	}
	orphans := make([]map[string]any, 0)
	for key, policy := range policies {
		if matchedPolicies[key] {
			continue
		}
		orphans = append(orphans, map[string]any{"family": policy.Family, "hostIP": policy.HostIP, "hostPort": policy.HostPort, "protocol": policy.Protocol, "policyUUID": policy.PolicyUUID, "mode": policy.Mode, "sources": policy.Sources, "description": policy.Description, "effective": false})
	}
	familyStatus := map[string]any{"state": "not_effective", "initialized": false, "bound": false, "effective": false}
	base := map[string]any{"name": "iptables", "version": "", "isExist": false, "initialized": false, "bound": false, "backend": "iptables", "ipv4": familyStatus, "ipv6": map[string]any{"state": "not_effective", "initialized": false, "bound": false, "effective": false}, "containers": items, "orphanPolicies": orphans}
	if _, err := runFirewallCommand(r.Context(), "iptables", "-L", "DOCKER", "-n"); err == nil {
		base["isExist"] = true
		if _, err := runFirewallCommand(r.Context(), "iptables", "-L", "WORKMESH_DOCKER", "-n"); err == nil {
			base["initialized"] = true
			familyStatus["initialized"] = true
			familyStatus["state"] = "disabled"
			if _, err := runFirewallCommand(r.Context(), "iptables", "-C", "DOCKER-USER", "-j", "WORKMESH_DOCKER"); err == nil {
				base["bound"] = true
			}
			if base["bound"] == true {
				familyStatus["bound"], familyStatus["effective"], familyStatus["state"] = true, true, "effective"
			}
		}
	}
	if bound, _ := base["bound"].(bool); bound {
		for _, container := range items {
			for _, endpoint := range container["endpoints"].([]map[string]any) {
				if endpoint["policyUUID"] != nil {
					endpoint["effective"] = true
				}
			}
		}
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"base": base, "containers": items, "orphanPolicies": orphans}})
}

func handleDockerGuardPolicyBatch(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Endpoints   []dockerGuardPolicy `json:"endpoints"`
		Mode        string              `json:"mode"`
		Sources     []string            `json:"sources"`
		Description string              `json:"description"`
	}
	if err := decodeJSON(r, &request); err != nil || len(request.Endpoints) == 0 {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "端口防护端点不能为空"})
		return
	}
	if request.Mode != "deny_sources" && request.Mode != "allow_sources" && request.Mode != "deny_all" {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "防护模式无效"})
		return
	}
	for _, source := range request.Sources {
		if err := validateDockerGuardAddress(request.Mode, "", source, ""); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
	}
	dockerGuardMu.Lock()
	defer dockerGuardMu.Unlock()
	state := dockerGuardState()
	for _, item := range request.Endpoints {
		if err := validateDockerGuardPolicy(item, request.Mode, request.Sources); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		item.Mode, item.Sources, item.Description = request.Mode, request.Sources, request.Description
		item.PolicyUUID = firewallRuleUUID(dockerGuardKey(item.Family, item.HostIP, item.HostPort, item.Protocol))
		state[dockerGuardKey(item.Family, item.HostIP, item.HostPort, item.Protocol)] = item
	}
	if err := saveDockerGuardState(state); err != nil {
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"updated": len(request.Endpoints)}})
}

func validateDockerGuardPolicy(item dockerGuardPolicy, mode string, sources []string) error {
	if item.Family != "ipv4" && item.Family != "ipv6" {
		return fmt.Errorf("端口防护地址族无效")
	}
	if item.HostPort < 1 || item.HostPort > 65535 {
		return fmt.Errorf("端口防护端口无效")
	}
	if item.Protocol != "tcp" && item.Protocol != "udp" {
		return fmt.Errorf("端口防护协议无效")
	}
	if err := validateDockerGuardAddress(mode, item.Family, item.HostIP, "host"); err != nil {
		return err
	}
	if mode != "deny_all" && len(sources) == 0 {
		return fmt.Errorf("该防护模式至少需要一个来源")
	}
	for _, source := range sources {
		if err := validateDockerGuardAddress(mode, item.Family, source, "source"); err != nil {
			return err
		}
	}
	return nil
}

func validateDockerGuardAddress(mode, family, value, label string) error {
	if label == "host" {
		if family == "ipv4" && (value == "0.0.0.0" || value == "0.0.0.0/0") {
			return nil
		}
		if family == "ipv6" && (value == "::" || value == "::/0") {
			return nil
		}
	}
	if value == "" {
		if mode == "deny_all" && label == "source" {
			return nil
		}
		return fmt.Errorf("%s 地址不能为空", label)
	}
	ip := net.ParseIP(value)
	if ip == nil {
		if parsed, _, err := net.ParseCIDR(value); err == nil {
			ip = parsed
		} else {
			return fmt.Errorf("%s 地址无效: %s", label, value)
		}
	}
	if family == "ipv4" && ip.To4() == nil {
		return fmt.Errorf("%s 必须是 IPv4 地址", label)
	}
	if family == "ipv6" && ip.To4() != nil {
		return fmt.Errorf("%s 必须是 IPv6 地址", label)
	}
	return nil
}

func handleDockerGuardPolicyDelete(w http.ResponseWriter, r *http.Request) {
	var request struct {
		UUIDs []string `json:"uuids"`
	}
	if err := decodeJSON(r, &request); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	dockerGuardMu.Lock()
	defer dockerGuardMu.Unlock()
	state := dockerGuardState()
	deleted := 0
	for key, policy := range state {
		for _, uuid := range request.UUIDs {
			if uuid == policy.PolicyUUID {
				delete(state, key)
				deleted++
			}
		}
	}
	if err := saveDockerGuardState(state); err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"deleted": deleted}})
}

func handleDockerGuardSync(w http.ResponseWriter, r *http.Request) {
	if _, err := dockerGuardEndpointMap(r); err != nil {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if !requireFirewallMutation(w) {
		return
	}
	plan, err := buildDockerFirewallSyncPlan(r.Context(), firewallSyncRequest{Subsystem: "docker", TargetProvider: "iptables"})
	if err != nil {
		status := http.StatusServiceUnavailable
		if syncErr, ok := err.(*firewallSyncError); ok {
			status = syncErr.status
		}
		wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	result := executeDockerFirewallSync(r.Context(), plan)
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
}

func handleDockerGuardOperate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Operation string `json:"operation"`
	}
	if err := decodeJSON(r, &request); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	operation := strings.ToLower(strings.TrimSpace(request.Operation))
	if operation != "initialize" && operation != "bind" && operation != "unbind" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "Docker 防护操作无效"})
		return
	}
	if strings.TrimSpace(os.Getenv("WORKMESH_ALLOW_FIREWALL_MUTATION")) != "1" {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": "防火墙写操作需要 WORKMESH_ALLOW_FIREWALL_MUTATION=1"})
		return
	}
	const chain = "WORKMESH_DOCKER"
	created := false
	if operation != "unbind" {
		if _, err := runFirewallCommand(r.Context(), "iptables", "-N", chain); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "exist") {
				wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": "创建 Docker 防护链失败: " + err.Error()})
				return
			}
		} else {
			created = true
		}
	}
	bound := false
	changed := created
	switch operation {
	case "initialize":
		// Initialization only creates the isolated chain. It does not change
		// Docker-USER ordering until the caller explicitly requests bind.
	case "bind":
		if _, err := runFirewallCommand(r.Context(), "iptables", "-C", "DOCKER-USER", "-j", chain); err == nil {
			bound = true
		} else if _, insertErr := runFirewallCommand(r.Context(), "iptables", "-I", "DOCKER-USER", "1", "-j", chain); insertErr != nil {
			if created {
				_, _ = runFirewallCommand(r.Context(), "iptables", "-F", chain)
				_, _ = runFirewallCommand(r.Context(), "iptables", "-X", chain)
			}
			wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": "绑定 Docker 防护链失败: " + insertErr.Error()})
			return
		} else {
			bound = true
			changed = true
		}
	case "unbind":
		if _, err := runFirewallCommand(r.Context(), "iptables", "-C", "DOCKER-USER", "-j", chain); err == nil {
			if _, deleteErr := runFirewallCommand(r.Context(), "iptables", "-D", "DOCKER-USER", "-j", chain); deleteErr != nil {
				wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": "解绑 Docker 防护链失败: " + deleteErr.Error()})
				return
			}
			changed = true
		}
	}
	initialized := false
	if _, err := runFirewallCommand(r.Context(), "iptables", "-L", chain, "-n"); err == nil {
		initialized = true
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"operation": operation, "initialized": initialized, "bound": bound, "changed": changed}})
}
