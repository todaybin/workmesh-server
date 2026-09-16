// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// aiDeleteReferences 返回账户被 Agent 引用的记录，供删除前确认影响范围。
func aiDeleteReferences(s *executionState, body map[string]any) []map[string]any {
	wanted := aiID(body, "id", "agentId", "accountId")
	if wanted == "" {
		return make([]map[string]any, 0)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	refs := make([]map[string]any, 0)
	for _, agent := range s.data.Agents {
		if aiID(agent, "accountId", "providerAccountId") == wanted {
			refs = append(refs, map[string]any{"type": "agent", "id": agent["id"], "name": agent["name"]})
		}
	}
	return refs
}

// collectionFor 根据 API 路径选择对应的持久化集合。
func collectionFor(s *aiPersistentData, path string) *[]map[string]any {
	switch {
	case strings.HasPrefix(path, "ollama/"):
		return &s.Ollama
	case strings.HasPrefix(path, "mcp/"):
		return &s.MCP
	case strings.HasPrefix(path, "tensorrt/"):
		return &s.TensorRT
	case path == "accounts" || strings.HasPrefix(path, "accounts/"):
		return &s.Accounts
	case strings.HasPrefix(path, "agents/plugins/"):
		return &s.Plugins
	case strings.HasPrefix(path, "agents/skills/"):
		return &s.Skills
	default:
		return &s.Agents
	}
}

// accountByIDLocked 返回账户并由调用方持有写锁或读锁，避免并发读写竞态。
func accountByIDLocked(accounts []map[string]any, id string) (int, map[string]any) {
	for i, account := range accounts {
		if aiID(account, "id") == id {
			return i, account
		}
	}
	return -1, nil
}

// accountModels 读取账户模型并复制对象，避免调用方修改共享状态。
func accountModels(account map[string]any) []map[string]any {
	models := make([]map[string]any, 0)
	if raw, ok := account["models"].([]any); ok {
		for _, value := range raw {
			if item, ok := value.(map[string]any); ok {
				models = append(models, cloneMap(item))
			}
		}
	}
	return models
}

// setAccountModels 将模型切片转换为 JSON 兼容结构并写回账户。
func setAccountModels(account map[string]any, models []map[string]any) {
	raw := make([]any, 0, len(models))
	for _, model := range models {
		raw = append(raw, model)
	}
	account["models"] = raw
}

// accountModelFromBody 统一解析模型对象和简写模型 ID 参数。
func accountModelFromBody(body map[string]any) map[string]any {
	if modelValue, ok := body["model"].(map[string]any); ok {
		return cloneMap(modelValue)
	}
	return map[string]any{"id": aiString(body, "model", "modelId"), "name": aiString(body, "model", "modelId")}
}

// handleAccountRoute 实现 AI 账户及模型的增删改查，数据写入 ai.json 并在响应中脱敏。
func handleAccountRoute(w http.ResponseWriter, s *executionState, path string, body map[string]any) bool {
	if path == "accounts/providers" || !strings.HasPrefix(path, "accounts") {
		return false
	}
	switch path {
	case "accounts/counts":
		return handleAccountCounts(w, s)
	case "accounts/search":
		return handleAccountSearch(w, s, body)
	case "accounts/models":
		return handleAccountModels(w, s, body)
	case "accounts/models/discover":
		return handleAccountModelDiscovery(w, body)
	case "accounts/verify":
		return handleAccountVerify(w, body)
	case "accounts", "accounts/update":
		return handleAccountWrite(w, s, path, body)
	case "accounts/delete":
		aiDelete(w, s, path, body)
		return true
	}
	if strings.HasPrefix(path, "accounts/models/") {
		return handleAccountModelWrite(w, s, path, body)
	}
	return false
}

// handleAccountCounts 返回各 AI 提供商的账户数量。
func handleAccountCounts(w http.ResponseWriter, s *executionState) bool {
	s.mu.RLock()
	counts := make(map[string]int)
	for _, account := range s.data.Accounts {
		counts[aiString(account, "provider")]++
	}
	s.mu.RUnlock()
	aiOK(w, counts)
	return true
}

// handleAccountSearch 按提供商和名称筛选账户并返回分页结果。
func handleAccountSearch(w http.ResponseWriter, s *executionState, body map[string]any) bool {
	items := aiItems(s, "accounts")
	provider, name := aiString(body, "provider"), aiString(body, "name")
	filtered := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if provider != "" && !strings.EqualFold(provider, aiString(item, "provider")) {
			continue
		}
		if name != "" && !strings.Contains(strings.ToLower(aiString(item, "name")), strings.ToLower(name)) {
			continue
		}
		filtered = append(filtered, item)
	}
	page, size := accountSearchPage(body)
	start := (page - 1) * size
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + size
	if end > len(filtered) {
		end = len(filtered)
	}
	aiOK(w, map[string]any{"items": filtered[start:end], "total": len(filtered), "page": page, "pageSize": size})
	return true
}

