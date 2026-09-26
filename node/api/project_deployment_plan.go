// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"strings"
	"time"
)

// handleProjectDeploymentDryRun 只生成待审批的部署计划，不触发任何部署执行。
func handleProjectDeploymentDryRun(w http.ResponseWriter, r *http.Request) {
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
	environmentID := validTeamID(aiString(body, "environmentId"))
	if environmentID == "" {
		teamError(w, http.StatusBadRequest, "DEPLOYMENT_ENVIRONMENT_INVALID", "environmentId 必须是安全标识")
		return
	}
	switch strings.ToLower(environmentID) {
	case "production", "prod", "live":
		teamError(w, http.StatusForbidden, "DEPLOYMENT_PRODUCTION_FORBIDDEN", "dry-run 不允许使用生产环境")
		return
	}
	version := strings.TrimSpace(aiString(body, "version", "candidateVersion"))
	if version == "" || len(version) > 256 || strings.IndexFunc(version, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		teamError(w, http.StatusBadRequest, "DEPLOYMENT_VERSION_INVALID", "version 必须为 1-256 个可打印字符")
		return
	}
	idempotencyKey := boundedText(aiString(body, "idempotencyKey"), 160)
	if raw := aiString(body, "idempotencyKey"); raw != "" && idempotencyKey != raw {
		teamError(w, http.StatusBadRequest, "DEPLOYMENT_IDEMPOTENCY_INVALID", "idempotencyKey 长度不能超过 160")
		return
	}
	artifactRefs, validationErr := deploymentArtifactReferences(body["artifactRefs"])
	if validationErr != "" {
		teamError(w, http.StatusBadRequest, "DEPLOYMENT_ARTIFACT_REFS_INVALID", validationErr)
		return
	}
	healthChecks, validationErr := deploymentHealthChecks(body["healthChecks"])
	if validationErr != "" {
		teamError(w, http.StatusBadRequest, "DEPLOYMENT_HEALTH_CHECKS_INVALID", validationErr)
		return
	}

	s := getAIState()
	s.mu.Lock()
	defer s.mu.Unlock()
	if idempotencyKey != "" {
		for _, existing := range s.data.DeploymentPlans {
			if aiString(existing, "projectId") == projectID && aiString(existing, "idempotencyKey") == idempotencyKey {
				aiOK(w, map[string]any{"plan": sanitizeTeamMap(existing), "replayed": true})
				return
			}
		}
	}
	for i := range artifactRefs {
		ref := &artifactRefs[i]
		task := findTeamTaskLocked(s, ref.taskID)
		if task == nil || aiString(task, "projectId") != projectID {
			teamError(w, http.StatusNotFound, "DEPLOYMENT_TASK_NOT_FOUND", "Artifact 所属任务不存在或不属于当前项目")
			return
		}
		artifact := findTeamArtifactLocked(task, ref.artifactID)
		if artifact == nil {
			teamError(w, http.StatusNotFound, "DEPLOYMENT_ARTIFACT_NOT_FOUND", "Artifact 不存在")
			return
		}
		if aiString(artifact, "uploadStatus") != "complete" {
			teamError(w, http.StatusConflict, "DEPLOYMENT_ARTIFACT_NOT_READY", "Artifact 尚未完成上传")
			return
		}
		storageRef := boundedText(aiString(artifact, "storageRef"), 256)
		if validTeamID(storageRef) == "" {
			storageRef = teamArtifactStorageRef(projectID, ref.taskID, ref.artifactID)
		}
		ref.metadata = map[string]any{
			"taskId": ref.taskID, "artifactId": ref.artifactID,
			"size":       boundedInt64(artifact["size"], 0, teamArtifactMaxBytes),
			"sha256":     boundedText(aiString(artifact, "sha256"), 128),
			"storageRef": storageRef,
		}
	}
	items := make([]map[string]any, len(artifactRefs))
	for i := range artifactRefs {
		items[i] = artifactRefs[i].metadata
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	plan := map[string]any{
		"planId": teamNewID("deploy-plan"), "projectId": projectID, "environmentId": environmentID,
		"candidateVersion": version, "status": "awaiting_approval", "dryRun": true,
		"approvalRequired": true, "destructive": false, "deploymentStarted": false,
		"artifactRefs": items, "healthChecks": healthChecks, "createdAt": now,
		"idempotencyKey": idempotencyKey,
	}
	if deploymentPlanCountForProjectLocked(s, projectID) >= teamDeploymentPlanLimit {
		teamError(w, http.StatusTooManyRequests, "DEPLOYMENT_PLAN_LIMIT", "项目部署计划已达到保留上限")
		return
	}
	before := snapshotTeamStateLocked(s)
	s.data.DeploymentPlans = append(s.data.DeploymentPlans, plan)
	if err := saveTeamStateLocked(s, before); err != nil {
		teamError(w, http.StatusInternalServerError, "DEPLOYMENT_PLAN_SAVE_FAILED", err.Error())
		return
	}
	aiOK(w, map[string]any{"plan": sanitizeTeamMap(plan), "replayed": false})
}

type deploymentArtifactRef struct {
	taskID, artifactID string
	metadata           map[string]any
}

func deploymentArtifactReferences(raw any) ([]deploymentArtifactRef, string) {
	values, ok := raw.([]any)
	if !ok || len(values) == 0 || len(values) > 64 {
		return nil, "artifactRefs 必须包含 1-64 个 Artifact 引用"
	}
	refs := make([]deploymentArtifactRef, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, "Artifact 引用必须是对象"
		}
		taskID, artifactID := validTeamID(aiString(item, "taskId")), validTeamID(aiString(item, "artifactId"))
		if taskID == "" || artifactID == "" {
			return nil, "Artifact 引用必须包含有效 taskId 和 artifactId"
		}
		key := taskID + "\x00" + artifactID
		if _, exists := seen[key]; exists {
			return nil, "Artifact 引用不能重复"
		}
		seen[key] = struct{}{}
		refs = append(refs, deploymentArtifactRef{taskID: taskID, artifactID: artifactID})
	}
	return refs, ""
}

