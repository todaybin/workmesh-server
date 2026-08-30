// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/service/taskruntime"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// aiPersistentData 是 AI 执行面的小型持久化模型，避免为控制配置常驻数据库连接。
type aiPersistentData struct {
	Accounts  []map[string]any            `json:"accounts"`
	Agents    []map[string]any            `json:"agents"`
	MCP       []map[string]any            `json:"mcp"`
	Ollama    []map[string]any            `json:"ollama"`
	TensorRT  []map[string]any            `json:"tensorrt"`
	Domains   map[string]map[string]any   `json:"domains"`
	Configs   map[string]map[string]any   `json:"configs"`
	Sessions  map[string][]map[string]any `json:"sessions"`
	Plugins   []map[string]any            `json:"plugins"`
	Skills    []map[string]any            `json:"skills"`
	Sandboxes []map[string]any            `json:"sandboxes"`
	Tasks     []map[string]any            `json:"tasks"`
}

type executionState struct {
	mu    sync.RWMutex
	path  string
	data  aiPersistentData
	tasks map[string]map[string]any
}

var aiState executionState
var aiStateInit sync.Mutex

// taskProviderState 按进程缓存受控任务 Provider，避免每个请求重复校验 CLI 摘要。
// 生产环境必须通过 WORKMESH_TASK_CLI 和 WORKMESH_TASK_CLI_SHA256 显式启用。
var taskProviderState struct {
	sync.Mutex
	provider *taskruntime.TaskProvider
	command  string
	digest   string
}

// SetTaskProvider 为集成测试或启动装配注入真实隔离任务 Provider。
func SetTaskProvider(provider *taskruntime.TaskProvider) {
	taskProviderState.Lock()
	taskProviderState.provider = provider
	taskProviderState.command = ""
	taskProviderState.digest = ""
	taskProviderState.Unlock()
}

func getTaskProvider() *taskruntime.TaskProvider {
	command := strings.TrimSpace(os.Getenv("WORKMESH_TASK_CLI"))
	digest := strings.TrimSpace(os.Getenv("WORKMESH_TASK_CLI_SHA256"))
	taskProviderState.Lock()
	defer taskProviderState.Unlock()
	if taskProviderState.provider != nil && taskProviderState.command == "" && taskProviderState.digest == "" {
		return taskProviderState.provider
	}
	if command == "" || digest == "" {
		return nil
	}
	if taskProviderState.provider != nil && taskProviderState.command == command && taskProviderState.digest == digest {
		return taskProviderState.provider
	}
	backend, err := taskruntime.NewCLITaskBackend(command, digest, 30*time.Minute, 8<<20)
	if err != nil {
		return nil
	}
	provider, err := taskruntime.NewTaskProvider(backend)
	if err != nil {
		return nil
	}
	taskProviderState.provider = provider
	taskProviderState.command = command
	taskProviderState.digest = digest
	// CLI 后端可复用重启前的沙盒句柄；无效记录由 Provider 的状态校验忽略。
	s := getAIState()
	s.mu.RLock()
	handles := make([]taskruntime.TaskHandle, 0, len(s.data.Tasks))
	for _, item := range s.data.Tasks {
		handles = append(handles, taskruntime.TaskHandle{
			TaskID: aiID(item, "taskId", "id"), SandboxID: aiString(item, "sandboxId"), ImageDigest: aiString(item, "imageDigest"),
			SandboxType: aiString(item, "sandboxType"), Backend: aiString(item, "backend"), State: taskruntime.TaskState(aiString(item, "status")),
		})
	}
	s.mu.RUnlock()
	provider.Restore(handles)
	return provider
}

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
	data := aiPersistentData{Domains: make(map[string]map[string]any), Configs: make(map[string]map[string]any), Sessions: make(map[string][]map[string]any), Sandboxes: make([]map[string]any, 0), Tasks: make([]map[string]any, 0)}
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
		data.Sessions = make(map[string][]map[string]any)
	}
	if data.Sandboxes == nil {
		data.Sandboxes = make([]map[string]any, 0)
	}
	tasks := make(map[string]map[string]any, len(data.Tasks))
	for _, item := range data.Tasks {
		if id := aiID(item, "taskId", "id"); id != "" {
			tasks[id] = cloneMap(item)
		}
	}
	aiState = executionState{path: path, data: data, tasks: tasks}
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
	registerAIExplicitRoutes(mux)
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
		aiOK(w, detectGPU())
	case "gpu/options":
		load := detectGPU()
		options := []string{"cpu"}
		if load["available"] == true {
			options = append(options, "cuda")
		}
		aiOK(w, map[string]any{"gpuType": load["type"], "options": options, "devices": load["gpu"], "backends": options, "chartHide": make([]string, 0)})
	case "mcp/domain/get", "domain/get":
		s.mu.RLock()
		domain := cloneMap(s.data.Domains[domainKey(path)])
		s.mu.RUnlock()
		if domain == nil {
			domain = map[string]any{"domain": "", "sslID": 0, "allowIPs": make([]string, 0), "connUrl": "", "acmeAccountID": 0}
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

// detectGPU 探测本机 GPU，优先调用 nvidia-smi 并设置 3 秒超时；失败时返回 CPU 能力而非伪造设备。
func detectGPU() map[string]any {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	result := map[string]any{"type": "cpu", "cudaVersion": "", "driverVersion": "", "xpuDriverVersion": "", "gpu": make([]map[string]any, 0), "npu": make([]map[string]any, 0), "xpu": make([]map[string]any, 0), "available": false, "reason": "未检测到可用 GPU", "heapAlloc": mem.HeapAlloc, "cpus": runtime.NumCPU()}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=index,name,memory.total,driver_version", "--format=csv,noheader,nounits")
	output, err := command.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result["reason"] = "GPU 探测超时"
		}
		return result
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	devices := make([]map[string]any, 0)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ",")
		if len(fields) < 4 {
			continue
		}
		devices = append(devices, map[string]any{"index": strings.TrimSpace(fields[0]), "name": strings.TrimSpace(fields[1]), "memoryTotalMB": strings.TrimSpace(fields[2]), "driverVersion": strings.TrimSpace(fields[3])})
	}
	if len(devices) > 0 {
		result["type"], result["available"], result["reason"], result["gpu"] = "cuda", true, "", devices
		if first, ok := devices[0]["driverVersion"].(string); ok {
			result["driverVersion"] = first
		}
	}
	return result
}

