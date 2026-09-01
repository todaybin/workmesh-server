// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func remoteCatalogZip(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create("1panel.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte(`{
  "additionalProperties":{"version":"2026.08","tags":[{"key":"database","name":"Database","locales":{"zh":"数据库","en":"Database","zh-hant":"資料庫"}}]},
  "lastModified":1700000001,
  "apps":[{
    "name":"Demo App","readMe":"# Demo","icon":"logo.png",
    "additionalProperties":{"key":"demo","name":"Demo App","type":"app","tags":["database"],"shortDescZh":"演示应用","description":{"zh":"演示应用描述"},"limit":1,"recommend":1,"batchInstallSupport":true},
    "versions":[{"name":"1.2.3","downloadUrl":"https://example.invalid/demo.tar.gz","lastModified":1700000002,"additionalProperties":{"formFields":[{"type":"text","envKey":"DEMO_VALUE","default":"ok"}]}}]
  }]
}`))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestRemoteAppCatalogLazyLoadAndDetails(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	var zipData []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stable/1panel.json.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipData)
		case "/stable/1panel/demo/1.2.3/docker-compose.yml":
			_, _ = w.Write([]byte("services:\n  demo:\n    image: demo:1.2.3\n"))
		case "/stable/1panel/demo/logo.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	zipData = remoteCatalogZip(t)
	t.Setenv("WORKMESH_APP_REPO_URL", server.URL)
	t.Setenv("WORKMESH_APP_REPO_MODE", "stable")
	resetAppStoreForTest()
	defer resetAppStoreForTest()
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)

	search := httptest.NewRecorder()
	mux.ServeHTTP(search, httptest.NewRequest(http.MethodPost, "/api/v2/apps/search", strings.NewReader(`{"page":1,"pageSize":30,"tags":["database"]}`)))
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), "demo") || !strings.Contains(search.Body.String(), "演示应用描述") {
		t.Fatalf("remote search status=%d body=%s", search.Code, search.Body.String())
	}
	tags := httptest.NewRecorder()
	tagRequest := httptest.NewRequest(http.MethodGet, "/api/v2/apps/tags", nil)
	tagRequest.Header.Set("Accept-Language", "zh")
	mux.ServeHTTP(tags, tagRequest)
	if tags.Code != http.StatusOK || !strings.Contains(tags.Body.String(), `"key":"database"`) || !strings.Contains(tags.Body.String(), `"name":"数据库"`) {
		t.Fatalf("app tags contract mismatch: status=%d body=%s", tags.Code, tags.Body.String())
	}
	traditionalTags := httptest.NewRecorder()
	traditionalRequest := httptest.NewRequest(http.MethodGet, "/api/v2/apps/tags", nil)
	traditionalRequest.Header.Set("Accept-Language", "zh-Hant")
	mux.ServeHTTP(traditionalTags, traditionalRequest)
	if traditionalTags.Code != http.StatusOK || !strings.Contains(traditionalTags.Body.String(), `"name":"資料庫"`) {
		t.Fatalf("localized app tags contract mismatch: status=%d body=%s", traditionalTags.Code, traditionalTags.Body.String())
	}

	app := httptest.NewRecorder()
	mux.ServeHTTP(app, httptest.NewRequest(http.MethodGet, "/api/v2/apps/demo", nil))
	if app.Code != http.StatusOK || !strings.Contains(app.Body.String(), `"versions":["1.2.3"]`) {
		t.Fatalf("app detail status=%d body=%s", app.Code, app.Body.String())
	}

	detail := httptest.NewRecorder()
	mux.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "/api/v2/apps/detail/"+appStableID("app:demo")+"/1.2.3/app", nil))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"id":"`) || !strings.Contains(detail.Body.String(), "demo:1.2.3") || !strings.Contains(detail.Body.String(), `"dockerCompose":"services:`) || !strings.Contains(detail.Body.String(), `"params":{"formFields"`) {
		t.Fatalf("version detail status=%d body=%s", detail.Code, detail.Body.String())
	}

	icon := httptest.NewRecorder()
	mux.ServeHTTP(icon, httptest.NewRequest(http.MethodGet, "/api/v2/apps/icon/demo", nil))
	if icon.Code != http.StatusOK || icon.Header().Get("Content-Type") != "image/png" || icon.Body.String() != "png" {
		t.Fatalf("icon status=%d type=%s body=%q", icon.Code, icon.Header().Get("Content-Type"), icon.Body.String())
	}
}

