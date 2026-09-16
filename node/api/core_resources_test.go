// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestScriptSyncUsesConfiguredRemoteAndSQLite(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("当前沙箱禁止 loopback 监听: %v", err)
	}
	remote := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"remote-1","name":"health","script":"echo ok"}]`))
	}))
	remote.Listener = listener
	remote.Start()
	defer remote.Close()
	t.Setenv("WORKMESH_SCRIPT_REPO_URL", remote.URL)
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := ensureControlTables(db); err != nil {
		t.Fatal(err)
	}
	controlStoreMu.Lock()
	controlStoreDB = db
	controlStoreMu.Unlock()
	defer resetSharedStoreForTest()
	scriptStoreMu.Lock()
	scriptStore = nil
	scriptStoreMu.Unlock()
	mux := http.NewServeMux()
	registerCoreResourceRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/core/script/sync", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"count":1`) {
		t.Fatalf("script sync failed: %d %s", res.Code, res.Body.String())
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM script_library`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("script sync persistence failed: count=%d err=%v", count, err)
	}
}

func TestScriptRunRequiresCommandToken(t *testing.T) {
	mux := http.NewServeMux()
	registerCoreResourceRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/core/script/run?command=echo+ok", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", res.Code)
	}
}

func TestScriptLibraryPersistsInSQLiteWithoutJSON(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := ensureControlTables(db); err != nil {
		t.Fatal(err)
	}
	controlStoreMu.Lock()
	controlStoreDB = db
	controlStoreMu.Unlock()
	defer resetSharedStoreForTest()
	scriptStoreMu.Lock()
	scriptStore = nil
	scriptStoreMu.Unlock()

	mux := http.NewServeMux()
	registerCoreResourceRoutes(mux)
	created := httptest.NewRecorder()
	mux.ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/api/v2/core/script", bytes.NewBufferString(`{"name":"health","script":"echo ok","approved":true}`)))
	if created.Code != http.StatusOK {
		t.Fatalf("创建脚本失败: status=%d body=%s", created.Code, created.Body.String())
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM script_library`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("SQLite 脚本数量异常: count=%d err=%v", count, err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "scripts.json")); !os.IsNotExist(err) {
		t.Fatalf("运行时不应写入 scripts.json: %v", err)
	}

	searched := httptest.NewRecorder()
	mux.ServeHTTP(searched, httptest.NewRequest(http.MethodPost, "/api/v2/core/script/search", bytes.NewBufferString(`{"name":"health","page":1,"pageSize":20}`)))
	var response struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(searched.Body.Bytes(), &response); err != nil || response.Data.Total != 1 {
		t.Fatalf("查询脚本失败: total=%d err=%v body=%s", response.Data.Total, err, searched.Body.String())
	}
}
