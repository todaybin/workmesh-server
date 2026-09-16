// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"strings"

	"github.com/todaybin/workmesh-server/node/service"
)

// registerWebsiteExtensionReadRoutes 注册网站扩展中的只读和默认页面接口。
// 这些路径必须显式注册，避免被网站详情通配符拦截。
func registerWebsiteExtensionReadRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/websites/databases", handleWebsiteExtensionDatabases)
	mux.HandleFunc("GET /api/v2/websites/default/html/{type}", handleWebsiteDefaultHTML)
	mux.HandleFunc("POST /api/v2/websites/default/html/update", handleWebsiteDefaultHTMLUpdate)
}

// handleWebsiteExtensionDatabases 返回真实数据库记录的前端兼容字段。
func handleWebsiteExtensionDatabases(w http.ResponseWriter, r *http.Request) {
	items := service.NewDatabaseService(nil).Search(r.Context(), "", "")
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		databaseName := item.InitialDB
		if databaseName == "" {
			databaseName = item.Name
		}
		out = append(out, map[string]any{"id": item.ID, "type": item.Type, "name": item.Name, "databaseName": databaseName, "from": item.From, "host": item.Host, "port": item.Port})
	}
	extensionJSON(w, out)
}

// handleWebsiteDefaultHTML 读取指定类型的默认页面内容。
func handleWebsiteDefaultHTML(w http.ResponseWriter, r *http.Request) {
	item, err := service.NewWebsiteService("").DefaultHTML(r.PathValue("type"))
	if err != nil {
		extensionError(w, http.StatusBadRequest, err)
		return
	}
	extensionJSON(w, item)
}

// handleWebsiteDefaultHTMLUpdate 保存默认页面并按请求决定是否同步真实站点文件。
func handleWebsiteDefaultHTMLUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Type    string `json:"type"`
		Content string `json:"content"`
		Sync    bool   `json:"sync"`
	}
	if err := decodeJSON(r, &in); err != nil {
		extensionError(w, http.StatusBadRequest, err)
		return
	}
	// 旧客户端省略 type 时，按静态首页 index 处理；其他类型仍由服务层严格校验。
	if strings.TrimSpace(in.Type) == "" {
		in.Type = "index"
	}
	item, err := service.NewWebsiteService("").UpdateDefaultHTML(in.Type, in.Content, in.Sync)
	if err != nil {
		extensionError(w, http.StatusBadRequest, err)
		return
	}
	extensionJSON(w, item)
}
