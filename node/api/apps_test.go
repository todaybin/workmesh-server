// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func resetAppStoreForTest() {
	appStoreMu.Lock()
	appStoreInstance = nil
	appStoreMu.Unlock()
}

func TestAppInstallAndList(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	install := httptest.NewRecorder()
	mux.ServeHTTP(install, httptest.NewRequest(http.MethodPost, "/api/v2/apps/install", strings.NewReader(`{"id":"demo","name":"Demo","version":"1.0"}`)))
	if install.Code != http.StatusOK {
		t.Fatalf("install status=%d", install.Code)
	}
	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodPost, "/api/v2/apps/installed/search", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "demo") {
		t.Fatalf("list body=%s", list.Body.String())
	}
}

func TestAppInstalledCheckUsesEnvironmentProbe(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	binDir := t.TempDir()
	name, content := "mysqld", "#!/bin/sh\nprintf 'mysqld 8.4.0'\n"
	perm := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		name, content, perm = "mysqld.cmd", "@echo mysqld 8.4.0\r\n", 0o644
	}
	if err := os.WriteFile(filepath.Join(binDir, name), []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	resetAppStoreForTest()
	defer resetAppStoreForTest()
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v2/apps/installed/check", strings.NewReader(`{"key":"mysql","name":"mysql"}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"isExist":true`) || !strings.Contains(rec.Body.String(), `"version":"8.4.0"`) {
		t.Fatalf("environment probe contract mismatch: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAppOperationsAndCatalog(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	install := httptest.NewRecorder()
	mux.ServeHTTP(install, httptest.NewRequest(http.MethodPost, "/api/v2/apps/install", strings.NewReader(`{"id":"42","key":"demo","name":"Demo","version":"1.0","port":8080}`)))
	if install.Code != http.StatusOK || !strings.Contains(install.Body.String(), "running") {
		t.Fatalf("install: %d %s", install.Code, install.Body.String())
	}
	port := httptest.NewRecorder()
	mux.ServeHTTP(port, httptest.NewRequest(http.MethodPost, "/api/v2/apps/installed/port/change", strings.NewReader(`{"installID":"42","port":9090}`)))
	if port.Code != http.StatusOK {
		t.Fatalf("port update: %d", port.Code)
	}
	info := httptest.NewRecorder()
	mux.ServeHTTP(info, httptest.NewRequest(http.MethodGet, "/api/v2/apps/installed/info/42", nil))
	if info.Code != http.StatusOK || !strings.Contains(info.Body.String(), "9090") {
		t.Fatalf("info: %d %s", info.Code, info.Body.String())
	}
	stop := httptest.NewRecorder()
	mux.ServeHTTP(stop, httptest.NewRequest(http.MethodPost, "/api/v2/apps/installed/op", strings.NewReader(`{"installId":"42","operate":"stop"}`)))
	if stop.Code != http.StatusOK || !strings.Contains(stop.Body.String(), "stopped") {
		t.Fatalf("stop: %d %s", stop.Code, stop.Body.String())
	}
	icon := httptest.NewRecorder()
	mux.ServeHTTP(icon, httptest.NewRequest(http.MethodGet, "/api/v2/apps/icon/demo", nil))
	if icon.Code != http.StatusOK || icon.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("icon: %d %s", icon.Code, icon.Header().Get("Content-Type"))
	}
}

func TestAppDerivedDetailsAndDeleteCheck(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	install := httptest.NewRecorder()
	mux.ServeHTTP(install, httptest.NewRequest(http.MethodPost, "/api/v2/apps/install", strings.NewReader(`{"id":"99","key":"demo","name":"Demo","version":"1.2","containerName":"demo-web","params":{"port":8080}}`)))
	if install.Code != http.StatusOK {
		t.Fatalf("install status=%d", install.Code)
	}
	detail := httptest.NewRecorder()
	mux.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "/api/v2/apps/details/99", nil))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "port") {
		t.Fatalf("detail body=%s", detail.Body.String())
	}
	check := httptest.NewRecorder()
	mux.ServeHTTP(check, httptest.NewRequest(http.MethodGet, "/api/v2/apps/installed/delete/check/99", nil))
	if check.Code != http.StatusOK || !strings.Contains(check.Body.String(), "demo-web") {
		t.Fatalf("delete check body=%s", check.Body.String())
	}
	services := httptest.NewRecorder()
	mux.ServeHTTP(services, httptest.NewRequest(http.MethodGet, "/api/v2/apps/services/demo", nil))
	if services.Code != http.StatusOK || !strings.Contains(services.Body.String(), "running") {
		t.Fatalf("services body=%s", services.Body.String())
	}
}

