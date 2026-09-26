// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
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
	release := dashboardOSRelease()
	info := map[string]any{"os": runtime.GOOS, "platform": release.ID, "platformFamily": release.Family, "kernelArch": dashboardKernelArch(), "kernelVersion": dashboardKernelVersion(), "diskSize": diskSize, "prettyDistro": release.Pretty}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": info})
}

// handleDashboardBase 返回首页所需的主机、资源和快捷入口概览。
func handleDashboardBase(w http.ResponseWriter, r *http.Request) {
	ioOption, netOption := dashboardOptionsFromRequest(r)
	current := dashboardCurrent(r.Context(), ioOption, netOption)
	host, _ := os.Hostname()
	cpu := dashboardCPUInfo()
	release := dashboardOSRelease()
	physical, logical := dashboardCPUCores()
	counts := dashboardQuickCounts()
	data := map[string]any{
		"hostname": host, "os": runtime.GOOS, "platform": release.ID, "platformFamily": release.Family,
		"platformVersion": release.Version, "prettyDistro": release.Pretty, "kernelArch": dashboardKernelArch(),
		"kernelVersion": dashboardKernelVersion(), "virtualizationSystem": "", "ipV4Addr": dashboardIPv4(), "httpProxy": "",
		"cpuCores": physical, "cpuLogicalCores": logical, "cpuModelName": cpu["model"], "cpuMhz": cpu["mhz"],
		"websiteNumber": counts["Website"], "agentNumber": counts["Agent"], "databaseNumber": counts["Database"],
		"cronjobNumber": counts["Cronjob"], "appInstalledNumber": counts["AppInstalled"],
		"currentInfo": current, "quickJump": dashboardQuickJumpsWithCounts(counts),
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}

// handleDashboardCurrent 返回当前 CPU、内存、磁盘和网络采集结果。
func handleDashboardCurrent(w http.ResponseWriter, r *http.Request) {
	ioOption, netOption := dashboardOptionsFromRequest(r)
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardCurrent(r.Context(), ioOption, netOption)})
}

// handleDashboardNode 兼容节点概览路径并复用当前资源采集逻辑。
func handleDashboardNode(w http.ResponseWriter, r *http.Request) { handleDashboardCurrent(w, r) }

// handleDashboardTopCPU 返回按 CPU 使用率排序的进程列表。
func handleDashboardTopCPU(w http.ResponseWriter, _ *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardProcesses()})
}

// handleDashboardTopMem 返回按内存使用率排序的进程列表。
func handleDashboardTopMem(w http.ResponseWriter, _ *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardProcesses()})
}

// handleDashboardQuickOption 返回首页快捷入口配置和实时数量。
func handleDashboardQuickOption(w http.ResponseWriter, _ *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": dashboardQuickJumpsWithCounts(dashboardQuickCounts())})
}

// dashboardOptionsFromRequest 解析首页采集接口中的磁盘和网络选项。
func dashboardOptionsFromRequest(r *http.Request) (string, string) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i, part := range parts {
		if (part == "base" || part == "current") && i+2 < len(parts) {
			return parts[i+1], parts[i+2]
		}
	}
	return "all", "all"
}

// dashboardQuickCounts 从真实持久化集合计算首页入口数量。
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

// dashboardQuickJumpsWithCounts 将实时数量合并到快捷入口副本。
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

// handleDashboardMutation 保存首页快捷入口和启动器显示配置。
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

// dashboardVisibilityFromSettings 将持久化设置转换为启动器可见性映射。
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

// cloneDashboardQuickJumps 深复制快捷入口，避免修改全局默认配置。
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

// normalizeStoredQuickJumps 将 SQLite 设置中的多种数组表示归一化。
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

// normalizeDashboardQuickJumps 校验并规范首页快捷入口提交数据。
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

// handleDashboardRestart 按显式环境授权请求服务或系统重启。
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
