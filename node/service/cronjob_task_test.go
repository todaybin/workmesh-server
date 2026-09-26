// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/node/model"
	_ "modernc.org/sqlite"
)

func TestCronjobDatabaseBackupSelectsRegisteredChildren(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	db, err := sql.Open("sqlite", filepath.Join(root, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE databases (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'local', app_install_id INTEGER NOT NULL DEFAULT 0, container_name TEXT NOT NULL DEFAULT '', address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 0, initial_db TEXT NOT NULL DEFAULT '', username TEXT NOT NULL DEFAULT '', password TEXT NOT NULL DEFAULT '', ssl INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	SetSharedDatabase(db)
	t.Cleanup(func() { SetSharedDatabase(nil) })
	svc := NewDatabaseService(nil)
	for _, item := range []Database{
		{Name: "postgresql", Type: "postgresql", From: "local", Host: "127.0.0.1", Port: 5432, ContainerName: "WorkMesh-postgresql-ZNMP", Username: "workmesh"},
		{Name: "postgresql-sp", Type: "postgresql", From: "local", Host: "127.0.0.1", Port: 55432, ContainerName: "WorkMesh-postgresql-SP", Username: "sp_admin"},
		{Name: "znmp_sopvip_com", Type: "postgresql", From: "local", Host: "127.0.0.1", Port: 5432, ContainerName: "WorkMesh-postgresql-ZNMP", InitialDB: "postgresql"},
		{Name: "sp_sopvip_com", Type: "postgresql", From: "local", Host: "127.0.0.1", Port: 55432, ContainerName: "WorkMesh-postgresql-SP", InitialDB: "postgresql-sp"},
	} {
		if _, err := svc.Create(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}
	cron := newTestCronjobService(t)
	var backed []string
	cron.databaseBackup = func(_ context.Context, target cronDatabaseTarget, artifact string) error {
		backed = append(backed, target.Instance+"/"+target.Name+"@"+artifact)
		if err := os.MkdirAll(filepath.Dir(artifact), 0o750); err != nil {
			return err
		}
		return os.WriteFile(artifact, []byte("dump"), 0o640)
	}
	job, err := cron.Create(context.Background(), model.Cronjob{Name: "备份数据库", Type: "database", DBType: "postgresql", DBName: "all"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := cron.HandleOnce(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("backup: %v %s", err, result.Stderr)
	}
	joined := strings.Join(backed, "\n")
	if !strings.Contains(joined, "postgresql/znmp_sopvip_com") || !strings.Contains(joined, "postgresql-sp/sp_sopvip_com") {
		t.Fatalf("backed = %s", joined)
	}
}

func TestCronjobDirectoryArchiveAndIPGroup(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	source := filepath.Join(root, "site")
	if err := os.MkdirAll(source, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "index.html"), []byte("ok"), 0o640); err != nil {
		t.Fatal(err)
	}
	cron := newTestCronjobService(t)
	directory, err := cron.Create(context.Background(), model.Cronjob{Name: "目录", Type: "directory", SourceDir: source, RetainCopies: 1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := cron.HandleOnce(context.Background(), directory.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(strings.TrimSpace(result.Stdout)); err != nil {
		t.Fatalf("archive missing: %v %s", err, result.Stdout)
	}
	group := filepath.Join(root, "ip_group")
	if err := os.MkdirAll(filepath.Join(group, "ip_group_url"), 0o750); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("1.1.1.1\n"))
	}))
	defer server.Close()
	if err := os.WriteFile(filepath.Join(group, "ip_group_url", "office_url"), []byte(server.URL), 0o640); err != nil {
		t.Fatal(err)
	}
	syncJob, err := cron.Create(context.Background(), model.Cronjob{Name: "同步IP", Type: "syncIpGroup", SourceDir: group})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cron.HandleOnce(context.Background(), syncJob.ID); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(group, "office"))
	if err != nil || string(body) != "1.1.1.1\n" {
		t.Fatalf("ip group = %q err=%v", body, err)
	}
}
