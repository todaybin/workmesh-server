// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// dashboardQuickJump 是控制面首页的轻量默认快捷入口。
var dashboardQuickJump = []map[string]any{
	{"id": 1, "name": "Agent", "title": "aiTools.agents.agent", "detail": "0", "recommend": 1, "isShow": true, "router": "/ai/agents/agent"},
	{"id": 2, "name": "Website", "title": "menu.website", "detail": "0", "recommend": 10, "isShow": true, "router": "/websites"},
	{"id": 3, "name": "Database", "title": "menu.database", "detail": "0", "recommend": 30, "isShow": true, "router": "/databases"},
	{"id": 4, "name": "Cronjob", "title": "menu.cronjob", "detail": "0", "recommend": 50, "isShow": false, "router": "/cronjobs"},
	{"id": 5, "name": "AppInstalled", "title": "home.appInstalled", "detail": "0", "recommend": 70, "isShow": true, "router": "/apps/installed"},
	{"id": 6, "name": "File", "title": "home.quickDir", "detail": "/", "recommend": 90, "isShow": false, "router": "/hosts/files"},
}

func handleDashboardOS(w http.ResponseWriter, _ *http.Request) {
	disks := dashboardDisks()
	var diskSize uint64
	for _, disk := range disks {
		if total, ok := dashboardUint64(disk["total"]); ok {
			diskSize += total
		}
	}
	info := map[string]any{"os": runtime.GOOS, "platform": runtime.GOOS, "platformFamily": runtime.GOOS, "kernelArch": runtime.GOARCH, "kernelVersion": "", "diskSize": diskSize}
	if data, err := os.ReadFile("/proc/version"); err == nil {
		info["kernelVersion"] = strings.TrimSpace(string(data))
	}
	if release, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(release), "\n") {
			key, value, found := strings.Cut(line, "=")
			if found && key == "PRETTY_NAME" {
				info["prettyDistro"] = strings.Trim(strings.TrimSpace(value), "\"")
				break
			}
		}
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": info})
}

func handleDashboardBase(w http.ResponseWriter, r *http.Request) {
	ioOption, netOption := dashboardOptionsFromRequest(r)
	current := dashboardCurrent(r.Context(), ioOption, netOption)
	host, _ := os.Hostname()
	cpu := dashboardCPUInfo()
	distro := runtime.GOOS
	if value, ok := cpu["prettyDistro"].(string); ok && value != "" {
		distro = value
	}
	counts := dashboardQuickCounts()
	data := map[string]any{
		"hostname": host, "os": runtime.GOOS, "platform": runtime.GOOS, "platformFamily": runtime.GOOS,
		"platformVersion": "", "prettyDistro": distro, "kernelArch": runtime.GOARCH,
		"kernelVersion": "", "virtualizationSystem": "", "ipV4Addr": dashboardIPv4(), "httpProxy": "",
		"cpuCores": runtime.NumCPU(), "cpuLogicalCores": runtime.NumCPU(), "cpuModelName": cpu["model"], "cpuMhz": cpu["mhz"],
		"websiteNumber": counts["Website"], "agentNumber": counts["Agent"], "databaseNumber": counts["Database"],
		"cronjobNumber": counts["Cronjob"], "appInstalledNumber": counts["AppInstalled"],
		"currentInfo": current, "quickJump": dashboardQuickJumpsWithCounts(counts),
	}
	if dataRaw, err := os.ReadFile("/proc/version"); err == nil {
		data["kernelVersion"] = strings.TrimSpace(string(dataRaw))
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}

func handleDashboardCurrent(w http.ResponseWriter, r *http.Request) {
	ioOption, netOption := dashboardOptionsFromRequest(r)
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardCurrent(r.Context(), ioOption, netOption)})
}

func handleDashboardNode(w http.ResponseWriter, r *http.Request) { handleDashboardCurrent(w, r) }

func handleDashboardTopCPU(w http.ResponseWriter, _ *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardProcesses()})
}

func handleDashboardTopMem(w http.ResponseWriter, _ *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardProcesses()})
}

func handleDashboardQuickOption(w http.ResponseWriter, _ *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardQuickJumpsWithCounts(dashboardQuickCounts())})
}

func dashboardOptionsFromRequest(r *http.Request) (string, string) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i, part := range parts {
		if (part == "base" || part == "current") && i+2 < len(parts) {
			return parts[i+1], parts[i+2]
		}
	}
	return "all", "all"
}

