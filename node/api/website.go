// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// registerWebsiteFunctionalRoutes 注册网站、WAF 和 OpenResty 的兼容接口。
func registerWebsiteFunctionalRoutes(mux *http.ServeMux) {
	svc := service.NewWebsiteService("")
	registerWebsiteCertificateRoutes(mux, service.NewWebsiteSecurityService(""))
	registerWebsiteCRUD(mux, svc)
	registerWebsiteAdvancedRoutes(mux, svc)
	registerWAFRoutes(mux, svc)
	registerOpenRestyRoutes(mux, svc)
	registerXPackWebsiteAliases(mux, svc)
	registerWebsiteExtensionRoutes(mux)
}

// registerWebsiteAdvancedRoutes 注册站点运行、域名、HTTPS 和配置管理接口。
func registerWebsiteAdvancedRoutes(mux *http.ServeMux, svc *service.WebsiteService) {
	// 网站监控契约映射到统一 analytics 采集器，配置与查询均使用同一持久化状态源。
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
	mux.HandleFunc("POST /api/v2/websites/check", func(w http.ResponseWriter, r *http.Request) {
		var in model.WebsiteCreateRequest
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		items := svc.List(in.PrimaryDomain, 0, 2)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"available": len(items) == 0, "domain": strings.TrimSpace(in.PrimaryDomain)}})
	})
	mux.HandleFunc("POST /api/v2/websites/options", func(w http.ResponseWriter, r *http.Request) {
		items := svc.List("", 0, 500)
		types := []map[string]string{{"value": "static", "label": "静态网站"}, {"value": "proxy", "label": "反向代理"}, {"value": "php", "label": "PHP"}}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"types": types, "websites": items}})
	})
	registerDomainRoutes(mux, svc)
	registerWebsiteConfigRoutes(mux, svc)
}

