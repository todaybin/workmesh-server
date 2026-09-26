// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func dashboardCurrent(_ context.Context, options ...string) map[string]any {
	ioOption, netOption := "all", "all"
	if len(options) > 0 && strings.TrimSpace(options[0]) != "" {
		ioOption = strings.TrimSpace(options[0])
	}
	if len(options) > 1 && strings.TrimSpace(options[1]) != "" {
		netOption = strings.TrimSpace(options[1])
	}
	var load1, load5, load15 float64
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) >= 3 {
			load1, _ = strconv.ParseFloat(parts[0], 64)
			load5, _ = strconv.ParseFloat(parts[1], 64)
			load15, _ = strconv.ParseFloat(parts[2], 64)
		}
	}
	memTotal, memAvail, memFree, memCache, swapTotal, swapFree := uint64(0), uint64(0), uint64(0), uint64(0), uint64(0), uint64(0)
	if file, err := os.Open("/proc/meminfo"); err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) >= 2 {
				value, _ := strconv.ParseUint(fields[1], 10, 64)
				if fields[0] == "MemTotal:" {
					memTotal = value * 1024
				}
				if fields[0] == "MemAvailable:" {
					memAvail = value * 1024
				}
				if fields[0] == "MemFree:" {
					memFree = value * 1024
				}
				if fields[0] == "Cached:" || fields[0] == "SReclaimable:" {
					memCache += value * 1024
				}
				if fields[0] == "SwapTotal:" {
					swapTotal = value * 1024
				}
				if fields[0] == "SwapFree:" {
					swapFree = value * 1024
				}
			}
		}
		file.Close()
	}
	used := uint64(0)
	if memTotal > memAvail {
		used = memTotal - memAvail
	}
	network := dashboardNetwork(netOption)
	cpuInfo := dashboardCPUInfo()
	cpuUsage, _ := cpuInfo["usedPercent"].(float64)
	if cpuUsage < 0 || cpuUsage > 100 {
		cpuUsage = 0
	}
	cpuPercent, _ := cpuInfo["perCore"].([]float64)
	if len(cpuPercent) == 0 {
		cpuPercent = []float64{cpuUsage}
	}
	detailed, _ := cpuInfo["detailed"].([]float64)
	if len(detailed) < 8 {
		detailed = append(detailed, make([]float64, 8-len(detailed))...)
	}
	io := dashboardIO(ioOption)
	swapUsed := uint64(0)
	if swapTotal > swapFree {
		swapUsed = swapTotal - swapFree
	}
	uptime := dashboardUptime()
	return map[string]any{
		"uptime": uptime, "procs": dashboardProcessCount(), "load1": load1, "load5": load5, "load15": load15,
		// 前端直接展示启动时间字符串，格式与 1Panel 的本地 DateTimeLayout 一致。
		"timeSinceUptime": time.Unix(time.Now().Unix()-int64(uptime), 0).Format("2006-01-02 15:04:05"),
		"runningTime": map[string]uint64{
			"days":    uptime / 86400,
			"hours":   (uptime % 86400) / 3600,
			"minutes": (uptime % 3600) / 60,
			"seconds": uptime % 60,
		},
		"loadUsagePercent": dashboardLoadUsage(load1, runtime.NumCPU()), "cpuPercent": cpuPercent, "cpuUsedPercent": cpuUsage,
		"cpuDetailedPercent": detailed, "cpuUsed": cpuUsage / 100 * float64(runtime.NumCPU()), "cpuTotal": runtime.NumCPU(),
		"memoryTotal": memTotal, "memoryAvailable": memAvail, "memoryUsed": used, "memoryFree": memFree,
		"memoryShard": uint64(0), "memoryCache": memCache,
		"memoryUsedPercent": percent(used, memTotal), "swapMemoryTotal": swapTotal, "swapMemoryAvailable": swapFree, "swapMemoryUsed": swapUsed,
		"swapMemoryUsedPercent": percent(swapUsed, swapTotal), "diskData": dashboardDisks(),
		// 保留旧字段以兼容类型契约；首页已移除未使用的加速器和进程面板。
		"gpuData": []map[string]any{}, "npuData": []map[string]any{}, "xpuData": []map[string]any{},
		"ioReadBytes": io["readBytes"], "ioWriteBytes": io["writeBytes"], "ioCount": io["count"], "ioReadTime": io["readTime"], "ioWriteTime": io["writeTime"],
		"topCPUItems": []map[string]any{}, "topMemItems": []map[string]any{},
		"netBytesSent": network["bytesSent"], "netBytesRecv": network["bytesRecv"], "shotTime": time.Now().UTC(),
	}
}

