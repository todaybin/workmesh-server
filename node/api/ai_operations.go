// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"strings"
	"time"
)

// handleAIPost 统一分派 AI 账号、Agent、MCP、模型和会话操作。
func handleAIPost(w http.ResponseWriter, r *http.Request, s *executionState, path string, body map[string]any) {
	if handleAccountRoute(w, s, path, body) {
		return
	}
	if handleAgentRoute(w, s, path, body) {
		return
	}
	if handleAIResourceOperation(w, s, path, body) {
		return
	}
	if handleAIPostPrimary(w, s, path, body) || handleAIPostConfig(w, s, path, body) || handleAIPostQuery(w, s, path, body) || handleAIPostMutation(w, r, s, path, body) {
		return
	}
	item := aiUpsert(s, path, body)
	if path == "agents/agent/create" {
		aiOK(w, map[string]any{"output": sanitizeAIMap(item)})
		return
	}
	aiOK(w, sanitizeAIMap(item))
}

// handleAIResourceOperation 将启停、重建和详情查询作用于既有的 AI 资源。
// 旧实现会调用运行时；独立服务没有可用运行时时只更新已保存资源的期望状态，绝不创建操作记录冒充模型或 MCP 服务。
func handleAIResourceOperation(w http.ResponseWriter, s *executionState, path string, body map[string]any) bool {
	if path == "ollama/model/load" {
		name := aiString(body, "name")
		if name == "" {
			aiError(w, http.StatusBadRequest, "MODEL_NAME_REQUIRED", "模型名称不能为空")
			return true
		}
		for _, item := range aiItems(s, "ollama/model") {
			if aiString(item, "name") == name {
				aiOK(w, sanitizeAIMap(item))
				return true
			}
		}
		aiError(w, http.StatusNotFound, "OLLAMA_MODEL_NOT_FOUND", "Ollama 模型不存在")
		return true
	}
	operate := ""
	collectionPath := ""
	switch path {
	case "ollama/close":
		operate, collectionPath = "stop", "ollama/model"
	case "ollama/model/recreate":
		operate, collectionPath = "restart", "ollama/model"
	case "mcp/server/op":
		operate, collectionPath = strings.ToLower(aiString(body, "operate")), "mcp/server"
	case "tensorrt/operate":
		operate, collectionPath = strings.ToLower(aiString(body, "operate")), "tensorrt"
	default:
		return false
	}
	if operate != "start" && operate != "stop" && operate != "restart" {
		aiError(w, http.StatusBadRequest, "AI_OPERATION_INVALID", "operate 仅支持 start、stop 或 restart")
		return true
	}
	id, name := aiID(body, "id", "modelId", "serverId"), aiString(body, "name")
	if id == "" && name == "" {
		aiError(w, http.StatusBadRequest, "AI_RESOURCE_REQUIRED", "资源 id 或 name 不能为空")
		return true
	}
	collection := collectionFor(&s.data, collectionPath)
	s.mu.Lock()
	found := -1
	for i, item := range *collection {
		if (id != "" && aiID(item, "id") == id) || (name != "" && aiString(item, "name") == name) {
			found = i
			break
		}
	}
	if found < 0 {
		s.mu.Unlock()
		aiError(w, http.StatusNotFound, "AI_RESOURCE_NOT_FOUND", "AI 资源不存在")
		return true
	}
	item := cloneMap((*collection)[found])
	if operate == "stop" {
		item["status"] = "stopped"
	} else {
		item["status"] = "running"
	}
	item["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	(*collection)[found] = item
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		aiError(w, http.StatusInternalServerError, "AI_RESOURCE_SAVE_FAILED", err.Error())
		return true
	}
	aiOK(w, sanitizeAIMap(item))
	return true
}

// requiresAgentRuntime 判断必须在 Agent 容器内执行的写操作，避免通用状态存储伪造执行结果。
func requiresAgentRuntime(path string) bool {
	if strings.HasPrefix(path, "agents/plugin/") || strings.HasPrefix(path, "agents/plugins/install") || strings.HasPrefix(path, "agents/plugins/operate") {
		return true
	}
	if strings.HasPrefix(path, "agents/skills/") {
		switch path {
		case "agents/skills/list", "agents/skills/search":
			return false
		default:
			return true
		}
	}
	return false
}

