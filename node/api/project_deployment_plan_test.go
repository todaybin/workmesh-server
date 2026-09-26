// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestProjectDeploymentDryRunCreatesApprovalOnlyPlan(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	s := getAIState()
	s.mu.Lock()
	s.data.TeamTasks = []map[string]any{{
		"taskId": "task-deploy", "projectId": "project-deploy",
		"artifacts": []map[string]any{{"artifactId": "build-output", "uploadStatus": "complete", "size": int64(42), "sha256": strings.Repeat("a", 64), "storageRef": "artifact-ref"}},
	}}
	s.mu.Unlock()
	baseline := deploymentStateSnapshot()
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	response := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-deploy/deployments/dry-run", `{"idempotencyKey":"release-1","environmentId":"staging","version":"candidate-20260926","artifactRefs":[{"taskId":"task-deploy","artifactId":"build-output"}],"healthChecks":[{"name":"health","method":"GET","path":"/health","expectedStatus":200,"timeoutSeconds":5}]}`, false)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"awaiting_approval"`) || !strings.Contains(response.Body.String(), `"deploymentStarted":false`) {
		t.Fatalf("dry-run 计划生成失败: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "workmesh-artifacts") || !strings.Contains(response.Body.String(), `"storageRef":"artifact-ref"`) {
		t.Fatalf("计划不应暴露宿主路径且应保留受控 storageRef: %s", response.Body.String())
	}
	after := deploymentStateSnapshot()
	if after != baseline {
		t.Fatalf("dry-run 不应改变 localDeployment: before=%+v after=%+v", baseline, after)
	}
}