func TestAppCheckUpdateUsesConfiguredCatalogAndPersistsMetadata(t *testing.T) {
	dataDir := t.TempDir()
	catalogPath := filepath.Join(dataDir, "catalog.json")
	if err := os.WriteFile(catalogPath, []byte(`{
  "version": "2026.08",
  "lastModified": 1700000000,
  "apps": [
    {"id":"demo-v1","key":"demo","name":"Demo","version":"1.3.0"},
    {"id":"demo-v2","key":"demo","name":"Demo","version":"1.10.0"}
  ]
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	t.Setenv("WORKMESH_APP_CATALOG", catalogPath)
	resetAppStoreForTest()
	defer resetAppStoreForTest()
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	install := httptest.NewRecorder()
	mux.ServeHTTP(install, httptest.NewRequest(http.MethodPost, "/api/v2/apps/install", strings.NewReader(`{"id":"installed-demo","key":"demo","name":"Demo","version":"1.2.0"}`)))
	if install.Code != http.StatusOK {
		t.Fatalf("install status=%d body=%s", install.Code, install.Body.String())
	}
	check := httptest.NewRecorder()
	mux.ServeHTTP(check, httptest.NewRequest(http.MethodGet, "/api/v2/apps/checkupdate", nil))
	if check.Code != http.StatusOK {
		t.Fatalf("check status=%d body=%s", check.Code, check.Body.String())
	}
	body := check.Body.String()
	for _, want := range []string{`"canUpdate":true`, `"latestVersion":"1.10.0"`, `"appStoreLastModified":1700000000`, `"appStoreVersion":"2026.08"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("check body missing %s: %s", want, body)
		}
	}
	// 重启后清除目录环境，仍应使用 apps.json 中持久化的目录快照。
	t.Setenv("WORKMESH_APP_CATALOG", "")
	resetAppStoreForTest()
	mux = http.NewServeMux()
	RegisterAppRoutes(mux)
	check = httptest.NewRecorder()
	mux.ServeHTTP(check, httptest.NewRequest(http.MethodGet, "/api/v2/apps/checkupdate", nil))
	if check.Code != http.StatusOK || !strings.Contains(check.Body.String(), `"latestVersion":"1.10.0"`) {
		t.Fatalf("persisted catalog check status=%d body=%s", check.Code, check.Body.String())
	}
}

func TestAppCheckUpdateNoUpdateForEquivalentVersions(t *testing.T) {
	dataDir := t.TempDir()
	catalogPath := filepath.Join(dataDir, "catalog.json")
	if err := os.WriteFile(catalogPath, []byte(`[{"id":"demo","key":"demo","name":"Demo","version":"v1.10.0"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	t.Setenv("WORKMESH_APP_CATALOG", catalogPath)
	resetAppStoreForTest()
	defer resetAppStoreForTest()
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v2/apps/install", strings.NewReader(`{"id":"demo","key":"demo","name":"Demo","version":"1.10"}`)))
	check := httptest.NewRecorder()
	mux.ServeHTTP(check, httptest.NewRequest(http.MethodGet, "/api/v2/apps/checkupdate", nil))
	if check.Code != http.StatusOK || !strings.Contains(check.Body.String(), `"canUpdate":false`) || !strings.Contains(check.Body.String(), `"total":0`) {
		t.Fatalf("equivalent version status=%d body=%s", check.Code, check.Body.String())
	}
}

func TestAppCheckUpdateReportsCatalogError(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_APP_CATALOG", filepath.Join(t.TempDir(), "missing.json"))
	resetAppStoreForTest()
	defer resetAppStoreForTest()
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	check := httptest.NewRecorder()
	mux.ServeHTTP(check, httptest.NewRequest(http.MethodGet, "/api/v2/apps/checkupdate", nil))
	if check.Code != http.StatusBadGateway || !strings.Contains(check.Body.String(), "应用目录不可用") {
		t.Fatalf("catalog error status=%d body=%s", check.Code, check.Body.String())
	}
}

func TestCompareAppVersion(t *testing.T) {
	tests := []struct {
		latest, current string
		greater         bool
	}{
		{"1.10.0", "1.2.0", true},
		{"v1.10.0", "1.10", false},
		{"1.2.0", "1.2.0-beta", true},
		{"1.2.0-alpha.2", "1.2.0-alpha.10", false},
		{"2026.08", "2026.7", true},
	}
	for _, tt := range tests {
		if got := versionGreater(tt.latest, tt.current); got != tt.greater {
			t.Errorf("versionGreater(%q,%q)=%v, want %v", tt.latest, tt.current, got, tt.greater)
		}
	}
}
