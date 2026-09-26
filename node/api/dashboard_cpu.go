// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// cpuCounters 是 /proc/stat 的一次累计快照。单次读取只能得到开机至今的平均值，不能当当前使用率。
type cpuCounters struct {
	total     uint64
	idle      uint64
	aggregate [8]uint64
	per       []cpuCoreCounters
}

type cpuCoreCounters struct {
	total uint64
	idle  uint64
}

var cpuUsageState struct {
	mu       sync.Mutex
	last     cpuCounters
	lastAt   time.Time
	usage    float64
	perCore  []float64
	detailed []float64
}

// dashboardCPUUsage 返回当前 CPU 使用率。没有历史样本时只阻塞 100ms 采样两次，之后复用上一拍差值。
func dashboardCPUUsage() (float64, []float64, []float64) {
	cpuUsageState.mu.Lock()
	fresh := cpuUsageState.last.total > 0 && time.Since(cpuUsageState.lastAt) < 3*time.Second && time.Since(cpuUsageState.lastAt) < time.Minute
	if fresh {
		usage := cpuUsageState.usage
		per := append([]float64(nil), cpuUsageState.perCore...)
		detailed := append([]float64(nil), cpuUsageState.detailed...)
		cpuUsageState.mu.Unlock()
		return usage, per, detailed
	}
	last := cpuUsageState.last
	needBaseline := last.total == 0 || time.Since(cpuUsageState.lastAt) >= time.Minute
	cpuUsageState.mu.Unlock()

	if needBaseline {
		first := readCPUCounters()
		time.Sleep(100 * time.Millisecond)
		second := readCPUCounters()
		usage, per, detailed := diffCPUUsage(first, second)
		return storeCPUUsage(usage, per, detailed, second)
	}
	current := readCPUCounters()
	usage, per, detailed := diffCPUUsage(last, current)
	return storeCPUUsage(usage, per, detailed, current)
}

func storeCPUUsage(usage float64, per []float64, detailed []float64, current cpuCounters) (float64, []float64, []float64) {
	if len(detailed) != 8 {
		detailed = []float64{0, 0, 0, 100, 0, 0, 0, 0}
	}
	if len(per) == 0 {
		per = []float64{usage}
	}
	cpuUsageState.mu.Lock()
	cpuUsageState.last = current
	cpuUsageState.lastAt = time.Now()
	cpuUsageState.usage = usage
	cpuUsageState.perCore = append([]float64(nil), per...)
	cpuUsageState.detailed = append([]float64(nil), detailed...)
	cpuUsageState.mu.Unlock()
	return usage, per, detailed
}

func diffCPUUsage(previous, current cpuCounters) (float64, []float64, []float64) {
	usage := cpuDeltaPercent(previous.total, previous.idle, current.total, current.idle)
	per := make([]float64, 0, len(current.per))
	limit := len(current.per)
	if len(previous.per) < limit {
		limit = len(previous.per)
	}
	for i := 0; i < limit; i++ {
		per = append(per, cpuDeltaPercent(previous.per[i].total, previous.per[i].idle, current.per[i].total, current.per[i].idle))
	}
	detailed := make([]float64, 8)
	deltaTotal := uint64(0)
	var delta [8]uint64
	for i := range delta {
		if current.aggregate[i] >= previous.aggregate[i] {
			delta[i] = current.aggregate[i] - previous.aggregate[i]
			deltaTotal += delta[i]
		}
	}
	if deltaTotal > 0 {
		for i, value := range delta {
			detailed[i] = percent(value, deltaTotal)
		}
	} else {
		detailed[3] = 100
	}
	return usage, per, detailed
}

func cpuDeltaPercent(previousTotal, previousIdle, currentTotal, currentIdle uint64) float64 {
	if currentTotal <= previousTotal {
		return 0
	}
	idle := uint64(0)
	if currentIdle >= previousIdle {
		idle = currentIdle - previousIdle
	}
	return percent(currentTotal-previousTotal-idle, currentTotal-previousTotal)
}

func readCPUCounters() cpuCounters {
	var counters cpuCounters
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return counters
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		if fields[0] != "cpu" && strings.Trim(fields[0][3:], "0123456789") != "" {
			continue
		}
		var values [8]uint64
		total := uint64(0)
		for i := 0; i < len(values) && i+1 < len(fields); i++ {
			values[i], _ = strconv.ParseUint(fields[i+1], 10, 64)
			total += values[i]
		}
		idle := values[3] + values[4]
		if fields[0] == "cpu" {
			counters.total = total
			counters.idle = idle
			counters.aggregate = values
			continue
		}
		counters.per = append(counters.per, cpuCoreCounters{total: total, idle: idle})
	}
	return counters
}

// dashboardLoadUsage 按 1Panel 的负载占用公式计算，分母使用逻辑核数。
func dashboardLoadUsage(load1 float64, logicalCores int) float64 {
	if logicalCores < 1 || load1 < 0 {
		return 0
	}
	usage := load1 / (float64(logicalCores*2) * 0.75) * 100
	if usage < 0 {
		return 0
	}
	return usage
}

// dashboardCPUCores 返回物理核数和逻辑核数。缺少 cpuinfo 时两者都退回 runtime.NumCPU。
func dashboardCPUCores() (physical, logical int) {
	logical = runtime.NumCPU()
	if logical < 1 {
		logical = 1
	}
	physical = logical
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return physical, logical
	}
	packages := map[string]struct{}{}
	coresPerPackage := 0
	processors := 0
	for _, line := range strings.Split(string(data), "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(strings.ToLower(key)) {
		case "processor":
			processors++
		case "physical id":
			packages[strings.TrimSpace(value)] = struct{}{}
		case "cpu cores":
			if coresPerPackage == 0 {
				coresPerPackage, _ = strconv.Atoi(strings.TrimSpace(value))
			}
		}
	}
	if processors > 0 {
		logical = processors
	}
	if coresPerPackage > 0 {
		count := len(packages)
		if count == 0 {
			count = 1
		}
		physical = coresPerPackage * count
	}
	if physical < 1 {
		physical = logical
	}
	return physical, logical
}

// dashboardProcessCount 统计 /proc 下的进程目录，而不是当前进程的 goroutine 数。
func dashboardProcessCount() uint64 {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	var count uint64
	for _, entry := range entries {
		if entry.IsDir() && allDigits(entry.Name()) {
			count++
		}
	}
	return count
}
