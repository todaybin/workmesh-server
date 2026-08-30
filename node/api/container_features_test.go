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

func TestContainerRepositoryAndTemplatePersistence(t *testing.T) {
	dir := filepath.Join(".tmp", "container-feature-test")
	_ = os.RemoveAll(dir)
	t.Setenv("WORKMESH_DATA_DIR", dir)
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	post := func(path, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body)))
		return res
	}
	if res := post("/api/v2/containers/repo", `{"name":"private","downloadUrl":"https://registry.example"}`); res.Code != 200 || !strings.Contains(res.Body.String(), "private") {
		t.Fatalf("repo create: %d %s", res.Code, res.Body.String())
	}
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v2/containers/repo", nil))
	if res.Code != 200 || !strings.Contains(res.Body.String(), "private") {
		t.Fatalf("repo list: %d %s", res.Code, res.Body.String())
	}
	if res := post("/api/v2/containers/template", `{"name":"web","description":"demo","content":"services:\n  web:\n    image: nginx"}`); res.Code != 200 {
		t.Fatalf("template create: %d %s", res.Code, res.Body.String())
	}
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/containers/template/search", bytes.NewBufferString(`{"keyword":"web"}`)))
	if res.Code != 200 || !strings.Contains(res.Body.String(), "services") {
		t.Fatalf("template search: %d %s", res.Code, res.Body.String())
	}
	b, err := os.ReadFile(filepath.Join(dir, "containers.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state containerState
	if err := json.Unmarshal(b, &state); err != nil || len(state.Repositories) != 1 || len(state.Templates) != 1 {
		t.Fatalf("state persistence: %v %+v", err, state)
	}
}

func TestComposeCreateUpdatePinAndEnv(t *testing.T) {
	dir := filepath.Join(".tmp", "compose-feature-test")
	_ = os.RemoveAll(dir)
	t.Setenv("WORKMESH_DATA_DIR", dir)
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	composeDir := filepath.Join(dir, "compose")
	_ = os.MkdirAll(composeDir, 0o750)
	path := filepath.Join(composeDir, "docker-compose.yml")
	_ = os.WriteFile(filepath.Join(composeDir, ".env"), []byte("PORT=8080\n# comment\nDEBUG=\"true\"\n"), 0o600)
	createBody, _ := json.Marshal(map[string]any{"name": "demo", "path": path, "content": "services: {}"})
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/containers/compose", bytes.NewReader(createBody)))
	if create.Code != 200 {
		t.Fatalf("compose create: %d %s", create.Code, create.Body.String())
	}
	updateBody, _ := json.Marshal(map[string]any{"name": "demo", "path": path, "content": "services:\n  app:\n    image: nginx"})
	updated := httptest.NewRecorder()
	mux.ServeHTTP(updated, httptest.NewRequest(http.MethodPost, "/api/v2/containers/compose/update", bytes.NewReader(updateBody)))
	if updated.Code != 200 {
		t.Fatalf("compose update: %d %s", updated.Code, updated.Body.String())
	}
	if content, _ := os.ReadFile(path); !strings.Contains(string(content), "nginx") {
		t.Fatalf("compose not updated: %s", content)
	}
	envBody, _ := json.Marshal(map[string]any{"path": path})
	env := httptest.NewRecorder()
	mux.ServeHTTP(env, httptest.NewRequest(http.MethodPost, "/api/v2/containers/compose/env", bytes.NewReader(envBody)))
	if env.Code != 200 || !strings.Contains(env.Body.String(), "8080") || !strings.Contains(env.Body.String(), "true") {
		t.Fatalf("compose env: %d %s", env.Code, env.Body.String())
	}
	invalid := httptest.NewRecorder()
	mux.ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/api/v2/containers/compose", bytes.NewBufferString(`{"name":"x","path":"../escape.yml","content":"services: {}"}`)))
	if invalid.Code != 400 {
		t.Fatalf("unsafe compose path status=%d", invalid.Code)
	}
	defaultCreateBody, _ := json.Marshal(map[string]any{"name": "from-edit", "from": "edit", "file": "services: {}"})
	defaultCreate := httptest.NewRecorder()
	mux.ServeHTTP(defaultCreate, httptest.NewRequest(http.MethodPost, "/api/v2/containers/compose", bytes.NewReader(defaultCreateBody)))
	if defaultCreate.Code != 200 {
		t.Fatalf("compose edit create: %d %s", defaultCreate.Code, defaultCreate.Body.String())
	}
}

func TestContainerImageImportExportRequirePath(t *testing.T) {
	if _, err := handleImageOperation(httptest.NewRequest(http.MethodPost, "/", nil), "image/save", containerRequest{Image: "nginx"}); err == nil {
		t.Fatal("image save without path should fail")
	}
	if _, err := handleImageOperation(httptest.NewRequest(http.MethodPost, "/", nil), "image/load", containerRequest{Path: "../image.tar"}); err == nil {
		t.Fatal("unsafe image load path should fail")
	}
}
