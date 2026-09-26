// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

const (
	teamLeaseDuration            = 15 * time.Second
	teamEventLimit               = 2000
	teamPendingCommandLimit      = 1000
	teamTaskLimit                = 10000
	teamCommandWaitMax           = 10 * time.Second
	teamCommandVisibility        = 30 * time.Second
	teamCommandHistoryLimit      = 2000
	teamArtifactReclaimPlanLimit = 100
	teamDeploymentPlanLimit      = 100
	teamDefaultResourceUnits     = 4
	teamMaxResourceUnits         = 512
)

var teamResourceProfiles = map[string]struct{}{
	"small":  {},
	"medium": {},
	"large":  {},
}

var teamIDCounter atomic.Uint64

// registerAgentTeamRoutes 注册项目 Agent 控制面和 Desktop 事件流接口。
func registerAgentTeamRoutes(mux *http.ServeMux) {
	for _, pattern := range []string{
		"POST /api/v2/agent-runtime/register",
		"POST /api/v2/agent-runtime/heartbeat",
		"POST /api/v2/agent-runtime/unregister",
		"POST /api/v2/agent-runtime/events",
		"POST /api/v2/agent-runtime/artifacts/init",
		"PUT /api/v2/agent-runtime/artifacts/{artifactId}/chunks",
		"GET /api/v2/agent-runtime/commands",
		"POST /api/v2/agent-runtime/commands/ack",
	} {
		mux.HandleFunc(pattern, handleAgentRuntimeRoute)
	}
	mux.HandleFunc("GET /api/v2/projects/{projectId}/agent-runtime", handleProjectRuntimeStatus)
	mux.HandleFunc("POST /api/v2/projects/{projectId}/tasks", handleProjectTaskCreate)
	mux.HandleFunc("GET /api/v2/projects/{projectId}/tasks", handleProjectTaskList)
	mux.HandleFunc("GET /api/v2/projects/{projectId}/events/stream", handleProjectEventStream)
	mux.HandleFunc("GET /api/v2/dev/tasks/{taskId}", handleDevTaskGet)
	mux.HandleFunc("GET /api/v2/dev/tasks/{taskId}/events", handleDevTaskEvents)
	mux.HandleFunc("GET /api/v2/dev/tasks/{taskId}/artifacts", handleDevTaskArtifacts)
	mux.HandleFunc("GET /api/v2/dev/tasks/{taskId}/evidence", handleDevTaskEvidence)
	mux.HandleFunc("GET /api/v2/dev/tasks/{taskId}/artifacts/verify", handleDevTaskArtifactVerify)
	mux.HandleFunc("GET /api/v2/dev/tasks/{taskId}/artifacts/reconcile", handleDevTaskArtifactReconcile)
	mux.HandleFunc("GET /api/v2/projects/{projectId}/artifacts/reconcile", handleProjectArtifactReconcile)
	mux.HandleFunc("POST /api/v2/projects/{projectId}/artifacts/reclaim-plan", handleProjectArtifactReclaimPlan)
	mux.HandleFunc("GET /api/v2/projects/{projectId}/artifacts/reclaim-plans", handleProjectArtifactReclaimPlanList)
	mux.HandleFunc("GET /api/v2/projects/{projectId}/artifacts/reclaim-plans/{planId}", handleProjectArtifactReclaimPlanGet)
	mux.HandleFunc("POST /api/v2/projects/{projectId}/deployments/dry-run", handleProjectDeploymentDryRun)
	mux.HandleFunc("GET /api/v2/projects/{projectId}/deployments/plans", handleProjectDeploymentPlanList)
	mux.HandleFunc("GET /api/v2/projects/{projectId}/deployments/plans/{planId}", handleProjectDeploymentPlanGet)
	mux.HandleFunc("GET /api/v2/dev/tasks/{taskId}/artifacts/{artifactId}", handleDevTaskArtifactDownload)
	for _, action := range []string{"input", "cancel", "retry", "complete", "approve", "reject"} {
		mux.HandleFunc("POST /api/v2/dev/tasks/{taskId}/"+action, handleDevTaskAction)
	}
}

// IsAgentRuntimeRequest 标识由 Agent Service 使用独立 runtime 令牌认证的请求。
// 外层 HTTP 会话中间件只对精确 runtime 入口放行，真正的令牌校验仍在本文件完成。
func IsAgentRuntimeRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	switch r.Method + " " + r.URL.Path {
	case "POST /api/v2/agent-runtime/register", "POST /api/v2/agent-runtime/heartbeat", "POST /api/v2/agent-runtime/unregister", "POST /api/v2/agent-runtime/events", "POST /api/v2/agent-runtime/artifacts/init", "GET /api/v2/agent-runtime/commands", "POST /api/v2/agent-runtime/commands/ack":
		return true
	default:
		return r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v2/agent-runtime/artifacts/") && strings.HasSuffix(r.URL.Path, "/chunks")
	}
}

// handleAgentRuntimeRoute 处理 Agent Service 自注册、心跳、注销和上行事件。
func handleAgentRuntimeRoute(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/v2/agent-runtime/artifacts/") {
		handleAgentRuntimeArtifactRoute(w, r, getAIState())
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/v2/agent-runtime/commands" {
		if !requireAgentRuntimeAuth(w, r, r.URL.Query().Get("projectId")) {
			return
		}
		handleAgentRuntimeCommands(w, r, getAIState())
		return
	}
	body, err := aiBody(r)
	if err != nil {
		teamError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if !requireAgentRuntimeAuth(w, r, aiString(body, "projectId")) {
		return
	}
	s := getAIState()
	switch r.URL.Path {
	case "/api/v2/agent-runtime/register":
		handleAgentRuntimeRegister(w, s, body)
	case "/api/v2/agent-runtime/heartbeat":
		handleAgentRuntimeHeartbeat(w, s, body)
	case "/api/v2/agent-runtime/unregister":
		handleAgentRuntimeUnregister(w, s, body)
	case "/api/v2/agent-runtime/events":
		handleAgentRuntimeEvents(w, s, body)
	case "/api/v2/agent-runtime/commands/ack":
		handleAgentRuntimeCommandAck(w, s, body)
	default:
		teamError(w, http.StatusNotFound, "AGENT_ROUTE_NOT_FOUND", "Agent 路由不存在")
	}
}

func requireAgentRuntimeAuth(w http.ResponseWriter, r *http.Request, projectID string) bool {
	master := strings.TrimSpace(os.Getenv("WORKMESH_AGENT_RUNTIME_TOKEN"))
	provided := strings.TrimSpace(r.Header.Get("X-WorkMesh-Agent-Token"))
	if master == "" {
		teamError(w, http.StatusServiceUnavailable, "AGENT_RUNTIME_AUTH_UNCONFIGURED", "未配置 Agent runtime 认证令牌")
		return false
	}
	projectID = validTeamID(projectID)
	if projectID == "" || provided == "" || subtle.ConstantTimeCompare([]byte(deriveAgentProjectToken(master, projectID)), []byte(provided)) != 1 {
		teamError(w, http.StatusUnauthorized, "AGENT_RUNTIME_AUTH_REQUIRED", "Agent runtime 认证失败")
		return false
	}
	return true
}

func deriveAgentProjectToken(master, projectID string) string {
	mac := hmac.New(sha256.New, []byte(master))
	_, _ = mac.Write([]byte("workmesh.agent.v1:" + projectID))
	return hex.EncodeToString(mac.Sum(nil))
}

