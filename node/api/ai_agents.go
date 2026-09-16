// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
)

// handleAgentRoute 处理 Agent 资源级操作，保证写入目标 Agent 而不是创建同名的伪记录。
// Agent 的凭据只写入本地受限状态文件，响应统一经过脱敏，避免令牌泄露到控制面。
func handleAgentRoute(w http.ResponseWriter, s *executionState, path string, body map[string]any) bool {
	if !strings.HasPrefix(path, "agents/") {
		return false
	}
	switch path {
	case "agents/agent/list", "agents/agent/channels":
		return handleAgentList(w, s, path, body)
	case "agents/remark", "agents/token/reset", "agents/website/bind", "agents/website/unbind":
		return handleAgentMetadata(w, s, path, body)
	case "agents/agent/create":
		return handleAgentRoleCreate(w, s, body)
	case "agents/agent/delete", "agents/agent/bind", "agents/agent/unbind":
		return handleAgentRoleChange(w, s, path, body)
	default:
		return false
	}
}

// handleAgentList 返回 Agent 角色列表或按频道聚合的绑定信息。
func handleAgentList(w http.ResponseWriter, s *executionState, path string, body map[string]any) bool {
	id := aiID(body, "agentId")
	if id == "" && path == "agents/agent/list" {
		aiOK(w, aiItems(s, path))
		return true
	}
	if id == "" && path == "agents/agent/channels" {
		aiOK(w, allAgentChannels(s))
		return true
	}
	if id == "" {
		aiError(w, http.StatusBadRequest, "AGENT_ID_REQUIRED", "agentId 不能为空")
		return true
	}
	s.mu.RLock()
	_, agent := accountByIDLocked(s.data.Agents, id)
	roles := make([]map[string]any, 0)
	if agent != nil {
		roles = mapSlice(agent["roles"])
	}
	s.mu.RUnlock()
	if agent == nil {
		aiError(w, http.StatusNotFound, "AGENT_NOT_FOUND", "Agent 不存在")
		return true
	}
	if path == "agents/agent/list" {
		aiOK(w, roles)
		return true
	}
	aiOK(w, agentRoleChannels(roles))
	return true
}

// allAgentChannels 聚合所有 Agent 的频道与账户绑定关系。
func allAgentChannels(s *executionState) []map[string]any {
	s.mu.RLock()
	agents := append([]map[string]any(nil), s.data.Agents...)
	s.mu.RUnlock()
	channels := make(map[string][]string)
	for _, item := range agents {
		for _, channel := range mapStringSlice(item["channels"]) {
			channels[channel] = append(channels[channel], aiID(item, "id"))
		}
		for _, role := range mapSlice(item["roles"]) {
			channel, accountID := aiString(role, "channel"), aiString(role, "accountId")
			if channel != "" && accountID != "" {
				channels[channel] = append(channels[channel], accountID)
			}
		}
	}
	return agentRoleChannelsFromMap(channels)
}

// agentRoleChannels 将角色列表转换为频道到账户 ID 的聚合结果。
func agentRoleChannels(roles []map[string]any) []map[string]any {
	channels := make(map[string][]string)
	for _, role := range roles {
		channel, accountID := aiString(role, "channel"), aiString(role, "accountId")
		if channel != "" && accountID != "" {
			channels[channel] = append(channels[channel], accountID)
		}
	}
	return agentRoleChannelsFromMap(channels)
}

// agentRoleChannelsFromMap 生成前端使用的频道聚合响应对象。
func agentRoleChannelsFromMap(channels map[string][]string) []map[string]any {
	items := make([]map[string]any, 0, len(channels))
	for channel, accountIDs := range channels {
		items = append(items, map[string]any{"name": channel, "bound": true, "accountIds": uniqueStrings(accountIDs)})
	}
	return items
}

// handleAgentMetadata 更新 Agent 备注、令牌或网站绑定状态。
func handleAgentMetadata(w http.ResponseWriter, s *executionState, path string, body map[string]any) bool {
	id := aiID(body, "id", "agentId")
	if id == "" {
		aiError(w, http.StatusBadRequest, "AGENT_ID_REQUIRED", "agentId 不能为空")
		return true
	}
	s.mu.Lock()
	index, agent := accountByIDLocked(s.data.Agents, id)
	if agent == nil {
		s.mu.Unlock()
		aiError(w, http.StatusNotFound, "AGENT_NOT_FOUND", "Agent 不存在")
		return true
	}
	agent = cloneMap(agent)
	if errCode, errMessage := applyAgentMetadata(path, body, agent); errCode != "" {
		s.mu.Unlock()
		aiError(w, http.StatusBadRequest, errCode, errMessage)
		return true
	}
	agent["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.data.Agents[index] = agent
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		aiError(w, http.StatusInternalServerError, "AGENT_SAVE_FAILED", err.Error())
		return true
	}
	aiOK(w, map[string]any{"updated": true, "agent": sanitizeAIMap(agent)})
	return true
}