func dashboardQuickCounts() map[string]int {
	counts := map[string]int{"Agent": 0, "Website": 0, "Database": 0, "Cronjob": 0, "AppInstalled": 0}
	if store := getAppStore(); store != nil {
		store.mu.RLock()
		for _, app := range store.state.Apps {
			if appStatusCountsAsInstalled(app.Status) {
				counts["AppInstalled"]++
			}
		}
		store.mu.RUnlock()
	}
	counts["Website"] = service.NewWebsiteService("").Count("")
	counts["Cronjob"] = len(sharedCronjobs.List(context.Background()))
	// Agent and database resources are represented by their persisted domain collections.
	store := getDomainStore()
	store.mu.RLock()
	if raw, ok := store.state.Settings["agents"].([]any); ok {
		counts["Agent"] = len(raw)
	}
	if raw, ok := store.state.Settings["databases"].([]any); ok {
		counts["Database"] = len(raw)
	}
	store.mu.RUnlock()
	return counts
}

func dashboardQuickJumpsWithCounts(counts map[string]int) []map[string]any {
	items := dashboardQuickJumps()
	for _, item := range items {
		name := fmt.Sprint(item["name"])
		if value, ok := counts[name]; ok {
			item["detail"] = strconv.Itoa(value)
		}
	}
	return items
}

func handleDashboardLauncher(w http.ResponseWriter, _ *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardAppLaunchers()})
}

// dashboardAppLaunchers 按应用目录聚合安装实例，保持首页应用卡片契约。
func dashboardAppLaunchers() []map[string]any {
	store := getAppStore()
	store.mu.RLock()
	defer store.mu.RUnlock()
	catalog := store.state.Catalog
	if len(catalog) == 0 {
		catalog = store.state.Apps
	}
	installedByKey := make(map[string][]map[string]any)
	for _, install := range store.state.Apps {
		if strings.TrimSpace(install.ID) == "" || strings.TrimSpace(install.Key) == "" || strings.EqualFold(install.Status, "failed") {
			continue
		}
		installedByKey[install.Key] = append(installedByKey[install.Key], map[string]any{
			"installID": install.ID,
			"detailID":  install.ID,
			"name":      install.Name,
			"version":   install.Version,
			"status":    normalizeAppStatus(install.Status),
			"path":      appInstallPath(install),
			"webUI":     appValue(install.Config, "webUI", "WebUI", "url"),
			"httpPort":  appConfiguredInt(install.Config, 0, "httpPort", "port", "PANEL_APP_PORT_HTTP"),
			"httpsPort": appConfiguredInt(install.Config, 0, "httpsPort", "PANEL_APP_PORT_HTTPS"),
		})
	}
	visibility := dashboardLauncherVisibility()
	result := make([]map[string]any, 0, len(catalog))
	seen := make(map[string]bool)
	for _, app := range catalog {
		key := strings.TrimSpace(app.Key)
		if key == "" || seen[key] {
			continue
		}
		details := installedByKey[key]
		if visible, ok := visibility[key]; ok && !visible {
			continue
		}
		if len(details) == 0 && app.Recommend <= 0 {
			continue
		}
		item := appRecordDataLocalized(app, "", store.state.CatalogTags)
		item["key"] = key
		item["type"] = app.Type
		item["appType"] = app.Type
		item["isInstall"] = len(details) > 0
		item["detail"] = details
		result = append(result, item)
		seen[key] = true
	}
	// 保留目录缺失但已安装的应用，便于目录同步异常时仍可管理实例。
	for key, details := range installedByKey {
		if seen[key] {
			continue
		}
		if visible, ok := visibility[key]; ok && !visible {
			continue
		}
		install := appRecord{Key: key, Name: key}
		for _, candidate := range store.state.Apps {
			if candidate.Key == key {
				install = candidate
				break
			}
		}
		item := appRecordDataLocalized(install, "", nil)
		item["appType"] = install.Type
		item["isInstall"] = true
		item["detail"] = details
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		installedI, _ := result[i]["isInstall"].(bool)
		installedJ, _ := result[j]["isInstall"].(bool)
		if installedI != installedJ {
			return installedI
		}
		return fmt.Sprint(result[i]["recommend"]) < fmt.Sprint(result[j]["recommend"])
	})
	return result
}

