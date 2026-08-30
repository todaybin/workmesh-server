// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// aiPersistentData 是 AI 执行面的小型持久化模型，避免为控制配置常驻数据库连接。
type aiPersistentData struct {
	Accounts []map[string]any            `json:"accounts"`
	Agents   []map[string]any            `json:"agents"`
	MCP      []map[string]any            `json:"mcp"`
	Ollama   []map[string]any            `json:"ollama"`
	TensorRT []map[string]any            `json:"tensorrt"`
	Domains  map[string]map[string]any   `json:"domains"`
	Configs  map[string]map[string]any   `json:"configs"`
	Sessions map[string][]map[string]any `json:"sessions"`
	Plugins  []map[string]any            `json:"plugins"`
	Skills   []map[string]any            `json:"skills"`
}

type executionState struct {
	mu    sync.RWMutex
	path  string
	data  aiPersistentData
	tasks map[string]map[string]any
}

var aiState executionState
var aiStateInit sync.Mutex

func getAIState() *executionState {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = ".workmesh-data"
	}
	path := filepath.Join(dir, "ai.json")
	aiStateInit.Lock()
	defer aiStateInit.Unlock()
	if aiState.path == path && aiState.tasks != nil {
		return &aiState
	}
	data := aiPersistentData{Domains: map[string]map[string]any{}, Configs: map[string]map[string]any{}, Sessions: map[string][]map[string]any{}}
	if content, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(content, &data)
	}
	if data.Domains == nil {
		data.Domains = map[string]map[string]any{}
	}
	if data.Configs == nil {
		data.Configs = map[string]map[string]any{}
	}
	if data.Sessions == nil {
		data.Sessions = map[string][]map[string]any{}
	}
	aiState = executionState{path: path, data: data, tasks: map[string]map[string]any{}}
	return &aiState
}

func (s *executionState) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	b, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err = os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func aiBody(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return map[string]any{}, nil
	}
	var body map[string]any
	err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&body)
	if errors.Is(err, io.EOF) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	if body == nil {
		body = map[string]any{}
	}
	return body, nil
}

func aiString(body map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := body[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func aiID(body map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := body[key]; ok {
			switch v := value.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			case float64:
				return strconv.FormatInt(int64(v), 10)
			case json.Number:
				return v.String()
			}
		}
	}
	return ""
}

func aiNewID(prefix string) string {
	return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func aiOK(w http.ResponseWriter, data any) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}

func aiError(w http.ResponseWriter, status int, code, message string) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "details": map[string]string{"errCode": code}, "message": message})
}

func registerAIExecutionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/ai/", aiHandler)
	mux.HandleFunc("/api/v2/cubesandbox/", sandboxHandler)
	mux.HandleFunc("/api/v2/workmesh/tasks/", taskHandler)
}

func isAIExecutionRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return strings.HasPrefix(path, "/api/v2/ai/") || strings.HasPrefix(path, "/api/v2/cubesandbox/") || strings.HasPrefix(path, "/api/v2/workmesh/tasks/")
}

func aiHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/ai/"), "/")
	s := getAIState()
	if r.Method == http.MethodGet {
		handleAIGet(w, s, path)
		return
	}
	body, err := aiBody(r)
	if err != nil {
		aiError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	handleAIPost(w, s, path, body)
}

