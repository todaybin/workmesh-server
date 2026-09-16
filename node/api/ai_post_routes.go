// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"strings"
)

// handleAIPostPrimary 处理需要精确路径匹配的 AI 查询和配置入口。
func handleAIPostPrimary(w http.ResponseWriter, s *executionState, path string, body map[string]any) bool {
	switch path {
	case "agents/hermes/chat/sessions":
		writeAISessions(w, s, body)
	case "mcp/server/detail":
		writeMCPServerDetail(w, s, body)
	case "mcp/server/status/sync":
		aiOK(w, syncMCPStatuses(s, body))
	case "agents/model/get":
		writeAgentModel(w, s, body)
	case "agents/agent/md/list":
		writeAgentMarkdownFiles(w, s, body)
	case "agents/agent/list":
		aiOK(w, aiItems(s, path))
	case "agents/agent/channels":
		aiOK(w, aiChannelItems(s))
	default:
		return handleAIPostDomainOrConnection(w, s, path, body)
	}
	return true
}

// writeAISessions 返回指定 Agent 在持久化状态中的会话列表。
func writeAISessions(w http.ResponseWriter, s *executionState, body map[string]any) {
	agent := aiID(body, "agentId")
	s.mu.RLock()
	items := append([]map[string]any(nil), s.data.Sessions[agent]...)
	s.mu.RUnlock()
	if items == nil {
		items = make([]map[string]any, 0)
	}
	aiOK(w, items)
}

// writeMCPServerDetail 按标识或名称返回已保存的 MCP 服务详情。
func writeMCPServerDetail(w http.ResponseWriter, s *executionState, body map[string]any) {
	id := aiID(body, "id")
	for _, item := range aiItems(s, "mcp/server/detail") {
		if id == "" || aiID(item, "id") == id || aiString(item, "name") == aiString(body, "name") {
			aiOK(w, sanitizeAIMap(item))
			return
		}
	}
	aiError(w, http.StatusNotFound, "MCP_SERVER_NOT_FOUND", "MCP 服务不存在")
}

// writeAgentModel 返回 Agent 保存的模型配置，并兼容旧 Agent 字段。
func writeAgentModel(w http.ResponseWriter, s *executionState, body map[string]any) {
	id := aiID(body, "agentId")
	s.mu.RLock()
	value := cloneMap(s.data.Configs["agents/model/get:"+id])
	if value == nil {
		for _, item := range s.data.Agents {
			if aiID(item, "id") == id {
				value = map[string]any{"accountId": item["accountId"], "model": item["model"], "fallbacks": make([]any, 0)}
				break
			}
		}
	}
	s.mu.RUnlock()
	if value == nil {
		value = map[string]any{"accountId": 0, "model": "", "fallbacks": make([]any, 0)}
	}
	aiOK(w, sanitizeAIMap(value))
}

// writeAgentMarkdownFiles 返回 Agent 配置中记录的 Markdown 文件列表。
func writeAgentMarkdownFiles(w http.ResponseWriter, s *executionState, body map[string]any) {
	id := aiID(body, "agentId")
	s.mu.RLock()
	value := cloneMap(s.data.Configs["agents/agent/md/list:"+id])
	s.mu.RUnlock()
	files := make([]map[string]any, 0)
	if raw, ok := value["files"].([]any); ok {
		for _, item := range raw {
			if file, ok := item.(map[string]any); ok {
				files = append(files, file)
			}
		}
	}
	aiOK(w, files)
}

// handleAIPostDomainOrConnection 处理领域配置和真实 MCP 连通性检测。
func handleAIPostDomainOrConnection(w http.ResponseWriter, s *executionState, path string, body map[string]any) bool {
	if strings.HasPrefix(path, "domain/") || strings.HasPrefix(path, "mcp/domain/") {
		updateAIDomain(w, s, path, body)
		return true
	}
	if path == "mcp/server/connection/test" {
		result, err := testMCPConnection(s, body)
		if err != nil {
			aiError(w, http.StatusBadGateway, "MCP_CONNECTION_FAILED", err.Error())
			return true
		}
		aiOK(w, result)
		return true
	}
	if strings.HasSuffix(path, "/connection/test") {
		aiError(w, http.StatusNotImplemented, "CONNECTION_TEST_UNSUPPORTED", "当前功能域未提供连接测试实现")
		return true
	}
	return false
}

