// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// TestExternalRuntimeLifecycles uses only locally available images and disposable
// names. It verifies that runtime registration and lifecycle operations act on
// real Docker containers and remove them on deletion.
func TestExternalRuntimeLifecycles(t *testing.T) {
	if os.Getenv("WORKMESH_RUNTIME_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_RUNTIME_EXTERNAL_TEST=1 to run runtime Docker acceptance")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI unavailable")
	}
	images := []struct{ typ, image string }{
		{"go", "golang:1.24"},
		{"node", "node:22-alpine"},
		{"python", "python:3.12-alpine"},
		{"java", "eclipse-temurin:21-jdk-alpine"},
		{"dotnet", "mcr.microsoft.com/dotnet/runtime:8.0-alpine"},
		{"php", "1panel-php-fpm:8.5.10"},
	}
	for _, target := range images {
		t.Run(target.typ, func(t *testing.T) {
			name := fmt.Sprintf("workmesh-acceptance-runtime-%s-%d", target.typ, time.Now().UnixNano())
			t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
			if output, err := exec.Command("docker", "run", "-d", "--name", name, "--entrypoint", "sh", target.image, "-c", "sleep 120").CombinedOutput(); err != nil {
				t.Skipf("本地镜像 %s 无法启动: %v (%s)", target.image, err, strings.TrimSpace(string(output)))
			}
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
			runtimeStoreMu.Lock()
			runtimeStoreInstance = nil
			runtimeStoreMu.Unlock()
			mux := http.NewServeMux()
			registerRuntimeRoutes(mux, getRuntimeStore())
			call := func(method, path, body string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				res := httptest.NewRecorder()
				mux.ServeHTTP(res, req)
				return res
			}
			created := call(http.MethodPost, "/api/v2/runtimes", fmt.Sprintf(`{"id":%q,"name":%q,"type":%q,"version":"external","container":%q}`, name, name, target.typ, name))
			if created.Code != http.StatusOK {
				t.Fatalf("注册运行时失败: %d %s", created.Code, created.Body.String())
			}
			for _, operation := range []string{"down", "up", "restart"} {
				res := call(http.MethodPost, "/api/v2/runtimes/operate", fmt.Sprintf(`{"id":%q,"operate":%q}`, name, operation))
				if res.Code != http.StatusOK {
					t.Fatalf("%s 失败: %d %s", operation, res.Code, res.Body.String())
				}
			}
			inspect, err := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", name).Output()
			if err != nil || strings.TrimSpace(string(inspect)) != "running" {
				t.Fatalf("重启后容器不是 running: %v %q", err, string(inspect))
			}
			deleted := call(http.MethodPost, "/api/v2/runtimes/del", fmt.Sprintf(`{"id":%q}`, name))
			if deleted.Code != http.StatusOK {
				t.Fatalf("删除运行时失败: %d %s", deleted.Code, deleted.Body.String())
			}
			if err := exec.CommandContext(context.Background(), "docker", "inspect", name).Run(); err == nil {
				t.Fatal("运行时删除后容器仍存在")
			}
		})
	}
}

// TestExternalPHPExtensionInstall verifies the complete extension path against
// a disposable PHP-FPM container: install-ext, image commit, restart and the
// runtime extension query. It is intentionally opt-in because it downloads a
// PECL package and Debian build dependencies.
func TestExternalPHPExtensionInstall(t *testing.T) {
	if os.Getenv("WORKMESH_RUNTIME_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_RUNTIME_EXTERNAL_TEST=1 to run PHP extension acceptance")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI unavailable")
	}
	name := fmt.Sprintf("workmesh-acceptance-php-ext-%d", time.Now().UnixNano())
	if output, err := exec.Command("docker", "run", "-d", "--name", name, "--entrypoint", "sh", "1panel-php-fpm:8.5.10", "-c", "sleep 300").CombinedOutput(); err != nil {
		t.Skipf("PHP image unavailable: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
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
	runtimeStoreMu.Lock()
	runtimeStoreInstance = nil
	runtimeStoreMu.Unlock()
	s := getRuntimeStore()
	s.mu.Lock()
	s.state.Runtimes = append(s.state.Runtimes, runtimeRecord{ID: name, Name: name, Type: "php", Version: "8.5.10", Image: "1panel-php-fpm:8.5.10", Container: name, Status: "Running", Extensions: []string{}, Params: map[string]any{}})
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	mux := http.NewServeMux()
	registerRuntimeRoutes(mux, s)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/php/extensions/install", strings.NewReader(fmt.Sprintf(`{"id":%q,"name":"redis"}`, name))))
	if res.Code != http.StatusAccepted {
		t.Fatalf("extension request status=%d body=%s", res.Code, res.Body.String())
	}
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		s.mu.RLock()
		var current runtimeRecord
		for _, item := range s.state.Runtimes {
			if item.ID == name {
				current = item
				break
			}
		}
		s.mu.RUnlock()
		if strings.EqualFold(current.TaskStatus, "success") {
			break
		}
		if strings.EqualFold(current.TaskStatus, "failed") || strings.EqualFold(current.Status, "error") {
			t.Fatalf("PHP extension install failed: %s", current.Error)
		}
		time.Sleep(2 * time.Second)
	}
	s.mu.RLock()
	var current runtimeRecord
	for _, item := range s.state.Runtimes {
		if item.ID == name {
			current = item
			break
		}
	}
	s.mu.RUnlock()
	if !strings.EqualFold(current.TaskStatus, "success") {
		t.Fatalf("PHP extension install timed out: status=%q task=%q error=%q", current.Status, current.TaskStatus, current.Error)
	}
	check := exec.Command("docker", "exec", name, "php", "-m")
	output, err := check.CombinedOutput()
	if err != nil || !strings.Contains(strings.ToLower(string(output)), "redis") {
		t.Fatalf("redis extension missing: %v %s", err, output)
	}
}

