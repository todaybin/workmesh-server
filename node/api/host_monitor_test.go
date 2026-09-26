// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

func TestHostMonitorLegacySettingsKeepStoreDays(t *testing.T) {
	settings := normalizeHostMonitorSettings(map[string]any{"enabled": true, "interval": 10, "key": "MonitorStoreDays", "value": "30"})
	if settings.MonitorStatus != "Enable" || settings.MonitorStoreDays != "30" || settings.MonitorInterval != "300" || settings.DefaultNetwork != "all" {
		t.Fatalf("legacy settings were not normalized: %+v", settings)
	}
}

func TestHostMonitorSampleSearchAndClean(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("monitor sampling reads proc")
	}
	resetSharedStoreForTest()
	store, err := storage.Open(filepath.Join(t.TempDir(), "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		resetSharedStoreForTest()
		_ = store.Close()
	})
	if err := storage.ApplyMigrations(context.Background(), store.DB(), []storage.Migration{HostMonitorMigration()}); err != nil {
		t.Fatal(err)
	}
	controlStoreMu.Lock()
	controlStoreDB = store.DB()
	controlStoreMu.Unlock()

	if err := sampleHostMonitor(context.Background()); err != nil {
		t.Fatalf("sample: %v", err)
	}
	data, err := searchHostMonitor(hostMonitorSearch{Param: "all", IO: "all", Network: "all", StartTime: time.Now().Add(-time.Hour), EndTime: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, item := range data {
		param := item["param"].(string)
		values, _ := item["value"].([]any)
		seen[param] = len(values)
	}
	if seen["base"] < 1 || seen["io"] < 1 || seen["network"] < 1 {
		t.Fatalf("monitor series missing: %#v", data)
	}
	if err := cleanHostMonitor(); err != nil {
		t.Fatal(err)
	}
	data, err = searchHostMonitor(hostMonitorSearch{Param: "all", StartTime: time.Now().Add(-time.Hour), EndTime: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range data {
		values, _ := item["value"].([]any)
		if len(values) != 0 {
			t.Fatalf("clean left %v rows in %v", len(values), item["param"])
		}
	}
}

func TestHostMonitorNetworkOptionsIncludeAll(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v2/hosts/monitor/netoptions", nil))
	var envelope struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data) == 0 || envelope.Data[0] != "all" {
		t.Fatalf("netoptions = %#v", envelope.Data)
	}
}

func TestDashboardIOKeepsSelectedPartition(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("diskstats is linux-only")
	}
	data, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		t.Fatal(err)
	}
	partition := ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}
		name := fields[2]
		if dashboardIsPartition(name) && !strings.HasPrefix(name, "loop") && !strings.HasPrefix(name, "ram") {
			partition = name
			break
		}
	}
	if partition == "" {
		t.Skip("no partition in diskstats")
	}
	io := dashboardIO(partition)
	if _, ok := io["readBytes"].(uint64); !ok {
		t.Fatalf("selected partition %s was not counted: %#v", partition, io)
	}
}
