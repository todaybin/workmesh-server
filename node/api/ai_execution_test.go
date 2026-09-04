// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/node/service/taskruntime"
)

type routeTaskBackend struct{}

func (routeTaskBackend) Create(context.Context, taskruntime.TaskSpec) (string, error) {
	return "sandbox-route", nil
}
func (routeTaskBackend) Start(context.Context, string) error   { return nil }
func (routeTaskBackend) Cancel(context.Context, string) error  { return nil }
func (routeTaskBackend) Destroy(context.Context, string) error { return nil }
func (routeTaskBackend) Exec(context.Context, string, []string) (taskruntime.TaskExecResult, error) {
	return taskruntime.TaskExecResult{ExitCode: 0, Stdout: "ok"}, nil
}
func (routeTaskBackend) Collect(context.Context, string) (taskruntime.TaskExecResult, error) {
	return taskruntime.TaskExecResult{ExitCode: 0, Stdout: "collected"}, nil
}

func TestAIProviderAndSandboxStatus(t *testing.T) {
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	provider := httptest.NewRecorder()
	mux.ServeHTTP(provider, httptest.NewRequest(http.MethodGet, "/api/v2/ai/accounts/providers", nil))
	if provider.Code != http.StatusOK || !strings.Contains(provider.Body.String(), `"code":200`) {
		t.Fatalf("provider response: %d %s", provider.Code, provider.Body.String())
	}
	sandbox := httptest.NewRecorder()
	mux.ServeHTTP(sandbox, httptest.NewRequest(http.MethodGet, "/api/v2/cubesandbox/status", nil))
	if sandbox.Code != http.StatusOK || !strings.Contains(sandbox.Body.String(), "available") {
		t.Fatalf("sandbox response: %d %s", sandbox.Code, sandbox.Body.String())
	}
}

func TestAIAccountModelsAndValidation(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/ai/accounts", strings.NewReader(`{"provider":"custom","name":"local","apiType":"openai-completions","models":[{"id":"base","name":"Base"}]}`)))
	if create.Code != http.StatusOK {
		t.Fatalf("create account: %d %s", create.Code, create.Body.String())
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	id, ok := envelope.Data["id"].(string)
	if !ok || id == "" {
		t.Fatalf("missing account id: %v", envelope.Data)
	}
	addBody := fmt.Sprintf(`{"accountId":"%s","model":{"id":"extra","name":"Extra"}}`, id)
	add := httptest.NewRecorder()
	mux.ServeHTTP(add, httptest.NewRequest(http.MethodPost, "/api/v2/ai/accounts/models/create", strings.NewReader(addBody)))
	if add.Code != http.StatusOK {
		t.Fatalf("add model: %d %s", add.Code, add.Body.String())
	}
	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodPost, "/api/v2/ai/accounts/models", strings.NewReader(fmt.Sprintf(`{"accountId":"%s"}`, id))))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "extra") {
		t.Fatalf("list models: %d %s", list.Code, list.Body.String())
	}
	dup := httptest.NewRecorder()
	mux.ServeHTTP(dup, httptest.NewRequest(http.MethodPost, "/api/v2/ai/accounts/models/create", strings.NewReader(addBody)))
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate model status = %d", dup.Code)
	}
	bad := httptest.NewRecorder()
	mux.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/api/v2/ai/accounts/verify", strings.NewReader(`{"provider":"custom","apiType":"openai-completions"}`)))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("verify without key status = %d", bad.Code)
	}
}

func TestAIErrorUsesRequestLocale(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/ai/accounts", strings.NewReader(`{}`))
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("参数错误状态码=%d body=%s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), "provider 和 name 不能为空") {
		t.Fatalf("英文请求不应返回固定中文错误: %s", res.Body.String())
	}
	if !strings.Contains(strings.ToLower(res.Body.String()), "agent") {
		t.Fatalf("英文本地化消息缺少上下文: %s", res.Body.String())
	}
}

