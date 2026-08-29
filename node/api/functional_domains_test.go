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

func TestFunctionalBackupAlertSettings(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	mux := http.NewServeMux()
	registerBackupAlertLogSettingsRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/backups/backup", bytes.NewBufferString(`{"name":"daily"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK { t.Fatalf("backup status = %d", res.Code) }
	var envelope struct { Code int `json:"code"`; Data backupItem `json:"data"` }
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil { t.Fatal(err) }
	if envelope.Code != 200 || envelope.Data.Name != "daily" || envelope.Data.ID == "" { t.Fatalf("unexpected backup: %#v", envelope) }

	alertReq := httptest.NewRequest(http.MethodPost, "/api/v2/alert/update", bytes.NewBufferString(`{"type":"cpu","name":"CPU","enabled":true}`))
	alertReq.Header.Set("Content-Type", "application/json")
	alertRes := httptest.NewRecorder(); mux.ServeHTTP(alertRes, alertReq)
	if alertRes.Code != http.StatusOK { t.Fatalf("alert status = %d", alertRes.Code) }

	settingReq := httptest.NewRequest(http.MethodPost, "/api/v2/config/global", bytes.NewBufferString(`{"language":"en"}`))
	settingRes := httptest.NewRecorder(); mux.ServeHTTP(settingRes, settingReq)
	if settingRes.Code != http.StatusOK { t.Fatalf("settings status = %d", settingRes.Code) }
	if _, err := os.Stat(filepath.Join(dataDir, "domains.json")); err != nil { t.Fatalf("state file missing: %v", err) }

	listReq := httptest.NewRequest(http.MethodPost, "/api/v2/backups/search", bytes.NewBufferString(`{}`))
	listRes := httptest.NewRecorder(); mux.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK { t.Fatalf("list status = %d", listRes.Code) }
	var list struct { Data struct { Total int `json:"total"` } `json:"data"` }
	if err := json.NewDecoder(listRes.Body).Decode(&list); err != nil { t.Fatal(err) }
	if list.Data.Total != 1 { t.Fatalf("backup total = %d, want 1", list.Data.Total) }
}

func TestFunctionalLogValidation(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux(); registerBackupAlertLogSettingsRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/logs/system/read", bytes.NewBufferString(`{}`))
	res := httptest.NewRecorder(); mux.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest { t.Fatalf("status = %d, want 400", res.Code) }
	var body map[string]any; _ = json.NewDecoder(res.Body).Decode(&body)
	if body["code"] != "ERR" { t.Fatalf("unexpected envelope: %#v", body) }
}