// applyAgentMetadata 在持有写锁时应用 Agent 属性变更。
func applyAgentMetadata(path string, body, agent map[string]any) (string, string) {
	switch path {
	case "agents/remark":
		if remark, ok := body["remark"].(string); ok {
			agent["remark"] = strings.TrimSpace(remark)
		}
	case "agents/token/reset":
		token, err := newAgentToken()
		if err != nil {
			return "AGENT_TOKEN_FAILED", "生成 Agent 令牌失败"
		}
		agent["token"] = token
	case "agents/website/bind":
		websiteID := aiID(body, "websiteId", "websiteID")
		if websiteID == "" {
			return "WEBSITE_ID_REQUIRED", "websiteId 不能为空"
		}
		agent["websiteId"] = websiteID
	case "agents/website/unbind":
		delete(agent, "websiteId")
		delete(agent, "websiteID")
	}
	return "", ""
}

// handleAgentRoleCreate 为 Agent 创建新的角色绑定。
func handleAgentRoleCreate(w http.ResponseWriter, s *executionState, body map[string]any) bool {
	parentID := aiID(body, "agentId", "parentId")
	name := strings.TrimSpace(aiString(body, "name"))
	if parentID == "" || name == "" {
		aiError(w, http.StatusBadRequest, "AGENT_ROLE_REQUIRED", "agentId 和 name 不能为空")
		return true
	}
	s.mu.Lock()
	index, parent := accountByIDLocked(s.data.Agents, parentID)
	if parent == nil {
		s.mu.Unlock()
		aiError(w, http.StatusNotFound, "AGENT_NOT_FOUND", "Agent 不存在")
		return true
	}
	parent = cloneMap(parent)
	roles := mapSlice(parent["roles"])
	for _, role := range roles {
		if aiString(role, "name") == name {
			s.mu.Unlock()
			aiError(w, http.StatusConflict, "AGENT_ROLE_EXISTS", "Agent 角色已存在")
			return true
		}
	}
	role := cloneMap(body)
	role["id"] = aiNewID("role")
	role["name"] = name
	roles = append(roles, role)
	parent["roles"] = mapsToAny(roles)
	parent["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.data.Agents[index] = parent
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		aiError(w, http.StatusInternalServerError, "AGENT_SAVE_FAILED", err.Error())
		return true
	}
	aiOK(w, map[string]any{"output": sanitizeAIMap(role)})
	return true
}

// handleAgentRoleChange 删除角色或更新角色的频道账户绑定。
func handleAgentRoleChange(w http.ResponseWriter, s *executionState, path string, body map[string]any) bool {
	parentID, roleID := aiID(body, "agentId", "parentId"), aiID(body, "id", "roleId")
	if parentID == "" || roleID == "" {
		aiError(w, http.StatusBadRequest, "AGENT_ROLE_REQUIRED", "agentId 和 id 不能为空")
		return true
	}
	s.mu.Lock()
	index, parent := accountByIDLocked(s.data.Agents, parentID)
	if parent == nil {
		s.mu.Unlock()
		aiError(w, http.StatusNotFound, "AGENT_NOT_FOUND", "Agent 不存在")
		return true
	}
	parent = cloneMap(parent)
	roles := mapSlice(parent["roles"])
	updated, found := applyAgentRoleChange(path, body, roles, roleID)
	if !found {
		s.mu.Unlock()
		aiError(w, http.StatusNotFound, "AGENT_ROLE_NOT_FOUND", "Agent 角色不存在")
		return true
	}
	parent["roles"] = mapsToAny(updated)
	parent["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.data.Agents[index] = parent
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		aiError(w, http.StatusInternalServerError, "AGENT_SAVE_FAILED", err.Error())
		return true
	}
	aiOK(w, map[string]any{"updated": true, "roles": updated})
	return true
}

// applyAgentRoleChange 在角色集合中执行指定操作。
func applyAgentRoleChange(path string, body map[string]any, roles []map[string]any, roleID string) ([]map[string]any, bool) {
	for i, role := range roles {
		if aiID(role, "id", "roleId") != roleID {
			continue
		}
		switch path {
		case "agents/agent/delete":
			return append(roles[:i], roles[i+1:]...), true
		case "agents/agent/bind":
			role["channel"], role["accountId"] = aiString(body, "channel"), aiString(body, "accountId")
			roles[i] = role
		case "agents/agent/unbind":
			delete(role, "channel")
			delete(role, "accountId")
			roles[i] = role
		}
		return roles, true
	}
	return roles, false
}

// mapSlice 将任意 JSON 数组安全转换为独立的对象切片。
func mapSlice(value any) []map[string]any {
	result := make([]map[string]any, 0)
	switch raw := value.(type) {
	case []any:
		for _, item := range raw {
			if row, ok := item.(map[string]any); ok {
				result = append(result, cloneMap(row))
			}
		}
	case []map[string]any:
		for _, row := range raw {
			result = append(result, cloneMap(row))
		}
	}
	return result
}

// mapsToAny 将对象切片转换为通用数组，便于写入 JSON 持久化结构。
func mapsToAny(values []map[string]any) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