// accountSearchPage 解析账户搜索的分页参数并应用边界限制。
func accountSearchPage(body map[string]any) (int, int) {
	page, size := 1, 50
	if value, ok := body["page"].(float64); ok && int(value) > 0 {
		page = int(value)
	}
	if value, ok := body["pageSize"].(float64); ok && int(value) > 0 && int(value) <= 200 {
		size = int(value)
	}
	return page, size
}

// handleAccountModels 返回指定账户当前保存的模型列表。
func handleAccountModels(w http.ResponseWriter, s *executionState, body map[string]any) bool {
	id := aiID(body, "accountId")
	s.mu.RLock()
	_, account := accountByIDLocked(s.data.Accounts, id)
	models := make([]map[string]any, 0)
	if account != nil {
		models = accountModels(account)
	}
	s.mu.RUnlock()
	aiOK(w, models)
	return true
}

// handleAccountModelDiscovery 从账户服务的真实模型端点读取模型目录。
func handleAccountModelDiscovery(w http.ResponseWriter, body map[string]any) bool {
	models, err := discoverAIModels(body)
	if err != nil {
		aiError(w, http.StatusBadGateway, "MODEL_DISCOVERY_FAILED", err.Error())
		return true
	}
	aiOK(w, models)
	return true
}

// handleAccountVerify 校验账户参数并按需执行真实网络验证。
func handleAccountVerify(w http.ResponseWriter, body map[string]any) bool {
	result, err := verifyAIAccount(body)
	if err != nil {
		aiError(w, http.StatusBadRequest, "ACCOUNT_VERIFY_FAILED", err.Error())
		return true
	}
	aiOK(w, result)
	return true
}

// handleAccountWrite 创建或更新 AI 账户，并在响应中隐藏敏感字段。
func handleAccountWrite(w http.ResponseWriter, s *executionState, path string, body map[string]any) bool {
	provider, name := aiString(body, "provider"), aiString(body, "name")
	if provider == "" || name == "" {
		aiError(w, http.StatusBadRequest, "ACCOUNT_REQUIRED", "provider 和 name 不能为空")
		return true
	}
	if path == "accounts/update" {
		return updateAIAccount(w, s, body)
	}
	body["provider"], body["name"] = provider, name
	if aiString(body, "apiType") == "" {
		body["apiType"] = "openai-completions"
	}
	if aiString(body, "apiKey") == "" {
		body["apiKey"] = ""
	}
	if _, ok := body["models"]; !ok {
		setAccountModels(body, make([]map[string]any, 0))
	}
	item := aiUpsert(s, path, body)
	aiOK(w, sanitizeAIMap(item))
	return true
}

// updateAIAccount 合并账户字段并持久化更新结果。
func updateAIAccount(w http.ResponseWriter, s *executionState, body map[string]any) bool {
	id := aiID(body, "id")
	s.mu.Lock()
	index, account := accountByIDLocked(s.data.Accounts, id)
	if account == nil {
		s.mu.Unlock()
		aiError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "账户不存在")
		return true
	}
	for key, value := range body {
		if key != "apiKey" || strings.TrimSpace(aiString(body, "apiKey")) != "" {
			account[key] = value
		}
	}
	account["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.data.Accounts[index] = account
	err := s.saveLocked()
	out := sanitizeAIMap(account)
	s.mu.Unlock()
	if err != nil {
		aiError(w, http.StatusInternalServerError, "ACCOUNT_SAVE_FAILED", err.Error())
		return true
	}
	aiOK(w, out)
	return true
}

