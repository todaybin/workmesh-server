// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

func registerWebsiteCRUD(mux *http.ServeMux, svc *service.WebsiteService) {
	list := func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items := svc.List(name, offset, limit)
		total := svc.Count(name)
		if r.URL.Path == "/api/v2/websites/list" {
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": items})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"total": total, "items": items}})
	}
	mux.HandleFunc("GET /api/v2/websites", list)
	mux.HandleFunc("GET /api/v2/websites/list", list)
	mux.HandleFunc("POST /api/v2/websites/search", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name            string `json:"name"`
			Domain          string `json:"domain"`
			Page            int    `json:"page"`
			PageSize        int    `json:"pageSize"`
			WebsiteGroupID  uint   `json:"websiteGroupId"`
			Type            string `json:"type"`
			Status          string `json:"status"`
			WebsiteSSLID    uint   `json:"websiteSSLId"`
			RuntimeID       string `json:"runtimeID"`
			AppInstallID    uint   `json:"appInstallId"`
			ParentWebsiteID uint   `json:"parentWebsiteID"`
			OrderBy         string `json:"orderBy"`
			Order           string `json:"order"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		page := req.Page
		if page < 1 {
			page = 1
		}
		size := req.PageSize
		if size <= 0 || size > 500 {
			size = 100
		}
		all := svc.List(req.Name, 0, 100000)
		filtered := make([]model.Website, 0, len(all))
		for _, item := range all {
			if req.Domain != "" && !strings.Contains(strings.ToLower(item.PrimaryDomain), strings.ToLower(req.Domain)) {
				continue
			}
			if req.WebsiteGroupID != 0 && item.WebsiteGroupID != req.WebsiteGroupID {
				continue
			}
			if req.Type != "" && item.Type != req.Type {
				continue
			}
			if req.Status != "" && !strings.EqualFold(item.Status, req.Status) {
				continue
			}
			if req.WebsiteSSLID != 0 && item.WebsiteSSLID != req.WebsiteSSLID {
				continue
			}
			if strings.TrimSpace(req.RuntimeID) != "" && item.RuntimeID != req.RuntimeID {
				continue
			}
			if req.AppInstallID != 0 && item.AppInstallID != req.AppInstallID {
				continue
			}
			if req.ParentWebsiteID != 0 && item.ParentWebsiteID != req.ParentWebsiteID {
				continue
			}
			filtered = append(filtered, item)
		}
		sort.SliceStable(filtered, func(i, j int) bool {
			by := strings.ToLower(req.OrderBy)
			less := filtered[i].ID < filtered[j].ID
			switch by {
			case "name", "alias":
				less = strings.ToLower(filtered[i].Alias) < strings.ToLower(filtered[j].Alias)
			case "domain", "primarydomain":
				less = strings.ToLower(filtered[i].PrimaryDomain) < strings.ToLower(filtered[j].PrimaryDomain)
			case "createdat":
				less = filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
			}
			if strings.EqualFold(req.Order, "desc") {
				return !less
			}
			return less
		})
		start := (page - 1) * size
		if start > len(filtered) {
			start = len(filtered)
		}
		end := start + size
		if end > len(filtered) {
			end = len(filtered)
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"total": len(filtered), "items": filtered[start:end]}})
	})
	mux.HandleFunc("GET /api/v2/websites/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := svc.Get(id)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("POST /api/v2/websites", func(w http.ResponseWriter, r *http.Request) {
		var req model.WebsiteCreateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		// Deployment sites must point at a running installed application.  The
		// frontend can submit either a numeric appInstallId or a string key (for
		// example "showdoc"); both are normalized into AppInstallRef by the
		// request model.  Do not create a site with an empty/fake upstream.
		if strings.EqualFold(strings.TrimSpace(req.Type), "deployment") || strings.EqualFold(strings.TrimSpace(req.AppType), "deployment") {
			ref := strings.TrimSpace(req.AppInstallRef)
			if ref == "" && req.AppInstallID != 0 {
				ref = strconv.FormatUint(uint64(req.AppInstallID), 10)
			}
			if strings.EqualFold(strings.TrimSpace(req.AppType), "new") {
				writeError(w, http.StatusConflict, errors.New("一键部署应用尚未安装，请先完成应用安装任务"))
				return
			}
			if ref == "" {
				writeError(w, http.StatusBadRequest, errors.New("一键部署必须选择已安装应用"))
				return
			}
			store := getAppStore()
			store.mu.RLock()
			_, app := findApp(store.state.Apps, ref)
			if app.ID == "" {
				for _, candidate := range store.state.Apps {
					if candidate.Key == ref || candidate.Name == ref {
						app = candidate
						break
					}
				}
			}
			store.mu.RUnlock()
			if app.ID == "" {
				writeError(w, http.StatusConflict, fmt.Errorf("应用 %q 未安装", ref))
				return
			}
			if !strings.EqualFold(normalizeAppStatus(app.Status), "Running") {
				writeError(w, http.StatusConflict, fmt.Errorf("应用 %q 当前状态为 %s，必须运行后才能创建站点", ref, normalizeAppStatus(app.Status)))
				return
			}
			req.AppInstallRef = app.ID
			if req.AppInstallID == 0 {
				if id, parseErr := strconv.ParseUint(app.ID, 10, 32); parseErr == nil {
					req.AppInstallID = uint(id)
				}
			}
			if strings.TrimSpace(req.Proxy) == "" {
				port := appConfiguredInt(app.Config, 0, "PANEL_APP_PORT_HTTP", "httpPort", "port")
				if port <= 0 {
					writeError(w, http.StatusConflict, fmt.Errorf("应用 %q 未提供可用 HTTP 端口", ref))
					return
				}
				req.Proxy = fmt.Sprintf("127.0.0.1:%d", port)
			}
		}
		item, err := svc.Create(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("POST /api/v2/websites/update", func(w http.ResponseWriter, r *http.Request) {
		var req model.WebsiteUpdateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := svc.Update(req)
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
	mux.HandleFunc("POST /api/v2/websites/del", func(w http.ResponseWriter, r *http.Request) {
		var req model.WebsiteDeleteRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := svc.Delete(req.ID); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
	})
	// 旧 Agent 的 sites 路径是网站 WAF 兼容别名，保留同一份网站元数据。
	mux.HandleFunc("POST /api/v2/sites", func(w http.ResponseWriter, r *http.Request) {
		var req model.WebsiteCreateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := svc.Create(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("GET /api/v2/sites", func(w http.ResponseWriter, _ *http.Request) {
		items := svc.List("", 0, 1000)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
	})
}