func handleDashboardLauncherOption(w http.ResponseWriter, r *http.Request) {
	request, err := requestMap(r)
	if err != nil {
		domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	filter := strings.ToLower(valueString(request, "filter", "name", "key"))
	visibility := dashboardLauncherVisibility()
	options := make([]map[string]any, 0)
	store := getAppStore()
	store.mu.RLock()
	keys := make(map[string]struct{})
	for _, item := range store.state.Catalog {
		if key := strings.TrimSpace(item.Key); key != "" {
			keys[key] = struct{}{}
		}
	}
	for _, item := range store.state.Apps {
		if key := strings.TrimSpace(item.Key); key != "" {
			keys[key] = struct{}{}
		}
	}
	store.mu.RUnlock()
	for key := range keys {
		if key == "" || (filter != "" && !strings.Contains(strings.ToLower(key), filter)) {
			continue
		}
		isShow := true
		if value, ok := visibility[key]; ok {
			isShow = value
		}
		options = append(options, map[string]any{"key": key, "isShow": isShow})
	}
	// Keep explicitly persisted entries visible in the settings response even when
	// their app/shortcut is currently unavailable. This preserves the user's
	// choice across install/uninstall cycles and matches the original panel.
	seen := make(map[string]bool, len(options))
	for _, item := range options {
		seen[fmt.Sprint(item["key"])] = true
	}
	for key, isShow := range visibility {
		if key == "" || seen[key] || (filter != "" && !strings.Contains(strings.ToLower(key), filter)) {
			continue
		}
		options = append(options, map[string]any{"key": key, "isShow": isShow})
	}
	sort.Slice(options, func(i, j int) bool { return fmt.Sprint(options[i]["key"]) < fmt.Sprint(options[j]["key"]) })
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": options})
}

func handleDashboardMutation(w http.ResponseWriter, r *http.Request) {
	request, err := requestMap(r)
	if err != nil {
		domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	store := getDomainStore()
	switch r.URL.Path {
	case "/api/v2/dashboard/quick/change":
		quicks, err := normalizeDashboardQuickJumps(request["quicks"])
		if err != nil {
			domainError(w, http.StatusBadRequest, "INVALID_QUICK_JUMPS", err.Error())
			return
		}
		shown := 0
		for _, item := range quicks {
			if visible, ok := item["isShow"].(bool); ok && visible {
				shown++
			}
		}
		if shown == 0 {
			domainError(w, http.StatusBadRequest, "MIN_QUICK_JUMP", "至少保留一个可见的快速入口")
			return
		}
		if shown > 4 {
			domainError(w, http.StatusBadRequest, "MAX_QUICK_JUMP", "可见的快速入口不能超过 4 个")
			return
		}
		store.mu.Lock()
		if store.state.Settings == nil {
			store.state.Settings = map[string]any{}
		}
		store.state.Settings["dashboardQuickJump"] = quicks
		err = store.saveLocked()
		store.mu.Unlock()
		if err != nil {
			domainError(w, http.StatusInternalServerError, "STATE_SAVE", fmt.Sprintf("保存快速入口失败: %v", err))
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": quicks})
		return
	case "/api/v2/dashboard/app/launcher/show":
		key := strings.TrimSpace(valueString(request, "key"))
		value := strings.TrimSpace(valueString(request, "value"))
		if key == "" || len(key) > 128 {
			domainError(w, http.StatusBadRequest, "INVALID_LAUNCHER_KEY", "启动器 key 不能为空且长度不能超过 128")
			return
		}
		var visible bool
		switch {
		case strings.EqualFold(value, "Enable"):
			visible = true
		case strings.EqualFold(value, "Disable"):
			visible = false
		default:
			domainError(w, http.StatusBadRequest, "INVALID_LAUNCHER_STATUS", "启动器状态必须为 Enable 或 Disable")
			return
		}
		store.mu.Lock()
		if store.state.Settings == nil {
			store.state.Settings = map[string]any{}
		}
		visibility := map[string]any{}
		if existing, ok := store.state.Settings["dashboardLauncherVisibility"].(map[string]any); ok {
			for itemKey, itemValue := range existing {
				visibility[itemKey] = itemValue
			}
		}
		visibility[key] = visible
		store.state.Settings["dashboardLauncherVisibility"] = visibility
		err = store.saveLocked()
		store.mu.Unlock()
		if err != nil {
			domainError(w, http.StatusInternalServerError, "STATE_SAVE", fmt.Sprintf("保存启动器配置失败: %v", err))
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"key": key, "isShow": visible}})
		return
	default:
		// 未知变更路径拒绝写入，避免误把任意 JSON 持久化到设置空间。
		domainError(w, http.StatusBadRequest, "UNSUPPORTED_DASHBOARD_MUTATION", "不支持的首页配置操作")
	}
}

// dashboardQuickJumps 从持久化设置读取快速入口；首次使用时返回稳定的默认副本。
func dashboardQuickJumps() []map[string]any {
	items := dashboardConfiguredQuickJumps()
	visibility := dashboardLauncherVisibility()
	filtered := make([]map[string]any, 0, len(items))
	for _, item := range items {
		key := strings.TrimSpace(fmt.Sprint(item["name"]))
		if key == "" {
			key = strings.TrimSpace(fmt.Sprint(item["key"]))
		}
		if visible, ok := item["isShow"].(bool); ok && !visible {
			continue
		}
		if visible, ok := visibility[key]; ok && !visible {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// dashboardConfiguredQuickJumps 返回已保存或默认的完整入口列表，供设置页展示隐藏项。
func dashboardConfiguredQuickJumps() []map[string]any {
	store := getDomainStore()
	store.mu.RLock()
	raw := store.state.Settings["dashboardQuickJump"]
	store.mu.RUnlock()
	items := normalizeStoredQuickJumps(raw)
	if len(items) == 0 {
		items = cloneDashboardQuickJumps(dashboardQuickJump)
	}
	return items
}

func dashboardLauncherVisibility() map[string]bool {
	store := getDomainStore()
	store.mu.RLock()
	defer store.mu.RUnlock()
	return dashboardVisibilityFromSettings(store.state.Settings)
}

func dashboardVisibilityFromSettings(settings map[string]any) map[string]bool {
	visibility := make(map[string]bool)
	if raw, ok := settings["dashboardLauncherVisibility"].(map[string]any); ok {
		for key, value := range raw {
			if visible, ok := value.(bool); ok {
				visibility[key] = visible
			}
		}
	}
	return visibility
}

func cloneDashboardQuickJumps(source []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(source))
	for _, item := range source {
		copyItem := make(map[string]any, len(item))
		for key, value := range item {
			copyItem[key] = value
		}
		result = append(result, copyItem)
	}
	return result
}

func normalizeStoredQuickJumps(raw any) []map[string]any {
	switch items := raw.(type) {
	case []map[string]any:
		return cloneDashboardQuickJumps(items)
	case []any:
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if value, ok := item.(map[string]any); ok {
				result = append(result, cloneDashboardQuickJumps([]map[string]any{value})[0])
			}
		}
		return result
	default:
		return nil
	}
}

func normalizeDashboardQuickJumps(raw any) ([]map[string]any, error) {
	items := normalizeStoredQuickJumps(raw)
	if len(items) == 0 {
		return nil, errors.New("quicks 必须是非空数组")
	}
	if len(items) > 100 {
		return nil, errors.New("quicks 数量不能超过 100")
	}
	for index, item := range items {
		name := strings.TrimSpace(fmt.Sprint(item["name"]))
		if name == "" {
			name = strings.TrimSpace(fmt.Sprint(item["key"]))
		}
		if name == "" || len(name) > 128 {
			return nil, fmt.Errorf("第 %d 个快速入口缺少有效 name", index+1)
		}
		item["name"] = name
		if _, ok := item["isShow"].(bool); !ok {
			item["isShow"] = true
		}
	}
	return items, nil
}

func handleDashboardRestart(w http.ResponseWriter, r *http.Request) {
	operation := strings.TrimSpace(r.PathValue("operation"))
	if operation == "" {
		// 兼容旧的冒号路径在未转换时仍可从 URL 末段取得操作名。
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) > 0 {
			operation = parts[len(parts)-1]
		}
	}
	if operation == "" {
		domainError(w, http.StatusBadRequest, "RESTART_OPERATION_REQUIRED", "重启操作不能为空")
		return
	}
	if runtime.GOOS == "windows" {
		domainError(w, http.StatusServiceUnavailable, "RESTART_UNSUPPORTED", "当前平台不支持服务重启")
		return
	}
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		domainError(w, http.StatusServiceUnavailable, "SYSTEMD_UNAVAILABLE", "systemd 不可用: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if operation == "system" {
		if os.Getenv("WORKMESH_ALLOW_SYSTEM_REBOOT") != "1" {
			domainError(w, http.StatusServiceUnavailable, "RESTART_NOT_AUTHORIZED", "系统重启未获显式授权")
			return
		}
		cmd := exec.CommandContext(ctx, systemctl, "reboot")
		if err := cmd.Start(); err != nil {
			domainError(w, http.StatusBadGateway, "RESTART_START_FAILED", "启动系统重启失败: "+err.Error())
			return
		}
		wmhttp.JSON(w, http.StatusAccepted, map[string]any{"code": 200, "data": map[string]any{"operation": operation, "accepted": true}})
		return
	}
	if os.Getenv("WORKMESH_ALLOW_RESTART") != "1" {
		domainError(w, http.StatusServiceUnavailable, "RESTART_NOT_AUTHORIZED", "服务重启未获显式授权")
		return
	}
	serviceName := strings.TrimSpace(os.Getenv("WORKMESH_SERVICE_NAME"))
	if operation == "1panel-agent" {
		serviceName = strings.TrimSpace(os.Getenv("WORKMESH_AGENT_SERVICE_NAME"))
		if serviceName == "" {
			serviceName = "workmesh-server-agent.service"
		}
	} else if operation == "1panel" || operation == "workmesh-server" {
		if serviceName == "" {
			serviceName = "workmesh-server.service"
		}
	} else {
		domainError(w, http.StatusBadRequest, "RESTART_OPERATION_INVALID", "不支持的重启操作: "+operation)
		return
	}
	if err := exec.CommandContext(ctx, systemctl, "is-active", "--quiet", serviceName).Run(); err != nil {
		domainError(w, http.StatusServiceUnavailable, "SERVICE_NOT_RUNNING", "服务未运行: "+serviceName)
		return
	}
	cmd := exec.CommandContext(ctx, systemctl, "restart", serviceName)
	if err := cmd.Start(); err != nil {
		domainError(w, http.StatusBadGateway, "RESTART_START_FAILED", "启动服务重启失败: "+err.Error())
		return
	}
	wmhttp.JSON(w, http.StatusAccepted, map[string]any{"code": 200, "data": map[string]any{"operation": operation, "service": serviceName, "accepted": true}})
}

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
	cpuUsage := numberOrZero(cpuInfo["usedPercent"])
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
		"uptime": uptime, "procs": runtime.NumGoroutine(), "load1": load1, "load5": load5, "load15": load15,
		// 前端契约要求 runningTime 为拆分后的时分秒对象，不能直接返回 uptime 整数。
		"timeSinceUptime": time.Now().Add(-time.Duration(uptime) * time.Second).UTC().Format(time.RFC3339),
		"runningTime": map[string]uint64{
			"days":    uptime / 86400,
			"hours":   (uptime % 86400) / 3600,
			"minutes": (uptime % 3600) / 60,
			"seconds": uptime % 60,
		},
		"loadUsagePercent": cpuUsage, "cpuPercent": cpuPercent, "cpuUsedPercent": cpuUsage,
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
		device, filesystem, mount := fields[0], fields[2], fields[1]
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
		total, free, available := dashboardDiskUsage(mount)
		used := uint64(0)
		if total > free {
			used = total - free
		}
		disks = append(disks, map[string]any{
			"path": mount, "mount": mount, "type": filesystem, "device": device, "filesystem": filesystem,
			"available": available, "usedPercent": percent(used, total), "free": free, "total": total, "used": used,
			"inodesTotal": uint64(0), "inodesUsed": uint64(0), "inodesFree": uint64(0), "inodesUsedPercent": float64(0),
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

// dashboardDiskUsage 由平台实现，用于读取挂载点容量；失败时返回明确的不可用状态。
func dashboardDiskUsage(path string) (total, free uint64, available bool) {
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
	result := map[string]any{"perCore": []float64{}, "detailed": []float64{0, 0, 0, 100, 0, 0, 0, 0}, "usedPercent": 0.0, "model": "", "mhz": 0.0, "prettyDistro": ""}
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return result
	}
	var aggregate [8]uint64
	var totalAll uint64
	perCore := make([]float64, 0, runtime.NumCPU())
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "cpu" && !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		if fields[0] != "cpu" && (len(fields[0]) == 3 || strings.Trim(fields[0][3:], "0123456789") != "") {
			continue
		}
		var values [8]uint64
		for i := 0; i < len(values) && i+1 < len(fields); i++ {
			values[i], _ = strconv.ParseUint(fields[i+1], 10, 64)
			// /proc/stat 首行 cpu 已经是所有核心的汇总；不能再把每个核心重复累加。
			if fields[0] == "cpu" {
				aggregate[i] = values[i]
				totalAll += values[i]
			}
		}
		if fields[0] != "cpu" {
			total := uint64(0)
			for _, value := range values {
				total += value
			}
			idle := values[3] + values[4]
			if total > 0 {
				perCore = append(perCore, percent(total-idle, total))
			}
		}
	}
	if totalAll > 0 {
		idle := aggregate[3] + aggregate[4]
		result["usedPercent"] = percent(totalAll-idle, totalAll)
		detailed := make([]float64, 8)
		for i, value := range aggregate {
			detailed[i] = percent(value, totalAll)
		}
		result["detailed"] = detailed
	}
	result["perCore"] = perCore
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
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || dashboardIsPartition(name) {
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

func dashboardIPv4() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err == nil && ip.To4() != nil {
				return ip.To4().String()
			}
		}
	}
	return ""
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