func handleAgentRuntimeRegister(w http.ResponseWriter, s *executionState, body map[string]any) {
	projectID, runtimeID := validTeamID(aiString(body, "projectId")), validTeamID(aiString(body, "runtimeId"))
	instanceID, memberID := validTeamID(aiString(body, "instanceId")), validTeamID(aiString(body, "memberId"))
	if projectID == "" || runtimeID == "" || instanceID == "" || memberID == "" {
		teamError(w, http.StatusBadRequest, "AGENT_RUNTIME_FIELDS_REQUIRED", "projectId、runtimeId、instanceId 和 memberId 不能为空且只能包含安全标识字符")
		return
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	before := snapshotTeamStateLocked(s)
	var current map[string]any
	for _, item := range s.data.AgentRuntimes {
		if aiString(item, "projectId") == projectID && aiString(item, "status") != "offline" {
			current = item
			break
		}
	}
	if current != nil && aiString(current, "runtimeId") != runtimeID && parseTeamTime(aiString(current, "leaseExpiresAt")).After(now) {
		teamError(w, http.StatusConflict, "AGENT_RUNTIME_ALREADY_ACTIVE", "同一项目已有活动 Agent runtime")
		return
	}
	fencing := teamNewID("fence")
	leaseID := teamNewID("lease")
	item := map[string]any{
		"projectId": projectID, "runtimeId": runtimeID, "instanceId": instanceID, "memberId": memberID,
		"deviceId": validTeamID(aiString(body, "deviceId")), "status": "online", "fencingToken": fencing,
		"leaseId": leaseID, "activeSessions": boundedInt(body["activeSessions"], 0, 10000),
		"activeTasks": boundedInt(body["activeTasks"], 0, 10000), "registeredAt": now.Format(time.RFC3339Nano),
		"lastSeenAt": now.Format(time.RFC3339Nano), "leaseExpiresAt": now.Add(teamLeaseDuration).Format(time.RFC3339Nano),
	}
	updated := make([]map[string]any, 0, len(s.data.AgentRuntimes)+1)
	for _, old := range s.data.AgentRuntimes {
		if aiString(old, "projectId") == projectID && aiString(old, "runtimeId") != runtimeID {
			old = cloneMap(old)
			old["status"] = "fenced"
		}
		if aiString(old, "runtimeId") != runtimeID || aiString(old, "projectId") != projectID {
			updated = append(updated, old)
		}
	}
	updated = append(updated, item)
	s.data.AgentRuntimes = updated
	for i, command := range s.data.AgentCommands {
		if aiString(command, "projectId") != projectID || aiString(command, "status") == "acked" || aiString(command, "status") == "superseded" {
			continue
		}
		copy := cloneMap(command)
		copy["runtimeId"], copy["status"] = runtimeID, "queued"
		s.data.AgentCommands[i] = copy
	}
	for _, task := range s.data.TeamTasks {
		if aiString(task, "projectId") != projectID || aiString(task, "status") != "accepted" {
			continue
		}
		found := false
		for _, command := range s.data.AgentCommands {
			if aiString(command, "taskId") == aiString(task, "taskId") && aiString(command, "kind") == "task.start" && aiString(command, "status") != "acked" && aiString(command, "status") != "superseded" {
				found = true
				break
			}
		}
		if !found {
			enqueueTeamCommandLocked(s, task)
		}
	}
	appendTeamEventLocked(s, projectID, "runtime.online", "", runtimeID, map[string]any{"memberId": memberID})
	if err := saveTeamStateLocked(s, before); err != nil {
		teamError(w, http.StatusInternalServerError, "AGENT_RUNTIME_SAVE_FAILED", err.Error())
		return
	}
	aiOK(w, map[string]any{"runtime": sanitizeAIMap(item), "accepted": true})
}

func teamDependencyIDs(values ...any) ([]string, string) {
	var raw []any
	for _, value := range values {
		if value == nil {
			continue
		}
		switch item := value.(type) {
		case []any:
			if len(raw) == 0 {
				raw = item
			}
		case []string:
			if len(raw) == 0 {
				raw = make([]any, len(item))
				for i := range item {
					raw[i] = item[i]
				}
			}
		default:
			return nil, "依赖任务必须是数组"
		}
	}
	if len(raw) > 32 {
		return nil, "依赖任务最多 32 个"
	}
	result := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		id := validTeamID(fmt.Sprint(value))
		if id == "" {
			return nil, "依赖任务 ID 无效"
		}
		if _, exists := seen[id]; exists {
			return nil, "依赖任务不能重复"
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, ""
}

func validateTeamDependenciesLocked(s *executionState, projectID, taskID string, dependencies []string) string {
	for _, dependencyID := range dependencies {
		if dependencyID == taskID {
			return "任务不能依赖自身"
		}
		dependency := findTeamTaskLocked(s, dependencyID)
		if dependency == nil || aiString(dependency, "projectId") != projectID {
			return "依赖任务不存在或不属于当前项目"
		}
	}
	return ""
}

func teamDependencyStatusLocked(s *executionState, dependencies []string) string {
	for _, dependencyID := range dependencies {
		dependency := findTeamTaskLocked(s, dependencyID)
		if dependency == nil || aiString(dependency, "status") == "failed" || aiString(dependency, "status") == "cancelled" || aiString(dependency, "status") == "blocked" {
			return "blocked"
		}
		if aiString(dependency, "status") != "completed" {
			return "waiting_dependency"
		}
	}
	return "accepted"
}

func activateReadyTeamTasksLocked(s *executionState, projectID string) {
	for changed := true; changed; {
		changed = false
		for i, task := range s.data.TeamTasks {
			if aiString(task, "projectId") != projectID || (aiString(task, "status") != "waiting_dependency" && aiString(task, "status") != "waiting_resource") {
				continue
			}
			dependencies := teamStringSlice(task["dependencyTaskIds"])
			status := teamDependencyStatusLocked(s, dependencies)
			if status == "waiting_dependency" {
				continue
			}
			if status == "accepted" && !teamResourceAvailableLocked(s, projectID, aiString(task, "resourceProfile"), aiString(task, "taskId")) {
				if aiString(task, "status") != "waiting_resource" {
					updated := cloneMap(task)
					updated["status"] = "waiting_resource"
					updated["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
					s.data.TeamTasks[i] = updated
					appendTeamEventLocked(s, projectID, "task.waiting_resource", aiString(updated, "taskId"), "", map[string]any{"resourceProfile": updated["resourceProfile"], "reason": "项目资源预算已用尽"})
					changed = true
				}
				continue
			}
			updated := cloneMap(task)
			updated["status"] = status
			updated["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
			s.data.TeamTasks[i] = updated
			changed = true
			if status == "accepted" {
				appendTeamEventLocked(s, projectID, "task.ready", aiString(updated, "taskId"), "", map[string]any{"dependencyTaskIds": dependencies})
				enqueueTeamCommandLocked(s, updated)
				continue
			}
			appendTeamEventLocked(s, projectID, "task.blocked", aiString(updated, "taskId"), "", map[string]any{"reason": "依赖任务未成功完成", "dependencyTaskIds": dependencies})
		}
	}
}

func teamResourceProfileUnits(profile string) int {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "large":
		return 4
	case "medium":
		return 2
	default:
		return 1
	}
}

func teamResourceBudget() int {
	value := strings.TrimSpace(os.Getenv("WORKMESH_AGENT_MAX_RESOURCE_UNITS"))
	if value == "" {
		return teamDefaultResourceUnits
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 || parsed > teamMaxResourceUnits {
		return teamDefaultResourceUnits
	}
	return parsed
}

// teamResourceAvailableLocked 在 Server 控制面预留项目资源，避免客户端堆积超过 Relay 预算的任务。
func teamResourceAvailableLocked(s *executionState, projectID, profile, excludeTaskID string) bool {
	requested := teamResourceProfileUnits(profile)
	used := 0
	for _, task := range s.data.TeamTasks {
		if aiString(task, "projectId") != projectID || aiString(task, "taskId") == excludeTaskID {
			continue
		}
		switch aiString(task, "status") {
		case "accepted", "running", "waiting_input":
			used += teamResourceProfileUnits(aiString(task, "resourceProfile"))
		}
	}
	return used+requested <= teamResourceBudget()
}

func teamStringSlice(value any) []string {
	values, ok := value.([]any)
	if !ok {
		if strings, isStrings := value.([]string); isStrings {
			return append([]string(nil), strings...)
		}
		return nil
	}
	result := make([]string, 0, len(values))
	for _, item := range values {
		if id := validTeamID(fmt.Sprint(item)); id != "" {
			result = append(result, id)
		}
	}
	return result
}

func handleAgentRuntimeHeartbeat(w http.ResponseWriter, s *executionState, body map[string]any) {
	projectID, runtimeID, fencing := validTeamID(aiString(body, "projectId")), validTeamID(aiString(body, "runtimeId")), strings.TrimSpace(aiString(body, "fencingToken"))
	if projectID == "" || runtimeID == "" || fencing == "" {
		teamError(w, http.StatusBadRequest, "AGENT_HEARTBEAT_FIELDS_REQUIRED", "projectId、runtimeId 和 fencingToken 不能为空")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	before := snapshotTeamStateLocked(s)
	item := findRuntimeLocked(s, projectID, runtimeID)
	if item == nil || aiString(item, "fencingToken") != fencing || aiString(item, "status") == "fenced" {
		teamError(w, http.StatusConflict, "AGENT_RUNTIME_FENCED", "Agent runtime 租约已失效")
		return
	}
	now := time.Now().UTC()
	item = cloneMap(item)
	item["status"] = "busy"
	if boundedInt(body["activeTasks"], 0, 10000) == 0 && boundedInt(body["activeSessions"], 0, 10000) == 0 {
		item["status"] = "online"
	}
	item["activeSessions"] = boundedInt(body["activeSessions"], 0, 10000)
	item["activeTasks"] = boundedInt(body["activeTasks"], 0, 10000)
	item["lastSeenAt"] = now.Format(time.RFC3339Nano)
	item["leaseExpiresAt"] = now.Add(teamLeaseDuration).Format(time.RFC3339Nano)
	replaceRuntimeLocked(s, item)
	if err := saveTeamStateLocked(s, before); err != nil {
		teamError(w, http.StatusInternalServerError, "AGENT_RUNTIME_SAVE_FAILED", err.Error())
		return
	}
	aiOK(w, map[string]any{"runtime": sanitizeAIMap(item), "accepted": true})
}

func handleAgentRuntimeUnregister(w http.ResponseWriter, s *executionState, body map[string]any) {
	projectID, runtimeID, fencing := validTeamID(aiString(body, "projectId")), validTeamID(aiString(body, "runtimeId")), strings.TrimSpace(aiString(body, "fencingToken"))
	s.mu.Lock()
	defer s.mu.Unlock()
	before := snapshotTeamStateLocked(s)
	item := findRuntimeLocked(s, projectID, runtimeID)
	if item == nil || aiString(item, "fencingToken") != fencing {
		teamError(w, http.StatusConflict, "AGENT_RUNTIME_FENCED", "Agent runtime 租约已失效")
		return
	}
	item = cloneMap(item)
	item["status"], item["lastSeenAt"], item["leaseExpiresAt"] = "offline", time.Now().UTC().Format(time.RFC3339Nano), ""
	replaceRuntimeLocked(s, item)
	appendTeamEventLocked(s, projectID, "runtime.offline", "", runtimeID, nil)
	if err := saveTeamStateLocked(s, before); err != nil {
		teamError(w, http.StatusInternalServerError, "AGENT_RUNTIME_SAVE_FAILED", err.Error())
		return
	}
	aiOK(w, map[string]any{"accepted": true})
}

func handleAgentRuntimeEvents(w http.ResponseWriter, s *executionState, body map[string]any) {
	projectID, runtimeID, fencing := validTeamID(aiString(body, "projectId")), validTeamID(aiString(body, "runtimeId")), strings.TrimSpace(aiString(body, "fencingToken"))
	raw, ok := body["events"].([]any)
	if projectID == "" || runtimeID == "" || fencing == "" || !ok || len(raw) == 0 || len(raw) > 100 {
		teamError(w, http.StatusBadRequest, "AGENT_EVENTS_INVALID", "projectId、runtimeId、fencingToken 和 1-100 条 events 必须同时提供")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	before := snapshotTeamStateLocked(s)
	runtimeItem := findRuntimeLocked(s, projectID, runtimeID)
	if runtimeItem == nil || aiString(runtimeItem, "fencingToken") != fencing || aiString(runtimeItem, "status") == "fenced" || parseTeamTime(aiString(runtimeItem, "leaseExpiresAt")).Before(time.Now().UTC()) {
		teamError(w, http.StatusConflict, "AGENT_RUNTIME_FENCED", "Agent runtime 租约已失效")
		return
	}
	last := boundedInt64(runtimeItem["lastSequence"], 0, 1<<60)
	expected := last
	for _, value := range raw {
		event, valid := value.(map[string]any)
		if !valid {
			teamError(w, http.StatusBadRequest, "AGENT_EVENT_INVALID", "事件必须是对象")
			return
		}
		sequence := boundedInt64(event["sequence"], 0, 1<<60)
		typ := validEventType(aiString(event, "type", "eventType"))
		if sequence <= 0 || typ == "" {
			teamError(w, http.StatusBadRequest, "AGENT_EVENT_INVALID", "事件必须提供正数 sequence 和合法 type")
			return
		}
		if sequence <= last {
			continue
		}
		if sequence != expected+1 {
			teamError(w, http.StatusConflict, "AGENT_EVENT_SEQUENCE_GAP", "Agent 事件 sequence 必须连续")
			return
		}
		taskID := aiString(event, "taskId")
		if taskID != "" {
			task := findTeamTaskLocked(s, validTeamID(taskID))
			if task == nil || aiString(task, "projectId") != projectID {
				teamError(w, http.StatusBadRequest, "AGENT_EVENT_TASK_INVALID", "事件 taskId 不属于当前项目")
				return
			}
		}
		if rawAttempt, supplied := event["attempt"]; supplied {
			attempt, valid := rawAttempt.(float64)
			if !valid || attempt < 1 || attempt > 1_000_000 || float64(int64(attempt)) != attempt {
				teamError(w, http.StatusBadRequest, "AGENT_EVENT_ATTEMPT_INVALID", "事件 attempt 必须是正整数")
				return
			}
		}
		if _, err := normalizeTeamCollaborationEvent(typ, event["payload"]); err != "" {
			teamError(w, http.StatusBadRequest, "AGENT_EVENT_PAYLOAD_INVALID", err)
			return
		}
		expected = sequence
	}
	accepted := int64(0)
	for _, value := range raw {
		event, ok := value.(map[string]any)
		if !ok {
			teamError(w, http.StatusBadRequest, "AGENT_EVENT_INVALID", "事件必须是对象")
			return
		}
		sequence := boundedInt64(event["sequence"], 0, 1<<60)
		typ := validEventType(aiString(event, "type", "eventType"))
		if sequence <= last {
			continue
		}
		payload, _ := normalizeTeamCollaborationEvent(typ, event["payload"])
		taskID := validTeamID(aiString(event, "taskId"))
		attempt := boundedInt(event["attempt"], 1, 1_000_000)
		stored := appendTeamEventLocked(s, projectID, typ, taskID, runtimeID, payload)
		if taskID != "" {
			stored["attempt"] = attempt
		}
		task := findTeamTaskLocked(s, taskID)
		stale := task != nil && attempt != teamTaskAttempt(task)
		if stale {
			stored["staleAttempt"] = true
		} else {
			updateTeamTaskFromEventLocked(s, projectID, taskID, typ, payload)
			applyTeamCollaborationEventLocked(s, projectID, taskID, typ, payload)
		}
		if !stale && (typ == "task.completed" || typ == "task.failed" || typ == "task.cancelled" || typ == "task.blocked") {
			activateReadyTeamTasksLocked(s, projectID)
		}
		last, accepted = sequence, accepted+1
	}
	updated := cloneMap(runtimeItem)
	updated["lastSequence"], updated["lastSeenAt"], updated["leaseExpiresAt"] = last, time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Add(teamLeaseDuration).Format(time.RFC3339Nano)
	replaceRuntimeLocked(s, updated)
	if err := saveTeamStateLocked(s, before); err != nil {
		teamError(w, http.StatusInternalServerError, "AGENT_EVENT_SAVE_FAILED", err.Error())
		return
	}
	aiOK(w, map[string]any{"accepted": accepted, "acceptedThrough": last})
}

func handleProjectRuntimeStatus(w http.ResponseWriter, r *http.Request) {
	projectID := validTeamID(r.PathValue("projectId"))
	if projectID == "" {
		teamError(w, http.StatusBadRequest, "PROJECT_ID_INVALID", "projectId 无效")
		return
	}
	s := getAIState()
	s.mu.Lock()
	now := time.Now().UTC()
	if hasExpiredRuntimeLeaseLocked(s, now) {
		before := snapshotTeamStateLocked(s)
		expireRuntimeLeasesLocked(s, now)
		if err := saveTeamStateLocked(s, before); err != nil {
			s.mu.Unlock()
			teamError(w, http.StatusInternalServerError, "AGENT_RUNTIME_SAVE_FAILED", err.Error())
			return
		}
	}
	items := make([]map[string]any, 0)
	for _, item := range s.data.AgentRuntimes {
		if aiString(item, "projectId") != projectID {
			continue
		}
		copy := sanitizeTeamMap(item)
		delete(copy, "fencingToken")
		delete(copy, "leaseId")
		items = append(items, copy)
	}
	s.mu.Unlock()
	aiOK(w, map[string]any{"items": items, "total": len(items)})
}

// expireRuntimeLeasesLocked 将已过期租约落盘并广播离线事件，保证状态查询和事件流使用同一权威状态。
func expireRuntimeLeasesLocked(s *executionState, now time.Time) bool {
	changed := false
	for i, item := range s.data.AgentRuntimes {
		if item == nil || aiString(item, "status") == "fenced" || aiString(item, "status") == "offline" {
			continue
		}
		expiresAt := parseTeamTime(aiString(item, "leaseExpiresAt"))
		if expiresAt.IsZero() || expiresAt.After(now) {
			continue
		}
		updated := cloneMap(item)
		updated["status"] = "offline"
		updated["leaseExpiresAt"] = ""
		updated["lastSeenAt"] = now.Format(time.RFC3339Nano)
		s.data.AgentRuntimes[i] = updated
		appendTeamEventLocked(s, aiString(updated, "projectId"), "runtime.offline", "", aiString(updated, "runtimeId"), map[string]any{"reason": "lease_expired"})
		changed = true
	}
	return changed
}

func hasExpiredRuntimeLeaseLocked(s *executionState, now time.Time) bool {
	for _, item := range s.data.AgentRuntimes {
		if item == nil || aiString(item, "status") == "fenced" || aiString(item, "status") == "offline" {
			continue
		}
		expiresAt := parseTeamTime(aiString(item, "leaseExpiresAt"))
		if !expiresAt.IsZero() && !expiresAt.After(now) {
			return true
		}
	}
	return false
}

func handleProjectTaskCreate(w http.ResponseWriter, r *http.Request) {
	projectID := validTeamID(r.PathValue("projectId"))
	if projectID == "" {
		teamError(w, http.StatusBadRequest, "PROJECT_ID_INVALID", "projectId 无效")
		return
	}
	body, err := aiBody(r)
	if err != nil {
		teamError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	title := boundedText(aiString(body, "title", "name"), 240)
	instruction := boundedText(aiString(body, "instruction", "prompt", "description"), 64000)
	if title == "" || instruction == "" {
		teamError(w, http.StatusBadRequest, "TASK_FIELDS_REQUIRED", "title 和 instruction 不能为空")
		return
	}
	resourceProfile := strings.ToLower(strings.TrimSpace(aiString(body, "resourceProfile")))
	if resourceProfile == "" {
		resourceProfile = "small"
	}
	if _, ok := teamResourceProfiles[resourceProfile]; !ok {
		teamError(w, http.StatusBadRequest, "TASK_RESOURCE_PROFILE_INVALID", "resourceProfile 只能是 small、medium 或 large")
		return
	}
	executionMetadata, metadataErr := teamTaskExecutionMetadata(body)
	if metadataErr != "" {
		teamError(w, http.StatusBadRequest, "TASK_EXECUTION_METADATA_INVALID", metadataErr)
		return
	}
	dependencies, dependencyErr := teamDependencyIDs(body["dependsOn"], body["dependencyTaskIds"])
	if dependencyErr != "" {
		teamError(w, http.StatusBadRequest, "TASK_DEPENDENCIES_INVALID", dependencyErr)
		return
	}
	s := getAIState()
	s.mu.Lock()
	defer s.mu.Unlock()
	before := snapshotTeamStateLocked(s)
	clientKey := boundedText(aiString(body, "idempotencyKey", "clientTaskId"), 160)
	for _, old := range s.data.TeamTasks {
		if aiString(old, "projectId") == projectID && clientKey != "" && aiString(old, "idempotencyKey") == clientKey {
			aiOK(w, map[string]any{"task": sanitizeTeamMap(old), "replayed": true})
			return
		}
	}
	projectTasks := 0
	for _, existing := range s.data.TeamTasks {
		if aiString(existing, "projectId") == projectID {
			projectTasks++
		}
	}
	if projectTasks >= teamTaskLimit || pendingTeamCommandsLocked(s, projectID) >= teamPendingCommandLimit {
		teamError(w, http.StatusTooManyRequests, "TEAM_CAPACITY_REACHED", "项目任务或待处理命令已达到上限")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	taskID := teamNewID("task")
	if err := validateTeamDependenciesLocked(s, projectID, taskID, dependencies); err != "" {
		teamError(w, http.StatusBadRequest, "TASK_DEPENDENCIES_INVALID", err)
		return
	}
	status := teamDependencyStatusLocked(s, dependencies)
	if status == "accepted" && !teamResourceAvailableLocked(s, projectID, resourceProfile, "") {
		status = "waiting_resource"
	}
	task := map[string]any{"taskId": taskID, "projectId": projectID, "title": title, "instruction": instruction, "status": status, "attempt": 1, "dependencyTaskIds": dependencies, "idempotencyKey": clientKey, "createdAt": now, "updatedAt": now, "ownerMemberId": validTeamID(aiString(body, "ownerMemberId")), "resourceProfile": resourceProfile}
	for key, value := range executionMetadata {
		task[key] = value
	}
	s.data.TeamTasks = append(s.data.TeamTasks, task)
	eventType := "task.accepted"
	if status == "waiting_dependency" {
		eventType = "task.waiting_dependency"
	} else if status == "waiting_resource" {
		eventType = "task.waiting_resource"
	}
	appendTeamEventLocked(s, projectID, eventType, aiString(task, "taskId"), "", map[string]any{"title": title, "ownerMemberId": task["ownerMemberId"], "dependencyTaskIds": dependencies})
	var command map[string]any
	if status == "accepted" {
		command = enqueueTeamCommandLocked(s, task)
	}
	if err := saveTeamStateLocked(s, before); err != nil {
		teamError(w, http.StatusInternalServerError, "TASK_SAVE_FAILED", err.Error())
		return
	}
	dispatchStatus := "awaiting_runtime"
	if status == "waiting_dependency" {
		dispatchStatus = "waiting_dependency"
	} else if status == "waiting_resource" {
		dispatchStatus = "waiting_resource"
	}
	if command != nil {
		dispatchStatus = "queued"
	}
	aiOK(w, map[string]any{"task": sanitizeTeamMap(task), "dispatchStatus": dispatchStatus})
}

// handleAgentRuntimeCommands 返回当前租约 runtime 尚未确认的下行任务命令。
func handleAgentRuntimeCommands(w http.ResponseWriter, r *http.Request, s *executionState) {
	projectID, runtimeID := validTeamID(r.URL.Query().Get("projectId")), validTeamID(r.URL.Query().Get("runtimeId"))
	fencing := strings.TrimSpace(r.Header.Get("X-WorkMesh-Fencing-Token"))
	if projectID == "" || runtimeID == "" || fencing == "" {
		teamError(w, http.StatusBadRequest, "AGENT_COMMAND_FIELDS_REQUIRED", "projectId、runtimeId 和 fencingToken 不能为空")
		return
	}
	limit := boundedQueryInt64(r.URL.Query().Get("limit"), 20, 50)
	waitSeconds := boundedQueryInt64(r.URL.Query().Get("waitSeconds"), 0, int64(teamCommandWaitMax/time.Second))
	deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)
	for {
		s.mu.Lock()
		runtimeItem := findRuntimeLocked(s, projectID, runtimeID)
		if runtimeItem == nil || aiString(runtimeItem, "fencingToken") != fencing || aiString(runtimeItem, "status") == "fenced" || parseTeamTime(aiString(runtimeItem, "leaseExpiresAt")).Before(time.Now().UTC()) {
			s.mu.Unlock()
			teamError(w, http.StatusConflict, "AGENT_RUNTIME_FENCED", "Agent runtime 租约已失效")
			return
		}
		before := snapshotTeamStateLocked(s)
		items := make([]map[string]any, 0, limit)
		for i := range s.data.AgentCommands {
			command := s.data.AgentCommands[i]
			if aiString(command, "projectId") != projectID || aiString(command, "runtimeId") != runtimeID || !teamCommandAvailable(command, time.Now().UTC()) {
				continue
			}
			command = cloneMap(command)
			attempt := boundedInt(command["deliveryAttempt"], 0, 1_000_000) + 1
			command["status"] = "delivered"
			command["deliveryAttempt"] = attempt
			command["deliveredAt"] = time.Now().UTC().Format(time.RFC3339Nano)
			command["deliveryExpiresAt"] = time.Now().UTC().Add(teamCommandVisibility).Format(time.RFC3339Nano)
			s.data.AgentCommands[i] = command
			items = append(items, sanitizeTeamMap(command))
			if int64(len(items)) >= limit {
				break
			}
		}
		if len(items) > 0 {
			if err := saveTeamStateLocked(s, before); err != nil {
				s.mu.Unlock()
				teamError(w, http.StatusInternalServerError, "AGENT_COMMAND_SAVE_FAILED", err.Error())
				return
			}
		}
		notify := s.notify
		s.mu.Unlock()
		if len(items) > 0 || waitSeconds == 0 || time.Now().After(deadline) {
			aiOK(w, map[string]any{"items": items, "total": len(items)})
			return
		}
		remaining := time.Until(deadline)
		timer := time.NewTimer(remaining)
		select {
		case <-r.Context().Done():
			stopTeamCommandTimer(timer)
			return
		case <-notify:
			stopTeamCommandTimer(timer)
		case <-timer.C:
			aiOK(w, map[string]any{"items": []map[string]any{}, "total": 0})
			return
		}
	}
}

func stopTeamCommandTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func handleAgentRuntimeCommandAck(w http.ResponseWriter, s *executionState, body map[string]any) {
	projectID, runtimeID, fencing := validTeamID(aiString(body, "projectId")), validTeamID(aiString(body, "runtimeId")), strings.TrimSpace(aiString(body, "fencingToken"))
	values, ok := body["commandIds"].([]any)
	if projectID == "" || runtimeID == "" || fencing == "" || !ok || len(values) == 0 || len(values) > 100 {
		teamError(w, http.StatusBadRequest, "AGENT_COMMAND_ACK_INVALID", "必须提供 projectId、runtimeId、fencingToken 和 1-100 个 commandIds")
		return
	}
	ids := make(map[string]struct{}, len(values))
	for _, value := range values {
		if id := validTeamID(fmt.Sprint(value)); id != "" {
			ids[id] = struct{}{}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	before := snapshotTeamStateLocked(s)
	runtimeItem := findRuntimeLocked(s, projectID, runtimeID)
	if runtimeItem == nil || aiString(runtimeItem, "fencingToken") != fencing || aiString(runtimeItem, "status") == "fenced" || parseTeamTime(aiString(runtimeItem, "leaseExpiresAt")).Before(time.Now().UTC()) {
		teamError(w, http.StatusConflict, "AGENT_RUNTIME_FENCED", "Agent runtime 租约已失效")
		return
	}
	acked := 0
	for i, command := range s.data.AgentCommands {
		if aiString(command, "projectId") != projectID || aiString(command, "runtimeId") != runtimeID {
			continue
		}
		if aiString(command, "status") != "delivered" {
			continue
		}
		if _, found := ids[aiString(command, "commandId")]; !found {
			continue
		}
		command = cloneMap(command)
		command["status"] = "acked"
		command["ackedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
		s.data.AgentCommands[i] = command
		acked++
	}
	compactTeamCommandsLocked(s, projectID)
	if err := saveTeamStateLocked(s, before); err != nil {
		teamError(w, http.StatusInternalServerError, "AGENT_COMMAND_SAVE_FAILED", err.Error())
		return
	}
	aiOK(w, map[string]any{"acked": acked})
}

func enqueueTeamCommandLocked(s *executionState, task map[string]any) map[string]any {
	payload := map[string]any{"title": task["title"], "instruction": task["instruction"], "resourceProfile": task["resourceProfile"], "attempt": teamTaskAttempt(task)}
	if timeoutSeconds := boundedInt(task["timeoutSeconds"], 0, 1800); timeoutSeconds > 0 {
		payload["timeoutSeconds"] = timeoutSeconds
	}
	for _, key := range []string{"workstreamId", "baseRevision", "worktreeId", "environmentId", "verificationOwner"} {
		if value := aiString(task, key); value != "" {
			payload[key] = value
		}
	}
	return enqueueTaskCommandLocked(s, task, "task.start", payload)
}

// teamTaskAttempt 兼容引入执行轮次前已保存的任务。
func teamTaskAttempt(task map[string]any) int {
	return boundedInt(task["attempt"], 1, 1_000_000)
}

func enqueueTaskCommandLocked(s *executionState, task map[string]any, kind string, payload map[string]any) map[string]any {
	projectID := aiString(task, "projectId")
	if kind == "task.start" && teamDependencyStatusLocked(s, teamStringSlice(task["dependencyTaskIds"])) != "accepted" {
		return nil
	}
	var runtime map[string]any
	for _, item := range s.data.AgentRuntimes {
		if aiString(item, "projectId") == projectID && aiString(item, "status") != "fenced" && parseTeamTime(aiString(item, "leaseExpiresAt")).After(time.Now().UTC()) {
			runtime = item
			break
		}
	}
	if runtime == nil {
		return nil
	}
	command := map[string]any{"commandId": teamNewID("command"), "projectId": projectID, "runtimeId": aiString(runtime, "runtimeId"), "taskId": aiString(task, "taskId"), "kind": kind, "status": "queued", "payload": boundedPayload(payload), "createdAt": time.Now().UTC().Format(time.RFC3339Nano)}
	s.data.AgentCommands = append(s.data.AgentCommands, command)
	compactTeamCommandsLocked(s, projectID)
	return command
}

// compactTeamCommandsLocked 限制每个项目的终态命令历史，始终保留 queued/delivered 活动命令。
func compactTeamCommandsLocked(s *executionState, projectID string) {
	terminal := 0
	for _, command := range s.data.AgentCommands {
		if aiString(command, "projectId") == projectID && teamCommandTerminal(command) {
			terminal++
		}
	}
	if terminal <= teamCommandHistoryLimit {
		return
	}
	removeBefore := terminal - teamCommandHistoryLimit
	result := make([]map[string]any, 0, len(s.data.AgentCommands)-removeBefore)
	for _, command := range s.data.AgentCommands {
		if aiString(command, "projectId") == projectID && teamCommandTerminal(command) && removeBefore > 0 {
			removeBefore--
			continue
		}
		result = append(result, command)
	}
	s.data.AgentCommands = result
}

func teamCommandTerminal(command map[string]any) bool {
	status := aiString(command, "status")
	return status == "acked" || status == "superseded"
}

func pendingTeamCommandsLocked(s *executionState, projectID string) int {
	count := 0
	for _, item := range s.data.AgentCommands {
		if aiString(item, "projectId") == projectID && aiString(item, "status") != "acked" && aiString(item, "status") != "superseded" {
			count++
		}
	}
	return count
}

func pendingTeamTaskCommandsLocked(s *executionState, taskID string) int {
	count := 0
	for _, command := range s.data.AgentCommands {
		if aiString(command, "taskId") == taskID && !teamCommandTerminal(command) {
			count++
		}
	}
	return count
}

func teamCommandAvailable(command map[string]any, now time.Time) bool {
	switch aiString(command, "status") {
	case "queued":
		return true
	case "delivered":
		expiresAt := parseTeamTime(aiString(command, "deliveryExpiresAt"))
		return !expiresAt.IsZero() && !expiresAt.After(now)
	default:
		return false
	}
}

// supersedeTeamCommandsLocked 阻止人工完成后旧指令再次驱动任务。
func supersedeTeamCommandsLocked(s *executionState, taskID string) {
	for i, command := range s.data.AgentCommands {
		if aiString(command, "taskId") != taskID || aiString(command, "status") == "acked" || aiString(command, "status") == "superseded" {
			continue
		}
		copy := cloneMap(command)
		copy["status"] = "superseded"
		copy["supersededAt"] = time.Now().UTC().Format(time.RFC3339Nano)
		s.data.AgentCommands[i] = copy
	}
}

func hasPendingTeamCommandLocked(s *executionState, taskID string) bool {
	for _, command := range s.data.AgentCommands {
		if aiString(command, "taskId") == taskID && aiString(command, "status") != "acked" && aiString(command, "status") != "superseded" {
			return true
		}
	}
	return false
}

func handleProjectTaskList(w http.ResponseWriter, r *http.Request) {
	projectID := validTeamID(r.PathValue("projectId"))
	if projectID == "" {
		teamError(w, http.StatusBadRequest, "PROJECT_ID_INVALID", "projectId 无效")
		return
	}
	page := boundedQueryInt64(r.URL.Query().Get("page"), 1, 10000)
	pageSize := boundedQueryInt64(r.URL.Query().Get("pageSize"), 50, 100)
	if page == 0 || pageSize == 0 {
		teamError(w, http.StatusBadRequest, "TASK_PAGE_INVALID", "page 和 pageSize 必须为正数")
		return
	}
	start := (page - 1) * pageSize
	s := getAIState()
	s.mu.RLock()
	items := make([]map[string]any, 0, pageSize)
	total := int64(0)
	for i := len(s.data.TeamTasks) - 1; i >= 0; i-- {
		if aiString(s.data.TeamTasks[i], "projectId") == projectID {
			if total >= start && total < start+pageSize {
				items = append(items, sanitizeTeamMap(s.data.TeamTasks[i]))
			}
			total++
		}
	}
	s.mu.RUnlock()
	aiOK(w, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

func handleDevTaskGet(w http.ResponseWriter, r *http.Request) {
	taskID := validTeamID(r.PathValue("taskId"))
	s := getAIState()
	s.mu.RLock()
	task := findTeamTaskLocked(s, taskID)
	s.mu.RUnlock()
	if task == nil {
		teamError(w, http.StatusNotFound, "TASK_NOT_FOUND", "任务不存在")
		return
	}
	aiOK(w, sanitizeTeamMap(task))
}

// handleDevTaskArtifacts 返回任务已登记的 Artifact 元数据，不读取或暴露宿主文件路径。
func handleDevTaskArtifacts(w http.ResponseWriter, r *http.Request) {
	handleDevTaskArtifactList(w, r, false)
}

// handleDevTaskEvidence 返回任务中标记为视觉或验证证据的 Artifact 元数据。
func handleDevTaskEvidence(w http.ResponseWriter, r *http.Request) {
	handleDevTaskArtifactList(w, r, true)
}

func handleDevTaskArtifactList(w http.ResponseWriter, r *http.Request, evidenceOnly bool) {
	taskID := validTeamID(r.PathValue("taskId"))
	page := boundedQueryInt64(r.URL.Query().Get("page"), 1, 10000)
	pageSize := boundedQueryInt64(r.URL.Query().Get("pageSize"), 50, 100)
	if page == 0 || pageSize == 0 {
		teamError(w, http.StatusBadRequest, "ARTIFACT_PAGE_INVALID", "page 和 pageSize 必须为正数")
		return
	}
	s := getAIState()
	s.mu.RLock()
	task := findTeamTaskLocked(s, taskID)
	var source []map[string]any
	if task != nil {
		source = teamArtifactHistory(task["artifacts"])
	}
	s.mu.RUnlock()
	if task == nil {
		teamError(w, http.StatusNotFound, "TASK_NOT_FOUND", "任务不存在")
		return
	}
	filtered := make([]map[string]any, 0, len(source))
	for _, item := range source {
		if evidenceOnly && aiString(item, "evidenceType") == "" && aiString(item, "kind") != "visual" && aiString(item, "kind") != "screenshot" {
			continue
		}
		filtered = append(filtered, sanitizeTeamMap(item))
	}
	start := (page - 1) * pageSize
	items := make([]map[string]any, 0, pageSize)
	for index, item := range filtered {
		if int64(index) < start || int64(index) >= start+pageSize {
			continue
		}
		items = append(items, item)
	}
	aiOK(w, map[string]any{"items": items, "total": len(filtered), "page": page, "pageSize": pageSize})
}

func handleDevTaskAction(w http.ResponseWriter, r *http.Request) {
	taskID := validTeamID(r.PathValue("taskId"))
	action := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	body, err := aiBody(r)
	if err != nil {
		teamError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	s := getAIState()
	s.mu.Lock()
	defer s.mu.Unlock()
	before := snapshotTeamStateLocked(s)
	task := findTeamTaskLocked(s, taskID)
	if task == nil {
		teamError(w, http.StatusNotFound, "TASK_NOT_FOUND", "任务不存在")
		return
	}
	projectID := aiString(task, "projectId")
	typ := "task." + action
	if aiString(task, "status") == "completed" {
		if action == "complete" && aiString(task, "completionSource") == "manual" {
			aiOK(w, map[string]any{"task": sanitizeTeamMap(task), "accepted": true, "replayed": true})
			return
		}
		if action == "complete" || aiString(task, "completionSource") == "manual" {
			teamError(w, http.StatusConflict, "TASK_ALREADY_COMPLETED", "任务已完成；如需继续，请创建新任务")
			return
		}
	}
	if action == "cancel" && aiString(task, "status") == "cancelled" {
		aiOK(w, map[string]any{"task": sanitizeTeamMap(task), "accepted": true, "replayed": true})
		return
	}
	if action == "retry" {
		if teamTaskAttempt(task) >= 1_000_000 {
			teamError(w, http.StatusConflict, "TASK_ATTEMPT_EXHAUSTED", "任务重试次数已达到上限")
			return
		}
		// 重试总是先取消旧轮次，覆盖失败事件与本地 Session 释放之间的短暂竞态。
		required := 2
		if pendingTeamCommandsLocked(s, projectID)-pendingTeamTaskCommandsLocked(s, taskID)+required > teamPendingCommandLimit {
			teamError(w, http.StatusTooManyRequests, "TEAM_COMMAND_CAPACITY_REACHED", "项目待处理命令已达到上限")
			return
		}
	}
	if (action == "input" || action == "cancel" || action == "complete") && pendingTeamCommandsLocked(s, projectID) >= teamPendingCommandLimit && ((action != "complete" && action != "cancel") || !hasPendingTeamCommandLocked(s, taskID)) {
		teamError(w, http.StatusTooManyRequests, "TEAM_COMMAND_CAPACITY_REACHED", "项目待处理命令已达到上限")
		return
	}
	status := ""
	switch action {
	case "input":
		if boundedText(aiString(body, "input", "message", "prompt"), 64000) == "" {
			teamError(w, http.StatusBadRequest, "TASK_INPUT_REQUIRED", "input 不能为空")
			return
		}
	case "cancel":
		status = "cancelled"
		task = cloneMap(task)
		task["cancellationSource"] = "manual"
		task["cancellationReason"] = boundedText(aiString(body, "reason"), 2000)
		task["cancelledAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	case "retry":
		task = cloneMap(task)
		previousAttempt := teamTaskAttempt(task)
		supersedeTeamCommandsLocked(s, taskID)
		enqueueTaskCommandLocked(s, task, "task.cancel", map[string]any{"attempt": previousAttempt, "reason": "retry"})
		task["attempt"] = previousAttempt + 1
		delete(task, "completionSource")
		delete(task, "completionReason")
		delete(task, "completedAt")
		delete(task, "cancellationSource")
		delete(task, "cancellationReason")
		delete(task, "cancelledAt")
		status = teamDependencyStatusLocked(s, teamStringSlice(task["dependencyTaskIds"]))
		if status == "accepted" && !teamResourceAvailableLocked(s, projectID, aiString(task, "resourceProfile"), taskID) {
			status = "waiting_resource"
		}
	case "complete":
		status = "completed"
		task = cloneMap(task)
		task["completionSource"] = "manual"
		task["completionReason"] = boundedText(aiString(body, "reason"), 2000)
		if task["completionReason"] == "" {
			task["completionReason"] = "用户直接完成"
		}
		task["completedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	case "approve":
		task = cloneMap(task)
		task["approvalStatus"] = "approved"
	case "reject":
		task = cloneMap(task)
		task["approvalStatus"] = "rejected"
		status = "blocked"
	default:
		teamError(w, http.StatusNotFound, "TASK_ACTION_NOT_FOUND", "任务操作不存在")
		return
	}
	if status != "" {
		task = cloneMap(task)
		task["status"] = status
	}
	task["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	replaceTeamTaskLocked(s, task)
	appendTeamEventLocked(s, projectID, typ, taskID, "", boundedPayload(body))
	if action == "complete" {
		supersedeTeamCommandsLocked(s, taskID)
		enqueueTaskCommandLocked(s, task, "task.cancel", map[string]any{"attempt": teamTaskAttempt(task), "reason": task["completionReason"], "completionSource": "manual"})
		activateReadyTeamTasksLocked(s, projectID)
	}
	if action == "cancel" || action == "reject" {
		if action == "cancel" {
			supersedeTeamCommandsLocked(s, taskID)
		}
		activateReadyTeamTasksLocked(s, projectID)
	}
	if action == "input" || action == "cancel" {
		payload := boundedPayload(body)
		payload["attempt"] = teamTaskAttempt(task)
		enqueueTaskCommandLocked(s, task, typ, payload)
	}
	if action == "retry" && status == "accepted" {
		enqueueTeamCommandLocked(s, task)
	}
	if err := saveTeamStateLocked(s, before); err != nil {
		teamError(w, http.StatusInternalServerError, "TASK_SAVE_FAILED", err.Error())
		return
	}
	aiOK(w, map[string]any{"task": sanitizeTeamMap(task), "accepted": true})
}

func handleProjectEventStream(w http.ResponseWriter, r *http.Request) {
	projectID := validTeamID(r.PathValue("projectId"))
	if projectID == "" {
		teamError(w, http.StatusBadRequest, "PROJECT_ID_INVALID", "projectId 无效")
		return
	}
	serveTeamSSE(w, r, projectID, "")
}

func handleDevTaskEvents(w http.ResponseWriter, r *http.Request) {
	taskID := validTeamID(r.PathValue("taskId"))
	s := getAIState()
	s.mu.RLock()
	task := findTeamTaskLocked(s, taskID)
	s.mu.RUnlock()
	if task == nil {
		teamError(w, http.StatusNotFound, "TASK_NOT_FOUND", "任务不存在")
		return
	}
	serveTeamSSE(w, r, aiString(task, "projectId"), taskID)
}

func serveTeamSSE(w http.ResponseWriter, r *http.Request, projectID, taskID string) {
	releaseStream, available := tryRuntimeSlotFor("streams", nodeRuntimeLimits.streams)
	if !available {
		teamError(w, http.StatusTooManyRequests, "SSE_LIMIT_REACHED", "实时事件连接已达到资源上限")
		return
	}
	defer releaseStream()
	controller := http.NewResponseController(w)
	cursor := parseCursor(r.Header.Get("Last-Event-ID"), r.URL.Query().Get("cursor"))
	snapshot := getAIState()
	snapshot.mu.Lock()
	now := time.Now().UTC()
	if hasExpiredRuntimeLeaseLocked(snapshot, now) {
		initialBefore := snapshotTeamStateLocked(snapshot)
		expireRuntimeLeasesLocked(snapshot, now)
		if err := saveTeamStateLocked(snapshot, initialBefore); err != nil {
			snapshot.mu.Unlock()
			teamError(w, http.StatusInternalServerError, "AGENT_RUNTIME_SAVE_FAILED", err.Error())
			return
		}
	}
	initialEvents := append([]map[string]any(nil), snapshot.data.TeamEvents[projectID]...)
	snapshot.mu.Unlock()
	if len(initialEvents) > 0 {
		oldest := boundedInt64(initialEvents[0]["sequence"], 0, 1<<60)
		if cursor > 0 && oldest > 0 && cursor < oldest-1 {
			teamError(w, http.StatusConflict, "EVENT_CURSOR_EXPIRED", "事件游标已超出保留窗口，请先重新读取任务快照")
			return
		}
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	for {
		s := getAIState()
		s.mu.Lock()
		now := time.Now().UTC()
		if hasExpiredRuntimeLeaseLocked(s, now) {
			before := snapshotTeamStateLocked(s)
			expireRuntimeLeasesLocked(s, now)
			if err := saveTeamStateLocked(s, before); err != nil {
				s.mu.Unlock()
				return
			}
		}
		events := append([]map[string]any(nil), s.data.TeamEvents[projectID]...)
		notify := s.notify
		s.mu.Unlock()
		for _, event := range events {
			sequence := boundedInt64(event["sequence"], 0, 1<<60)
			if sequence <= cursor || (taskID != "" && aiString(event, "taskId") != taskID) {
				continue
			}
			if writeTeamSSE(w, event) {
				cursor = sequence
				if err := controller.Flush(); err != nil {
					return
				}
			}
		}
		select {
		case <-r.Context().Done():
			return
		case <-notify:
		case <-time.After(15 * time.Second):
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}
}

func writeTeamSSE(w http.ResponseWriter, event map[string]any) bool {
	safeEvent := sanitizeTeamMap(event)
	raw, err := json.Marshal(safeEvent)
	if err != nil {
		return false
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", boundedInt64(safeEvent["sequence"], 0, 1<<60), aiString(safeEvent, "type"), raw)
	return err == nil
}

func appendTeamEventLocked(s *executionState, projectID, typ, taskID, runtimeID string, payload map[string]any) map[string]any {
	if s.data.TeamEvents == nil {
		s.data.TeamEvents = make(map[string][]map[string]any)
	}
	if s.data.TeamSequences == nil {
		s.data.TeamSequences = make(map[string]int64)
	}
	s.data.TeamSequences[projectID]++
	event := map[string]any{"eventId": teamNewID("event"), "sequence": s.data.TeamSequences[projectID], "projectId": projectID, "type": typ, "taskId": taskID, "runtimeId": runtimeID, "payload": boundedPayload(payload), "createdAt": time.Now().UTC().Format(time.RFC3339Nano)}
	events := append(s.data.TeamEvents[projectID], event)
	if len(events) > teamEventLimit {
		events = events[len(events)-teamEventLimit:]
	}
	s.data.TeamEvents[projectID] = events
	return event
}

type teamStateSnapshot struct {
	runtimes        []map[string]any
	tasks           []map[string]any
	commands        []map[string]any
	reclaimPlans    []map[string]any
	deploymentPlans []map[string]any
	events          map[string][]map[string]any
	sequence        map[string]int64
}

// snapshotTeamStateLocked 保存本次写入涉及的集合，磁盘写失败时恢复内存可见状态。
func snapshotTeamStateLocked(s *executionState) teamStateSnapshot {
	before := teamStateSnapshot{
		runtimes:        append([]map[string]any(nil), s.data.AgentRuntimes...),
		tasks:           append([]map[string]any(nil), s.data.TeamTasks...),
		commands:        append([]map[string]any(nil), s.data.AgentCommands...),
		reclaimPlans:    append([]map[string]any(nil), s.data.ArtifactReclaimPlans...),
		deploymentPlans: append([]map[string]any(nil), s.data.DeploymentPlans...),
		events:          make(map[string][]map[string]any, len(s.data.TeamEvents)),
		sequence:        make(map[string]int64, len(s.data.TeamSequences)),
	}
	for projectID, events := range s.data.TeamEvents {
		before.events[projectID] = events
	}
	for projectID, sequence := range s.data.TeamSequences {
		before.sequence[projectID] = sequence
	}
	return before
}

// saveTeamStateLocked 在持久化成功后才通知 SSE；失败时回滚所有团队集合。
func saveTeamStateLocked(s *executionState, before teamStateSnapshot) error {
	if err := s.saveLocked(); err != nil {
		s.data.AgentRuntimes = before.runtimes
		s.data.TeamTasks = before.tasks
		s.data.AgentCommands = before.commands
		s.data.ArtifactReclaimPlans = before.reclaimPlans
		s.data.DeploymentPlans = before.deploymentPlans
		s.data.TeamEvents = before.events
		s.data.TeamSequences = before.sequence
		return err
	}
	for projectID, sequence := range s.data.TeamSequences {
		if sequence > before.sequence[projectID] {
			if s.notify == nil {
				s.notify = make(chan struct{})
			}
			close(s.notify)
			s.notify = make(chan struct{})
			break
		}
	}
	return nil
}

// teamTaskHumanTerminal 防止晚到的 Agent 事件覆盖用户明确结束的任务。
func teamTaskHumanTerminal(task map[string]any) bool {
	status := aiString(task, "status")
	return (status == "completed" && aiString(task, "completionSource") == "manual") ||
		(status == "cancelled" && aiString(task, "cancellationSource") == "manual")
}

func updateTeamTaskFromEventLocked(s *executionState, projectID, taskID, typ string, payload map[string]any) {
	if taskID == "" {
		return
	}
	task := findTeamTaskLocked(s, taskID)
	if task == nil || aiString(task, "projectId") != projectID {
		return
	}
	if teamTaskHumanTerminal(task) {
		return
	}
	status := map[string]string{"task.started": "running", "task.blocked": "blocked", "task.completed": "completed", "task.failed": "failed", "task.cancelled": "cancelled"}[typ]
	if status == "" {
		return
	}
	task = cloneMap(task)
	task["status"], task["updatedAt"] = status, time.Now().UTC().Format(time.RFC3339Nano)
	if summary := boundedText(aiString(payload, "summary", "message"), 64000); summary != "" {
		task["summary"] = summary
	}
	replaceTeamTaskLocked(s, task)
}

// applyTeamCollaborationEventLocked 将交接和完成报告写入任务快照。
// 这些字段属于任务控制面，正文仍限制在有界 payload 内，不把完整日志或源码放入任务记录。
func applyTeamCollaborationEventLocked(s *executionState, projectID, taskID, typ string, payload map[string]any) {
	if typ != "task.handoff" && typ != "task.completion_report" && typ != "artifact.created" {
		return
	}
	task := findTeamTaskLocked(s, taskID)
	if task == nil || aiString(task, "projectId") != projectID {
		return
	}
	if teamTaskHumanTerminal(task) {
		return
	}
	updated := cloneMap(task)
	if typ == "task.handoff" {
		history := appendTeamHistory(task["handoffHistory"], payload, 20)
		updated["handoffHistory"] = history
		updated["lastHandoff"] = cloneMap(payload)
		if memberID := validTeamID(aiString(payload, "toMemberId")); memberID != "" {
			updated["ownerMemberId"] = memberID
		}
		if sessionID := validTeamID(aiString(payload, "toSessionId")); sessionID != "" {
			updated["ownerSessionId"] = sessionID
		}
		updated["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
		replaceTeamTaskLocked(s, updated)
		return
	}
	if typ == "artifact.created" {
		artifacts := teamArtifactHistory(task["artifacts"])
		artifactID := aiString(payload, "artifactId")
		found := false
		for i, item := range artifacts {
			if aiString(item, "artifactId") == artifactID {
				artifacts[i] = cloneMap(payload)
				found = true
				break
			}
		}
		if !found {
			artifacts = append(artifacts, cloneMap(payload))
		}
		if len(artifacts) > 64 {
			artifacts = artifacts[len(artifacts)-64:]
		}
		updated["artifacts"] = artifacts
		updated["artifactCount"] = len(artifacts)
		updated["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
		replaceTeamTaskLocked(s, updated)
		return
	}

	reports := appendTeamHistory(task["completionReports"], payload, 20)
	updated["completionReports"] = reports
	updated["completionReport"] = cloneMap(payload)
	updated["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	switch aiString(payload, "status") {
	case "completed":
		updated["status"] = "completed"
		updated["completionSource"] = "agent_report"
		updated["completedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	case "failed":
		updated["status"] = "failed"
	case "blocked":
		updated["status"] = "blocked"
	case "needs_input":
		updated["status"] = "waiting_input"
	case "in_progress":
		updated["status"] = "running"
	}
	replaceTeamTaskLocked(s, updated)
	if aiString(updated, "status") == "completed" || aiString(updated, "status") == "failed" || aiString(updated, "status") == "blocked" {
		activateReadyTeamTasksLocked(s, projectID)
	}
}

func appendTeamHistory(existing any, value map[string]any, limit int) []any {
	items := make([]any, 0, limit)
	switch raw := existing.(type) {
	case []any:
		items = append(items, raw...)
	case []map[string]any:
		for _, item := range raw {
			items = append(items, item)
		}
	}
	items = append(items, cloneMap(value))
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items
}

func normalizeTeamCollaborationEvent(typ string, value any) (map[string]any, string) {
	payload := boundedPayload(value)
	if typ != "task.handoff" && typ != "task.completion_report" && typ != "artifact.created" {
		return payload, ""
	}
	if typ == "task.handoff" {
		toMemberID := validTeamID(aiString(payload, "toMemberId"))
		toSessionID := validTeamID(aiString(payload, "toSessionId"))
		if toMemberID == "" && toSessionID == "" {
			return nil, "task.handoff 必须提供 toMemberId 或 toSessionId"
		}
		if fromMemberID := aiString(payload, "fromMemberId"); fromMemberID != "" && validTeamID(fromMemberID) == "" {
			return nil, "task.handoff 的 fromMemberId 无效"
		}
		if fromSessionID := aiString(payload, "fromSessionId"); fromSessionID != "" && validTeamID(fromSessionID) == "" {
			return nil, "task.handoff 的 fromSessionId 无效"
		}
		normalized := map[string]any{}
		for _, key := range []string{"fromMemberId", "fromSessionId", "toMemberId", "toSessionId", "reason", "summary"} {
			if text := boundedText(aiString(payload, key), 2000); text != "" {
				normalized[key] = text
			}
		}
		if toMemberID != "" {
			normalized["toMemberId"] = toMemberID
		}
		if toSessionID != "" {
			normalized["toSessionId"] = toSessionID
		}
		return normalized, ""
	}
	if typ == "artifact.created" {
		artifactID := validTeamID(aiString(payload, "artifactId"))
		if artifactID == "" {
			return nil, "artifact.created 必须提供有效 artifactId"
		}
		kind := aiString(payload, "kind")
		switch kind {
		case "diff", "test", "log", "screenshot", "video", "trace", "visual", "other":
		default:
			return nil, "artifact.created 的 kind 无效"
		}
		hash := strings.ToLower(strings.TrimSpace(aiString(payload, "sha256")))
		if len(hash) != 64 {
			return nil, "artifact.created 必须提供 64 位 sha256"
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return nil, "artifact.created 的 sha256 无效"
		}
		size, ok := boundedArtifactSize(payload["size"])
		if !ok {
			return nil, "artifact.created 的 size 必须是 0 到 1GiB 的整数"
		}
		normalized := map[string]any{"artifactId": artifactID, "kind": kind, "sha256": hash, "size": size}
		for _, key := range []string{"storageRef", "mediaType", "evidenceType", "label"} {
			if text := boundedText(aiString(payload, key), 512); text != "" {
				if key == "storageRef" && validTeamID(text) == "" {
					return nil, "artifact.created 的 storageRef 只能使用安全标识"
				}
				normalized[key] = text
			}
		}
		return normalized, ""
	}

	status := aiString(payload, "status")
	switch status {
	case "in_progress", "completed", "failed", "blocked", "needs_input":
	default:
		return nil, "task.completion_report 的 status 无效"
	}
	normalized := map[string]any{"status": status}
	for _, key := range []string{"summary", "nextAction", "verification"} {
		if text := boundedText(aiString(payload, key), 64000); text != "" {
			normalized[key] = text
		}
	}
	if sessionID := validTeamID(aiString(payload, "sessionId")); sessionID != "" {
		normalized["sessionId"] = sessionID
	} else if aiString(payload, "sessionId") != "" {
		return nil, "task.completion_report 的 sessionId 无效"
	}
	if rawArtifacts, ok := payload["artifactIds"].([]any); ok {
		if len(rawArtifacts) > 64 {
			return nil, "task.completion_report 的 artifactIds 最多 64 个"
		}
		artifacts := make([]string, 0, len(rawArtifacts))
		for _, item := range rawArtifacts {
			artifactID := validTeamID(fmt.Sprint(item))
			if artifactID == "" {
				return nil, "task.completion_report 的 artifactIds 包含无效标识"
			}
			artifacts = append(artifacts, artifactID)
		}
		if len(artifacts) > 0 {
			normalized["artifactIds"] = artifacts
		}
	} else if payload["artifactIds"] != nil {
		return nil, "task.completion_report 的 artifactIds 必须是数组"
	}
	return normalized, ""
}

func boundedArtifactSize(value any) (int64, bool) {
	if value == nil {
		return 0, false
	}
	var size int64
	switch item := value.(type) {
	case int:
		size = int64(item)
	case int64:
		size = item
	case float64:
		if item < 0 || item != float64(int64(item)) {
			return 0, false
		}
		size = int64(item)
	case json.Number:
		parsed, err := item.Int64()
		if err != nil {
			return 0, false
		}
		size = parsed
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(item), 10, 64)
		if err != nil {
			return 0, false
		}
		size = parsed
	default:
		return 0, false
	}
	if size < 0 || size > 1<<30 {
		return 0, false
	}
	return size, true
}

func teamArtifactHistory(value any) []map[string]any {
	items := make([]map[string]any, 0, 64)
	switch raw := value.(type) {
	case []any:
		for _, item := range raw {
			if mapped, ok := item.(map[string]any); ok {
				items = append(items, cloneMap(mapped))
			}
		}
	case []map[string]any:
		for _, item := range raw {
			items = append(items, cloneMap(item))
		}
	}
	return items
}

func findRuntimeLocked(s *executionState, projectID, runtimeID string) map[string]any {
	for _, item := range s.data.AgentRuntimes {
		if aiString(item, "projectId") == projectID && aiString(item, "runtimeId") == runtimeID {
			return item
		}
	}
	return nil
}

func replaceRuntimeLocked(s *executionState, replacement map[string]any) {
	for i, item := range s.data.AgentRuntimes {
		if aiString(item, "projectId") == aiString(replacement, "projectId") && aiString(item, "runtimeId") == aiString(replacement, "runtimeId") {
			s.data.AgentRuntimes[i] = replacement
			return
		}
	}
	s.data.AgentRuntimes = append(s.data.AgentRuntimes, replacement)
}

func findTeamTaskLocked(s *executionState, taskID string) map[string]any {
	for _, item := range s.data.TeamTasks {
		if aiString(item, "taskId", "id") == taskID {
			return item
		}
	}
	return nil
}

func replaceTeamTaskLocked(s *executionState, replacement map[string]any) {
	for i, item := range s.data.TeamTasks {
		if aiString(item, "taskId", "id") == aiString(replacement, "taskId", "id") {
			s.data.TeamTasks[i] = replacement
			return
		}
	}
}

func validTeamID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' && r != '.' {
			return ""
		}
	}
	return value
}

// teamTaskExecutionMetadata 校验任务工作区和验收上下文，只保存引用标识，不接受宿主机路径或命令。
func teamTaskExecutionMetadata(body map[string]any) (map[string]any, string) {
	metadata := make(map[string]any)
	for _, key := range []string{"workstreamId", "worktreeId", "environmentId", "verificationOwner"} {
		raw := strings.TrimSpace(aiString(body, key))
		if raw == "" {
			continue
		}
		if validTeamID(raw) == "" {
			return nil, key + " 必须是安全标识"
		}
		metadata[key] = raw
	}
	baseRevision := strings.TrimSpace(aiString(body, "baseRevision"))
	if baseRevision != "" {
		if len(baseRevision) > 256 || strings.IndexFunc(baseRevision, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
			return nil, "baseRevision 长度或字符无效"
		}
		metadata["baseRevision"] = baseRevision
	}
	if raw, ok := body["timeoutSeconds"]; ok {
		timeoutSeconds := boundedInt64(raw, -1, 3601)
		if timeoutSeconds < 1 || timeoutSeconds > 1800 {
			return nil, "timeoutSeconds 必须是 1-1800 的整数"
		}
		metadata["timeoutSeconds"] = timeoutSeconds
	}
	return metadata, ""
}

func validEventType(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 80 {
		return ""
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '.' && r != '_' && r != '-' {
			return ""
		}
	}
	return value
}

func boundedText(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) > max {
		return value[:max]
	}
	return value
}

func boundedPayload(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	item, ok := value.(map[string]any)
	if !ok {
		return map[string]any{"value": boundedText(fmt.Sprint(value), 64000)}
	}
	copy := sanitizeTeamMap(item)
	raw, err := json.Marshal(copy)
	if err == nil && len(raw) > 128<<10 {
		return map[string]any{"truncated": true, "bytes": len(raw)}
	}
	return copy
}

// sanitizeTeamMap 递归脱敏事件和命令 payload 中的凭据字段，避免流式事件回放泄露密钥。
// 只按字段名处理，不扫描普通文本，避免破坏模型输出、日志摘要和代码片段。
func sanitizeTeamMap(source map[string]any) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		if sensitiveTeamField(key) {
			result[key] = "******"
			continue
		}
		result[key] = sanitizeTeamValue(value)
	}
	return result
}

func sanitizeTeamValue(value any) any {
	switch item := value.(type) {
	case map[string]any:
		return sanitizeTeamMap(item)
	case []any:
		items := make([]any, len(item))
		for index, nested := range item {
			items[index] = sanitizeTeamValue(nested)
		}
		return items
	case []map[string]any:
		items := make([]map[string]any, len(item))
		for index, nested := range item {
			items[index] = sanitizeTeamMap(nested)
		}
		return items
	default:
		return value
	}
}

func sensitiveTeamField(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(strings.TrimSpace(key)))
	if normalized == "" {
		return false
	}
	for _, marker := range []string{"password", "secret", "apikey", "credential", "privatekey", "authorization"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return normalized == "token" || strings.HasSuffix(normalized, "token")
}

func boundedInt(value any, fallback, max int) int {
	parsed := boundedInt64(value, int64(fallback), int64(max))
	return int(parsed)
}

func boundedInt64(value any, fallback, max int64) int64 {
	var parsed int64
	switch item := value.(type) {
	case float64:
		parsed = int64(item)
	case int:
		parsed = int64(item)
	case int64:
		parsed = item
	case json.Number:
		parsed, _ = item.Int64()
	case string:
		parsed, _ = strconv.ParseInt(strings.TrimSpace(item), 10, 64)
	default:
		return fallback
	}
	if parsed < 0 {
		return fallback
	}
	if parsed > max {
		return max
	}
	return parsed
}

func boundedQueryInt64(value string, fallback, max int64) int64 {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return boundedInt64(value, fallback, max)
}

func parseTeamTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Unix(0, 0).UTC()
	}
	return parsed
}

func parseCursor(values ...string) int64 {
	for _, value := range values {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && parsed >= 0 {
			return parsed
		}
	}
	return 0
}

func teamNewID(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UTC().UnixNano(), teamIDCounter.Add(1))
}

func teamError(w http.ResponseWriter, status int, code, message string) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "details": map[string]string{"errCode": code}, "message": message})
}
