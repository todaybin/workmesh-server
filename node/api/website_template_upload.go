// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

const (
	// websiteTemplateUploadLimit 限制单个模板压缩包大小，避免单进程服务被大文件拖垮。
	websiteTemplateUploadLimit = 128 << 20
	// websiteTemplateEntryLimit 限制 ZIP 中的条目数量，避免解析恶意压缩包时耗尽资源。
	websiteTemplateEntryLimit = 4096
	// websiteTemplateTextLimit 限制扫描变量时单个文本文件的读取量。
	websiteTemplateTextLimit = 8 << 20
	// websiteTemplateScanLimit 限制一次上传扫描的文本总量，避免 ZIP 炸弹消耗单进程资源。
	websiteTemplateScanLimit = 64 << 20
)

var websiteTemplateVariablePattern = regexp.MustCompile(`\{\{(\w+)\}\}`)

// registerWebsiteTemplateUploadRoute 注册与 1Panel 相同的 multipart 模板上传契约。
// 上传内容实际写入 WorkMesh 数据目录，后续创建模板时使用返回的文件路径。
func registerWebsiteTemplateUploadRoute(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/websites/templates/upload", websiteTemplateUpload)
}

// websiteTemplateUpload 接收 ZIP 文件并以临时文件加原子重命名方式落盘。
// 失败时不留下半成品，返回错误而不是伪造成功状态。
func websiteTemplateUpload(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		websiteTemplateUploadError(w, http.StatusBadRequest, errors.New("上传文件不能为空"))
		return
	}
	// MaxBytesReader 同时限制 multipart 头部和文件内容，给少量 multipart 开销留出空间。
	r.Body = http.MaxBytesReader(w, r.Body, websiteTemplateUploadLimit+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		websiteTemplateUploadError(w, http.StatusBadRequest, fmt.Errorf("解析模板上传请求失败: %w", err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		websiteTemplateUploadError(w, http.StatusBadRequest, errors.New("请求中缺少 file 文件"))
		return
	}
	defer file.Close()
	if header == nil || !strings.EqualFold(filepath.Ext(header.Filename), ".zip") {
		websiteTemplateUploadError(w, http.StatusBadRequest, errors.New("模板文件必须为 ZIP 格式"))
		return
	}

	dir := websiteTemplateUploadDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		websiteTemplateUploadError(w, http.StatusInternalServerError, fmt.Errorf("创建模板目录失败: %w", err))
		return
	}
	tmp, err := os.CreateTemp(dir, ".template-upload-*.zip")
	if err != nil {
		websiteTemplateUploadError(w, http.StatusInternalServerError, fmt.Errorf("创建模板临时文件失败: %w", err))
		return
	}
	tmpPath := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpPath)
		}
	}()

	written, err := io.Copy(tmp, io.LimitReader(file, websiteTemplateUploadLimit+1))
	if err != nil {
		websiteTemplateUploadError(w, http.StatusBadRequest, fmt.Errorf("读取模板文件失败: %w", err))
		return
	}
	if written > websiteTemplateUploadLimit {
		websiteTemplateUploadError(w, http.StatusRequestEntityTooLarge, errors.New("模板文件超过 128 MiB 限制"))
		return
	}
	if err := tmp.Sync(); err != nil {
		websiteTemplateUploadError(w, http.StatusInternalServerError, fmt.Errorf("同步模板文件失败: %w", err))
		return
	}
	if err := tmp.Close(); err != nil {
		websiteTemplateUploadError(w, http.StatusInternalServerError, fmt.Errorf("关闭模板临时文件失败: %w", err))
		return
	}

	variables, err := scanWebsiteTemplateZip(tmpPath)
	if err != nil {
		websiteTemplateUploadError(w, http.StatusBadRequest, err)
		return
	}
	name := filepath.Base(header.Filename)
	finalPath := filepath.Join(dir, fmt.Sprintf("%d_%s", time.Now().UnixNano(), name))
	if err := os.Rename(tmpPath, finalPath); err != nil {
		websiteTemplateUploadError(w, http.StatusInternalServerError, fmt.Errorf("保存模板文件失败: %w", err))
		return
	}
	keep = true
	wmhttp.JSON(w, http.StatusOK, map[string]any{
		"code": 200,
		"data": map[string]any{"filePath": finalPath, "variables": variables},
	})
}

// websiteTemplateUploadDir 返回模板上传目录，业务文件统一放在数据目录内。
func websiteTemplateUploadDir() string {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	return filepath.Join(root, "templates", "files")
}

// scanWebsiteTemplateZip 校验 ZIP 并提取模板变量，绝不解压到站点目录。
func scanWebsiteTemplateZip(path string) ([]string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, errors.New("模板文件不是有效的 ZIP 压缩包")
	}
	defer reader.Close()
	if len(reader.File) > websiteTemplateEntryLimit {
		return nil, errors.New("模板压缩包文件数量超过限制")
	}

	seen := make(map[string]struct{})
	variables := make([]string, 0)
	var scanned uint64
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() || !websiteTemplateTextFile(entry.Name) || entry.UncompressedSize64 > websiteTemplateTextLimit {
			continue
		}
		if scanned+entry.UncompressedSize64 > websiteTemplateScanLimit {
			return nil, errors.New("模板文本总量超过扫描限制")
		}
		part, openErr := entry.Open()
		if openErr != nil {
			return nil, fmt.Errorf("读取模板文件 %q 失败: %w", entry.Name, openErr)
		}
		content, readErr := io.ReadAll(io.LimitReader(part, websiteTemplateTextLimit+1))
		_ = part.Close()
		if readErr != nil {
			return nil, fmt.Errorf("读取模板文件 %q 失败: %w", entry.Name, readErr)
		}
		scanned += uint64(len(content))
		for _, match := range websiteTemplateVariablePattern.FindAllStringSubmatch(string(content), -1) {
			if _, exists := seen[match[1]]; exists {
				continue
			}
			seen[match[1]] = struct{}{}
			variables = append(variables, match[1])
		}
	}
	return variables, nil
}

// websiteTemplateTextFile 只扫描常见文本模板，避免把二进制资源当作模板变量读取。
func websiteTemplateTextFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm", ".css", ".js", ".json", ".txt", ".xml", ".md", ".php", ".conf", ".tpl", ".svg":
		return true
	default:
		return false
	}
}

// websiteTemplateUploadError 使用统一 API 错误外壳，保持前端错误处理兼容。
func websiteTemplateUploadError(w http.ResponseWriter, status int, err error) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
}