// dashboardNetwork 从 Linux 内核接口汇总网络字节；其他系统返回明确的 unsupported 状态。
func dashboardNetwork(selected ...string) map[string]any {
	option := "all"
	if len(selected) > 0 && strings.TrimSpace(selected[0]) != "" {
		option = strings.TrimSpace(selected[0])
	}
	result := map[string]any{"bytesSent": uint64(0), "bytesRecv": uint64(0), "interfaces": make([]map[string]any, 0), "supported": false}
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return result
	}
	defer file.Close()
	result["supported"] = true
	interfaces := make([]map[string]any, 0)
	var sent, recv uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}
		rx, e1 := strconv.ParseUint(fields[0], 10, 64)
		tx, e2 := strconv.ParseUint(fields[8], 10, 64)
		if e1 != nil || e2 != nil {
			continue
		}
		name := strings.TrimSpace(parts[0])
		recv += rx
		sent += tx
		interfaces = append(interfaces, map[string]any{"name": name, "bytesRecv": rx, "bytesSent": tx})
		if option != "all" && option == name {
			result["bytesRecv"], result["bytesSent"] = rx, tx
		}
	}
	if option != "all" {
		if _, ok := result["bytesRecv"].(uint64); !ok {
			result["bytesRecv"], result["bytesSent"] = uint64(0), uint64(0)
		}
	}
	if option == "all" {
		result["bytesSent"], result["bytesRecv"] = sent, recv
	}
	result["interfaces"] = interfaces
	return result
}

// dashboardDisks 返回挂载点清单，避免在未授权时执行外部 df 命令。
func dashboardDisks() []map[string]any {
	disks := make([]map[string]any, 0)
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return disks
	}
	defer file.Close()
	seen := make(map[string]struct{})
	seenDevice := make(map[string]struct{})
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		if len(disks) >= 64 {
			break
		}
		device, filesystem, mount := unescapeProcField(fields[0]), fields[2], unescapeProcField(fields[1])
		if !dashboardShouldIncludeMount(device, filesystem, mount) {
			continue
		}
		if _, ok := seen[mount]; ok {
			continue
		}
		// 一个本地分区可能通过 bind mount 出现在多个目录；首页只展示一次，
		// 避免同一块磁盘被重复计算和占满状态卡片。
		if _, ok := seenDevice[device]; ok {
			continue
		}
		seen[mount] = struct{}{}
		seenDevice[device] = struct{}{}
		// 前端契约使用 path/usedPercent；mount 作为兼容字段保留。无法跨平台读取磁盘用量时明确返回 0，避免 NaN/undefined 传播。
		usage := dashboardDiskUsage(mount)
		disks = append(disks, map[string]any{
			"path": mount, "mount": mount, "type": filesystem, "device": device, "filesystem": filesystem,
			"available": usage.Available, "usedPercent": usage.UsedPercent, "free": usage.Free, "total": usage.Total, "used": usage.Used,
			"inodesTotal": usage.InodesTotal, "inodesUsed": usage.InodesUsed, "inodesFree": usage.InodesFree, "inodesUsedPercent": usage.InodesUsedPercent,
		})
	}
	return disks
}

