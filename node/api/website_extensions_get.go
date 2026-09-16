// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/todaybin/workmesh-server/node/service"
)

// websiteExtensionGetHandler 返回兼容网站数据库、证书、负载均衡和资源查询的统一处理器。
func websiteExtensionGetHandler(store *websiteExtensionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/websites/"), "/")
		parts := strings.Split(rest, "/")
		store.mu.Lock()
		defer store.mu.Unlock()
		switch {
		case rest == "databases":
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
		case len(parts) == 2 && parts[0] == "ca":
			for _, item := range store.ACME {
				if fmt.Sprint(item["id"]) == parts[1] {
					extensionJSON(w, item)
					return
				}
			}
			extensionError(w, http.StatusNotFound, errors.New("证书账户不存在"))
		case len(parts) == 3 && parts[0] == "default" && parts[1] == "html":
			item, itemErr := service.NewWebsiteService("").DefaultHTML(parts[2])
			if itemErr != nil {
				extensionError(w, http.StatusBadRequest, itemErr)
				return
			}
			extensionJSON(w, item)
		case len(parts) == 2 && parts[1] == "lbs":
			id, err := parseID(parts[0])
			if err != nil {
				extensionError(w, http.StatusBadRequest, err)
				return
			}
			svc := service.NewWebsiteService("")
			if _, err := svc.Get(id); err != nil {
				extensionError(w, http.StatusNotFound, err)
				return
			}
			cfg, err := svc.GetConfig(id, "lbs")
			if err != nil {
				extensionError(w, http.StatusInternalServerError, err)
				return
			}
			upstreams := cfg["upstreams"]
			if upstreams == nil {
				upstreams = []any{}
			}
			extensionJSON(w, upstreams)
		case len(parts) == 2 && parts[0] == "resource":
			id, err := parseID(parts[1])
			if err != nil {
				extensionError(w, http.StatusBadRequest, err)
				return
			}
			svc := service.NewWebsiteService("")
			website, err := svc.Get(id)
			if err != nil {
				extensionError(w, http.StatusNotFound, err)
				return
			}
			domains, _ := svc.ListDomains(id)
			resources := []map[string]any{{"name": website.PrimaryDomain, "type": "website", "resourceID": website.ID, "detail": website}}
			for _, domain := range domains {
				resources = append(resources, map[string]any{"name": domain.Domain, "type": "domain", "resourceID": domain.ID, "detail": domain})
			}
			if strings.TrimSpace(website.RuntimeID) != "" {
				resources = append(resources, map[string]any{"name": website.RuntimeID, "type": "runtime", "resourceID": website.RuntimeID, "detail": map[string]any{"id": website.RuntimeID}})
			}
			if website.DbID > 0 {
				for _, db := range service.NewDatabaseService(nil).Search(r.Context(), "", "") {
					if uint(db.ID) == website.DbID {
						resources = append(resources, map[string]any{"name": db.Name, "type": "database", "resourceID": db.ID, "detail": db})
						break
					}
				}
			}
			extensionJSON(w, resources)
		default:
			extensionError(w, http.StatusNotFound, errors.New("网站扩展接口不存在"))
		}
	}
}
