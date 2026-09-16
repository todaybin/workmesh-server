// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"strings"
)

// registerRuntimeSubroutes 注册运行时扩展、PHP 配置和节点子路由。
func registerRuntimeSubroutes(mux *http.ServeMux, s *runtimeStore) {
	registerNodeRuntimeRoutes(mux, s)
	registerPHPExtensionOperationRoutes(mux, s)
	registerPHPConfigurationRoutes(mux, s)
	registerPHPSupervisorRoutes(mux, s)
	registerPHPExtensionTemplateRoutes(mux, s)
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/search", phpExtensionSearchHandler(s))
	mux.HandleFunc("/api/v2/runtimes/php/", phpExtensionListHandler(s))
}

// runtimeContractPaths documents the concrete dynamic endpoint handled by
// the shared /runtimes/php/ dispatcher.  It is consumed by the static route
// audit and keeps the no-slash URL from being redirected by ServeMux.
var runtimeContractPaths = []string{"GET /api/v2/runtimes/php/{id}/extensions"}

// phpExtensionSearchHandler 返回 PHP 扩展模板分页搜索处理器。
func phpExtensionSearchHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		name := strings.ToLower(runtimeString(body, "name", "search"))
		s.mu.RLock()
		templates, templateErr := phpExtensionTemplatesLocked(s)
		s.mu.RUnlock()
		if templateErr != nil {
			runtimeErr(w, http.StatusInternalServerError, templateErr.Error())
			return
		}
		items := make([]phpExtensionTemplate, 0, len(templates))
		for _, template := range templates {
			if name != "" && !strings.Contains(strings.ToLower(template.Name), name) {
				continue
			}
			items = append(items, template)
		}
		page, pageSize, pageErr := runtimePage(body)
		if pageErr != nil {
			runtimeErr(w, http.StatusBadRequest, pageErr.Error())
			return
		}
		total := len(items)
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		if all, _ := body["all"].(bool); all {
			start, end = 0, total
		}
		if items == nil {
			items = []phpExtensionTemplate{}
		}
		runtimeOK(w, map[string]any{"items": items[start:end], "total": total, "page": page, "pageSize": pageSize})
	}
}

// phpExtensionListHandler 返回指定 PHP 运行时扩展查询处理器。
func phpExtensionListHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/runtimes/php/"), "/"), "/")
		if id := r.PathValue("id"); id != "" {
			parts = []string{id, "extensions"}
		}
		if len(parts) >= 2 && parts[0] != "" {
			id := parts[0]
			s.mu.RLock()
			var rec *runtimeRecord
			for i := range s.state.Runtimes {
				if s.state.Runtimes[i].ID == id {
					copy := s.state.Runtimes[i]
					rec = &copy
					break
				}
			}
			s.mu.RUnlock()
			if rec == nil {
				runtimeErr(w, 404, "runtime not found")
				return
			}
			if parts[1] == "extensions" {
				exts := append([]string(nil), rec.Extensions...)
				if strings.TrimSpace(rec.Container) != "" {
					var err error
					exts, err = phpInstalledExtensions(s.commandExecutor(), *rec)
					if err != nil {
						runtimeErr(w, http.StatusBadGateway, "读取 PHP 扩展失败: "+err.Error())
						return
					}
				}
				installed := make(map[string]bool, len(exts))
				for _, extension := range exts {
					installed[strings.ToLower(strings.TrimSpace(extension))] = true
				}
				support := make([]map[string]any, 0, len(phpExtensionCatalog))
				for _, definition := range phpExtensionCatalog {
					support = append(support, definition.toMap(installed[strings.ToLower(definition.Check)]))
				}
				runtimeOK(w, map[string]any{"id": id, "extensions": exts, "supportExtensions": support, "total": len(exts), "status": rec.Status})
				return
			}
		}
		runtimeErr(w, 404, "运行时路径不存在")
	}
}
