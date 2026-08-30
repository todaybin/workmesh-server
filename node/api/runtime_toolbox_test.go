// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNodeRuntimePackageAndModules(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"demo","scripts":{"build":"go build","test":"go test"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	moduleDir := filepath.Join(dir, "node_modules", "demo-module")
	if err := os.MkdirAll(moduleDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "package.json"), []byte(`{"name":"demo-module","version":"1.2.3","license":"MIT","description":"demo"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterRuntimeToolboxRoutes(mux)
	createBody := map[string]any{"id": "node-1", "name": "Node", "type": "node", "version": "20", "codeDir": dir}
	createJSON, _ := json.Marshal(createBody)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes", bytes.NewReader(createJSON)))
	if create.Code != http.StatusOK {
		t.Fatalf("create runtime status=%d body=%s", create.Code, create.Body.String())
	}
	packageReq := httptest.NewRecorder()
	mux.ServeHTTP(packageReq, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/node/package", strings.NewReader(`{"codeDir":"`+strings.ReplaceAll(dir, `\`, `\\`)+`"}`)))
	if packageReq.Code != http.StatusOK || !strings.Contains(packageReq.Body.String(), `"build"`) {
		t.Fatalf("package scripts status=%d body=%s", packageReq.Code, packageReq.Body.String())
	}
	modulesReq := httptest.NewRecorder()
	mux.ServeHTTP(modulesReq, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/node/modules", strings.NewReader(`{"id":"node-1"}`)))
	if modulesReq.Code != http.StatusOK || !strings.Contains(modulesReq.Body.String(), "demo-module") {
		t.Fatalf("node modules status=%d body=%s", modulesReq.Code, modulesReq.Body.String())
	}
	badOperation := httptest.NewRecorder()
	mux.ServeHTTP(badOperation, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/node/modules/operate", strings.NewReader(`{"id":"node-1","operate":"install","pkgManager":"powershell","module":"x"}`)))
	if badOperation.Code != http.StatusBadRequest {
		t.Fatalf("invalid package manager status=%d body=%s", badOperation.Code, badOperation.Body.String())
	}
}

func TestRuntimeAndSSHRoutes(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	RegisterRuntimeToolboxRoutes(mux)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes", strings.NewReader(`{"id":"php-82","name":"PHP 8.2","type":"php","version":"8.2"}`)))
	if create.Code != http.StatusOK {
		t.Fatalf("create runtime status=%d body=%s", create.Code, create.Body.String())
	}
	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/search", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "php-82") {
		t.Fatalf("runtime list body=%s", list.Body.String())
	}
	ssh := httptest.NewRecorder()
	mux.ServeHTTP(ssh, httptest.NewRequest(http.MethodPost, "/api/v2/settings/ssh", strings.NewReader(`{"host":"127.0.0.1","password":"secret"}`)))
	if ssh.Code != http.StatusOK || strings.Contains(ssh.Body.String(), "secret") {
		t.Fatalf("ssh response leaked or failed: %s", ssh.Body.String())
	}
	for _, path := range []string{"/api/v2/runtimes/php/php-82/extensions", "/api/v2/runtimes/php/php-82/config", "/api/v2/runtimes/php/php-82/container", "/api/v2/runtimes/php/php-82/fpm/config", "/api/v2/runtimes/php/php-82/fpm/status"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "php-82") {
			t.Fatalf("runtime detail %s status=%d body=%s", path, res.Code, res.Body.String())
		}
	}
	missing := httptest.NewRecorder()
	mux.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/v2/runtimes/php/missing/extensions", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing runtime status=%d body=%s", missing.Code, missing.Body.String())
	}
	supervisor := httptest.NewRecorder()
	mux.ServeHTTP(supervisor, httptest.NewRequest(http.MethodGet, "/api/v2/runtimes/supervisor/process/web", nil))
	if supervisor.Code != http.StatusOK || !strings.Contains(supervisor.Body.String(), "not_configured") {
		t.Fatalf("supervisor status=%d body=%s", supervisor.Code, supervisor.Body.String())
	}
}

func TestToolboxGetDataUsesHostState(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	s := getRuntimeStore()
	users := toolboxGetData(s, "/api/v2/toolbox/device/users")
	if users["items"] == nil || users["total"] == nil {
		t.Fatalf("users response missing fields: %#v", users)
	}
	zones := toolboxGetData(s, "/api/v2/toolbox/device/zone/options")
	if zones["current"] == "" {
		t.Fatalf("timezone response missing current zone: %#v", zones)
	}
	ftp := toolboxGetData(s, "/api/v2/toolbox/ftp/base")
	if ftp["status"] == nil && ftp["enabled"] == nil {
		t.Fatalf("ftp response missing state: %#v", ftp)
	}
}

func TestToolboxDeviceDNSAndFTPState(t *testing.T) {
	root := filepath.Join(".tmp", "toolbox-device-test")
	_ = os.RemoveAll(root)
	defer os.RemoveAll(root)
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_FAIL2BAN_CONFIG", filepath.Join(root, "fail2ban.local"))
	mux := http.NewServeMux()
	RegisterRuntimeToolboxRoutes(mux)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/device/base", `{}`); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"hostname"`) {
		t.Fatalf("设备基础信息失败: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/device/check/dns", `{"host":"localhost"}`); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"resolved":true`) {
		t.Fatalf("DNS 探测失败: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/device/check/dns", `{"host":"../etc/passwd"}`); res.Code != http.StatusBadRequest {
		t.Fatalf("非法 DNS 主机名应拒绝: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/ftp", `{"name":"测试 FTP","host":"ftp.example","username":"deploy","password":"secret"}`); res.Code != http.StatusOK || strings.Contains(res.Body.String(), "secret") {
		t.Fatalf("FTP 创建失败或泄漏密码: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/ftp/search", `{"keyword":"ftp.example"}`); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "ftp.example") {
		t.Fatalf("FTP 搜索失败: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/ftp/log/search", `{"page":1,"pageSize":20}`); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "create_or_update") {
		t.Fatalf("FTP 日志查询失败: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/fail2ban/update", `{"content":"[sshd]\nenabled=true"}`); res.Code != http.StatusOK {
		t.Fatalf("Fail2ban 配置保存失败: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/fail2ban/search", `{"keyword":"sshd"}`); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "sshd") {
		t.Fatalf("Fail2ban 配置检索失败: %d %s", res.Code, res.Body.String())
	}
}