func registerDomainRoutes(mux *http.ServeMux, svc *service.WebsiteService) {
	list := func(w http.ResponseWriter, r *http.Request) {
		idText := strings.TrimPrefix(r.URL.Path, "/api/v2/websites/domains/")
		if idText == r.URL.Path {
			idText = r.PathValue("second")
		}
		id, err := parseID(idText)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		items, err := svc.ListDomains(id)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if err != nil {
			writeError(w, 500, err)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
	}
	// 仅注册旧契约的 websiteId 参数，避免与站点配置通配符产生 ServeMux 冲突。
	// 兼容旧路径 GET /api/v2/websites/domains/:websiteId；与 HTTPS 双段路径统一分发。
	mux.HandleFunc("GET /api/v2/websites/{first}/{second}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("first") == "domains" {
			list(w, r)
			return
		}
		if r.PathValue("first") == "cors" {
			id, err := parseID(r.PathValue("second"))
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			cfg, err := svc.GetConfig(id, "cors")
			if errors.Is(err, os.ErrNotExist) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": cfg})
			return
		}
		if r.PathValue("second") == "https" {
			id, err := parseID(r.PathValue("first"))
			if err != nil {
				writeError(w, 400, err)
				return
			}
			cfg, err := svc.GetConfig(id, "https")
			if errors.Is(err, os.ErrNotExist) {
				writeError(w, 404, err)
				return
			}
			if err != nil {
				writeError(w, 500, err)
				return
			}
			wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": cfg})
			return
		}
		websiteFallbackHandler(w, r)
	})
	mux.HandleFunc("POST /api/v2/websites/domains", func(w http.ResponseWriter, r *http.Request) {
		var in model.WebsiteDomain
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, 400, err)
			return
		}
		item, err := svc.UpsertDomain(in)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, 404, err)
			return
		}
		if err != nil {
			writeError(w, 400, err)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("POST /api/v2/websites/domains/update", func(w http.ResponseWriter, r *http.Request) {
		var in model.WebsiteDomain
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, 400, err)
			return
		}
		item, err := svc.UpsertDomain(in)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, 404, err)
			return
		}
		if err != nil {
			writeError(w, 400, err)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("POST /api/v2/websites/domains/del", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			WebsiteID uint   `json:"websiteID"`
			ID        string `json:"id"`
			DomainID  string `json:"domainID"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, 400, err)
			return
		}
		if in.ID == "" {
			in.ID = in.DomainID
		}
		if err := svc.DeleteDomain(in.WebsiteID, in.ID); errors.Is(err, os.ErrNotExist) {
			writeError(w, 404, err)
			return
		} else if err != nil {
			writeError(w, 400, err)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200})
	})
}

func registerWebsiteConfigRoutes(mux *http.ServeMux, svc *service.WebsiteService) {
	get := func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r.PathValue("id"))
		if err != nil {
			writeError(w, 400, err)
			return
		}
		cfg, err := svc.GetConfig(id, r.PathValue("type"))
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, 404, err)
			return
		}
		if err != nil {
			writeError(w, 500, err)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": cfg})
	}
	mux.HandleFunc("GET /api/v2/websites/{id}/config/{type}", get)
	// 为旧客户端保留的专用配置查询路由，均读取 WebsiteService 持久化配置。
	for _, item := range []struct {
		path string
		typ  string
	}{
		{"/api/v2/websites/proxy/config/{id}", "proxy"},
		{"/api/v2/websites/realip/config/{id}", "realip"},
	} {
		typ := item.typ
		mux.HandleFunc("GET "+item.path, func(w http.ResponseWriter, r *http.Request) {
			id, err := parseID(r.PathValue("id"))
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			cfg, err := svc.GetConfig(id, typ)
			if errors.Is(err, os.ErrNotExist) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": cfg})
		})
	}
	// DNS/CORS/LBS/代理等站点配置写操作统一落入带类型隔离的持久化配置存储。
	for _, item := range []struct{ path, typ string }{
		{"/api/v2/websites/cors/update", "cors"},
		{"/api/v2/websites/dns/update", "dns"},
		{"/api/v2/websites/lbs/create", "lbs"},
		{"/api/v2/websites/lbs/update", "lbs"},
		{"/api/v2/websites/lbs/file", "lbs-file"},
		{"/api/v2/websites/proxy/clear", "proxy"},
		{"/api/v2/websites/stream/update", "stream"},
		{"/api/v2/websites/default/server", "default-server"},
		{"/api/v2/websites/default/html/update", "default-html"},
	} {
		typ := item.typ
		mux.HandleFunc("POST "+item.path, func(w http.ResponseWriter, r *http.Request) {
			websiteConfigWriteType(svc, typ, w, r)
		})
	}
	mux.HandleFunc("POST /api/v2/websites/dns/search", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			WebsiteID uint `json:"websiteID"`
			WebsiteId uint `json:"websiteId"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if in.WebsiteID == 0 {
			in.WebsiteID = in.WebsiteId
		}
		cfg, err := svc.GetConfig(in.WebsiteID, "dns")
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": cfg})
	})
	mux.HandleFunc("POST /api/v2/websites/dns/del", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			WebsiteID uint `json:"websiteID"`
			WebsiteId uint `json:"websiteId"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if in.WebsiteID == 0 {
			in.WebsiteID = in.WebsiteId
		}
		// 删除操作只记录删除标记，不构造固定空列表，保留后续审计和恢复所需的状态来源。
		_, err := svc.UpdateConfig(in.WebsiteID, "dns", map[string]any{"deleted": true, "updatedAt": time.Now().UTC()})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
	})
	mux.HandleFunc("POST /api/v2/websites/config", func(w http.ResponseWriter, r *http.Request) { websiteConfigWrite(svc, w, r) })
	mux.HandleFunc("POST /api/v2/websites/config/update", func(w http.ResponseWriter, r *http.Request) { websiteConfigWrite(svc, w, r) })
	mux.HandleFunc("POST /api/v2/websites/nginx/update", func(w http.ResponseWriter, r *http.Request) { websiteConfigWrite(svc, w, r) })
	// 以下配置接口复用同一持久化存储，但每个类型均单独命名，避免配置相互覆盖。
	for _, item := range []struct{ path, typ string }{
		{"/api/v2/websites/rewrite", "rewrite"}, {"/api/v2/websites/rewrite/update", "rewrite"}, {"/api/v2/websites/rewrite/custom", "rewrite-custom"},
		{"/api/v2/websites/dir", "dir"}, {"/api/v2/websites/dir/update", "dir"}, {"/api/v2/websites/dir/permission", "dir-permission"},
		{"/api/v2/websites/leech", "leech"}, {"/api/v2/websites/leech/update", "leech"},
		{"/api/v2/websites/redirect", "redirect"}, {"/api/v2/websites/redirect/update", "redirect"}, {"/api/v2/websites/redirect/file", "redirect-file"},
	} {
		typ := item.typ
		mux.HandleFunc("POST "+item.path, func(w http.ResponseWriter, r *http.Request) {
			websiteConfigWriteType(svc, typ, w, r)
		})
	}
	mux.HandleFunc("GET /api/v2/websites/rewrite/custom", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r.URL.Query().Get("websiteID"))
		if err != nil {
			writeError(w, 400, err)
			return
		}
		cfg, err := svc.GetConfig(id, "rewrite-custom")
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, 404, err)
			return
		}
		if err != nil {
			writeError(w, 500, err)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": cfg})
	})
	// GET /api/v2/websites/:id/https 由上方双段路由分发。
	mux.HandleFunc("POST /api/v2/websites/{id}/https", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r.PathValue("id"))
		if err != nil {
			writeError(w, 400, err)
			return
		}
		var in struct {
			Enabled *bool  `json:"enabled"`
			Operate string `json:"operate"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, 400, err)
			return
		}
		enabled := in.Enabled != nil && *in.Enabled
		if in.Operate != "" {
			enabled = in.Operate == "enable"
		}
		cfg, err := svc.UpdateConfig(id, "https", map[string]any{"enabled": enabled})
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, 404, err)
			return
		}
		if err != nil {
			writeError(w, 400, err)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": cfg})
	})
}

