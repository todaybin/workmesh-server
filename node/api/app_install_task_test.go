package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestShowdocInstallCreatesTaskLogAndComposeEnv(t *testing.T) {
	dataDir := t.TempDir()
	binDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	resetAppStoreForTest()
	defer resetAppStoreForTest()

	dockerName := "docker"
	dockerContent := "#!/bin/sh\nexit 0\n"
	dockerMode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		dockerName = "docker.cmd"
		dockerContent = "@echo off\r\nexit /b 0\r\n"
		dockerMode = 0o644
	}
	if err := os.WriteFile(filepath.Join(binDir, dockerName), []byte(dockerContent), dockerMode); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	mux := http.NewServeMux()
	RegisterAppRoutes(mux)
	registerBackupAlertLogSettingsRoutes(mux)
	payload := `{"appDetailId":2126191141,"params":{"format":"","collation":"","PANEL_APP_PORT_HTTP":4999},"name":"showdoc","advanced":true,"containerName":"","allowPort":false,"editCompose":false,"dockerCompose":"services:\n  showdoc:\n    image: star7th/showdoc:v3.9.2\n    container_name: ${CONTAINER_NAME}\n    networks:\n      - 1panel-network\n    ports:\n      - ${PANEL_APP_PORT_HTTP}:80\nnetworks:\n  1panel-network:\n    external: true","version":"3.9.2","pullImage":true,"taskID":"d4b0d59d-0333-439b-8b2e-9a9001478c9f"}`
	install := httptest.NewRecorder()
	mux.ServeHTTP(install, httptest.NewRequest(http.MethodPost, "/api/v2/apps/install", strings.NewReader(payload)))
	if install.Code != http.StatusOK || !strings.Contains(install.Body.String(), `"taskID":"d4b0d59d-0333-439b-8b2e-9a9001478c9f"`) {
		t.Fatalf("install response=%d %s", install.Code, install.Body.String())
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		logResponse := httptest.NewRecorder()
		mux.ServeHTTP(logResponse, httptest.NewRequest(http.MethodPost, "/api/v2/logs/tasks/read", bytes.NewBufferString(`{"taskID":"d4b0d59d-0333-439b-8b2e-9a9001478c9f","page":1,"pageSize":500}`)))
		if logResponse.Code == http.StatusOK && strings.Contains(logResponse.Body.String(), `"taskStatus":"Success"`) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	envPath := filepath.Join(dataDir, "apps", "showdoc", "showdoc", ".env")
	env, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env: %v", err)
	}
	if !strings.Contains(string(env), "PANEL_APP_PORT_HTTP=4999") || !strings.Contains(string(env), "CONTAINER_NAME=WorkMesh-showdoc-") {
		t.Fatalf("unexpected env: %s", env)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(envPath), "docker-compose.yml")); err != nil {
		t.Fatalf("compose file missing: %v", err)
	}
}
