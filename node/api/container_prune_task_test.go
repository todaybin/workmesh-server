// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContainerBuildCachePruneCreatesReadableTaskLog(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nprintf 'docker %s\\n' \"$*\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	functionalStoreMu.Lock()
	functionalStoreInstance = nil
	functionalStoreMu.Unlock()
	t.Cleanup(func() {
		functionalStoreMu.Lock()
		functionalStoreInstance = nil
		functionalStoreMu.Unlock()
	})

	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	registerBackupAlertLogSettingsRoutes(mux)
	prune := httptest.NewRecorder()
	mux.ServeHTTP(prune, httptest.NewRequest(http.MethodPost, "/api/v2/containers/prune", bytes.NewBufferString(`{"taskID":"prune-task","pruneType":"buildcache","withTagAll":true}`)))
	if prune.Code != http.StatusOK || !strings.Contains(prune.Body.String(), "builder prune -f -a") {
		t.Fatalf("prune status=%d body=%s", prune.Code, prune.Body.String())
	}

	logResponse := httptest.NewRecorder()
	mux.ServeHTTP(logResponse, httptest.NewRequest(http.MethodPost, "/api/v2/logs/tasks/read?operateNode=primary-main", bytes.NewBufferString(`{"taskID":"prune-task","page":1,"pageSize":100,"latest":true}`)))
	if logResponse.Code != http.StatusOK || !strings.Contains(logResponse.Body.String(), "Docker 清理完成") || !strings.Contains(logResponse.Body.String(), "[TASK-END]") {
		t.Fatalf("task log status=%d body=%s", logResponse.Code, logResponse.Body.String())
	}
}
