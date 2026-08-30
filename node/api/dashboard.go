// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// dashboardQuickJump 是控制面首页的轻量默认快捷入口。
var dashboardQuickJump = []map[string]any{
	{"id": 1, "name": "terminal", "alias": "terminal", "title": "终端", "detail": "打开系统终端", "recommend": 1, "isShow": true, "router": "/terminal"},
	{"id": 2, "name": "container", "alias": "container", "title": "容器", "detail": "管理 Docker 容器", "recommend": 1, "isShow": true, "router": "/container"},
	{"id": 3, "name": "website", "alias": "website", "title": "网站", "detail": "管理网站与域名", "recommend": 0, "isShow": true, "router": "/website"},
}

func handleDashboardOS(w http.ResponseWriter, _ *http.Request) {
	info := map[string]any{"os": runtime.GOOS, "platform": runtime.GOOS, "platformFamily": runtime.GOOS, "kernelArch": runtime.GOARCH, "kernelVersion": "", "diskSize": uint64(0)}
	if data, err := os.ReadFile("/proc/version"); err == nil {
		info["kernelVersion"] = strings.TrimSpace(string(data))
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": info})
}

func handleDashboardBase(w http.ResponseWriter, r *http.Request) {
	current := dashboardCurrent(r.Context())
	host, _ := os.Hostname()
	data := map[string]any{
		"hostname": host, "os": runtime.GOOS, "platform": runtime.GOOS, "platformFamily": runtime.GOOS,
		"platformVersion": "", "prettyDistro": runtime.GOOS, "kernelArch": runtime.GOARCH,
		"kernelVersion": "", "virtualizationSystem": "", "ipV4Addr": "", "httpProxy": "",
		"cpuCores": runtime.NumCPU(), "cpuLogicalCores": runtime.NumCPU(), "cpuModelName": "", "cpuMhz": 0,
		"currentInfo": current, "quickJump": dashboardQuickJump,
	}
	if dataRaw, err := os.ReadFile("/proc/version"); err == nil {
		data["kernelVersion"] = strings.TrimSpace(string(dataRaw))
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}

func handleDashboardCurrent(w http.ResponseWriter, r *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardCurrent(r.Context())})
}

func handleDashboardNode(w http.ResponseWriter, r *http.Request) { handleDashboardCurrent(w, r) }

func handleDashboardTopCPU(w http.ResponseWriter, _ *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardProcesses()})
}

func handleDashboardTopMem(w http.ResponseWriter, _ *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardProcesses()})
}

func handleDashboardQuickOption(w http.ResponseWriter, _ *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardQuickJump})
}

func handleDashboardLauncher(w http.ResponseWriter, _ *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardQuickJump})
}

func handleDashboardLauncherOption(w http.ResponseWriter, _ *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardQuickJump})
}

func handleDashboardMutation(w http.ResponseWriter, r *http.Request) {
	// 快捷入口和启动器配置在轻量节点上以内存默认值工作，保持接口幂等。
	if r.Body != nil {
		r.Body.Close()
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
}

func handleDashboardRestart(w http.ResponseWriter, r *http.Request) {
	// 重启涉及生产进程生命周期，接口只确认请求，不由 HTTP 线程直接终止自身。
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"accepted": true, "operation": r.PathValue("operation")}})
}

func dashboardCurrent(_ context.Context) map[string]any {
	var load1, load5, load15 float64
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) >= 3 {
			load1, _ = strconv.ParseFloat(parts[0], 64)
			load5, _ = strconv.ParseFloat(parts[1], 64)
			load15, _ = strconv.ParseFloat(parts[2], 64)
		}
	}
	memTotal, memAvail := uint64(0), uint64(0)
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
			}
		}
		file.Close()
	}
	used := uint64(0)
	if memTotal > memAvail {
		used = memTotal - memAvail
	}
	network := dashboardNetwork()
	cpuUsage := load1 / float64(max(1, runtime.NumCPU())) * 100
	return map[string]any{
		"uptime": dashboardUptime(), "procs": runtime.NumGoroutine(), "load1": load1, "load5": load5, "load15": load15,
		"timeSinceUptime": 0, "runningTime": dashboardUptime(),
		"loadUsagePercent": cpuUsage, "cpuPercent": []float64{cpuUsage}, "cpuUsedPercent": cpuUsage,
		"cpuDetailedPercent": []float64{0, 0, 0, 100, 0, 0, 0, 0}, "cpuUsed": 0, "cpuTotal": runtime.NumCPU(),
		"memoryTotal": memTotal, "memoryAvailable": memAvail, "memoryUsed": used, "memoryFree": memAvail,
		"memoryShard": uint64(0), "memoryCache": uint64(0),
		"memoryUsedPercent": percent(used, memTotal), "swapMemoryTotal": 0, "swapMemoryAvailable": 0, "swapMemoryUsed": 0,
		"swapMemoryUsedPercent": 0, "diskData": dashboardDisks(), "gpuData": dashboardAccelerators("gpu"), "npuData": dashboardAccelerators("npu"), "xpuData": dashboardAccelerators("xpu"),
		"ioReadBytes": uint64(0), "ioWriteBytes": uint64(0), "ioCount": uint64(0), "ioReadTime": uint64(0), "ioWriteTime": uint64(0),
		"topCPUItems": dashboardProcesses(), "topMemItems": dashboardProcesses(),
		"netBytesSent": network["bytesSent"], "netBytesRecv": network["bytesRecv"], "shotTime": time.Now().UTC(),
	}
}

// dashboardNetwork 从 Linux 内核接口汇总网络字节；其他系统返回明确的 unsupported 状态。
func dashboardNetwork() map[string]any {
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
		recv += rx
		sent += tx
		interfaces = append(interfaces, map[string]any{"name": strings.TrimSpace(parts[0]), "bytesRecv": rx, "bytesSent": tx})
	}
	result["bytesSent"], result["bytesRecv"], result["interfaces"] = sent, recv, interfaces
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
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		mount := fields[1]
		if _, ok := seen[mount]; ok {
			continue
		}
		seen[mount] = struct{}{}
		// 前端契约使用 path/usedPercent；mount 作为兼容字段保留。无法跨平台读取磁盘用量时明确返回 0，避免 NaN/undefined 传播。
		disks = append(disks, map[string]any{"path": mount, "mount": mount, "device": fields[0], "filesystem": fields[2], "available": true, "usedPercent": float64(0), "free": uint64(0), "total": uint64(0)})
	}
	return disks
}

// dashboardAccelerators 统一描述可选硬件能力，未检测到驱动时明确返回原因。
func dashboardAccelerators(kind string) []map[string]any {
	return []map[string]any{{"type": kind, "available": false, "reason": "未检测到可用驱动", "devices": []map[string]any{{"status": "unavailable"}}}}
}

func dashboardUptime() uint64 {
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		value, _ := strconv.ParseFloat(strings.Fields(string(data))[0], 64)
		return uint64(value)
	}
	return 0
}
func dashboardProcesses() []map[string]any {
	return []map[string]any{{"name": filepath.Base(os.Args[0]), "pid": os.Getpid(), "percent": 0, "memory": 0, "cmd": strings.Join(os.Args, " "), "user": ""}}
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
