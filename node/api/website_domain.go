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
	"strings"
)

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
		// 原版前端将 res.data 直接作为表格数组使用，不能套分页对象。
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
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
		if r.PathValue("second") == "lbs" {
			id, err := parseID(r.PathValue("first"))
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if _, err := svc.Get(id); err != nil {
				writeError(w, http.StatusNotFound, err)
				return
			}
			cfg, err := svc.GetConfig(id, "lbs")
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			// Return the NginxUpstream shape consumed by the frontend. Older
			// records stored only `upstreams`; newer writes retain the original
			// name/algorithm/servers fields and are returned losslessly.
			if servers, ok := cfg["servers"].([]any); ok || cfg["name"] != nil || cfg["algorithm"] != nil {
				if !ok {
					servers = []any{}
				}
				if len(servers) == 0 {
					if raw, exists := cfg["upstreams"].([]any); exists {
						for _, item := range raw {
							server, _ := item.(map[string]any)
							address := fmt.Sprint(server["server"])
							if address == "<nil>" || address == "" {
								address = fmt.Sprint(server["address"])
							}
							servers = append(servers, map[string]any{"server": address, "weight": server["weight"], "failTimeout": server["failTimeout"], "failTimeoutUnit": server["failTimeoutUnit"], "maxFails": server["maxFails"], "maxConns": server["maxConns"], "flag": server["flag"]})
						}
					}
				}
				item := map[string]any{"websiteID": id, "name": cfg["name"], "algorithm": cfg["algorithm"], "servers": servers}
				wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": []map[string]any{item}})
				return
			}
			upstreams := cfg["upstreams"]
			if upstreams == nil {
				upstreams = []any{}
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": upstreams})
			return
		}
		if r.PathValue("first") == "resource" {
			id, err := parseID(r.PathValue("second"))
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			website, err := svc.Get(id)
			if err != nil {
				writeError(w, http.StatusNotFound, err)
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
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": resources})
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
