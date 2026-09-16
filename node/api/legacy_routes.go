// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"strings"
)

// RegisterLegacyCompatibilityRoutes 为尚未拆分到领域文件的 v2 公开契约提供统一入口。
// 已有真实处理器的路由会被过滤；其余契约明确返回 501，禁止伪造成功响应。
// routeRegistrar 抽象 HandleFunc，便于过滤已经迁移的旧占位路由。
type routeRegistrar interface {
	HandleFunc(string, http.HandlerFunc)
}

type legacyFilterMux struct{ mux *http.ServeMux }

func (m legacyFilterMux) HandleFunc(pattern string, handler http.HandlerFunc) {
	if isExplicitCoreAuthRoute(pattern) || isDashboardRoute(pattern) {
		return
	}
	if isBackupAlertLogSettingsRoute(pattern) || isWebsiteFunctionalRoute(pattern) || isAnalyticsRoute(pattern) || isContainerRoute(pattern) || isHostRoute(pattern) || isAIExecutionRoute(pattern) || isCoreResourceRoute(pattern) || isCoreCommandRoute(pattern) || isFileRoute(pattern) || isDatabaseRoute(pattern) || isDeploymentProcessRoute(pattern) || isRuntimeToolboxRoute(pattern) || isAppRoute(pattern) || isSitesRoute(pattern) || isCronjobRoute(pattern) || isGroupRoute(pattern) {
		return
	}
	if isCoreAuthRoute(pattern) {
		m.mux.HandleFunc(normalizeServeMuxPattern(pattern), handler)
		return
	}
	if isLegacyConcreteRoute(pattern) {
		defer func() { _ = recover() }()
		m.mux.HandleFunc(normalizeServeMuxPattern(pattern), handler)
		return
	}
	defer func() { _ = recover() }()
	registerFallbackRoute(m.mux, normalizeServeMuxPattern(pattern))
}

// isCronjobRoute 和 isGroupRoute 防止已注册的计划任务、分组契约再次落入兼容占位处理器。
func isCronjobRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return path == "/api/v2/cronjobs" || strings.HasPrefix(path, "/api/v2/cronjobs/")
}

func isGroupRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return path == "/api/v2/groups" || strings.HasPrefix(path, "/api/v2/groups/")
}

// isExplicitCoreAuthRoute 判断已由核心认证处理器显式注册的路径，避免兼容层重复注册。
func isExplicitCoreAuthRoute(pattern string) bool {
	return pattern == "POST /api/v2/core/auth/login" || pattern == "POST /api/v2/core/auth/logout" || pattern == "GET /api/v2/core/auth/current"
}

// isDashboardRoute 判断仪表盘专用路径，统一交给真实指标处理器。
func isDashboardRoute(pattern string) bool {
	for _, route := range []string{
		"GET /api/v2/dashboard/app/launcher",
		"GET /api/v2/dashboard/base/os",
		"GET /api/v2/dashboard/base/:ioOption/:netOption",
		"GET /api/v2/dashboard/base/{ioOption}/{netOption}",
		"GET /api/v2/dashboard/current/node",
		"GET /api/v2/dashboard/current/top/cpu",
		"GET /api/v2/dashboard/current/top/mem",
		"GET /api/v2/dashboard/current/:ioOption/:netOption",
		"GET /api/v2/dashboard/current/{ioOption}/{netOption}",
		"GET /api/v2/dashboard/quick/option",
		"POST /api/v2/dashboard/app/launcher/option",
		"POST /api/v2/dashboard/app/launcher/show",
		"POST /api/v2/dashboard/quick/change",
		"POST /api/v2/dashboard/system/restart/:operation",
		"POST /api/v2/dashboard/system/restart/{operation}",
	} {
		if pattern == route {
			return true
		}
	}
	return false
}

// normalizeServeMuxPattern 将旧 Gin 风格 :id/*path 转换为 Go ServeMux 通配符。
func normalizeServeMuxPattern(pattern string) string {
	parts := strings.SplitN(pattern, " ", 2)
	if len(parts) != 2 {
		return pattern
	}
	segments := strings.Split(parts[1], "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ":") && len(segment) > 1 {
			segments[i] = "{" + strings.TrimPrefix(segment, ":") + "}"
		} else if strings.HasPrefix(segment, "*") && len(segment) > 1 {
			segments[i] = "{" + strings.TrimPrefix(segment, "*") + "...}"
		}
	}
	return parts[0] + " " + strings.Join(segments, "/")
}

// isLegacyConcreteRoute 列出仍由旧迁移处理器承接、已经具备真实行为的少量路由。
func isLegacyConcreteRoute(pattern string) bool {
	for _, route := range []string{
		"GET /api/v2/dashboard/app/launcher",
		"GET /api/v2/dashboard/base/:ioOption/:netOption",
		"GET /api/v2/dashboard/base/os",
		"GET /api/v2/dashboard/current/:ioOption/:netOption",
		"GET /api/v2/dashboard/current/node",
		"GET /api/v2/dashboard/current/top/cpu",
		"GET /api/v2/dashboard/current/top/mem",
		"GET /api/v2/dashboard/quick/option",
		"POST /api/v2/dashboard/app/launcher/option",
		"POST /api/v2/dashboard/app/launcher/show",
		"POST /api/v2/dashboard/quick/change",
		"POST /api/v2/core/auth/login",
		"POST /api/v2/core/auth/logout",
		"GET /api/v2/files/download",
		"GET /api/v2/files/tree",
		"POST /api/v2/files/upload",
	} {
		if pattern == route {
			return true
		}
	}
	return false
}

func isSitesRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return path == "/api/v2/sites" || strings.HasPrefix(path, "/api/v2/sites/")
}

// RegisterLegacyCompatibilityRoutes 为所有旧公开契约提供统一入口。
func RegisterLegacyCompatibilityRoutes(mux *http.ServeMux) {
	registerLegacyCompatibilityRoutes(legacyFilterMux{mux: mux})
}

func registerLegacyCompatibilityRoutes(mux routeRegistrar) {
	// 按 API 领域注册兼容路由，保持所有原始路径和处理器契约不变。
	registerLegacyMiscRoutes(mux)
	registerLegacyAiRoutes(mux)
	registerLegacyAlertRoutes(mux)
	registerLegacyAppsRoutes(mux)
	registerLegacyBackupsRoutes(mux)
	registerLegacyContainersRoutes(mux)
	registerLegacyCoreRoutes(mux)
	registerLegacyCronjobsRoutes(mux)
	registerLegacyDashboardRoutes(mux)
	registerLegacyDatabasesRoutes(mux)
	registerLegacyDeploymentRoutes(mux)
	registerLegacyFilesRoutes(mux)
	registerLegacyHostsRoutes(mux)
	registerLegacyOpenrestyRoutes(mux)
	registerLegacyRuntimesRoutes(mux)
	registerLegacySettingsRoutes(mux)
	registerLegacyToolboxRoutes(mux)
	registerLegacyWebsitesRoutes(mux)
	registerLegacyGroupsRoutes(mux)
}
