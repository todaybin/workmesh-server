//go:build windows

// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

// dashboardDiskUsagePlatform Windows 下不依赖外部命令，无法读取时明确返回不可用。
func dashboardDiskUsagePlatform(_ string) dashboardDiskUsageResult {
	return dashboardDiskUsageResult{}
}
