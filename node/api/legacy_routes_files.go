// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import "net/http"

// registerLegacyFilesRoutes 注册 files 领域尚未迁移的兼容路由。
func registerLegacyFilesRoutes(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v2/files/download", func(w http.ResponseWriter, r *http.Request) {
		handleFilesDownload(w, r)
	})
	mux.HandleFunc("GET /api/v2/files/recycle/status", func(w http.ResponseWriter, r *http.Request) {
		fileAdvancedHandler(w, r)
	})
	mux.HandleFunc("GET /api/v2/files/share/check", func(w http.ResponseWriter, r *http.Request) {
		fileAdvancedHandler(w, r)
	})
	mux.HandleFunc("GET /api/v2/files/share/download", func(w http.ResponseWriter, r *http.Request) {
		fileAdvancedHandler(w, r)
	})
	mux.HandleFunc("GET /api/v2/files/share/info", func(w http.ResponseWriter, r *http.Request) {
		fileAdvancedHandler(w, r)
	})
	mux.HandleFunc("GET /api/v2/files/share/qrcode", func(w http.ResponseWriter, r *http.Request) {
		fileAdvancedHandler(w, r)
	})
	mux.HandleFunc("GET /api/v2/files/wget/process", fallbackRouteHandler)
	mux.HandleFunc("GET /api/v2/files/wget/process/keys", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/ai-search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/batch/check", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/batch/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/batch/role", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/check", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/chunkdownload", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/chunkupload", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/chunkupload/stop", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/compress", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/compress/stop", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/content", func(w http.ResponseWriter, r *http.Request) {
		handleFilesContent(w, r)
	})
	mux.HandleFunc("POST /api/v2/files/convert", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/convert/log", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/decompress", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/decompress/stop", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/del", func(w http.ResponseWriter, r *http.Request) {
		handleFilesDelete(w, r)
	})
	mux.HandleFunc("POST /api/v2/files/depth/size", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/favorite", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/favorite/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/favorite/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/history/content", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/history/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/history/restore", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/history/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/mode", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/mount", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/move", func(w http.ResponseWriter, r *http.Request) {
		handleFilesMove(w, r)
	})
	mux.HandleFunc("POST /api/v2/files/move/stop", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/owner", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/preview", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/read/:type", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/recycle/clear", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/recycle/reduce", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/recycle/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/remark", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/remarks", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/rename", func(w http.ResponseWriter, r *http.Request) {
		handleFilesRename(w, r)
	})
	mux.HandleFunc("POST /api/v2/files/save", func(w http.ResponseWriter, r *http.Request) {
		handleFilesSave(w, r)
	})
	mux.HandleFunc("POST /api/v2/files/search", func(w http.ResponseWriter, r *http.Request) {
		handleFilesSearch(w, r)
	})
	mux.HandleFunc("POST /api/v2/files/share/create", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/share/del", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/share/detail", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/share/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/size", func(w http.ResponseWriter, r *http.Request) {
		handleFilesSize(w, r)
	})
	mux.HandleFunc("POST /api/v2/files/tree", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/upload", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/upload/search", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/user/group", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/wget", fallbackRouteHandler)
	mux.HandleFunc("POST /api/v2/files/wget/stop", fallbackRouteHandler)
}
