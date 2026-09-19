// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

//go:build linux

package resources

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadCgroupMemoryStat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.stat")
	if err := os.WriteFile(path, []byte("anon 1024\nfile 2048\ninactive_file 512\ninvalid nope\nnegative -1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values := readCgroupMemoryStat(path)
	if values["anon"] != 1024 || values["file"] != 2048 || values["inactive_file"] != 512 {
		t.Fatalf("unexpected cgroup stat: %#v", values)
	}
	if _, ok := values["invalid"]; ok {
		t.Fatal("invalid cgroup stat value was accepted")
	}
	if _, ok := values["negative"]; ok {
		t.Fatal("negative cgroup stat value was accepted")
	}
}
