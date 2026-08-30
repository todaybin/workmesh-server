// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDockerStatusInfoMissingCLI(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	status, err := NewDockerService().StatusInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.IsExist || status.IsActive {
		t.Fatalf("missing docker should be unavailable: %#v", status)
	}
}

func TestDockerStatusInfoHealthyCLI(t *testing.T) {
	dir := t.TempDir()
	name := "docker"
	content := "#!/bin/sh\nprintf '%s' '{\"Server\":{\"Version\":\"test-1.2.3\"}}'\n"
	perm := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		name = "docker.cmd"
		content = "@echo {\"Server\":{\"Version\":\"test-1.2.3\"}}\r\n"
		perm = 0o644
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	status, err := NewDockerService().StatusInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.IsExist || !status.IsActive {
		t.Fatalf("healthy docker should be active: %#v", status)
	}
	if status.Version == "" {
		t.Fatalf("version should be returned: %#v", status)
	}
}
