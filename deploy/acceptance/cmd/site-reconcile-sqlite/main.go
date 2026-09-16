// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "", "WorkMesh SQLite database path")
	websiteRoot := flag.String("website-root", "/www/wwwroot", "website root used when site_dir is empty")
	kind := flag.String("kind", "", "export kind: websites or domains")
	flag.Parse()
	if strings.TrimSpace(*dbPath) == "" {
		fatalf("missing --db")
	}

	db, err := sql.Open("sqlite", readonlyDSN(*dbPath))
	if err != nil {
		fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		fatalf("ping sqlite: %v", err)
	}

	switch *kind {
	case "websites":
		if err := exportWebsites(ctx, db, *websiteRoot); err != nil {
			fatalf("export websites: %v", err)
		}
	case "domains":
		if err := exportDomains(ctx, db); err != nil {
			fatalf("export domains: %v", err)
		}
	default:
		fatalf("unsupported --kind %q", *kind)
	}
}

func readonlyDSN(path string) string {
	normalized := filepath.ToSlash(filepath.Clean(path))
	if volume := filepath.VolumeName(path); volume != "" {
		normalized = strings.TrimPrefix(normalized, "/")
	}
	query := url.Values{}
	query.Set("mode", "ro")
	query.Add("_pragma", "busy_timeout(5000)")
	return "file:" + normalized + "?" + query.Encode()
}

func exportWebsites(ctx context.Context, db *sql.DB, websiteRoot string) error {
	rows, err := db.QueryContext(ctx, `SELECT id, primary_domain, COALESCE(NULLIF(site_dir,''), ? || '/' || primary_domain), status, type FROM websites ORDER BY id`, strings.TrimRight(websiteRoot, "/"))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var primaryDomain, siteDir, status, siteType string
		if err := rows.Scan(&id, &primaryDomain, &siteDir, &status, &siteType); err != nil {
			return err
		}
		fmt.Printf("%d\t%s\t%s\t%s\t%s\n", id, tsv(primaryDomain), tsv(siteDir), tsv(status), tsv(siteType))
	}
	return rows.Err()
}

func exportDomains(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT website_id, domain, port, ssl FROM website_domains ORDER BY website_id, domain`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var websiteID, port, ssl int64
		var domain string
		if err := rows.Scan(&websiteID, &domain, &port, &ssl); err != nil {
			return err
		}
		fmt.Printf("%d\t%s\t%d\t%d\n", websiteID, tsv(domain), port, ssl)
	}
	return rows.Err()
}

func tsv(value string) string {
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
