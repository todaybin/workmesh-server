// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func registerFileRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/files/content", handleFilesContent)
	mux.HandleFunc("POST /api/v2/files/del", handleFilesDelete)
	mux.HandleFunc("POST /api/v2/files/move", handleFilesMove)
	mux.HandleFunc("POST /api/v2/files/rename", handleFilesRename)
	mux.HandleFunc("POST /api/v2/files/save", handleFilesSave)
	mux.HandleFunc("POST /api/v2/files/search", handleFilesSearch)
	mux.HandleFunc("POST /api/v2/files/size", handleFilesSize)
	mux.HandleFunc("POST /api/v2/files/tree", handleFilesTree)
	mux.HandleFunc("POST /api/v2/files/upload", handleFilesUpload)
	mux.HandleFunc("GET /api/v2/files/download", handleFilesDownload)
	mux.HandleFunc("/api/v2/files/", fileAdvancedHandler)
}

func isFileRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return path == "/api/v2/files" || strings.HasPrefix(path, "/api/v2/files/")
}

func appendConvertLog(item fileConvertLog) {
	fileAux.Lock()
	loadFileAuxLocked()
	fileAux.data.ConvertLogs = append(fileAux.data.ConvertLogs, item)
	if len(fileAux.data.ConvertLogs) > 2000 {
		fileAux.data.ConvertLogs = fileAux.data.ConvertLogs[len(fileAux.data.ConvertLogs)-2000:]
	}
	_ = saveFileAuxLocked()
	fileAux.Unlock()
}

// runMediaConversion 在受控超时内执行外部转换器，并记录可查询的成功/失败状态。
func runMediaConversion(converter, input, output, typ, taskID string, deleteSource bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	ext := filepath.Ext(output)
	tmp, err := os.CreateTemp(filepath.Dir(output), ".workmesh-convert-*"+ext)
	if err == nil {
		tmpPath := tmp.Name()
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		cmd := exec.CommandContext(ctx, converter, input, tmpPath)
		combined, runErr := cmd.CombinedOutput()
		if runErr == nil {
			if st, statErr := os.Stat(tmpPath); statErr == nil && st.Size() > 0 {
				err = os.Rename(tmpPath, output)
			} else {
				err = errors.New("转换器未生成有效输出文件")
			}
		}
		if runErr != nil {
			err = fmt.Errorf("转换器执行失败: %w (%s)", runErr, strings.TrimSpace(string(combined)))
		}
		if err == nil && deleteSource {
			_ = os.Remove(input)
		}
		status, message := "SUCCESS", "SUCCESS"
		if err != nil {
			status, message = "FAILED", err.Error()
			_ = os.Remove(tmpPath)
		}
		appendConvertLog(fileConvertLog{Date: time.Now().Format("2006-01-02 15:04:05"), Type: typ, Log: fmt.Sprintf("%s -> %s", input, output), Status: status, Message: message, TaskID: taskID})
		return
	}
	appendConvertLog(fileConvertLog{Date: time.Now().Format("2006-01-02 15:04:05"), Type: typ, Log: fmt.Sprintf("%s -> %s", input, output), Status: "FAILED", Message: err.Error(), TaskID: taskID})
}
