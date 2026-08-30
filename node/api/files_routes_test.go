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

func TestZipAndUnzipPath(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "a.txt"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "archive.zip")
	if err := zipPath(source, archive); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "destination")
	if err := unzipPath(archive, destination); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "source", "a.txt"))
	if err != nil || string(content) != "ok" {
		t.Fatalf("unexpected extracted file: %q %v", content, err)
	}
}

func TestFileShareLifecycle(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	root := t.TempDir()
	path := filepath.Join(root, "share.txt")
	if err := os.WriteFile(path, []byte("shared"), 0o600); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	payload, _ := json.Marshal(map[string]string{"path": path})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/files/share/create", bytes.NewReader(payload)))
	if res.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", res.Code, res.Body.String())
	}
	var envelope struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil || envelope.Data.Token == "" {
		t.Fatalf("share response=%s", res.Body.String())
	}
	check := httptest.NewRequest(http.MethodGet, "/api/v2/files/share/check?token="+envelope.Data.Token, nil)
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, check)
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte(`"exists":true`)) {
		t.Fatalf("check response=%s", res.Body.String())
	}
	deletePayload, _ := json.Marshal(map[string]string{"token": envelope.Data.Token})
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/files/share/del", bytes.NewReader(deletePayload)))
	if res.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestFileFavoriteAndRecycleLifecycle(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("note"), 0o600); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	post := func(route string, value map[string]any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(value)
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, route, bytes.NewReader(body)))
		return res
	}
	if res := post("/api/v2/files/favorite", map[string]any{"path": path}); res.Code != http.StatusOK {
		t.Fatalf("favorite create status=%d body=%s", res.Code, res.Body.String())
	}
	if res := post("/api/v2/files/favorite/search", map[string]any{"page": 1, "pageSize": 10}); res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte("note.txt")) {
		t.Fatalf("favorite search=%d body=%s", res.Code, res.Body.String())
	}
	res := post("/api/v2/files/del", map[string]any{"path": path, "forceDelete": false})
	if res.Code != http.StatusOK || func() bool { _, err := os.Stat(path); return !os.IsNotExist(err) }() {
		t.Fatalf("recycle delete=%d body=%s", res.Code, res.Body.String())
	}
	search := post("/api/v2/files/recycle/search", map[string]any{"page": 1, "pageSize": 10})
	if search.Code != http.StatusOK || !bytes.Contains(search.Body.Bytes(), []byte("originalPath")) {
		t.Fatalf("recycle search=%d body=%s", search.Code, search.Body.String())
	}
	var envelope struct {
		Data struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(search.Body.Bytes(), &envelope); err != nil || len(envelope.Data.Items) != 1 {
		t.Fatalf("recycle item=%s", search.Body.String())
	}
	if res := post("/api/v2/files/recycle/reduce", map[string]any{"id": envelope.Data.Items[0].ID}); res.Code != http.StatusOK {
		t.Fatalf("recycle reduce=%d body=%s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("restored file missing: %v", err)
	}
}

func TestFileBatchCheckRoleAndAISearch(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	root := t.TempDir()
	path := filepath.Join(root, "needle.txt")
	if err := os.WriteFile(path, []byte("hello needle\nsecond line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	post := func(route string, value map[string]any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(value)
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, route, bytes.NewReader(body)))
		return res
	}
	if res := post("/api/v2/files/check", map[string]any{"path": path}); res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte(`"exist":true`)) {
		t.Fatalf("check=%d %s", res.Code, res.Body.String())
	}
	if res := post("/api/v2/files/batch/check", map[string]any{"paths": []string{path, filepath.Join(root, "missing")}}); res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte("needle.txt")) {
		t.Fatalf("batch check=%d %s", res.Code, res.Body.String())
	}
	if res := post("/api/v2/files/batch/role", map[string]any{"paths": []string{path}, "mode": 0o640, "user": "", "group": ""}); res.Code != http.StatusOK {
		t.Fatalf("batch role=%d %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("权限更新后文件不存在: %v", err)
	}
	if res := post("/api/v2/files/ai-search", map[string]any{"path": root, "query": "needle", "containSub": true}); res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte("needle.txt")) {
		t.Fatalf("ai search=%d %s", res.Code, res.Body.String())
	}
}

func TestFileShareQueryAliasesAndWgetKeys(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	root := t.TempDir()
	path := filepath.Join(root, "share.txt")
	if err := os.WriteFile(path, []byte("shared"), 0o600); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	body, _ := json.Marshal(map[string]any{"path": path})
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/files/share/create", bytes.NewReader(body)))
	var envelope struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(create.Body.Bytes(), &envelope)
	for _, endpoint := range []string{"share/check", "share/info", "share/qrcode"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v2/files/"+endpoint+"?code="+envelope.Data.Token, nil))
		if res.Code != http.StatusOK {
			t.Fatalf("%s=%d %s", endpoint, res.Code, res.Body.String())
		}
	}
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v2/files/wget/process/keys", nil))
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte(`"keys"`)) {
		t.Fatalf("wget keys=%d %s", res.Code, res.Body.String())
	}
}
