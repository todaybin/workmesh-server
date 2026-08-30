// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// registerWebsiteFunctionalRoutes 注册网站、WAF 和 OpenResty 的兼容接口。
func registerWebsiteFunctionalRoutes(mux *http.ServeMux) {
	svc := service.NewWebsiteService("")
	registerWebsiteCRUD(mux, svc)
	registerWAFRoutes(mux, svc)
	registerOpenRestyRoutes(mux, svc)
	registerXPackWebsiteAliases(mux)
}

// registerXPackWebsiteAliases 保留旧 Agent 的 xpack 监控/WAF 路径。
// 专用统计采集器接入前，先使用统一兼容存储承接请求，避免隐藏路由返回 404。
func registerXPackWebsiteAliases(mux *http.ServeMux) {
	// 使用显式模式便于契约扫描器发现每一条隐藏路由，并保留方法级约束。
	mux.HandleFunc("GET /api/v2/xpack/monitor/status", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/monitor/stat", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/monitor/visitors", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/monitor/visitors/loc", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/monitor/qps", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/monitor/rank", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/monitor/trend", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/monitor/logs/search", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/monitor/logs/stat", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/monitor/logs/detail", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/monitor/logs/clear", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/monitor/websites", compatibilityHandler)
	mux.HandleFunc("GET /api/v2/xpack/monitor/config/global", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/monitor/config/global", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/monitor/config/site", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/monitor/config/site/update", compatibilityHandler)
	mux.HandleFunc("GET /api/v2/xpack/waf/status", compatibilityHandler)
	mux.HandleFunc("GET /api/v2/xpack/waf/standard-rules", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/waf/test", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/waf/global", compatibilityHandler)
	mux.HandleFunc("GET /api/v2/xpack/waf/sites", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/waf/sites", compatibilityHandler)
	mux.HandleFunc("GET /api/v2/xpack/waf/sites/{id}/rules", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/waf/rules", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/waf/rules/delete", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/waf/attack/stat", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/waf/log/search", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/waf/block/search", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/waf/relation/stat", compatibilityHandler)
	mux.HandleFunc("GET /api/v2/xpack/waf/access-lists", compatibilityHandler)
	mux.HandleFunc("POST /api/v2/xpack/waf/access-lists", compatibilityHandler)
}

// isFunctionalDomainRoute 让 legacy 路由过滤器跳过已经实现的占位契约。
func isWebsiteFunctionalRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	if len(parts) != 2 {
		return false
	}
	method, path := parts[0], parts[1]
	// 网站域采用统一子树兼容入口，避免旧动态段与 SSL 等专用路由产生 ServeMux 冲突。
	if strings.HasPrefix(path, "/api/v2/websites/") || path == "/api/v2/websites" {
		return true
	}
	if strings.HasPrefix(path, "/api/v2/openresty/") && (method == "GET" || method == "POST") {
		return true
	}
	if path == "/api/v2/sites" || path == "/api/v2/standard-rules" || path == "/api/v2/access-lists" || path == "/api/v2/websites" || path == "/api/v2/websites/list" || path == "/api/v2/websites/search" || path == "/api/v2/websites/update" || path == "/api/v2/websites/del" {
		return method == "GET" || method == "POST"
	}
	if path == "/api/v2/websites/:id" {
		return method == "GET"
	}
	if path == "/api/v2/sites/:id/rules" {
		return method == "GET"
	}
	if path == "/api/v2/rules" || path == "/api/v2/rules/delete" {
		return method == "POST"
	}
	switch path {
	case "/api/v2/websites/waf/status", "/api/v2/websites/waf/standard-rules", "/api/v2/websites/waf/sites", "/api/v2/websites/waf/sites/:id/rules", "/api/v2/websites/waf/access-lists":
		return method == "GET" || method == "POST"
	case "/api/v2/websites/waf/global", "/api/v2/websites/waf/rules", "/api/v2/websites/waf/rules/delete", "/api/v2/websites/waf/sites/:id", "/api/v2/websites/waf/test":
		if path == "/api/v2/websites/waf/global" {
			return method == "GET" || method == "POST"
		}
		return method == "POST"
	}
	return false
}

func registerWebsiteCRUD(mux *http.ServeMux, svc *service.WebsiteService) {
	list := func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items := svc.List(name, offset, limit)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"total": len(items), "items": items}})
	}
	mux.HandleFunc("GET /api/v2/websites", list)
	mux.HandleFunc("GET /api/v2/websites/list", list)
	mux.HandleFunc("POST /api/v2/websites/search", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name     string `json:"name"`
			Page     int    `json:"page"`
			PageSize int    `json:"pageSize"`
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
		items := svc.List(req.Name, (page-1)*size, size)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"total": len(svc.List(req.Name, 0, 100000)), "items": items}})
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

