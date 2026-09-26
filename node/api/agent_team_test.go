// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAgentTeamRuntimeTaskEventsAndSSE(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_AGENT_RUNTIME_TOKEN", "runtime-secret")
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	unauthorized := teamTestRequestWithToken(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", `{}`, "wrong-token")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("错误 Agent token 未拒绝: %d %s", unauthorized.Code, unauthorized.Body.String())
	}
	projectB := `{"projectId":"project-b","runtimeId":"runtime-b","instanceId":"instance-b","memberId":"leader-b"}`
	crossProject := teamTestRequestWithToken(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", projectB, deriveAgentProjectToken("runtime-secret", "project-a"))
	if crossProject.Code != http.StatusUnauthorized {
		t.Fatalf("项目 A 令牌不能注册项目 B: %d %s", crossProject.Code, crossProject.Body.String())
	}
	masterToken := teamTestRequestWithToken(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", projectB, "runtime-secret")
	if masterToken.Code != http.StatusUnauthorized {
		t.Fatalf("主密钥不能作为项目令牌使用: %d %s", masterToken.Code, masterToken.Body.String())
	}

	register := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", `{"projectId":"project-a","runtimeId":"runtime-a","instanceId":"instance-a","memberId":"leader-a"}`, true)
	if register.Code != http.StatusOK {
		t.Fatalf("注册失败: %d %s", register.Code, register.Body.String())
	}
	var registerEnvelope struct {
		Data struct {
			Runtime map[string]any `json:"runtime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(register.Body.Bytes(), &registerEnvelope); err != nil {
		t.Fatal(err)
	}
	fencing, _ := registerEnvelope.Data.Runtime["fencingToken"].(string)
	if fencing == "" {
		t.Fatalf("缺少 fencingToken: %s", register.Body.String())
	}
	conflict := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", `{"projectId":"project-a","runtimeId":"runtime-b","instanceId":"instance-b","memberId":"leader-b"}`, true)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("活动 runtime 未阻止重复注册: %d %s", conflict.Code, conflict.Body.String())
	}
	heartbeat := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/heartbeat", `{"projectId":"project-a","runtimeId":"runtime-a","fencingToken":"`+fencing+`","activeTasks":1}`, true)
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("心跳失败: %d %s", heartbeat.Code, heartbeat.Body.String())
	}
	invalidProfile := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-a/tasks", `{"title":"非法资源配置","instruction":"不得派发","resourceProfile":"gpu"}`, false)
	if invalidProfile.Code != http.StatusBadRequest || !strings.Contains(invalidProfile.Body.String(), "TASK_RESOURCE_PROFILE_INVALID") {
		t.Fatalf("非法 resourceProfile 未拒绝: %d %s", invalidProfile.Code, invalidProfile.Body.String())
	}
	invalidWorktree := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-a/tasks", `{"title":"非法工作区","instruction":"不得派发","worktreeId":"../host"}`, false)
	if invalidWorktree.Code != http.StatusBadRequest || !strings.Contains(invalidWorktree.Body.String(), "TASK_EXECUTION_METADATA_INVALID") {
		t.Fatalf("非法 worktreeId 未拒绝: %d %s", invalidWorktree.Code, invalidWorktree.Body.String())
	}
	invalidTimeout := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-a/tasks", `{"title":"非法超时","instruction":"不得派发","timeoutSeconds":1801}`, false)
	if invalidTimeout.Code != http.StatusBadRequest || !strings.Contains(invalidTimeout.Body.String(), "TASK_EXECUTION_METADATA_INVALID") {
		t.Fatalf("非法 timeoutSeconds 未拒绝: %d %s", invalidTimeout.Code, invalidTimeout.Body.String())
	}

	taskResponse := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-a/tasks", `{"title":"修复接口","instruction":"执行测试并修复失败用例","resourceProfile":"medium","timeoutSeconds":120,"idempotencyKey":"client-1","workstreamId":"stream-api","baseRevision":"abc123","worktreeId":"tree-task-1","environmentId":"staging","verificationOwner":"reviewer-1"}`, false)
	if taskResponse.Code != http.StatusOK {
		t.Fatalf("创建任务失败: %d %s", taskResponse.Code, taskResponse.Body.String())
	}
	var taskEnvelope struct {
		Data struct {
			Task map[string]any `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(taskResponse.Body.Bytes(), &taskEnvelope); err != nil {
		t.Fatal(err)
	}
	taskID, _ := taskEnvelope.Data.Task["taskId"].(string)
	if taskID == "" {
		t.Fatalf("缺少任务 ID: %s", taskResponse.Body.String())
	}
	commands := teamTestRequestWithHeaders(t, mux, http.MethodGet, "/api/v2/agent-runtime/commands?projectId=project-a&runtimeId=runtime-a", "", map[string]string{"X-WorkMesh-Fencing-Token": fencing, "X-WorkMesh-Agent-Token": deriveAgentProjectToken("runtime-secret", "project-a")})
	if commands.Code != http.StatusOK || !strings.Contains(commands.Body.String(), `"kind":"task.start"`) {
		t.Fatalf("Agent 未收到下行任务命令: %d %s", commands.Code, commands.Body.String())
	}
	if !strings.Contains(commands.Body.String(), `"resourceProfile":"medium"`) {
		t.Fatalf("下行命令未携带资源配置: %s", commands.Body.String())
	}
	if !strings.Contains(commands.Body.String(), `"timeoutSeconds":120`) {
		t.Fatalf("下行命令未携带 timeoutSeconds: %s", commands.Body.String())
	}
	for _, metadata := range []string{`"workstreamId":"stream-api"`, `"baseRevision":"abc123"`, `"worktreeId":"tree-task-1"`, `"environmentId":"staging"`, `"verificationOwner":"reviewer-1"`} {
		if !strings.Contains(commands.Body.String(), metadata) {
			t.Fatalf("下行命令未携带任务执行元数据 %s: %s", metadata, commands.Body.String())
		}
	}
	var commandEnvelope struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(commands.Body.Bytes(), &commandEnvelope); err != nil {
		t.Fatal(err)
	}
	commandID, _ := commandEnvelope.Data.Items[0]["commandId"].(string)
	if repeated := teamTestRequestWithHeaders(t, mux, http.MethodGet, "/api/v2/agent-runtime/commands?projectId=project-a&runtimeId=runtime-a", "", map[string]string{"X-WorkMesh-Fencing-Token": fencing, "X-WorkMesh-Agent-Token": deriveAgentProjectToken("runtime-secret", "project-a")}); repeated.Code != http.StatusOK || strings.Contains(repeated.Body.String(), `"kind":"task.start"`) {
		t.Fatalf("未确认命令不应在可见期内重复投递: %d %s", repeated.Code, repeated.Body.String())
	}
	s := getAIState()
	s.mu.Lock()
	for i, command := range s.data.AgentCommands {
		if aiString(command, "commandId") == commandID {
			copy := cloneMap(command)
			copy["deliveryExpiresAt"] = time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
			s.data.AgentCommands[i] = copy
		}
	}
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	redelivered := teamTestRequestWithHeaders(t, mux, http.MethodGet, "/api/v2/agent-runtime/commands?projectId=project-a&runtimeId=runtime-a", "", map[string]string{"X-WorkMesh-Fencing-Token": fencing, "X-WorkMesh-Agent-Token": deriveAgentProjectToken("runtime-secret", "project-a")})
	if redelivered.Code != http.StatusOK || !strings.Contains(redelivered.Body.String(), `"deliveryAttempt":2`) {
		t.Fatalf("投递租约过期后未重投: %d %s", redelivered.Code, redelivered.Body.String())
	}
	gap := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", `{"projectId":"project-a","runtimeId":"runtime-a","fencingToken":"`+fencing+`","events":[{"sequence":2,"type":"task.started","taskId":"`+taskID+`"}]}`, true)
	if gap.Code != http.StatusConflict {
		t.Fatalf("事件 sequence 缺口未拒绝: %d %s", gap.Code, gap.Body.String())
	}
	ack := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/commands/ack", `{"projectId":"project-a","runtimeId":"runtime-a","fencingToken":"`+fencing+`","commandIds":["`+commandID+`"]}`, true)
	if ack.Code != http.StatusOK || !strings.Contains(ack.Body.String(), `"acked":1`) {
		t.Fatalf("Agent 命令确认失败: %d %s", ack.Code, ack.Body.String())
	}
	repeatedAck := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/commands/ack", `{"projectId":"project-a","runtimeId":"runtime-a","fencingToken":"`+fencing+`","commandIds":["`+commandID+`"]}`, true)
	if repeatedAck.Code != http.StatusOK || !strings.Contains(repeatedAck.Body.String(), `"acked":0`) {
		t.Fatalf("重复命令确认未幂等: %d %s", repeatedAck.Code, repeatedAck.Body.String())
	}
	events := `{"projectId":"project-a","runtimeId":"runtime-a","fencingToken":"` + fencing + `","events":[{"sequence":1,"type":"task.started","taskId":"` + taskID + `"},{"sequence":2,"type":"message.delta","taskId":"` + taskID + `","payload":{"text":"已开始","debug":{"apiKey":"stream-secret","nested":[{"access_token":"nested-secret"}]}}},{"sequence":3,"type":"task.completed","taskId":"` + taskID + `","payload":{"summary":"测试通过"}}]}`
	eventResponse := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", events, true)
	if eventResponse.Code != http.StatusOK || !strings.Contains(eventResponse.Body.String(), `"accepted":3`) {
		t.Fatalf("事件接收失败: %d %s", eventResponse.Code, eventResponse.Body.String())
	}
	eventState := getAIState()
	eventState.mu.RLock()
	eventJSON, marshalErr := json.Marshal(eventState.data.TeamEvents["project-a"])
	eventState.mu.RUnlock()
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(eventJSON), "stream-secret") || strings.Contains(string(eventJSON), "nested-secret") || !strings.Contains(string(eventJSON), `"apiKey":"******"`) || !strings.Contains(string(eventJSON), `"access_token":"******"`) {
		t.Fatalf("事件 payload 未递归脱敏: %s", eventJSON)
	}
	duplicate := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", strings.Replace(events, `"sequence":1`, `"sequence":2`, 1), true)
	if duplicate.Code != http.StatusOK || !strings.Contains(duplicate.Body.String(), `"accepted":0`) {
		t.Fatalf("重复事件未幂等: %d %s", duplicate.Code, duplicate.Body.String())
	}
	task := teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/"+taskID, "", false)
	if task.Code != http.StatusOK || !strings.Contains(task.Body.String(), `"status":"completed"`) {
		t.Fatalf("任务状态未更新: %d %s", task.Code, task.Body.String())
	}
	list := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-a/tasks", "", false)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), taskID) {
		t.Fatalf("任务列表缺少任务: %d %s", list.Code, list.Body.String())
	}

	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	streamRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v2/projects/project-a/events/stream?cursor=0", nil)
	if err != nil {
		t.Fatal(err)
	}
	streamResponse, err := http.DefaultClient.Do(streamRequest)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(streamResponse.Body)
	seenEvent := false
	for i := 0; i < 8; i++ {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			t.Fatal(readErr)
		}
		if strings.Contains(line, "event: task.accepted") {
			seenEvent = true
			break
		}
	}
	cancel()
	_ = streamResponse.Body.Close()
	if !seenEvent {
		t.Fatal("SSE 未回放 task.accepted 事件")
	}

	aiState = executionState{}
	restarted := teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/"+taskID, "", false)
	if restarted.Code != http.StatusOK || !strings.Contains(restarted.Body.String(), `"status":"completed"`) {
		t.Fatalf("任务重启恢复失败: %d %s", restarted.Code, restarted.Body.String())
	}
}

