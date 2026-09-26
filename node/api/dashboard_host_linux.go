//go:build linux

// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"runtime"
	"syscall"
)

// dashboardKernelArch 返回 uname -m，首页「系统类型」据此显示 x86_64 而不是 Go 的 amd64。
func dashboardKernelArch() string {
	var name syscall.Utsname
	if err := syscall.Uname(&name); err != nil {
		return runtime.GOARCH
	}
	buffer := make([]byte, 0, len(name.Machine))
	for _, char := range name.Machine {
		if char == 0 {
			break
		}
		buffer = append(buffer, byte(char))
	}
	if len(buffer) == 0 {
		return runtime.GOARCH
	}
	return string(buffer)
}
