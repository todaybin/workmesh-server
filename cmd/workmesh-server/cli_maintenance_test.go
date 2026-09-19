// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

func TestMaintenanceRequiresExactConfirmation(t *testing.T) {
	for _, args := range [][]string{{"maintenance"}, {"maintenance", "reset-history"}, {"maintenance", "reset-history", "--yes"}} {
		handled, err := runCLI(args, t.TempDir())
		if !handled || err == nil {
			t.Fatalf("args=%v handled=%v err=%v", args, handled, err)
		}
	}
}

func TestResetHistoryRemovesSixtyThousandLogsAndPreservesBusinessState(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.Open(filepath.Join(dir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := store.DB()
	for _, statement := range []string{
		`CREATE TABLE functional_domain_state (id INTEGER PRIMARY KEY, payload BLOB NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE app_store_state (id INTEGER PRIMARY KEY, payload BLOB NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE app_catalog_cache (id INTEGER PRIMARY KEY, payload BLOB NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE core_users (id INTEGER PRIMARY KEY, username TEXT NOT NULL)`,
		`CREATE TABLE websites (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE cronjobs (id TEXT PRIMARY KEY, payload BLOB NOT NULL, records BLOB NOT NULL, updated_at TEXT NOT NULL)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range historyTables {
		if table == "migration_runs" {
			if _, err := db.Exec(`INSERT INTO migration_runs(to_version,status,started_at) VALUES('old','done',?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ` + table + ` (id INTEGER PRIMARY KEY)`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO ` + table + `(id) VALUES(1)`); err != nil {
			t.Fatal(err)
		}
	}
	logs := make([]map[string]any, 60000)
	for i := range logs {
		logs[i] = map[string]any{"id": i, "message": "historical runtime line"}
	}
	domain, _ := json.Marshal(map[string]any{"logs": logs, "settings": map[string]any{"language": "zh", "securityEntrance": "keepme"}, "alerts": []any{map[string]any{"id": "alert-1"}}})
	apps, _ := json.Marshal(map[string]any{"apps": []any{map[string]any{"id": "installed-1", "key": "demo"}}, "ignored": []any{}, "storeConfig": map[string]any{"upgradeBackup": "true"}, "catalog": logs})
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO functional_domain_state VALUES(1,?,?)`, domain, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app_store_state VALUES(1,?,?)`, apps, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app_catalog_cache VALUES(1,'{}',?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO core_users VALUES(1,'admin'),(2,'operator')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO websites VALUES(1,'example.com')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO cronjobs VALUES('job-1','{}','[{"id":1}]',?)`, now); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "logs", "tasks", "app"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logs", "tasks", "app", "old.log"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logs", "server.log"), []byte("server history"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if err := resetHistory(dir); err != nil {
		t.Fatal(err)
	}
	directDB, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer directDB.Close()
	db = directDB
	for _, table := range historyTables {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	var payload []byte
	if err := db.QueryRow(`SELECT payload FROM functional_domain_state WHERE id=1`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var domainAfter map[string]any
	if err := json.Unmarshal(payload, &domainAfter); err != nil {
		t.Fatal(err)
	}
	if _, exists := domainAfter["logs"]; exists {
		t.Fatal("functional logs were retained")
	}
	settings := domainAfter["settings"].(map[string]any)
	if settings["securityEntrance"] != "keepme" || len(domainAfter["alerts"].([]any)) != 1 {
		t.Fatalf("business state changed: %#v", domainAfter)
	}
	if err := db.QueryRow(`SELECT payload FROM app_store_state WHERE id=1`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var appsAfter map[string]any
	if err := json.Unmarshal(payload, &appsAfter); err != nil {
		t.Fatal(err)
	}
	if _, exists := appsAfter["catalog"]; exists {
		t.Fatal("catalog remained in app state")
	}
	if len(appsAfter["apps"].([]any)) != 1 || appsAfter["storeConfig"].(map[string]any)["upgradeBackup"] != "true" {
		t.Fatalf("installed app state changed: %#v", appsAfter)
	}
	for _, query := range []string{`SELECT COUNT(*) FROM core_users`, `SELECT COUNT(*) FROM websites`} {
		var count int
		if err := db.QueryRow(query).Scan(&count); err != nil || count == 0 {
			t.Fatalf("preserved table query=%s count=%d err=%v", query, count, err)
		}
	}
	var records string
	if err := db.QueryRow(`SELECT records FROM cronjobs WHERE id='job-1'`).Scan(&records); err != nil || records != "[]" {
		t.Fatalf("cron records=%q err=%v", records, err)
	}
	info, err := os.Stat(filepath.Join(dir, "logs", "server.log"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("server log size=%d", info.Size())
	}
}
