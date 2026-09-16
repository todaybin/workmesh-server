// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package logsource

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileMaintenanceTruncatesAllowlistedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "access.log")
	if err := os.WriteFile(path, []byte("sensitive log\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := (FileMaintenance{}).Cleanup(context.Background(), CleanupRequest{
		Paths: []string{path, path}, Mode: CleanupTruncate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ClearedPaths) != 1 {
		t.Fatalf("cleared paths=%v", result.ClearedPaths)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("file size=%d", info.Size())
	}
}

func TestFileMaintenanceCreateAndRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "error.log")
	if _, err := (FileMaintenance{}).Cleanup(context.Background(), CleanupRequest{
		Paths: []string{path}, Mode: CleanupTruncate, CreateIfMissing: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if _, err := (FileMaintenance{}).Cleanup(context.Background(), CleanupRequest{
		Paths: []string{path}, Mode: CleanupRemove,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}
}

func TestFileMaintenanceRejectsDirectoryAndInvalidMode(t *testing.T) {
	dir := t.TempDir()
	if _, err := (FileMaintenance{}).Cleanup(context.Background(), CleanupRequest{
		Paths: []string{dir}, Mode: CleanupTruncate,
	}); err == nil {
		t.Fatal("expected directory rejection")
	}
	if _, err := (FileMaintenance{}).Cleanup(context.Background(), CleanupRequest{
		Paths: []string{filepath.Join(dir, "x")}, Mode: CleanupMode("invalid"),
	}); err == nil {
		t.Fatal("expected invalid mode rejection")
	}
}
