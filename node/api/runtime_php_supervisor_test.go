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
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/node/model"
)

type supervisorRuntimeExecutor struct {
	recordingRuntimeExecutor
	status string
	failAt string
}

func (s *supervisorRuntimeExecutor) Execute(ctx context.Context, request model.CommandRequest) (model.CommandResult, error) {
	s.recordingRuntimeExecutor.mu.Lock()
	s.recordingRuntimeExecutor.requests = append(s.recordingRuntimeExecutor.requests, request)
	s.recordingRuntimeExecutor.mu.Unlock()
	command := strings.Join(request.Args, " ")
	if s.failAt != "" && strings.Contains(command, s.failAt) {
		return model.CommandResult{ExitCode: 1, Stderr: "forced failure"}, nil
	}
	if strings.HasSuffix(command, "supervisorctl status") {
		return model.CommandResult{Stdout: s.status}, nil
	}
	return model.CommandResult{}, nil
}

func TestSupervisorConfigCRUDAndStatus(t *testing.T) {
	installDir := t.TempDir()
	composePath := filepath.Join(installDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &supervisorRuntimeExecutor{status: "worker:worker_00 RUNNING pid 42, uptime 0:01:02\n"}
	s := &runtimeStore{commands: executor, state: runtimeState{Runtimes: []runtimeRecord{{ID: "php85", Name: "php85", Type: "php", Container: "php85", InstallPath: installDir, ComposePath: composePath}}, Settings: map[string]any{}}}
	mux := http.NewServeMux()
	registerPHPSupervisorRoutes(mux, s)

	create := httptest.NewRecorder()
	createBody := `{"id":"php85","operate":"create","name":"worker","command":"php /www/worker.php","user":"www-data","dir":"/www","numprocs":"1","autoRestart":"true","autoStart":"true","environment":"APP_ENV=prod"}`
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/supervisor/process", strings.NewReader(createBody)))
	if create.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(installDir, "supervisor", "supervisor.d", "worker.ini"))
	if err != nil || !strings.Contains(string(content), "command=php /www/worker.php") || !strings.Contains(string(content), "environment=APP_ENV=prod") {
		t.Fatalf("supervisor config err=%v content=%s", err, content)
	}

	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v2/runtimes/supervisor/process/php85", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"PID":"42"`) || !strings.Contains(list.Body.String(), `"name":"worker"`) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}

	read := httptest.NewRecorder()
	mux.ServeHTTP(read, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/supervisor/process/file", strings.NewReader(`{"id":"php85","name":"worker","file":"config","operate":"get"}`)))
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), "worker.php") {
		t.Fatalf("read status=%d body=%s", read.Code, read.Body.String())
	}

	restart := httptest.NewRecorder()
	mux.ServeHTTP(restart, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/supervisor/process", strings.NewReader(`{"id":"php85","name":"worker","operate":"restart"}`)))
	if restart.Code != http.StatusOK || !strings.Contains(strings.Join(executor.commandLines(), "\n"), "supervisorctl restart worker:*") {
		t.Fatalf("restart status=%d commands=%v", restart.Code, executor.commandLines())
	}

	remove := httptest.NewRecorder()
	mux.ServeHTTP(remove, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/supervisor/process", strings.NewReader(`{"id":"php85","name":"worker","operate":"delete"}`)))
	if remove.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", remove.Code, remove.Body.String())
	}
	if _, err := os.Stat(filepath.Join(installDir, "supervisor", "supervisor.d", "worker.ini")); !os.IsNotExist(err) {
		t.Fatalf("supervisor config still exists: %v", err)
	}
}

func TestSupervisorRejectsTraversalAndRollsBack(t *testing.T) {
	installDir := t.TempDir()
	composePath := filepath.Join(installDir, "docker-compose.yml")
	_ = os.WriteFile(composePath, []byte("services: {}\n"), 0o600)
	executor := &supervisorRuntimeExecutor{failAt: "supervisorctl update worker"}
	s := &runtimeStore{commands: executor, state: runtimeState{Runtimes: []runtimeRecord{{ID: "php", Type: "php", Container: "php", InstallPath: installDir, ComposePath: composePath}}, Settings: map[string]any{}}}
	mux := http.NewServeMux()
	registerPHPSupervisorRoutes(mux, s)

	bad := httptest.NewRecorder()
	mux.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/supervisor/process", strings.NewReader(`{"id":"php","operate":"create","name":"../bad","command":"php x","user":"www-data","dir":"/www","numprocs":"1"}`)))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("traversal status=%d body=%s", bad.Code, bad.Body.String())
	}
	failed := httptest.NewRecorder()
	valid := `{"id":"php","operate":"create","name":"worker","command":"php x","user":"www-data","dir":"/www","numprocs":"1"}`
	mux.ServeHTTP(failed, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/supervisor/process", strings.NewReader(valid)))
	if failed.Code != http.StatusBadGateway {
		t.Fatalf("failure status=%d body=%s", failed.Code, failed.Body.String())
	}
	if _, err := os.Stat(filepath.Join(installDir, "supervisor", "supervisor.d", "worker.ini")); !os.IsNotExist(err) {
		t.Fatalf("failed create was not rolled back: %v", err)
	}
}

func TestParseFastCGIStatusPayload(t *testing.T) {
	items, err := parseFastCGIStatusPayload("Status: 200 OK\r\nContent-Type: text/plain\r\n\r\npool: www\naccepted conn: 17\nactive processes: 2\n")
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(items)
	if !strings.Contains(string(payload), `"key":"accepted conn","value":"17"`) || strings.Contains(string(payload), "Content-Type") {
		t.Fatalf("unexpected status payload: %s", payload)
	}
	if _, err := parseFastCGIStatusPayload("Content-Type: text/plain\r\n\r\n"); err == nil {
		t.Fatal("empty FastCGI status must fail")
	}
}
