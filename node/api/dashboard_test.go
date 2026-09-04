// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

func TestDashboardRestartRequiresExplicitAuthorization(t *testing.T) {
	t.Setenv("WORKMESH_ALLOW_RESTART", "")
	t.Setenv("WORKMESH_ALLOW_SYSTEM_REBOOT", "")
	rec := httptest.NewRecorder()
	handleDashboardRestart(rec, httptest.NewRequest(http.MethodPost, "/api/v2/dashboard/system/restart/1panel", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("restart without authorization must fail explicitly: %d %s", rec.Code, rec.Body.String())
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
		device := strings.TrimSpace(fmt.Sprint(disk["device"]))
		mount := strings.TrimSpace(fmt.Sprint(disk["path"]))
		if !strings.HasPrefix(device, "/dev/") || strings.Contains(mount, "/docker/") || strings.Contains(mount, "/containerd/") {
			t.Fatalf("dashboard disk entry is not a local disk mount: %#v", disk)
		}
	}
}

func TestDashboardMountFilterKeepsOnlyLocalDiskMounts(t *testing.T) {
	cases := []struct {
		device, filesystem, mount string
		want                      bool
	}{
		{device: "/dev/sda1", filesystem: "ext4", mount: "/www", want: true},
		{device: "/dev/mapper/ubuntu--vg-ubuntu--lv", filesystem: "ext4", mount: "/", want: true},
		{device: "overlay", filesystem: "overlay", mount: "/var/lib/docker/rootfs/overlayfs/id", want: false},
		{device: "tmpfs", filesystem: "tmpfs", mount: "/run", want: false},
		{device: "/dev/sdb2", filesystem: "ext4", mount: "/boot", want: false},
		{device: "/dev/sda1", filesystem: "ext4", mount: "/var/lib/docker/volumes/data", want: false},
		{device: "server:/export", filesystem: "nfs4", mount: "/mnt/share", want: false},
	}
	for _, tc := range cases {
		if got := dashboardShouldIncludeMount(tc.device, tc.filesystem, tc.mount); got != tc.want {
			t.Errorf("dashboardShouldIncludeMount(%q, %q, %q) = %v, want %v", tc.device, tc.filesystem, tc.mount, got, tc.want)
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

func TestDashboardAppLaunchersUseCatalogContract(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	appStoreMu.Lock()
	appStoreInstance = &appStore{
		path: filepath.Join(dataDir, "apps.json"),
		state: appStoreState{
			Catalog: []appRecord{
				{Key: "demo", Name: "Demo", Type: "website", IconURL: "app_demo", Limit: 1, Recommend: 10, Description: "Demo app"},
				{Key: "recommended", Name: "Recommended", Type: "tool", IconURL: "app_recommended", Recommend: 20},
			},
			Apps: []appRecord{{ID: "install-1", Key: "demo", Name: "Demo instance", Version: "1.2.3", Status: "Running", Config: map[string]any{"httpPort": 8080}}},
		},
	}
	appStoreMu.Unlock()
	items := dashboardAppLaunchers()
	if len(items) != 2 {
		t.Fatalf("launcher count = %d, want 2: %#v", len(items), items)
	}
	if got, _ := items[0]["key"].(string); got != "demo" {
		t.Fatalf("installed app should sort first, got %q", got)
	}
	details, ok := items[0]["detail"].([]map[string]any)
	if !ok || len(details) != 1 {
		t.Fatalf("launcher detail contract invalid: %#v", items[0]["detail"])
	}
	if details[0]["installID"] != "install-1" || details[0]["httpPort"] != 8080 {
		t.Fatalf("launcher detail fields invalid: %#v", details[0])
	}
	if installed, _ := items[0]["isInstall"].(bool); !installed {
		t.Fatal("installed launcher must set isInstall=true")
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
