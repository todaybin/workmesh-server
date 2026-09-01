// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

func registerFallbackRoute(mux *http.ServeMux, pattern string) {
	mux.HandleFunc(pattern, fallbackRouteHandler)
}

// fallbackRouteHandler 对未接入专用领域模型的契约返回明确错误，不保存请求体或伪造成功。
func fallbackRouteHandler(w http.ResponseWriter, r *http.Request) {
	wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]any{"errCode": "MIGRATION_PENDING", "method": r.Method, "path": r.URL.Path}, "message": "该功能尚未接入真实业务存储"})
}
