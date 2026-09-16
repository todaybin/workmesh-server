// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// fileAdvancedHandler 按协议类型分发文件高级操作，具体业务由独立处理器承担。
func fileAdvancedHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/files/"), "/")
	operationPath := path
	if strings.HasPrefix(path, "read/") {
		operationPath = "read"
	}
	if r.Method == http.MethodPost && handleFileAdvancedMultipart(w, r, path) {
		return
	}
	if r.Method == http.MethodGet {
		handleFileAdvancedGet(w, r, path)
		return
	}
	var req fileAdvancedRequest
	if r.Body != nil {
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "INVALID_JSON"}, "message": err.Error()})
			return
		}
	}
	if handleFileAdvancedOperation(w, r, path, req) {
		return
	}
	if operationPath == "read" {
		handleFileAdvancedRead(w, r, path, req)
		return
	}
	fileError(w, http.StatusNotImplemented, errors.New("文件操作 \""+path+"\" 尚未实现"))
}

// handleFileAdvancedMultipart 在 JSON 解码前处理 multipart 分片操作。
func handleFileAdvancedMultipart(w http.ResponseWriter, r *http.Request, path string) bool {
	switch path {
	case "chunkupload":
		handleChunkUpload(w, r)
	case "chunkdownload":
		handleChunkDownload(w, r)
	default:
		return false
	}
	return true
}
