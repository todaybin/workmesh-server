//go:build windows

// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

// dashboardDiskUsagePlatform Windows 下不依赖外部命令，无法读取时明确返回不可用。
// 后续可在 Windows 专用构建中接入 GetDiskFreeSpaceEx，而不影响跨平台编译。
func dashboardDiskUsagePlatform(_ string) (total, free uint64, available bool) {
	return 0, 0, false
}