func TestLegacyCompatibilityDoesNotOverrideAppRoutes(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_APP_CATALOG", "")
	t.Setenv("WORKMESH_APP_REPO_URL", "")
	resetAppStoreForTest()
	defer resetAppStoreForTest()
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	RegisterLegacyCompatibilityRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v2/apps/missing", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"available":false`) {
		t.Fatalf("legacy compatibility replaced app route: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

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

func TestOpenRestyInstalledCheckUsesRecordedContainer(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	resetAppStoreForTest()
	defer resetAppStoreForTest()
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	install := httptest.NewRecorder()
	mux.ServeHTTP(install, httptest.NewRequest(http.MethodPost, "/api/v2/apps/install", strings.NewReader(`{"id":"resty","key":"openresty","name":"resty-prod","version":"1.27.1","containerName":"workmesh-openresty"}`)))
	if install.Code != http.StatusOK {
		t.Fatalf("openresty install status=%d body=%s", install.Code, install.Body.String())
	}
	store := getAppStore()
	store.containerStates = func(_ context.Context, names []string) (map[string]string, error) {
		if len(names) != 1 || names[0] != "workmesh-openresty" {
			t.Fatalf("unexpected recorded container names: %#v", names)
		}
		return map[string]string{"workmesh-openresty": "running"}, nil
	}
	check := httptest.NewRecorder()
	mux.ServeHTTP(check, httptest.NewRequest(http.MethodPost, "/api/v2/apps/installed/check", strings.NewReader(`{"key":"openresty","name":"resty"}`)))
	if check.Code != http.StatusOK || !strings.Contains(check.Body.String(), `"isExist":true`) || !strings.Contains(check.Body.String(), `"status":"Running"`) {
		t.Fatalf("openresty container check mismatch: %d %s", check.Code, check.Body.String())
	}
}

func TestApplyAppContainerStatesMatchesOriginalStatusRules(t *testing.T) {
	tests := []struct {
		name   string
		states map[string]string
		want   string
	}{
		{name: "全部停止", states: map[string]string{"web": "exited", "waf": "exited"}, want: "Stopped"},
		{name: "全部重启", states: map[string]string{"web": "restarting", "waf": "restarting"}, want: "ReStarting"},
		{name: "全部暂停", states: map[string]string{"web": "paused", "waf": "paused"}, want: "Paused"},
		{name: "全部缺失", states: map[string]string{}, want: "Error"},
		{name: "状态混合", states: map[string]string{"web": "running", "waf": "exited"}, want: "UnHealthy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := appRecord{Status: "Running"}
			applyAppContainerStates(&item, []string{"web", "waf"}, tt.states, false)
			if item.Status != tt.want {
				t.Fatalf("status=%s want=%s message=%s", item.Status, tt.want, item.Message)
			}
		})
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

func TestAppOperationValidatesTargetAndOperation(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	missing := httptest.NewRecorder()
	mux.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/api/v2/apps/installed/op", strings.NewReader(`{"installId":"missing","operate":"stop"}`)))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing app status=%d body=%s", missing.Code, missing.Body.String())
	}
	invalid := httptest.NewRecorder()
	mux.ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/api/v2/apps/installed/op", strings.NewReader(`{"installId":"missing","operate":"explode"}`)))
	if invalid.Code != http.StatusNotFound {
		t.Fatalf("target validation should precede operation validation: %d", invalid.Code)
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
