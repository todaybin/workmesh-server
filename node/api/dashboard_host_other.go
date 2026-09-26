//go:build !linux

// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import "runtime"

// dashboardKernelArch 在非 Linux 上退回 Go 架构，避免缺少 uname 时编译失败。
func dashboardKernelArch() string {
	return runtime.GOARCH
}
