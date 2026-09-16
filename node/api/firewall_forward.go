// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var forwardAddressPattern = regexp.MustCompile(`^[0-9A-Fa-f:.]+$`)
var forwardPortPattern = regexp.MustCompile(`^[0-9]{1,5}(?::[0-9]{1,5})?$`)

type forwardRule struct {
	ID          uint   `json:"id"`
	Chain       string `json:"chain"`
	Family      string `json:"family"`
	Address     string `json:"address"`
	Port        string `json:"port"`
	Protocol    string `json:"protocol"`
	Strategy    string `json:"strategy"`
	Num         string `json:"num"`
	TargetIP    string `json:"targetIP"`
	TargetPort  string `json:"targetPort"`
	Interface   string `json:"interface"`
	UsedStatus  string `json:"usedStatus"`
	Description string `json:"description"`
	IsDesired   bool   `json:"isDesired"`
	IsRuntime   bool   `json:"isRuntime"`
	SyncStatus  string `json:"syncStatus"`
	args        []string
}

func forwardCommand(family string) string {
	if family == "ipv6" {
		return "ip6tables"
	}
	return "iptables"
}

func parseForwardRules(r *http.Request) ([]forwardRule, error) {
	result := make([]forwardRule, 0)
	for _, family := range []string{"ipv4", "ipv6"} {
		command := forwardCommand(family)
		out, err := runFirewallCommand(r.Context(), command, "-t", "nat", "-S", "PREROUTING")
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				return nil, err
			}
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "-A PREROUTING") || !strings.Contains(line, "-j DNAT") {
				continue
			}
			fields := strings.Fields(line)
			item := forwardRule{Chain: "PREROUTING", Family: family, Strategy: "dnat", UsedStatus: "used", IsRuntime: true, SyncStatus: "converged", Description: line}
			for i := 2; i < len(fields); i++ {
				switch fields[i] {
				case "-p":
					if i+1 < len(fields) {
						item.Protocol = fields[i+1]
						i++
					}
				case "-i":
					if i+1 < len(fields) {
						item.Interface = fields[i+1]
						i++
					}
				case "--dport", "--destination-port":
					if i+1 < len(fields) {
						item.Port = fields[i+1]
						i++
					}
				case "-d":
					if i+1 < len(fields) {
						item.Address = fields[i+1]
						i++
					}
				case "--to-destination":
					if i+1 < len(fields) {
						target := strings.TrimSpace(fields[i+1])
						i++
						if strings.HasPrefix(target, "[") {
							if end := strings.LastIndex(target, "]:"); end > 0 {
								item.TargetIP, item.TargetPort = target[1:end], target[end+2:]
							}
						} else if host, port, splitErr := splitHostPortLoose(target); splitErr == nil {
							item.TargetIP, item.TargetPort = host, port
						}
					}
				}
			}
			digest := sha256.Sum256([]byte(family + ":" + line))
			item.ID = uint(binaryID(digest[:]))
			item.args = append([]string(nil), fields[1:]...)
			result = append(result, item)
		}
	}
	for i := range result {
		result[i].Num = strconv.Itoa(i + 1)
	}
	return result, nil
}

func binaryID(value []byte) uint {
	var result uint
	for _, b := range value[:4] {
		result = result<<8 | uint(b)
	}
	if result == 0 {
		result = 1
	}
	return result
}

func splitHostPortLoose(value string) (string, string, error) {
	idx := strings.LastIndex(value, ":")
	if idx <= 0 || idx == len(value)-1 {
		return "", "", fmt.Errorf("目标地址无效")
	}
	return value[:idx], value[idx+1:], nil
}

