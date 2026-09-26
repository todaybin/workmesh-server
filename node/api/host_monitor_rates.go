// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type monitorIOPoint struct {
	read  float64
	write float64
	count float64
	time  float64
}

type monitorNetPoint struct {
	up   float64
	down float64
}

type blockCounter struct {
	readBytes  uint64
	writeBytes uint64
	readCount  uint64
	writeCount uint64
	readTime   uint64
	writeTime  uint64
}

type nicCounter struct {
	rx uint64
	tx uint64
}

var monitorCounterState struct {
	mu   sync.Mutex
	at   time.Time
	io   map[string]blockCounter
	net  map[string]nicCounter
	seen bool
}

// monitorRates 用上一拍累计计数计算每秒速率。第一次调用只建立基线，速率为 0。
func monitorRates() (map[string]monitorIOPoint, map[string]monitorNetPoint) {
	currentIO := readBlockCounters()
	currentNet := readNICCounters()
	now := time.Now()
	monitorCounterState.mu.Lock()
	defer monitorCounterState.mu.Unlock()
	elapsed := now.Sub(monitorCounterState.at).Seconds()
	previousIO, previousNet, seen := monitorCounterState.io, monitorCounterState.net, monitorCounterState.seen
	monitorCounterState.io = currentIO
	monitorCounterState.net = currentNet
	monitorCounterState.at = now
	monitorCounterState.seen = true
	ios := make(map[string]monitorIOPoint, len(currentIO))
	for name, current := range currentIO {
		point := monitorIOPoint{}
		if seen && elapsed > 0 {
			if previous, ok := previousIO[name]; ok {
				point = ioPoint(previous, current, elapsed)
			}
		}
		ios[name] = point
	}
	nets := make(map[string]monitorNetPoint, len(currentNet))
	for name, current := range currentNet {
		point := monitorNetPoint{}
		if seen && elapsed > 0 {
			if previous, ok := previousNet[name]; ok {
				point.up = counterRate(previous.tx, current.tx, elapsed) / 1024
				point.down = counterRate(previous.rx, current.rx, elapsed) / 1024
			}
		}
		nets[name] = point
	}
	return ios, nets
}

func ioPoint(previous, current blockCounter, elapsed float64) monitorIOPoint {
	readCount := counterRate(previous.readCount, current.readCount, elapsed)
	writeCount := counterRate(previous.writeCount, current.writeCount, elapsed)
	readTime := counterRate(previous.readTime, current.readTime, elapsed)
	writeTime := counterRate(previous.writeTime, current.writeTime, elapsed)
	if writeCount > readCount {
		readCount = writeCount
	}
	if writeTime > readTime {
		readTime = writeTime
	}
	return monitorIOPoint{
		read:  counterRate(previous.readBytes, current.readBytes, elapsed),
		write: counterRate(previous.writeBytes, current.writeBytes, elapsed),
		count: readCount,
		time:  readTime,
	}
}

func counterRate(previous, current uint64, elapsed float64) float64 {
	if elapsed <= 0 || current < previous {
		return 0
	}
	return float64(current-previous) / elapsed
}

func readBlockCounters() map[string]blockCounter {
	data, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return map[string]blockCounter{"all": {}}
	}
	counters := map[string]blockCounter{}
	var all blockCounter
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}
		name := fields[2]
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
			continue
		}
		parse := func(index int) uint64 {
			value, _ := strconv.ParseUint(fields[index], 10, 64)
			return value
		}
		item := blockCounter{
			readBytes: parse(5) * 512, writeBytes: parse(9) * 512,
			readCount: parse(3), writeCount: parse(7), readTime: parse(6), writeTime: parse(10),
		}
		counters[name] = item
		if !dashboardIsPartition(name) {
			all.readBytes += item.readBytes
			all.writeBytes += item.writeBytes
			all.readCount += item.readCount
			all.writeCount += item.writeCount
			all.readTime += item.readTime
			all.writeTime += item.writeTime
		}
	}
	counters["all"] = all
	return counters
}

func readNICCounters() map[string]nicCounter {
	counters := map[string]nicCounter{}
	file, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		counters["all"] = nicCounter{}
		return counters
	}
	var all nicCounter
	for _, line := range strings.Split(string(file), "\n") {
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}
		rx, errRX := strconv.ParseUint(fields[0], 10, 64)
		tx, errTX := strconv.ParseUint(fields[8], 10, 64)
		if errRX != nil || errTX != nil {
			continue
		}
		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}
		counters[name] = nicCounter{rx: rx, tx: tx}
		all.rx += rx
		all.tx += tx
	}
	counters["all"] = all
	return counters
}
