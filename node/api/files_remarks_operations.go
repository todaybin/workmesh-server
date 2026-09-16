// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// handleFileRemarks 查询文件备注，数据继续使用文件辅助状态持久化。
func handleFileRemarks(w http.ResponseWriter, req fileAdvancedRequest) {
	fileAux.Lock()
	loadFileAuxLocked()
	out := map[string]string{}
	for _, path := range req.Paths {
		if remark, ok := fileAux.data.Remarks[path]; ok {
			out[path] = remark
		}
	}
	fileAux.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"remarks": out}})
}

// handleFileRemark 更新单个文件备注，保持原辅助存储和响应契约。
func handleFileRemark(w http.ResponseWriter, req fileAdvancedRequest) {
	clean, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, http.StatusBadRequest, err)
		return
	}
	fileAux.Lock()
	loadFileAuxLocked()
	if fileAux.data.Remarks == nil {
		fileAux.data.Remarks = map[string]string{}
	}
	fileAux.data.Remarks[clean] = req.Name
	saveErr := saveFileAuxLocked()
	fileAux.Unlock()
	if saveErr != nil {
		fileError(w, http.StatusInternalServerError, saveErr)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": clean, "remark": req.Name}})
}
