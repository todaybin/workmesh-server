// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/node/service/taskruntime"
	"github.com/todaybin/workmesh-server/runtime/gateway"
)

type limitTestTaskProvider struct {
	cancelled bool
}

func (p *limitTestTaskProvider) Create(context.Context, taskruntime.TaskSpec) (taskruntime.TaskHandle, error) {
	return taskruntime.TaskHandle{}, nil
}
func (p *limitTestTaskProvider) Start(context.Context, string) error { return nil }
func (p *limitTestTaskProvider) Exec(context.Context, string, []string) (taskruntime.TaskExecResult, error) {
	return taskruntime.TaskExecResult{}, nil
}
func (p *limitTestTaskProvider) Cancel(context.Context, string) error {
	p.cancelled = true
	return nil
}
func (p *limitTestTaskProvider) Collect(context.Context, string) (taskruntime.TaskExecResult, error) {
	return taskruntime.TaskExecResult{}, nil
}
func (p *limitTestTaskProvider) Destroy(context.Context, string) error { return nil }

func TestAppCatalogLazyLoadExpiresAndReadOnlySearchDoesNotRewriteState(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(catalogPath, []byte(`{"version":"v1","apps":[{"id":"demo","key":"demo","name":"Demo","version":"1.0.0"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_DATA_DIR", dir)
	t.Setenv("WORKMESH_APP_CATALOG", catalogPath)
	resetAppStoreForTest()
	if err := SetRuntimeLimits(RuntimeLimits{MaxConcurrentTasks: 1, MaxConcurrentConversions: 1, MaxSSEStreams: 1, MaxAIJobs: 1, MaxLogBytes: 1024, CacheTTL: 20 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		resetAppStoreForTest()
		_ = SetRuntimeLimits(RuntimeLimits{MaxConcurrentTasks: 4, MaxConcurrentConversions: 2, MaxSSEStreams: 64, MaxAIJobs: 4, MaxLogBytes: 8 << 20, CacheTTL: 5 * time.Minute})
	})
	store := getAppStore()
	if store.catalogLoaded || len(store.state.Catalog) != 0 {
		t.Fatal("catalog loaded during store startup")
	}
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	request := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v2/apps/search", bytes.NewBufferString(`{"page":1,"pageSize":10}`)))
		return response
	}
	if response := request(); response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"demo"`)) {
		t.Fatalf("first catalog response=%d %s", response.Code, response.Body.String())
	}
	store.mu.Lock()
	store.state.Apps = []appRecord{{ID: "installed-demo", Key: "demo", Name: "Demo", Version: "0.9.0", Status: "running"}}
	store.mu.Unlock()
	installed := httptest.NewRecorder()
	mux.ServeHTTP(installed, httptest.NewRequest(http.MethodGet, "/api/v2/apps/installed/list", nil))
	if installed.Code != http.StatusOK || !bytes.Contains(installed.Body.Bytes(), []byte(`"canUpdate":true`)) {
		t.Fatalf("installed catalog metadata response=%d %s", installed.Code, installed.Body.String())
	}
	cachePath := filepath.Join(dir, "app-catalog.json")
	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	firstModTime := info.ModTime()
	time.Sleep(5 * time.Millisecond)
	if response := request(); response.Code != http.StatusOK {
		t.Fatalf("second catalog response=%d %s", response.Code, response.Body.String())
	}
	info, err = os.Stat(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(firstModTime) {
		t.Fatal("read-only catalog search rewrote cache")
	}
	time.Sleep(50 * time.Millisecond)
	store.mu.RLock()
	loaded, count, tags := store.catalogLoaded, len(store.state.Catalog), len(store.state.CatalogTags)
	store.mu.RUnlock()
	if loaded || count != 0 || tags != 0 {
		t.Fatalf("expired catalog retained: loaded=%v count=%d tags=%d", loaded, count, tags)
	}
}

func TestAIStartPersistenceFailureCancelsTaskAndReleasesSlot(t *testing.T) {
	dataFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(dataFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_DATA_DIR", dataFile)
	if err := SetRuntimeLimits(RuntimeLimits{MaxConcurrentTasks: 1, MaxConcurrentConversions: 1, MaxSSEStreams: 1, MaxAIJobs: 1, MaxLogBytes: 1024, CacheTTL: time.Minute}); err != nil {
		t.Fatal(err)
	}
	managedRuntimeSlots.Lock()
	managedRuntimeSlots.aiJobs = make(map[string]func())
	managedRuntimeSlots.Unlock()
	t.Cleanup(func() {
		managedRuntimeSlots.Lock()
		managedRuntimeSlots.aiJobs = make(map[string]func())
		managedRuntimeSlots.Unlock()
		_ = SetRuntimeLimits(RuntimeLimits{MaxConcurrentTasks: 4, MaxConcurrentConversions: 2, MaxSSEStreams: 64, MaxAIJobs: 4, MaxLogBytes: 8 << 20, CacheTTL: 5 * time.Minute})
	})
	provider := &limitTestTaskProvider{}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/cubesandbox/start", strings.NewReader(`{"taskId":"limit-task"}`))
	operateSandboxTask(response, request, provider, "start")
	if response.Code != http.StatusInternalServerError || !provider.cancelled {
		t.Fatalf("response=%d body=%s cancelled=%v", response.Code, response.Body.String(), provider.cancelled)
	}
	release, ok := tryRuntimeSlot(nodeRuntimeLimits.aiJobs)
	if !ok {
		t.Fatal("AI 任务持久化失败后未释放并发槽")
	}
	release()
}

func TestRuntimeLimitExhaustion(t *testing.T) {
	state := newRuntimeLimitState(RuntimeLimits{MaxConcurrentTasks: 1, MaxConcurrentConversions: 1, MaxSSEStreams: 1, MaxAIJobs: 1, MaxLogBytes: 1, CacheTTL: time.Second})
	release, ok := tryRuntimeSlot(state.tasks)
	if !ok {
		t.Fatal("first slot rejected")
	}
	if _, ok := tryRuntimeSlot(state.tasks); ok {
		t.Fatal("limit exhaustion accepted")
	}
	release()
	if releaseAgain, ok := tryRuntimeSlot(state.tasks); !ok {
		t.Fatal("released slot unavailable")
	} else {
		releaseAgain()
	}
}

func TestRemoteResourcePolicyCanOnlyTightenLocalLimit(t *testing.T) {
	original := nodeRuntimeLimits
	nodeRuntimeLimits = newRuntimeLimitState(RuntimeLimits{MaxConcurrentTasks: 4, MaxConcurrentConversions: 2, MaxSSEStreams: 8, MaxAIJobs: 3, MaxLogBytes: 1, CacheTTL: time.Second})
	t.Cleanup(func() { nodeRuntimeLimits = original })
	if err := ApplyRemoteResourcePolicy(gateway.ResourcePolicy{Mode: "enforce", MaxConcurrentTasks: 2}, 1); err != nil {
		t.Fatal(err)
	}
	if got := effectiveRuntimeLimit("tasks"); got != 2 {
		t.Fatalf("Gateway 收紧策略未生效: %d", got)
	}
	if err := ApplyRemoteResourcePolicy(gateway.ResourcePolicy{Mode: "enforce", MaxConcurrentTasks: 20}, 2); err != nil {
		t.Fatal(err)
	}
	if got := effectiveRuntimeLimit("tasks"); got != 4 {
		t.Fatalf("Gateway 不得放宽本地硬上限: %d", got)
	}
}