// updateAIDomain 合并并持久化领域配置。
func updateAIDomain(w http.ResponseWriter, s *executionState, path string, body map[string]any) {
	s.mu.Lock()
	key := domainKey(path)
	current := cloneMap(s.data.Domains[key])
	if current == nil {
		current = map[string]any{}
	}
	for field, value := range body {
		current[field] = value
	}
	s.data.Domains[key] = current
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		aiError(w, http.StatusInternalServerError, "AI_CONFIG_SAVE_FAILED", err.Error())
		return
	}
	aiOK(w, sanitizeAIMap(current))
}

// handleAIPostConfig 处理 AI 配置更新、校验和删除引用查询。
func handleAIPostConfig(w http.ResponseWriter, s *executionState, path string, body map[string]any) bool {
	if isAIConfigUpdatePath(path) {
		key := path + ":" + aiID(body, "id", "agentId", "accountId")
		s.mu.Lock()
		s.data.Configs[key] = cloneMap(body)
		err := s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			aiError(w, http.StatusInternalServerError, "AI_CONFIG_SAVE_FAILED", err.Error())
			return true
		}
		aiOK(w, map[string]any{"updated": true})
		return true
	}
	if strings.HasSuffix(path, "/verify") {
		aiOK(w, map[string]any{"verified": true, "message": "账号参数已通过本地校验"})
		return true
	}
	if strings.HasSuffix(path, "/delete/check") {
		aiOK(w, aiDeleteReferences(s, body))
		return true
	}
	return false
}

// isAIConfigUpdatePath 判断路径是否属于兼容的 Agent 配置更新集合。
func isAIConfigUpdatePath(path string) bool {
	for _, suffix := range []string{"/config/update", "/security/update", "/other/update", "/model/update", "/md/update", "/channel/feishu/update", "/channel/telegram/update", "/channel/discord/update", "/channel/wecom/update", "/channel/dingtalk/update", "/channel/qqbot/update"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

// handleAIPostQuery 处理集合、详情和运行时依赖查询。
func handleAIPostQuery(w http.ResponseWriter, s *executionState, path string, body map[string]any) bool {
	if isAICollectionQueryPath(path) {
		handleAICollectionQuery(w, s, path, body)
		return true
	}
	if requiresAgentRuntime(path) {
		aiError(w, http.StatusServiceUnavailable, "AGENT_RUNTIME_UNAVAILABLE", "该操作需要已配置的 Agent 容器运行时")
		return true
	}
	if isAIConfigGetPath(path) {
		handleAIConfigGet(w, s, path, body)
		return true
	}
	return false
}

// isAICollectionQueryPath 判断路径是否返回集合或聚合信息。
func isAICollectionQueryPath(path string) bool {
	for _, suffix := range []string{"/search", "/list", "/counts", "/overview", "/status/sync", "/models", "/models/discover", "/channels"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

// isAIConfigGetPath 判断路径是否读取单项配置。
func isAIConfigGetPath(path string) bool {
	for _, suffix := range []string{"/get", "/detail", "/config-file/get", "/security/get", "/other/get", "/model/get"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

// handleAIPostMutation 处理会话、删除、配对及需要真实运行时的写操作。
func handleAIPostMutation(w http.ResponseWriter, r *http.Request, s *executionState, path string, body map[string]any) bool {
	if path == "agents/hermes/chat/sessions/rename" || path == "agents/hermes/chat/sessions/delete" {
		handleSessionMutation(w, s, path, body)
		return true
	}
	if strings.HasSuffix(path, "/delete") || strings.HasSuffix(path, "/del") || strings.HasSuffix(path, "/uninstall") {
		aiDelete(w, s, path, body)
		return true
	}
	if strings.HasSuffix(path, "/pairing/approve") {
		handleAgentPairingApprove(w, r, s, body)
		return true
	}
	if path == "agents/channel/weixin/login" {
		aiError(w, http.StatusServiceUnavailable, "AGENT_RUNTIME_UNAVAILABLE", "微信登录需要已配置的 Agent 运行时")
		return true
	}
	return false
}
