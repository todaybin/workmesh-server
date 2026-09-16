// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"io"
	"net/http"
	"os"
	"strings"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// handleFilePreviewContent 返回普通文件的完整内容，供 preview/content 共用。
func handleFilePreviewContent(w http.ResponseWriter, req fileAdvancedRequest) {
	clean, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, http.StatusBadRequest, err)
		return
	}
	f, err := os.Open(clean)
	if err != nil {
		fileError(w, http.StatusNotFound, err)
		return
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 2<<20))
	if err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": clean, "content": string(b), "size": len(b)}})
}

// handleFileRead 按页返回普通文本文件内容，保持原字段和分页默认值。
func handleFileRead(w http.ResponseWriter, req fileAdvancedRequest) {
	clean, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, http.StatusBadRequest, err)
		return
	}
	b, err := os.ReadFile(clean)
	if err != nil {
		fileError(w, http.StatusNotFound, err)
		return
	}
	lines := strings.Split(strings.TrimRight(string(b), "\r\n"), "\n")
	page, size := req.Page, req.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 500 {
		size = 100
	}
	start := (page - 1) * size
	if start > len(lines) {
		start = len(lines)
	}
	end := start + size
	if end > len(lines) {
		end = len(lines)
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": clean, "lines": lines[start:end], "totalLines": len(lines), "page": page, "pageSize": size, "end": end >= len(lines)}})
}