func TestAgentTeamJSONBodyLimitsAndRejectsTrailingValues(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	trailing := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-json/tasks", `{"title":"a","instruction":"b"}{"ignored":true}`, false)
	if trailing.Code != http.StatusBadRequest || !strings.Contains(trailing.Body.String(), `"errCode":"INVALID_JSON"`) {
		t.Fatalf("尾随 JSON 未拒绝: %d %s", trailing.Code, trailing.Body.String())
	}
	oversized := `{"title":"a","instruction":"` + strings.Repeat("x", 2<<20) + `"}`
	tooLarge := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-json/tasks", oversized, false)
	if tooLarge.Code != http.StatusBadRequest || !strings.Contains(tooLarge.Body.String(), `"errCode":"INVALID_JSON"`) {
		t.Fatalf("超大 JSON 未拒绝: %d %s", tooLarge.Code, tooLarge.Body.String())
	}
}

func TestAgentTeamHistoricalPayloadRedactionOnResponses(t *testing.T) {
	historical := map[string]any{
		"sequence": int64(7), "type": "message.delta",
		"payload": map[string]any{"debug": map[string]any{"apiKey": "legacy-secret", "text": "保留输出"}},
	}
	writer := httptest.NewRecorder()
	if !writeTeamSSE(writer, historical) {
		t.Fatal("历史事件 SSE 写出失败")
	}
	output := writer.Body.String()
	if strings.Contains(output, "legacy-secret") || !strings.Contains(output, `"apiKey":"******"`) || !strings.Contains(output, "保留输出") {
		t.Fatalf("历史事件响应脱敏错误: %s", output)
	}
	response := sanitizeTeamMap(map[string]any{"completionReport": map[string]any{"access_token": "legacy-token", "summary": "测试摘要"}})
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "legacy-token") || !strings.Contains(string(raw), `"access_token":"******"`) || !strings.Contains(string(raw), "测试摘要") {
		t.Fatalf("嵌套任务响应脱敏错误: %s", raw)
	}
}

