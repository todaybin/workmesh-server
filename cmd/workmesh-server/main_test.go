// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/config"
)

func TestHTTPMuxServesJavaScriptAssetsWithModuleMIME(t *testing.T) {
	root := t.TempDir()
	assetDir := filepath.Join(root, "web", "dist", "assets", "js")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "module.js"), []byte("export const ready = true;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })

	mux, _ := httpMux(configForTest())
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/js/module.js", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("静态资源状态码 = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/javascript") && !strings.HasPrefix(contentType, "application/javascript") {
		t.Fatalf("JavaScript MIME 类型错误: %q", contentType)
	}
	if !strings.Contains(recorder.Body.String(), "export const ready") {
		t.Fatalf("静态资源内容错误: %s", recorder.Body.String())
	}
}

func configForTest() config.Config {
	return config.Config{NodeID: "test-node", Role: "secondary"}
}
