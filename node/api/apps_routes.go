// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	workmeshi18n "github.com/todaybin/workmesh-server/i18n"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// RegisterAppRoutes 注册应用目录、安装实例和扩展应用接口。
func RegisterAppRoutes(mux *http.ServeMux) {
	handlers := appRouteHandlers{store: getAppStore()}
	registerAppCatalogRoutes(mux, handlers)
	registerInstalledAppRoutes(mux, handlers)
	registerExtendedAppRoutes(mux, handlers)
}

// registerAppCatalogRoutes 注册应用目录查询、同步和元数据读取接口。
func registerAppCatalogRoutes(mux *http.ServeMux, handlers appRouteHandlers) {
	for _, path := range []string{"/api/v2/apps/installed/list", "/api/v2/apps/installed/search"} {
		mux.HandleFunc("GET "+path, handlers.listInstalled)
		mux.HandleFunc("POST "+path, handlers.listInstalled)
	}
	for _, path := range []string{"/api/v2/apps/search", "/api/v2/apps/sync/local", "/api/v2/apps/sync/remote"} {
		mux.HandleFunc("POST "+path, handlers.searchCatalog)
	}
	mux.HandleFunc("GET /api/v2/apps/checkupdate", handlers.checkUpdate)
	mux.HandleFunc("GET /api/v2/apps/tags", handlers.listTags)
	mux.HandleFunc("GET /api/v2/apps/ignored/detail", handlers.ignoredDetail)
	mux.HandleFunc("GET /api/v2/apps/{key}", handlers.catalogByKey)
	mux.HandleFunc("GET /api/v2/apps/detail/{appId}/{version}/{type}", func(w http.ResponseWriter, r *http.Request) {
		handlers.catalogDetail(w, r, "appId")
	})
	mux.HandleFunc("GET /api/v2/apps/detail/node/{appKey}/{version}", func(w http.ResponseWriter, r *http.Request) {
		handlers.catalogDetail(w, r, "appKey")
	})
	mux.HandleFunc("GET /api/v2/apps/details/{id}", handlers.catalogByID)
	mux.HandleFunc("GET /api/v2/apps/services/{key}", handlers.services)
	mux.HandleFunc("GET /api/v2/apps/icon/{key}", handlers.icon)
}

// registerInstalledAppRoutes 注册已安装应用的详情和生命周期接口。
func registerInstalledAppRoutes(mux *http.ServeMux, handlers appRouteHandlers) {
	mux.HandleFunc("GET /api/v2/apps/installed/info/{appInstallId}", handlers.installedInfo)
	mux.HandleFunc("GET /api/v2/apps/installed/params/{appInstallId}", handlers.installedInfo)
	mux.HandleFunc("GET /api/v2/apps/installed/delete/check/{appInstallId}", handlers.deleteCheck)
	for _, path := range []string{"/api/v2/apps/install", "/api/v2/apps/installed/check", "/api/v2/apps/installed/conf", "/api/v2/apps/installed/config/update", "/api/v2/apps/installed/conninfo", "/api/v2/apps/installed/ignore", "/api/v2/apps/installed/loadport", "/api/v2/apps/installed/op", "/api/v2/apps/installed/params/update", "/api/v2/apps/installed/port/change", "/api/v2/apps/installed/sort/update", "/api/v2/apps/installed/sync", "/api/v2/apps/installed/update/versions", "/api/v2/apps/ignored/cancel"} {
		mux.HandleFunc("POST "+path, handlers.post)
	}
}

// registerExtendedAppRoutes 注册自定义应用商店和跨节点安装扩展接口。
func registerExtendedAppRoutes(mux *http.ServeMux, handlers appRouteHandlers) {
	mux.HandleFunc("POST /api/v2/custom/app/sync", handlers.customSync)
	mux.HandleFunc("GET /api/v2/custom/app/config", handlers.customConfig)
	mux.HandleFunc("POST /api/v2/core/xpack/sync/app/install", handlers.crossNodeInstall)
}

// appCatalogGet 返回指定应用和版本的目录详情。
func appCatalogGet(w http.ResponseWriter, s *appStore, r *http.Request, id, version string) {
	s.mu.Lock()
	if len(s.state.Catalog) == 0 && strings.TrimSpace(os.Getenv("WORKMESH_APP_CATALOG")) == "" {
		_ = s.refreshRemoteLocked(false)
	}
	s.mu.Unlock()
	s.mu.RLock()
	index, item := findApp(s.state.Catalog, id)
	if index < 0 {
		index, item = findApp(s.state.Apps, id)
	}
	catalog := append([]appRecord(nil), s.state.Catalog...)
	metadata := append([]appTagRecord(nil), s.state.CatalogTags...)
	s.mu.RUnlock()
	if index < 0 {
		appOK(w, map[string]any{"id": id, "key": id, "available": false})
		return
	}
	data := appRecordDataLocalized(item, workmeshi18n.LocaleFromRequest(r), metadata)
	data["available"] = true
	versions := make([]string, 0, len(item.Versions))
	for _, version := range item.Versions {
		if strings.TrimSpace(version.Version) != "" {
			versions = append(versions, version.Version)
		}
	}
	data["versions"] = versions
	data["readMe"] = item.ReadMe
	params := any(map[string]any{})
	selected := appVersionRecord{}
	for _, candidate := range item.Versions {
		if version == "" || candidate.Version == version {
			selected = candidate
			break
		}
	}
	if selected.Version == "" && len(item.Versions) > 0 {
		selected = item.Versions[0]
	}
	if version != "" && selected.ID != "" {
		data["appId"] = data["id"]
		data["id"] = selected.ID
		if numericID, err := strconv.ParseInt(selected.ID, 10, 64); err == nil {
			data["id"] = numericID
		}
	}
	if selected.Version != "" {
		params = selected.Params
		if normalizeRuntimeTypeFilter(item.Type) == "php" {
			params = phpRuntimeCatalogParams(catalog, selected)
		}
	} else if item.Config != nil {
		if configured, ok := item.Config["params"]; ok {
			params = configured
		}
	}
	compose := selected.DockerCompose
	if compose == "" && item.Config != nil {
		compose = appValue(item.Config, "dockerCompose", "compose")
	}
	if compose == "" && selected.ComposeURL != "" {
		compose = fetchRemoteCompose(selected.ComposeURL)
	}
	data["image"] = appRuntimeImage(item, compose)
	// 原版安装表单直接读取详情顶层字段；details 保留为兼容扩展字段。
	data["params"] = params
	data["dockerCompose"] = compose
	data["memoryRequired"] = item.MemoryRequired
	data["gpuSupport"] = item.GpuSupport
	data["architectures"] = item.Architectures
	data["hostMode"] = false
	data["details"] = map[string]any{"id": selected.ID, "version": selected.Version, "type": item.Type, "params": params, "dockerCompose": compose, "downloadUrl": selected.DownloadURL}
	appOK(w, data)
}

// phpRuntimeCatalogParams 使用 1Panel 通用 PHP 应用中同主版本的表单定义，
// 为 PHP 5/7/8 独立版本应用补齐扩展、版本和扩展源字段。
