// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
	"os"
	"strings"
)

func registerWebsiteConfigRoutes(mux *http.ServeMux, svc *service.WebsiteService) {
	get := func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r.PathValue("id"))
		if err != nil {
			writeError(w, 400, err)
			return
		}
		if r.PathValue("type") == "basic" {
			cfg, err := svc.BasicConfig(id)
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
		if r.PathValue("type") == "openresty" {
			cfg, err := svc.WebsiteConfigFile(id)
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
		{"/api/v2/websites/lbs/create", "lbs"},
		{"/api/v2/websites/lbs/update", "lbs"},
		{"/api/v2/websites/proxy/clear", "proxy"},
		{"/api/v2/websites/proxy/config", "proxy"},
		{"/api/v2/websites/realip/config", "realip"},
		{"/api/v2/websites/stream/update", "stream"},
		{"/api/v2/websites/default/server", "default-server"},
	} {
		typ := item.typ
		mux.HandleFunc("POST "+item.path, func(w http.ResponseWriter, r *http.Request) {
			websiteConfigWriteType(svc, typ, w, r)
		})
	}
	mux.HandleFunc("POST /api/v2/websites/lbs/file", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			WebsiteID uint   `json:"websiteID"`
			ID        uint   `json:"id"`
			Name      string `json:"name"`
			Content   string `json:"content"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if in.WebsiteID == 0 {
			in.WebsiteID = in.ID
		}
		result, err := svc.UpdateWebsiteLoadBalanceFile(in.WebsiteID, in.Name, in.Content)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
	})
	// 删除负载均衡必须同步更新托管 upstream 文件和 SQLite，不能落入
	// 扩展存储兼容层。name 既兼容 upstream 名称，也兼容单个 server 地址。
	mux.HandleFunc("POST /api/v2/websites/lbs/del", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			WebsiteID uint   `json:"websiteID"`
			ID        uint   `json:"id"`
			Name      string `json:"name"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if in.WebsiteID == 0 {
			in.WebsiteID = in.ID
		}
		if in.WebsiteID == 0 {
			writeError(w, http.StatusBadRequest, errors.New("网站 ID 无效"))
			return
		}
		cfg, err := svc.GetConfig(in.WebsiteID, "lbs")
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if in.Name == "" {
			cfg["upstreams"] = []any{}
			cfg["servers"] = []any{}
		} else {
			var rawItems []map[string]any
			if encoded, marshalErr := json.Marshal(cfg["upstreams"]); marshalErr == nil {
				_ = json.Unmarshal(encoded, &rawItems)
			}
			kept := make([]any, 0, len(rawItems))
			for _, item := range rawItems {
				candidate := fmt.Sprint(item["name"])
				if candidate == "<nil>" || candidate == "" {
					candidate = fmt.Sprint(item["address"])
				}
				if candidate == "<nil>" || candidate == "" {
					candidate = fmt.Sprint(item["server"])
				}
				if candidate != in.Name {
					kept = append(kept, item)
				}
			}
			cfg["upstreams"] = kept
			if len(kept) == 0 {
				delete(cfg, "name")
				delete(cfg, "algorithm")
				delete(cfg, "servers")
			}
		}
		cfg["enabled"] = false
		updated, err := svc.UpdateConfig(in.WebsiteID, "lbs", cfg)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": updated})
	})
	// 1Panel uses POST /websites/leech as the read endpoint. Keep it explicit
	// so the request cannot fall through to the generic extension store.
	mux.HandleFunc("POST /api/v2/websites/leech", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			WebsiteID uint `json:"websiteID"`
			ID        uint `json:"id"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, 400, err)
			return
		}
		if in.WebsiteID == 0 {
			in.WebsiteID = in.ID
		}
		cfg, err := svc.GetConfig(in.WebsiteID, "leech")
		if err != nil {
			writeError(w, 404, err)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": cfg})
	})
	mux.HandleFunc("POST /api/v2/websites/dns/search", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Page     int    `json:"page"`
			PageSize int    `json:"pageSize"`
			Keyword  string `json:"keyword"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		total, items := svc.ListDNSAccounts(in.Keyword, in.Page, in.PageSize)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"total": total, "items": items, "page": normalizedPage(in.Page), "pageSize": normalizedPageSize(in.PageSize)}})
	})
	mux.HandleFunc("POST /api/v2/websites/dns", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID            uint           `json:"id"`
			Name          string         `json:"name"`
			Type          string         `json:"type"`
			Provider      string         `json:"provider"`
			Authorization map[string]any `json:"authorization"`
			Credentials   map[string]any `json:"credentials"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if in.Provider == "" {
			in.Provider = in.Type
		}
		if in.Credentials == nil {
			in.Credentials = in.Authorization
		}
		item, err := svc.UpsertDNSAccount(service.DNSAccount{ID: in.ID, Name: in.Name, Type: in.Type, Provider: in.Provider, Credentials: in.Credentials})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("POST /api/v2/websites/dns/update", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID            uint           `json:"id"`
			Name          string         `json:"name"`
			Type          string         `json:"type"`
			Provider      string         `json:"provider"`
			Authorization map[string]any `json:"authorization"`
			Credentials   map[string]any `json:"credentials"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if in.Provider == "" {
			in.Provider = in.Type
		}
		if in.Credentials == nil {
			in.Credentials = in.Authorization
		}
		item, err := svc.UpsertDNSAccount(service.DNSAccount{ID: in.ID, Name: in.Name, Type: in.Type, Provider: in.Provider, Credentials: in.Credentials})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("POST /api/v2/websites/dns/del", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID uint `json:"id"`
		}
		if err := decodeJSON(r, &in); err != nil || in.ID == 0 {
			if err == nil {
				err = errors.New("DNS 账户 ID 无效")
			}
			writeError(w, 400, err)
			return
		}
		if err := svc.DeleteDNSAccount(in.ID); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				writeError(w, 404, err)
			} else {
				writeError(w, 400, err)
			}
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200})
	})
	configHandler := func(w http.ResponseWriter, r *http.Request) { websiteConfigWrite(svc, w, r) }
	mux.HandleFunc("POST /api/v2/websites/config", configHandler)
	mux.HandleFunc("POST /api/v2/websites/config/update", configHandler)
	// 代理列表必须读取站点域名目录 nginx/proxy 下的真实配置，不能落入旧扩展元数据兜底。
	proxyListHandler := func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID        json.RawMessage `json:"id"`
			WebsiteID uint            `json:"websiteID"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		id := in.WebsiteID
		if id == 0 && len(in.ID) > 0 {
			_ = json.Unmarshal(in.ID, &id)
		}
		items, err := svc.ListWebsiteProxies(id)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": items})
	}
	mux.HandleFunc("POST /api/v2/websites/proxies", proxyListHandler)
	mux.HandleFunc("POST /api/v2/websites/nginx/update", func(w http.ResponseWriter, r *http.Request) { websiteConfigWrite(svc, w, r) })
	mux.HandleFunc("POST /api/v2/websites/dir", func(w http.ResponseWriter, r *http.Request) { websiteDirRead(svc, w, r) })
	mux.HandleFunc("POST /api/v2/websites/dir/update", func(w http.ResponseWriter, r *http.Request) { websiteDirUpdate(svc, w, r) })
	mux.HandleFunc("POST /api/v2/websites/dir/permission", func(w http.ResponseWriter, r *http.Request) { websiteDirPermission(svc, w, r) })
	mux.HandleFunc("POST /api/v2/websites/rewrite", func(w http.ResponseWriter, r *http.Request) { websiteRewriteRead(svc, w, r) })
	mux.HandleFunc("POST /api/v2/websites/rewrite/update", func(w http.ResponseWriter, r *http.Request) { websiteRewriteUpdate(svc, w, r) })
	// 以下配置接口复用同一持久化存储，但每个类型均单独命名，避免配置相互覆盖。
	for _, item := range []struct{ path, typ string }{
		{"/api/v2/websites/leech/update", "leech"},
		{"/api/v2/websites/redirect", "redirect"}, {"/api/v2/websites/redirect/update", "redirect"}, {"/api/v2/websites/redirect/file", "redirect-file"},
	} {
		typ := item.typ
		mux.HandleFunc("POST "+item.path, func(w http.ResponseWriter, r *http.Request) {
			websiteConfigWriteType(svc, typ, w, r)
		})
	}
	mux.HandleFunc("POST /api/v2/websites/rewrite/custom", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			WebsiteID uint           `json:"websiteID"`
			ID        uint           `json:"id"`
			Config    map[string]any `json:"config"`
			Content   string         `json:"content"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if in.WebsiteID == 0 {
			in.WebsiteID = in.ID
		}
		if in.Config == nil {
			in.Config = map[string]any{}
		}
		if in.Content == "" {
			in.Content, _ = in.Config["content"].(string)
			if in.Content == "" {
				in.Content, _ = in.Config["rules"].(string)
			}
		}
		if in.WebsiteID == 0 || strings.TrimSpace(in.Content) == "" || len(in.Content) > 64<<10 || strings.IndexByte(in.Content, 0) >= 0 {
			writeError(w, http.StatusBadRequest, errors.New("自定义 rewrite 的站点 ID 或内容无效"))
			return
		}
		in.Config["content"] = in.Content
		config, err := svc.UpdateConfig(in.WebsiteID, "rewrite-custom", in.Config)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": config})
	})
	mux.HandleFunc("GET /api/v2/websites/rewrite/custom", func(w http.ResponseWriter, r *http.Request) {
		value := strings.TrimSpace(r.URL.Query().Get("websiteID"))
		if value == "" {
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": svc.ListCustomRewrites()})
			return
		}
		id, err := parseID(value)
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
			// 1Panel 的正式 HTTPS 请求字段是 enable；enabled 是早期
			// WorkMesh 客户端的兼容别名，两者都必须被识别。
			Enable       *bool    `json:"enable"`
			Enabled      *bool    `json:"enabled"`
			Operate      string   `json:"operate"`
			WebsiteSSLID uint     `json:"websiteSSLId"`
			SSLID        uint     `json:"SSLID"`
			HttpConfig   string   `json:"httpConfig"`
			SSLProtocol  []string `json:"SSLProtocol"`
			Algorithm    string   `json:"algorithm"`
			HSTS         *bool    `json:"hsts"`
			HSTSSub      *bool    `json:"hstsIncludeSubDomains"`
			HTTP3        *bool    `json:"http3"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, 400, err)
			return
		}
		enabled := false
		if in.Enable != nil {
			enabled = *in.Enable
		} else if in.Enabled != nil {
			enabled = *in.Enabled
		}
		if in.Operate != "" {
			switch strings.ToLower(strings.TrimSpace(in.Operate)) {
			case "enable":
				enabled = true
			case "disable":
				enabled = false
			default:
				writeError(w, http.StatusBadRequest, errors.New("HTTPS 操作无效"))
				return
			}
		}
		sslID := in.WebsiteSSLID
		if sslID == 0 {
			sslID = in.SSLID
		}
		if _, err := svc.UpdateHTTPS(id, enabled, sslID, in.HttpConfig); errors.Is(err, os.ErrNotExist) {
			writeError(w, 404, err)
			return
		} else if err != nil {
			writeError(w, 400, err)
			return
		}
		cfg, err := svc.GetConfig(id, "https")
		if cfg == nil {
			cfg = map[string]any{}
		}
		if in.HttpConfig == "" && enabled {
			in.HttpConfig = "HTTPToHTTPS"
		}
		// 返回同时包含原版字段和历史兼容字段；前端使用 enable、旧
		// 客户端使用 enabled，二者都反映同一个真实站点状态。
		cfg["enable"], cfg["enabled"] = enabled, enabled
		if !enabled {
			sslID = 0
		}
		cfg["websiteSSLId"], cfg["httpConfig"] = sslID, in.HttpConfig
		if in.SSLProtocol != nil {
			cfg["SSLProtocol"] = in.SSLProtocol
		}
		if in.Algorithm != "" {
			cfg["algorithm"] = in.Algorithm
		}
		if in.HSTS != nil {
			cfg["hsts"] = *in.HSTS
		}
		if in.HSTSSub != nil {
			cfg["hstsIncludeSubDomains"] = *in.HSTSSub
		}
		if in.HTTP3 != nil {
			cfg["http3"] = *in.HTTP3
		}
		cfg, err = svc.UpdateConfig(id, "https", cfg)
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