// handleAICollectionQuery 返回持久化集合、聚合统计和概览数据。
func handleAICollectionQuery(w http.ResponseWriter, s *executionState, path string, body map[string]any) {
	if strings.HasSuffix(path, "/counts") {
		s.mu.RLock()
		counts := map[string]int{}
		for _, item := range s.data.Accounts {
			provider := aiString(item, "provider")
			counts[provider]++
		}
		s.mu.RUnlock()
		aiOK(w, counts)
		return
	}
	items := aiItems(s, path)
	name := aiString(body, "name", "info")
	if name != "" {
		filtered := items[:0]
		for _, item := range items {
			if strings.Contains(strings.ToLower(aiString(item, "name", "id", "key")), strings.ToLower(name)) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if strings.HasSuffix(path, "/models") {
		id := aiID(body, "accountId")
		for _, account := range items {
			if aiID(account, "id") == id {
				if models, ok := account["models"].([]any); ok {
					aiOK(w, models)
					return
				}
			}
		}
		aiOK(w, make([]map[string]any, 0))
		return
	}
	if strings.HasSuffix(path, "/overview") {
		s.mu.RLock()
		channelCount := len(aiChannelItemsLocked(s))
		sessionCount := 0
		for _, sessions := range s.data.Sessions {
			sessionCount += len(sessions)
		}
		taskCount := len(s.tasks)
		agentCount := len(s.data.Agents)
		accountCount := len(s.data.Accounts)
		skillCount := len(s.data.Skills)
		s.mu.RUnlock()
		aiOK(w, map[string]any{"snapshot": map[string]any{"containerStatus": "unknown", "appVersion": "workmesh-server", "defaultModel": "", "agentCount": agentCount, "accountCount": accountCount, "channelCount": channelCount, "skillCount": skillCount, "jobCount": taskCount, "sessionCount": sessionCount}})
		return
	}
	if strings.HasSuffix(path, "/channels") || strings.HasSuffix(path, "/status/sync") || strings.HasSuffix(path, "/models/discover") {
		aiOK(w, items)
		return
	}
	aiOK(w, map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": 50})
}

// aiUpsert 合并 AI 资源并将变更写入共享 SQLite 状态。
func aiUpsert(s *executionState, path string, body map[string]any) map[string]any {
	collection := collectionFor(&s.data, path)
	item := cloneMap(body)
	if item["id"] == nil {
		item["id"] = aiNewID(strings.Split(path, "/")[0])
	}
	if item["name"] == nil {
		item["name"] = aiString(body, "key", "id")
	}
	if item["status"] == nil {
		item["status"] = "running"
	}
	item["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	key := aiID(item, "id")
	name := aiString(item, "name")
	s.mu.Lock()
	found := false
	for i, existing := range *collection {
		if (key != "" && aiID(existing, "id") == key) || (name != "" && aiString(existing, "name") == name) {
			merged := cloneMap(existing)
			for k, v := range item {
				merged[k] = v
			}
			(*collection)[i] = merged
			item = merged
			found = true
			break
		}
	}
	if !found {
		*collection = append(*collection, item)
	}
	_ = s.saveLocked()
	s.mu.Unlock()
	return cloneMap(item)
}

// aiDelete 删除指定 AI 资源并返回实际删除数量。
func aiDelete(w http.ResponseWriter, s *executionState, path string, body map[string]any) {
	collection := collectionFor(&s.data, path)
	wanted := aiID(body, "id", "agentId", "accountId", "recordId")
	ids := map[string]bool{}
	if raw, ok := body["ids"].([]any); ok {
		for _, value := range raw {
			ids[aiID(map[string]any{"id": value}, "id")] = true
		}
	}
	s.mu.Lock()
	kept := (*collection)[:0]
	deleted := 0
	for _, item := range *collection {
		id := aiID(item, "id", "recordId")
		if (wanted != "" && id == wanted) || ids[id] {
			deleted++
			continue
		}
		kept = append(kept, item)
	}
	*collection = kept
	_ = s.saveLocked()
	s.mu.Unlock()
	aiOK(w, map[string]any{"deleted": true, "count": deleted})
}

// handleAIConfigGet 读取指定 AI 资源的持久化配置。
func handleAIConfigGet(w http.ResponseWriter, s *executionState, path string, body map[string]any) {
	key := path + ":" + aiID(body, "id", "agentId", "accountId")
	s.mu.RLock()
	value := cloneMap(s.data.Configs[key])
	s.mu.RUnlock()
	if value == nil {
		value = map[string]any{}
	}
	aiOK(w, value)
}

// handleSessionMutation 修改或删除已存在的 Hermes 会话。
func handleSessionMutation(w http.ResponseWriter, s *executionState, path string, body map[string]any) {
	agent := aiID(body, "agentId")
	session := aiID(body, "id")
	if agent == "" || session == "" {
		aiError(w, http.StatusBadRequest, "SESSION_REQUIRED", "agentId 和会话 id 不能为空")
		return
	}
	s.mu.Lock()
	if _, owner := accountByIDLocked(s.data.Agents, agent); owner == nil {
		s.mu.Unlock()
		aiError(w, http.StatusNotFound, "AGENT_NOT_FOUND", "Agent 不存在")
		return
	}
	items := s.data.Sessions[agent]
	updated := false
	for index, value := range items {
		if aiID(value, "id") != session {
			continue
		}
		if strings.HasSuffix(path, "/delete") {
			items = append(items[:index], items[index+1:]...)
		} else {
			title := aiString(body, "title")
			if title == "" {
				s.mu.Unlock()
				aiError(w, http.StatusBadRequest, "SESSION_TITLE_REQUIRED", "会话标题不能为空")
				return
			}
			items[index]["title"] = title
		}
		updated = true
		break
	}
	if !updated {
		s.mu.Unlock()
		aiError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "会话不存在")
		return
	}
	s.data.Sessions[agent] = items
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		aiError(w, http.StatusInternalServerError, "SESSION_SAVE_FAILED", err.Error())
		return
	}
	aiOK(w, map[string]any{"updated": true})
}
