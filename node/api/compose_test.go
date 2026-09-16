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

func TestComposeRequiresPath(t *testing.T) {
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/containers/compose/operate", bytes.NewBufferString(`{"operation":"up"}`))
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest || !bytes.Contains(res.Body.Bytes(), []byte("Compose")) {
		t.Fatalf("unexpected response: %d %s", res.Code, res.Body.String())
	}
}

func TestContainerImageRejectsInvalidIdentifier(t *testing.T) {
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/containers/image/pull", bytes.NewBufferString(`{"image":"bad; rm -rf /"}`))
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest || !bytes.Contains(res.Body.Bytes(), []byte("镜像名称")) {
		t.Fatalf("unexpected response: %d %s", res.Code, res.Body.String())
	}
}

func TestComposeRebuildRunsDownThenBuildUp(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "docker.args")
	dockerPath := filepath.Join(root, "docker")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + logPath + "\"\nprintf 'compose-ok\\n'\n"
	if err := os.WriteFile(dockerPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))

	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/containers/compose/operate", bytes.NewBufferString(`{"path":"/srv/demo/docker-compose.yml","operation":"rebuild","services":["web"]}`))
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("rebuild status=%d body=%s", res.Code, res.Body.String())
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Fatalf("rebuild should invoke Docker twice, got %q", content)
	}
	if lines[0] != "compose -f /srv/demo/docker-compose.yml down --remove-orphans" {
		t.Fatalf("unexpected rebuild down args: %q", lines[0])
	}
	if lines[1] != "compose -f /srv/demo/docker-compose.yml up -d --build web" {
		t.Fatalf("unexpected rebuild up args: %q", lines[1])
	}
}