func aiProviders() []map[string]any {
	// 目录来自内置协议元数据，环境变量仅用于追加自定义提供商，不返回伪造密钥。
	type providerMeta struct {
		name, display, base, api string
		models                   []string
	}
	catalog := []providerMeta{
		{name: "openai", display: "OpenAI", base: "https://api.openai.com/v1", api: "openai-responses", models: []string{"gpt-5.4", "gpt-5.4-mini"}},
		{name: "anthropic", display: "Anthropic", base: "https://api.anthropic.com", api: "anthropic-messages", models: []string{"claude-sonnet-4-6", "claude-haiku-4-5"}},
		{name: "deepseek", display: "DeepSeek", base: "https://api.deepseek.com", api: "openai-completions", models: []string{"deepseek-v4-flash", "deepseek-v4-pro"}},
		{name: "ollama", display: "Ollama", base: "http://127.0.0.1:11434", api: "openai-completions", models: nil},
		{name: "custom", display: "Custom", base: "", api: "openai-completions", models: nil},
	}
	providers := make([]map[string]any, 0, len(catalog))
	for _, item := range catalog {
		models := make([]map[string]any, 0, len(item.models))
		for _, modelID := range item.models {
			models = append(models, map[string]any{"id": modelID, "name": modelID})
		}
		providers = append(providers, map[string]any{"provider": item.name, "displayName": item.display, "baseUrl": item.base, "defaultApiType": item.api, "apiTypes": []string{item.api}, "models": models})
	}
	for _, name := range strings.Split(os.Getenv("WORKMESH_AI_PROVIDERS"), ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		known := false
		for _, p := range providers {
			if p["provider"] == name {
				known = true
				break
			}
		}
		if !known {
			providers = append(providers, map[string]any{"provider": name, "displayName": name, "baseUrl": "", "defaultApiType": "openai-completions", "apiTypes": []string{"openai-completions"}, "models": make([]map[string]any, 0)})
		}
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

func setAccountModels(account map[string]any, models []map[string]any) {
	raw := make([]any, 0, len(models))
	for _, model := range models {
		raw = append(raw, model)
	}
	account["models"] = raw
}

func accountModelFromBody(body map[string]any) map[string]any {
	if modelValue, ok := body["model"].(map[string]any); ok {
		return cloneMap(modelValue)
	}
	return map[string]any{"id": aiString(body, "model", "modelId"), "name": aiString(body, "model", "modelId")}
}

// handleAccountRoute 实现 AI 账户及模型的增删改查，数据写入 ai.json 并在响应中脱敏。
func handleAccountRoute(w http.ResponseWriter, s *executionState, path string, body map[string]any) bool {
	if path == "accounts/providers" {
		return false
	}
	if !strings.HasPrefix(path, "accounts") {
		return false
	}
	if path == "accounts/counts" {
		s.mu.RLock()
		counts := make(map[string]int)
		for _, account := range s.data.Accounts {
			counts[aiString(account, "provider")]++
		}
		s.mu.RUnlock()
		aiOK(w, counts)
		return true
	}
	if path == "accounts/search" {
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
		page, size := 1, 50
		if value, ok := body["page"].(float64); ok && int(value) > 0 {
			page = int(value)
		}
		if value, ok := body["pageSize"].(float64); ok && int(value) > 0 && int(value) <= 200 {
			size = int(value)
		}
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
	if path == "accounts/models" {
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
	if path == "accounts/models/discover" {
		models, err := discoverAIModels(body)
		if err != nil {
			aiError(w, http.StatusBadGateway, "MODEL_DISCOVERY_FAILED", err.Error())
			return true
		}
		aiOK(w, models)
		return true
	}
	if path == "accounts/verify" {
		result, err := verifyAIAccount(body)
		if err != nil {
			aiError(w, http.StatusBadRequest, "ACCOUNT_VERIFY_FAILED", err.Error())
			return true
		}
		aiOK(w, result)
		return true
	}
	if path == "accounts" || path == "accounts/update" {
		provider, name := aiString(body, "provider"), aiString(body, "name")
		if provider == "" || name == "" {
			aiError(w, http.StatusBadRequest, "ACCOUNT_REQUIRED", "provider 和 name 不能为空")
			return true
		}
		if path == "accounts/update" {
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
	if path == "accounts/delete" {
		aiDelete(w, s, path, body)
		return true
	}
	if strings.HasPrefix(path, "accounts/models/") {
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
		switch path {
		case "accounts/models/create":
			for _, existing := range models {
				if aiString(existing, "id", "name") == modelID {
					s.mu.Unlock()
					aiError(w, http.StatusConflict, "MODEL_EXISTS", "模型已存在")
					return true
				}
			}
			if aiString(modelValue, "name") == "" {
				modelValue["name"] = modelID
			}
			modelValue["recordId"] = aiNewID("model")
			models = append(models, modelValue)
		case "accounts/models/update":
			found := false
			for i, existing := range models {
				if aiString(existing, "id", "name") == modelID || aiID(existing, "recordId") == aiID(body, "recordId") {
					models[i] = modelValue
					found = true
					break
				}
			}
			if !found {
				s.mu.Unlock()
				aiError(w, http.StatusNotFound, "MODEL_NOT_FOUND", "模型不存在")
				return true
			}
		case "accounts/models/delete":
			kept := make([]map[string]any, 0, len(models))
			removed := false
			target := aiID(body, "recordId")
			for _, existing := range models {
				if (target != "" && aiID(existing, "recordId") == target) || (target == "" && aiString(existing, "id", "name") == modelID) {
					removed = true
					continue
				}
				kept = append(kept, existing)
			}
			if !removed {
				s.mu.Unlock()
				aiError(w, http.StatusNotFound, "MODEL_NOT_FOUND", "模型不存在")
				return true
			}
			models = kept
		}
		setAccountModels(account, models)
		account["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
		s.data.Accounts[index] = account
		err := s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			aiError(w, http.StatusInternalServerError, "ACCOUNT_SAVE_FAILED", err.Error())
			return true
		}
		aiOK(w, models)
		return true
	}
	return false
}

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

func handleAIPost(w http.ResponseWriter, s *executionState, path string, body map[string]any) {
	if handleAccountRoute(w, s, path, body) {
		return
	}
	if path == "agents/hermes/chat/sessions" {
		agent := aiID(body, "agentId")
		s.mu.RLock()
		items := append([]map[string]any(nil), s.data.Sessions[agent]...)
		s.mu.RUnlock()
		if items == nil {
			items = make([]map[string]any, 0)
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
		return
	}
	if path == "agents/agent/md/list" {
		id := aiID(body, "agentId")
		s.mu.RLock()
		value := cloneMap(s.data.Configs[path+":"+id])
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
		s := getAIState()
		s.mu.RLock()
		instances := make([]map[string]any, 0, len(s.data.Sandboxes))
		for _, item := range s.data.Sandboxes {
			instances = append(instances, sanitizeAIMap(item))
		}
		s.mu.RUnlock()
		aiOK(w, map[string]any{"status": status, "available": available, "path": path, "instances": instances})
		return
	}
	body, err := aiBody(r)
	if err != nil {
		aiError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	id := aiID(body, "id", "sandboxId", "taskId")
	if id == "" {
		aiError(w, http.StatusBadRequest, "SANDBOX_ID_REQUIRED", "沙盒 ID 不能为空")
		return
	}
	if path == "start" && !available {
		aiError(w, http.StatusServiceUnavailable, "CUBESANDBOX_UNAVAILABLE", "当前主机缺少 KVM，无法启动 MicroVM")
		return
	}
	s := getAIState()
	s.mu.Lock()
	index := -1
	for i, item := range s.data.Sandboxes {
		if aiID(item, "id") == id {
			index = i
			break
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if path == "reconcile" && index < 0 {
		s.mu.Unlock()
		aiError(w, http.StatusNotFound, "SANDBOX_NOT_FOUND", "待对账的沙盒不存在")
		return
	}
	if index < 0 {
		s.data.Sandboxes = append(s.data.Sandboxes, map[string]any{"id": id, "status": "running", "createdAt": now, "updatedAt": now})
		index = len(s.data.Sandboxes) - 1
	}
	instance := s.data.Sandboxes[index]
	switch path {
	case "start":
		instance["status"] = "running"
	case "stop":
		instance["status"] = "stopped"
	case "reconcile":
		if strings.TrimSpace(aiString(body, "status")) != "" {
			instance["status"] = aiString(body, "status")
		}
	}
	instance["updatedAt"] = now
	s.data.Sandboxes[index] = instance
	err = s.saveLocked()
	out := sanitizeAIMap(instance)
	s.mu.Unlock()
	if err != nil {
		aiError(w, http.StatusInternalServerError, "SANDBOX_SAVE_FAILED", err.Error())
		return
	}
	aiOK(w, out)
}

func taskHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/workmesh/tasks/"), "/")
	if r.Method != http.MethodPost {
		aiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "任务接口只允许 POST")
		return
	}
	// 若部署配置了任务令牌，则所有写操作均要求相同的节点凭据；未配置时
	// Provider 仍会因未装配返回 503，不会把请求降级为宿主命令执行。
	if token := strings.TrimSpace(os.Getenv("WORKMESH_TASK_TOKEN")); token != "" && r.Header.Get("X-WorkMesh-Token") != token {
		aiError(w, http.StatusUnauthorized, "TASK_AUTH_REQUIRED", "任务操作需要 X-WorkMesh-Token")
		return
	}
	provider := getTaskProvider()
	if provider == nil {
		aiError(w, http.StatusServiceUnavailable, "TASK_PROVIDER_UNAVAILABLE", "任务隔离 Provider 未配置或 CLI 摘要校验失败")
		return
	}
	if path == "create" {
		var req struct {
			TaskID         string                    `json:"taskId"`
			ImageDigest    string                    `json:"imageDigest"`
			Worktree       string                    `json:"worktree"`
			Entrypoint     []string                  `json:"entrypoint"`
			TimeoutSeconds int                       `json:"timeoutSeconds"`
			RuntimePolicy  taskruntime.RuntimePolicy `json:"runtimePolicy"`
		}
		if err := decodeTaskJSON(w, r, &req); err != nil {
			aiError(w, http.StatusBadRequest, "INVALID_TASK_REQUEST", err.Error())
			return
		}
		timeout := 30 * time.Minute
		if req.TimeoutSeconds > 0 {
			timeout = time.Duration(req.TimeoutSeconds) * time.Second
		}
		if timeout > 30*time.Minute {
			aiError(w, http.StatusBadRequest, "TASK_TIMEOUT_INVALID", "timeoutSeconds 不能超过 1800")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		handle, err := provider.Create(ctx, taskruntime.TaskSpec{TaskID: strings.TrimSpace(req.TaskID), ImageDigest: strings.TrimSpace(req.ImageDigest), Worktree: strings.TrimSpace(req.Worktree), Entrypoint: req.Entrypoint, Timeout: timeout, RuntimePolicy: req.RuntimePolicy})
		cancel()
		if err != nil {
			aiError(w, http.StatusBadRequest, "TASK_CREATE_FAILED", err.Error())
			return
		}
		if err := persistTaskHandle(handle); err != nil {
			aiError(w, http.StatusInternalServerError, "TASK_STATE_SAVE_FAILED", err.Error())
			return
		}
		aiOK(w, handle)
		return
	}
	var req struct {
		TaskID string   `json:"taskId"`
		Argv   []string `json:"argv"`
	}
	if err := decodeTaskJSON(w, r, &req); err != nil {
		aiError(w, http.StatusBadRequest, "INVALID_TASK_REQUEST", err.Error())
		return
	}
	id := strings.TrimSpace(req.TaskID)
	if id == "" {
		aiError(w, http.StatusBadRequest, "TASK_ID_REQUIRED", "taskId 不能为空")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	switch path {
	case "start":
		if err := provider.Start(ctx, id); err != nil {
			taskError(w, "TASK_START_FAILED", err)
			return
		}
		if err := updatePersistedTask(id, "running"); err != nil {
			aiError(w, http.StatusInternalServerError, "TASK_STATE_SAVE_FAILED", err.Error())
			return
		}
		aiOK(w, map[string]string{"taskId": id})
	case "exec":
		result, err := provider.Exec(ctx, id, req.Argv)
		if err != nil {
			taskError(w, "TASK_EXEC_FAILED", err)
			return
		}
		aiOK(w, result)
	case "collect":
		result, err := provider.Collect(ctx, id)
		if err != nil {
			taskError(w, "TASK_COLLECT_FAILED", err)
			return
		}
		if err := updatePersistedTask(id, "completed"); err != nil {
			aiError(w, http.StatusInternalServerError, "TASK_STATE_SAVE_FAILED", err.Error())
			return
		}
		aiOK(w, result)
	case "cancel":
		if err := provider.Cancel(ctx, id); err != nil {
			taskError(w, "TASK_CANCEL_FAILED", err)
			return
		}
		if err := updatePersistedTask(id, "cancelled"); err != nil {
			aiError(w, http.StatusInternalServerError, "TASK_STATE_SAVE_FAILED", err.Error())
			return
		}
		aiOK(w, map[string]string{"taskId": id})
	case "destroy":
		if err := provider.Destroy(ctx, id); err != nil {
			taskError(w, "TASK_DESTROY_FAILED", err)
			return
		}
		if err := updatePersistedTask(id, "destroyed"); err != nil {
			aiError(w, http.StatusInternalServerError, "TASK_STATE_SAVE_FAILED", err.Error())
			return
		}
		aiOK(w, map[string]string{"taskId": id})
	default:
		aiError(w, http.StatusNotFound, "TASK_OPERATION_NOT_FOUND", "未知任务操作: "+path)
	}
}

func decodeTaskJSON(w http.ResponseWriter, r *http.Request, target any) error {
	if r.Body == nil {
		return errors.New("请求体不能为空")
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("请求只能包含一个 JSON 对象")
	}
	return nil
}

func taskError(w http.ResponseWriter, code string, err error) {
	status := http.StatusBadRequest
	if strings.Contains(err.Error(), "不存在") {
		status = http.StatusNotFound
	}
	aiError(w, status, code, err.Error())
}

func persistTaskHandle(handle taskruntime.TaskHandle) error {
	s := getAIState()
	s.mu.Lock()
	item := map[string]any{"taskId": handle.TaskID, "sandboxId": handle.SandboxID, "imageDigest": handle.ImageDigest, "sandboxType": handle.SandboxType, "backend": handle.Backend, "status": string(handle.State), "createdAt": time.Now().UTC().Format(time.RFC3339), "updatedAt": time.Now().UTC().Format(time.RFC3339)}
	s.tasks[handle.TaskID] = item
	upsertPersistentTaskLocked(s, item)
	err := s.saveLocked()
	s.mu.Unlock()
	return err
}

func updatePersistedTask(id, status string) error {
	s := getAIState()
	s.mu.Lock()
	item := s.tasks[id]
	if item == nil {
		item = map[string]any{"taskId": id}
	}
	item["status"] = status
	item["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.tasks[id] = item
	upsertPersistentTaskLocked(s, item)
	err := s.saveLocked()
	s.mu.Unlock()
	return err
}

func upsertPersistentTaskLocked(s *executionState, item map[string]any) {
	id := aiID(item, "taskId", "id")
	for i, existing := range s.data.Tasks {
		if aiID(existing, "taskId", "id") == id {
			s.data.Tasks[i] = cloneMap(item)
			return
		}
	}
	s.data.Tasks = append(s.data.Tasks, cloneMap(item))
}
