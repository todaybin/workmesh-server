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

// websiteDirRead 读取站点真实目录配置。
func websiteDirRead(svc *service.WebsiteService, w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID        uint `json:"id"`
		WebsiteID uint `json:"websiteID"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.WebsiteID == 0 {
		in.WebsiteID = in.ID
	}
	result, err := svc.WebsiteDirConfig(in.WebsiteID)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, 404, err)
		return
	}
	if err != nil {
		writeError(w, 400, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": result})
}

// websiteDirUpdate 更新站点目录并持久化到 SQLite。
func websiteDirUpdate(svc *service.WebsiteService, w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID        uint   `json:"id"`
		WebsiteID uint   `json:"websiteID"`
		Dir       string `json:"dir"`
		SiteDir   string `json:"siteDir"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.WebsiteID == 0 {
		in.WebsiteID = in.ID
	}
	if in.Dir == "" {
		in.Dir = in.SiteDir
	}
	result, err := svc.UpdateWebsiteDir(in.WebsiteID, in.Dir)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, 404, err)
		return
	}
	if err != nil {
		writeError(w, 400, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": result})
}

// websiteDirPermission 更新站点目录运行用户和用户组。
func websiteDirPermission(svc *service.WebsiteService, w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID        uint   `json:"id"`
		WebsiteID uint   `json:"websiteID"`
		User      string `json:"user"`
		Group     string `json:"userGroup"`
		UserGroup string `json:"group"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.WebsiteID == 0 {
		in.WebsiteID = in.ID
	}
	if in.Group == "" {
		in.Group = in.UserGroup
	}
	if err := svc.UpdateWebsiteDirPermission(in.WebsiteID, in.User, in.Group); err != nil {
		writeError(w, 400, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"updated": true}})
}

// websiteRewriteRead 读取站点当前伪静态规则。
func websiteRewriteRead(svc *service.WebsiteService, w http.ResponseWriter, r *http.Request) {
	var in struct {
		WebsiteID uint   `json:"websiteID"`
		ID        uint   `json:"id"`
		Name      string `json:"name"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.WebsiteID == 0 {
		in.WebsiteID = in.ID
	}
	if in.Name == "" {
		in.Name = "current"
	}
	result, err := svc.GetRewrite(in.WebsiteID, in.Name)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, 404, err)
		return
	}
	if err != nil {
		writeError(w, 400, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": result})
}

// websiteRewriteUpdate 写入站点伪静态规则。
func websiteRewriteUpdate(svc *service.WebsiteService, w http.ResponseWriter, r *http.Request) {
	var in struct {
		WebsiteID uint   `json:"websiteID"`
		ID        uint   `json:"id"`
		Name      string `json:"name"`
		Content   string `json:"content"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.WebsiteID == 0 {
		in.WebsiteID = in.ID
	}
	if err := svc.UpdateRewrite(in.WebsiteID, in.Name, in.Content); errors.Is(err, os.ErrNotExist) {
		writeError(w, 404, err)
		return
	} else if err != nil {
		writeError(w, 400, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"updated": true}})
}

type websiteConfigWriteRequest struct {
	WebsiteID   uint             `json:"websiteID"`
	ID          uint             `json:"id"`
	Config      map[string]any   `json:"config"`
	Content     string           `json:"content"`
	Enable      *bool            `json:"enable"`
	Enabled     *bool            `json:"enabled"`
	Open        *bool            `json:"open"`
	IPFrom      string           `json:"ipFrom"`
	IPHeader    string           `json:"ipHeader"`
	IPOther     string           `json:"ipOther"`
	Trusted     any              `json:"trusted"`
	Domains     string           `json:"domains"`
	ServerNames []string         `json:"serverNames"`
	Cache       *bool            `json:"cache"`
	CacheTime   int              `json:"cacheTime"`
	CacheUnit   string           `json:"cacheUnit"`
	CacheUint   string           `json:"cacheUint"`
	NoneRef     *bool            `json:"noneRef"`
	Blocked     *bool            `json:"blocked"`
	LogEnable   *bool            `json:"logEnable"`
	Extends     string           `json:"extends"`
	Return      string           `json:"return"`
	StreamPorts string           `json:"streamPorts"`
	UDP         *bool            `json:"udp"`
	Algorithm   string           `json:"algorithm"`
	Servers     []map[string]any `json:"servers"`
	Name        string           `json:"name"`
}

// websiteConfigWriteType 解析通用网站配置请求，并按配置类型分发到对应写入逻辑。
func websiteConfigWriteType(svc *service.WebsiteService, typ string, w http.ResponseWriter, r *http.Request) {
	var in websiteConfigWriteRequest
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.WebsiteID == 0 {
		in.WebsiteID = in.ID
	}
	if in.Config == nil && (typ == "realip" || typ == "leech" || typ == "hotlink") {
		in.Config = map[string]any{}
		if in.Enable != nil {
			in.Config["enable"] = *in.Enable
		}
		if in.Enabled != nil {
			in.Config["enabled"] = *in.Enabled
		}
		if in.Open != nil {
			in.Config["open"] = *in.Open
		}
		if in.IPFrom != "" {
			in.Config["ipFrom"] = in.IPFrom
		}
		if in.IPHeader != "" {
			in.Config["ipHeader"] = in.IPHeader
		}
		if in.IPOther != "" {
			in.Config["ipOther"] = in.IPOther
		}
		if in.Trusted != nil {
			in.Config["trusted"] = in.Trusted
		}
		if in.Domains != "" {
			in.Config["domains"] = in.Domains
		}
		if in.ServerNames != nil {
			in.Config["serverNames"] = in.ServerNames
		}
		if in.Cache != nil {
			in.Config["cache"] = *in.Cache
		}
		if in.CacheTime != 0 {
			in.Config["cacheTime"] = in.CacheTime
		}
		if in.CacheUnit != "" {
			in.Config["cacheUnit"] = in.CacheUnit
		}
		if in.CacheUint != "" {
			in.Config["cacheUint"] = in.CacheUint
		}
		if in.NoneRef != nil {
			in.Config["noneRef"] = *in.NoneRef
		}
		if in.Blocked != nil {
			in.Config["blocked"] = *in.Blocked
		}
		if in.LogEnable != nil {
			in.Config["logEnable"] = *in.LogEnable
		}
		if in.Extends != "" {
			in.Config["extends"] = in.Extends
		}
		if in.Return != "" {
			in.Config["return"] = in.Return
		}
		// A read request contains only the website ID. Do not overwrite the
		// existing setting with an empty object.
		if len(in.Config) == 0 {
			cfg, err := svc.GetConfig(in.WebsiteID, typ)
			if err != nil {
				writeError(w, 404, err)
				return
			}
			wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": cfg})
			return
		}
	}
	if typ == "lbs" {
		value := in.Config
		if value == nil {
			value = map[string]any{}
		}
		// 1Panel sends LoadBalanceReq as name/algorithm/servers, while the
		// renderer stores its normalized upstream representation. Preserve both
		// forms so subsequent GET requests can round-trip the frontend model.
		if in.Name != "" {
			value["name"] = in.Name
		}
		if in.Algorithm != "" {
			value["algorithm"] = in.Algorithm
		}
		if in.Servers != nil {
			upstreams := make([]any, 0, len(in.Servers))
			for _, server := range in.Servers {
				item := map[string]any{}
				for key, raw := range server {
					item[key] = raw
				}
				if address := fmt.Sprint(server["server"]); address != "" && address != "<nil>" {
					item["address"] = address
				}
				if in.Name != "" {
					item["name"] = in.Name
				}
				upstreams = append(upstreams, item)
			}
			value["upstreams"] = upstreams
		}
		in.Config = value
	}
	if writeWebsiteStreamConfig(svc, typ, in, w) {
		return
	}
	if writeWebsiteRedirectRead(svc, typ, r.URL.Path, in.WebsiteID, w) {
		return
	}
	writeWebsiteGenericConfig(svc, typ, r.URL.Path, in, w)
}

// writeWebsiteStreamConfig 将流式站点请求转换为真实 stream 配置并写入。
func writeWebsiteStreamConfig(svc *service.WebsiteService, typ string, in websiteConfigWriteRequest, w http.ResponseWriter) bool {
	if typ != "stream" {
		return false
	}
	site, siteErr := svc.Get(in.WebsiteID)
	if siteErr != nil || !strings.EqualFold(site.Type, "stream") {
		return false
	}
	value := in.Config
	if value == nil {
		value = map[string]any{}
	}
	ports := in.StreamPorts
	if ports == "" {
		ports = fmt.Sprint(value["streamPorts"])
		if ports == "<nil>" {
			ports = fmt.Sprint(value["ports"])
		}
	}
	udp := false
	if in.UDP != nil {
		udp = *in.UDP
	} else if raw, ok := value["udp"].(bool); ok {
		udp = raw
	}
	algorithm := in.Algorithm
	if algorithm == "" {
		algorithm = fmt.Sprint(value["algorithm"])
	}
	servers := in.Servers
	if servers == nil {
		servers, _ = value["servers"].([]map[string]any)
		if servers == nil {
			if encoded, marshalErr := json.Marshal(value["servers"]); marshalErr == nil {
				_ = json.Unmarshal(encoded, &servers)
			}
		}
	}
	result, err := svc.UpdateStream(in.WebsiteID, ports, udp, algorithm, servers)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, 404, err)
		return true
	}
	if err != nil {
		writeError(w, 400, err)
		return true
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": result})
	return true
}

// writeWebsiteRedirectRead 兼容重定向同一路径承担读取配置的历史契约。
func writeWebsiteRedirectRead(svc *service.WebsiteService, typ, path string, websiteID uint, w http.ResponseWriter) bool {
	if typ != "redirect" || path != "/api/v2/websites/redirect" {
		return false
	}
	cfg, err := svc.GetConfig(websiteID, typ)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, err)
		return true
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return true
	}
	if len(cfg) == 0 || strings.TrimSpace(fmt.Sprint(cfg["content"])) == "" {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "message": "", "data": nil})
		return true
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "message": "", "data": cfg})
	return true
}

// writeWebsiteGenericConfig 按原有 v2 契约写入普通网站配置。
func writeWebsiteGenericConfig(svc *service.WebsiteService, typ, path string, in websiteConfigWriteRequest, w http.ResponseWriter) {
	value := in.Config
	if value == nil {
		value = map[string]any{"content": in.Content}
	}
	if typ == "proxy" && path == "/api/v2/websites/proxy/clear" {
		value["enabled"] = false
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
