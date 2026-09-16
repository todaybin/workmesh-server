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

func TestSSHFileEndpointsUseConfiguredPaths(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "sshd_config")
	if err := os.WriteFile(config, []byte("Port 2222\nPasswordAuthentication no\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_SSHD_CONFIG", config)
	mux := http.NewServeMux()
	registerHostSSHRoutes(mux)

	read := httptest.NewRecorder()
	mux.ServeHTTP(read, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/ssh/file", bytes.NewBufferString(`{"name":"sshdConf"}`)))
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), "Port 2222") {
		t.Fatalf("read ssh config status=%d body=%s", read.Code, read.Body.String())
	}

	keys := httptest.NewRecorder()
	mux.ServeHTTP(keys, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/ssh/file", bytes.NewBufferString(`{"name":"sshdConfOptions"}`)))
	if keys.Code != http.StatusOK || !strings.Contains(keys.Body.String(), config) {
		t.Fatalf("config options status=%d body=%s", keys.Code, keys.Body.String())
	}
}

func TestSSHFileUpdateRejectsEscapeAndPreservesConfigOnRestartFailure(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "sshd_config")
	original := "Port 2222\n"
	if err := os.WriteFile(config, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_SSHD_CONFIG", config)
	// A deliberately missing service command forces the restart path to fail,
	// allowing the handler's file rollback to be verified without touching sshd.
	t.Setenv("PATH", filepath.Join(root, "missing-bin"))
	mux := http.NewServeMux()
	registerHostSSHRoutes(mux)

	escape := httptest.NewRecorder()
	mux.ServeHTTP(escape, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/ssh/file/update", bytes.NewBufferString(`{"key":"sshdConfPath","path":"/tmp/escape","value":"Port 1"}`)))
	if escape.Code != http.StatusBadRequest {
		t.Fatalf("escape status=%d body=%s", escape.Code, escape.Body.String())
	}

	update := httptest.NewRecorder()
	mux.ServeHTTP(update, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/ssh/file/update", bytes.NewBufferString(`{"key":"sshdConf","value":"Port 2200\n"}`)))
	if update.Code != http.StatusBadGateway {
		t.Fatalf("restart failure status=%d body=%s", update.Code, update.Body.String())
	}
	got, err := os.ReadFile(config)
	if err != nil || string(got) != original {
		t.Fatalf("config rollback failed: err=%v content=%q", err, got)
	}
}

func TestSSHInfoReturnsConfigValues(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "sshd_config")
	if err := os.WriteFile(config, []byte("Port 2200\nListenAddress 127.0.0.1\nPermitRootLogin prohibit-password\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_SSHD_CONFIG", config)
	t.Setenv("PATH", filepath.Join(root, "missing-bin"))
	mux := http.NewServeMux()
	registerHostSSHRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/ssh/search", bytes.NewBufferString(`{}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("search status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data["port"] != "2200" || payload.Data["listenAddress"] != "127.0.0.1" || payload.Data["permitRootLogin"] != "prohibit-password" {
		t.Fatalf("unexpected ssh info: %#v", payload.Data)
	}
}
