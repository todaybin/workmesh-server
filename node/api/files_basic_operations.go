// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// handleFilesSearch 查询目录内容并按前端请求的字段排序。
func handleFilesSearch(w http.ResponseWriter, r *http.Request) {
	req, err := decodeFileRequest(r)
	if err != nil {
		fileError(w, http.StatusBadRequest, err)
		return
	}
	root, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, http.StatusBadRequest, err)
		return
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		fileError(w, http.StatusNotFound, err)
		return
	}
	if !rootInfo.IsDir() {
		root = filepath.Dir(root)
		rootInfo, err = os.Stat(root)
		if err != nil {
			fileError(w, http.StatusNotFound, err)
			return
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		fileError(w, http.StatusNotFound, err)
		return
	}
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if !req.ShowHidden && strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if strings.TrimSpace(req.Search) != "" && !strings.Contains(strings.ToLower(entry.Name()), strings.ToLower(strings.TrimSpace(req.Search))) {
			continue
		}
		info, e := entry.Info()
		if e == nil {
			items = append(items, fileInfo(filepath.Join(root, entry.Name()), info))
		}
	}
	sortFileItems(items, req.SortBy, req.SortOrder)
	// 原系统返回完整的 FileInfo 根对象，前端依赖 data.path 判断当前目录是否有效。
	result := fileInfo(root, rootInfo)
	result["items"] = items
	result["itemTotal"] = len(items)
	result["total"] = len(items)
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
}

// handleFilesContent 读取文件内容及其元数据，保持原面板响应字段。
func handleFilesContent(w http.ResponseWriter, r *http.Request) {
	req, err := decodeFileRequest(r)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	path, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fileError(w, 404, err)
		return
	}
	info, _ := os.Stat(path)
	result := fileContentResult(path, data, info)
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": result})
}

// fileContentResult 组装文件读取接口的稳定响应结构。
func fileContentResult(path string, data []byte, info os.FileInfo) map[string]any {
	extension := filepath.Ext(path)
	result := map[string]any{
		"path": path, "name": filepath.Base(path), "content": string(data), "size": len(data),
		"extension": extension, "mimeType": mime.TypeByExtension(extension),
	}
	if info != nil {
		result["isDir"] = info.IsDir()
		result["mode"] = info.Mode().Perm()
		result["isSymlink"] = info.Mode()&os.ModeSymlink != 0
	}
	return result
}

// handleFilesSave 原子保存文件，并把旧内容写入历史版本记录。
func handleFilesSave(w http.ResponseWriter, r *http.Request) {
	req, err := decodeFileRequest(r)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	path, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	// 临时文件与目标位于同一目录，确保 rename 在同一文件系统内原子替换。
	if err = os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	recordFileHistory(path)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".workmesh-save-*")
	if err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.WriteString(req.Content)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmpName, path)
	}
	if err != nil {
		fileError(w, 500, err)
		return
	}
	applyWebsiteOwnership(path)
	wmhttp.JSON(w, 200, map[string]any{"code": 200})
}

// recordFileHistory 保存不超过 2 MiB 的旧文件内容，供历史接口恢复。
func recordFileHistory(path string) {
	old, readErr := os.ReadFile(path)
	if readErr != nil || len(old) > 2<<20 {
		return
	}
	fileAux.Lock()
	defer fileAux.Unlock()
	loadFileAuxLocked()
	now := time.Now().UTC()
	fileAux.data.History = append(fileAux.data.History, fileHistoryItem{
		ID: fileAuxID("history"), FileID: path, Path: path, CurrentPath: path,
		FileName: filepath.Base(path), Extension: filepath.Ext(path), FileMode: "",
		Operation: "save", ContentSize: int64(len(old)), Content: string(old), CreatedAt: now, UpdatedAt: now,
	})
	if len(fileAux.data.History) > 200 {
		fileAux.data.History = fileAux.data.History[len(fileAux.data.History)-200:]
	}
	_ = saveFileAuxLocked()
}

// handleFilesCreate 创建文件或目录，并返回新对象的完整元数据。
func handleFilesCreate(w http.ResponseWriter, r *http.Request) {
	req, err := decodeFileRequest(r)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	path, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	if req.IsDir {
		err = os.MkdirAll(path, 0755)
	} else {
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL, 0644)
		if e == nil {
			e = f.Close()
		}
		err = e
	}
	if err != nil {
		fileError(w, 409, err)
		return
	}
	applyWebsiteOwnership(path)
	info, _ := os.Stat(path)
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": fileInfo(path, info)})
}

// handleFilesDelete 删除文件；非强制删除会先移动到可恢复的回收目录。
func handleFilesDelete(w http.ResponseWriter, r *http.Request) {
	req, err := decodeFileRequest(r)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	path, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	if req.ForceDelete {
		err = os.RemoveAll(path)
	} else {
		err = recycleFile(path)
	}
	if err != nil {
		fileError(w, 404, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200})
}

// recycleFile 移动文件到回收目录并记录原路径，支持跨设备目录回退。
func recycleFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	trashRoot := filepath.Join(filepath.Dir(fileAuxPath()), "recycle")
	if err = os.MkdirAll(trashRoot, 0o750); err != nil {
		return err
	}
	trashPath := filepath.Join(trashRoot, fileAuxID("item"))
	err = os.Rename(path, trashPath)
	if errors.Is(err, syscall.EXDEV) {
		localRoot := filepath.Join(filepath.Dir(path), ".workmesh-recycle")
		if err = os.MkdirAll(localRoot, 0o750); err != nil {
			return err
		}
		trashPath = filepath.Join(localRoot, fileAuxID("item"))
		err = os.Rename(path, trashPath)
	}
	if err != nil {
		return err
	}
	fileAux.Lock()
	defer fileAux.Unlock()
	loadFileAuxLocked()
	fileAux.data.Recycle = append(fileAux.data.Recycle, fileRecycleItem{
		ID: fileAuxID("recycle"), OriginalPath: path, TrashPath: trashPath,
		Name: info.Name(), Size: info.Size(), IsDir: info.IsDir(), DeletedAt: time.Now().UTC(),
	})
	return saveFileAuxLocked()
}