func websiteConfigWrite(svc *service.WebsiteService, w http.ResponseWriter, r *http.Request) {
	var in struct {
		WebsiteID uint           `json:"websiteID"`
		ID        uint           `json:"id"`
		Type      string         `json:"type"`
		Config    map[string]any `json:"config"`
		Content   string         `json:"content"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.WebsiteID == 0 {
		in.WebsiteID = in.ID
	}
	if in.Type == "" {
		in.Type = "nginx"
	}
	value := in.Config
	if value == nil {
		value = map[string]any{"content": in.Content}
	}
	cfg, err := svc.UpdateConfig(in.WebsiteID, in.Type, value)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, 404, err)
		return
	}
	if err != nil {
		writeError(w, 400, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": cfg})
}

func websiteConfigWriteType(svc *service.WebsiteService, typ string, w http.ResponseWriter, r *http.Request) {
	var in struct {
		WebsiteID uint           `json:"websiteID"`
		ID        uint           `json:"id"`
		Config    map[string]any `json:"config"`
		Content   string         `json:"content"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.WebsiteID == 0 {
		in.WebsiteID = in.ID
	}
	value := in.Config
	if value == nil {
		value = map[string]any{"content": in.Content}
	}
	cfg, err := svc.UpdateConfig(in.WebsiteID, typ, value)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, 404, err)
		return
	}
	if err != nil {
		writeError(w, 400, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": cfg})
}

// registerXPackWebsiteAliases 保留旧 Agent 的 xpack 监控/WAF 路径。
// 专用统计采集器接入前，先使用统一兼容存储承接请求，避免隐藏路由返回 404。
func registerXPackWebsiteAliases(mux *http.ServeMux, svc *service.WebsiteService) {
	// 使用显式模式便于契约扫描器发现每一条隐藏路由，并保留方法级约束。
	register := func(method, source, target string) {
		mux.HandleFunc(method+source, proxyFunctionalPath(target, http.HandlerFunc(analyticsHandler)))
	}
	register("GET", "/api/v2/xpack/monitor/status", "/api/v2/status")
	register("POST", "/api/v2/xpack/monitor/stat", "/api/v2/stat")
	register("POST", "/api/v2/xpack/monitor/visitors", "/api/v2/visitors")
	register("POST", "/api/v2/xpack/monitor/visitors/loc", "/api/v2/visitors/loc")
	register("POST", "/api/v2/xpack/monitor/qps", "/api/v2/qps")
	register("POST", "/api/v2/xpack/monitor/rank", "/api/v2/rank")
	register("POST", "/api/v2/xpack/monitor/trend", "/api/v2/trend")
	register("POST", "/api/v2/xpack/monitor/logs/search", "/api/v2/stat")
	register("POST", "/api/v2/xpack/monitor/logs/stat", "/api/v2/stat")
	register("POST", "/api/v2/xpack/monitor/logs/detail", "/api/v2/stat")
	register("POST", "/api/v2/xpack/monitor/logs/clear", "/api/v2/stat")
	register("POST", "/api/v2/xpack/monitor/websites", "/api/v2/rank")
	register("GET", "/api/v2/xpack/monitor/config/global", "/api/v2/global")
	register("POST", "/api/v2/xpack/monitor/config/global", "/api/v2/global")
	register("POST", "/api/v2/xpack/monitor/config/site", "/api/v2/config/site")
	register("POST", "/api/v2/xpack/monitor/config/site/update", "/api/v2/config/site/update")
	wafMux := http.NewServeMux()
	registerWAFRoutes(wafMux, svc)
	waf := func(method, source, target string) {
		mux.HandleFunc(method+source, proxyFunctionalPath(target, wafMux))
	}
	waf("GET", "/api/v2/xpack/waf/status", "/api/v2/websites/waf/status")
	waf("GET", "/api/v2/xpack/waf/standard-rules", "/api/v2/websites/waf/standard-rules")
	waf("POST", "/api/v2/xpack/waf/test", "/api/v2/websites/waf/test")
	waf("POST", "/api/v2/xpack/waf/global", "/api/v2/websites/waf/global")
	waf("GET", "/api/v2/xpack/waf/sites", "/api/v2/websites/waf/sites")
	waf("POST", "/api/v2/xpack/waf/sites", "/api/v2/websites/waf/sites")
	waf("GET", "/api/v2/xpack/waf/sites/{id}/rules", "/api/v2/websites/waf/sites/{id}/rules")
	waf("POST", "/api/v2/xpack/waf/rules", "/api/v2/websites/waf/rules")
	waf("POST", "/api/v2/xpack/waf/rules/delete", "/api/v2/websites/waf/rules/delete")
	waf("GET", "/api/v2/xpack/waf/access-lists", "/api/v2/websites/waf/access-lists")
	waf("POST", "/api/v2/xpack/waf/access-lists", "/api/v2/websites/waf/access-lists")
	register("POST", "/api/v2/xpack/waf/attack/stat", "/api/v2/attack/stat")
	register("POST", "/api/v2/xpack/waf/block/search", "/api/v2/block/search")
	register("POST", "/api/v2/xpack/waf/relation/stat", "/api/v2/relation/stat")
	register("POST", "/api/v2/xpack/waf/log/search", "/api/v2/stat")
}

// proxyFunctionalPath 将旧别名请求映射到同一进程中的真实处理器。
func proxyFunctionalPath(target string, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clone := r.Clone(r.Context())
		clone.URL.Path = strings.ReplaceAll(target, "{id}", r.PathValue("id"))
		next.ServeHTTP(w, clone)
	}
}

// websiteFallbackHandler 为尚未拥有专用业务动作的网站路径提供真实状态查询。
// 写操作必须由专用路由处理，兜底接口拒绝未知动作，避免伪造成功。
func websiteFallbackHandler(w http.ResponseWriter, r *http.Request) {
	svc := service.NewWebsiteService("")
	if r.Method == http.MethodGet {
		idText := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/websites/"), "/")
		if idText == "" {
			items := svc.List("", 0, 100)
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
			return
		}
		digits := strings.FieldsFunc(idText, func(r rune) bool { return r < '0' || r > '9' })
		if len(digits) > 0 {
			id, err := strconv.ParseUint(digits[0], 10, 32)
			if err != nil || id == 0 {
				writeError(w, http.StatusBadRequest, errors.New("WEBSITE_ID_INVALID"))
				return
			}
			item, getErr := svc.Get(uint(id))
			if getErr != nil {
				writeError(w, http.StatusNotFound, getErr)
				return
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
			return
		}
	}
	writeError(w, http.StatusNotImplemented, errors.New("WEBSITE_OPERATION_REQUIRES_EXPLICIT_ROUTE"))
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
