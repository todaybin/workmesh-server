// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