func registerWAFRoutes(mux *http.ServeMux, svc *service.WebsiteService) {
	getLists := func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": svc.GetLists()})
	}
	updateLists := func(w http.ResponseWriter, r *http.Request) {
		var req model.WAFAccessLists
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := svc.UpdateLists(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
	}
	for _, path := range []string{"/api/v2/access-lists", "/api/v2/websites/waf/access-lists"} {
		mux.HandleFunc("GET "+path, getLists)
		mux.HandleFunc("POST "+path, updateLists)
	}
	mux.HandleFunc("GET /api/v2/websites/waf/status", func(w http.ResponseWriter, r *http.Request) {
		cfg := svc.GetGlobal()
		resty := svc.GetOpenResty()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"available": true, "enabled": cfg.Enabled, "mode": cfg.Mode, "runtime": "openresty", "crs": "4.14.0", "standardRules": cfg.StandardRules, "version": resty.Version}})
	})
	standard := func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": svc.StandardRules()})
	}
	mux.HandleFunc("GET /api/v2/standard-rules", standard)
	mux.HandleFunc("GET /api/v2/websites/waf/standard-rules", standard)
	mux.HandleFunc("GET /api/v2/websites/waf/sites", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": svc.ListWAFSites()})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/sites", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WebsiteID uint   `json:"websiteID"`
			Enabled   bool   `json:"enabled"`
			Mode      string `json:"mode"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := svc.UpdateWAFSite(req.WebsiteID, req.Enabled, req.Mode)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("GET /api/v2/websites/waf/sites/{id}/rules", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		rules, err := svc.ListRules(id)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": rules})
	})
	// xpack/waf 兼容别名与网站 WAF 使用同一份规则存储。
	mux.HandleFunc("GET /api/v2/sites/{id}/rules", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		rules, err := svc.ListRules(id)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": rules})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/rules", func(w http.ResponseWriter, r *http.Request) {
		var req model.WAFRule
		var body struct {
			WebsiteID uint `json:"websiteID"`
			WebsiteId uint `json:"websiteId"`
			model.WAFRule
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		req = body.WAFRule
		id := body.WebsiteID
		if id == 0 {
			id = body.WebsiteId
		}
		item, err := svc.UpsertRule(id, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/rules/delete", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WebsiteID uint   `json:"websiteID"`
			WebsiteId uint   `json:"websiteId"`
			ID        string `json:"id"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		id := req.WebsiteID
		if id == 0 {
			id = req.WebsiteId
		}
		if err := svc.DeleteRule(id, req.ID); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
	})
	mux.HandleFunc("POST /api/v2/rules", func(w http.ResponseWriter, r *http.Request) { wafRuleUpsert(svc, w, r) })
	mux.HandleFunc("POST /api/v2/rules/delete", func(w http.ResponseWriter, r *http.Request) { wafRuleDelete(svc, w, r) })
	mux.HandleFunc("POST /api/v2/websites/waf/global", func(w http.ResponseWriter, r *http.Request) {
		var req model.WAFGlobalConfig
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		cfg, err := svc.UpdateGlobal(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": cfg})
	})
	mux.HandleFunc("GET /api/v2/websites/waf/global", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": svc.GetGlobal()})
	})
}

func registerOpenRestyRoutes(mux *http.ServeMux, svc *service.WebsiteService) {
	mux.HandleFunc("GET /api/v2/openresty/status", func(w http.ResponseWriter, r *http.Request) {
		cfg := svc.GetOpenResty()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"version": cfg.Version, "enabled": cfg.Enabled, "defaultHttps": cfg.DefaultHTTPS, "modules": cfg.Modules}})
	})
	mux.HandleFunc("GET /api/v2/openresty/modules", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": svc.GetOpenResty().Modules})
	})
	mux.HandleFunc("GET /api/v2/openresty/https", func(w http.ResponseWriter, r *http.Request) {
		cfg := svc.GetOpenResty()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"open": cfg.DefaultHTTPS, "enabled": cfg.DefaultHTTPS}})
	})
	mux.HandleFunc("POST /api/v2/openresty/update", openRestyUpdate(svc))
	mux.HandleFunc("POST /api/v2/openresty/file", openRestyUpdate(svc))
	mux.HandleFunc("POST /api/v2/openresty/scope", openRestyUpdate(svc))
	mux.HandleFunc("POST /api/v2/openresty/build", openRestyUpdate(svc))
	mux.HandleFunc("POST /api/v2/openresty/modules/update", openRestyUpdate(svc))
	mux.HandleFunc("POST /api/v2/openresty/https", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Operate string `json:"operate"`
		}
		if err := decodeJSON(r, &req); err != nil || (req.Operate != "enable" && req.Operate != "disable") {
			writeError(w, http.StatusBadRequest, errors.New("HTTPS 操作无效"))
			return
		}
		cfg := svc.GetOpenResty()
		cfg.DefaultHTTPS = req.Operate == "enable"
		result, err := svc.UpdateOpenResty(cfg)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
	})
}

func openRestyUpdate(svc *service.WebsiteService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req model.OpenRestyConfig
		_ = decodeJSON(r, &req)
		cfg := svc.GetOpenResty()
		if req.Version != "" {
			cfg.Version = req.Version
		}
		if req.ConfigContent != "" {
			cfg.ConfigContent = req.ConfigContent
		}
		if req.Modules != nil {
			cfg.Modules = req.Modules
		}
		result, err := svc.UpdateOpenResty(cfg)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
	}
}

func wafRuleUpsert(svc *service.WebsiteService, w http.ResponseWriter, r *http.Request) {
	var body struct {
		WebsiteID uint `json:"websiteID"`
		WebsiteId uint `json:"websiteId"`
		model.WAFRule
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := body.WebsiteID
	if id == 0 {
		id = body.WebsiteId
	}
	item, err := svc.UpsertRule(id, body.WAFRule)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
}

func wafRuleDelete(svc *service.WebsiteService, w http.ResponseWriter, r *http.Request) {
	var req struct {
		WebsiteID uint   `json:"websiteID"`
		WebsiteId uint   `json:"websiteId"`
		ID        string `json:"id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := req.WebsiteID
	if id == 0 {
		id = req.WebsiteId
	}
	if err := svc.DeleteRule(id, req.ID); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
}

func parseID(value string) (uint, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
	if err != nil || id == 0 {
		return 0, errors.New("ID 无效")
	}
	return uint(id), nil
}
