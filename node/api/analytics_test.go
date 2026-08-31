// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAnalyticsRoutesReturnTypedData(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerAnalyticsRoutes(mux)
	paths := []string{"/api/v2/status", "/api/v2/attack/stat", "/api/v2/block/search", "/api/v2/qps", "/api/v2/rank", "/api/v2/relation/stat", "/api/v2/stat", "/api/v2/test", "/api/v2/trend", "/api/v2/visitors", "/api/v2/visitors/loc"}
	for _, path := range paths {
		r := httptest.NewRecorder()
		method := http.MethodPost
		if path == "/api/v2/status" {
			method = http.MethodGet
		}
		mux.ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(`{"websiteID":1}`)))
		if r.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, r.Code, r.Body.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(r.Body.Bytes(), &envelope); err != nil || envelope["code"] != float64(200) {
			t.Fatalf("%s invalid envelope: %s", path, r.Body.String())
		}
	}
}

func TestAnalyticsConfigPersists(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerAnalyticsRoutes(mux)
	update := httptest.NewRecorder()
	mux.ServeHTTP(update, httptest.NewRequest(http.MethodPost, "/api/v2/config/site/update", strings.NewReader(`{"websiteID":7,"enabled":false}`)))
	if update.Code != http.StatusOK {
		t.Fatalf("update status=%d", update.Code)
	}
	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest(http.MethodPost, "/api/v2/config/site", strings.NewReader(`{"websiteID":7}`)))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"enabled":false`) {
		t.Fatalf("config body=%s", get.Body.String())
	}
}

func TestAnalyticsReadsBoundedAccessLogAndAggregatesMetrics(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "access.log")
	now := time.Now().UTC()
	stamp := now.Format("02/Jan/2006:15:04:05 +0000")
	content := "203.0.113.10 - - [" + stamp + "] \"GET /index.html HTTP/1.1\" 200 100 \"-\" \"Mozilla/5.0 (X11; Linux x86_64) Chrome\"\n" +
		"203.0.113.11 - - [" + stamp + "] \"GET /missing HTTP/1.1\" 404 200 \"-\" \"crawler bot\"\n"
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_ANALYTICS_LOG", logPath)
	query := map[string]any{"startTime": now.Add(-time.Minute).Format(time.RFC3339), "endTime": now.Add(time.Minute).Format(time.RFC3339)}
	data, err := analyticsData("/api/v2/stat", query)
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := data.([]map[string]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("unexpected daily rows: %#v", data)
	}
	if rows[0]["pv"] != int64(2) || rows[0]["uv"] != int64(2) || rows[0]["flow"] != int64(300) || rows[0]["count4xx"] != int64(1) || rows[0]["spider"] != int64(1) {
		t.Fatalf("unexpected aggregated metrics: %#v", rows[0])
	}
	qps, err := analyticsData("/api/v2/qps", query)
	if err != nil {
		t.Fatal(err)
	}
	qpsMap := qps.(map[string]any)
	if qpsMap["qps"] != int64(2) || qpsMap["flow"] != int64(300) || qpsMap["source"] != logPath {
		t.Fatalf("unexpected qps metrics: %#v", qpsMap)
	}
}
