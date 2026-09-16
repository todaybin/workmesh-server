// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// resetAppStoreForTest 清理进程内应用仓库缓存，模拟重启后的重新加载。
func resetAppStoreForTest() {
	appStoreMu.Lock()
	appStoreInstance = nil
	appStoreMu.Unlock()
}

// TestAppInstallAndList 验证应用安装记录写入真实状态并可分页查询。
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

func TestInstalledListProvidesRuntimeCompatibilityFields(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	resetAppStoreForTest()
	defer resetAppStoreForTest()
	store := getAppStore()
	item := appRecord{
		ID: "openresty-install", Key: "openresty", Name: "OpenResty",
		Version: "1.27.1", Status: "Running", ContainerName: "workmesh-openresty-waf",
		Config: map[string]any{
			"composeProject":       "workmesh-openresty",
			"PANEL_APP_PORT_HTTP":  9999,
			"PANEL_APP_PORT_HTTPS": 443,
			"WORKMESH_MODE":        "waf",
		},
	}
	store.mu.Lock()
	store.state.Apps = []appRecord{item}
	store.mu.Unlock()

	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v2/apps/installed/search", nil))
	body := response.Body.String()
	for _, expected := range []string{
		`"path":"` + strings.ReplaceAll(appInstallPath(item), `\`, `\\`) + `"`,
		`"container":"workmesh-openresty-waf"`,
		`"serviceName":"workmesh-openresty"`,
		`"httpPort":9999`,
		`"httpsPort":443`,
		`"WORKMESH_MODE":"waf"`,
	} {
		if response.Code != http.StatusOK || !strings.Contains(body, expected) {
			t.Fatalf("installed response missing %s: %d %s", expected, response.Code, body)
		}
	}
}

// TestAppWriteRejectsMalformedAndTrailingJSON 验证应用写接口不会把无效 JSON 当成空请求执行。
func TestAppWriteRejectsMalformedAndTrailingJSON(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	resetAppStoreForTest()
	defer resetAppStoreForTest()
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	for name, body := range map[string]string{
		"malformed": `{"id":"broken"`,
		"trailing":  `{"id":"broken"}{"id":"second"}`,
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v2/apps/install", strings.NewReader(body)))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("无效 JSON 应返回 400，实际=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	// 空请求体仍保持旧前端兼容行为，但因缺少应用标识返回业务 400，而不是解析错误 500。
	empty := httptest.NewRecorder()
	mux.ServeHTTP(empty, httptest.NewRequest(http.MethodPost, "/api/v2/apps/install", nil))
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("空体缺少应用标识应返回 400，实际=%d body=%s", empty.Code, empty.Body.String())
	}
}

// TestAppStoreUsesSQLiteWithoutWritingAppsJSON 验证共享 SQLite 启用后生产应用状态不再写 apps.json。
func TestAppStoreUsesSQLiteWithoutWritingAppsJSON(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	resetAppStoreForTest()
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		resetAppStoreForTest()
		resetSharedStoreForTest()
		_ = store.Close()
	})

	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v2/apps/install", strings.NewReader(`{"id":"sqlite-app","name":"SQLite App"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("SQLite 应用写入失败: %d %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dataDir, "apps.json")); !os.IsNotExist(err) {
		t.Fatalf("共享 SQLite 模式不应创建 apps.json，stat err=%v", err)
	}
	var count int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM app_installs WHERE id=?`, "sqlite-app").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("应用关系表记录数=%d，want=1", count)
	}
}

// TestInstalledSyncAndCustomStoreRequireRealSources 验证同步和自定义目录拒绝虚假来源。
func TestInstalledSyncAndCustomStoreRequireRealSources(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_CUSTOM_APP_ARCHIVE", "")
	t.Setenv("WORKMESH_CUSTOM_APP_PACKAGE", "")
	resetAppStoreForTest()
	defer resetAppStoreForTest()
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	sync := httptest.NewRecorder()
	mux.ServeHTTP(sync, httptest.NewRequest(http.MethodPost, "/api/v2/apps/installed/sync", strings.NewReader(`{}`)))
	if sync.Code != http.StatusOK || !strings.Contains(sync.Body.String(), `"synced"`) {
		t.Fatalf("installed sync failed: %d %s", sync.Code, sync.Body.String())
	}
	custom := httptest.NewRecorder()
	mux.ServeHTTP(custom, httptest.NewRequest(http.MethodPost, "/api/v2/custom/app/sync", strings.NewReader(`{"taskID":"t1"}`)))
	if custom.Code != http.StatusServiceUnavailable || !strings.Contains(custom.Body.String(), "归档") {
		t.Fatalf("custom sync should reject missing source: %d %s", custom.Code, custom.Body.String())
	}
}

// TestAppInstalledCheckUsesEnvironmentProbe 验证已安装状态来自容器环境探测。
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

// TestOpenRestyInstalledCheckUsesRecordedContainer 验证 OpenResty 状态使用登记的真实容器。
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

// TestApplyAppContainerStatesMatchesOriginalStatusRules 验证容器状态映射遵循参考状态规则。
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

// TestAppOperationsAndCatalog 验证应用生命周期操作与目录查询的持久化行为。
func TestAppOperationsAndCatalog(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\ncase \"$*\" in *ps*) printf 'demo-web\\texited\\n';; esac\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	install := httptest.NewRecorder()
	mux.ServeHTTP(install, httptest.NewRequest(http.MethodPost, "/api/v2/apps/install", strings.NewReader(`{"id":"42","key":"demo","name":"Demo","version":"1.0","containerName":"demo-web","port":8080}`)))
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
	if stop.Code != http.StatusOK || !strings.Contains(strings.ToLower(stop.Body.String()), "stopped") {
		t.Fatalf("stop: %d %s", stop.Code, stop.Body.String())
	}
	icon := httptest.NewRecorder()
	mux.ServeHTTP(icon, httptest.NewRequest(http.MethodGet, "/api/v2/apps/icon/demo", nil))
	if icon.Code != http.StatusOK || icon.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("icon: %d %s", icon.Code, icon.Header().Get("Content-Type"))
	}
}

// TestAppOperationWithoutInstallIDRoutesOpenResty 验证旧版 OpenResty 卡片缺失 installId 时仍操作真实容器。
func TestAppOperationWithoutInstallIDRoutesOpenResty(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_OPENRESTY_CONTAINER", "workmesh-openresty-waf")
	t.Setenv("WORKMESH_OPENRESTY_BIN", "")
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/bin/sh
case "$1 $2" in
  "ps -a") printf 'workmesh-openresty-waf\tworkmesh/openresty-waf:20260913\tExited (0) 1 second ago\n' ;;
  "exec") exit 0 ;;
  "stop") exit 0 ;;
  *) exit 0 ;;
esac
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	resetAppStoreForTest()
	defer resetAppStoreForTest()
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v2/apps/installed/op", strings.NewReader(`{"installId":"","operate":"stop"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("OpenResty operation status=%d body=%s", response.Code, response.Body.String())
	}
	for _, expected := range []string{`"app":"openresty"`, `"operate":"stop"`, `"accepted":true`, `"binary":"docker://workmesh-openresty-waf"`} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("OpenResty operation missing %s: %s", expected, response.Body.String())
		}
	}
}

