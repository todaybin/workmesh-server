// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingsDaemonJSONReturnsEffectivePath(t *testing.T) {
	t.Setenv("WORKMESH_DOCKER_DAEMON_JSON", "/tmp/workmesh-daemon.json")
	mux := http.NewServeMux()
	registerBackupAlertLogSettingsRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v2/settings/daemonjson", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "/tmp/workmesh-daemon.json") {
		t.Fatalf("daemonjson path status=%d body=%s", res.Code, res.Body.String())
	}
}
