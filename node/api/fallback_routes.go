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

// fallbackRouteHandler 对未实现的 v2 契约返回明确错误，不保存请求体或伪造成功。
func fallbackRouteHandler(w http.ResponseWriter, r *http.Request) {
	wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{
		"code":    "ERR",
		"details": map[string]any{"errCode": "NOT_IMPLEMENTED", "method": r.Method, "path": r.URL.Path},
		"message": "该 v2 功能尚未实现",
	})
}
