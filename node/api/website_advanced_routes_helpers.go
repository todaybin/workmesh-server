// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// registerWebsiteMonitorAnalyticsAliases 注册网站监控别名并转发到统一统计处理器。
func registerWebsiteMonitorAnalyticsAliases(mux *http.ServeMux) {
	for _, item := range []struct{ path, target string }{
		{"/api/v2/websites/monitor/config/global", "/api/v2/global"},
		{"/api/v2/websites/monitor/config/site", "/api/v2/config/site"},
		{"/api/v2/websites/monitor/config/site/update", "/api/v2/config/site/update"},
		{"/api/v2/websites/monitor/qps", "/api/v2/qps"},
		{"/api/v2/websites/monitor/rank", "/api/v2/rank"},
		{"/api/v2/websites/monitor/stat", "/api/v2/stat"},
		{"/api/v2/websites/monitor/trend", "/api/v2/trend"},
		{"/api/v2/websites/monitor/visitors", "/api/v2/visitors"},
		{"/api/v2/websites/monitor/visitors/loc", "/api/v2/visitors/loc"},
		{"/api/v2/websites/monitor/websites", "/api/v2/rank"},
	} {
		target := item.target
		mux.HandleFunc("POST "+item.path, func(w http.ResponseWriter, r *http.Request) {
			clone := r.Clone(r.Context())
			clone.URL.Path = target
			analyticsHandler(w, clone)
		})
	}
	mux.HandleFunc("GET /api/v2/websites/monitor/config/global", func(w http.ResponseWriter, r *http.Request) {
		clone := r.Clone(r.Context())
		clone.URL.Path = "/api/v2/global"
		analyticsHandler(w, clone)
	})
}

// registerWebsiteOperateRoute 注册网站启停等生命周期操作接口。
func registerWebsiteOperateRoute(mux *http.ServeMux, svc *service.WebsiteService) {
	mux.HandleFunc("POST /api/v2/websites/operate", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID        uint   `json:"id"`
			WebsiteID uint   `json:"websiteID"`
			Operate   string `json:"operate"`
			Operation string `json:"operation"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if in.ID == 0 {
			in.ID = in.WebsiteID
		}
		if in.Operation == "" {
			in.Operation = in.Operate
		}
		item, err := svc.Operate(in.ID, in.Operation)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
}

// registerWebsiteCheckRoute 注册 OpenResty 安装状态检查接口。
func registerWebsiteCheckRoute(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/websites/check", websiteCheckHandler)
}

// websiteCheckHandler 根据真实应用状态和 OpenResty 探针生成检查结果。
func websiteCheckHandler(w http.ResponseWriter, r *http.Request) {
	var in struct {
		InstallIDs []uint `json:"installIds"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	store := getAppStore()
	store.mu.RLock()
	_, openresty := findApp(store.state.Apps, "openresty")
	_, catalogApp := findApp(store.state.Catalog, "openresty")
	store.mu.RUnlock()
	probe := service.NewWebsiteService("").ProbeOpenResty(r.Context())
	if openresty.ID == "" && !probe.Available {
		appName := catalogApp.Name
		if appName == "" {
			appName = "OpenResty"
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": []map[string]any{{"name": "", "appName": appName, "version": "", "status": appName + " 未安装"}}})
		return
	}
	ids := websiteCheckInstallIDs(openresty.ID, in.InstallIDs)
	items := make([]map[string]any, 0, len(ids))
	showError := false
	for _, id := range ids {
		item, exists, err := store.syncAppInstallStatus(r.Context(), id, false)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if !exists {
			continue
		}
		status := normalizeAppStatus(item.Status)
		appName := item.Name
		if strings.EqualFold(item.Key, "openresty") {
			appName = "OpenResty"
		}
		store.mu.RLock()
		_, catalogItem := findApp(store.state.Catalog, item.Key)
		store.mu.RUnlock()
		if catalogItem.Name != "" {
			appName = catalogItem.Name
		}
		items = append(items, map[string]any{"name": item.Name, "appName": appName, "version": item.Version, "status": status})
		showError = showError || status != "Running"
	}
	if !showError {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": nil})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": items})
}

// websiteCheckInstallIDs 合并并去重 OpenResty 与请求中的安装记录 ID。
func websiteCheckInstallIDs(openrestyID string, installIDs []uint) []string {
	ids := make([]string, 0, len(installIDs)+1)
	seen := map[string]bool{}
	if openrestyID != "" {
		seen[openrestyID] = true
		ids = append(ids, openrestyID)
	}
	for _, id := range installIDs {
		if id == 0 {
			continue
		}
		value := strconv.FormatUint(uint64(id), 10)
		if !seen[value] {
			seen[value] = true
			ids = append(ids, value)
		}
	}
	if len(ids) == 0 {
		ids = append(ids, openrestyID)
	}
	return ids
}

// registerWebsiteOptionsRoute 注册网站选项查询接口，返回真实 SQLite 网站记录。
func registerWebsiteOptionsRoute(mux *http.ServeMux, svc *service.WebsiteService) {
	mux.HandleFunc("POST /api/v2/websites/options", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Types []string `json:"types"`
		}
		if r.Body != nil {
			if err := decodeJSON(r, &in); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
		}
		allowed := map[string]bool{}
		for _, typ := range in.Types {
			allowed[strings.ToLower(strings.TrimSpace(typ))] = true
		}
		items := svc.List("", 0, 500)
		options := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if len(allowed) > 0 && !allowed[strings.ToLower(strings.TrimSpace(item.Type))] {
				continue
			}
			options = append(options, map[string]any{"id": item.ID, "primaryDomain": item.PrimaryDomain, "alias": item.Alias})
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": options})
	})
}
