// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const monitorTimeLayout = "2006-01-02T15:04:05.000000000Z"

func monitorTime(value time.Time) string {
	return value.UTC().Format(monitorTimeLayout)
}

// hostMonitorSettings 是监控设置页读取的字段，名称与 1Panel MonitorSetting 一致。
type hostMonitorSettings struct {
	MonitorStatus    string `json:"monitorStatus"`
	MonitorStoreDays string `json:"monitorStoreDays"`
	MonitorInterval  string `json:"monitorInterval"`
	DefaultNetwork   string `json:"defaultNetwork"`
	DefaultIO        string `json:"defaultIO"`
}

type hostMonitorSearch struct {
	Param     string    `json:"param"`
	IO        string    `json:"io"`
	Network   string    `json:"network"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
}

var (
	hostMonitorLoopMu sync.Mutex
	hostMonitorParent context.Context
	hostMonitorCancel context.CancelFunc
)

// StartHostMonitor 在监控启用时启动唯一采样循环，进程取消时停止。
func StartHostMonitor(ctx context.Context) {
	hostMonitorLoopMu.Lock()
	hostMonitorParent = ctx
	hostMonitorLoopMu.Unlock()
	restartHostMonitorLoop()
	go func() {
		<-ctx.Done()
		hostMonitorLoopMu.Lock()
		if hostMonitorCancel != nil {
			hostMonitorCancel()
			hostMonitorCancel = nil
		}
		hostMonitorLoopMu.Unlock()
	}()
}

func restartHostMonitorLoop() {
	settings := loadHostMonitorSettings()
	hostMonitorLoopMu.Lock()
	defer hostMonitorLoopMu.Unlock()
	if hostMonitorCancel != nil {
		hostMonitorCancel()
		hostMonitorCancel = nil
	}
	if hostMonitorParent == nil || !strings.EqualFold(settings.MonitorStatus, "Enable") {
		return
	}
	ctx, cancel := context.WithCancel(hostMonitorParent)
	hostMonitorCancel = cancel
	interval := settings.interval()
	go runHostMonitorLoop(ctx, interval)
}

func runHostMonitorLoop(ctx context.Context, interval time.Duration) {
	_ = sampleHostMonitor(ctx)
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			_ = sampleHostMonitor(ctx)
			timer.Reset(interval)
		}
	}
}

func loadHostMonitorSettings() hostMonitorSettings {
	state := loadHostOperationalState()
	return normalizeHostMonitorSettings(state.Monitor)
}

func normalizeHostMonitorSettings(raw map[string]any) hostMonitorSettings {
	settings := hostMonitorSettings{MonitorStatus: "Enable", MonitorStoreDays: "7", MonitorInterval: "300", DefaultNetwork: "all", DefaultIO: "all"}
	if raw == nil {
		return settings
	}
	if value := monitorString(raw["monitorStatus"]); value != "" {
		settings.MonitorStatus = value
	}
	if value := monitorString(raw["monitorStoreDays"]); value != "" {
		settings.MonitorStoreDays = value
	}
	if value := monitorString(raw["monitorInterval"]); value != "" {
		settings.MonitorInterval = value
	}
	if value := monitorString(raw["defaultNetwork"]); value != "" {
		settings.DefaultNetwork = value
	}
	if value := monitorString(raw["defaultIO"]); value != "" {
		settings.DefaultIO = value
	}
	if _, ok := raw["monitorStoreDays"]; !ok && monitorString(raw["key"]) == "MonitorStoreDays" {
		if value := monitorString(raw["value"]); value != "" {
			settings.MonitorStoreDays = value
		}
	}
	if !strings.EqualFold(settings.MonitorStatus, "Enable") {
		settings.MonitorStatus = "Disable"
	} else {
		settings.MonitorStatus = "Enable"
	}
	return settings
}

func (s hostMonitorSettings) interval() time.Duration {
	seconds, _ := strconv.Atoi(strings.TrimSpace(s.MonitorInterval))
	if seconds < 10 {
		seconds = 300
	}
	if seconds > 43200 {
		seconds = 43200
	}
	return time.Duration(seconds) * time.Second
}

func (s hostMonitorSettings) storeDays() int {
	days, _ := strconv.Atoi(strings.TrimSpace(s.MonitorStoreDays))
	if days < 1 {
		return 7
	}
	if days > 3650 {
		return 3650
	}
	return days
}

func updateHostMonitorSetting(key, value string) (hostMonitorSettings, error) {
	current := loadHostMonitorSettings()
	value = strings.TrimSpace(value)
	switch key {
	case "MonitorStatus":
		if !strings.EqualFold(value, "Enable") && !strings.EqualFold(value, "Disable") {
			return current, errors.New("监控状态必须为 Enable 或 Disable")
		}
		if strings.EqualFold(value, "Enable") {
			current.MonitorStatus = "Enable"
		} else {
			current.MonitorStatus = "Disable"
		}
	case "MonitorStoreDays":
		days, err := strconv.Atoi(value)
		if err != nil || days < 1 || days > 3650 {
			return current, errors.New("监控保存天数必须是 1 到 3650 的整数")
		}
		current.MonitorStoreDays = strconv.Itoa(days)
	case "MonitorInterval":
		seconds, err := strconv.Atoi(value)
		if err != nil || seconds < 10 || seconds > 43200 {
			return current, errors.New("监控间隔必须在 10 到 43200 秒之间")
		}
		current.MonitorInterval = strconv.Itoa(seconds)
	case "DefaultNetwork", "DefaultIO":
		if value == "" || len(value) > 64 || strings.ContainsAny(value, " \t\r\n") {
			return current, errors.New("默认设备名称无效")
		}
		if key == "DefaultNetwork" {
			current.DefaultNetwork = value
		} else {
			current.DefaultIO = value
		}
	default:
		return current, errors.New("不支持的监控设置项")
	}
	state := hostOperationalState{Monitor: map[string]any{
		"monitorStatus": current.MonitorStatus, "monitorStoreDays": current.MonitorStoreDays,
		"monitorInterval": current.MonitorInterval, "defaultNetwork": current.DefaultNetwork, "defaultIO": current.DefaultIO,
	}}
	if err := saveHostOperationalState(state); err != nil {
		return current, err
	}
	return current, nil
}

func monitorString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return ""
	}
}

// overlayHostMonitorOnSettings 让首页和监控页读取同一份默认网卡、默认磁盘。
func overlayHostMonitorOnSettings(settings map[string]any) {
	if settings == nil {
		return
	}
	current := loadHostMonitorSettings()
	settings["defaultNetwork"] = current.DefaultNetwork
	settings["defaultIO"] = current.DefaultIO
}

type monitorInputError struct{ error }

func searchHostMonitor(request hostMonitorSearch) ([]map[string]any, error) {
	param := strings.TrimSpace(request.Param)
	if param == "" {
		param = "all"
	}
	switch param {
	case "all", "cpu", "memory", "load", "io", "network":
	default:
		return nil, monitorInputError{errors.New("不支持的监控查询类型")}
	}
	end := request.EndTime
	start := request.StartTime
	if end.IsZero() {
		end = time.Now()
	}
	if start.IsZero() {
		start = end.Add(-24 * time.Hour)
	}
	if end.Before(start) {
		return nil, monitorInputError{errors.New("监控结束时间不能早于开始时间")}
	}
	if end.Sub(start) > 366*24*time.Hour {
		return nil, monitorInputError{errors.New("监控查询范围不能超过 366 天")}
	}
	ioName := strings.TrimSpace(request.IO)
	if ioName == "" {
		ioName = "all"
	}
	netName := strings.TrimSpace(request.Network)
	if netName == "" {
		netName = "all"
	}
	data := make([]map[string]any, 0, 3)
	db := sharedDB()
	if param == "all" || param == "cpu" || param == "memory" || param == "load" {
		values, dates, err := queryMonitorBase(db, start, end)
		if err != nil {
			return nil, err
		}
		data = append(data, map[string]any{"param": "base", "date": dates, "value": values})
	}
	if param == "all" || param == "io" {
		values, dates, err := queryMonitorIO(db, ioName, start, end)
		if err != nil {
			return nil, err
		}
		data = append(data, map[string]any{"param": "io", "date": dates, "value": values})
	}
	if param == "all" || param == "network" {
		values, dates, err := queryMonitorNetwork(db, netName, start, end)
		if err != nil {
			return nil, err
		}
		data = append(data, map[string]any{"param": "network", "date": dates, "value": values})
	}
	return data, nil
}

func queryMonitorBase(db *sql.DB, start, end time.Time) ([]any, []time.Time, error) {
	values := make([]any, 0)
	dates := make([]time.Time, 0)
	if db == nil {
		return values, dates, nil
	}
	rows, err := db.Query(`SELECT created_at, cpu, memory, load_usage, cpu_load1, cpu_load5, cpu_load15, top_cpu, top_mem FROM monitor_bases WHERE created_at >= ? AND created_at <= ? ORDER BY created_at ASC LIMIT 20000`, monitorTime(start), monitorTime(end))
	if err != nil {
		return nil, nil, fmt.Errorf("查询监控基础数据失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var created string
		var cpu, memory, loadUsage, load1, load5, load15 float64
		var topCPU, topMem string
		if err := rows.Scan(&created, &cpu, &memory, &loadUsage, &load1, &load5, &load15, &topCPU, &topMem); err != nil {
			return nil, nil, err
		}
		stamp, err := time.Parse(monitorTimeLayout, created)
		if err != nil {
			continue
		}
		dates = append(dates, stamp)
		values = append(values, map[string]any{
			"cpu": cpu, "memory": memory, "loadUsage": loadUsage,
			"cpuLoad1": load1, "cpuLoad5": load5, "cpuLoad15": load15,
			"topCPUItems": decodeMonitorProcesses(topCPU), "topMemItems": decodeMonitorProcesses(topMem),
		})
	}
	return values, dates, rows.Err()
}

func queryMonitorIO(db *sql.DB, name string, start, end time.Time) ([]any, []time.Time, error) {
	values := make([]any, 0)
	dates := make([]time.Time, 0)
	if db == nil {
		return values, dates, nil
	}
	rows, err := db.Query(`SELECT created_at, read_bytes, write_bytes, count, time_ms FROM monitor_ios WHERE name = ? AND created_at >= ? AND created_at <= ? ORDER BY created_at ASC LIMIT 20000`, name, monitorTime(start), monitorTime(end))
	if err != nil {
		return nil, nil, fmt.Errorf("查询磁盘监控失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var created string
		var readBytes, writeBytes, count, ioTime float64
		if err := rows.Scan(&created, &readBytes, &writeBytes, &count, &ioTime); err != nil {
			return nil, nil, err
		}
		stamp, err := time.Parse(monitorTimeLayout, created)
		if err != nil {
			continue
		}
		dates = append(dates, stamp)
		values = append(values, map[string]any{"name": name, "read": readBytes, "write": writeBytes, "count": count, "time": ioTime})
	}
	return values, dates, rows.Err()
}

func queryMonitorNetwork(db *sql.DB, name string, start, end time.Time) ([]any, []time.Time, error) {
	values := make([]any, 0)
	dates := make([]time.Time, 0)
	if db == nil {
		return values, dates, nil
	}
	rows, err := db.Query(`SELECT created_at, up, down FROM monitor_networks WHERE name = ? AND created_at >= ? AND created_at <= ? ORDER BY created_at ASC LIMIT 20000`, name, monitorTime(start), monitorTime(end))
	if err != nil {
		return nil, nil, fmt.Errorf("查询网络监控失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var created string
		var up, down float64
		if err := rows.Scan(&created, &up, &down); err != nil {
			return nil, nil, err
		}
		stamp, err := time.Parse(monitorTimeLayout, created)
		if err != nil {
			continue
		}
		dates = append(dates, stamp)
		values = append(values, map[string]any{"name": name, "up": up, "down": down})
	}
	return values, dates, rows.Err()
}

func decodeMonitorProcesses(raw string) []any {
	var items []any
	if json.Unmarshal([]byte(raw), &items) != nil || items == nil {
		return []any{}
	}
	return items
}

func cleanHostMonitor() error {
	db := sharedDB()
	if db == nil {
		return nil
	}
	for _, table := range []string{"monitor_bases", "monitor_ios", "monitor_networks"} {
		if _, err := db.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("清空 %s 失败: %w", table, err)
		}
	}
	return nil
}

// sampleHostMonitor 写入一条基础、磁盘和网卡速率。没有上一拍计数时速率记 0，避免伪造流量。
func sampleHostMonitor(ctx context.Context) error {
	db := sharedDB()
	if db == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	usage, _, _ := dashboardCPUUsage()
	load1, load5, load15 := dashboardLoadTriple()
	logical := runtimeLogicalCores()
	memory := dashboardMemoryPercent()
	stamp := time.Now().UTC().Format(monitorTimeLayout)
	topCPU, _ := json.Marshal(dashboardTopProcesses(5, false))
	topMem, _ := json.Marshal(dashboardTopProcesses(5, true))
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO monitor_bases(created_at,cpu,memory,load_usage,cpu_load1,cpu_load5,cpu_load15,top_cpu,top_mem) VALUES(?,?,?,?,?,?,?,?,?)`, stamp, usage, memory, dashboardLoadUsage(load1, logical), load1, load5, load15, string(topCPU), string(topMem)); err != nil {
		return err
	}
	ios, nets := monitorRates()
	for name, item := range ios {
		if _, err := tx.ExecContext(ctx, `INSERT INTO monitor_ios(created_at,name,read_bytes,write_bytes,count,time_ms) VALUES(?,?,?,?,?,?)`, stamp, name, item.read, item.write, item.count, item.time); err != nil {
			return err
		}
	}
	for name, item := range nets {
		if _, err := tx.ExecContext(ctx, `INSERT INTO monitor_networks(created_at,name,up,down) VALUES(?,?,?,?)`, stamp, name, item.up, item.down); err != nil {
			return err
		}
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -loadHostMonitorSettings().storeDays()).Format(monitorTimeLayout)
	for _, statement := range []string{
		`DELETE FROM monitor_bases WHERE created_at < ?`,
		`DELETE FROM monitor_ios WHERE created_at < ?`,
		`DELETE FROM monitor_networks WHERE created_at < ?`,
	} {
		if _, err := tx.ExecContext(ctx, statement, cutoff); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func runtimeLogicalCores() int {
	_, logical := dashboardCPUCores()
	if logical < 1 {
		return 1
	}
	return logical
}

func dashboardLoadTriple() (float64, float64, float64) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0
	}
	load1, _ := strconv.ParseFloat(fields[0], 64)
	load5, _ := strconv.ParseFloat(fields[1], 64)
	load15, _ := strconv.ParseFloat(fields[2], 64)
	return load1, load5, load15
}

func dashboardMemoryPercent() float64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	var total, available uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			total = value
		case "MemAvailable:":
			available = value
		}
	}
	if total == 0 || total < available {
		return 0
	}
	return percent(total-available, total)
}

func dashboardTopProcesses(limit int, byMemory bool) []map[string]any {
	items := dashboardProcesses()
	sort.SliceStable(items, func(i, j int) bool {
		if byMemory {
			left, _ := dashboardUint64(items[i]["memory"])
			right, _ := dashboardUint64(items[j]["memory"])
			return left > right
		}
		return processPercent(items[i]) > processPercent(items[j])
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func processPercent(item map[string]any) float64 {
	switch value := item["percent"].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	default:
		return 0
	}
}
