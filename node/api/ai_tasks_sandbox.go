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
	"path/filepath"
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

type taskStateReader interface {
	State(string) (taskruntime.TaskState, error)
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
		ProjectID       string                    `json:"projectId"`
		TaskID          string                    `json:"taskId"`
		ImageDigest     string                    `json:"imageDigest"`
		Worktree        string                    `json:"worktree"`
		Entrypoint      []string                  `json:"entrypoint"`
		TimeoutSeconds  int                       `json:"timeoutSeconds"`
		ResourceProfile string                    `json:"resourceProfile"`
		RuntimePolicy   taskruntime.RuntimePolicy `json:"runtimePolicy"`
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
	worktree, err := resolveTaskWorktree(req.ProjectID, req.TaskID, req.Worktree)
	if err != nil {
		aiError(w, http.StatusBadRequest, "TASK_WORKSPACE_INVALID", err.Error())
		return
	}
	if strings.TrimSpace(req.ProjectID) != "" {
		layout, layoutErr := taskruntime.NewWorkspaceLayout(os.Getenv("WORKMESH_AGENT_WORKSPACE_ROOT"), strings.TrimSpace(req.ProjectID), strings.TrimSpace(req.TaskID))
		if layoutErr != nil {
			aiError(w, http.StatusBadRequest, "TASK_WORKSPACE_INVALID", layoutErr.Error())
			return
		}
		if ensureErr := layout.Ensure(r.Context()); ensureErr != nil {
			aiError(w, http.StatusBadRequest, "TASK_WORKSPACE_UNAVAILABLE", ensureErr.Error())
			return
		}
	}
	policy := req.RuntimePolicy
	if strings.TrimSpace(req.ProjectID) != "" {
		policy.WorkspaceRef = strings.TrimSpace(req.ProjectID)
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	handle, err := provider.Create(ctx, taskruntime.TaskSpec{TaskID: strings.TrimSpace(req.TaskID), ImageDigest: strings.TrimSpace(req.ImageDigest), Worktree: worktree, Entrypoint: req.Entrypoint, Timeout: timeout, ResourceProfile: req.ResourceProfile, RuntimePolicy: policy})
	cancel()
	if err != nil {
		var capabilityErr *taskruntime.SandboxCapabilityError
		if errors.As(err, &capabilityErr) {
			aiError(w, http.StatusServiceUnavailable, "TASK_PROVIDER_UNAVAILABLE", err.Error())
			return
		}
		aiError(w, http.StatusBadRequest, "TASK_CREATE_FAILED", err.Error())
		return
	}
	if err := persistTaskHandle(handle); err != nil {
		// 控制面未持久化成功时立即销毁已创建的沙盒，避免出现仅存在于
		// 进程内、重启后无法回收的孤儿运行时句柄。
		_ = provider.Destroy(context.Background(), handle.TaskID)
		aiError(w, http.StatusInternalServerError, "TASK_STATE_SAVE_FAILED", err.Error())
		return
	}
	aiOK(w, handle)
}

// resolveTaskWorktree 为项目模式派生服务端工作区；未提供 projectId 时保留旧调用的显式路径校验。
func resolveTaskWorktree(projectID, taskID, legacyWorktree string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	taskID = strings.TrimSpace(taskID)
	legacyWorktree = strings.TrimSpace(legacyWorktree)
	if projectID == "" {
		return legacyWorktree, nil
	}
	if validTeamID(projectID) == "" || projectID == "." || projectID == ".." {
		return "", errors.New("projectId 必须是安全项目标识")
	}
	if validTeamID(taskID) == "" || taskID == "." || taskID == ".." {
		return "", errors.New("taskId 必须是安全任务标识")
	}
	if legacyWorktree != "" {
		return "", errors.New("projectId 模式不接受客户端 worktree")
	}
	root := strings.TrimSpace(os.Getenv("WORKMESH_AGENT_WORKSPACE_ROOT"))
	if root == "" || !filepath.IsAbs(root) {
		return "", errors.New("项目模式要求配置绝对路径 WORKMESH_AGENT_WORKSPACE_ROOT")
	}
	root = filepath.Clean(root)
	target := filepath.Join(root, projectID, "worktrees", taskID)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("派生工作区越出 workspace root")
	}
	return target, nil
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
	case "recover":
		recoverer, ok := provider.(interface {
			RecoverInspect(context.Context, string) (taskruntime.TaskState, error)
		})
		if !ok {
			aiError(w, http.StatusServiceUnavailable, "TASK_RECOVERY_UNAVAILABLE", "任务 Provider 不支持只读状态核验")
			return
		}
		state, err := recoverer.RecoverInspect(ctx, id)
		if err != nil {
			taskError(w, "TASK_RECOVERY_FAILED", err)
			return
		}
		if err := updatePersistedTask(id, string(state)); err != nil {
			aiError(w, http.StatusInternalServerError, "TASK_RECOVERY_SAVE_FAILED", err.Error())
			return
		}
		aiOK(w, map[string]string{"taskId": id, "status": string(state)})
	case "sync":
		stateStore := getAIState()
		stateStore.mu.RLock()
		needsSync, _ := stateStore.tasks[id]["stateSyncRequired"].(bool)
		stateStore.mu.RUnlock()
		if !needsSync {
			aiError(w, http.StatusConflict, "TASK_STATE_SYNC_NOT_REQUIRED", "当前进程没有该任务的状态写盘失败记录；不能用本地缓存状态替代 Sandbox 后端对账")
			return
		}
		reader, ok := provider.(taskStateReader)
		if !ok {
			aiError(w, http.StatusServiceUnavailable, "TASK_STATE_SYNC_UNAVAILABLE", "任务 Provider 不支持状态对账")
			return
		}
		state, err := reader.State(id)
		if err != nil {
			taskError(w, "TASK_STATE_SYNC_FAILED", err)
			return
		}
		if err := updatePersistedTask(id, string(state)); err != nil {
			aiError(w, http.StatusInternalServerError, "TASK_STATE_SYNC_FAILED", err.Error())
			return
		}
		aiOK(w, map[string]string{"taskId": id, "status": string(state)})
	case "start":
		acquired, ok := acquireManagedSlot(managedRuntimeSlots.aiJobs, id, "aiJobs", nodeRuntimeLimits.aiJobs)
		if !ok {
			aiError(w, http.StatusTooManyRequests, "AI_JOB_LIMIT_REACHED", "AI 任务并发数已达到上限")
			return
		}
		if err := provider.Start(ctx, id); err != nil {
			if acquired {
				releaseManagedSlot(managedRuntimeSlots.aiJobs, id)
			}
			taskError(w, "TASK_START_FAILED", err)
			return
		}
		if err := updatePersistedTask(id, "running"); err != nil {
			// 外部启动已经完成，状态写盘失败时优先回收运行时；若回收也失败，
			// 必须保留资源租约并把内存状态标记为 running，避免重试造成重复启动。
			if cancelErr := provider.Cancel(ctx, id); cancelErr == nil {
				releaseManagedSlot(managedRuntimeSlots.aiJobs, id)
				_ = updateTaskStateAfterSaveFailure(id, "cancelled")
			} else {
				markTaskStateUnsynced(id, "running", cancelErr)
			}
			aiError(w, http.StatusInternalServerError, "TASK_STATE_SAVE_FAILED", taskStateSaveFailureMessage(err))
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
		status := string(taskruntime.TaskCompleted)
		if reader, ok := provider.(taskStateReader); ok {
			state, stateErr := reader.State(id)
			if stateErr != nil {
				aiError(w, http.StatusInternalServerError, "TASK_STATE_READ_FAILED", stateErr.Error())
				return
			}
			status = string(state)
		}
		if err := updatePersistedTask(id, status); err != nil {
			// collect 已经取得终态结果，资源槽位必须释放；保留内存中的真实
			// 终态并记录未同步标记，后续对账可以重试写盘。
			releaseManagedSlot(managedRuntimeSlots.aiJobs, id)
			markTaskStateUnsynced(id, status, err)
			aiError(w, http.StatusInternalServerError, "TASK_STATE_SAVE_FAILED", taskStateSaveFailureMessage(err))
			return
		}
		releaseManagedSlot(managedRuntimeSlots.aiJobs, id)
		aiOK(w, result)
	case "cancel":
		if err := provider.Cancel(ctx, id); err != nil {
			taskError(w, "TASK_CANCEL_FAILED", err)
			return
		}
		releaseManagedSlot(managedRuntimeSlots.aiJobs, id)
		if err := updatePersistedTask(id, "cancelled"); err != nil {
			// 取消动作已完成，槽位不应因控制面写盘失败而继续占用。
			markTaskStateUnsynced(id, "cancelled", err)
			aiError(w, http.StatusInternalServerError, "TASK_STATE_SAVE_FAILED", taskStateSaveFailureMessage(err))
			return
		}
		aiOK(w, map[string]string{"taskId": id})
	case "destroy":
		if err := provider.Destroy(ctx, id); err != nil {
			var cleanupErr *taskruntime.WorkspaceCleanupError
			if errors.As(err, &cleanupErr) {
				_ = updatePersistedTask(id, "awaiting_human")
				aiError(w, http.StatusConflict, "TASK_CLEANUP_REQUIRED", err.Error())
				return
			}
			taskError(w, "TASK_DESTROY_FAILED", err)
			return
		}
		releaseManagedSlot(managedRuntimeSlots.aiJobs, id)
		if err := updatePersistedTask(id, "destroyed"); err != nil {
			// Sandbox 和临时目录均已清理，释放槽位并保留 destroyed 事实；
			// 失败只影响控制面重启后的对账，不应重新占用运行资源。
			markTaskStateUnsynced(id, "destroyed", err)
			aiError(w, http.StatusInternalServerError, "TASK_STATE_SAVE_FAILED", taskStateSaveFailureMessage(err))
			return
		}
		aiOK(w, map[string]string{"taskId": id})
	default:
		aiError(w, http.StatusNotFound, "TASK_OPERATION_NOT_FOUND", "未知任务操作: "+path)
	}
}