func deploymentHealthChecks(raw any) ([]map[string]any, string) {
	if raw == nil {
		return []map[string]any{{"name": "health", "method": "GET", "path": "/health", "expectedStatus": 200, "timeoutSeconds": 5}}, ""
	}
	values, ok := raw.([]any)
	if !ok || len(values) > 16 {
		return nil, "healthChecks 最多 16 个"
	}
	checks := make([]map[string]any, 0, len(values))
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, "健康检查必须是对象"
		}
		name := validTeamID(aiString(item, "name"))
		method := strings.ToUpper(strings.TrimSpace(aiString(item, "method")))
		path := strings.TrimSpace(aiString(item, "path"))
		status, statusOK := deploymentInteger(item["expectedStatus"])
		timeout, timeoutOK := deploymentInteger(item["timeoutSeconds"])
		if name == "" || (method != http.MethodGet && method != http.MethodHead) || !deploymentPathValid(path) || !statusOK || status < 100 || status > 599 || !timeoutOK || timeout < 1 || timeout > 30 {
			return nil, "健康检查字段无效：仅允许 GET/HEAD、站内路径、状态码 100-599、超时 1-30 秒"
		}
		checks = append(checks, map[string]any{"name": name, "method": method, "path": path, "expectedStatus": status, "timeoutSeconds": timeout})
	}
	return checks, ""
}

func deploymentInteger(value any) (int64, bool) {
	switch item := value.(type) {
	case float64:
		if item != float64(int64(item)) {
			return 0, false
		}
		return int64(item), true
	case int:
		return int64(item), true
	case int64:
		return item, true
	default:
		return 0, false
	}
}

func deploymentPathValid(path string) bool {
	if len(path) == 0 || len(path) > 2048 || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, "://") {
		return false
	}
	if strings.IndexFunc(path, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

func deploymentPlanCountForProjectLocked(s *executionState, projectID string) int {
	count := 0
	for _, plan := range s.data.DeploymentPlans {
		if aiString(plan, "projectId") == projectID {
			count++
		}
	}
	return count
}

// handleProjectDeploymentPlanList 返回项目部署计划，避免跨项目读取控制面数据。
func handleProjectDeploymentPlanList(w http.ResponseWriter, r *http.Request) {
	projectID := validTeamID(r.PathValue("projectId"))
	if projectID == "" {
		teamError(w, http.StatusBadRequest, "PROJECT_ID_INVALID", "projectId 无效")
		return
	}
	page := boundedQueryInt64(r.URL.Query().Get("page"), 1, 10000)
	pageSize := boundedQueryInt64(r.URL.Query().Get("pageSize"), 50, 100)
	if page == 0 || pageSize == 0 {
		teamError(w, http.StatusBadRequest, "DEPLOYMENT_PLAN_PAGE_INVALID", "page 和 pageSize 必须为正数")
		return
	}
	s := getAIState()
	s.mu.RLock()
	all := make([]map[string]any, 0)
	for i := len(s.data.DeploymentPlans) - 1; i >= 0; i-- {
		if aiString(s.data.DeploymentPlans[i], "projectId") == projectID {
			all = append(all, sanitizeTeamMap(s.data.DeploymentPlans[i]))
		}
	}
	s.mu.RUnlock()
	start := (page - 1) * pageSize
	items := make([]map[string]any, 0, pageSize)
	for i, item := range all {
		if int64(i) >= start && int64(i) < start+pageSize {
			items = append(items, item)
		}
	}
	aiOK(w, map[string]any{"projectId": projectID, "items": items, "total": len(all), "page": page, "pageSize": pageSize})
}

// handleProjectDeploymentPlanGet 返回单个项目部署计划，项目不匹配时统一返回不存在，避免泄露其他项目的计划标识。
func handleProjectDeploymentPlanGet(w http.ResponseWriter, r *http.Request) {
	projectID := validTeamID(r.PathValue("projectId"))
	planID := validTeamID(r.PathValue("planId"))
	if projectID == "" || planID == "" {
		teamError(w, http.StatusBadRequest, "DEPLOYMENT_PLAN_ID_INVALID", "projectId 或 planId 无效")
		return
	}
	s := getAIState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, plan := range s.data.DeploymentPlans {
		if aiString(plan, "planId") == planID && aiString(plan, "projectId") == projectID {
			aiOK(w, map[string]any{"plan": sanitizeTeamMap(plan)})
			return
		}
	}
	teamError(w, http.StatusNotFound, "DEPLOYMENT_PLAN_NOT_FOUND", "部署计划不存在")
}
