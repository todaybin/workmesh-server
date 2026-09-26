//go:build !windows

// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package taskruntime

import (
	"os/exec"
	"syscall"
)

// configureCLIProcess 将受控 CLI 放入独立进程组，便于取消时回收其派生进程。
func configureCLIProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateCLIProcess 按进程组发送终止信号；force 只在宽限期后使用。
func terminateCLIProcess(cmd *exec.Cmd, force bool) error {
	if cmd == nil || cmd.Process == nil || cmd.ProcessState != nil {
		return nil
	}
	signal := syscall.SIGTERM
	if force {
		signal = syscall.SIGKILL
	}
	return syscall.Kill(-cmd.Process.Pid, signal)
}