// markTaskStateUnsynced 在外部动作已完成而控制面写盘失败时保留真实状态。
// updatePersistedTask 会在失败时回滚内存快照，这里再应用已确认的外部终态，
// 使后续重试和资源对账不会把任务误判为仍在运行。
func markTaskStateUnsynced(id, status string, cause error) {
	s := getAIState()
	s.mu.Lock()
	item := cloneMap(s.tasks[id])
	if item == nil {
		item = map[string]any{"taskId": id}
	}
	item["status"] = status
	item["stateSyncRequired"] = true
	if cause != nil {
		item["stateSyncError"] = cause.Error()
	}
	item["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.tasks[id] = item
	upsertPersistentTaskLocked(s, item)
	s.mu.Unlock()
}

// updateTaskStateAfterSaveFailure 尝试在回收动作成功后补写最终状态；失败时
// 不再覆盖真实内存状态，等待后续控制面对账。
func updateTaskStateAfterSaveFailure(id, status string) error {
	if err := updatePersistedTask(id, status); err != nil {
		markTaskStateUnsynced(id, status, err)
		return err
	}
	return nil
}

func taskStateSaveFailureMessage(err error) string {
	return "任务外部动作已完成，但状态尚未可靠写入；请稍后重试或执行任务对账: " + err.Error()
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
	previousItem, hadPreviousItem := s.tasks[handle.TaskID]
	previousTasks := append([]map[string]any(nil), s.data.Tasks...)
	item := map[string]any{"taskId": handle.TaskID, "sandboxId": handle.SandboxID, "imageDigest": handle.ImageDigest, "sandboxType": handle.SandboxType, "backend": handle.Backend, "workspaceRef": handle.WorkspaceRef, "status": string(handle.State), "createdAt": time.Now().UTC().Format(time.RFC3339), "updatedAt": time.Now().UTC().Format(time.RFC3339)}
	s.tasks[handle.TaskID] = item
	upsertPersistentTaskLocked(s, item)
	err := s.saveLocked()
	if err != nil {
		if hadPreviousItem {
			s.tasks[handle.TaskID] = previousItem
		} else {
			delete(s.tasks, handle.TaskID)
		}
		s.data.Tasks = previousTasks
	}
	s.mu.Unlock()
	return err
}

func updatePersistedTask(id, status string) error {
	s := getAIState()
	s.mu.Lock()
	previousItem, hadPreviousItem := s.tasks[id]
	previousTasks := append([]map[string]any(nil), s.data.Tasks...)
	item := cloneMap(previousItem)
	if item == nil {
		item = map[string]any{"taskId": id}
	}
	item["status"] = status
	delete(item, "stateSyncRequired")
	delete(item, "stateSyncError")
	item["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	s.tasks[id] = item
	upsertPersistentTaskLocked(s, item)
	err := s.saveLocked()
	if err != nil {
		if hadPreviousItem {
			s.tasks[id] = previousItem
		} else {
			delete(s.tasks, id)
		}
		s.data.Tasks = previousTasks
	}
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