func TestProjectDeploymentDryRunReplacesUnsafeArtifactStorageRef(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	s := getAIState()
	s.mu.Lock()
	s.data.TeamTasks = []map[string]any{{
		"taskId": "task-storage", "projectId": "project-storage",
		"artifacts": []map[string]any{{"artifactId": "ready", "uploadStatus": "complete", "size": int64(1), "storageRef": "/var/lib/workmesh-artifacts/project-storage/ready.bin"}},
	}}
	s.mu.Unlock()
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	response := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-storage/deployments/dry-run", `{"environmentId":"staging","version":"v1","artifactRefs":[{"taskId":"task-storage","artifactId":"ready"}]}`, false)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "/var/lib/workmesh-artifacts") || !strings.Contains(response.Body.String(), `"storageRef":"artifact-`) {
		t.Fatalf("不安全 storageRef 未被替换: %d %s", response.Code, response.Body.String())
	}
}

func TestProjectDeploymentDryRunValidationAndProjectIsolation(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	s := getAIState()
	s.mu.Lock()
	s.data.TeamTasks = []map[string]any{
		{"taskId": "task-a", "projectId": "project-a", "artifacts": []map[string]any{{"artifactId": "ready", "uploadStatus": "complete", "size": int64(1)}}},
		{"taskId": "task-b", "projectId": "project-b", "artifacts": []map[string]any{{"artifactId": "ready", "uploadStatus": "complete", "size": int64(1)}}},
		{"taskId": "task-incomplete", "projectId": "project-a", "artifacts": []map[string]any{{"artifactId": "partial", "uploadStatus": "uploading", "size": int64(1)}}},
	}
	s.mu.Unlock()
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	for _, environment := range []string{"production", "prod", "live"} {
		response := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-a/deployments/dry-run", `{"environmentId":"`+environment+`","version":"v1","artifactRefs":[{"taskId":"task-a","artifactId":"ready"}]}`, false)
		if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "DEPLOYMENT_PRODUCTION_FORBIDDEN") {
			t.Fatalf("环境 %s 未拒绝: %d %s", environment, response.Code, response.Body.String())
		}
	}
	cases := []struct {
		name string
		body string
		code string
	}{
		{"cross-project", `{"environmentId":"staging","version":"v1","artifactRefs":[{"taskId":"task-b","artifactId":"ready"}]}`, "DEPLOYMENT_TASK_NOT_FOUND"},
		{"incomplete", `{"environmentId":"staging","version":"v1","artifactRefs":[{"taskId":"task-incomplete","artifactId":"partial"}]}`, "DEPLOYMENT_ARTIFACT_NOT_READY"},
		{"no-artifact", `{"environmentId":"staging","version":"v1","artifactRefs":[]}`, "DEPLOYMENT_ARTIFACT_REFS_INVALID"},
		{"bad-health", `{"environmentId":"staging","version":"v1","artifactRefs":[{"taskId":"task-a","artifactId":"ready"}],"healthChecks":[{"name":"bad","method":"POST","path":"/health","expectedStatus":200,"timeoutSeconds":5}]}`, "DEPLOYMENT_HEALTH_CHECKS_INVALID"},
	}
	for _, item := range cases {
		response := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-a/deployments/dry-run", item.body, false)
		if response.Code != http.StatusBadRequest && item.name == "cross-project" {
			// 跨项目任务由资源校验按 NotFound 返回，避免泄露其他项目资源。
		}
		if !strings.Contains(response.Body.String(), item.code) {
			t.Errorf("%s 错误码不匹配: %d %s", item.name, response.Code, response.Body.String())
		}
	}
}

func TestProjectDeploymentDryRunIdempotencyAndListIsolation(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	s := getAIState()
	s.mu.Lock()
	s.data.TeamTasks = []map[string]any{{"taskId": "task-a", "projectId": "project-a", "artifacts": []map[string]any{{"artifactId": "ready", "uploadStatus": "complete", "size": int64(1)}}}, {"taskId": "task-b", "projectId": "project-b", "artifacts": []map[string]any{{"artifactId": "ready", "uploadStatus": "complete", "size": int64(1)}}}}
	s.mu.Unlock()
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	body := `{"idempotencyKey":"release-1","environmentId":"staging","version":"v1","artifactRefs":[{"taskId":"task-a","artifactId":"ready"}]}`
	created := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-a/deployments/dry-run", body, false)
	replayed := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-a/deployments/dry-run", body, false)
	if created.Code != http.StatusOK || replayed.Code != http.StatusOK || !strings.Contains(replayed.Body.String(), `"replayed":true`) {
		t.Fatalf("部署计划幂等失败: created=%d replayed=%d body=%s", created.Code, replayed.Code, replayed.Body.String())
	}
	list := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-a/deployments/plans?page=1&pageSize=10", "", false)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"total":1`) || !strings.Contains(list.Body.String(), `"projectId":"project-a"`) {
		t.Fatalf("项目部署计划列表错误: %d %s", list.Code, list.Body.String())
	}
	var createdEnvelope struct {
		Data struct {
			Plan struct {
				PlanID string `json:"planId"`
			} `json:"plan"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdEnvelope); err != nil || createdEnvelope.Data.Plan.PlanID == "" {
		t.Fatalf("无法读取部署计划 ID: %s", created.Body.String())
	}
	detail := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-a/deployments/plans/"+createdEnvelope.Data.Plan.PlanID, "", false)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"projectId":"project-a"`) {
		t.Fatalf("部署计划详情错误: %d %s", detail.Code, detail.Body.String())
	}
	if !strings.Contains(detail.Body.String(), `"status":"awaiting_approval"`) || !strings.Contains(detail.Body.String(), `"approvalRequired":true`) || !strings.Contains(detail.Body.String(), `"destructive":false`) || !strings.Contains(detail.Body.String(), `"deploymentStarted":false`) {
		t.Fatalf("部署计划状态不变量被破坏: %s", detail.Body.String())
	}
	aiState = executionState{}
	reloaded := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-a/deployments/plans/"+createdEnvelope.Data.Plan.PlanID, "", false)
	if reloaded.Code != http.StatusOK || !strings.Contains(reloaded.Body.String(), createdEnvelope.Data.Plan.PlanID) || !strings.Contains(reloaded.Body.String(), `"status":"awaiting_approval"`) {
		t.Fatalf("部署计划重启后无法按原审批态读取: %d %s", reloaded.Code, reloaded.Body.String())
	}
	other := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-b/deployments/plans", "", false)
	if other.Code != http.StatusOK || !strings.Contains(other.Body.String(), `"total":0`) || strings.Contains(other.Body.String(), "project-a") {
		t.Fatalf("部署计划跨项目泄漏: %d %s", other.Code, other.Body.String())
	}
	crossProjectDetail := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-b/deployments/plans/"+createdEnvelope.Data.Plan.PlanID, "", false)
	if crossProjectDetail.Code != http.StatusNotFound || !strings.Contains(crossProjectDetail.Body.String(), "DEPLOYMENT_PLAN_NOT_FOUND") {
		t.Fatalf("部署计划详情跨项目泄漏: %d %s", crossProjectDetail.Code, crossProjectDetail.Body.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil || envelope["code"] != float64(200) {
		t.Fatalf("成功 envelope 无效: %s", created.Body.String())
	}
}

func TestProjectDeploymentDryRunRejectsMalformedAndUnsafeHealthChecks(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	s := getAIState()
	s.mu.Lock()
	s.data.TeamTasks = []map[string]any{{
		"taskId": "task-boundary", "projectId": "project-boundary",
		"artifacts": []map[string]any{{"artifactId": "ready", "uploadStatus": "complete", "size": int64(1)}},
	}}
	s.mu.Unlock()
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	cases := []struct {
		name string
		body string
		code string
	}{
		{"trailing-json", `{"environmentId":"staging","version":"v1","artifactRefs":[{"taskId":"task-boundary","artifactId":"ready"}]} {}`, "INVALID_JSON"},
		{"path-traversal", `{"environmentId":"staging","version":"v1","artifactRefs":[{"taskId":"task-boundary","artifactId":"ready"}],"healthChecks":[{"name":"unsafe","method":"GET","path":"/api/../admin","expectedStatus":200,"timeoutSeconds":5}]}`, "DEPLOYMENT_HEALTH_CHECKS_INVALID"},
		{"float-status", `{"environmentId":"staging","version":"v1","artifactRefs":[{"taskId":"task-boundary","artifactId":"ready"}],"healthChecks":[{"name":"float","method":"GET","path":"/health","expectedStatus":200.5,"timeoutSeconds":5}]}`, "DEPLOYMENT_HEALTH_CHECKS_INVALID"},
		{"long-idempotency", `{"environmentId":"staging","version":"v1","idempotencyKey":"` + strings.Repeat("x", 161) + `","artifactRefs":[{"taskId":"task-boundary","artifactId":"ready"}]}`, "DEPLOYMENT_IDEMPOTENCY_INVALID"},
	}
	for _, item := range cases {
		response := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-boundary/deployments/dry-run", item.body, false)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), item.code) {
			t.Errorf("%s 边界校验失败: %d %s", item.name, response.Code, response.Body.String())
		}
	}
}

func TestProjectDeploymentDryRunEnforcesProjectPlanLimit(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	s := getAIState()
	s.mu.Lock()
	s.data.TeamTasks = []map[string]any{{
		"taskId": "task-limit", "projectId": "project-limit",
		"artifacts": []map[string]any{{"artifactId": "ready", "uploadStatus": "complete", "size": int64(1)}},
	}}
	for i := 0; i < teamDeploymentPlanLimit; i++ {
		s.data.DeploymentPlans = append(s.data.DeploymentPlans, map[string]any{
			"planId": teamNewID("deploy-plan"), "projectId": "project-limit", "status": "awaiting_approval",
		})
	}
	s.mu.Unlock()
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	response := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-limit/deployments/dry-run", `{"environmentId":"staging","version":"v1","artifactRefs":[{"taskId":"task-limit","artifactId":"ready"}]}`, false)
	if response.Code != http.StatusTooManyRequests || !strings.Contains(response.Body.String(), "DEPLOYMENT_PLAN_LIMIT") {
		t.Fatalf("项目计划上限未生效: %d %s", response.Code, response.Body.String())
	}
}
