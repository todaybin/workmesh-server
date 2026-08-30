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
