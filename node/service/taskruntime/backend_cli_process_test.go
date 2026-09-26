//go:build !windows

// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package taskruntime

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunCLIKillsUnixProcessGroupOnTimeout(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("测试环境没有 sh")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err = runCLI(ctx, sh, []string{"-c", "sleep 60 & echo $! > '" + pidFile + "'; wait"}, 4096)
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) || err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("CLI 超时错误不符合预期: ctx=%v err=%v", ctx.Err(), err)
	}
	raw, readErr := os.ReadFile(pidFile)
	if readErr != nil {
		t.Fatalf("未记录派生进程 PID: %v", readErr)
	}
	childPID, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
	if parseErr != nil || childPID <= 0 {
		t.Fatalf("派生进程 PID 无效: %q", raw)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(childPID, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("CLI 超时后派生进程仍存活: pid=%d", childPID)
}
