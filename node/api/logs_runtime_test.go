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
	var firstPage struct {
		Data struct {
			Source     string           `json:"source"`
			Items      []map[string]any `json:"items"`
			HasMore    bool             `json:"hasMore"`
			NextCursor string           `json:"nextCursor"`
			Content    string           `json:"content"`
		} `json:"data"`
	}
	pageBody, _ := json.Marshal(map[string]any{"path": logPath, "pageSize": 1})
	pageRead := httptest.NewRecorder()
	mux.ServeHTTP(pageRead, httptest.NewRequest(http.MethodPost, "/api/v2/logs/system/read", bytes.NewReader(pageBody)))
	if pageRead.Code != http.StatusOK || json.Unmarshal(pageRead.Body.Bytes(), &firstPage) != nil {
		t.Fatalf("paged system read status=%d body=%s", pageRead.Code, pageRead.Body.String())
	}
	if firstPage.Data.Source != "file" || len(firstPage.Data.Items) != 1 || !firstPage.Data.HasMore || firstPage.Data.NextCursor == "" {
		t.Fatalf("unexpected v2 page=%s", pageRead.Body.String())
	}
	if firstPage.Data.Content != "line one\n" {
		t.Fatalf("legacy content should match page, got %q", firstPage.Data.Content)
	}
	secondBody, _ := json.Marshal(map[string]any{"path": logPath, "pageSize": 1, "cursor": firstPage.Data.NextCursor})
	secondRead := httptest.NewRecorder()
	mux.ServeHTTP(secondRead, httptest.NewRequest(http.MethodPost, "/api/v2/logs/system/read", bytes.NewReader(secondBody)))
	var secondPage struct {
		Data struct {
			Items      []map[string]any `json:"items"`
			HasMore    bool             `json:"hasMore"`
			NextCursor string           `json:"nextCursor"`
		} `json:"data"`
	}
	if secondRead.Code != http.StatusOK || json.Unmarshal(secondRead.Body.Bytes(), &secondPage) != nil || len(secondPage.Data.Items) != 1 || !secondPage.Data.HasMore {
		t.Fatalf("unexpected second page status=%d body=%s", secondRead.Code, secondRead.Body.String())
	}
	if secondPage.Data.Items[0]["message"] != "line two" {
		t.Fatalf("second page item=%v", secondPage.Data.Items[0])
	}
	thirdBody, _ := json.Marshal(map[string]any{"path": logPath, "pageSize": 1, "cursor": secondPage.Data.NextCursor})
	thirdRead := httptest.NewRecorder()
	mux.ServeHTTP(thirdRead, httptest.NewRequest(http.MethodPost, "/api/v2/logs/system/read", bytes.NewReader(thirdBody)))
	var thirdPage struct {
		Data struct {
			Items   []map[string]any `json:"items"`
			HasMore bool             `json:"hasMore"`
		} `json:"data"`
	}
	if thirdRead.Code != http.StatusOK || json.Unmarshal(thirdRead.Body.Bytes(), &thirdPage) != nil || len(thirdPage.Data.Items) != 1 || thirdPage.Data.HasMore {
		t.Fatalf("unexpected third page status=%d body=%s", thirdRead.Code, thirdRead.Body.String())
	}
	invalidCursor := httptest.NewRecorder()
	mux.ServeHTTP(invalidCursor, httptest.NewRequest(http.MethodPost, "/api/v2/logs/system/read", bytes.NewBufferString(`{"path":"`+logPath+`","cursor":"invalid"}`)))
	if invalidCursor.Code != http.StatusBadRequest {
		t.Fatalf("invalid cursor status=%d body=%s", invalidCursor.Code, invalidCursor.Body.String())
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
	latestRead := httptest.NewRecorder()
	mux.ServeHTTP(latestRead, httptest.NewRequest(http.MethodGet, "/api/v2/logs/tasks/read?id=task-1&page=1&pageSize=1&latest=true&operateNode=primary-main", nil))
	if latestRead.Code != http.StatusOK || !strings.Contains(latestRead.Body.String(), "line three") {
		t.Fatalf("latest task read status=%d body=%s", latestRead.Code, latestRead.Body.String())
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

func TestTaskLogFileFallbackUsesBoundedSource(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	logPath := filepath.Join(dataDir, "logs", "tasks", "bounded-task.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("first\nsecond\nthird\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := getDomainStore()
	store.mu.Lock()
	store.state.Logs = []logItem{{ID: "bounded-task", Type: "task", Level: "executing", Meta: map[string]any{"path": logPath}}}
	store.mu.Unlock()
	mux := http.NewServeMux()
	registerLogRoutes(mux, store)

	read := httptest.NewRecorder()
	mux.ServeHTTP(read, httptest.NewRequest(http.MethodPost, "/api/v2/logs/tasks/read", bytes.NewBufferString(`{"id":"bounded-task","page":2,"pageSize":1}`)))
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"second"`) {
		t.Fatalf("bounded task log read status=%d body=%s", read.Code, read.Body.String())
	}
	missing := httptest.NewRecorder()
	mux.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/api/v2/logs/tasks/read", bytes.NewBufferString(`{"path":"`+filepath.Join(dataDir, "logs", "tasks", "missing.log")+`"}`)))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing task log status=%d body=%s", missing.Code, missing.Body.String())
	}
}
