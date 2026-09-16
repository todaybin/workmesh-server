// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"

	"github.com/todaybin/workmesh-server/node/service"
)

// registerWebsiteFunctionalRoutes 注册网站、WAF 和 OpenResty 的兼容接口。
func registerWebsiteFunctionalRoutes(mux *http.ServeMux) {
	svc := service.NewWebsiteService("")
	registerWebsiteCertificateRoutes(mux, service.NewWebsiteSecurityService(""))
	registerWebsiteCRUD(mux, svc)
	registerWebsiteAdvancedRoutes(mux, svc)
	registerWebsiteMonitorLogRoutes(mux)
	registerWAFRoutes(mux, svc)
	registerOpenRestyRoutes(mux, svc)
	registerXPackWebsiteAliases(mux, svc)
	registerWebsiteExtensionRoutes(mux)
}

// registerWebsiteAdvancedRoutes 注册站点运行、域名、HTTPS 和配置管理接口。
func registerWebsiteAdvancedRoutes(mux *http.ServeMux, svc *service.WebsiteService) {
	registerWebsiteMonitorAnalyticsAliases(mux)
	registerWebsiteOperateRoute(mux, svc)
	registerWebsiteCheckRoute(mux)
	registerWebsiteOptionsRoute(mux, svc)
	registerDomainRoutes(mux, svc)
	registerWebsiteConfigRoutes(mux, svc)
}
