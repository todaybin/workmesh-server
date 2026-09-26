// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/service/taskruntime"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// aiPersistentData 是 AI 执行面的小型持久化模型，避免为控制配置常驻数据库连接。
type aiPersistentData struct {
	Accounts             []map[string]any            `json:"accounts"`
	Agents               []map[string]any            `json:"agents"`
	MCP                  []map[string]any            `json:"mcp"`
	Ollama               []map[string]any            `json:"ollama"`
	TensorRT             []map[string]any            `json:"tensorrt"`
	Domains              map[string]map[string]any   `json:"domains"`
	Configs              map[string]map[string]any   `json:"configs"`
	Sessions             map[string][]map[string]any `json:"sessions"`
	Plugins              []map[string]any            `json:"plugins"`
	Skills               []map[string]any            `json:"skills"`
	Sandboxes            []map[string]any            `json:"sandboxes"`
	Tasks                []map[string]any            `json:"tasks"`
	AgentRuntimes        []map[string]any            `json:"agentRuntimes"`
	TeamTasks            []map[string]any            `json:"teamTasks"`
	ArtifactReclaimPlans []map[string]any            `json:"artifactReclaimPlans"`
	DeploymentPlans      []map[string]any            `json:"deploymentPlans"`
	AgentCommands        []map[string]any            `json:"agentCommands"`
	TeamEvents           map[string][]map[string]any `json:"teamEvents"`
	TeamSequences        map[string]int64            `json:"teamSequences"`
}

type executionState struct {
	mu     sync.RWMutex
	path   string
	data   aiPersistentData
	tasks  map[string]map[string]any
	notify chan struct{}
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
	provider, err := taskruntime.NewCapabilityCheckedTaskProvider(backend)
	if err != nil {
		return nil
	}
	taskProviderState.provider = provider
	taskProviderState.command = command
	taskProviderState.digest = digest
	// CLI 后端只允许通过只读 inspect 核验重启前的沙盒句柄；没有 inspect
	// 能力的后端会把句柄标记为 unknown，禁止继续执行或销毁。
	s := getAIState()
	s.mu.RLock()
	handles := make([]taskruntime.TaskHandle, 0, len(s.data.Tasks))
	for _, item := range s.data.Tasks {
		handles = append(handles, taskruntime.TaskHandle{
			TaskID: aiID(item, "taskId", "id"), SandboxID: aiString(item, "sandboxId"), ImageDigest: aiString(item, "imageDigest"),
			SandboxType: aiString(item, "sandboxType"), Backend: aiString(item, "backend"), WorkspaceRef: aiString(item, "workspaceRef"), State: taskruntime.TaskState(aiString(item, "status")),
		})
	}
	s.mu.RUnlock()
	if inspected, inspectErr := provider.RestoreAndInspect(context.Background(), handles); inspectErr == nil {
		for _, handle := range inspected {
			_ = updatePersistedTask(handle.TaskID, string(handle.State))
		}
	}
	return provider
}

func getAIState() *executionState {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = "./data"
	}
	path := filepath.Join(dir, "ai.json")
	aiStateInit.Lock()
	defer aiStateInit.Unlock()
	if aiState.path == path && aiState.tasks != nil {
		return &aiState
	}
	data := aiPersistentData{Domains: make(map[string]map[string]any), Configs: make(map[string]map[string]any), Sessions: make(map[string][]map[string]any), Sandboxes: make([]map[string]any, 0), Tasks: make([]map[string]any, 0), AgentRuntimes: make([]map[string]any, 0), TeamTasks: make([]map[string]any, 0), ArtifactReclaimPlans: make([]map[string]any, 0), DeploymentPlans: make([]map[string]any, 0), AgentCommands: make([]map[string]any, 0), TeamEvents: make(map[string][]map[string]any), TeamSequences: make(map[string]int64)}
	if db := sharedDB(); db != nil {
		// 公共 SQLite 存在时，ai.json 仅作为一次性迁移输入。
		if !loadJSONState("ai_state", &data) {
			if content, err := os.ReadFile(path); err == nil && len(content) > 0 && json.Unmarshal(content, &data) == nil {
				if saveErr := saveJSONState("ai_state", data); saveErr == nil {
					archiveDir := filepath.Join(filepath.Dir(path), "backups")
					if os.MkdirAll(archiveDir, 0o750) == nil {
						archivePath := filepath.Join(archiveDir, "legacy-ai-"+time.Now().UTC().Format("20060102T150405.000000000Z")+".json")
						_ = os.Rename(path, archivePath)
					}
				}
			}
		}
	} else if content, err := os.ReadFile(path); err == nil {
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
	if data.AgentRuntimes == nil {
		data.AgentRuntimes = make([]map[string]any, 0)
	}
	if data.TeamTasks == nil {
		data.TeamTasks = make([]map[string]any, 0)
	}
	if data.ArtifactReclaimPlans == nil {
		data.ArtifactReclaimPlans = make([]map[string]any, 0)
	}
	if data.DeploymentPlans == nil {
		data.DeploymentPlans = make([]map[string]any, 0)
	}
	if data.AgentCommands == nil {
		data.AgentCommands = make([]map[string]any, 0)
	}
	if data.TeamEvents == nil {
		data.TeamEvents = make(map[string][]map[string]any)
	}
	if data.TeamSequences == nil {
		data.TeamSequences = make(map[string]int64)
	}
	tasks := make(map[string]map[string]any, len(data.Tasks))
	for _, item := range data.Tasks {
		if id := aiID(item, "taskId", "id"); id != "" {
			tasks[id] = cloneMap(item)
		}
	}
	aiState = executionState{path: path, data: data, tasks: tasks, notify: make(chan struct{})}
	return &aiState
}

func (s *executionState) saveLocked() error {
	if sharedDB() != nil {
		return saveJSONState("ai_state", s.data)
	}
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
	const maxAIRequestBytes = 2 << 20
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxAIRequestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxAIRequestBytes {
		return nil, errors.New("JSON 请求超过 2MiB 限制")
	}
	var body map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	err = decoder.Decode(&body)
	if errors.Is(err, io.EOF) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("JSON 请求不能包含尾随值")
		}
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
	if localized, ok := w.(*localizedResponseWriter); ok {
		message = localizeErrorMessage(localized.locale, code, message)
	}
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "details": map[string]string{"errCode": code}, "message": message})
}
