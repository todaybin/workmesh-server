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
	var envelope struct {
		Data struct {
			Path  string           `json:"path"`
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("search response is not JSON: %v", err)
	}
	if envelope.Data.Path != root || len(envelope.Data.Items) != 1 {
		t.Fatalf("search response does not match FileInfo contract: %#v", envelope.Data)
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

func TestFilesSaveReplacesAtomically(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "saved.txt")
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	body, _ := json.Marshal(map[string]any{"path": path, "content": "atomic-content"})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/files/save", bytes.NewReader(body)))
	if res.Code != http.StatusOK {
		t.Fatalf("save nested status=%d body=%s", res.Code, res.Body.String())
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "atomic-content" {
		t.Fatalf("saved content mismatch: %q err=%v", string(data), err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(root, "nested", ".workmesh-save-*"))
	if len(leftovers) != 0 {
		t.Fatalf("temporary save file should not remain: %v", leftovers)
	}
}

func TestCleanFilePathRejectsTraversal(t *testing.T) {
	for _, input := range []string{"../outside", "a/../../outside", `..\\outside`} {
		if _, err := cleanFilePath(input); err == nil {
			t.Fatalf("expected traversal rejection for %q", input)
		}
	}
}
