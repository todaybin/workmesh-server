// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import "net/http"

// registerWebsiteExtensionRoutes 注册旧网站模块中未被专用处理器覆盖的真实接口。
func registerWebsiteExtensionRoutes(mux *http.ServeMux) {
	store := newWebsiteExtensionStore()
	registerWebsiteTemplateUploadRoute(mux)
	registerWebsiteTemplateRoutes(mux, newWebsiteTemplateRepository(store.db))
	registerWebsiteExtensionReadRoutes(mux)
	mux.HandleFunc("GET /api/v2/websites/{rest...}", websiteExtensionGetHandler(store))
	mux.HandleFunc("POST /api/v2/websites/{rest...}", websiteExtensionPostHandler(store))
}
