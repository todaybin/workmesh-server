// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// handleFileShareGet 处理分享查询、二维码和下载的 GET 操作。
func handleFileShareGet(w http.ResponseWriter, r *http.Request, path string) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("code"))
	}
	if token == "" {
		fileError(w, http.StatusBadRequest, errors.New("分享 code/token 不能为空"))
		return
	}
	share, ok := findFileShare(token)
	if path == "share/check" && !ok {
		fileError(w, http.StatusNotFound, errors.New("分享不存在或文件已删除"))
		return
	}
	if path == "share/qrcode" {
		// 当前轻量服务返回可编码的分享 URL，避免引入图像库和额外进程。
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"url": "/s/" + url.PathEscape(token), "token": token, "exists": ok}})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": share.Path, "exists": ok, "token": share.Token, "code": share.Token, "createdAt": share.CreatedAt}})
}

// handleFileShareDownload 下载已存在的分享文件并保持原 Content-Disposition 行为。
func handleFileShareDownload(w http.ResponseWriter, r *http.Request) {
	share, ok := findFileShare(r.URL.Query().Get("token"))
	if !ok {
		fileError(w, http.StatusNotFound, errors.New("分享不存在或文件已删除"))
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(share.Path)+"\"")
	http.ServeFile(w, r, share.Path)
}

// handleFileShareCreate 创建分享记录并持久化真实文件路径。
func handleFileShareCreate(w http.ResponseWriter, req fileAdvancedRequest) {
	clean, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := os.Stat(clean); err != nil {
		fileError(w, http.StatusNotFound, err)
		return
	}
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	share := fileShare{Token: hex.EncodeToString(raw[:]), Path: clean, CreatedAt: time.Now().UTC()}
	fileShareState.Lock()
	loadFileSharesLocked()
	fileShareState.items[share.Token] = share
	err = saveFileSharesLocked()
	fileShareState.Unlock()
	if err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": share})
}

// handleFileShareDelete 删除分享记录并保留真实持久化结果。
func handleFileShareDelete(w http.ResponseWriter, req fileAdvancedRequest) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		fileError(w, http.StatusBadRequest, errors.New("分享 token 不能为空"))
		return
	}
	fileShareState.Lock()
	loadFileSharesLocked()
	if _, ok := fileShareState.items[token]; !ok {
		fileShareState.Unlock()
		fileError(w, http.StatusNotFound, errors.New("分享不存在"))
		return
	}
	delete(fileShareState.items, token)
	err := saveFileSharesLocked()
	fileShareState.Unlock()
	if err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"deleted": true, "token": token}})
}

// handleFileShareSearch 返回仍然存在的分享文件记录。
func handleFileShareSearch(w http.ResponseWriter) {
	fileShareState.Lock()
	loadFileSharesLocked()
	items := make([]fileShare, 0, len(fileShareState.items))
	for _, item := range fileShareState.items {
		if _, err := os.Stat(item.Path); err == nil {
			items = append(items, item)
		}
	}
	fileShareState.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
}

// handleFileShareDetail 返回分享详情，兼容 token 和 code 两种请求字段。
func handleFileShareDetail(w http.ResponseWriter, req fileAdvancedRequest) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		token = strings.TrimSpace(req.Code)
	}
	share, ok := findFileShare(token)
	if !ok {
		fileError(w, http.StatusNotFound, errors.New("分享不存在或文件已删除"))
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": share})
}
