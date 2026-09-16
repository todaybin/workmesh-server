// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// handleFileConvert 校验媒体转换任务，并以受控参数启动异步转换。
func handleFileConvert(w http.ResponseWriter, req fileAdvancedRequest) {
	converter := strings.TrimSpace(os.Getenv("WORKMESH_MEDIA_CONVERTER"))
	if converter == "" {
		fileError(w, http.StatusServiceUnavailable, errors.New("未配置媒体转换器，请设置 WORKMESH_MEDIA_CONVERTER"))
		return
	}
	if _, err := exec.LookPath(converter); err != nil {
		fileError(w, http.StatusServiceUnavailable, fmt.Errorf("媒体转换器不可执行: %w", err))
		return
	}
	items, err := fileConvertItems(req)
	if err != nil {
		fileError(w, http.StatusBadRequest, err)
		return
	}
	outputRoot := req.OutputPath
	if outputRoot == "" && req.Dst != "" {
		outputRoot = filepath.Dir(req.Dst)
	}
	outputRoot, err = cleanFilePath(outputRoot)
	if err != nil {
		fileError(w, http.StatusBadRequest, err)
		return
	}
	if err := os.MkdirAll(outputRoot, 0o750); err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	if status, err := validateFileConvertItems(items); err != nil {
		fileError(w, status, err)
		return
	}
	taskID := strings.TrimSpace(req.TaskID)
	if taskID == "" {
		taskID = idToken()
	}
	for _, item := range items {
		input := filepath.Join(item.Path, item.InputFile)
		name := strings.TrimSuffix(filepath.Base(item.InputFile), filepath.Ext(item.InputFile)) + "." + strings.TrimPrefix(item.OutputFormat, ".")
		output := filepath.Join(outputRoot, name)
		appendConvertLog(fileConvertLog{Date: time.Now().Format("2006-01-02 15:04:05"), Type: item.Type, Log: fmt.Sprintf("%s -> %s", input, output), Status: "RUNNING", Message: "QUEUED", TaskID: taskID})
		go runMediaConversion(converter, input, output, item.Type, taskID, req.DeleteSource)
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"taskID": taskID, "status": "queued", "total": len(items)}})
}

// fileConvertItems 兼容 files 数组和单个 path/dst 请求格式。
func fileConvertItems(req fileAdvancedRequest) ([]fileConvertItem, error) {
	var items []fileConvertItem
	_ = json.Unmarshal(req.Files, &items)
	if len(items) > 0 {
		return items, nil
	}
	if req.Path == "" || req.Dst == "" {
		return nil, errors.New("files 或 path/dst 不能为空")
	}
	return []fileConvertItem{{Path: filepath.Dir(req.Path), InputFile: filepath.Base(req.Path), OutputFormat: strings.TrimPrefix(filepath.Ext(req.Dst), "."), Type: req.Type}}, nil
}

// validateFileConvertItems 校验转换输入必须是授权根内的普通文件。
func validateFileConvertItems(items []fileConvertItem) (int, error) {
	for _, item := range items {
		if strings.TrimSpace(item.InputFile) == "" || filepath.Base(item.InputFile) != item.InputFile || strings.ContainsAny(item.InputFile, `/\`) {
			return http.StatusBadRequest, errors.New("inputFile 无效")
		}
		input, err := cleanFilePath(filepath.Join(item.Path, item.InputFile))
		if err != nil {
			return http.StatusBadRequest, err
		}
		if info, err := os.Stat(input); err != nil || !info.Mode().IsRegular() {
			if err != nil {
				return http.StatusNotFound, err
			}
			return http.StatusNotFound, errors.New("输入文件不是普通文件")
		}
		if strings.TrimSpace(item.OutputFormat) == "" {
			return http.StatusBadRequest, errors.New("outputFormat 不能为空")
		}
	}
	return http.StatusOK, nil
}

// handleFileConvertLog 分页查询 SQLite 持久化的转换任务日志。
func handleFileConvertLog(w http.ResponseWriter, r *http.Request) {
	values, err := requestMap(r)
	if err != nil {
		domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	page, size := intValue(values, "page"), intValue(values, "pageSize")
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 50
	}
	status, typ, taskID := strings.ToLower(valueString(values, "status")), strings.ToLower(valueString(values, "type")), valueString(values, "taskID")
	fileAux.Lock()
	loadFileAuxLocked()
	all := append([]fileConvertLog(nil), fileAux.data.ConvertLogs...)
	fileAux.Unlock()
	filtered := all[:0]
	for _, item := range all {
		if (status == "" || strings.ToLower(item.Status) == status) && (typ == "" || strings.ToLower(item.Type) == typ) && (taskID == "" || item.TaskID == taskID) {
			filtered = append(filtered, item)
		}
	}
	total, start := len(filtered), (page-1)*size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": filtered[start:end], "total": total, "page": page, "pageSize": size}})
}