func TestExternalSupervisorLifecycle(t *testing.T) {
	if os.Getenv("WORKMESH_RUNTIME_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_RUNTIME_EXTERNAL_TEST=1 to run Supervisor acceptance")
	}
	name := fmt.Sprintf("workmesh-acceptance-supervisor-%d", time.Now().UnixNano())
	installDir := t.TempDir()
	configDir := filepath.Join(installDir, "supervisor", "supervisor.d")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("docker", "run", "-d", "--name", name, "-v", configDir+":/etc/supervisor.d", "--entrypoint", "supervisord", "1panel-php-fpm:8.5.10", "--nodaemon", "--configuration", "/etc/supervisord.conf").CombinedOutput(); err != nil {
		t.Skipf("PHP image unavailable: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	for attempt := 0; attempt < 30; attempt++ {
		if err := exec.Command("docker", "exec", name, "test", "-S", "/run/supervisor.sock").Run(); err == nil {
			break
		}
		if attempt == 29 {
			t.Fatal("Supervisor socket did not become ready")
		}
		time.Sleep(200 * time.Millisecond)
	}
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
	runtimeStoreMu.Lock()
	runtimeStoreInstance = nil
	runtimeStoreMu.Unlock()
	s := getRuntimeStore()
	s.mu.Lock()
	s.state.Runtimes = append(s.state.Runtimes, runtimeRecord{ID: name, Name: name, Type: "php", Container: name, InstallPath: installDir, Status: "Running", Params: map[string]any{}})
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	mux := http.NewServeMux()
	registerRuntimeRoutes(mux, s)
	body := fmt.Sprintf(`{"id":%q,"name":"worker","operate":"create","command":"/bin/sleep 120","user":"root","dir":"/www","numprocs":"1"}`, name)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/supervisor/process", strings.NewReader(body)))
	if res.Code != http.StatusOK {
		t.Fatalf("supervisor create status=%d body=%s", res.Code, res.Body.String())
	}
	var status []byte
	var statusErr error
	for attempt := 0; attempt < 20; attempt++ {
		status, statusErr = exec.Command("docker", "exec", name, "supervisorctl", "status", "worker:*").CombinedOutput()
		if statusErr == nil && strings.Contains(string(status), "RUNNING") {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if statusErr != nil || !strings.Contains(string(status), "RUNNING") {
		t.Fatalf("supervisor worker not running: %v %s", statusErr, status)
	}
	restart := httptest.NewRecorder()
	mux.ServeHTTP(restart, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/supervisor/process", strings.NewReader(fmt.Sprintf(`{"id":%q,"name":"worker","operate":"restart"}`, name))))
	if restart.Code != http.StatusOK {
		t.Fatalf("supervisor restart status=%d body=%s", restart.Code, restart.Body.String())
	}
	remove := httptest.NewRecorder()
	mux.ServeHTTP(remove, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/supervisor/process", strings.NewReader(fmt.Sprintf(`{"id":%q,"name":"worker","operate":"delete"}`, name))))
	if remove.Code != http.StatusOK {
		t.Fatalf("supervisor delete status=%d body=%s", remove.Code, remove.Body.String())
	}
}

func TestExternalPHPMultiVersionFPMConfig(t *testing.T) {
	if os.Getenv("WORKMESH_RUNTIME_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_RUNTIME_EXTERNAL_TEST=1 to run multi-version FPM acceptance")
	}
	for _, image := range []string{"1panel-php-fpm:7.4.33", "1panel-php-fpm:8.5.10"} {
		t.Run(image, func(t *testing.T) {
			name := fmt.Sprintf("workmesh-acceptance-fpm-%d", time.Now().UnixNano())
			if output, err := exec.Command("docker", "run", "-d", "--name", name, "--entrypoint", "sh", image, "-c", "sleep 120").CombinedOutput(); err != nil {
				t.Skipf("image unavailable: %v (%s)", err, output)
			}
			t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
			output, err := exec.Command("docker", "exec", name, "php-fpm", "-t").CombinedOutput()
			if err != nil || !strings.Contains(strings.ToLower(string(output)), "test is successful") {
				t.Fatalf("php-fpm config check failed: %v %s", err, output)
			}
		})
	}
}
