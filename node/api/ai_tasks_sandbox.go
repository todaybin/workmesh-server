// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/service/taskruntime"
)

// taskProvider 定义 HTTP 层所需的最小任务运行时接口，便于隔离协议与实现。
type taskProvider interface {
	Create(context.Context, taskruntime.TaskSpec) (taskruntime.TaskHandle, error)
	Start(context.Context, string) error
	Exec(context.Context, string, []string) (taskruntime.TaskExecResult, error)
	Cancel(context.Context, string) error
	Collect(context.Context, string) (taskruntime.TaskExecResult, error)
	Destroy(context.Context, string) error
}

// sandboxHandler 返回沙盒能力并执行已存在实例的生命周期操作。
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

// taskHandler 通过隔离 Provider 执行任务创建、运行、收集、取消和销毁。
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
		createSandboxTask(w, r, provider)
		return
	}
	operateSandboxTask(w, r, provider, path)
}

// createSandboxTask 校验创建参数，调用隔离 Provider 并保存任务句柄。
func createSandboxTask(w http.ResponseWriter, r *http.Request, provider taskProvider) {
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
}

// operateSandboxTask 执行任务启动、命令、收集、取消和销毁操作。
func operateSandboxTask(w http.ResponseWriter, r *http.Request, provider taskProvider, path string) {
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
