// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func handleWebsiteFileRead(w http.ResponseWriter, req fileAdvancedRequest) {
	idText := strings.TrimSpace(string(req.ID))
	id, err := strconv.ParseUint(idText, 10, 32)
	if err != nil || id == 0 {
		fileError(w, http.StatusBadRequest, errors.New("网站 ID 无效"))
		return
	}
	site, err := service.NewWebsiteService("").Get(uint(id))
	if err != nil {
		fileError(w, http.StatusNotFound, errors.New("网站不存在"))
		return
	}
	root := filepath.Clean(site.SiteDir)
	target := root
	if requested := strings.TrimSpace(req.Path); requested != "" {
		if filepath.IsAbs(requested) {
			target = filepath.Clean(requested)
		} else {
			target = filepath.Join(root, requested)
		}
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		fileError(w, http.StatusForbidden, errors.New("路径必须位于网站目录内"))
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		fileError(w, http.StatusNotFound, err)
		return
	}
	if info.IsDir() {
		entries, err := os.ReadDir(target)
		if err != nil {
			fileError(w, http.StatusForbidden, err)
			return
		}
		items := make([]map[string]any, 0, len(entries))
		for _, entry := range entries {
			entryInfo, statErr := entry.Info()
			if statErr != nil {
				continue
			}
			items = append(items, map[string]any{"name": entry.Name(), "path": filepath.Join(target, entry.Name()), "isDir": entry.IsDir(), "size": entryInfo.Size(), "modTime": entryInfo.ModTime()})
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"websiteID": id, "root": root, "path": target, "isDir": true, "items": items}})
		return
	}
	if info.Size() > 4<<20 {
		fileError(w, http.StatusRequestEntityTooLarge, errors.New("文件超过 4 MiB 读取限制"))
		return
	}
	data, err := os.ReadFile(target)
	if err != nil {
		fileError(w, http.StatusForbidden, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"websiteID": id, "root": root, "path": target, "isDir": false, "content": string(data), "size": info.Size()}})
}

// handleFileAISearch 在指定目录内执行受限内容搜索，避免将搜索请求转发为占位响应。
