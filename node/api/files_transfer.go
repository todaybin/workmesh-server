// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"path/filepath"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// handleFilesDownload 输出指定文件并设置下载响应头。
func handleFilesDownload(w http.ResponseWriter, r *http.Request) {
	path, err := cleanFilePath(r.URL.Query().Get("path"))
	if err != nil {
		fileError(w, 400, err)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(path)+"\"")
	http.ServeFile(w, r, path)
}

// fileError 输出文件接口统一错误响应。
func fileError(w http.ResponseWriter, status int, err error) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
}
