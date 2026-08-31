// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboardOS(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/dashboard/base/os", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	var envelope struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 200 || envelope.Data["platform"] == nil {
		t.Fatalf("unexpected dashboard envelope: %#v", envelope)
	}
}

func TestDashboardCurrentPathParameters(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/dashboard/current/none/none", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
}

func TestDashboardCurrentContractInitializesFrontendFields(t *testing.T) {
	current := dashboardCurrent(nil)
	for _, key := range []string{"cpuPercent", "cpuDetailedPercent", "topCPUItems", "topMemItems", "diskData", "gpuData", "npuData", "xpuData"} {
		if current[key] == nil {
			t.Fatalf("dashboard current missing initialized field %q: %#v", key, current)
		}
	}
	for _, key := range []string{"ioReadBytes", "ioWriteBytes", "ioCount", "ioReadTime", "ioWriteTime", "memoryUsedPercent", "loadUsagePercent"} {
		if _, ok := current[key]; !ok {
			t.Fatalf("dashboard current missing numeric field %q: %#v", key, current)
		}
	}
	// 仪表盘数值会直接进入图表计算；所有浮点字段必须是有限值，避免前端 toFixed/NaN 崩溃。
	for _, key := range []string{"load1", "load5", "load15", "loadUsagePercent", "cpuUsedPercent", "memoryUsedPercent", "swapMemoryUsedPercent"} {
		value, ok := current[key].(float64)
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			t.Fatalf("dashboard numeric field %q is invalid: %#v", key, current[key])
		}
	}
	if perCore, ok := current["cpuPercent"].([]float64); !ok || len(perCore) == 0 {
		t.Fatalf("cpuPercent must contain at least one sample: %#v", current["cpuPercent"])
	}
	if detailed, ok := current["cpuDetailedPercent"].([]float64); !ok || len(detailed) != 8 {
		t.Fatalf("cpuDetailedPercent must contain eight values: %#v", current["cpuDetailedPercent"])
	}
}

func TestDashboardNetworkAndDisks(t *testing.T) {
	network := dashboardNetwork()
	if _, ok := network["bytesSent"]; !ok {
		t.Fatal("network bytesSent field missing")
	}
	if _, ok := network["bytesRecv"]; !ok {
		t.Fatal("network bytesRecv field missing")
	}
	if _, ok := network["supported"]; !ok {
		t.Fatal("network supported field missing")
	}
	disks := dashboardDisks()
	if disks == nil {
		t.Fatal("disk data must be initialized")
	}
	for _, disk := range disks {
		if disk["mount"] == nil || disk["path"] == nil || disk["usedPercent"] == nil || disk["device"] == nil {
			t.Fatalf("disk entry missing identity: %#v", disk)
		}
	}
}

func TestDashboardAcceleratorsExplainUnavailable(t *testing.T) {
	for _, kind := range []string{"gpu", "npu", "xpu"} {
		items := dashboardAccelerators(kind)
		if len(items) != 1 || items[0]["available"] != false || items[0]["reason"] == "" {
			t.Fatalf("unexpected %s capability: %#v", kind, items)
		}
	}
}

func TestDashboardQuickJumpChangePersistsAndFiltersLauncher(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	change := httptest.NewRecorder()
	handleDashboardMutation(change, httptest.NewRequest(http.MethodPost, "/api/v2/dashboard/quick/change", strings.NewReader(`{"quicks":[{"name":"terminal","isShow":true},{"name":"container","isShow":false}]}`)))
	if change.Code != http.StatusOK {
		t.Fatalf("quick change status=%d body=%s", change.Code, change.Body.String())
	}
	list := httptest.NewRecorder()
	handleDashboardQuickOption(list, httptest.NewRequest(http.MethodGet, "/api/v2/dashboard/quick/option", nil))
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), `"name":"container"`) {
		t.Fatalf("hidden quick jump leaked into launcher list: %s", list.Body.String())
	}
	store := getDomainStore()
	store.mu.Lock()
	store.state.Settings = map[string]any{}
	store.mu.Unlock()
	// 重新读取同一数据目录，确认写入不是只驻留在内存。
	functionalStoreMu.Lock()
	functionalStoreInstance = nil
	functionalStoreMu.Unlock()
	list = httptest.NewRecorder()
	handleDashboardQuickOption(list, httptest.NewRequest(http.MethodGet, "/api/v2/dashboard/quick/option", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"name":"terminal"`) || strings.Contains(list.Body.String(), `"name":"container"`) {
		t.Fatalf("persisted quick jump state was not restored: %s", list.Body.String())
	}
}

func TestDashboardLauncherOptionIncludesHiddenState(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	change := httptest.NewRecorder()
	handleDashboardMutation(change, httptest.NewRequest(http.MethodPost, "/api/v2/dashboard/app/launcher/show", strings.NewReader(`{"key":"terminal","value":"Disable"}`)))
	if change.Code != http.StatusOK {
		t.Fatalf("launcher mutation status=%d body=%s", change.Code, change.Body.String())
	}
	options := httptest.NewRecorder()
	handleDashboardLauncherOption(options, httptest.NewRequest(http.MethodPost, "/api/v2/dashboard/app/launcher/option", strings.NewReader(`{"filter":"terminal"}`)))
	if options.Code != http.StatusOK || !strings.Contains(options.Body.String(), `"key":"terminal"`) || !strings.Contains(options.Body.String(), `"isShow":false`) {
		t.Fatalf("launcher option did not expose persisted hidden state: %s", options.Body.String())
	}
}

func TestDashboardQuickJumpChangeRejectsInvalidVisibleCount(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	request := `{"quicks":[{"name":"a","isShow":false},{"name":"b","isShow":false}]}`
	response := httptest.NewRecorder()
	handleDashboardMutation(response, httptest.NewRequest(http.MethodPost, "/api/v2/dashboard/quick/change", strings.NewReader(request)))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "MIN_QUICK_JUMP") {
		t.Fatalf("expected minimum visible validation, status=%d body=%s", response.Code, response.Body.String())
	}
}