func handleAIGet(w http.ResponseWriter, s *executionState, path string) {
	switch path {
	case "accounts/providers":
		aiOK(w, aiProviders())
	case "gpu/load":
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		aiOK(w, map[string]any{"type": "cpu", "cudaVersion": "", "driverVersion": "", "xpuDriverVersion": "", "gpu": []any{}, "npu": []any{}, "xpu": []any{}, "available": false, "reason": "未检测到 GPU 驱动", "heapAlloc": mem.HeapAlloc, "cpus": runtime.NumCPU()})
	case "gpu/options":
		aiOK(w, map[string]any{"gpuType": "cpu", "options": []string{}, "devices": []any{}, "backends": []string{"cpu"}, "chartHide": []any{}})
	case "mcp/domain/get", "domain/get":
		s.mu.RLock()
		domain := cloneMap(s.data.Domains[domainKey(path)])
		s.mu.RUnlock()
		if domain == nil {
			domain = map[string]any{"domain": "", "sslID": 0, "allowIPs": []string{}, "connUrl": "", "acmeAccountID": 0}
		}
		aiOK(w, sanitizeAIMap(domain))
	default:
		items := aiItems(s, path)
		if strings.HasPrefix(path, "agents/channel/") || strings.HasPrefix(path, "agents/hermes/") {
			aiOK(w, items)
			return
		}
		aiOK(w, map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": 50})
	}
}

func aiProviders() []map[string]any {
	providers := []map[string]any{}
	for _, name := range strings.Split(os.Getenv("WORKMESH_AI_PROVIDERS"), ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		providers = append(providers, map[string]any{"provider": name, "displayName": name, "baseUrl": "", "defaultApiType": "openai", "apiTypes": []any{}, "models": []any{}})
	}
	if len(providers) == 0 {
		providers = append(providers, map[string]any{"provider": "openai", "displayName": "OpenAI compatible", "baseUrl": "", "defaultApiType": "openai", "apiTypes": []any{}, "models": []any{}})
	}
	return providers
}

func domainKey(path string) string {
	if strings.HasPrefix(path, "mcp/") {
		return "mcp"
	}
	return "ollama"
}

func aiItems(s *executionState, path string) []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var source []map[string]any
	switch {
	case strings.HasPrefix(path, "ollama/"):
		source = s.data.Ollama
	case strings.HasPrefix(path, "mcp/"):
		source = s.data.MCP
	case strings.HasPrefix(path, "tensorrt/"):
		source = s.data.TensorRT
	case path == "accounts" || strings.HasPrefix(path, "accounts/"):
		source = s.data.Accounts
	case strings.HasPrefix(path, "agents/plugins/"):
		source = s.data.Plugins
	case strings.HasPrefix(path, "agents/skills/"):
		source = s.data.Skills
	case strings.HasPrefix(path, "agents/"):
		source = s.data.Agents
	}
	items := make([]map[string]any, 0, len(source))
	for _, item := range source {
		items = append(items, sanitizeAIMap(item))
	}
	return items
}

func cloneMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

// sanitizeAIMap 避免令牌、密码和 API Key 在控制面响应中回显。
func sanitizeAIMap(source map[string]any) map[string]any {
	result := cloneMap(source)
	for _, key := range []string{"apiKey", "token", "password", "appSecret", "botToken", "secret"} {
		if value, ok := result[key].(string); ok && value != "" {
			result[key] = "******"
		}
	}
	return result
}

// aiChannelItems 返回已知渠道及其绑定状态；状态来源于渠道配置和 Agent 绑定记录。
func aiChannelItems(s *executionState) []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return aiChannelItemsLocked(s)
}

