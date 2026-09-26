//go:build windows

// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package taskruntime

import (
	"fmt"
	"os/exec"
)

// configureCLIProcess 保留 Windows 的默认创建语义，终止时通过 taskkill 递归回收树。
func configureCLIProcess(_ *exec.Cmd) {}

// terminateCLIProcess 使用固定参数递归结束 CLI 进程树，避免仅留下派生 helper。
func terminateCLIProcess(cmd *exec.Cmd, _ bool) error {
	if cmd == nil || cmd.Process == nil || cmd.ProcessState != nil {
		return nil
	}
	return exec.Command("taskkill.exe", "/PID", fmt.Sprint(cmd.Process.Pid), "/T", "/F").Run()
}
