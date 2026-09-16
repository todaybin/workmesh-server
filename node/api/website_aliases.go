// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
	"strconv"
	"strings"
)

func registerXPackWebsiteAliases(mux *http.ServeMux, svc *service.WebsiteService) {
	// 使用显式模式便于契约扫描器发现每一条隐藏路由，并保留方法级约束。
	register := func(method, source, target string) {
		// ServeMux 模式必须使用“方法 + 空格 + 路径”，否则会被当作普通路径而永远无法匹配。
		mux.HandleFunc(method+" "+source, proxyFunctionalPath(target, http.HandlerFunc(analyticsHandler)))
	}
	register("GET", "/api/v2/xpack/monitor/status", "/api/v2/status")
	register("POST", "/api/v2/xpack/monitor/stat", "/api/v2/stat")
	register("POST", "/api/v2/xpack/monitor/visitors", "/api/v2/visitors")
	register("POST", "/api/v2/xpack/monitor/visitors/loc", "/api/v2/visitors/loc")
	register("POST", "/api/v2/xpack/monitor/qps", "/api/v2/qps")
	register("POST", "/api/v2/xpack/monitor/rank", "/api/v2/rank")
	register("POST", "/api/v2/xpack/monitor/trend", "/api/v2/trend")
	register("POST", "/api/v2/xpack/monitor/websites", "/api/v2/rank")
	register("GET", "/api/v2/xpack/monitor/config/global", "/api/v2/global")
	register("POST", "/api/v2/xpack/monitor/config/global", "/api/v2/global")
	register("POST", "/api/v2/xpack/monitor/config/site", "/api/v2/config/site")
	register("POST", "/api/v2/xpack/monitor/config/site/update", "/api/v2/config/site/update")
	monitorMux := http.NewServeMux()
	registerWebsiteMonitorLogRoutes(monitorMux)
	monitor := func(method, source, target string) {
		mux.HandleFunc(method+" "+source, proxyFunctionalPath(target, monitorMux))
	}
	monitor("POST", "/api/v2/xpack/monitor/logs/search", "/api/v2/websites/monitor/logs/search")
	monitor("POST", "/api/v2/xpack/monitor/logs/stat", "/api/v2/websites/monitor/logs/stat")
	monitor("POST", "/api/v2/xpack/monitor/logs/detail", "/api/v2/websites/monitor/logs/detail")
	monitor("POST", "/api/v2/xpack/monitor/logs/clear", "/api/v2/websites/monitor/logs/clear")
	wafMux := http.NewServeMux()
	registerWAFRoutes(wafMux, svc)
	waf := func(method, source, target string) {
		mux.HandleFunc(method+" "+source, proxyFunctionalPath(target, wafMux))
	}
	waf("GET", "/api/v2/xpack/waf/status", "/api/v2/websites/waf/status")
	waf("GET", "/api/v2/xpack/waf/standard-rules", "/api/v2/websites/waf/standard-rules")
	waf("POST", "/api/v2/xpack/waf/test", "/api/v2/websites/waf/test")
	waf("POST", "/api/v2/xpack/waf/global", "/api/v2/websites/waf/global")
	waf("GET", "/api/v2/xpack/waf/global/default-rules", "/api/v2/websites/waf/global/default-rules")
	waf("POST", "/api/v2/xpack/waf/global/default-rules", "/api/v2/websites/waf/global/default-rules")
	waf("GET", "/api/v2/xpack/waf/global/custom-rules", "/api/v2/websites/waf/global/custom-rules")
	waf("POST", "/api/v2/xpack/waf/global/custom-rules", "/api/v2/websites/waf/global/custom-rules")
	waf("POST", "/api/v2/xpack/waf/global/apply", "/api/v2/websites/waf/global/apply")
	waf("GET", "/api/v2/xpack/waf/sites", "/api/v2/websites/waf/sites")
	waf("POST", "/api/v2/xpack/waf/sites", "/api/v2/websites/waf/sites")
	waf("GET", "/api/v2/xpack/waf/sites/{id}/rules", "/api/v2/websites/waf/sites/{id}/rules")
	waf("POST", "/api/v2/xpack/waf/rules", "/api/v2/websites/waf/rules")
	waf("POST", "/api/v2/xpack/waf/rules/delete", "/api/v2/websites/waf/rules/delete")
	waf("GET", "/api/v2/xpack/waf/access-lists", "/api/v2/websites/waf/access-lists")
	waf("POST", "/api/v2/xpack/waf/access-lists", "/api/v2/websites/waf/access-lists")
	waf("POST", "/api/v2/xpack/waf/attack/stat", "/api/v2/websites/waf/attack/stat")
	waf("POST", "/api/v2/xpack/waf/block/search", "/api/v2/websites/waf/block/search")
	waf("POST", "/api/v2/xpack/waf/relation/stat", "/api/v2/websites/waf/relation/stat")
	waf("POST", "/api/v2/xpack/waf/log/search", "/api/v2/websites/waf/log/search")
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
	if path == "/api/v2/sites" || path == "/api/v2/standard-rules" || path == "/api/v2/access-lists" || path == "/api/v2/websites" || path == "/api/v2/websites/list" || path == "/api/v2/websites/search" || path == "/api/v2/websites/update" || path == "/api/v2/websites/del" || path == "/api/v2/websites/check" {
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