func TestAgentTeamOfflineTaskDeliveredAfterRegister(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_AGENT_RUNTIME_TOKEN", "runtime-secret")
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	task := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-offline/tasks", `{"title":"离线任务","instruction":"Agent 恢复后领取"}`, false)
	if task.Code != http.StatusOK || !strings.Contains(task.Body.String(), `"dispatchStatus":"awaiting_runtime"`) {
		t.Fatalf("离线任务未正确排队: %d %s", task.Code, task.Body.String())
	}
	register := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", `{"projectId":"project-offline","runtimeId":"runtime-new","instanceId":"instance-new","memberId":"leader-new"}`, true)
	if register.Code != http.StatusOK {
		t.Fatalf("Agent 注册失败: %d %s", register.Code, register.Body.String())
	}
	var envelope struct {
		Data struct {
			Runtime map[string]any `json:"runtime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(register.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	fencing, _ := envelope.Data.Runtime["fencingToken"].(string)
	commands := teamTestRequestWithHeaders(t, mux, http.MethodGet, "/api/v2/agent-runtime/commands?projectId=project-offline&runtimeId=runtime-new", "", map[string]string{"X-WorkMesh-Fencing-Token": fencing, "X-WorkMesh-Agent-Token": deriveAgentProjectToken("runtime-secret", "project-offline")})
	if commands.Code != http.StatusOK || !strings.Contains(commands.Body.String(), `"kind":"task.start"`) {
		t.Fatalf("恢复后未领取离线任务: %d %s", commands.Code, commands.Body.String())
	}
}

func TestAgentTeamRuntimeStatusPersistsExpiredLeaseAndEmitsEvent(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_AGENT_RUNTIME_TOKEN", "runtime-secret")
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	registered := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", `{"projectId":"project-expiry","runtimeId":"runtime-expiry","instanceId":"instance-expiry","memberId":"leader-expiry"}`, true)
	if registered.Code != http.StatusOK {
		t.Fatalf("Agent 注册失败: %d %s", registered.Code, registered.Body.String())
	}
	s := getAIState()
	s.mu.Lock()
	for i, runtime := range s.data.AgentRuntimes {
		if aiString(runtime, "projectId") != "project-expiry" {
			continue
		}
		updated := cloneMap(runtime)
		updated["leaseExpiresAt"] = time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
		s.data.AgentRuntimes[i] = updated
	}
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	status := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-expiry/agent-runtime", "", false)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"offline"`) {
		t.Fatalf("过期 runtime 状态错误: %d %s", status.Code, status.Body.String())
	}
	s.mu.RLock()
	var offlineEvents int
	var persistedOffline bool
	for _, runtime := range s.data.AgentRuntimes {
		if aiString(runtime, "projectId") == "project-expiry" && aiString(runtime, "status") == "offline" {
			persistedOffline = true
		}
	}
	for _, event := range s.data.TeamEvents["project-expiry"] {
		if aiString(event, "type") == "runtime.offline" {
			offlineEvents++
		}
	}
	s.mu.RUnlock()
	if !persistedOffline || offlineEvents != 1 {
		t.Fatalf("过期租约未持久化或未广播一次离线事件: offline=%v events=%d", persistedOffline, offlineEvents)
	}
	second := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-expiry/agent-runtime", "", false)
	if second.Code != http.StatusOK {
		t.Fatalf("重复状态查询失败: %d %s", second.Code, second.Body.String())
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, event := range s.data.TeamEvents["project-expiry"] {
		if aiString(event, "type") == "runtime.offline" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("重复状态查询不应重复广播离线事件: %d", count)
	}
}

func TestAgentTeamSSEEmitsExpiredRuntimeOfflineEvent(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_AGENT_RUNTIME_TOKEN", "runtime-secret")
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	registered := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", `{"projectId":"project-sse-expiry","runtimeId":"runtime-sse-expiry","instanceId":"instance-sse-expiry","memberId":"leader-sse-expiry"}`, true)
	if registered.Code != http.StatusOK {
		t.Fatalf("Agent 注册失败: %d %s", registered.Code, registered.Body.String())
	}
	s := getAIState()
	s.mu.Lock()
	cursor := s.data.TeamSequences["project-sse-expiry"]
	for i, runtime := range s.data.AgentRuntimes {
		if aiString(runtime, "projectId") != "project-sse-expiry" {
			continue
		}
		updated := cloneMap(runtime)
		updated["leaseExpiresAt"] = time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
		s.data.AgentRuntimes[i] = updated
	}
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/v2/projects/project-sse-expiry/events/stream?cursor="+strconv.FormatInt(cursor, 10), nil).WithContext(ctx)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "event: runtime.offline") || !strings.Contains(response.Body.String(), `"reason":"lease_expired"`) {
		t.Fatalf("SSE 未回放过期租约的离线事件: %d %s", response.Code, response.Body.String())
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, runtime := range s.data.AgentRuntimes {
		if aiString(runtime, "projectId") == "project-sse-expiry" && aiString(runtime, "status") != "offline" {
			t.Fatalf("SSE 处理过期租约后未持久化 offline: %#v", runtime)
		}
	}
}

func TestAgentTeamHandoffAndCompletionReport(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_AGENT_RUNTIME_TOKEN", "runtime-secret")
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	registered := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", `{"projectId":"project-collab","runtimeId":"runtime-collab","instanceId":"instance-collab","memberId":"leader-collab"}`, true)
	if registered.Code != http.StatusOK {
		t.Fatalf("Agent 注册失败: %d %s", registered.Code, registered.Body.String())
	}
	var envelope struct {
		Data struct {
			Runtime map[string]any `json:"runtime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	fencing, _ := envelope.Data.Runtime["fencingToken"].(string)
	created := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-collab/tasks", `{"title":"交接任务","instruction":"由分析 Session 交给编码 Session"}`, false)
	if created.Code != http.StatusOK {
		t.Fatalf("任务创建失败: %d %s", created.Code, created.Body.String())
	}
	var taskEnvelope struct {
		Data struct {
			Task map[string]any `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &taskEnvelope); err != nil {
		t.Fatal(err)
	}
	taskID, _ := taskEnvelope.Data.Task["taskId"].(string)
	if taskID == "" {
		t.Fatalf("任务缺少 taskId: %s", created.Body.String())
	}
	events := `{"projectId":"project-collab","runtimeId":"runtime-collab","fencingToken":"` + fencing + `","events":[` +
		`{"sequence":1,"type":"task.handoff","taskId":"` + taskID + `","payload":{"fromSessionId":"planner","toSessionId":"coder","toMemberId":"coder-member","summary":"分析完成，开始实现"}},` +
		`{"sequence":2,"type":"task.completion_report","taskId":"` + taskID + `","payload":{"status":"completed","sessionId":"coder","summary":"实现和测试已完成","verification":"go test ./node/api","artifactIds":["diff-1","test-1"]}}]}`
	reported := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", events, true)
	if reported.Code != http.StatusOK || !strings.Contains(reported.Body.String(), `"accepted":2`) {
		t.Fatalf("交接或完成报告接收失败: %d %s", reported.Code, reported.Body.String())
	}
	task := teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/"+taskID, "", false)
	if task.Code != http.StatusOK {
		t.Fatalf("读取任务失败: %d %s", task.Code, task.Body.String())
	}
	for _, expected := range []string{`"status":"completed"`, `"ownerSessionId":"coder"`, `"ownerMemberId":"coder-member"`, `"completionSource":"agent_report"`, `"artifactIds":["diff-1","test-1"]`} {
		if !strings.Contains(task.Body.String(), expected) {
			t.Fatalf("任务缺少协作结果 %s: %s", expected, task.Body.String())
		}
	}
	invalid := `{"projectId":"project-collab","runtimeId":"runtime-collab","fencingToken":"` + fencing + `","events":[{"sequence":3,"type":"task.completion_report","taskId":"` + taskID + `","payload":{"status":"done"}}]}`
	invalidResponse := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", invalid, true)
	if invalidResponse.Code != http.StatusBadRequest || !strings.Contains(invalidResponse.Body.String(), "AGENT_EVENT_PAYLOAD_INVALID") {
		t.Fatalf("非法完成报告未拒绝: %d %s", invalidResponse.Code, invalidResponse.Body.String())
	}
	validAfterReject := `{"projectId":"project-collab","runtimeId":"runtime-collab","fencingToken":"` + fencing + `","events":[{"sequence":3,"type":"task.completion_report","taskId":"` + taskID + `","payload":{"status":"completed","summary":"重放完成报告"}}]}`
	acceptedAfterReject := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", validAfterReject, true)
	if acceptedAfterReject.Code != http.StatusOK || !strings.Contains(acceptedAfterReject.Body.String(), `"accepted":1`) || !strings.Contains(acceptedAfterReject.Body.String(), `"acceptedThrough":3`) {
		t.Fatalf("非法报告不应消耗 sequence: %d %s", acceptedAfterReject.Code, acceptedAfterReject.Body.String())
	}
}

func TestAgentTeamArtifactMetadataAndEvidence(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_AGENT_RUNTIME_TOKEN", "runtime-secret")
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	registered := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", `{"projectId":"project-artifact","runtimeId":"runtime-artifact","instanceId":"instance-artifact","memberId":"leader-artifact"}`, true)
	if registered.Code != http.StatusOK {
		t.Fatalf("Agent 注册失败: %d %s", registered.Code, registered.Body.String())
	}
	var envelope struct {
		Data struct {
			Runtime map[string]any `json:"runtime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	fencing, _ := envelope.Data.Runtime["fencingToken"].(string)
	created := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-artifact/tasks", `{"title":"证据任务","instruction":"登记测试证据"}`, false)
	if created.Code != http.StatusOK {
		t.Fatalf("任务创建失败: %d %s", created.Code, created.Body.String())
	}
	var taskEnvelope struct {
		Data struct {
			Task map[string]any `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &taskEnvelope); err != nil {
		t.Fatal(err)
	}
	taskID, _ := taskEnvelope.Data.Task["taskId"].(string)
	content := []byte("artifact-body")
	digest := sha256.Sum256(content)
	hash := hex.EncodeToString(digest[:])
	artifactEvent := `{"projectId":"project-artifact","runtimeId":"runtime-artifact","fencingToken":"` + fencing + `","events":[{"sequence":1,"type":"artifact.created","taskId":"` + taskID + `","payload":{"artifactId":"visual-1","kind":"screenshot","sha256":"` + hash + `","size":` + strconv.Itoa(len(content)) + `,"storageRef":"artifact-ref-1","evidenceType":"visual_regression","label":"首页截图"}}]}`
	createdArtifact := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", artifactEvent, true)
	if createdArtifact.Code != http.StatusOK {
		t.Fatalf("Artifact 元数据登记失败: %d %s", createdArtifact.Code, createdArtifact.Body.String())
	}
	artifacts := teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/"+taskID+"/artifacts?page=1&pageSize=10", "", false)
	if artifacts.Code != http.StatusOK || !strings.Contains(artifacts.Body.String(), `"artifactId":"visual-1"`) || !strings.Contains(artifacts.Body.String(), `"sha256":"`+hash+`"`) {
		t.Fatalf("Artifact 查询失败: %d %s", artifacts.Code, artifacts.Body.String())
	}
	evidence := teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/"+taskID+"/evidence", "", false)
	if evidence.Code != http.StatusOK || !strings.Contains(evidence.Body.String(), `"evidenceType":"visual_regression"`) {
		t.Fatalf("Evidence 查询失败: %d %s", evidence.Code, evidence.Body.String())
	}
	initialized := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/artifacts/init", `{"projectId":"project-artifact","runtimeId":"runtime-artifact","fencingToken":"`+fencing+`","taskId":"`+taskID+`","artifactId":"visual-1"}`, true)
	if initialized.Code != http.StatusOK || !strings.Contains(initialized.Body.String(), `"chunkSize":8388608`) {
		t.Fatalf("Artifact 上传初始化失败: %d %s", initialized.Code, initialized.Body.String())
	}
	chunkPath := "/api/v2/agent-runtime/artifacts/visual-1/chunks?projectId=project-artifact&runtimeId=runtime-artifact&taskId=" + taskID + "&offset=0"
	wrongOffset := teamTestRequestWithHeaders(t, mux, http.MethodPut, strings.Replace(chunkPath, "offset=0", "offset=1", 1), "x", map[string]string{
		"X-WorkMesh-Agent-Token":   deriveAgentProjectToken("runtime-secret", "project-artifact"),
		"X-WorkMesh-Fencing-Token": fencing,
		"Content-Type":             "application/octet-stream",
	})
	if wrongOffset.Code != http.StatusConflict || !strings.Contains(wrongOffset.Body.String(), "ARTIFACT_OFFSET_MISMATCH") {
		t.Fatalf("错误 offset 未拒绝: %d %s", wrongOffset.Code, wrongOffset.Body.String())
	}
	chunk := teamTestRequestWithHeaders(t, mux, http.MethodPut, chunkPath, string(content), map[string]string{
		"X-WorkMesh-Agent-Token":   deriveAgentProjectToken("runtime-secret", "project-artifact"),
		"X-WorkMesh-Fencing-Token": fencing,
		"Content-Type":             "application/octet-stream",
	})
	if chunk.Code != http.StatusOK || !strings.Contains(chunk.Body.String(), `"complete":true`) || !strings.Contains(chunk.Body.String(), `"sha256":"`+hash+`"`) {
		t.Fatalf("Artifact 分块上传失败: %d %s", chunk.Code, chunk.Body.String())
	}
	downloaded := teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/"+taskID+"/artifacts/visual-1", "", false)
	if downloaded.Code != http.StatusOK || downloaded.Body.String() != string(content) {
		t.Fatalf("Artifact 下载失败: %d %s", downloaded.Code, downloaded.Body.String())
	}
	verified := teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/"+taskID+"/artifacts/verify", "", false)
	if verified.Code != http.StatusOK || !strings.Contains(verified.Body.String(), `"status":"healthy"`) || !strings.Contains(verified.Body.String(), `"healthy":1`) {
		t.Fatalf("Artifact 完整性核验失败: %d %s", verified.Code, verified.Body.String())
	}
	reconciled := teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/"+taskID+"/artifacts/reconcile", "", false)
	if reconciled.Code != http.StatusOK || !strings.Contains(reconciled.Body.String(), `"destructive":false`) || !strings.Contains(reconciled.Body.String(), `"status":"registered"`) {
		t.Fatalf("Artifact 对账失败: %d %s", reconciled.Code, reconciled.Body.String())
	}
	invalid := `{"projectId":"project-artifact","runtimeId":"runtime-artifact","fencingToken":"` + fencing + `","events":[{"sequence":2,"type":"artifact.created","taskId":"` + taskID + `","payload":{"artifactId":"bad","kind":"log","sha256":"` + hash + `","size":"oops"}}]}`
	invalidResponse := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", invalid, true)
	if invalidResponse.Code != http.StatusBadRequest || !strings.Contains(invalidResponse.Body.String(), "AGENT_EVENT_PAYLOAD_INVALID") {
		t.Fatalf("非法 Artifact 未拒绝: %d %s", invalidResponse.Code, invalidResponse.Body.String())
	}
}

func TestTeamArtifactPathRejectsSymlinkRoot(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	outside := t.TempDir()
	root := filepath.Join(dataDir, "workmesh-artifacts")
	if err := os.Symlink(outside, root); err != nil {
		t.Skipf("当前平台不支持创建符号链接: %v", err)
	}
	if _, err := teamArtifactPath("project", "task", "artifact", ".part"); err == nil {
		t.Fatal("Artifact 存储根目录符号链接未被拒绝")
	}
}

func TestTeamArtifactReadOnlyChecksDoNotCreateDirectories(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	path, err := teamArtifactPathReadOnly("project", "task", "artifact", ".bin")
	if err != nil {
		t.Fatalf("只读路径计算失败: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("只读路径不应创建文件: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "workmesh-artifacts")); !os.IsNotExist(err) {
		t.Fatalf("只读核验不应创建 Artifact 根目录: %v", err)
	}
}

func TestTeamArtifactLifecycleReconcileReportsResidualFiles(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	s := getAIState()
	s.mu.Lock()
	s.data.TeamTasks = []map[string]any{{
		"taskId": "task-lifecycle", "projectId": "project-lifecycle",
		"artifacts": []map[string]any{
			{"artifactId": "registered", "uploadStatus": "complete", "size": int64(3), "sha256": strings.Repeat("a", 64)},
			{"artifactId": "uploading", "uploadStatus": "uploading", "uploadOffset": int64(1), "size": int64(3), "sha256": strings.Repeat("b", 64)},
		},
	}}
	s.mu.Unlock()
	partPath, err := teamArtifactPath("project-lifecycle", "task-lifecycle", "registered", ".part")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(partPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	completedPath, err := teamArtifactPath("project-lifecycle", "task-lifecycle", "uploading", ".bin")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(completedPath, []byte("xxx"), 0o600); err != nil {
		t.Fatal(err)
	}
	orphanPath, err := teamArtifactPath("project-lifecycle", "task-lifecycle", "orphan", ".part")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphanPath, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	verified := teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/task-lifecycle/artifacts/verify", "", false)
	if verified.Code != http.StatusOK || !strings.Contains(verified.Body.String(), `"status":"missing"`) {
		t.Fatalf("缺失完成 Artifact 未被核验报告: %d %s", verified.Code, verified.Body.String())
	}
	reconciled := teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/task-lifecycle/artifacts/reconcile", "", false)
	if reconciled.Code != http.StatusOK {
		t.Fatalf("Artifact 对账失败: %d %s", reconciled.Code, reconciled.Body.String())
	}
	for _, status := range []string{`"status":"partial"`, `"status":"completed_file_without_metadata"`, `"status":"unregistered_partial"`, `"destructive":false`} {
		if !strings.Contains(reconciled.Body.String(), status) {
			t.Fatalf("对账结果缺少 %s: %s", status, reconciled.Body.String())
		}
	}
}

func TestTeamArtifactReconcileReadsAtMostLimitPlusOne(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	s := getAIState()
	s.mu.Lock()
	s.data.TeamTasks = []map[string]any{{"taskId": "task-bounded", "projectId": "project-bounded"}}
	s.mu.Unlock()
	firstPath, err := teamArtifactPath("project-bounded", "task-bounded", "entry-0000", ".part")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(firstPath)
	for i := 0; i <= teamArtifactReconcileMaxFiles; i++ {
		path := filepath.Join(root, fmt.Sprintf("entry-%04d.part", i))
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	response := teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/task-bounded/artifacts/reconcile", "", false)
	if response.Code != http.StatusOK {
		t.Fatalf("对账失败: %d %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Items     []map[string]any `json:"items"`
			Total     int              `json:"total"`
			Truncated bool             `json:"truncated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Data.Truncated || envelope.Data.Total != teamArtifactReconcileMaxFiles || len(envelope.Data.Items) != teamArtifactReconcileMaxFiles {
		t.Fatalf("对账读取上限错误: total=%d items=%d truncated=%t", envelope.Data.Total, len(envelope.Data.Items), envelope.Data.Truncated)
	}
}

func TestProjectArtifactReconcileIsReadOnlyAndProjectScoped(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	s := getAIState()
	s.mu.Lock()
	s.data.TeamTasks = []map[string]any{
		{"taskId": "task-project-a", "projectId": "project-a", "artifacts": []map[string]any{
			{"artifactId": "complete", "uploadStatus": "complete", "size": int64(3)},
			{"artifactId": "uploading", "uploadStatus": "uploading", "size": int64(4)},
		}},
		{"taskId": "task-project-b", "projectId": "project-b", "artifacts": []map[string]any{
			{"artifactId": "other", "uploadStatus": "complete", "size": int64(5)},
		}},
	}
	s.mu.Unlock()
	completePath, err := teamArtifactPath("project-a", "task-project-a", "complete", ".bin")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(completePath, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	partialPath, err := teamArtifactPath("project-a", "task-project-a", "uploading", ".part")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(partialPath, []byte("ab"), 0o600); err != nil {
		t.Fatal(err)
	}
	orphanPartial, err := teamArtifactPath("project-a", "task-project-a", "orphan", ".part")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphanPartial, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	orphanComplete, err := teamArtifactPath("project-a", "task-project-a", "orphan-complete", ".bin")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphanComplete, []byte("xyz"), 0o600); err != nil {
		t.Fatal(err)
	}
	other, err := teamArtifactPath("project-b", "task-project-b", "other", ".bin")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	response := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-a/artifacts/reconcile", "", false)
	if response.Code != http.StatusOK {
		t.Fatalf("项目 Artifact 对账失败: %d %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Items        []map[string]any            `json:"items"`
			ScannedTasks int                         `json:"scannedTasks"`
			ScannedFiles int                         `json:"scannedFiles"`
			Truncated    bool                        `json:"truncated"`
			Summary      map[string]map[string]int64 `json:"summary"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.ScannedTasks != 1 || envelope.Data.ScannedFiles != 4 || envelope.Data.Truncated {
		t.Fatalf("项目范围或扫描计数错误: %+v", envelope.Data)
	}
	for _, status := range []string{"registered_complete", "partial", "unregistered_partial", "unregistered_complete"} {
		if _, ok := envelope.Data.Summary[status]; !ok {
			t.Fatalf("项目对账缺少状态 %s: %s", status, response.Body.String())
		}
	}
	if strings.Contains(response.Body.String(), "project-b") || !strings.Contains(response.Body.String(), `"destructive":false`) {
		t.Fatalf("项目对账越界或缺少只读标记: %s", response.Body.String())
	}
	for _, path := range []string{completePath, partialPath, orphanPartial, orphanComplete, other} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("只读对账不应删除文件 %s: %v", path, err)
		}
	}
}

func TestProjectArtifactReconcileIsBounded(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	s := getAIState()
	s.mu.Lock()
	s.data.TeamTasks = []map[string]any{{"taskId": "task-project-bounded", "projectId": "project-bounded"}}
	s.mu.Unlock()
	firstPath, err := teamArtifactPath("project-bounded", "task-project-bounded", "entry-0000", ".part")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(firstPath)
	for i := 0; i <= teamArtifactProjectReconcileMaxFiles; i++ {
		path := filepath.Join(root, fmt.Sprintf("entry-%04d.part", i))
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	response := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-bounded/artifacts/reconcile", "", false)
	if response.Code != http.StatusOK {
		t.Fatalf("项目对账失败: %d %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Total     int  `json:"total"`
			Truncated bool `json:"truncated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Data.Truncated || envelope.Data.Total != teamArtifactProjectReconcileMaxFiles {
		t.Fatalf("项目对账上限错误: total=%d truncated=%t", envelope.Data.Total, envelope.Data.Truncated)
	}
	plan := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-bounded/artifacts/reclaim-plan", `{}`, false)
	if plan.Code != http.StatusConflict || !strings.Contains(plan.Body.String(), "ARTIFACT_RECLAIM_PLAN_TRUNCATED") {
		t.Fatalf("截断对账不应生成回收计划: %d %s", plan.Code, plan.Body.String())
	}
}

func TestProjectArtifactReconcileCollectsCompletionReferences(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	s := getAIState()
	s.mu.Lock()
	s.data.TeamTasks = []map[string]any{
		{"taskId": "task-ref-a", "projectId": "project-ref", "artifacts": []map[string]any{{"artifactId": "shared", "uploadStatus": "complete", "size": int64(1), "uploadedAt": "2020-01-01T00:00:00Z"}}, "completionReport": map[string]any{"artifactIds": []any{"shared", "unknown"}}, "completionReports": []map[string]any{{"artifactIds": []string{"shared"}}}},
		{"taskId": "task-ref-b", "projectId": "project-ref", "artifacts": []map[string]any{{"artifactId": "shared", "uploadStatus": "complete", "size": int64(1), "uploadedAt": "2020-01-01T00:00:00Z"}}},
	}
	s.mu.Unlock()
	for _, taskID := range []string{"task-ref-a", "task-ref-b"} {
		path, err := teamArtifactPath("project-ref", taskID, "shared", ".bin")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	orphanPath, err := teamArtifactPath("project-ref", "task-ref-a", "orphan", ".bin")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphanPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-teamArtifactRetentionDuration - time.Hour)
	if err := os.Chtimes(orphanPath, old, old); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	response := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-ref/artifacts/reconcile", "", false)
	if response.Code != http.StatusOK {
		t.Fatalf("项目 Artifact 引用对账失败: %d %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			References []map[string]any `json:"references"`
			Items      []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	findReference := func(taskID, artifactID string) map[string]any {
		for _, item := range envelope.Data.References {
			if aiString(item, "taskId") == taskID && aiString(item, "artifactId") == artifactID {
				return item
			}
		}
		return nil
	}
	shared := findReference("task-ref-a", "shared")
	if shared == nil || boundedInt64(shared["referenceCount"], 0, 100) != 3 || !projectArtifactBool(shared["crossTask"]) {
		t.Fatalf("重复/跨任务引用统计错误: %+v", envelope.Data.References)
	}
	unknown := findReference("task-ref-a", "unknown")
	if unknown == nil || aiString(unknown, "fileStatus") != "unknown_reference" {
		t.Fatalf("未知引用未被识别: %+v", unknown)
	}
	foundCandidate := false
	for _, item := range envelope.Data.Items {
		if aiString(item, "artifactId") == "shared" && aiString(item, "taskId") == "task-ref-a" {
			if aiString(item, "retentionState") != "expired" || projectArtifactBool(item["reclaimCandidate"]) {
				t.Fatalf("已有引用的 Artifact 不应成为回收候选: %+v", item)
			}
		}
		if aiString(item, "artifactId") == "orphan" {
			foundCandidate = projectArtifactBool(item["reclaimCandidate"])
		}
	}
	if !foundCandidate {
		t.Fatalf("过保留期且无引用的残留文件应进入 dry-run 候选: %+v", envelope.Data.Items)
	}
}

func TestProjectArtifactReclaimPlanIsApprovalOnlyAndIdempotent(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	s := getAIState()
	s.mu.Lock()
	s.data.TeamTasks = []map[string]any{{
		"taskId": "task-reclaim", "projectId": "project-reclaim",
		"artifacts": []map[string]any{{"artifactId": "old-output", "uploadStatus": "complete", "size": int64(3), "sha256": "", "uploadedAt": "2020-01-01T00:00:00Z"}},
	}}
	s.mu.Unlock()
	path, err := teamArtifactPath("project-reclaim", "task-reclaim", "old-output", ".bin")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-teamArtifactRetentionDuration - time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	created := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-reclaim/artifacts/reclaim-plan", `{"idempotencyKey":"gc-1"}`, false)
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), `"status":"awaiting_approval"`) || !strings.Contains(created.Body.String(), `"destructive":false`) {
		t.Fatalf("回收计划生成失败: %d %s", created.Code, created.Body.String())
	}
	if !strings.Contains(created.Body.String(), `"artifactId":"old-output"`) {
		t.Fatalf("过期无引用 Artifact 未进入计划: %s", created.Body.String())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("生成计划不应删除 Artifact: %v", err)
	}
	replayed := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-reclaim/artifacts/reclaim-plan", `{"idempotencyKey":"gc-1"}`, false)
	if replayed.Code != http.StatusOK || !strings.Contains(replayed.Body.String(), `"replayed":true`) {
		t.Fatalf("重复回收计划未幂等: %d %s", replayed.Code, replayed.Body.String())
	}
	var createdEnvelope struct {
		Data struct {
			Plan struct {
				PlanID string `json:"planId"`
			} `json:"plan"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdEnvelope); err != nil || createdEnvelope.Data.Plan.PlanID == "" {
		t.Fatalf("无法读取回收计划 ID: %s", created.Body.String())
	}
	detail := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-reclaim/artifacts/reclaim-plans/"+createdEnvelope.Data.Plan.PlanID, "", false)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"projectId":"project-reclaim"`) {
		t.Fatalf("回收计划详情错误: %d %s", detail.Code, detail.Body.String())
	}
	for _, invariant := range []string{`"status":"awaiting_approval"`, `"approvalRequired":true`, `"destructive":false`} {
		if !strings.Contains(detail.Body.String(), invariant) {
			t.Fatalf("回收计划状态不变量缺失 %s: %s", invariant, detail.Body.String())
		}
	}
	otherDetail := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/other-project/artifacts/reclaim-plans/"+createdEnvelope.Data.Plan.PlanID, "", false)
	if otherDetail.Code != http.StatusNotFound || !strings.Contains(otherDetail.Body.String(), "ARTIFACT_RECLAIM_PLAN_NOT_FOUND") {
		t.Fatalf("回收计划详情跨项目泄漏: %d %s", otherDetail.Code, otherDetail.Body.String())
	}
	aiState = executionState{}
	reloadedDetail := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-reclaim/artifacts/reclaim-plans/"+createdEnvelope.Data.Plan.PlanID, "", false)
	if reloadedDetail.Code != http.StatusOK || !strings.Contains(reloadedDetail.Body.String(), createdEnvelope.Data.Plan.PlanID) {
		t.Fatalf("回收计划重启后无法读取: %d %s", reloadedDetail.Code, reloadedDetail.Body.String())
	}
	listed := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-reclaim/artifacts/reclaim-plans?page=1&pageSize=10", "", false)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"total":1`) || !strings.Contains(listed.Body.String(), `"projectId":"project-reclaim"`) {
		t.Fatalf("回收计划列表错误: %d %s", listed.Code, listed.Body.String())
	}
	other := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/other-project/artifacts/reclaim-plans", "", false)
	if other.Code != http.StatusOK || !strings.Contains(other.Body.String(), `"total":0`) {
		t.Fatalf("回收计划跨项目泄漏: %d %s", other.Code, other.Body.String())
	}
}

func TestAgentTeamSaveFailureDoesNotExposeTaskOrEvent(t *testing.T) {
	blockedParent := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blockedParent, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_DATA_DIR", blockedParent)
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	s := getAIState()
	s.mu.RLock()
	notification := s.notify
	s.mu.RUnlock()
	created := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-a/tasks", `{"title":"持久化失败","instruction":"不得显示"}`, false)
	if created.Code != http.StatusInternalServerError {
		t.Fatalf("预期持久化失败: %d %s", created.Code, created.Body.String())
	}
	list := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-a/tasks", "", false)
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), "持久化失败") {
		t.Fatalf("失败任务泄露到内存状态: %d %s", list.Code, list.Body.String())
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.data.TeamTasks) != 0 || len(s.data.TeamEvents["project-a"]) != 0 || s.data.TeamSequences["project-a"] != 0 {
		t.Fatal("持久化失败后任务和事件未回滚")
	}
	select {
	case <-notification:
		t.Fatal("持久化失败后错误通知 SSE")
	default:
	}
}

func TestAgentTeamManualCompletionStopsQueuedWorkAndKeepsStatus(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_AGENT_RUNTIME_TOKEN", "runtime-secret")
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	registered := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", `{"projectId":"project-manual","runtimeId":"runtime-manual","instanceId":"instance-manual","memberId":"leader-manual"}`, true)
	if registered.Code != http.StatusOK {
		t.Fatalf("注册失败: %d %s", registered.Code, registered.Body.String())
	}
	var runtimeResponse struct {
		Data struct {
			Runtime map[string]any `json:"runtime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &runtimeResponse); err != nil {
		t.Fatal(err)
	}
	fencing, _ := runtimeResponse.Data.Runtime["fencingToken"].(string)
	created := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-manual/tasks", `{"title":"人工完成","instruction":"尚未执行"}`, false)
	if created.Code != http.StatusOK {
		t.Fatalf("创建失败: %d %s", created.Code, created.Body.String())
	}
	var taskResponse struct {
		Data struct {
			Task map[string]any `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &taskResponse); err != nil {
		t.Fatal(err)
	}
	taskID, _ := taskResponse.Data.Task["taskId"].(string)
	completed := teamTestRequest(t, mux, http.MethodPost, "/api/v2/dev/tasks/"+taskID+"/complete", `{"reason":"用户已经手工修复"}`, false)
	if completed.Code != http.StatusOK || !strings.Contains(completed.Body.String(), `"completionSource":"manual"`) || !strings.Contains(completed.Body.String(), "用户已经手工修复") {
		t.Fatalf("人工完成未记录来源和原因: %d %s", completed.Code, completed.Body.String())
	}
	replayed := teamTestRequest(t, mux, http.MethodPost, "/api/v2/dev/tasks/"+taskID+"/complete", `{}`, false)
	if replayed.Code != http.StatusOK || !strings.Contains(replayed.Body.String(), `"replayed":true`) {
		t.Fatalf("重复人工完成未幂等: %d %s", replayed.Code, replayed.Body.String())
	}
	retry := teamTestRequest(t, mux, http.MethodPost, "/api/v2/dev/tasks/"+taskID+"/retry", `{}`, false)
	if retry.Code != http.StatusConflict {
		t.Fatalf("人工完成后旧任务不能重启: %d %s", retry.Code, retry.Body.String())
	}
	commands := teamTestRequestWithHeaders(t, mux, http.MethodGet, "/api/v2/agent-runtime/commands?projectId=project-manual&runtimeId=runtime-manual", "", map[string]string{"X-WorkMesh-Fencing-Token": fencing, "X-WorkMesh-Agent-Token": deriveAgentProjectToken("runtime-secret", "project-manual")})
	if commands.Code != http.StatusOK || strings.Contains(commands.Body.String(), `"kind":"task.start"`) || !strings.Contains(commands.Body.String(), `"kind":"task.cancel"`) {
		t.Fatalf("人工完成后命令队列错误: %d %s", commands.Code, commands.Body.String())
	}
	late := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", `{"projectId":"project-manual","runtimeId":"runtime-manual","fencingToken":"`+fencing+`","events":[{"sequence":1,"type":"task.failed","taskId":"`+taskID+`"}]}`, true)
	if late.Code != http.StatusOK {
		t.Fatalf("晚到 Agent 事件应保留审计: %d %s", late.Code, late.Body.String())
	}
	task := teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/"+taskID, "", false)
	if task.Code != http.StatusOK || !strings.Contains(task.Body.String(), `"status":"completed"`) || !strings.Contains(task.Body.String(), `"completionSource":"manual"`) {
		t.Fatalf("晚到事件覆盖人工完成: %d %s", task.Code, task.Body.String())
	}
}

func TestAgentTeamCancelSupersedesStartCommand(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_AGENT_RUNTIME_TOKEN", "runtime-secret")
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	registered := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", `{"projectId":"project-cancel","runtimeId":"runtime-cancel","instanceId":"instance-cancel","memberId":"leader-cancel"}`, true)
	if registered.Code != http.StatusOK {
		t.Fatalf("注册失败: %d %s", registered.Code, registered.Body.String())
	}
	var runtimeEnvelope struct {
		Data struct {
			Runtime map[string]any `json:"runtime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &runtimeEnvelope); err != nil {
		t.Fatal(err)
	}
	fencing, _ := runtimeEnvelope.Data.Runtime["fencingToken"].(string)
	created := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-cancel/tasks", `{"title":"待取消","instruction":"不得启动"}`, false)
	if created.Code != http.StatusOK {
		t.Fatalf("创建任务失败: %d %s", created.Code, created.Body.String())
	}
	var taskEnvelope struct {
		Data struct {
			Task map[string]any `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &taskEnvelope); err != nil {
		t.Fatal(err)
	}
	taskID, _ := taskEnvelope.Data.Task["taskId"].(string)
	queueState := getAIState()
	queueState.mu.Lock()
	for i := 1; i < teamPendingCommandLimit; i++ {
		queueState.data.AgentCommands = append(queueState.data.AgentCommands, map[string]any{
			"commandId": fmt.Sprintf("queued-%d", i), "projectId": "project-cancel", "runtimeId": "runtime-cancel", "taskId": taskID, "kind": "task.start", "status": "queued",
		})
	}
	queueState.mu.Unlock()
	cancelled := teamTestRequest(t, mux, http.MethodPost, "/api/v2/dev/tasks/"+taskID+"/cancel", `{"reason":"用户取消"}`, false)
	if cancelled.Code != http.StatusOK || !strings.Contains(cancelled.Body.String(), `"status":"cancelled"`) || !strings.Contains(cancelled.Body.String(), `"cancellationSource":"manual"`) {
		t.Fatalf("取消任务失败: %d %s", cancelled.Code, cancelled.Body.String())
	}
	replayed := teamTestRequest(t, mux, http.MethodPost, "/api/v2/dev/tasks/"+taskID+"/cancel", `{"reason":"重复取消"}`, false)
	if replayed.Code != http.StatusOK || !strings.Contains(replayed.Body.String(), `"replayed":true`) {
		t.Fatalf("重复取消未幂等: %d %s", replayed.Code, replayed.Body.String())
	}
	queueState.mu.RLock()
	pending := pendingTeamCommandsLocked(queueState, "project-cancel")
	queueState.mu.RUnlock()
	if pending != 1 {
		t.Fatalf("取消后应仅保留一条待投递取消命令，实际 %d", pending)
	}
	commands := teamTestRequestWithHeaders(t, mux, http.MethodGet, "/api/v2/agent-runtime/commands?projectId=project-cancel&runtimeId=runtime-cancel", "", map[string]string{"X-WorkMesh-Fencing-Token": fencing, "X-WorkMesh-Agent-Token": deriveAgentProjectToken("runtime-secret", "project-cancel")})
	if commands.Code != http.StatusOK || strings.Contains(commands.Body.String(), `"kind":"task.start"`) || !strings.Contains(commands.Body.String(), `"kind":"task.cancel"`) {
		t.Fatalf("取消后仍可领取启动命令: %d %s", commands.Code, commands.Body.String())
	}
	late := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", `{"projectId":"project-cancel","runtimeId":"runtime-cancel","fencingToken":"`+fencing+`","events":[{"sequence":1,"type":"task.started","taskId":"`+taskID+`"},{"sequence":2,"type":"task.completion_report","taskId":"`+taskID+`","payload":{"status":"completed","summary":"晚到完成"}}]}`, true)
	if late.Code != http.StatusOK || !strings.Contains(late.Body.String(), `"accepted":2`) {
		t.Fatalf("晚到事件应留审计: %d %s", late.Code, late.Body.String())
	}
	afterLate := teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/"+taskID, "", false)
	if afterLate.Code != http.StatusOK || !strings.Contains(afterLate.Body.String(), `"status":"cancelled"`) || strings.Contains(afterLate.Body.String(), "晚到完成") {
		t.Fatalf("晚到事件覆盖人工取消: %d %s", afterLate.Code, afterLate.Body.String())
	}
}

func TestAgentTeamRetryDispatchesNewAttemptAndIgnoresLateEvents(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_AGENT_RUNTIME_TOKEN", "runtime-secret")
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	registered := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", `{"projectId":"project-retry","runtimeId":"runtime-retry","instanceId":"instance-retry","memberId":"leader-retry"}`, true)
	if registered.Code != http.StatusOK {
		t.Fatalf("注册失败: %d %s", registered.Code, registered.Body.String())
	}
	var registration struct {
		Data struct {
			Runtime map[string]any `json:"runtime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &registration); err != nil {
		t.Fatal(err)
	}
	fencing := aiString(registration.Data.Runtime, "fencingToken")
	created := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-retry/tasks", `{"title":"需要重试","instruction":"重新执行"}`, false)
	if created.Code != http.StatusOK {
		t.Fatalf("创建任务失败: %d %s", created.Code, created.Body.String())
	}
	var creation struct {
		Data struct {
			Task map[string]any `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &creation); err != nil {
		t.Fatal(err)
	}
	taskID := aiString(creation.Data.Task, "taskId")
	if boundedInt(creation.Data.Task["attempt"], 0, 10) != 1 {
		t.Fatalf("首次执行轮次无效: %s", created.Body.String())
	}
	invalid := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", fmt.Sprintf(`{"projectId":"project-retry","runtimeId":"runtime-retry","fencingToken":%q,"events":[{"sequence":1,"type":"task.started","taskId":%q,"attempt":"1"}]}`, fencing, taskID), true)
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "AGENT_EVENT_ATTEMPT_INVALID") {
		t.Fatalf("非整数轮次未拒绝: %d %s", invalid.Code, invalid.Body.String())
	}
	first := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", fmt.Sprintf(`{"projectId":"project-retry","runtimeId":"runtime-retry","fencingToken":%q,"events":[{"sequence":1,"type":"task.started","taskId":%q,"attempt":1}]}`, fencing, taskID), true)
	if first.Code != http.StatusOK {
		t.Fatalf("首次开始事件失败: %d %s", first.Code, first.Body.String())
	}
	retried := teamTestRequest(t, mux, http.MethodPost, "/api/v2/dev/tasks/"+taskID+"/retry", `{}`, false)
	if retried.Code != http.StatusOK || !strings.Contains(retried.Body.String(), `"attempt":2`) {
		t.Fatalf("重试未提升轮次: %d %s", retried.Code, retried.Body.String())
	}
	commands := teamTestRequestWithHeaders(t, mux, http.MethodGet, "/api/v2/agent-runtime/commands?projectId=project-retry&runtimeId=runtime-retry", "", map[string]string{"X-WorkMesh-Fencing-Token": fencing, "X-WorkMesh-Agent-Token": deriveAgentProjectToken("runtime-secret", "project-retry")})
	var delivery struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(commands.Body.Bytes(), &delivery); err != nil {
		t.Fatal(err)
	}
	if len(delivery.Data.Items) != 2 || aiString(delivery.Data.Items[0], "kind") != "task.cancel" || aiString(delivery.Data.Items[1], "kind") != "task.start" {
		t.Fatalf("重试应按顺序取消旧执行并启动新执行: %s", commands.Body.String())
	}
	if boundedInt(delivery.Data.Items[0]["payload"].(map[string]any)["attempt"], 0, 10) != 1 || boundedInt(delivery.Data.Items[1]["payload"].(map[string]any)["attempt"], 0, 10) != 2 {
		t.Fatalf("重试命令轮次不匹配: %s", commands.Body.String())
	}
	late := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", fmt.Sprintf(`{"projectId":"project-retry","runtimeId":"runtime-retry","fencingToken":%q,"events":[{"sequence":2,"type":"task.completion_report","taskId":%q,"attempt":1,"payload":{"status":"completed","summary":"旧执行结果"}}]}`, fencing, taskID), true)
	if late.Code != http.StatusOK || !strings.Contains(late.Body.String(), `"accepted":1`) {
		t.Fatalf("旧执行事件未留痕: %d %s", late.Code, late.Body.String())
	}
	current := teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/"+taskID, "", false)
	if !strings.Contains(current.Body.String(), `"status":"accepted"`) || strings.Contains(current.Body.String(), "旧执行结果") {
		t.Fatalf("旧轮次覆盖新任务: %s", current.Body.String())
	}
	legacy := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", fmt.Sprintf(`{"projectId":"project-retry","runtimeId":"runtime-retry","fencingToken":%q,"events":[{"sequence":3,"type":"task.failed","taskId":%q}]}`, fencing, taskID), true)
	if legacy.Code != http.StatusOK {
		t.Fatalf("旧版事件未留审计: %d %s", legacy.Code, legacy.Body.String())
	}
	completed := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", fmt.Sprintf(`{"projectId":"project-retry","runtimeId":"runtime-retry","fencingToken":%q,"events":[{"sequence":4,"type":"task.completed","taskId":%q,"attempt":2}]}`, fencing, taskID), true)
	if completed.Code != http.StatusOK {
		t.Fatalf("新执行完成事件失败: %d %s", completed.Code, completed.Body.String())
	}
	current = teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/"+taskID, "", false)
	if !strings.Contains(current.Body.String(), `"status":"completed"`) {
		t.Fatalf("新执行未完成任务: %s", current.Body.String())
	}
}

func TestAgentTeamCommandLongPollWakesWhenTaskQueued(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_AGENT_RUNTIME_TOKEN", "runtime-secret")
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	registered := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", `{"projectId":"project-poll","runtimeId":"runtime-poll","instanceId":"instance-poll","memberId":"leader-poll"}`, true)
	if registered.Code != http.StatusOK {
		t.Fatalf("注册失败: %d %s", registered.Code, registered.Body.String())
	}
	var runtimeResponse struct {
		Data struct {
			Runtime map[string]any `json:"runtime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &runtimeResponse); err != nil {
		t.Fatal(err)
	}
	fencing, _ := runtimeResponse.Data.Runtime["fencingToken"].(string)
	server := httptest.NewServer(mux)
	defer server.Close()
	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/v2/agent-runtime/commands?projectId=project-poll&runtimeId=runtime-poll&waitSeconds=3", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-WorkMesh-Fencing-Token", fencing)
	request.Header.Set("X-WorkMesh-Agent-Token", deriveAgentProjectToken("runtime-secret", "project-poll"))
	result := make(chan *http.Response, 1)
	go func() {
		response, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			t.Errorf("长轮询失败: %v", requestErr)
			return
		}
		result <- response
	}()
	time.Sleep(100 * time.Millisecond)
	created := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-poll/tasks", `{"title":"长轮询任务","instruction":"唤醒 Agent"}`, false)
	if created.Code != http.StatusOK {
		t.Fatalf("创建任务失败: %d %s", created.Code, created.Body.String())
	}
	select {
	case response := <-result:
		defer response.Body.Close()
		body, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != http.StatusOK || !strings.Contains(string(body), `"kind":"task.start"`) {
			t.Fatalf("长轮询未被任务唤醒: %d %s", response.StatusCode, body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("长轮询未在任务入队后及时返回")
	}
}

func TestAgentTeamTaskListPaginatesWithinProject(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	for _, projectID := range []string{"project-a", "project-b", "project-a", "project-a"} {
		created := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/"+projectID+"/tasks", `{"title":"分页任务","instruction":"检查项目隔离"}`, false)
		if created.Code != http.StatusOK {
			t.Fatalf("创建任务失败: %d %s", created.Code, created.Body.String())
		}
	}
	page := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-a/tasks?page=2&pageSize=2", "", false)
	if page.Code != http.StatusOK {
		t.Fatalf("分页失败: %d %s", page.Code, page.Body.String())
	}
	var envelope struct {
		Data struct {
			Items    []map[string]any `json:"items"`
			Total    int              `json:"total"`
			Page     int              `json:"page"`
			PageSize int              `json:"pageSize"`
		} `json:"data"`
	}
	if err := json.Unmarshal(page.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Total != 3 || envelope.Data.Page != 2 || envelope.Data.PageSize != 2 || len(envelope.Data.Items) != 1 || envelope.Data.Items[0]["projectId"] != "project-a" {
		t.Fatalf("项目隔离分页结果错误: %s", page.Body.String())
	}
}

func TestAgentTeamServerResourceBudgetQueuesAndReleasesTasks(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_AGENT_RUNTIME_TOKEN", "runtime-secret")
	t.Setenv("WORKMESH_AGENT_MAX_RESOURCE_UNITS", "4")
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	registered := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", `{"projectId":"project-budget","runtimeId":"runtime-budget","instanceId":"instance-budget","memberId":"leader-budget"}`, true)
	if registered.Code != http.StatusOK {
		t.Fatalf("注册失败: %d %s", registered.Code, registered.Body.String())
	}
	var runtimeEnvelope struct {
		Data struct {
			Runtime map[string]any `json:"runtime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &runtimeEnvelope); err != nil {
		t.Fatal(err)
	}
	fencing, _ := runtimeEnvelope.Data.Runtime["fencingToken"].(string)
	first := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-budget/tasks", `{"title":"大任务","instruction":"占用预算","resourceProfile":"large"}`, false)
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"status":"accepted"`) {
		t.Fatalf("大任务未接受: %d %s", first.Code, first.Body.String())
	}
	var firstEnvelope struct {
		Data struct {
			Task map[string]any `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstEnvelope); err != nil {
		t.Fatal(err)
	}
	firstID, _ := firstEnvelope.Data.Task["taskId"].(string)
	second := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-budget/tasks", `{"title":"小任务","instruction":"等待预算","resourceProfile":"small"}`, false)
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"status":"waiting_resource"`) || !strings.Contains(second.Body.String(), `"dispatchStatus":"waiting_resource"`) {
		t.Fatalf("资源超限任务未进入等待: %d %s", second.Code, second.Body.String())
	}
	var secondEnvelope struct {
		Data struct {
			Task map[string]any `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondEnvelope); err != nil {
		t.Fatal(err)
	}
	secondID, _ := secondEnvelope.Data.Task["taskId"].(string)
	completed := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", `{"projectId":"project-budget","runtimeId":"runtime-budget","fencingToken":"`+fencing+`","events":[{"sequence":1,"type":"task.completed","taskId":"`+firstID+`"}]}`, true)
	if completed.Code != http.StatusOK {
		t.Fatalf("大任务完成事件失败: %d %s", completed.Code, completed.Body.String())
	}
	updated := teamTestRequest(t, mux, http.MethodGet, "/api/v2/dev/tasks/"+secondID, "", false)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"status":"accepted"`) {
		t.Fatalf("资源释放后任务未接受: %d %s", updated.Code, updated.Body.String())
	}
	state := getAIState()
	state.mu.RLock()
	commandCount := len(state.data.AgentCommands)
	state.mu.RUnlock()
	if commandCount < 2 {
		t.Fatalf("资源释放后未生成第二条命令: %d", commandCount)
	}
	commands := teamTestRequestWithHeaders(t, mux, http.MethodGet, "/api/v2/agent-runtime/commands?projectId=project-budget&runtimeId=runtime-budget", "", map[string]string{"X-WorkMesh-Fencing-Token": fencing, "X-WorkMesh-Agent-Token": deriveAgentProjectToken("runtime-secret", "project-budget")})
	if commands.Code != http.StatusOK || !strings.Contains(commands.Body.String(), `"taskId":"`+secondID+`"`) {
		t.Fatalf("资源释放后任务未派发: %d %s", commands.Code, commands.Body.String())
	}
}

func TestAgentTeamCommandCompactionKeepsActiveCommands(t *testing.T) {
	s := &executionState{data: aiPersistentData{AgentCommands: make([]map[string]any, 0, teamCommandHistoryLimit+3)}}
	for i := 0; i < teamCommandHistoryLimit+3; i++ {
		s.data.AgentCommands = append(s.data.AgentCommands, map[string]any{
			"projectId": "project-compact", "commandId": fmt.Sprintf("old-%d", i), "status": "acked",
		})
	}
	s.data.AgentCommands = append(s.data.AgentCommands,
		map[string]any{"projectId": "project-compact", "commandId": "active-queued", "status": "queued"},
		map[string]any{"projectId": "project-compact", "commandId": "active-delivered", "status": "delivered"},
		map[string]any{"projectId": "other-project", "commandId": "other-terminal", "status": "acked"},
	)
	compactTeamCommandsLocked(s, "project-compact")
	if len(s.data.AgentCommands) != teamCommandHistoryLimit+3 {
		t.Fatalf("命令压缩数量错误: %d", len(s.data.AgentCommands))
	}
	for _, command := range s.data.AgentCommands {
		if aiString(command, "commandId") == "active-queued" || aiString(command, "commandId") == "active-delivered" || aiString(command, "commandId") == "other-terminal" {
			continue
		}
		if aiString(command, "projectId") == "project-compact" && !teamCommandTerminal(command) {
			t.Fatalf("非终态命令被压缩: %#v", command)
		}
	}
}

func TestAgentTeamSSERejectsExpiredCursor(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	s := getAIState()
	s.mu.Lock()
	s.data.TeamEvents["project-cursor"] = []map[string]any{{"sequence": 100, "projectId": "project-cursor", "type": "task.accepted"}}
	s.data.TeamSequences["project-cursor"] = 100
	s.mu.Unlock()
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	response := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-cursor/events/stream?cursor=1", "", false)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "EVENT_CURSOR_EXPIRED") {
		t.Fatalf("过期 cursor 未明确拒绝: %d %s", response.Code, response.Body.String())
	}
}

func TestAgentTeamTaskDependenciesGateAndRelease(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_AGENT_RUNTIME_TOKEN", "runtime-secret")
	aiState = executionState{}
	t.Cleanup(func() { aiState = executionState{} })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	registered := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/register", `{"projectId":"project-dag","runtimeId":"runtime-dag","instanceId":"instance-dag","memberId":"leader-dag"}`, true)
	if registered.Code != http.StatusOK {
		t.Fatalf("注册失败: %d %s", registered.Code, registered.Body.String())
	}
	var runtimeResponse struct {
		Data struct {
			Runtime map[string]any `json:"runtime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &runtimeResponse); err != nil {
		t.Fatal(err)
	}
	fencing, _ := runtimeResponse.Data.Runtime["fencingToken"].(string)
	first := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-dag/tasks", `{"title":"先行任务","instruction":"先完成"}`, false)
	if first.Code != http.StatusOK {
		t.Fatalf("创建先行任务失败: %d %s", first.Code, first.Body.String())
	}
	var firstEnvelope struct {
		Data struct {
			Task map[string]any `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstEnvelope); err != nil {
		t.Fatal(err)
	}
	firstID, _ := firstEnvelope.Data.Task["taskId"].(string)
	second := teamTestRequest(t, mux, http.MethodPost, "/api/v2/projects/project-dag/tasks", `{"title":"后置任务","instruction":"依赖先行任务","dependsOn":["`+firstID+`"]}`, false)
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"status":"waiting_dependency"`) || !strings.Contains(second.Body.String(), `"dispatchStatus":"waiting_dependency"`) {
		t.Fatalf("依赖任务未等待: %d %s", second.Code, second.Body.String())
	}
	var secondEnvelope struct {
		Data struct {
			Task map[string]any `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondEnvelope); err != nil {
		t.Fatal(err)
	}
	secondID, _ := secondEnvelope.Data.Task["taskId"].(string)
	commands := teamTestRequestWithHeaders(t, mux, http.MethodGet, "/api/v2/agent-runtime/commands?projectId=project-dag&runtimeId=runtime-dag", "", map[string]string{"X-WorkMesh-Fencing-Token": fencing, "X-WorkMesh-Agent-Token": deriveAgentProjectToken("runtime-secret", "project-dag")})
	if commands.Code != http.StatusOK || !strings.Contains(commands.Body.String(), `"kind":"task.start"`) {
		t.Fatalf("先行任务未派发: %d %s", commands.Code, commands.Body.String())
	}
	var commandEnvelope struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(commands.Body.Bytes(), &commandEnvelope); err != nil {
		t.Fatal(err)
	}
	firstCommandID, _ := commandEnvelope.Data.Items[0]["commandId"].(string)
	ack := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/commands/ack", `{"projectId":"project-dag","runtimeId":"runtime-dag","fencingToken":"`+fencing+`","commandIds":["`+firstCommandID+`"]}`, true)
	if ack.Code != http.StatusOK {
		t.Fatalf("先行命令确认失败: %d %s", ack.Code, ack.Body.String())
	}
	event := teamTestRequest(t, mux, http.MethodPost, "/api/v2/agent-runtime/events", `{"projectId":"project-dag","runtimeId":"runtime-dag","fencingToken":"`+fencing+`","events":[{"sequence":1,"type":"task.completed","taskId":"`+firstID+`"}]}`, true)
	if event.Code != http.StatusOK {
		t.Fatalf("先行任务完成事件失败: %d %s", event.Code, event.Body.String())
	}
	list := teamTestRequest(t, mux, http.MethodGet, "/api/v2/projects/project-dag/tasks", "", false)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"status":"accepted"`) {
		t.Fatalf("后置任务未释放: %d %s", list.Code, list.Body.String())
	}
	released := teamTestRequestWithHeaders(t, mux, http.MethodGet, "/api/v2/agent-runtime/commands?projectId=project-dag&runtimeId=runtime-dag", "", map[string]string{"X-WorkMesh-Fencing-Token": fencing, "X-WorkMesh-Agent-Token": deriveAgentProjectToken("runtime-secret", "project-dag")})
	if released.Code != http.StatusOK || !strings.Contains(released.Body.String(), `"kind":"task.start"`) || !strings.Contains(released.Body.String(), `"taskId":"`+secondID+`"`) {
		t.Fatalf("后置任务未派发: %d %s", released.Code, released.Body.String())
	}
}

func teamTestRequest(t *testing.T, mux http.Handler, method, path, body string, agent bool) *httptest.ResponseRecorder {
	t.Helper()
	token := ""
	if agent {
		projectID := httptest.NewRequest(method, path, nil).URL.Query().Get("projectId")
		if projectID == "" {
			var payload struct {
				ProjectID string `json:"projectId"`
			}
			if err := json.Unmarshal([]byte(body), &payload); err != nil {
				t.Fatal(err)
			}
			projectID = payload.ProjectID
		}
		token = deriveAgentProjectToken("runtime-secret", projectID)
	}
	return teamTestRequestWithToken(t, mux, method, path, body, token)
}

func teamTestRequestWithToken(t *testing.T, mux http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		request.Header.Set("X-WorkMesh-Agent-Token", token)
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

func teamTestRequestWithHeaders(t *testing.T, mux http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}
