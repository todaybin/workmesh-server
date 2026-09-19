// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package resources 提供不启动后台采样器的有界进程资源快照。
package resources

import "runtime"

// CgroupSnapshot 是当前进程所在 cgroup 的内存边界。
type CgroupSnapshot struct {
	CurrentBytes int64 `json:"currentBytes,omitempty"`
	HighBytes    int64 `json:"highBytes,omitempty"`
	MaxBytes     int64 `json:"maxBytes,omitempty"`
}

// Snapshot 是随既有 Gateway 心跳即时采集的进程资源状态。
type Snapshot struct {
	RSSBytes       int64          `json:"rssBytes,omitempty"`
	PSSBytes       int64          `json:"pssBytes,omitempty"`
	AnonymousBytes int64          `json:"anonymousPssBytes,omitempty"`
	FileBytes      int64          `json:"filePssBytes,omitempty"`
	SwapBytes      int64          `json:"swapBytes,omitempty"`
	HeapBytes      uint64         `json:"heapBytes"`
	HeapIdleBytes  uint64         `json:"heapIdleBytes"`
	Goroutines     int            `json:"goroutines"`
	Threads        int            `json:"threads,omitempty"`
	OpenFDs        int            `json:"openFds,omitempty"`
	Cgroup         CgroupSnapshot `json:"cgroup,omitempty"`
	ActiveLeases   map[string]int `json:"activeLeases,omitempty"`
}

// Collect 在调用线程中采集一次快照，不创建 ticker、goroutine 或持久缓存。
func Collect(activeLeases map[string]int) Snapshot {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	snapshot := collectPlatform()
	snapshot.HeapBytes = memory.HeapAlloc
	snapshot.HeapIdleBytes = memory.HeapIdle
	snapshot.Goroutines = runtime.NumGoroutine()
	if len(activeLeases) > 0 {
		snapshot.ActiveLeases = make(map[string]int, len(activeLeases))
		for key, value := range activeLeases {
			if value > 0 {
				snapshot.ActiveLeases[key] = value
			}
		}
	}
	return snapshot
}
