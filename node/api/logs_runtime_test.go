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

func TestSystemLogEndpointsCollectAndRead(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	logDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(logDir, "server.log")
	if err := os.WriteFile(logPath, []byte("line one\nline two\nline three\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	store := getDomainStore()
	registerBackupAlertLogSettingsRoutes(mux)
	files := httptest.NewRecorder()
	mux.ServeHTTP(files, httptest.NewRequest(http.MethodGet, "/api/v2/logs/system/files", nil))
	if files.Code != http.StatusOK || !strings.Contains(files.Body.String(), "server.log") {
		t.Fatalf("system files status=%d body=%s", files.Code, files.Body.String())
	}

	readBody, _ := json.Marshal(map[string]any{"path": logPath})
	read := httptest.NewRecorder()
	mux.ServeHTTP(read, httptest.NewRequest(http.MethodPost, "/api/v2/logs/system/read", bytes.NewReader(readBody)))
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), "line two") {
		t.Fatalf("system read status=%d body=%s", read.Code, read.Body.String())
	}
	forbidden := httptest.NewRecorder()
	mux.ServeHTTP(forbidden, httptest.NewRequest(http.MethodPost, "/api/v2/logs/system/read", bytes.NewBufferString(`{"path":"/etc/passwd"}`)))
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("forbidden status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}

	store.mu.Lock()
	store.state.Logs = []logItem{{ID: "task-1", Type: "task", Level: "executing", Message: "running", Meta: map[string]any{"path": logPath}}}
	store.mu.Unlock()
	taskRead := httptest.NewRecorder()
	mux.ServeHTTP(taskRead, httptest.NewRequest(http.MethodPost, "/api/v2/logs/tasks/read", bytes.NewBufferString(`{"id":"task-1","page":2,"pageSize":1}`)))
	if taskRead.Code != http.StatusOK || !strings.Contains(taskRead.Body.String(), "line two") {
		t.Fatalf("task read status=%d body=%s", taskRead.Code, taskRead.Body.String())
	}
	count := httptest.NewRecorder()
	mux.ServeHTTP(count, httptest.NewRequest(http.MethodGet, "/api/v2/logs/tasks/executing/count", nil))
	if count.Code != http.StatusOK || !strings.Contains(count.Body.String(), `"data":1`) {
		t.Fatalf("task count status=%d body=%s", count.Code, count.Body.String())
	}
}

func TestSystemLogStatusAndServicesHaveStructuredData(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerBackupAlertLogSettingsRoutes(mux)
	for _, path := range []string{"/api/v2/logs/system/status", "/api/v2/logs/system/services"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"code":200`) {
			t.Fatalf("%s status=%d body=%s", path, res.Code, res.Body.String())
		}
	}
}
