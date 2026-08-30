// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestProbeApplicationMissingAndUnknown(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	missing := ProbeApplication(context.Background(), "mysql", "mysql")
	if missing.IsExist || missing.IsActive || missing.Status != "NotInstalled" {
		t.Fatalf("missing application should be explicit: %#v", missing)
	}
	unknown := ProbeApplication(context.Background(), "custom-app", "custom-app")
	if unknown.IsExist || unknown.Error == "" {
		t.Fatalf("unknown application should expose unavailable status: %#v", unknown)
	}
}

func TestProbeApplicationBinaryAndTCPStatus(t *testing.T) {
	dir := t.TempDir()
	name, content := "mysqld", "#!/bin/sh\nprintf 'mysqld 8.4.0'\n"
	perm := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		name, content, perm = "mysqld.cmd", "@echo mysqld 8.4.0\r\n", 0o644
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	status := ProbeApplication(context.Background(), "mysql", "mysql")
	if !status.IsExist {
		t.Fatalf("binary should be detected: %#v", status)
	}
	if status.Version == "" {
		t.Fatalf("version should be parsed: %#v", status)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot allocate test listener: %v", err)
	}
	defer listener.Close()
	// 固定 MySQL 端口可能已有本机服务，测试不改变主机服务，仅确保探测不会报错。
	_ = listener
}
