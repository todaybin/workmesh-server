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

// handleFileAdvancedOperation 将写操作路由到已有领域处理器。
func handleFileAdvancedOperation(w http.ResponseWriter, r *http.Request, path string, req fileAdvancedRequest) bool {
	switch path {
	case "read/website":
		handleWebsiteFileRead(w, req)
	case "check":
		handleFilePathCheck(w, req)
	case "batch/check", "batch/del", "batch/role":
		handleFileBatchOperation(w, path, req)
	case "ai-search":
		handleFileAISearch(w, r, req)
	case "wget":
		handleFileWget(w, r, req)
	case "wget/stop":
		handleFileWgetStop(w, req)
	case "favorite":
		handleFileFavoriteCreate(w, req)
	case "favorite/del":
		handleFileFavoriteDelete(w, req)
	case "compress":
		handleFileCompress(w, req)
	case "decompress":
		handleFileDecompress(w, req)
	case "share/create":
		handleFileShareCreate(w, req)
	case "share/del":
		handleFileShareDelete(w, req)
	case "share/search":
		handleFileShareSearch(w)
	case "recycle/search", "favorite/search", "upload/search":
		handleFileAuxSearch(w, path, req)
	case "recycle/clear":
		handleFileRecycleClear(w)
	case "recycle/reduce":
		handleFileRecycleReduce(w, req)
	case "compress/stop", "decompress/stop", "move/stop":
		taskID := strings.TrimSpace(string(req.TaskID))
		if taskID == "" {
			fileError(w, http.StatusBadRequest, errors.New("taskID 不能为空"))
			return true
		}
		kind := strings.TrimSuffix(path, "/stop")
		if err := stopFileAsyncTask(taskID, kind); err != nil {
			fileError(w, http.StatusNotFound, err)
			return true
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"taskID": taskID, "stopped": true}})
	case "chunkupload/stop":
		handleFileChunkUploadStop(w, req)
	case "depth/size":
		handleFileDepthSize(w, req)
	case "mode":
		handleFileMode(w, req)
	case "owner":
		handleFileOwner(w, req)
	case "preview", "content":
		handleFilePreviewContent(w, req)
	case "read":
		handleFileRead(w, req)
	case "remarks":
		handleFileRemarks(w, req)
	case "remark":
		handleFileRemark(w, req)
	case "history/search":
		handleFileHistorySearch(w, req)
	case "history/content":
		handleFileHistoryContent(w, req)
	case "history/del":
		handleFileHistoryDelete(w, req)
	case "history/restore":
		handleFileHistoryRestore(w, req)
	case "share/detail":
		handleFileShareDetail(w, req)
	case "mount":
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": listAlertDisks()})
	case "user/group":
		handleFileUserGroup(w)
	case "convert":
		handleFileConvert(w, req)
	case "convert/log":
		handleFileConvertLog(w, r)
	default:
		return false
	}
	return true
}

// handleFilePathCheck 检查授权路径，并按 withInit 创建缺失目录。
func handleFilePathCheck(w http.ResponseWriter, req fileAdvancedRequest) {
	clean, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, http.StatusBadRequest, err)
		return
	}
	_, statErr := os.Stat(clean)
	exists := statErr == nil
	if !exists && req.WithInit {
		if err := os.MkdirAll(clean, 0o750); err != nil {
			fileError(w, http.StatusInternalServerError, err)
			return
		}
		exists = true
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"exist": exists, "path": clean}})
}

// handleFileWgetStop 取消指定下载任务并更新可查询状态。
func handleFileWgetStop(w http.ResponseWriter, req fileAdvancedRequest) {
	key := strings.TrimSpace(string(req.ID))
	if key == "" {
		key = strings.TrimSpace(req.Token)
	}
	if key == "" {
		key = strings.TrimSpace(req.Key)
	}
	if key == "" {
		fileError(w, http.StatusBadRequest, errors.New("下载任务 key 不能为空"))
		return
	}
	initFileWgetState()
	fileWgetState.Lock()
	cancel, item := fileWgetState.cancel[key], fileWgetState.items[key]
	if cancel != nil {
		cancel()
	}
	if item != nil {
		item.Status = "cancelled"
	}
	fileWgetState.Unlock()
	if cancel == nil {
		fileError(w, http.StatusNotFound, errors.New("下载任务不存在"))
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"key": key, "stopped": true}})
}

// handleFileChunkUploadStop 清理指定分片上传的临时目录。
func handleFileChunkUploadStop(w http.ResponseWriter, req fileAdvancedRequest) {
	id := strings.TrimSpace(req.UploadID)
	if id == "" {
		id = strings.TrimSpace(string(req.ID))
	}
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\`) {
		fileError(w, http.StatusBadRequest, errors.New("uploadID 无效"))
		return
	}
	if err := os.RemoveAll(filepath.Join(fileChunkDir(), id)); err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"stopped": true, "uploadID": id}})
}

// handleFileDepthSize 汇总目录直属子项的递归文件大小。
func handleFileDepthSize(w http.ResponseWriter, req fileAdvancedRequest) {
	clean, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, http.StatusBadRequest, err)
		return
	}
	info, err := os.Stat(clean)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("路径不是目录")
		}
		fileError(w, http.StatusNotFound, err)
		return
	}
	entries, _ := os.ReadDir(clean)
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(clean, entry.Name())
		var size int64
		_ = filepath.Walk(path, func(_ string, item os.FileInfo, walkErr error) error {
			if walkErr == nil && item != nil && !item.IsDir() {
				size += item.Size()
			}
			return nil
		})
		items = append(items, map[string]any{"path": path, "name": entry.Name(), "size": size, "isDir": entry.IsDir()})
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": items})
}
