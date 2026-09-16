// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// TestRuntimeInstallCommandOrder 验证运行时相关功能、失败边界和持久化结果。
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

// TestPHPExtensionCatalogAndDirectOperations 验证运行时相关功能、失败边界和持久化结果。
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

// TestUninstallPHPExtensionRollsBackWhenRestartFails 验证运行时相关功能、失败边界和持久化结果。
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

// TestDeployRuntimeArchiveUsesVersionDirectoryAndPreservesPrevious 验证运行时相关功能、失败边界和持久化结果。
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

// TestInstallRuntimeRunScriptPreservesBackupAndRollsBack 验证运行时相关功能、失败边界和持久化结果。
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

// TestRuntimeValidationAndStatusSync 验证运行时相关功能、失败边界和持久化结果。
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
	failed := &recordingRuntimeExecutor{inspect: "created\n"}
	failedStore := &runtimeStore{commands: failed, state: runtimeState{Runtimes: []runtimeRecord{{ID: "go-failed", Container: "go-failed", Status: "Error", TaskID: "task-failed", TaskStatus: "failed", Error: "address already in use", Message: "address already in use"}}, Settings: map[string]any{}}}
	syncRuntimeContainerStatus(failedStore)
	if got := failedStore.state.Runtimes[0].Status; got != "Error" {
		t.Fatalf("failed task status=%q, want Error", got)
	}
	if got := failedStore.state.Runtimes[0].Error; got != "address already in use" {
		t.Fatalf("failed task error=%q, want preserved error", got)
	}
}

// TestRuntimePortDefaultsAcrossLanguageRuntimes 验证运行时相关功能、失败边界和持久化结果。
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

// TestPHPConfigurationRoutesReadUpdateAndRollbackBoundary 验证运行时相关功能、失败边界和持久化结果。
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