// TestAppOperationValidatesTargetAndOperation 验证应用操作目标和动作白名单。
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

// TestAppOperationUsesComposeUpAndRealDockerBinary 验证应用启动使用受控 Docker Compose 二进制。
func TestAppOperationUsesComposeUpAndRealDockerBinary(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker-args.log")
	dockerPath := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + logPath + "\"\ncase \"$*\" in *' ps '*|*' ps --format '* ) printf 'demo-web\\n' ;; esac\n"
	if err := os.WriteFile(dockerPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	resetAppStoreForTest()
	defer resetAppStoreForTest()
	store := getAppStore()
	item := appRecord{ID: "42", Key: "demo", Name: "Demo", Version: "1.0", Status: "Stopped", ContainerName: "demo-web", Config: map[string]any{}}
	if err := os.MkdirAll(appInstallPath(item), 0o750); err != nil {
		t.Fatal(err)
	}
	composePath := appComposePath(item)
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: demo:1.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.state.Apps = []appRecord{item}
	store.mu.Unlock()
	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v2/apps/installed/op", strings.NewReader(`{"installId":"42","operate":"start"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("operation status=%d body=%s", rec.Code, rec.Body.String())
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "compose") || !strings.Contains(string(args), "up -d") {
		t.Fatalf("expected compose up -d invocation, got %q", args)
	}
}

// TestAppDerivedDetailsAndDeleteCheck 验证应用详情派生字段和删除前置检查。
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

// TestAppCheckUpdateUsesConfiguredCatalogAndPersistsMetadata 验证更新检查读取目录并保存元数据。
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

// TestAppCheckUpdateNoUpdateForEquivalentVersions 验证相同版本不会误报可更新。
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

// TestAppCheckUpdateReportsCatalogError 验证目录不可用时返回明确错误而非假成功。
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

// TestCompareAppVersion 验证应用版本比较覆盖数字、前缀和缺省版本。
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
