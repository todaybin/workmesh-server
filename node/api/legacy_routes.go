// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
	"strings"
)

const migrationPendingMessage = "该接口正在迁移"

// RegisterLegacyCompatibilityRoutes 为所有旧公开契约提供统一入口。
// 业务域迁移完成前返回结构化错误，避免前端出现 404；已迁移路由由专用处理器注册。
// routeRegistrar 抽象 HandleFunc，便于过滤已经迁移的旧占位路由。
type routeRegistrar interface {
	HandleFunc(string, http.HandlerFunc)
}

type legacyFilterMux struct{ mux *http.ServeMux }

func (m legacyFilterMux) HandleFunc(pattern string, handler http.HandlerFunc) {
	if isBackupAlertLogSettingsRoute(pattern) || isWebsiteFunctionalRoute(pattern) || isContainerRoute(pattern) || isHostRoute(pattern) || isAIExecutionRoute(pattern) || isCoreResourceRoute(pattern) || isFileRoute(pattern) || isDatabaseRoute(pattern) || isDeploymentProcessRoute(pattern) || isRuntimeToolboxRoute(pattern) {
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
	registerCompatibilityRoute(m.mux, normalizeServeMuxPattern(pattern))
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
		"GET /api/v2/dashboard/base/os",
		"GET /api/v2/dashboard/current/node",
		"GET /api/v2/dashboard/current/top/cpu",
		"GET /api/v2/dashboard/current/top/mem",
		"GET /api/v2/dashboard/quick/option",
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

// RegisterLegacyCompatibilityRoutes 为所有旧公开契约提供统一入口。
func RegisterLegacyCompatibilityRoutes(mux *http.ServeMux) {
	registerLegacyCompatibilityRoutes(legacyFilterMux{mux: mux})
}

func registerLegacyCompatibilityRoutes(mux routeRegistrar) {
	// 兼容契约路径：PHP 扩展与配置动态段由下方统一子路径处理器承接。
	// HandleFunc("GET /api/v2/runtimes/php/:id/extensions", ...)
	// HandleFunc("GET /api/v2/runtimes/php/config/:id", ...)
	mux.HandleFunc("GET /api/v2/access-lists", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/ai/accounts/providers", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/ai/gpu/load", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/ai/gpu/options", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/ai/mcp/domain/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/alert/clams/list", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/alert/disks/list", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/apps/:key", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/apps/checkupdate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/apps/detail/:appId/:version/:type", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/apps/detail/node/:appKey/:version", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/apps/details/:id", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/apps/icon/:key", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/apps/ignored/detail", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/apps/installed/delete/check/:appInstallId", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/apps/installed/info/:appInstallId", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/apps/installed/list", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/apps/installed/params/:appInstallId", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/apps/services/:key", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/apps/tags", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/backups/check/:name", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/backups/local", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/backups/options", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/config/global", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/containers/daemonjson", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/containers/daemonjson/file", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/containers/image", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/containers/image/all", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/containers/limit", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/containers/list/stats", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/containers/network", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/containers/repo", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/containers/search/log", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/containers/stats/:id", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/containers/status", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/containers/template", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/containers/volume", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/core/auth/captcha", func(w http.ResponseWriter, r *http.Request) {
		handleCoreCaptcha(w, r)
	})
	mux.HandleFunc("GET /api/v2/core/auth/current", func(w http.ResponseWriter, r *http.Request) {
		handleCoreCurrent(w, r)
	})
	mux.HandleFunc("GET /api/v2/core/auth/setting", func(w http.ResponseWriter, r *http.Request) {
		handleCoreAuthSetting(w, r)
	})
	mux.HandleFunc("GET /api/v2/core/auth/welcome", func(w http.ResponseWriter, r *http.Request) {
		handleCoreWelcome(w, r)
	})
	mux.HandleFunc("GET /api/v2/core/backups/client/:clientType", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/core/script/run", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/core/settings/apps/store/config", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/core/settings/interface", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/core/settings/memo", func(w http.ResponseWriter, r *http.Request) {
		handleCoreSettings(w, r)
	})
	mux.HandleFunc("GET /api/v2/core/settings/search/available", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/core/settings/ssl/info", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/core/settings/upgrade", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/core/settings/upgrade/releases", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/cronjobs/script/options", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/cubesandbox/health", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/cubesandbox/status", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/dashboard/app/launcher", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardLauncher(w, r)
	})
	mux.HandleFunc("GET /api/v2/dashboard/base/:ioOption/:netOption", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/dashboard/base/os", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardOS(w, r)
	})
	mux.HandleFunc("GET /api/v2/dashboard/current/:ioOption/:netOption", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/dashboard/current/node", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardNode(w, r)
	})
	mux.HandleFunc("GET /api/v2/dashboard/current/top/cpu", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardTopCPU(w, r)
	})
	mux.HandleFunc("GET /api/v2/dashboard/current/top/mem", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardTopMem(w, r)
	})
	mux.HandleFunc("GET /api/v2/dashboard/quick/option", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardQuickOption(w, r)
	})
	mux.HandleFunc("GET /api/v2/databases/db/:name", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/databases/db/item/:type", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/databases/db/list/:type", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/databases/redis/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/deployment/status", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/files/download", func(w http.ResponseWriter, r *http.Request) {
		handleFilesDownload(w, r)
	})
	mux.HandleFunc("GET /api/v2/files/recycle/status", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/files/share/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/files/share/download", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/files/share/info", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/files/share/qrcode", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/files/wget/process", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/files/wget/process/keys", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/hosts/diagnostics/goroutines", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/hosts/diagnostics/summary", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/hosts/disks", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/hosts/firewall/docker/endpoints", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/hosts/firewall/docker/ports", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/hosts/firewall/rules/sync/task", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/hosts/firewall/settings", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/hosts/monitor/iooptions", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/hosts/monitor/netoptions", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/hosts/monitor/setting", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/hosts/terminal/container", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/hosts/terminal/local", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/hosts/terminal/ssh", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/hosts/tool/supervisor/process", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/images/*filename", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/logs/system/files", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/logs/system/services", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/logs/system/status", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/logs/tasks/executing/count", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/openresty/https", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/openresty/modules", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/openresty/status", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/process/ws", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/runtimes/:id", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/runtimes/installed/delete/check/:id", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	// PHP 子路径统一进入兼容处理器，避免 ServeMux 动态段与静态段产生歧义。
	mux.HandleFunc("GET /api/v2/runtimes/php/{path...}", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/runtimes/php/container/:id", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/runtimes/php/fpm/config/:id", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/runtimes/php/fpm/status/:id", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/runtimes/supervisor/process/:id", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/settings/basedir", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/settings/search/available", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/settings/snapshot/load", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/settings/ssh/conn", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/settings/website/dir", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/sites", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/sites/:id/rules", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/standard-rules", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/static/*filename", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/status", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/toolbox/device/users", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/toolbox/device/zone/options", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/toolbox/fail2ban/base", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/toolbox/fail2ban/load/conf", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/toolbox/ftp/base", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/:id", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/:id/config/:type", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/:id/https", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/:id/lbs", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/ca/:id", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/cors/:id", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/databases", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/default/html/:type", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/domains/:websiteId", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/list", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/monitor/config/global", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/proxy/config/:id", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/realip/config/:id", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/resource/:id", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/rewrite/custom", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/waf/access-lists", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/waf/sites", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/waf/sites/:id/rules", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/waf/standard-rules", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /api/v2/websites/waf/status", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("GET /assets/*filepath", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/access-lists", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/accounts", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/accounts/counts", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/accounts/delete", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/accounts/models", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/accounts/models/create", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/accounts/models/delete", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/accounts/models/discover", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/accounts/models/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/accounts/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/accounts/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/accounts/verify", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/agent/bind", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/agent/channels", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/agent/create", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/agent/delete", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/agent/list", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/agent/md/list", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/agent/md/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/agent/unbind", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/batch/install", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/batch/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/batch/skill/install", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/batch/upgrade", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/delete", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/dingtalk/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/dingtalk/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/discord/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/discord/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/feishu/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/feishu/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/pairing/approve", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/qqbot/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/qqbot/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/telegram/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/telegram/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/wecom/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/wecom/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/weixin/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/channel/weixin/login", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/config-file/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/config-file/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/delete", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/delete/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/hermes/chat/sessions", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/hermes/chat/sessions/delete", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/hermes/chat/sessions/rename", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/model/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/model/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/other/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/other/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/overview", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/plugin/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/plugin/install", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/plugin/uninstall", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/plugin/upgrade", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/plugins/install", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/plugins/list", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/plugins/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/plugins/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/remark", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/security/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/security/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/skills/install", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/skills/list", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/skills/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/skills/uninstall", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/skills/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/token/reset", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/website/bind", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/agents/website/unbind", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/domain/bind", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/domain/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/domain/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/gpu/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/mcp/domain/bind", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/mcp/domain/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/mcp/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/mcp/server", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/mcp/server/connection/test", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/mcp/server/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/mcp/server/detail", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/mcp/server/op", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/mcp/server/status/sync", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/mcp/server/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/ollama/close", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/ollama/model", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/ollama/model/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/ollama/model/load", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/ollama/model/recreate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/ollama/model/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/ollama/model/sync", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/tensorrt/create", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/tensorrt/delete", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/tensorrt/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/tensorrt/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/ai/tensorrt/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/alert/config/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/alert/config/info", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/alert/config/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/alert/config/test", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/alert/config/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/alert/cronjob/list", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/alert/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/alert/logs/clean", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/alert/logs/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/alert/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/alert/status", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/alert/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/ignored/cancel", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/install", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/installed/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/installed/conf", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/installed/config/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/installed/conninfo", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/installed/ignore", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/installed/loadport", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/installed/op", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/installed/params/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/installed/port/change", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/installed/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/installed/sort/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/installed/sync", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/installed/update/versions", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/sync/local", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/apps/sync/remote", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/attack/stat", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/backup", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/buckets", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/conn/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/record/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/record/description/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/record/download", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/record/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/record/search/bycronjob", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/record/size", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/recover", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/recover/byupload", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/refresh/token", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/search/files", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/backups/upload", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/block/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/config/global", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/config/site", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/config/site/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/clean/log", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/commit", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/compose", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/compose/clean/log", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/compose/env", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/compose/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/compose/pin", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/compose/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/compose/test", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/compose/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/daemonjson/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/daemonjson/update/byfile", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/docker/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/download/log", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/files/content", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/files/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/files/download", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/files/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/files/size", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/files/upload", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/image/build", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/image/load", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/image/pull", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/image/push", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/image/remove", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/image/save", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/image/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/image/tag", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/info", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/inspect", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/ipv6option/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/item/stats", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/list", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/list/byimage", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/logoption/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/network", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/network/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/network/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/prune", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/rename", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/repo", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/repo/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/repo/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/repo/status", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/repo/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/template", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/template/batch", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/template/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/template/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/template/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/upgrade", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/users", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/volume", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/volume/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/containers/volume/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/auth/login", func(w http.ResponseWriter, r *http.Request) {
		handleCoreLogin(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		handleCoreLogout(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/backups/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/backups/refresh/token", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/backups/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/commands/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/commands/export", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/commands/import", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/commands/list", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/commands/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/commands/tree", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/commands/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/commands/upload", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/groups/del", func(w http.ResponseWriter, r *http.Request) {
		handleCoreGroups(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/groups/search", func(w http.ResponseWriter, r *http.Request) {
		handleCoreGroups(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/groups/update", func(w http.ResponseWriter, r *http.Request) {
		handleCoreGroups(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/logs/clean", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/logs/login", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/logs/operation", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/script/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/script/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/script/sync", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/script/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/settings/apps/store/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/settings/bind/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/settings/memo", func(w http.ResponseWriter, r *http.Request) {
		handleCoreSettings(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/settings/menu/default", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/settings/menu/update", func(w http.ResponseWriter, r *http.Request) {
		handleCoreSettings(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/settings/port/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/settings/proxy/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/settings/search", func(w http.ResponseWriter, r *http.Request) {
		handleCoreSettings(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/settings/search/base", func(w http.ResponseWriter, r *http.Request) {
		handleCoreSettings(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/settings/ssl/download", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/settings/ssl/reload", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/settings/ssl/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/settings/terminal/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/settings/terminal/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/settings/update", func(w http.ResponseWriter, r *http.Request) {
		handleCoreSettings(w, r)
	})
	mux.HandleFunc("POST /api/v2/core/settings/upgrade", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/core/settings/upgrade/notes", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/export", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/group/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/import", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/load/info", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/next", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/records/clean", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/records/log", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/search/records", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/status", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/stop", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/cubesandbox/reconcile", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/cubesandbox/start", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/cubesandbox/stop", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/dashboard/app/launcher/option", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardLauncherOption(w, r)
	})
	mux.HandleFunc("POST /api/v2/dashboard/app/launcher/show", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardMutation(w, r)
	})
	mux.HandleFunc("POST /api/v2/dashboard/quick/change", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardMutation(w, r)
	})
	mux.HandleFunc("POST /api/v2/dashboard/system/restart/:operation", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardRestart(w, r)
	})
	mux.HandleFunc("POST /api/v2/databases/change/access", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/change/password", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/common/info", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/common/load/file", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/common/update/conf", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/db", func(w http.ResponseWriter, r *http.Request) {
		handleDatabaseCreate(w, r)
	})
	mux.HandleFunc("POST /api/v2/databases/db/check", func(w http.ResponseWriter, r *http.Request) {
		handleDatabaseCheck(w, r)
	})
	mux.HandleFunc("POST /api/v2/databases/db/del", func(w http.ResponseWriter, r *http.Request) {
		handleDatabaseDelete(w, r)
	})
	mux.HandleFunc("POST /api/v2/databases/db/del/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/db/search", func(w http.ResponseWriter, r *http.Request) {
		handleDatabaseSearch(w, r)
	})
	mux.HandleFunc("POST /api/v2/databases/db/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/del/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/description/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/format/options", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/grants", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/grants/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/grants/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/grants/summary", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/load", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/mongodb", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/mongodb/bind", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/mongodb/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/mongodb/del/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/mongodb/description", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/mongodb/load", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/mongodb/password", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/mongodb/privileges", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/mongodb/privileges/change", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/mongodb/root/password", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/mongodb/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/pg", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/pg/:database/load", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/pg/bind", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/pg/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/pg/del/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/pg/description", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/pg/password", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/pg/privileges", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/pg/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/redis/conf", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/redis/conf/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/redis/install/cli", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/redis/password", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/redis/persistence/conf", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/redis/persistence/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/redis/status", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/remote", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/status", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/users", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/users/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/users/password", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/users/password/save", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/users/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/users/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/variables", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/databases/variables/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/deployment-artifact/activate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/deployment-manifest/verify", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/deployment/execute", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/deployment/rollback", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/ai-search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/batch/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/batch/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/batch/role", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/chunkdownload", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/chunkupload", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/chunkupload/stop", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/compress", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/compress/stop", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/content", func(w http.ResponseWriter, r *http.Request) {
		handleFilesContent(w, r)
	})
	mux.HandleFunc("POST /api/v2/files/convert", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/convert/log", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/decompress", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/decompress/stop", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/del", func(w http.ResponseWriter, r *http.Request) {
		handleFilesDelete(w, r)
	})
	mux.HandleFunc("POST /api/v2/files/depth/size", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/favorite", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/favorite/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/favorite/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/history/content", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/history/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/history/restore", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/history/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/mode", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/mount", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/move", func(w http.ResponseWriter, r *http.Request) {
		handleFilesMove(w, r)
	})
	mux.HandleFunc("POST /api/v2/files/move/stop", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/owner", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/preview", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/read/:type", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/recycle/clear", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/recycle/reduce", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/recycle/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/remark", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/remarks", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/rename", func(w http.ResponseWriter, r *http.Request) {
		handleFilesRename(w, r)
	})
	mux.HandleFunc("POST /api/v2/files/save", func(w http.ResponseWriter, r *http.Request) {
		handleFilesSave(w, r)
	})
	mux.HandleFunc("POST /api/v2/files/search", func(w http.ResponseWriter, r *http.Request) {
		handleFilesSearch(w, r)
	})
	mux.HandleFunc("POST /api/v2/files/share/create", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/share/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/share/detail", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/share/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/size", func(w http.ResponseWriter, r *http.Request) {
		handleFilesSize(w, r)
	})
	mux.HandleFunc("POST /api/v2/files/tree", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/upload", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/upload/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/user/group", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/wget", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/files/wget/stop", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/global", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/groups/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/groups/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/groups/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/diagnostics/profiles", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/disks/mount", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/disks/partition", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/disks/unmount", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/base", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/docker/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/docker/policies/batch", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/docker/policies/delete/batch", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/docker/sync", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/filter/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/forward/base", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/forward/enable", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/forward/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/forward/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/delete", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/native/detail", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/reorder", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/reset", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/sync", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/sync/preview", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/settings/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/info", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/monitor/clean", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/monitor/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/monitor/setting/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/ssh/cert", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/ssh/cert/delete", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/ssh/cert/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/ssh/cert/sync", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/ssh/cert/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/ssh/file", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/ssh/file/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/ssh/log", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/ssh/log/clean", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/ssh/log/export", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/ssh/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/ssh/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/ssh/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/test/byid", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/test/byinfo", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/tool/config/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/tool/config/set", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/tool/init", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/tool/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/tool/status", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/tool/supervisor/process", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/tool/supervisor/process/file", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/tool/supervisor/process/file/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/tree", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/hosts/update/group", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/log/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/logs/clear", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/logs/detail", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/logs/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/logs/stat", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/logs/system/read", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/logs/tasks/read", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/logs/tasks/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/openresty/build", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/openresty/file", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/openresty/https", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/openresty/modules/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/openresty/scope", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/openresty/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/qps", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/rank", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/relation/stat", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/rules", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/rules/delete", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/node/modules", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/node/modules/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/node/package", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/config", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/container/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/install", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/uninstall", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/file", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/fpm/config", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/remark", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/supervisor/process", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/supervisor/process/file", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/sync", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/runtimes/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/description/save", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/file-history/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/file-history/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/files/ai/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/files/ai/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/snapshot", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/snapshot/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/snapshot/description/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/snapshot/import", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/snapshot/recover", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/snapshot/recreate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/snapshot/rollback", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/snapshot/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/ssh", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/ssh/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/ssh/check/info", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/ssh/default", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/terminal/ai/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/terminal/ai/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/settings/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/sites", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/stat", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/test", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/clam", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/clam/base", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/clam/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/clam/file/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/clam/file/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/clam/handle", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/clam/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/clam/record/clean", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/clam/record/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/clam/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/clam/status/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/clam/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/clean", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/base", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/check/dns", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/conf", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/update/byconf", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/update/conf", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/update/host", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/update/passwd", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/update/swap", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/fail2ban/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/fail2ban/operate/sshd", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/fail2ban/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/fail2ban/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/fail2ban/update/byconf", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/ftp", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/ftp/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/ftp/log/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/ftp/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/ftp/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/ftp/sync", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/ftp/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/toolbox/scan", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/trend", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/visitors", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/visitors/loc", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/:id/https", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/acme/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/acme/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/acme/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/auths", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/auths/path", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/auths/path/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/auths/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/batch/group", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/batch/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/batch/ssl", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/ca/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/ca/download", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/ca/obtain", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/ca/renew", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/ca/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/config", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/config/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/cors/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/crosssite", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/databases", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/default/html/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/default/server", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/dir", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/dir/permission", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/dir/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/dns/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/dns/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/dns/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/domains", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/domains/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/domains/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/exec/composer", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/group/change", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/lbs/create", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/lbs/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/lbs/file", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/lbs/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/leech", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/leech/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/log/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/log/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/monitor/config/global", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/monitor/config/site", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/monitor/config/site/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/monitor/logs/clear", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/monitor/logs/detail", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/monitor/logs/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/monitor/logs/stat", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/monitor/qps", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/monitor/rank", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/monitor/stat", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/monitor/trend", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/monitor/visitors", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/monitor/visitors/loc", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/monitor/websites", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/nginx/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/operate", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/options", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/php/version", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/proxies", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/proxies/delete", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/proxies/file", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/proxies/status", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/proxies/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/proxy/clear", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/proxy/config", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/realip/config", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/redirect", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/redirect/file", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/redirect/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/rewrite", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/rewrite/custom", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/rewrite/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/stream/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/templates/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/templates/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/templates/outputs", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/templates/outputs/del", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/templates/outputs/get", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/templates/outputs/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/templates/preview", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/templates/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/templates/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/templates/upload", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/update", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/access-lists", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/attack/stat", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/block/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/global", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/log/search", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/relation/stat", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/rules", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/rules/delete", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/sites", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/test", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/workmesh/tasks/cancel", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/workmesh/tasks/collect", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/workmesh/tasks/create", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/workmesh/tasks/destroy", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/workmesh/tasks/exec", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
	mux.HandleFunc("POST /api/v2/workmesh/tasks/start", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": migrationPendingMessage})
	})
}
