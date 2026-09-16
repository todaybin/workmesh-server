// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// handleFileAdvancedRead 解析特殊日志来源，普通类型继续读取授权文件路径。
func handleFileAdvancedRead(w http.ResponseWriter, r *http.Request, path string, req fileAdvancedRequest) {
	switch path {
	case "read/ssl":
		handleFileSSLLogRead(w, r, req)
	case "read/php", "read/php-fpm-slow-logs":
		handleFilePHPLogRead(w, path, req)
	default:
		handleFileGenericLogRead(w, path, req)
	}
}

// handleFileSSLLogRead 根据证书 SQLite 元数据读取申请日志，拒绝任意路径输入。
func handleFileSSLLogRead(w http.ResponseWriter, r *http.Request, req fileAdvancedRequest) {
	idText := firstFlexibleID(req.ID, req.SSLID, req.WebsiteSSLID)
	if idText == "" {
		fileError(w, http.StatusBadRequest, errors.New("证书 ID 不能为空"))
		return
	}
	id, err := strconv.ParseUint(idText, 10, 32)
	if err != nil || id == 0 {
		fileError(w, http.StatusBadRequest, errors.New("证书 ID 无效"))
		return
	}
	logPath, err := service.NewSSLService().LogPath(r.Context(), uint(id))
	if err != nil {
		fileError(w, http.StatusNotFound, err)
		return
	}
	lines, status, err := readRegularLog(logPath, true)
	if err != nil {
		fileError(w, status, err)
		return
	}
	writePagedFileLines(w, logPath, lines, req.Page, req.PageSize, map[string]any{"scope": "", "taskStatus": ""})
}

// firstFlexibleID 返回兼容 ID 字段中的第一个非空值。
func firstFlexibleID(ids ...flexibleID) string {
	for _, id := range ids {
		if value := strings.TrimSpace(string(id)); value != "" {
			return value
		}
	}
	return ""
}

// handleFilePHPLogRead 从运行环境安装目录读取构建或 FPM 慢日志。
func handleFilePHPLogRead(w http.ResponseWriter, path string, req fileAdvancedRequest) {
	item, index := runtimeByID(getRuntimeStore(), strings.TrimSpace(string(req.ID)))
	if index < 0 || normalizeRuntimeTypeFilter(item.Type) != "php" || strings.TrimSpace(item.InstallPath) == "" {
		fileError(w, http.StatusNotFound, errors.New("PHP 运行时不存在"))
		return
	}
	logName := "build.log"
	if path == "read/php-fpm-slow-logs" {
		logName = filepath.Join("log", "fpm.slow.log")
	}
	logPath := filepath.Join(item.InstallPath, logName)
	lines, status, err := readRegularLog(logPath, true)
	if err != nil {
		fileError(w, status, err)
		return
	}
	writePagedFileLines(w, logPath, lines, req.Page, req.PageSize, map[string]any{"type": strings.TrimPrefix(path, "read/")})
}

// readRegularLog 读取不超过 8 MiB 的普通日志文件，可将缺失文件视为空日志。
func readRegularLog(path string, allowMissing bool) ([]string, int, error) {
	info, err := os.Lstat(path)
	if err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return nil, http.StatusBadRequest, errors.New("日志不是普通文件")
	}
	content, err := os.ReadFile(path)
	if allowMissing && errors.Is(err, os.ErrNotExist) {
		content, err = []byte{}, nil
	}
	if err != nil {
		return nil, http.StatusNotFound, err
	}
	if len(content) > 8<<20 {
		return nil, http.StatusRequestEntityTooLarge, errors.New("日志超过 8 MiB 读取限制")
	}
	if len(content) == 0 {
		return []string{}, http.StatusOK, nil
	}
	return strings.Split(strings.TrimRight(string(content), "\r\n"), "\n"), http.StatusOK, nil
}

// writePagedFileLines 按 1Panel 兼容字段返回日志分页结果。
func writePagedFileLines(w http.ResponseWriter, path string, lines []string, page, size int, extra map[string]any) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 500 {
		size = 500
	}
	start := (page - 1) * size
	if start > len(lines) {
		start = len(lines)
	}
	end := start + size
	if end > len(lines) {
		end = len(lines)
	}
	total := (len(lines) + size - 1) / size
	if total == 0 {
		total = 1
	}
	data := map[string]any{"path": path, "lines": lines[start:end], "totalLines": len(lines), "total": total, "end": end == len(lines)}
	for key, value := range extra {
		data[key] = value
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}

// handleFileGenericLogRead 读取客户端明确指定的授权文件路径。
func handleFileGenericLogRead(w http.ResponseWriter, path string, req fileAdvancedRequest) {
	clean, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, http.StatusBadRequest, err)
		return
	}
	content, err := os.ReadFile(clean)
	if err != nil {
		fileError(w, http.StatusNotFound, err)
		return
	}
	lines := strings.Split(strings.TrimRight(string(content), "\r\n"), "\n")
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": clean, "lines": lines, "totalLines": len(lines), "type": strings.TrimPrefix(path, "read/")}})
}
