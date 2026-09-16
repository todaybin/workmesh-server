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

func TestSSHLogLifecycleUsesRealFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WORKMESH_SSH_LOG_DIR", dir)
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	path := filepath.Join(dir, "auth.log")
	if err := os.WriteFile(path, []byte("Jan  2 03:04:05 host sshd[1]: Accepted publickey for alice from 192.0.2.10 port 22 ssh2\nJan  2 03:05:05 host sshd[2]: Failed password for invalid user bob from 192.0.2.11 port 22 ssh2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerHostRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/ssh/log", bytes.NewBufferString(`{"page":1,"pageSize":1}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("load status=%d body=%s", res.Code, res.Body.String())
	}
	var envelope struct {
		Data struct {
			Items []sshHistory `json:"items"`
			Total int          `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Total != 2 || len(envelope.Data.Items) != 1 || envelope.Data.Items[0].Address != "192.0.2.11" {
		t.Fatalf("unexpected data=%s", res.Body.String())
	}

	export := httptest.NewRecorder()
	mux.ServeHTTP(export, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/ssh/log/export", bytes.NewBufferString(`{"status":"success"}`)))
	if export.Code != http.StatusOK || !strings.Contains(export.Body.String(), "ssh-log-") {
		t.Fatalf("export status=%d body=%s", export.Code, export.Body.String())
	}
	var exported struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(export.Body.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(exported.Data); err != nil || !bytes.Contains(b, []byte("alice")) {
		t.Fatalf("export file err=%v data=%s", err, b)
	}

	clean := httptest.NewRecorder()
	mux.ServeHTTP(clean, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/ssh/log/clean", bytes.NewBufferString(`{}`)))
	if clean.Code != http.StatusOK {
		t.Fatalf("clean status=%d body=%s", clean.Code, clean.Body.String())
	}
	if stat, err := os.Stat(path); err != nil || stat.Size() != 0 {
		t.Fatalf("log not truncated: stat=%v err=%v", stat, err)
	}
}
