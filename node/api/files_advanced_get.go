// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"strings"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// handleFileAdvancedGet 处理无需 JSON 请求体的文件高级查询。
func handleFileAdvancedGet(w http.ResponseWriter, r *http.Request, path string) {
	switch path {
	case "read/website":
		handleWebsiteFileRead(w, fileAdvancedRequest{ID: flexibleID(r.URL.Query().Get("id")), Path: r.URL.Query().Get("path")})
	case "recycle/status":
		handleFileRecycleStatus(w)
	case "share/check", "share/info", "share/qrcode":
		handleFileShareGet(w, r, path)
	case "share/download":
		handleFileShareDownload(w, r)
	case "wget/process", "wget/process/keys":
		handleFileWgetProcessQuery(w, r, path)
	default:
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": path, "operation": path}})
	}
}

// handleFileWgetProcessQuery 返回下载任务快照或任务键列表。
func handleFileWgetProcessQuery(w http.ResponseWriter, r *http.Request, path string) {
	if path == "wget/process" && strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		handleWgetProgressStream(w, r)
		return
	}
	initFileWgetState()
	fileWgetState.RLock()
	items := make([]fileWgetProcess, 0, len(fileWgetState.items))
	keys := make([]string, 0, len(fileWgetState.items))
	for key, item := range fileWgetState.items {
		if path == "wget/process" && r.URL.Query().Get("key") != "" && key != r.URL.Query().Get("key") {
			continue
		}
		items = append(items, *item)
		keys = append(keys, key)
	}
	fileWgetState.RUnlock()
	if path == "wget/process/keys" {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"keys": keys}})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
}
