// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

type recordingRuntimeExecutor struct {
	mu       sync.Mutex
	requests []model.CommandRequest
	inspect  string
}

type failingExtensionExecutor struct {
	recordingRuntimeExecutor
	failFragment string
}

func (r *failingExtensionExecutor) Execute(ctx context.Context, request model.CommandRequest) (model.CommandResult, error) {
	result, err := r.recordingRuntimeExecutor.Execute(ctx, request)
	if strings.Contains(strings.Join(request.Args, " "), r.failFragment) {
		return model.CommandResult{ExitCode: 1, Stderr: "injected restart failure"}, errors.New("injected restart failure")
	}
	return result, err
}

func (r *recordingRuntimeExecutor) Execute(_ context.Context, request model.CommandRequest) (model.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, request)
	if len(request.Args) > 0 && request.Args[0] == "inspect" {
		return model.CommandResult{Stdout: r.inspect}, nil
	}
	return model.CommandResult{}, nil
}

func (r *recordingRuntimeExecutor) commandLines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	lines := make([]string, 0, len(r.requests))
	for _, request := range r.requests {
		lines = append(lines, request.Program+" "+strings.Join(request.Args, " "))
	}
	return lines
}

func TestOperateRuntimeContainerProvidesComposeDefaults(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(compose, []byte("services:\n  php:\n    image: ${IMAGE_NAME:-php}:latest\n    volumes:\n      - ${PANEL_WEBSITE_DIR}:/www\n    environment:\n      TZ: ${TZ}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &recordingRuntimeExecutor{}
	item := runtimeRecord{ID: "php74", Type: "php", CodeDir: "/srv/site", ComposePath: compose, Params: map[string]any{"PHP_VERSION": "7.4"}}
	if err := operateRuntimeContainer(executor, item, "stop"); err != nil {
		t.Fatal(err)
	}
	executor.mu.Lock()
	defer executor.mu.Unlock()
	if len(executor.requests) != 2 {
		t.Fatalf("expected compose config and stop, got %d", len(executor.requests))
	}
	for _, request := range executor.requests {
		if request.Env["PANEL_WEBSITE_DIR"] != "/srv/site" || request.Env["TZ"] != "Asia/Shanghai" {
			t.Fatalf("compose defaults missing: %#v", request.Env)
		}
	}
}

func TestNodeRuntimePackageAndModules(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { resetSharedStoreForTest(); _ = store.Close() })
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
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { resetSharedStoreForTest(); _ = store.Close() })
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
	extensions := httptest.NewRecorder()
	mux.ServeHTTP(extensions, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/php/extensions/search", strings.NewReader(`{"all":true,"page":1,"pageSize":100}`)))
	if extensions.Code != http.StatusOK || !strings.Contains(extensions.Body.String(), `"name":"Default"`) || !strings.Contains(extensions.Body.String(), `"name":"WordPress"`) {
		t.Fatalf("PHP extension search body=%s", extensions.Body.String())
	}
	detailExtensions := httptest.NewRecorder()
	mux.ServeHTTP(detailExtensions, httptest.NewRequest(http.MethodGet, "/api/v2/runtimes/php/php-82/extensions", nil))
	if detailExtensions.Code != http.StatusOK || !strings.Contains(detailExtensions.Body.String(), `"supportExtensions"`) || !strings.Contains(detailExtensions.Body.String(), `"name":"bcmath"`) {
		t.Fatalf("PHP extension detail body=%s", detailExtensions.Body.String())
	}
	ssh := httptest.NewRecorder()
	mux.ServeHTTP(ssh, httptest.NewRequest(http.MethodPost, "/api/v2/settings/ssh", strings.NewReader(`{"host":"127.0.0.1","password":"secret"}`)))
	if ssh.Code != http.StatusOK || strings.Contains(ssh.Body.String(), "secret") {
		t.Fatalf("ssh response leaked or failed: %s", ssh.Body.String())
	}
	for _, path := range []string{"/api/v2/runtimes/php/php-82/config", "/api/v2/runtimes/php/php-82/container", "/api/v2/runtimes/php/php-82/fpm/config", "/api/v2/runtimes/php/php-82/fpm/status"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusNotFound {
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
	if supervisor.Code != http.StatusNotFound {
		t.Fatalf("supervisor status=%d body=%s", supervisor.Code, supervisor.Body.String())
	}
}

func TestPHPExtensionTemplatesPersistAndRuntimeParamsNormalize(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		runtimeStoreMu.Lock()
		runtimeStoreInstance = nil
		runtimeStoreMu.Unlock()
		resetSharedStoreForTest()
		_ = store.Close()
	})

	item, err := runtimeRecordFromRequest(map[string]any{
		"name": "php74", "type": "php", "source": "https://mirrors.tuna.tsinghua.edu.cn",
		"params": map[string]any{"PHP_EXTENSIONS": []any{"Redis", "curl", "redis"}},
	})
	if err != nil || item.Params["CONTAINER_PACKAGE_URL"] != "https://mirrors.tuna.tsinghua.edu.cn" || item.Params["PHP_EXTENSIONS"] != "redis,curl" {
		t.Fatalf("PHP params not normalized: err=%v params=%#v", err, item.Params)
	}

	mux := http.NewServeMux()
	RegisterRuntimeToolboxRoutes(mux)
	call := func(target, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, target, strings.NewReader(body)))
		return res
	}
	initial := call("/api/v2/runtimes/php/extensions/search", `{"all":true,"page":1,"pageSize":1}`)
	if initial.Code != http.StatusOK || !strings.Contains(initial.Body.String(), `"total":5`) || !strings.Contains(initial.Body.String(), `"name":"SeaCMS"`) {
		t.Fatalf("default PHP templates mismatch: %d %s", initial.Code, initial.Body.String())
	}
	created := call("/api/v2/runtimes/php/extensions", `{"name":"Custom","extensions":"redis,curl,redis"}`)
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), `"id":6`) || !strings.Contains(created.Body.String(), `"extensions":"redis,curl"`) {
		t.Fatalf("create PHP template failed: %d %s", created.Code, created.Body.String())
	}
	duplicate := call("/api/v2/runtimes/php/extensions", `{"name":"custom","extensions":"gd"}`)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate PHP template status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}

	runtimeStoreMu.Lock()
	runtimeStoreInstance = nil
	runtimeStoreMu.Unlock()
	mux = http.NewServeMux()
	RegisterRuntimeToolboxRoutes(mux)
	persisted := call("/api/v2/runtimes/php/extensions/search", `{"all":true,"page":1,"pageSize":100}`)
	if persisted.Code != http.StatusOK || !strings.Contains(persisted.Body.String(), `"name":"Custom"`) {
		t.Fatalf("PHP template not restored: %d %s", persisted.Code, persisted.Body.String())
	}
	updated := call("/api/v2/runtimes/php/extensions/update", `{"id":6,"extensions":"gd,imagick"}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update PHP template failed: %d %s", updated.Code, updated.Body.String())
	}
	deleted := call("/api/v2/runtimes/php/extensions/del", `{"id":6}`)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete PHP template failed: %d %s", deleted.Code, deleted.Body.String())
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
	store, err := storage.Open(filepath.Join(root, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { resetSharedStoreForTest(); _ = store.Close() })
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

func TestRuntimeInstallCommandOrder(t *testing.T) {
	tests := []struct {
		name string
		item runtimeRecord
		want []string
	}{
		{
			name: "go pull then up",
			item: runtimeRecord{Type: "go", ComposePath: filepath.Join(t.TempDir(), "docker-compose.yml")},
			want: []string{"compose -f", " pull", "compose -f", " up -d"},
		},
		{
			name: "php build install commit recreate without pull",
			item: runtimeRecord{Type: "php", Container: "php85", Image: "1panel-php-fpm:8.5.10", ComposePath: filepath.Join(t.TempDir(), "docker-compose.yml"), Params: map[string]any{"PHP_EXTENSIONS": "redis,curl"}},
			want: []string{" build", " up -d", "exec -i php85 install-ext redis,curl", "commit php85 1panel-php-fpm:8.5.10", " down", " up -d"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(test.item.ComposePath, []byte("services: {}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			executor := &recordingRuntimeExecutor{}
			if err := executeRuntimeInstall(executor, test.item, func(string, string) {}); err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(executor.commandLines(), "\n")
			position := 0
			for _, fragment := range test.want {
				next := strings.Index(joined[position:], fragment)
				if next < 0 {
					t.Fatalf("missing ordered command %q in:\n%s", fragment, joined)
				}
				position += next + len(fragment)
			}
			if test.item.Type == "php" && strings.Contains(joined, " pull") {
				t.Fatalf("PHP target image must not be pulled:\n%s", joined)
			}
		})
	}
}

func TestPHPExtensionCatalogAndDirectOperations(t *testing.T) {
	if len(phpExtensionCatalog) != 63 {
		t.Fatalf("PHP 扩展目录数量=%d, want=63", len(phpExtensionCatalog))
	}
	ionCube := phpExtensionDefinitionForName("ioncube")
	if ionCube.Check != "ionCube Loader" || ionCube.File != "ioncube_loader.so" {
		t.Fatalf("ionCube 映射错误: %#v", ionCube)
	}
	if !phpExtensionNamePattern.MatchString("ionCube") {
		t.Fatal("PHP 扩展名称应兼容 1Panel 的大小写名称")
	}

	root := t.TempDir()
	composePath := filepath.Join(root, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	item := runtimeRecord{Type: "php", Container: "php85", Image: "1panel-php-fpm:8.5.10", ComposePath: composePath}
	executor := &recordingRuntimeExecutor{}
	if err := installPHPExtension(executor, item, "redis"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(executor.commandLines(), "\n")
	for _, command := range []string{"exec -i php85 install-ext redis", "commit php85 1panel-php-fpm:8.5.10", "compose -f " + composePath + " down", "compose -f " + composePath + " up -d"} {
		if !strings.Contains(joined, command) {
			t.Fatalf("缺少 PHP 扩展安装命令 %q:\n%s", command, joined)
		}
	}
	if strings.Contains(joined, " build") {
		t.Fatalf("单扩展安装不应重建 PHP 镜像:\n%s", joined)
	}
}

func TestUninstallPHPExtensionRollsBackWhenRestartFails(t *testing.T) {
	root := t.TempDir()
	extensionDir := filepath.Join(root, "extensions", "no-debug-non-zts-20250925")
	if err := os.MkdirAll(extensionDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "conf", "conf.d"), 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(root, "docker-compose.yml"):                           "services: {}\n",
		filepath.Join(root, ".env"):                                         "PHP_EXTENSIONS=redis,ionCube\n",
		filepath.Join(root, "conf", "php.ini"):                              `zend_extension="ioncube_loader.so"` + "\n",
		filepath.Join(root, "conf", "conf.d", "docker-php-ext-ionCube.ini"): `zend_extension="ioncube_loader.so"` + "\n",
		filepath.Join(extensionDir, "ioncube_loader.so"):                    "module",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	current := runtimeRecord{
		ID: "php85", Type: "php", Container: "php85", InstallPath: root,
		ComposePath: filepath.Join(root, "docker-compose.yml"), Extensions: []string{"redis", "ionCube"}, Params: map[string]any{},
	}
	updated := current
	updated.Extensions = []string{"redis"}
	executor := &failingExtensionExecutor{failFragment: "up"}
	err := uninstallPHPExtension(executor, current, updated, phpExtensionDefinitionForName("ioncube"))
	if err == nil || !strings.Contains(err.Error(), "已恢复旧文件") {
		t.Fatalf("重启失败应返回已回滚错误: %v", err)
	}
	for _, path := range []string{
		filepath.Join(extensionDir, "ioncube_loader.so"),
		filepath.Join(root, "conf", "conf.d", "docker-php-ext-ionCube.ini"),
	} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("重启失败未恢复 %s: %v", path, statErr)
		}
	}
	env, _ := os.ReadFile(filepath.Join(root, ".env"))
	if !strings.Contains(string(env), "PHP_EXTENSIONS=redis,ionCube") {
		t.Fatalf("重启失败未恢复 .env: %s", env)
	}
}

func TestDeployRuntimeArchiveUsesVersionDirectoryAndPreservesPrevious(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "go-1.26.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	entries := map[string]string{
		"go/1.26/docker-compose.yml": "services:\n  golang: {}\n",
		"go/1.26/run.sh":             "#!/bin/bash\necho ok\n",
	}
	for name, content := range entries {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	installDir := filepath.Join(root, "runtimes", "go", "demo")
	if err := os.MkdirAll(installDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "old.txt"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := deployRuntimeArchive(archivePath, installDir, "go", "1.26"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(installDir, "run.sh")); err != nil {
		t.Fatalf("run.sh not deployed at runtime root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(installDir, "go", "1.26", "run.sh")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive wrapper directory leaked into runtime root: %v", err)
	}
	backups, err := filepath.Glob(installDir + ".previous-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("previous runtime backup missing: %v %#v", err, backups)
	}
}

func TestInstallRuntimeRunScriptPreservesBackupAndRollsBack(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "node-25.9.0.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	newScript := "#!/bin/sh\necho new\n"
	if err := tarWriter.WriteHeader(&tar.Header{Name: "node/25.9.0/run.sh", Mode: 0o750, Size: int64(len(newScript)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write([]byte(newScript)); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	installDir := filepath.Join(root, "runtimes", "node", "node25")
	if err := os.MkdirAll(installDir, 0o750); err != nil {
		t.Fatal(err)
	}
	oldScript := "#!/bin/sh\necho old\n"
	if err := os.WriteFile(filepath.Join(installDir, "run.sh"), []byte(oldScript), 0o750); err != nil {
		t.Fatal(err)
	}
	rollback, err := installRuntimeRunScriptFromArchive(archivePath, installDir, "node", "25.9.0")
	if err != nil {
		t.Fatal(err)
	}
	current, _ := os.ReadFile(filepath.Join(installDir, "run.sh"))
	backup, _ := os.ReadFile(filepath.Join(installDir, "run.sh.bak"))
	if string(current) != newScript || string(backup) != oldScript {
		t.Fatalf("unexpected scripts current=%q backup=%q", current, backup)
	}
	rollback()
	restored, err := os.ReadFile(filepath.Join(installDir, "run.sh"))
	if err != nil || string(restored) != oldScript {
		t.Fatalf("run.sh rollback failed: err=%v content=%q", err, restored)
	}
}

func TestRuntimeValidationAndStatusSync(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	php, err := runtimeRecordFromRequest(map[string]any{"id": "php74", "name": "php74", "type": "php", "version": "7", "params": map[string]any{"PHP_VERSION": "7.4.33"}})
	if err != nil {
		t.Fatalf("PHP 运行时参数解析失败: %v", err)
	}
	wantPath := filepath.Join(dataDir, "runtimes", "php", "php74")
	if php.InstallPath != wantPath || php.ComposePath != filepath.Join(wantPath, "docker-compose.yml") {
		t.Fatalf("PHP 运行时路径未按 1Panel 规则补全: path=%q compose=%q", php.InstallPath, php.ComposePath)
	}
	if err := validateRuntimeCodeDirectory(runtimeRecord{Type: "php"}); err != nil {
		t.Fatalf("PHP 运行时不应强制要求代码目录: %v", err)
	}
	if _, err := runtimeRecordFromRequest(map[string]any{"name": "bad", "type": "go", "exposedPorts": []any{map[string]any{"hostPort": 8080, "containerPort": 8080, "protocol": "sctp"}}}); err == nil {
		t.Fatal("unsupported port protocol must fail")
	}
	if _, err := validateAppArchiveURL("file:///etc/passwd"); err == nil {
		t.Fatal("file archive URL must fail")
	}
	executor := &recordingRuntimeExecutor{inspect: "running\n"}
	s := &runtimeStore{commands: executor, state: runtimeState{Runtimes: []runtimeRecord{{ID: "go-1", Container: "go-1", Status: "Creating"}}, Settings: map[string]any{}}}
	syncRuntimeContainerStatus(s)
	if got := s.state.Runtimes[0].Status; got != "Running" {
		t.Fatalf("synced status=%q, want Running", got)
	}
}

func TestRuntimePortDefaultsAcrossLanguageRuntimes(t *testing.T) {
	for _, runtimeType := range []string{"go", "java", "dotnet", "python", "node"} {
		t.Run(runtimeType, func(t *testing.T) {
			item, err := runtimeRecordFromRequest(map[string]any{
				"name": "runtime-" + runtimeType, "type": runtimeType, "version": "1", "install": true,
				"port": 8080, "params": map[string]any{"HOST_IP": "0.0.0.0"},
			})
			if err != nil {
				t.Fatalf("port default failed: %v", err)
			}
			if item.Port != 8080 || item.Params["APP_PORT"] != 8080 {
				t.Fatalf("unexpected normalized ports: host=%d params=%#v", item.Port, item.Params)
			}
		})
	}
	item, err := runtimeRecordFromRequest(map[string]any{
		"name": "runtime-exposed", "type": "node", "version": "1", "install": true,
		"exposedPorts": []any{map[string]any{"hostPort": 9000, "containerPort": 3000, "protocol": "tcp"}},
	})
	if err != nil || item.Port != 9000 || item.Params["APP_PORT"] != 3000 {
		t.Fatalf("exposed port default failed: item=%#v err=%v", item, err)
	}
	item, err = runtimeRecordFromRequest(map[string]any{
		"name": "runtime-param", "type": "java", "version": "1", "install": true,
		"port": 8080, "params": map[string]any{"APP_PORT": "9090"},
	})
	if err != nil || item.Port != 8080 || item.Params["APP_PORT"] != 9090 {
		t.Fatalf("APP_PORT precedence failed: item=%#v err=%v", item, err)
	}
	if _, err := runtimeRecordFromRequest(map[string]any{"name": "runtime-missing", "type": "python", "version": "1", "install": true}); err == nil {
		t.Fatal("missing runtime port must fail")
	}
}

func TestPHPConfigurationRoutesReadUpdateAndRollbackBoundary(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		runtimeStoreMu.Lock()
		runtimeStoreInstance = nil
		runtimeStoreMu.Unlock()
		resetSharedStoreForTest()
		_ = store.Close()
	})
	installDir := filepath.Join(dataDir, "runtimes", "php", "php85")
	if err := os.MkdirAll(filepath.Join(installDir, "conf"), 0o750); err != nil {
		t.Fatal(err)
	}
	phpINI := "[PHP]\nupload_max_filesize = 2M\nmax_execution_time = 30\ndisable_functions = exec\n"
	if err := os.WriteFile(filepath.Join(installDir, "conf", "php.ini"), []byte(phpINI), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "conf", "php-fpm.conf"), []byte("[www]\npm = dynamic\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(installDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &recordingRuntimeExecutor{}
	s := getRuntimeStore()
	s.commands = executor
	s.mu.Lock()
	s.state.Runtimes = append(s.state.Runtimes, runtimeRecord{ID: "php85", Name: "php85", Type: "php", Container: "php85", InstallPath: installDir, ComposePath: composePath, Status: "Running", Params: map[string]any{"PHP_VERSION": "8.5.10"}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	mux := http.NewServeMux()
	registerRuntimeRoutes(mux, s)
	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v2/runtimes/php/config/php85", nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"uploadMaxSize":"2M"`) {
		t.Fatalf("PHP config read failed: %d %s", get.Code, get.Body.String())
	}
	update := httptest.NewRecorder()
	mux.ServeHTTP(update, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/php/config", strings.NewReader(`{"id":"php85","uploadMaxSize":"64M","maxExecutionTime":"120","disableFunctions":["exec","system"]}`)))
	if update.Code != http.StatusOK {
		t.Fatalf("PHP config update failed: %d %s", update.Code, update.Body.String())
	}
	updated, err := os.ReadFile(filepath.Join(installDir, "conf", "php.ini"))
	if err != nil || !strings.Contains(string(updated), "upload_max_filesize = 64M") || !strings.Contains(string(updated), "disable_functions = exec,system") {
		t.Fatalf("PHP config not persisted: err=%v content=%s", err, updated)
	}
	logPath := filepath.Join(installDir, "build.log")
	if err := os.WriteFile(logPath, []byte("line-1\nline-2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(installDir, "log"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "log", "fpm.slow.log"), []byte("slow-request\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	logResponse := httptest.NewRecorder()
	fileAdvancedHandler(logResponse, httptest.NewRequest(http.MethodPost, "/api/v2/files/read/php", strings.NewReader(`{"id":"php85","page":1,"pageSize":1,"path":"/etc/passwd"}`)))
	if logResponse.Code != http.StatusOK || !strings.Contains(logResponse.Body.String(), `"line-1"`) || strings.Contains(logResponse.Body.String(), "passwd") {
		t.Fatalf("PHP build log read contract failed: %d %s", logResponse.Code, logResponse.Body.String())
	}
	slowLogResponse := httptest.NewRecorder()
	fileAdvancedHandler(slowLogResponse, httptest.NewRequest(http.MethodPost, "/api/v2/files/read/php-fpm-slow-logs", strings.NewReader(`{"id":"php85","page":1,"pageSize":10,"path":"/etc/passwd"}`)))
	if slowLogResponse.Code != http.StatusOK || !strings.Contains(slowLogResponse.Body.String(), `"slow-request"`) || strings.Contains(slowLogResponse.Body.String(), "passwd") {
		t.Fatalf("PHP slow log read contract failed: %d %s", slowLogResponse.Code, slowLogResponse.Body.String())
	}
}
