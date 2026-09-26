//go:build linux || darwin || freebsd || openbsd || netbsd

// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import "syscall"

// dashboardDiskUsagePlatform 按 gopsutil 口径读取挂载点容量和 inode。
// free 使用 Bfree，使用率是 used/(used+free)，与 df 的可用块 Bavail 不同。
func dashboardDiskUsagePlatform(path string) dashboardDiskUsageResult {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return dashboardDiskUsageResult{}
	}
	blockSize := uint64(stat.Bsize)
	if blockSize == 0 {
		return dashboardDiskUsageResult{}
	}
	result := dashboardDiskUsageResult{
		Total:       stat.Blocks * blockSize,
		Free:        stat.Bfree * blockSize,
		InodesTotal: stat.Files,
		InodesFree:  stat.Ffree,
		Available:   true,
	}
	if stat.Blocks > stat.Bfree {
		result.Used = (stat.Blocks - stat.Bfree) * blockSize
	}
	if result.Used+result.Free > 0 {
		result.UsedPercent = float64(result.Used) * 100 / float64(result.Used+result.Free)
	}
	if result.InodesTotal > result.InodesFree {
		result.InodesUsed = result.InodesTotal - result.InodesFree
	}
	if result.InodesTotal > 0 {
		result.InodesUsedPercent = float64(result.InodesUsed) * 100 / float64(result.InodesTotal)
	}
	return result
}
