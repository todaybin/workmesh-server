// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