// dashboardShouldIncludeMount 保持与原节点首页一致：只显示本地块设备，
// 不把容器层、内核伪文件系统、网络盘和运行时目录当作用户磁盘。
func dashboardShouldIncludeMount(device, filesystem, mount string) bool {
	device = strings.TrimSpace(device)
	filesystem = strings.ToLower(strings.TrimSpace(filesystem))
	mount = strings.TrimSpace(mount)
	if device == "" || filesystem == "" || mount == "" || mount == "-" {
		return false
	}
	if !strings.HasPrefix(device, "/dev/") {
		return false
	}
	if strings.Contains(mount, "/docker/") || strings.Contains(mount, "/containerd/") || strings.Contains(mount, "/podman/") || strings.HasPrefix(mount, "/snap/") {
		return false
	}
	for _, excluded := range []string{"/mnt/cdrom", "/boot", "/boot/efi", "/dev", "/dev/shm", "/run/lock", "/run", "/run/shm", "/run/user"} {
		if mount == excluded || strings.HasPrefix(mount, excluded+"/") {
			return false
		}
	}
	// These filesystems can be backed by a device but represent a container,
	// userspace or kernel mount rather than a local disk users should monitor.
	switch filesystem {
	case "overlay", "aufs", "squashfs", "tmpfs", "devtmpfs", "proc", "sysfs", "sysfs2", "devpts", "cgroup", "cgroup2", "mqueue", "pstore", "securityfs", "debugfs", "tracefs", "fusectl", "configfs", "efivarfs", "hugetlbfs", "binfmt_misc", "nsfs", "autofs", "rpc_pipefs":
		return false
	}
	// Network and FUSE filesystems are mounted resources, not local disks.
	if strings.HasPrefix(filesystem, "fuse") || strings.Contains(filesystem, "nfs") || strings.Contains(filesystem, "cifs") || filesystem == "sshfs" {
		return false
	}
	return true
}

// dashboardAccelerators 统一描述可选硬件能力，未检测到驱动时明确返回原因。
func dashboardAccelerators(kind string) []map[string]any {
	return []map[string]any{{"type": kind, "available": false, "reason": "未检测到可用驱动", "devices": []map[string]any{{"status": "unavailable"}}}}
}

func dashboardUptime() uint64 {
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			value, parseErr := strconv.ParseFloat(fields[0], 64)
			if parseErr == nil && value >= 0 {
				return uint64(value)
			}
		}
	}
	return 0
}
func dashboardProcesses() []map[string]any {
	if output, err := exec.Command("ps", "-eo", "pid=,comm=,user=,pcpu=,rss=,args=").Output(); err == nil {
		items := make([]map[string]any, 0, 128)
		for _, line := range strings.Split(string(output), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 5 {
				continue
			}
			pid, err := strconv.Atoi(fields[0])
			if err != nil || pid <= 0 {
				continue
			}
			percentValue, _ := strconv.ParseFloat(fields[3], 64)
			rssKB, _ := strconv.ParseUint(fields[4], 10, 64)
			items = append(items, map[string]any{"name": fields[1], "pid": pid, "percent": percentValue, "memory": rssKB * 1024, "cmd": strings.Join(fields[5:], " "), "user": fields[2]})
			if len(items) >= 256 {
				break
			}
		}
		if len(items) > 0 {
			return items
		}
	}
	memory := uint64(0)
	if raw, err := os.ReadFile("/proc/self/statm"); err == nil {
		fields := strings.Fields(string(raw))
		if len(fields) > 1 {
			if pages, parseErr := strconv.ParseUint(fields[1], 10, 64); parseErr == nil {
				memory = pages * uint64(os.Getpagesize())
			}
		}
	}
	return []map[string]any{{"name": filepath.Base(os.Args[0]), "pid": os.Getpid(), "percent": 0.0, "memory": memory, "cmd": strings.Join(os.Args, " "), "user": ""}}
}

// dashboardDiskUsageResult 是首页磁盘卡片使用的容量和 inode 快照。
type dashboardDiskUsageResult struct {
	Total             uint64
	Free              uint64
	Used              uint64
	UsedPercent       float64
	InodesTotal       uint64
	InodesUsed        uint64
	InodesFree        uint64
	InodesUsedPercent float64
	Available         bool
}

