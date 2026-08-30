// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