// handleAccountModelWrite 创建、更新或删除指定账户下的模型。
func handleAccountModelWrite(w http.ResponseWriter, s *executionState, path string, body map[string]any) bool {
	accountID := aiID(body, "accountId")
	s.mu.Lock()
	index, account := accountByIDLocked(s.data.Accounts, accountID)
	if account == nil {
		s.mu.Unlock()
		aiError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "账户不存在")
		return true
	}
	models := accountModels(account)
	modelValue := accountModelFromBody(body)
	modelID := aiString(modelValue, "id", "name")
	if modelID == "" {
		s.mu.Unlock()
		aiError(w, http.StatusBadRequest, "MODEL_REQUIRED", "模型 ID 不能为空")
		return true
	}
	updated, status, code, message, ok := applyAccountModelChange(path, body, models, modelValue, modelID)
	if status != 0 {
		s.mu.Unlock()
		aiError(w, status, code, message)
		return true
	}
	if !ok {
		s.mu.Unlock()
		return false
	}
	setAccountModels(account, updated)
	account["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.data.Accounts[index] = account
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		aiError(w, http.StatusInternalServerError, "ACCOUNT_SAVE_FAILED", err.Error())
		return true
	}
	aiOK(w, updated)
	return true
}

// applyAccountModelChange 在已持有账户锁时执行模型集合变更。
func applyAccountModelChange(path string, body map[string]any, models []map[string]any, modelValue map[string]any, modelID string) ([]map[string]any, int, string, string, bool) {
	switch path {
	case "accounts/models/create":
		for _, existing := range models {
			if aiString(existing, "id", "name") == modelID {
				return models, http.StatusConflict, "MODEL_EXISTS", "模型已存在", true
			}
		}
		if aiString(modelValue, "name") == "" {
			modelValue["name"] = modelID
		}
		modelValue["recordId"] = aiNewID("model")
		return append(models, modelValue), 0, "", "", true
	case "accounts/models/update":
		for i, existing := range models {
			if aiString(existing, "id", "name") == modelID || aiID(existing, "recordId") == aiID(body, "recordId") {
				models[i] = modelValue
				return models, 0, "", "", true
			}
		}
		return models, http.StatusNotFound, "MODEL_NOT_FOUND", "模型不存在", true
	case "accounts/models/delete":
		target := aiID(body, "recordId")
		kept := make([]map[string]any, 0, len(models))
		removed := false
		for _, existing := range models {
			if (target != "" && aiID(existing, "recordId") == target) || (target == "" && aiString(existing, "id", "name") == modelID) {
				removed = true
				continue
			}
			kept = append(kept, existing)
		}
		if !removed {
			return models, http.StatusNotFound, "MODEL_NOT_FOUND", "模型不存在", true
		}
		return kept, 0, "", "", true
	default:
		return models, 0, "", "", true
	}
}

// verifyAIAccount 校验 API 密钥，并在提供地址时执行真实模型目录探测。
func verifyAIAccount(body map[string]any) (map[string]any, error) {
	base := aiString(body, "baseURL", "baseUrl")
	key := aiString(body, "apiKey")
	if key == "" {
		return nil, errors.New("apiKey 不能为空")
	}
	if base == "" {
		return map[string]any{"verified": false, "network": false, "reason": "未提供 baseURL，仅完成本地参数校验"}, nil
	}
	models, err := discoverAIModels(body)
	if err != nil {
		return nil, err
	}
	return map[string]any{"verified": true, "network": true, "models": models}, nil
}

// discoverAIModels 请求 OpenAI 兼容的模型端点并转换为面板模型格式。
func discoverAIModels(body map[string]any) ([]map[string]any, error) {
	base := strings.TrimRight(aiString(body, "baseURL", "baseUrl"), "/")
	if base == "" {
		return nil, errors.New("baseURL 不能为空")
	}
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("baseURL 必须是有效的 HTTP(S) 地址")
	}
	endpoint := base
	if !strings.HasSuffix(endpoint, "/models") {
		endpoint += "/models"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if key := aiString(body, "apiKey"); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求模型目录失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("模型目录返回 HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析模型目录失败: %w", err)
	}
	result := make([]map[string]any, 0, len(payload.Data))
	for _, item := range payload.Data {
		id := item.ID
		if id == "" {
			id = item.Name
		}
		if id != "" {
			result = append(result, map[string]any{"id": id, "name": id})
		}
	}
	return result, nil
}

// testMCPConnection 按 MCP 传输协议探测服务，不以参数校验代替真实网络检查。
// SSE 传输必须返回 text/event-stream；Streamable HTTP 传输发送 initialize JSON-RPC 请求。
