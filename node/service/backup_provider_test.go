// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHTTPBackupProviderOAuthBucketsAndWrites(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "snapshot.tar")
	if err := os.WriteFile(source, []byte("backup-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	var uploaded atomic.Int32
	var deleted atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if r.Method != http.MethodPost || r.FormValue("grant_type") != "refresh_token" || r.FormValue("refresh_token") != "rt" {
				http.Error(w, "invalid oauth request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"at","refresh_token":"rt2","expires_in":120}`)
		case "/buckets":
			if r.Header.Get("Authorization") != "Bearer at" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"buckets":[{"name":"primary","region":"cn"},"archive"]}`)
		case "/upload":
			if r.Header.Get("Authorization") != "Bearer at" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if err := r.ParseMultipartForm(16 << 20); err != nil || r.FormValue("path") != "archive/snapshot.tar" {
				http.Error(w, "invalid upload", http.StatusBadRequest)
				return
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				http.Error(w, "missing file", http.StatusBadRequest)
				return
			}
			defer file.Close()
			body, _ := io.ReadAll(file)
			if string(body) != "backup-data" {
				http.Error(w, "invalid content", http.StatusBadRequest)
				return
			}
			uploaded.Add(1)
		case "/delete":
			var payload map[string]string
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if r.Header.Get("Authorization") != "Bearer at" || payload["path"] != "archive/snapshot.tar" {
				http.Error(w, "invalid delete", http.StatusBadRequest)
				return
			}
			deleted.Add(1)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	provider, err := NewHTTPBackupProvider("s3", map[string]any{
		"refresh_url":  server.URL + "/token",
		"buckets_url":  server.URL + "/buckets",
		"upload_url":   server.URL + "/upload",
		"delete_url":   server.URL + "/delete",
		"access_token": "at",
	}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	token, err := provider.RefreshToken(context.Background(), "rt")
	if err != nil || token.AccessToken != "at" || token.RefreshToken != "rt2" {
		t.Fatalf("oauth result=%+v err=%v", token, err)
	}
	provider.accessToken = token.AccessToken
	buckets, err := provider.ListBuckets(context.Background())
	if err != nil || len(buckets) != 2 || buckets[0].Name != "primary" || buckets[0].Region != "cn" {
		t.Fatalf("buckets=%+v err=%v", buckets, err)
	}
	if err := provider.Upload(context.Background(), source, "archive/snapshot.tar"); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if err := provider.Delete(context.Background(), "archive/snapshot.tar"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if uploaded.Load() != 1 || deleted.Load() != 1 {
		t.Fatalf("write counts upload=%d delete=%d", uploaded.Load(), deleted.Load())
	}
}

func TestHTTPBackupProviderRetriesTransientFailure(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `["ok"]`)
	}))
	defer server.Close()
	provider, err := NewHTTPBackupProvider("s3", map[string]any{"buckets_url": server.URL}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	buckets, err := provider.ListBuckets(context.Background())
	if err != nil || len(buckets) != 1 || buckets[0].Name != "ok" || attempts.Load() != 3 {
		t.Fatalf("retry buckets=%+v err=%v attempts=%d", buckets, err, attempts.Load())
	}
}

func TestHTTPBackupProviderRejectsUnsafeEndpointAndOversize(t *testing.T) {
	unsafe, err := NewHTTPBackupProvider("s3", map[string]any{"buckets_url": "https://example.invalid/list?token=secret"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unsafe.ListBuckets(context.Background()); err == nil || !strings.Contains(err.Error(), "端点") {
		t.Fatalf("unsafe endpoint error=%v", err)
	}
	provider, err := NewHTTPBackupProvider("s3", map[string]any{"upload_url": "https://example.invalid/upload"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "large.bin")
	if err := os.WriteFile(source, make([]byte, int(BackupProviderMaxUpload+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := provider.Upload(context.Background(), source, "large.bin"); err == nil || !strings.Contains(err.Error(), "大小限制") {
		t.Fatalf("oversize upload error=%v", err)
	}
}