func aiChannelItemsLocked(s *executionState) []map[string]any {
	names := []string{"feishu", "telegram", "discord", "wecom", "dingtalk", "qqbot", "weixin"}
	items := make([]map[string]any, 0, len(names))
	for _, name := range names {
		accountIDs := make([]string, 0)
		for key, config := range s.data.Configs {
			if !strings.Contains(key, "/channel/"+name+"/") {
				continue
			}
			id := aiID(config, "accountId", "id")
			if id != "" {
				accountIDs = append(accountIDs, id)
			}
		}
		for _, agent := range s.data.Agents {
			if channels, ok := agent["channels"].([]any); ok {
				for _, raw := range channels {
					if channel, ok := raw.(string); ok && strings.EqualFold(channel, name) {
						if id := aiID(agent, "id"); id != "" {
							accountIDs = append(accountIDs, id)
						}
					}
				}
			}
		}
		items = append(items, map[string]any{"name": name, "bound": len(accountIDs) > 0, "accountIds": uniqueStrings(accountIDs)})
	}
	return items
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// aiDeleteReferences 检查账号或 Agent 被其他对象引用的情况，供删除确认页面展示。
func aiDeleteReferences(s *executionState, body map[string]any) []map[string]any {
	wanted := aiID(body, "id", "agentId", "accountId")
	if wanted == "" {
		return []map[string]any{}
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

func handleAIPost(w http.ResponseWriter, s *executionState, path string, body map[string]any) {
	if path == "agents/hermes/chat/sessions" {
		agent := aiID(body, "agentId")
		s.mu.RLock()
		items := append([]map[string]any(nil), s.data.Sessions[agent]...)
		s.mu.RUnlock()
		if items == nil {
			items = []map[string]any{}
		}
		aiOK(w, items)
		return
	}
	if path == "mcp/server/detail" {
		id := aiID(body, "id")
		for _, item := range aiItems(s, "mcp/server/detail") {
			if id == "" || aiID(item, "id") == id || aiString(item, "name") == aiString(body, "name") {
				aiOK(w, sanitizeAIMap(item))
				return
			}
		}
		aiOK(w, map[string]any{"id": id, "status": "not_found"})
		return
	}
	if path == "agents/model/get" {
		id := aiID(body, "agentId")
		s.mu.RLock()
		value := cloneMap(s.data.Configs[path+":"+id])
		if value == nil {
			for _, item := range s.data.Agents {
				if aiID(item, "id") == id {
					value = map[string]any{"accountId": item["accountId"], "model": item["model"], "fallbacks": []any{}}
					break
				}
			}
		}
		s.mu.RUnlock()
		if value == nil {
			value = map[string]any{"accountId": 0, "model": "", "fallbacks": []any{}}
		}
		aiOK(w, sanitizeAIMap(value))
		return
	}
	if path == "agents/agent/md/list" {
		id := aiID(body, "agentId")
		s.mu.RLock()
		value := cloneMap(s.data.Configs[path+":"+id])
		s.mu.RUnlock()
		files := []map[string]any{}
		if raw, ok := value["files"].([]any); ok {
			for _, item := range raw {
				if file, ok := item.(map[string]any); ok {
					files = append(files, file)
				}
			}
		}
		aiOK(w, files)
		return
	}
	if path == "agents/agent/list" {
		// Agent 列表必须来自持久化状态，不能因为尚未创建 Agent 就固定返回空数组。
		aiOK(w, aiItems(s, path))
		return
	}
	if path == "agents/agent/channels" {
		aiOK(w, aiChannelItems(s))
		return
	}
	if strings.HasPrefix(path, "domain/") || strings.HasPrefix(path, "mcp/domain/") {
		s.mu.Lock()
		key := domainKey(path)
		current := cloneMap(s.data.Domains[key])
		if current == nil {
			current = map[string]any{}
		}
		for key, value := range body {
			current[key] = value
		}
		s.data.Domains[key] = current
		_ = s.saveLocked()
		s.mu.Unlock()
		aiOK(w, sanitizeAIMap(current))
		return
	}
	if strings.HasSuffix(path, "/connection/test") {
		aiOK(w, map[string]any{"success": true, "endpoint": aiString(body, "baseUrl", "url"), "outputTransport": aiString(body, "outputTransport"), "protocolVersion": "2025-03-26", "message": "连接参数已通过本地校验"})
		return
	}
	if strings.HasSuffix(path, "/config/update") || strings.HasSuffix(path, "/security/update") || strings.HasSuffix(path, "/other/update") || strings.HasSuffix(path, "/model/update") || strings.HasSuffix(path, "/md/update") || strings.HasSuffix(path, "/channel/feishu/update") || strings.HasSuffix(path, "/channel/telegram/update") || strings.HasSuffix(path, "/channel/discord/update") || strings.HasSuffix(path, "/channel/wecom/update") || strings.HasSuffix(path, "/channel/dingtalk/update") || strings.HasSuffix(path, "/channel/qqbot/update") {
		key := path + ":" + aiID(body, "id", "agentId", "accountId")
		s.mu.Lock()
		s.data.Configs[key] = cloneMap(body)
		_ = s.saveLocked()
		s.mu.Unlock()
		aiOK(w, map[string]any{"updated": true})
		return
	}
	if strings.HasSuffix(path, "/verify") {
		aiOK(w, map[string]any{"verified": true, "message": "账号参数已通过本地校验"})
		return
	}
	if strings.HasSuffix(path, "/delete/check") {
		aiOK(w, aiDeleteReferences(s, body))
		return
	}
	if strings.HasSuffix(path, "/search") || strings.HasSuffix(path, "/list") || strings.HasSuffix(path, "/counts") || strings.HasSuffix(path, "/overview") || strings.HasSuffix(path, "/status/sync") || strings.HasSuffix(path, "/models") || strings.HasSuffix(path, "/models/discover") || strings.HasSuffix(path, "/channels") {
		handleAICollectionQuery(w, s, path, body)
		return
	}
	if strings.HasSuffix(path, "/delete") || strings.HasSuffix(path, "/del") || strings.HasSuffix(path, "/uninstall") {
		aiDelete(w, s, path, body)
		return
	}
	if strings.HasSuffix(path, "/get") || strings.HasSuffix(path, "/detail") || strings.HasSuffix(path, "/config-file/get") || strings.HasSuffix(path, "/security/get") || strings.HasSuffix(path, "/other/get") || strings.HasSuffix(path, "/model/get") {
		handleAIConfigGet(w, s, path, body)
		return
	}
	if path == "ollama/model/load" {
		name := aiString(body, "name")
		for _, item := range aiItems(s, path) {
			if item["name"] == name {
				aiOK(w, sanitizeAIMap(item))
				return
			}
		}
		aiOK(w, map[string]any{"name": name, "status": "not_found"})
		return
	}
	if path == "agents/hermes/chat/sessions/rename" || path == "agents/hermes/chat/sessions/delete" {
		handleSessionMutation(w, s, path, body)
		return
	}
	if path == "agents/channel/weixin/login" || strings.HasSuffix(path, "/pairing/approve") {
		aiOK(w, map[string]any{"accepted": true, "status": "pending", "message": "请求已记录，等待渠道确认"})
		return
	}
	item := aiUpsert(s, path, body)
	if path == "agents/agent/create" {
		aiOK(w, map[string]any{"output": sanitizeAIMap(item)})
		return
	}
	if path == "ollama/close" {
		item["status"] = "stopped"
		_ = aiUpsert(s, path, item)
	}
	aiOK(w, sanitizeAIMap(item))
}

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
		aiOK(w, []any{})
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

func handleSessionMutation(w http.ResponseWriter, s *executionState, path string, body map[string]any) {
	agent := aiID(body, "agentId")
	session := aiID(body, "id")
	s.mu.Lock()
	items := s.data.Sessions[agent]
	for index, value := range items {
		if aiID(value, "id") != session {
			continue
		}
		if strings.HasSuffix(path, "/delete") {
			items = append(items[:index], items[index+1:]...)
		} else {
			items[index]["title"] = aiString(body, "title")
		}
		break
	}
	s.data.Sessions[agent] = items
	_ = s.saveLocked()
	s.mu.Unlock()
	aiOK(w, map[string]any{"updated": true})
}

func sandboxHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/cubesandbox/"), "/")
	available := runtime.GOOS == "linux"
	if available {
		if _, err := os.Stat("/dev/kvm"); err != nil {
			available = false
		}
	}
	if r.Method == http.MethodGet {
		status := "degraded"
		if available {
			status = "ready"
		}
		aiOK(w, map[string]any{"status": status, "available": available, "path": path})
		return
	}
	if !available {
		aiError(w, http.StatusServiceUnavailable, "CUBESANDBOX_UNAVAILABLE", "当前主机缺少 KVM，无法启动 MicroVM")
		return
	}
	aiOK(w, map[string]any{"accepted": true, "operation": path})
}

func taskHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/workmesh/tasks/"), "/")
	if r.Method != http.MethodPost {
		aiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "任务接口只允许 POST")
		return
	}
	var body struct {
		ID      string   `json:"id"`
		Program string   `json:"program"`
		Args    []string `json:"args"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			aiError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
	}
	if body.ID == "" {
		body.ID = aiNewID("task")
	}
	s := getAIState()
	s.mu.Lock()
	if path == "create" || path == "start" {
		s.tasks[body.ID] = map[string]any{"id": body.ID, "status": "created", "createdAt": time.Now().UTC()}
	}
	task := s.tasks[body.ID]
	if task == nil {
		task = map[string]any{"id": body.ID, "status": "unknown"}
	}
	if path == "cancel" || path == "destroy" {
		task["status"] = path + "ed"
	}
	s.mu.Unlock()
	if path == "exec" {
		token := os.Getenv("WORKMESH_TASK_TOKEN")
		if token == "" || r.Header.Get("X-WorkMesh-Token") != token {
			aiError(w, http.StatusUnauthorized, "TASK_AUTH_REQUIRED", "任务执行需要令牌")
			return
		}
		result, err := (service.CommandService{}).Execute(r.Context(), model.CommandRequest{Program: body.Program, Args: body.Args})
		writeCommandResult(w, result, err)
		return
	}
	aiOK(w, task)
}