func validateForwardRule(rule map[string]any) (forwardRule, error) {
	item := forwardRule{Family: firewallString(rule, "family"), Protocol: strings.ToLower(firewallString(rule, "protocol")), Port: firewallString(rule, "port"), TargetIP: firewallString(rule, "targetIP"), TargetPort: firewallString(rule, "targetPort"), Interface: firewallString(rule, "interface")}
	if item.Family == "" {
		item.Family = "ipv4"
	}
	if item.Family != "ipv4" && item.Family != "ipv6" {
		return item, fmt.Errorf("地址族无效")
	}
	if item.Protocol != "tcp" && item.Protocol != "udp" && item.Protocol != "tcp/udp" {
		return item, fmt.Errorf("转发协议无效")
	}
	if !forwardPortPattern.MatchString(item.Port) || !forwardPortPattern.MatchString(item.TargetPort) || !validForwardPortRange(item.Port) || !validForwardPortRange(item.TargetPort) {
		return item, fmt.Errorf("端口无效")
	}
	if !forwardAddressPattern.MatchString(item.TargetIP) {
		return item, fmt.Errorf("目标地址无效")
	}
	if item.Interface != "" && (!regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,32}$`).MatchString(item.Interface)) {
		return item, fmt.Errorf("网卡名称无效")
	}
	return item, nil
}

func validForwardPortRange(value string) bool {
	for _, part := range strings.Split(value, ":") {
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return true
}

func forwardArgs(item forwardRule, operation string) []string {
	args := []string{"-t", "nat", operation, "PREROUTING"}
	if item.Interface != "" {
		args = append(args, "-i", item.Interface)
	}
	if item.Protocol == "tcp/udp" {
		args = append(args, "-p", "tcp")
	} else {
		args = append(args, "-p", item.Protocol)
	}
	args = append(args, "--dport", item.Port, "-j", "DNAT", "--to-destination", item.TargetIP+":"+item.TargetPort)
	return args
}

func handleForwardRuleSearch(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Page     int    `json:"page"`
		PageSize int    `json:"pageSize"`
		Info     string `json:"info"`
		Strategy string `json:"strategy"`
	}
	if err := decodeJSON(r, &request); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if request.Page < 1 {
		request.Page = 1
	}
	if request.PageSize < 1 || request.PageSize > 200 {
		request.PageSize = 20
	}
	items, err := parseForwardRules(r)
	if err != nil {
		wmhttp.JSON(w, 503, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	keyword := strings.ToLower(strings.TrimSpace(request.Info))
	filtered := items[:0]
	for _, item := range items {
		if request.Strategy != "" && item.Strategy != request.Strategy {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(item.Description+item.Protocol+item.Port+item.TargetIP), keyword) {
			continue
		}
		filtered = append(filtered, item)
	}
	start := (request.Page - 1) * request.PageSize
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + request.PageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	page := filtered[start:end]
	for i := range page {
		page[i].args = nil
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": page, "total": len(filtered), "page": request.Page, "pageSize": request.PageSize}})
}

func handleForwardRuleOperate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ForceDelete bool `json:"forceDelete"`
		Rules       []struct {
			Operation  string         `json:"operation"`
			Rule       map[string]any `json:"rule"`
			ID         uint           `json:"id"`
			Family     string         `json:"family"`
			Protocol   string         `json:"protocol"`
			Port       string         `json:"port"`
			TargetIP   string         `json:"targetIP"`
			TargetPort string         `json:"targetPort"`
			Interface  string         `json:"interface"`
		} `json:"rules"`
	}
	if err := decodeJSON(r, &request); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if strings.TrimSpace(os.Getenv("WORKMESH_ALLOW_FIREWALL_MUTATION")) != "1" {
		wmhttp.JSON(w, 503, map[string]any{"code": "ERR", "message": "转发规则写操作需要 WORKMESH_ALLOW_FIREWALL_MUTATION=1"})
		return
	}
	current, _ := parseForwardRules(r)
	byID := map[uint]forwardRule{}
	for _, item := range current {
		byID[item.ID] = item
	}
	changed := 0
	for _, operation := range request.Rules {
		item := forwardRule{Family: operation.Family, Protocol: operation.Protocol, Port: operation.Port, TargetIP: operation.TargetIP, TargetPort: operation.TargetPort, Interface: operation.Interface}
		if operation.Rule != nil {
			itemMap := operation.Rule
			var err error
			item, err = validateForwardRule(itemMap)
			if err != nil {
				wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
				return
			}
		} else {
			itemMap := map[string]any{"family": item.Family, "protocol": item.Protocol, "port": item.Port, "targetIP": item.TargetIP, "targetPort": item.TargetPort, "interface": item.Interface}
			var err error
			item, err = validateForwardRule(itemMap)
			if err != nil {
				wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
				return
			}
		}
		command := forwardCommand(item.Family)
		op := strings.ToLower(strings.TrimSpace(operation.Operation))
		if op == "remove" && operation.ID != 0 {
			if old, ok := byID[operation.ID]; ok {
				item = old
				command = forwardCommand(old.Family)
			}
		}
		if op != "add" && op != "remove" {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "转发操作无效"})
			return
		}
		if _, err := runFirewallCommand(r.Context(), command, forwardArgs(item, map[bool]string{true: "-D", false: "-A"}[op == "remove"])...); err != nil {
			wmhttp.JSON(w, 502, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		changed++
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"changed": changed}})
}
