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
	"testing"
)

func TestFilesSearchAndContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "hello.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	body, _ := json.Marshal(map[string]any{"path": root})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/files/search", bytes.NewReader(body)))
	if res.Code != http.StatusOK {
		t.Fatalf("search status=%d", res.Code)
	}
	res = httptest.NewRecorder()
	body, _ = json.Marshal(map[string]any{"path": path})
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/files/content", bytes.NewReader(body)))
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte("hello")) {
		t.Fatalf("content response: %s", res.Body.String())
	}
}

func TestFilesSaveAndDelete(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "saved.txt")
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	body, _ := json.Marshal(map[string]any{"path": path, "content": "saved"})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/files/save", bytes.NewReader(body)))
	if res.Code != http.StatusOK {
		t.Fatalf("save status=%d", res.Code)
	}
	body, _ = json.Marshal(map[string]any{"path": path})
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/files/del", bytes.NewReader(body)))
	if res.Code != http.StatusOK {
		t.Fatalf("delete status=%d", res.Code)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file still exists")
	}
}
