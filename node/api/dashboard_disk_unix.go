//go:build linux || darwin || freebsd || openbsd || netbsd

// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import "syscall"

// dashboardDiskUsagePlatform 使用系统 statfs 读取挂载点容量，不启动外部命令。
func dashboardDiskUsagePlatform(path string) (total, free uint64, available bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, false
	}
	blockSize := uint64(stat.Bsize)
	return uint64(stat.Blocks) * blockSize, uint64(stat.Bavail) * blockSize, true
}