func TestAIAccountModelDiscoveryAndSandboxPersistence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"model-a"},{"id":"model-b"}]}`))
	}))
	defer server.Close()
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	discover := httptest.NewRecorder()
	mux.ServeHTTP(discover, httptest.NewRequest(http.MethodPost, "/api/v2/ai/accounts/models/discover", strings.NewReader(fmt.Sprintf(`{"baseURL":"%s/v1","apiKey":"secret"}`, server.URL))))
	if discover.Code != http.StatusOK || !strings.Contains(discover.Body.String(), "model-a") {
		t.Fatalf("discover: %d %s", discover.Code, discover.Body.String())
	}
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux = http.NewServeMux()
	registerAIExecutionRoutes(mux)
	start := httptest.NewRecorder()
	mux.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/api/v2/cubesandbox/start", strings.NewReader(`{"id":"sb-1"}`)))
	_, kvmErr := os.Stat("/dev/kvm")
	if runtime.GOOS == "linux" && kvmErr == nil {
		if start.Code != http.StatusOK {
			t.Fatalf("sandbox start: %d %s", start.Code, start.Body.String())
		}
	} else if start.Code != http.StatusServiceUnavailable {
		t.Fatalf("sandbox unavailable status: %d", start.Code)
	}
	status := httptest.NewRecorder()
	mux.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v2/cubesandbox/status", nil))
	if status.Code != http.StatusOK {
		t.Fatalf("sandbox status: %d", status.Code)
	}
}

func TestTaskExecRequiresToken(t *testing.T) {
	t.Setenv("WORKMESH_TASK_TOKEN", "expected")
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/workmesh/tasks/exec", strings.NewReader(`{"program":"echo","args":["ok"]}`))
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.Code)
	}
}

func TestAIPersistentAccountAndAgentState(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	account := httptest.NewRecorder()
	mux.ServeHTTP(account, httptest.NewRequest(http.MethodPost, "/api/v2/ai/accounts", strings.NewReader(`{"provider":"openai","name":"local","apiKey":"secret"}`)))
	if account.Code != http.StatusOK || !strings.Contains(account.Body.String(), `"code":200`) {
		t.Fatalf("create account: %d %s", account.Code, account.Body.String())
	}
	agent := httptest.NewRecorder()
	mux.ServeHTTP(agent, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents", strings.NewReader(`{"name":"demo-agent","agentType":"hermes-agent"}`)))
	if agent.Code != http.StatusOK || !strings.Contains(agent.Body.String(), "demo-agent") {
		t.Fatalf("create agent: %d %s", agent.Code, agent.Body.String())
	}
	// 新建路由实例模拟进程重启，数据应从 ai.json 恢复。
	mux = http.NewServeMux()
	registerAIExecutionRoutes(mux)
	search := httptest.NewRecorder()
	mux.ServeHTTP(search, httptest.NewRequest(http.MethodPost, "/api/v2/ai/accounts/search", strings.NewReader(`{"page":1,"pageSize":20}`)))
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), "local") {
		t.Fatalf("persistent account: %d %s", search.Code, search.Body.String())
	}
}

func TestAIMcpAndDomainOperations(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/ai/mcp/server", strings.NewReader(`{"id":1,"name":"demo-mcp","baseUrl":"http://127.0.0.1:1"}`)))
	if create.Code != http.StatusOK || !strings.Contains(create.Body.String(), "demo-mcp") {
		t.Fatalf("mcp create: %d %s", create.Code, create.Body.String())
	}
	update := httptest.NewRecorder()
	mux.ServeHTTP(update, httptest.NewRequest(http.MethodPost, "/api/v2/ai/mcp/server/op", strings.NewReader(`{"id":1,"operate":"start"}`)))
	if update.Code != http.StatusOK {
		t.Fatalf("mcp op: %d", update.Code)
	}
	domain := httptest.NewRecorder()
	mux.ServeHTTP(domain, httptest.NewRequest(http.MethodPost, "/api/v2/ai/mcp/domain/bind", strings.NewReader(`{"domain":"mcp.example.test","sslID":2}`)))
	if domain.Code != http.StatusOK || !strings.Contains(domain.Body.String(), "mcp.example.test") {
		t.Fatalf("domain bind: %d %s", domain.Code, domain.Body.String())
	}
}

func TestMCPConnectionTestPerformsNetworkProbe(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/mcp" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18"}}`))
	}))
	defer server.Close()
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/ai/mcp/server", strings.NewReader(fmt.Sprintf(`{"id":"mcp-1","baseUrl":"%s/mcp","outputTransport":"streamableHttp"}`, server.URL))))
	if create.Code != http.StatusOK {
		t.Fatalf("mcp create: %d %s", create.Code, create.Body.String())
	}
	probe := httptest.NewRecorder()
	mux.ServeHTTP(probe, httptest.NewRequest(http.MethodPost, "/api/v2/ai/mcp/server/connection/test", strings.NewReader(`{"id":"mcp-1"}`)))
	if probe.Code != http.StatusOK || !strings.Contains(probe.Body.String(), `"success":true`) {
		t.Fatalf("mcp probe: %d %s", probe.Code, probe.Body.String())
	}
	failed := httptest.NewRecorder()
	mux.ServeHTTP(failed, httptest.NewRequest(http.MethodPost, "/api/v2/ai/mcp/server/connection/test", strings.NewReader(`{"baseUrl":"http://127.0.0.1:1/mcp","outputTransport":"streamableHttp"}`)))
	if failed.Code != http.StatusBadGateway || strings.Contains(failed.Body.String(), `"success":true`) {
		t.Fatalf("failed probe must be explicit: %d %s", failed.Code, failed.Body.String())
	}
}

func TestMCPSSEConnectionRequiresEventStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: ready\\ndata: {}\\n\\n"))
	}))
	defer server.Close()
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	probe := httptest.NewRecorder()
	requestBody := fmt.Sprintf(`{"baseUrl":"%s","outputTransport":"sse"}`, server.URL)
	mux.ServeHTTP(probe, httptest.NewRequest(http.MethodPost, "/api/v2/ai/mcp/server/connection/test", strings.NewReader(requestBody)))
	if probe.Code != http.StatusOK || !strings.Contains(probe.Body.String(), `"success":true`) {
		t.Fatalf("sse probe: %d %s", probe.Code, probe.Body.String())
	}
}

func TestMCPSyncStatusPersistsProbeResult(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer server.Close()
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/ai/mcp/server", strings.NewReader(fmt.Sprintf(`{"id":"mcp-sync","baseUrl":"%s","outputTransport":"streamableHttp"}`, server.URL))))
	if create.Code != http.StatusOK {
		t.Fatalf("create: %d %s", create.Code, create.Body.String())
	}
	syncResult := httptest.NewRecorder()
	mux.ServeHTTP(syncResult, httptest.NewRequest(http.MethodPost, "/api/v2/ai/mcp/server/status/sync", strings.NewReader(`{"ids":["mcp-sync"]}`)))
	if syncResult.Code != http.StatusOK || !strings.Contains(syncResult.Body.String(), `"status":"running"`) {
		t.Fatalf("sync: %d %s", syncResult.Code, syncResult.Body.String())
	}
	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodPost, "/api/v2/ai/mcp/search", strings.NewReader(`{}`)))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"status":"running"`) {
		t.Fatalf("persisted status: %d %s", list.Code, list.Body.String())
	}
}

func TestAgentPairingRequiresRegisteredRuntime(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents", strings.NewReader(`{"id":"agent-pair","name":"pair-test","agentType":"openclaw"}`)))
	if create.Code != http.StatusOK {
		t.Fatalf("create agent: %d %s", create.Code, create.Body.String())
	}
	invalid := httptest.NewRecorder()
	mux.ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents/channel/pairing/approve", strings.NewReader(`{"agentId":"agent-pair","type":"unknown","pairingCode":"x"}`)))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid pairing status = %d", invalid.Code)
	}
	missingRuntime := httptest.NewRecorder()
	mux.ServeHTTP(missingRuntime, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents/channel/pairing/approve", strings.NewReader(`{"agentId":"agent-pair","type":"telegram","pairingCode":"AB-12"}`)))
	if missingRuntime.Code != http.StatusServiceUnavailable || strings.Contains(missingRuntime.Body.String(), `"accepted":true`) {
		t.Fatalf("missing runtime must be explicit: %d %s", missingRuntime.Code, missingRuntime.Body.String())
	}
}

func TestAgentPluginAndSkillWritesRequireRuntime(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	for _, path := range []string{"agents/plugin/install", "agents/plugin/upgrade", "agents/plugin/uninstall", "agents/skills/install", "agents/skills/update", "agents/skills/uninstall", "agents/plugins/operate"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/ai/"+path, strings.NewReader(`{"id":"item-1"}`)))
		if res.Code != http.StatusServiceUnavailable || strings.Contains(res.Body.String(), `"code":200`) {
			t.Fatalf("%s must require runtime: %d %s", path, res.Code, res.Body.String())
		}
	}
}

func TestAIAgentCollectionQueriesUsePersistedState(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents", strings.NewReader(`{"id":"agent-1","name":"demo-agent","accountId":"account-1","channels":["feishu"]}`)))
	if create.Code != http.StatusOK {
		t.Fatalf("create agent: %d %s", create.Code, create.Body.String())
	}
	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents/agent/list", strings.NewReader(`{"page":1,"pageSize":20}`)))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "agent-1") {
		t.Fatalf("agent list: %d %s", list.Code, list.Body.String())
	}
	channels := httptest.NewRecorder()
	mux.ServeHTTP(channels, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents/agent/channels", strings.NewReader(`{}`)))
	if channels.Code != http.StatusOK || !strings.Contains(channels.Body.String(), `"bound":true`) {
		t.Fatalf("channel binding: %d %s", channels.Code, channels.Body.String())
	}
	overview := httptest.NewRecorder()
	mux.ServeHTTP(overview, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents/overview", strings.NewReader(`{}`)))
	if overview.Code != http.StatusOK || !strings.Contains(overview.Body.String(), `"agentCount":1`) {
		t.Fatalf("overview: %d %s", overview.Code, overview.Body.String())
	}
	refs := httptest.NewRecorder()
	mux.ServeHTTP(refs, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents/delete/check", strings.NewReader(`{"accountId":"account-1"}`)))
	if refs.Code != http.StatusOK || !strings.Contains(refs.Body.String(), "agent-1") {
		t.Fatalf("delete references: %d %s", refs.Code, refs.Body.String())
	}
}

func TestAgentResourceMutationsAndSessionLifecycle(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents", strings.NewReader(`{"id":"agent-resource","name":"resource"}`)))
	if create.Code != http.StatusOK {
		t.Fatalf("create agent: %d %s", create.Code, create.Body.String())
	}
	remark := httptest.NewRecorder()
	mux.ServeHTTP(remark, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents/remark", strings.NewReader(`{"agentId":"agent-resource","remark":"updated"}`)))
	if remark.Code != http.StatusOK || !strings.Contains(remark.Body.String(), "updated") {
		t.Fatalf("remark: %d %s", remark.Code, remark.Body.String())
	}
	bind := httptest.NewRecorder()
	mux.ServeHTTP(bind, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents/website/bind", strings.NewReader(`{"agentId":"agent-resource","websiteId":"site-1"}`)))
	if bind.Code != http.StatusOK || !strings.Contains(bind.Body.String(), "site-1") {
		t.Fatalf("website bind: %d %s", bind.Code, bind.Body.String())
	}
	role := httptest.NewRecorder()
	mux.ServeHTTP(role, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents/agent/create", strings.NewReader(`{"agentId":"agent-resource","name":"primary","model":"m1"}`)))
	if role.Code != http.StatusOK || !strings.Contains(role.Body.String(), "primary") {
		t.Fatalf("role create: %d %s", role.Code, role.Body.String())
	}
	roles := httptest.NewRecorder()
	mux.ServeHTTP(roles, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents/agent/list", strings.NewReader(`{"agentId":"agent-resource"}`)))
	if roles.Code != http.StatusOK || !strings.Contains(roles.Body.String(), "primary") {
		t.Fatalf("role list: %d %s", roles.Code, roles.Body.String())
	}
	token := httptest.NewRecorder()
	mux.ServeHTTP(token, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents/token/reset", strings.NewReader(`{"id":"agent-resource"}`)))
	if token.Code != http.StatusOK || strings.Contains(token.Body.String(), "wm_") {
		t.Fatalf("token reset must not expose token: %d %s", token.Code, token.Body.String())
	}
	// 会话改动必须在通用 /delete 分支前处理，并拒绝不存在的会话。
	s := getAIState()
	s.mu.Lock()
	s.data.Sessions["agent-resource"] = []map[string]any{{"id": "session-1", "title": "old"}}
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	s.mu.Unlock()
	rename := httptest.NewRecorder()
	mux.ServeHTTP(rename, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents/hermes/chat/sessions/rename", strings.NewReader(`{"agentId":"agent-resource","id":"session-1","title":"new"}`)))
	if rename.Code != http.StatusOK {
		t.Fatalf("session rename: %d %s", rename.Code, rename.Body.String())
	}
	remove := httptest.NewRecorder()
	mux.ServeHTTP(remove, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents/hermes/chat/sessions/delete", strings.NewReader(`{"agentId":"agent-resource","id":"session-1"}`)))
	if remove.Code != http.StatusOK {
		t.Fatalf("session delete: %d %s", remove.Code, remove.Body.String())
	}
	missing := httptest.NewRecorder()
	mux.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/api/v2/ai/agents/hermes/chat/sessions/delete", strings.NewReader(`{"agentId":"agent-resource","id":"missing"}`)))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing session status = %d", missing.Code)
	}
}

func TestAIResourceOperationsRequireExistingResource(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	for _, tc := range []struct {
		path string
		body string
	}{
		{"/api/v2/ai/ollama/close", `{"name":"missing"}`},
		{"/api/v2/ai/ollama/model/load", `{"name":"missing"}`},
		{"/api/v2/ai/mcp/server/op", `{"id":"missing","operate":"start"}`},
	} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body)))
		if res.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, body=%s", tc.path, res.Code, res.Body.String())
		}
	}
}

func TestWorkMeshTaskRoutesUseIsolatedProvider(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_TASK_TOKEN", "task-token")
	provider, err := taskruntime.NewTaskProvider(routeTaskBackend{})
	if err != nil {
		t.Fatal(err)
	}
	SetTaskProvider(provider)
	t.Cleanup(func() { SetTaskProvider(nil) })
	mux := http.NewServeMux()
	registerAIExecutionRoutes(mux)
	valid := `{"taskId":"route-task","imageDigest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","worktree":"/srv/workmesh/project","entrypoint":["/opt/workmesh/task-bootstrap"]}`
	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/api/v2/workmesh/tasks/create", strings.NewReader(valid)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}
	request := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v2/workmesh/tasks/"+path, strings.NewReader(body))
		req.Header.Set("X-WorkMesh-Token", "task-token")
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, req)
		return response
	}
	created := request("create", valid)
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), "sandbox-route") {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	duplicate := request("create", valid)
	if duplicate.Code != http.StatusBadRequest || !strings.Contains(duplicate.Body.String(), "已存在") {
		t.Fatalf("duplicate = %d %s", duplicate.Code, duplicate.Body.String())
	}
	started := request("start", `{"taskId":"route-task"}`)
	if started.Code != http.StatusOK || !strings.Contains(started.Body.String(), "route-task") {
		t.Fatalf("start = %d %s", started.Code, started.Body.String())
	}
	executed := request("exec", `{"taskId":"route-task","argv":["run"]}`)
	if executed.Code != http.StatusOK || !strings.Contains(executed.Body.String(), "ok") {
		t.Fatalf("exec = %d %s", executed.Code, executed.Body.String())
	}
	collected := request("collect", `{"taskId":"route-task"}`)
	if collected.Code != http.StatusOK || !strings.Contains(collected.Body.String(), "collected") {
		t.Fatalf("collect = %d %s", collected.Code, collected.Body.String())
	}
	unknown := request("start", `{"taskId":"missing"}`)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown task status = %d", unknown.Code)
	}
}
