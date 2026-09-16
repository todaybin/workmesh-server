// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// remoteCatalogZip 构造真实 ZIP 目录响应，供远程应用目录解析测试使用。
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

// TestRemoteAppCatalogLazyLoadAndDetails 验证远程目录按需加载和详情字段转换。
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
	if !strings.Contains(detail.Body.String(), `"image":"demo"`) {
		t.Fatalf("runtime image repository missing: %s", detail.Body.String())
	}

	icon := httptest.NewRecorder()
	mux.ServeHTTP(icon, httptest.NewRequest(http.MethodGet, "/api/v2/apps/icon/demo", nil))
	if icon.Code != http.StatusOK || icon.Header().Get("Content-Type") != "image/png" || icon.Body.String() != "png" {
		t.Fatalf("icon status=%d type=%s body=%q", icon.Code, icon.Header().Get("Content-Type"), icon.Body.String())
	}
}

// TestPHPVersionAppUsesOnePanelRuntimeFormFields 验证 PHP 应用表单字段与参考契约一致。
func TestPHPVersionAppUsesOnePanelRuntimeFormFields(t *testing.T) {
	genericParams := map[string]any{"formFields": []any{
		map[string]any{"envKey": "PHP_EXTENSIONS", "multiple": true},
		map[string]any{"envKey": "PHP_VERSION", "default": "7.4.33"},
		map[string]any{"envKey": "CONTAINER_PACKAGE_URL", "default": "https://mirrors.tuna.tsinghua.edu.cn"},
		map[string]any{"envKey": "PANEL_APP_PORT_HTTP", "default": 9000},
	}}
	catalog := []appRecord{{Key: "php", Type: "php", Versions: []appVersionRecord{{Version: "7", Params: genericParams}}}}
	selected := appVersionRecord{Version: "7.4.33", Params: map[string]any{"formFields": []any{map[string]any{"envKey": "PANEL_APP_PORT_HTTP"}}}}
	params := phpRuntimeCatalogParams(catalog, selected)
	fields, ok := params["formFields"].([]any)
	if !ok || len(fields) != 4 {
		t.Fatalf("PHP runtime fields not supplied from 1Panel catalog: %#v", params)
	}
	for _, key := range []string{"PHP_EXTENSIONS", "PHP_VERSION", "CONTAINER_PACKAGE_URL", "PANEL_APP_PORT_HTTP"} {
		found := false
		for _, raw := range fields {
			field, _ := raw.(map[string]any)
			found = found || field["envKey"] == key
		}
		if !found {
			t.Fatalf("PHP runtime field %s missing: %#v", key, fields)
		}
	}
}

// TestRuntimeCatalogTypeIsolationAndUnifiedPHPEntry 验证运行时目录筛选隔离及统一 PHP 入口。
func TestRuntimeCatalogTypeIsolationAndUnifiedPHPEntry(t *testing.T) {
	tests := []struct {
		name     string
		app      appRecord
		typeName string
		want     bool
	}{
		{name: "unified php", app: appRecord{Key: "php", Type: "php"}, typeName: "php", want: true},
		{name: "deprecated php8", app: appRecord{Key: "php8", Type: "php"}, typeName: "php", want: false},
		{name: "phpmyadmin is not php", app: appRecord{Key: "phpmyadmin", Type: "app"}, typeName: "php", want: false},
		{name: "node", app: appRecord{Key: "node", Type: "node"}, typeName: "node", want: true},
		{name: "cross type", app: appRecord{Key: "go", Type: "go"}, typeName: "java", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := appMatchesRuntimeCatalog(test.app, test.typeName); got != test.want {
				t.Fatalf("appMatchesRuntimeCatalog(%#v, %q)=%v, want %v", test.app, test.typeName, got, test.want)
			}
		})
	}
}

// TestLegacyCompatibilityDoesNotOverrideAppRoutes 验证兼容路由不会覆盖应用专用处理器。
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
