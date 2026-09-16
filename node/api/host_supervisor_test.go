// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostSupervisorConfigRequiresRealFileAndMutationGate(t *testing.T) {
	t.Setenv("WORKMESH_SUPERVISOR_CONFIG", t.TempDir()+"/supervisord.conf")
	t.Setenv("WORKMESH_SUPERVISOR_INCLUDE_DIR", t.TempDir()+"/conf.d")
	t.Setenv("WORKMESH_ALLOW_HOST_MUTATION", "")
	mux := http.NewServeMux()
	registerHostSupervisorRoutes(mux)

	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/tool/config/get", strings.NewReader(`{"type":"supervisord"}`)))
	if get.Code != http.StatusServiceUnavailable || !strings.Contains(get.Body.String(), "配置文件不存在") {
		t.Fatalf("missing Supervisor config status=%d body=%s", get.Code, get.Body.String())
	}

	set := httptest.NewRecorder()
	mux.ServeHTTP(set, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/tool/config/set", strings.NewReader(`{"content":"[supervisord]\n"}`)))
	if set.Code != http.StatusServiceUnavailable || !strings.Contains(set.Body.String(), "WORKMESH_ALLOW_HOST_MUTATION") {
		t.Fatalf("un gated Supervisor write status=%d body=%s", set.Code, set.Body.String())
	}
}

func TestHostSupervisorProcessRejectsInvalidNameBeforeDependencyProbe(t *testing.T) {
	t.Setenv("WORKMESH_ALLOW_HOST_MUTATION", "1")
	mux := http.NewServeMux()
	registerHostSupervisorRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/tool/supervisor/process", strings.NewReader(`{"operate":"start","name":"../bad"}`)))
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "进程名称无效") {
		t.Fatalf("invalid Supervisor process status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestHostSupervisorProcessLifecycleUsesRealConfigFiles(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "supervisord.conf")
	include := filepath.Join(root, "conf.d")
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatal(err)
	}
	ctl := filepath.Join(bin, "supervisorctl")
	if err := os.WriteFile(ctl, []byte("#!/bin/sh\ncase \"$*\" in *status*) echo 'worker RUNNING pid 42, uptime 0:01:02';; esac\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("WORKMESH_SUPERVISOR_CONFIG", config)
	t.Setenv("WORKMESH_SUPERVISOR_INCLUDE_DIR", include)
	t.Setenv("WORKMESH_ALLOW_HOST_MUTATION", "1")
	mux := http.NewServeMux()
	registerHostSupervisorRoutes(mux)
	body := `{"operate":"create","name":"worker","command":"/bin/sleep 10","user":"root","dir":"/tmp","numprocs":"1"}`
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/tool/supervisor/process", strings.NewReader(body)))
	if create.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v2/hosts/tool/supervisor/process", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"worker"`) || !strings.Contains(list.Body.String(), `"RUNNING"`) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
}
