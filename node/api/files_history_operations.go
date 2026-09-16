// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// handleFileHistorySearch 查询文件历史版本并保持原分页响应结构。
func handleFileHistorySearch(w http.ResponseWriter, req fileAdvancedRequest) {
	fileAux.Lock()
	loadFileAuxLocked()
	list := append([]fileHistoryItem(nil), fileAux.data.History...)
	fileAux.Unlock()
	scope := strings.ToLower(strings.TrimSpace(req.Scope))
	if scope == "" {
		if strings.TrimSpace(req.Path) != "" {
			scope = "current"
		} else {
			scope = "all"
		}
	}
	path := filepath.Clean(strings.TrimSpace(req.Path))
	operation := strings.TrimSpace(req.Operation)
	filtered := make([]fileHistoryItem, 0, len(list))
	for _, item := range list {
		if item.FileName == "" {
			item.FileName = filepath.Base(item.Path)
		}
		if item.Extension == "" {
			item.Extension = filepath.Ext(item.FileName)
		}
		if item.Operation == "" {
			item.Operation = "save"
		}
		if item.CurrentPath == "" {
			item.CurrentPath = item.Path
		}
		if item.ContentSize == 0 {
			item.ContentSize = int64(len(item.Content))
		}
		if item.UpdatedAt.IsZero() {
			item.UpdatedAt = item.CreatedAt
		}
		if scope == "current" && path != "." && filepath.Clean(item.Path) != path && filepath.Clean(item.CurrentPath) != path {
			continue
		}
		if scope != "current" && scope != "all" {
			continue
		}
		if operation != "" && !strings.EqualFold(item.Operation, operation) {
			continue
		}
		filtered = append(filtered, item)
	}
	page, size := req.Page, req.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	start := (page - 1) * size
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + size
	if end > len(filtered) {
		end = len(filtered)
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": filtered[start:end], "total": len(filtered)}})
}

// handleFileHistoryContent 返回指定历史版本的真实内容记录。
func handleFileHistoryContent(w http.ResponseWriter, req fileAdvancedRequest) {
	fileAux.Lock()
	loadFileAuxLocked()
	var found *fileHistoryItem
	for i := range fileAux.data.History {
		if fileAux.data.History[i].ID == string(req.ID) {
			found = &fileAux.data.History[i]
			break
		}
	}
	fileAux.Unlock()
	if found == nil {
		fileError(w, 404, errors.New("历史版本不存在"))
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": found})
}

// handleFileHistoryDelete 删除一个或多个历史版本并持久化结果。
func handleFileHistoryDelete(w http.ResponseWriter, req fileAdvancedRequest) {
	fileAux.Lock()
	loadFileAuxLocked()
	kept := fileAux.data.History[:0]
	removed := false
	ids := map[string]bool{}
	for _, id := range req.IDs {
		ids[string(id)] = true
	}
	if len(ids) == 0 && string(req.ID) != "" {
		ids[string(req.ID)] = true
	}
	for _, item := range fileAux.data.History {
		if ids[item.ID] {
			removed = true
		} else {
			kept = append(kept, item)
		}
	}
	fileAux.data.History = kept
	err := saveFileAuxLocked()
	fileAux.Unlock()
	if err != nil {
		fileError(w, 500, err)
		return
	}
	if !removed {
		fileError(w, 404, errors.New("历史版本不存在"))
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"deleted": true}})
}

// handleFileHistoryRestore 使用临时文件和原子 rename 恢复历史版本。
func handleFileHistoryRestore(w http.ResponseWriter, req fileAdvancedRequest) {
	fileAux.Lock()
	loadFileAuxLocked()
	var found *fileHistoryItem
	for i := range fileAux.data.History {
		if fileAux.data.History[i].ID == string(req.ID) {
			found = &fileAux.data.History[i]
			break
		}
	}
	fileAux.Unlock()
	if found == nil {
		fileError(w, 404, errors.New("历史版本不存在"))
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(found.Path), ".workmesh-restore-*")
	if err == nil {
		_, err = tmp.WriteString(found.Content)
		_ = tmp.Close()
		if err == nil {
			err = os.Rename(tmp.Name(), found.Path)
		} else {
			_ = os.Remove(tmp.Name())
		}
	}
	if err != nil {
		fileError(w, 500, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"path": found.Path, "restored": true}})
}