// mapStringSlice 读取 JSON 字符串数组并过滤空白值。
func mapStringSlice(value any) []string {
	result := make([]string, 0)
	switch raw := value.(type) {
	case []any:
		for _, item := range raw {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, strings.TrimSpace(text))
			}
		}
	case []string:
		for _, text := range raw {
			if strings.TrimSpace(text) != "" {
				result = append(result, strings.TrimSpace(text))
			}
		}
	}
	return result
}

// newAgentToken 生成不可预测的 Agent 访问令牌。
func newAgentToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf("wm_%x", buf), nil
}

// handleAgentPairingApprove 通过固定的 Docker 参数调用 Agent 配对命令。
// 未发现已登记容器时返回明确的不可用状态，避免伪造“已接受”结果。
func handleAgentPairingApprove(w http.ResponseWriter, r *http.Request, s *executionState, body map[string]any) {
	agentID := aiID(body, "agentId")
	pairingType := strings.ToLower(aiString(body, "type"))
	code := strings.TrimSpace(aiString(body, "pairingCode"))
	if agentID == "" || code == "" || !validAgentChannel(pairingType) || !validPairingCode(code) {
		aiError(w, http.StatusBadRequest, "PAIRING_PARAMETERS_INVALID", "agentId、type 或 pairingCode 参数无效")
		return
	}
	s.mu.RLock()
	var agent map[string]any
	for _, item := range s.data.Agents {
		if aiID(item, "id") == agentID {
			agent = cloneMap(item)
			break
		}
	}
	s.mu.RUnlock()
	if agent == nil {
		aiError(w, http.StatusNotFound, "AGENT_NOT_FOUND", "Agent 不存在")
		return
	}
	container := strings.TrimSpace(aiString(agent, "containerName", "container"))
	if !validDockerIdentifier(container) {
		aiError(w, http.StatusServiceUnavailable, "AGENT_RUNTIME_UNAVAILABLE", "Agent 尚未绑定可用容器")
		return
	}
	program := "openclaw"
	if strings.EqualFold(aiString(agent, "agentType"), "hermes-agent") || strings.EqualFold(aiString(agent, "type"), "hermes") {
		program = "hermes"
	}
	args := []string{"exec", container, program, "pairing", "approve", pairingType, code}
	if accountID := strings.TrimSpace(aiString(body, "accountId")); accountID != "" {
		if !validDockerIdentifier(accountID) {
			aiError(w, http.StatusBadRequest, "PAIRING_ACCOUNT_INVALID", "accountId 参数无效")
			return
		}
		args = append(args, "--account", accountID)
	}
	result, err := (service.CommandService{}).Execute(r.Context(), model.CommandRequest{Program: "docker", Args: args, Timeout: 20 * time.Second})
	if err != nil {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = err.Error()
		}
		aiError(w, http.StatusBadGateway, "AGENT_PAIRING_FAILED", message)
		return
	}
	aiOK(w, map[string]any{"accepted": true, "status": "approved", "agentId": agentID, "type": pairingType, "output": result.Stdout})
}

// validAgentChannel 判断配对请求使用的消息频道是否受支持。
func validAgentChannel(value string) bool {
	switch value {
	case "feishu", "telegram", "discord", "wecom", "qqbot", "dingtalk":
		return true
	default:
		return false
	}
}

// validPairingCode 校验配对码字符范围和长度，避免命令参数注入。
func validPairingCode(value string) bool {
	if len(value) < 2 || len(value) > 128 {
		return false
	}
	for _, ch := range value {
		if !(ch == '-' || ch == '_' || ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z') {
			return false
		}
	}
	return true
}

// syncMCPStatuses 对指定 MCP 服务执行一次受限探测，并把最近状态写回 ai.json。
func syncMCPStatuses(s *executionState, body map[string]any) []map[string]any {
	requested := map[string]bool{}
	if raw, ok := body["ids"].([]any); ok {
		for _, value := range raw {
			if id := aiID(map[string]any{"id": value}, "id"); id != "" {
				requested[id] = true
			}
		}
	}
	s.mu.RLock()
	servers := make([]map[string]any, 0, len(s.data.MCP))
	for _, item := range s.data.MCP {
		id := aiID(item, "id")
		if len(requested) == 0 || requested[id] {
			servers = append(servers, cloneMap(item))
		}
	}
	s.mu.RUnlock()
	result := make([]map[string]any, 0, len(servers))
	for _, server := range servers {
		id := aiID(server, "id")
		probe, err := testMCPConnection(s, map[string]any{"id": id})
		status, message := "running", "连接成功"
		if err != nil {
			status, message = "error", err.Error()
		}
		s.mu.Lock()
		for i, item := range s.data.MCP {
			if aiID(item, "id") == id {
				item["status"], item["message"] = status, message
				item["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
				s.data.MCP[i] = item
				break
			}
		}
		if saveErr := s.saveLocked(); saveErr != nil {
			status = "error"
			message = fmt.Sprintf("保存 MCP 状态失败: %v", saveErr)
		}
		s.mu.Unlock()
		entry := map[string]any{"id": id, "status": status, "message": message}
		if probe != nil {
			entry["endpoint"] = probe["endpoint"]
		}
		result = append(result, entry)
	}
	return result
}
