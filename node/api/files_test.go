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
	"time"
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
	if got := envelope.Data.Items[0]["extension"]; got != ".txt" {
		t.Fatalf("search response extension=%v, want .txt", got)
	}
	res = httptest.NewRecorder()
	body, _ = json.Marshal(map[string]any{"path": path})
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/files/content", bytes.NewReader(body)))
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte("hello")) || !bytes.Contains(res.Body.Bytes(), []byte(`"extension":".txt"`)) {
		t.Fatalf("content response: %s", res.Body.String())
	}
}

func TestFilesSearchSortsByModificationTime(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old.txt")
	newPath := filepath.Join(root, "new.txt")
	if err := os.WriteFile(oldPath, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "folder"), 0755); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	sortRequest := func(order string) []map[string]any {
		body, _ := json.Marshal(map[string]any{"path": root, "sortBy": "modTime", "sortOrder": order})
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/files/search", bytes.NewReader(body)))
		if res.Code != http.StatusOK {
			t.Fatalf("sorted search status=%d body=%s", res.Code, res.Body.String())
		}
		var envelope struct {
			Data struct {
				Items []map[string]any `json:"items"`
			} `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Data.Items
	}
	ascending := sortRequest("ascending")
	if len(ascending) != 3 || ascending[0]["name"] != "folder" || ascending[1]["name"] != "old.txt" || ascending[2]["name"] != "new.txt" {
		t.Fatalf("ascending modification-time order=%v", ascending)
	}
	descending := sortRequest("descending")
	if len(descending) != 3 || descending[0]["name"] != "folder" || descending[1]["name"] != "new.txt" || descending[2]["name"] != "old.txt" {
		t.Fatalf("descending modification-time order=%v", descending)
	}
}

func TestFilesTreeReturnsRootNode(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	body, _ := json.Marshal(map[string]any{"path": root})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/files/tree", bytes.NewReader(body)))
	var treeEnvelope struct {
		Data []struct {
			Name     string           `json:"name"`
			Children []map[string]any `json:"children"`
		} `json:"data"`
	}
	if json.Unmarshal(res.Body.Bytes(), &treeEnvelope) != nil || res.Code != http.StatusOK || len(treeEnvelope.Data) != 1 || treeEnvelope.Data[0].Name == "" || len(treeEnvelope.Data[0].Children) != 2 {
		t.Fatalf("tree response=%d %s", res.Code, res.Body.String())
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
