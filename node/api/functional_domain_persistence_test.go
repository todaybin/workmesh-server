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
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

func TestOperationLogsReturnCompleteContract(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		resetSharedStoreForTest()
		_ = store.Close()
		functionalStoreMu.Lock()
		functionalStoreInstance = nil
		functionalStoreMu.Unlock()
	}()
	if err := SetSharedStore(store); err != nil {
		t.Fatal(err)
	}
	_, session, err := localCore.Login("admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/files/content", nil)
	req.RemoteAddr = "192.0.2.10:12345"
	req.Header.Set("CurrentNode", "primary-main")
	req.Header.Set("User-Agent", "test-agent")
	req.AddCookie(&http.Cookie{Name: "workmesh_session", Value: session.ID})
	RecordOperationLog(req, http.StatusOK, []byte(`{"code":200,"message":""}`), 2413570*time.Nanosecond)

	mux := http.NewServeMux()
	registerLogRoutes(mux, getDomainStore())
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/core/logs/operation", bytes.NewBufferString(`{"page":1,"pageSize":20}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("operation log status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Items) != 1 {
		t.Fatalf("operation log items=%d body=%s", len(payload.Data.Items), res.Body.String())
	}
	item := payload.Data.Items[0]
	if got, _ := item["id"].(float64); got < 1 {
		t.Errorf("id=%v, want numeric positive ID", item["id"])
	}
	checks := map[string]string{
		"source": "files", "user": "admin", "node": "primary-main", "ip": "192.0.2.10",
		"path": "/files/content", "method": "post", "userAgent": "test-agent", "status": "Success",
	}
	for key, want := range checks {
		if got, _ := item[key].(string); got != want {
			t.Errorf("%s=%q, want %q", key, got, want)
		}
	}
	if got, _ := item["latency"].(float64); got != 2413570 {
		t.Errorf("latency=%v, want 2413570", item["latency"])
	}
	detailZH, _ := item["detailZH"].(string)
	detailEN, _ := item["detailEN"].(string)
	if strings.TrimSpace(detailZH) == "" || strings.TrimSpace(detailEN) == "" {
		t.Errorf("localized operation details are empty: %#v", item)
	}
}

func TestFunctionalDomainLegacyImportToSQLite(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	legacy := domainState{Settings: map[string]any{"language": "en"}, Alerts: []alertItem{{ID: "alert-1", Name: "disk", Enabled: true}}}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(dataDir, "domains.json")
	if err := os.WriteFile(legacyPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		resetSharedStoreForTest()
		_ = store.Close()
		functionalStoreMu.Lock()
		functionalStoreInstance = nil
		functionalStoreMu.Unlock()
	}()
	if err := SetSharedStore(store); err != nil {
		t.Fatal(err)
	}
	loaded := getDomainStore()
	if len(loaded.state.Alerts) != 1 || loaded.state.Alerts[0].ID != "alert-1" {
		t.Fatalf("领域状态导入失败: %#v", loaded.state.Alerts)
	}
	if loaded.state.Settings["language"] != "en" {
		t.Fatalf("领域设置导入失败: %#v", loaded.state.Settings)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("旧 domains.json 未归档: %v", err)
	}
	loaded.mu.Lock()
	loaded.state.Settings["language"] = "zh"
	err = loaded.saveLocked()
	loaded.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	functionalStoreMu.Lock()
	functionalStoreInstance = nil
	functionalStoreMu.Unlock()
	restarted := getDomainStore()
	if restarted.state.Settings["language"] != "zh" {
		t.Fatalf("SQLite 重启恢复失败: %#v", restarted.state.Settings)
	}
	var count int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM functional_domain_state`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("functional_domain_state 行数=%d", count)
	}
}
