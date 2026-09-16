// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// handleFileCompress 保持原文件压缩接口的参数、任务日志和响应契约。
func handleFileCompress(w http.ResponseWriter, req fileAdvancedRequest) {
	var sources []string
	_ = json.Unmarshal(req.Files, &sources)
	if len(sources) == 0 && req.Path != "" {
		sources = []string{req.Path}
	}
	if len(sources) == 0 || req.Dst == "" || req.Name == "" {
		fileError(w, 400, errors.New("files、dst 和 name 不能为空"))
		return
	}
	destination := filepath.Join(req.Dst, req.Name)
	if !req.Replace {
		if _, statErr := os.Stat(destination); statErr == nil {
			fileError(w, http.StatusConflict, errors.New("压缩目标已存在"))
			return
		}
	}
	taskID := strings.TrimSpace(string(req.TaskID))
	if taskID == "" {
		taskID = idToken()
	}
	ctx, err := startFileAsyncTask(taskID, "compress", "开始压缩")
	if err != nil {
		fileError(w, http.StatusConflict, err)
		return
	}
	go func() {
		var archiveErr error
		if strings.EqualFold(req.Type, "tar.gz") || strings.EqualFold(req.Type, "tgz") {
			archiveErr = tarGzipPathsContext(ctx, sources, destination)
		} else {
			archiveErr = zipPathsContext(ctx, sources, destination)
		}
		if archiveErr != nil {
			fileTaskError(taskID, archiveErr)
			return
		}
		fileTaskSuccess(taskID, "压缩完成")
	}()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": destination, "taskID": taskID, "status": "queued"}})
}

// handleFileDecompress 保持原文件解压接口的类型选择和响应契约。
func handleFileDecompress(w http.ResponseWriter, req fileAdvancedRequest) {
	if req.Path == "" || req.Dst == "" {
		fileError(w, 400, errors.New("path 和 dst 不能为空"))
		return
	}
	taskID := strings.TrimSpace(string(req.TaskID))
	if taskID == "" {
		taskID = idToken()
	}
	ctx, err := startFileAsyncTask(taskID, "decompress", "开始解压")
	if err != nil {
		fileError(w, http.StatusConflict, err)
		return
	}
	go func() {
		var extractErr error
		if strings.EqualFold(req.Type, "tar.gz") || strings.EqualFold(req.Type, "tgz") {
			extractErr = untarGzipPathContext(ctx, req.Path, req.Dst)
		} else {
			extractErr = unzipPathContext(ctx, req.Path, req.Dst)
		}
		if extractErr != nil {
			fileTaskError(taskID, extractErr)
			return
		}
		fileTaskSuccess(taskID, "解压完成")
	}()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": req.Dst, "taskID": taskID, "status": "queued"}})
}
