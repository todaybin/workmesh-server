// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func registerAIExecutionRoutes(mux *http.ServeMux) {
	registerAgentTeamRoutes(mux)
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
	// 普通 JSON 错误根据 Accept-Language 返回本地化文案；升级流由专用处理器直接接管。
	w = &localizedResponseWriter{ResponseWriter: w, locale: r.Header.Get("Accept-Language")}
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
	handleAIPost(w, r, s, path, body)
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
