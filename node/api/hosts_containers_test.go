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

func TestHostDiagnostics(t *testing.T) {
	mux := http.NewServeMux()
	registerHostRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v2/hosts/diagnostics/summary", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"code":200`) {
		t.Fatalf("unexpected host response: %d %s", res.Code, res.Body.String())
	}
}

func TestContainerMethodValidation(t *testing.T) {
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodDelete, "/api/v2/containers/demo", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", res.Code)
	}
}

func TestHostCRUDPersists(t *testing.T) {
	dir := filepath.Join(".tmp", "host-test")
	_ = os.RemoveAll(dir)
	t.Setenv("WORKMESH_DATA_DIR", dir)
	mux := http.NewServeMux()
	registerHostRoutes(mux)
	payload, _ := json.Marshal(map[string]any{"name": "test-host", "address": "127.0.0.1", "port": 22})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/hosts", bytes.NewReader(payload)))
	if res.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", res.Code, res.Body.String())
	}
	if len(loadHosts()) != 1 {
		t.Fatalf("host not persisted")
	}
}

func TestContainerPathValidation(t *testing.T) {
	if validContainerPath("relative") || validContainerPath("/var/../etc") || validContainerPath("/var/\x00x") {
		t.Fatal("unsafe path accepted")
	}
	if !validContainerPath("/var/lib/app") {
		t.Fatal("valid path rejected")
	}
}
