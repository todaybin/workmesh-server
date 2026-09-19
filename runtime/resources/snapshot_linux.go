// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

//go:build linux

package resources

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func collectPlatform() Snapshot {
	snapshot := Snapshot{}
	if values := readProcKB("/proc/self/smaps_rollup"); len(values) > 0 {
		snapshot.RSSBytes = values["Rss"] * 1024
		snapshot.PSSBytes = values["Pss"] * 1024
		snapshot.AnonymousBytes = values["Pss_Anon"] * 1024
		snapshot.FileBytes = values["Pss_File"] * 1024
		snapshot.SwapBytes = values["Swap"] * 1024
	}
	status := readProcKB("/proc/self/status")
	if snapshot.RSSBytes == 0 {
		snapshot.RSSBytes = status["VmRSS"] * 1024
	}
	snapshot.Threads = int(status["Threads"])
	if entries, err := os.ReadDir("/proc/self/fd"); err == nil {
		snapshot.OpenFDs = len(entries)
	}
	snapshot.Cgroup = readCgroupV2()
	return snapshot
}

func readProcKB(filename string) map[string]int64 {
	file, err := os.Open(filename)
	if err != nil {
		return nil
	}
	defer file.Close()
	values := make(map[string]int64)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, raw, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			continue
		}
		if value, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
			values[strings.TrimSpace(key)] = value
		}
	}
	return values
}

func readCgroupV2() CgroupSnapshot {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return CgroupSnapshot{}
	}
	path := ""
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "0::") {
			path = strings.TrimPrefix(line, "0::")
			break
		}
	}
	root := filepath.Join("/sys/fs/cgroup", filepath.Clean("/"+path))
	return CgroupSnapshot{
		CurrentBytes: readCgroupLimit(filepath.Join(root, "memory.current")),
		HighBytes:    readCgroupLimit(filepath.Join(root, "memory.high")),
		MaxBytes:     readCgroupLimit(filepath.Join(root, "memory.max")),
	}
}

func readCgroupLimit(filename string) int64 {
	data, err := os.ReadFile(filename)
	if err != nil || strings.TrimSpace(string(data)) == "max" {
		return 0
	}
	value, _ := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	return value
}
