// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// handleFileRecycleStatus 返回真实回收站条目数量。
func handleFileRecycleStatus(w http.ResponseWriter) {
	fileAux.Lock()
	loadFileAuxLocked()
	count := len(fileAux.data.Recycle)
	fileAux.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"enabled": true, "items": count}})
}

// handleFileFavoriteCreate 创建收藏并持久化真实文件路径。
func handleFileFavoriteCreate(w http.ResponseWriter, req fileAdvancedRequest) {
	clean, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, http.StatusBadRequest, err)
		return
	}
	info, err := os.Stat(clean)
	if err != nil {
		fileError(w, http.StatusNotFound, err)
		return
	}
	fileAux.Lock()
	loadFileAuxLocked()
	for _, item := range fileAux.data.Favorites {
		if item.Path == clean {
			fileAux.Unlock()
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
			return
		}
	}
	item := fileFavorite{ID: fileAuxID("favorite"), Path: clean, Name: info.Name(), IsDir: info.IsDir(), CreatedAt: time.Now().UTC()}
	fileAux.data.Favorites = append(fileAux.data.Favorites, item)
	err = saveFileAuxLocked()
	fileAux.Unlock()
	if err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
}

// handleFileFavoriteDelete 删除收藏记录并持久化结果。
func handleFileFavoriteDelete(w http.ResponseWriter, req fileAdvancedRequest) {
	fileAux.Lock()
	loadFileAuxLocked()
	id := strings.TrimSpace(string(req.ID))
	if id == "" {
		id = strings.TrimSpace(req.Token)
	}
	pathValue := strings.TrimSpace(req.Path)
	kept := fileAux.data.Favorites[:0]
	removed := false
	for _, item := range fileAux.data.Favorites {
		if (id != "" && item.ID == id) || (pathValue != "" && item.Path == pathValue) {
			removed = true
			continue
		}
		kept = append(kept, item)
	}
	fileAux.data.Favorites = kept
	err := saveFileAuxLocked()
	fileAux.Unlock()
	if err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	if !removed {
		fileError(w, http.StatusNotFound, errors.New("收藏不存在"))
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"deleted": true}})
}

// handleFileAuxSearch 统一处理收藏、回收站和上传记录的真实查询。
func handleFileAuxSearch(w http.ResponseWriter, path string, req fileAdvancedRequest) {
	items, total, page, size := fileAuxSearch(path, req)
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": total, "page": page, "pageSize": size}})
}

// handleFileRecycleClear 清理回收站文件和真实持久化索引。
func handleFileRecycleClear(w http.ResponseWriter) {
	fileAux.Lock()
	loadFileAuxLocked()
	var firstErr error
	for _, item := range fileAux.data.Recycle {
		if err := os.RemoveAll(item.TrashPath); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	fileAux.data.Recycle = []fileRecycleItem{}
	err := saveFileAuxLocked()
	fileAux.Unlock()
	if firstErr != nil {
		fileError(w, http.StatusInternalServerError, firstErr)
		return
	}
	if err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"cleared": true}})
}

// handleFileRecycleReduce 恢复回收站文件并原子更新索引。
func handleFileRecycleReduce(w http.ResponseWriter, req fileAdvancedRequest) {
	id := strings.TrimSpace(string(req.ID))
	if id == "" {
		id = strings.TrimSpace(req.Token)
	}
	if id == "" {
		id = strings.TrimSpace(req.Path)
	}
	fileAux.Lock()
	loadFileAuxLocked()
	index := -1
	for i, item := range fileAux.data.Recycle {
		if item.ID == id || item.OriginalPath == id {
			index = i
			break
		}
	}
	if index < 0 {
		fileAux.Unlock()
		fileError(w, http.StatusNotFound, errors.New("回收站文件不存在"))
		return
	}
	item := fileAux.data.Recycle[index]
	if _, statErr := os.Stat(item.OriginalPath); statErr == nil {
		fileAux.Unlock()
		fileError(w, http.StatusConflict, errors.New("原路径已存在，无法恢复"))
		return
	}
	if err := os.MkdirAll(filepath.Dir(item.OriginalPath), 0o750); err != nil {
		fileAux.Unlock()
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.Rename(item.TrashPath, item.OriginalPath); err != nil {
		fileAux.Unlock()
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	fileAux.data.Recycle = append(fileAux.data.Recycle[:index], fileAux.data.Recycle[index+1:]...)
	err := saveFileAuxLocked()
	fileAux.Unlock()
	if err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
}
