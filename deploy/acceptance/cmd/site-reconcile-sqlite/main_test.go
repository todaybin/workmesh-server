// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestExportWebsitesAndDomains(t *testing.T) {
	dbPath := createSQLiteFixture(t)
	db, err := sql.Open("sqlite", readonlyDSN(dbPath))
	if err != nil {
		t.Fatalf("open readonly sqlite: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	websites, err := captureStdout(func() error {
		return exportWebsites(ctx, db, "/srv/www/")
	})
	if err != nil {
		t.Fatalf("export websites: %v", err)
	}
	wantWebsites := "1\texample.com\t/srv/www/example.com\trunning\tstatic\n" +
		"2\tspaced domain\t/custom/site\tstopped\tproxy\n"
	if websites != wantWebsites {
		t.Fatalf("unexpected websites export\nwant: %q\n got: %q", wantWebsites, websites)
	}

	domains, err := captureStdout(func() error {
		return exportDomains(ctx, db)
	})
	if err != nil {
		t.Fatalf("export domains: %v", err)
	}
	wantDomains := "1\twww.example.com\t443\t1\n2\talias.example.com\t80\t0\n"
	if domains != wantDomains {
		t.Fatalf("unexpected domains export\nwant: %q\n got: %q", wantDomains, domains)
	}
}

func TestReadonlyDSNDoesNotCreateMissingDatabase(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.db")
	db, err := sql.Open("sqlite", readonlyDSN(missing))
	if err != nil {
		t.Fatalf("open readonly sqlite handle: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err == nil {
		t.Fatal("expected ping to fail for missing readonly database")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("readonly DSN must not create missing database, stat err=%v", err)
	}
}

func TestCLIRejectsUnsupportedExportKind(t *testing.T) {
	dbPath := createSQLiteFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", ".", "--db", dbPath, "--kind", "waf")
	cmd.Env = append(os.Environ(),
		"GOWORK=off",
		"GOCACHE="+filepath.Join(os.TempDir(), "workmesh-go-cache"),
	)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("go run timed out: %v\n%s", ctx.Err(), out)
	}
	if err == nil {
		t.Fatalf("unsupported kind should fail\n%s", out)
	}
	if !strings.Contains(string(out), `unsupported --kind "waf"`) {
		t.Fatalf("unexpected unsupported kind output: %s", out)
	}
}

func TestTSVSanitizesControlWhitespace(t *testing.T) {
	got := tsv("  a\tb\r\nc  ")
	if got != "a b  c" {
		t.Fatalf("unexpected sanitized value: %q", got)
	}
}

func createSQLiteFixture(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "workmesh.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite fixture: %v", err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE websites (id INTEGER PRIMARY KEY, primary_domain TEXT, site_dir TEXT, status TEXT, type TEXT)`,
		`CREATE TABLE website_domains (website_id INTEGER, domain TEXT, port INTEGER, ssl INTEGER)`,
		`INSERT INTO websites (id, primary_domain, site_dir, status, type) VALUES (1, 'example.com', '', 'running', 'static')`,
		`INSERT INTO websites (id, primary_domain, site_dir, status, type) VALUES (2, 'spaced'||char(9)||'domain', '/custom/site', 'stopped', 'proxy')`,
		`INSERT INTO website_domains (website_id, domain, port, ssl) VALUES (1, 'www.example.com', 443, 1)`,
		`INSERT INTO website_domains (website_id, domain, port, ssl) VALUES (2, 'alias.example.com', 80, 0)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("exec fixture statement %q: %v", statement, err)
		}
	}
	return dbPath
}

func captureStdout(fn func() error) (string, error) {
	old := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = write
	err = fn()
	_ = write.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, readErr := io.Copy(&buf, read)
	_ = read.Close()
	if err != nil {
		return "", err
	}
	if readErr != nil {
		return "", readErr
	}
	return strings.ReplaceAll(buf.String(), "\r\n", "\n"), nil
}