// dashboardDiskUsage 由平台实现，用于读取挂载点容量；失败时返回明确的不可用状态。
func dashboardDiskUsage(path string) dashboardDiskUsageResult {
	return dashboardDiskUsagePlatform(path)
}

func dashboardUint64(value any) (uint64, bool) {
	switch number := value.(type) {
	case uint64:
		return number, true
	case int64:
		return uint64(number), number >= 0
	case float64:
		return uint64(number), number >= 0
	default:
		return 0, false
	}
}

func numberOrZero(value any) float64 {
	if number, ok := value.(float64); ok && number >= 0 && number <= 100 {
		return number
	}
	return 0
}

// dashboardCPUInfo 读取 Linux /proc/stat；无该接口的系统返回稳定的零值数组。
func dashboardCPUInfo() map[string]any {
	used, perCore, detailed := dashboardCPUUsage()
	result := map[string]any{"perCore": perCore, "detailed": detailed, "usedPercent": used, "model": "", "mhz": 0.0, "prettyDistro": ""}
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			key, value, found := strings.Cut(line, ":")
			if !found {
				continue
			}
			switch strings.TrimSpace(strings.ToLower(key)) {
			case "model name", "hardware":
				if result["model"] == "" {
					result["model"] = strings.TrimSpace(value)
				}
			case "cpu mhz":
				if number, parseErr := strconv.ParseFloat(strings.TrimSpace(value), 64); parseErr == nil {
					result["mhz"] = number
				}
			}
		}
	}
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			key, value, found := strings.Cut(line, "=")
			if found && key == "PRETTY_NAME" {
				result["prettyDistro"] = strings.Trim(strings.TrimSpace(value), "\"")
			}
		}
	}
	return result
}

// dashboardIO 汇总 Linux 块设备累计读写量，避免每次请求执行外部命令。
func dashboardIO(selected ...string) map[string]any {
	option := "all"
	if len(selected) > 0 && strings.TrimSpace(selected[0]) != "" {
		option = strings.TrimSpace(selected[0])
	}
	result := map[string]any{"readBytes": uint64(0), "writeBytes": uint64(0), "count": uint64(0), "readTime": uint64(0), "writeTime": uint64(0)}
	data, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return result
	}
	var reads, writes, sectorsRead, sectorsWrite, readTime, writeTime uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}
		name := fields[2]
		if option != "all" && option != name {
			continue
		}
		// 指定设备时保留分区，否则首页选择 sda1 这类分区会一直得到 0。
		// all 仍跳过分区，避免和整盘重复累加。
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || (option == "all" && dashboardIsPartition(name)) {
			continue
		}
		parse := func(index int) uint64 { value, _ := strconv.ParseUint(fields[index], 10, 64); return value }
		reads, writes = reads+parse(3), writes+parse(7)
		sectorsRead, sectorsWrite = sectorsRead+parse(5), sectorsWrite+parse(9)
		readTime, writeTime = readTime+parse(6), writeTime+parse(10)
	}
	result["readBytes"], result["writeBytes"] = sectorsRead*512, sectorsWrite*512
	result["count"], result["readTime"], result["writeTime"] = reads+writes, readTime, writeTime
	return result
}

// dashboardIsPartition 过滤块设备分区，避免同一磁盘的累计 I/O 被重复统计。
func dashboardIsPartition(name string) bool {
	if strings.HasPrefix(name, "nvme") {
		marker := strings.LastIndexByte(name, 'p')
		return marker > 0 && marker+1 < len(name) && allDigits(name[marker+1:])
	}
	for _, prefix := range []string{"sd", "hd", "vd", "xvd", "mmcblk"} {
		if strings.HasPrefix(name, prefix) {
			trimmed := strings.TrimRight(name, "0123456789")
			return trimmed != name && strings.HasPrefix(trimmed, prefix)
		}
	}
	return false
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func percent(value, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) * 100 / float64(total)
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
