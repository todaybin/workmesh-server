// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func fileChunkDir() string {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	return filepath.Join(root, "chunks")
}

// handleChunkUpload 按偏移量写入受控分片，并在最后一片使用原子 rename 完成提交。
func handleChunkUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		fileError(w, http.StatusBadRequest, err)
		return
	}
	part, _, err := r.FormFile("chunk")
	if err != nil {
		fileError(w, http.StatusBadRequest, errors.New("缺少 chunk 文件字段"))
		return
	}
	defer part.Close()
	filename := strings.TrimSpace(r.FormValue("filename"))
	dstDir, err := cleanFilePath(r.FormValue("path"))
	if err != nil || filename == "" || filepath.Base(filename) != filename || strings.ContainsAny(filename, `/\\`) {
		if err == nil {
			err = errors.New("filename 无效")
		}
		fileError(w, http.StatusBadRequest, err)
		return
	}
	chunkIndex, err1 := strconv.Atoi(r.FormValue("chunkIndex"))
	chunkCount, err2 := strconv.Atoi(r.FormValue("chunkCount"))
	if err1 != nil || err2 != nil || chunkCount <= 0 || chunkCount > 10000 || chunkIndex < 0 || chunkIndex >= chunkCount {
		fileError(w, http.StatusBadRequest, errors.New("chunkIndex/chunkCount 无效"))
		return
	}
	uploadID := strings.TrimSpace(r.FormValue("uploadID"))
	if uploadID == "" {
		uploadID = fileAuxID("upload")
	}
	if filepath.Base(uploadID) != uploadID || strings.ContainsAny(uploadID, `/\\`) {
		fileError(w, http.StatusBadRequest, errors.New("uploadID 无效"))
		return
	}
	offset := int64(0)
	if raw := strings.TrimSpace(r.FormValue("offset")); raw != "" {
		offset, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || offset < 0 {
			fileError(w, http.StatusBadRequest, errors.New("offset 无效"))
			return
		}
	}
	fileSize := int64(-1)
	if raw := strings.TrimSpace(r.FormValue("fileSize")); raw != "" {
		fileSize, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || fileSize < 0 || fileSize > 8<<30 {
			fileError(w, http.StatusBadRequest, errors.New("fileSize 超出范围"))
			return
		}
	}
	if err := os.MkdirAll(fileChunkDir(), 0o750); err != nil {
		fileError(w, 500, err)
		return
	}
	workDir := filepath.Join(fileChunkDir(), uploadID)
	if err := os.MkdirAll(workDir, 0o750); err != nil {
		fileError(w, 500, err)
		return
	}
	partPath := filepath.Join(workDir, filename+".part")
	out, err := os.OpenFile(partPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		fileError(w, 500, err)
		return
	}
	if offset == 0 && chunkIndex == 0 {
		_ = out.Truncate(0)
	}
	if _, err = out.Seek(offset, io.SeekStart); err == nil {
		_, err = io.Copy(out, io.LimitReader(part, 64<<20))
	}
	_ = out.Close()
	if err != nil {
		fileError(w, 500, err)
		return
	}
	done := chunkIndex+1 == chunkCount
	dstFile := filepath.Join(dstDir, filename)
	if done {
		if fileSize >= 0 {
			if info, statErr := os.Stat(partPath); statErr != nil || info.Size() != fileSize {
				fileError(w, http.StatusBadRequest, errors.New("分片文件大小不匹配"))
				return
			}
		}
		if err := os.MkdirAll(dstDir, 0o750); err != nil {
			fileError(w, 500, err)
			return
		}
		overwrite := !strings.EqualFold(strings.TrimSpace(r.FormValue("overwrite")), "false")
		if !overwrite {
			if _, statErr := os.Stat(dstFile); statErr == nil {
				fileError(w, http.StatusConflict, errors.New("目标文件已存在"))
				return
			}
		}
		if err := moveChunkIntoPlace(partPath, dstFile); err != nil {
			fileError(w, 500, err)
			return
		}
		// OpenResty runs in its own container user namespace. Website content
		// must remain readable after a chunk's secure 0600 staging file is moved
		// into the public site root.
		if isWebsiteContentPath(dstFile) {
			_ = os.Chmod(dstFile, 0o644)
		}
		applyWebsiteOwnership(dstFile)
		_ = os.RemoveAll(workDir)
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"uploadID": uploadID, "chunkIndex": chunkIndex, "chunkCount": chunkCount, "completed": done, "path": dstFile}})
}

func isWebsiteContentPath(path string) bool {
	root := strings.TrimSpace(os.Getenv("PANEL_WEBSITE_DIR"))
	if root == "" {
		root = "/www/wwwroot"
	}
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return false
	}
	target, err := filepath.Abs(filepath.Clean(path))
	if err != nil || target == root {
		return false
	}
	prefix := root + string(filepath.Separator)
	return strings.HasPrefix(target, prefix)
}

// moveChunkIntoPlace keeps the fast atomic rename path and falls back to a
// destination-filesystem copy when the chunk directory is on another mount.
func moveChunkIntoPlace(source, destination string) error {
	if err := os.Rename(source, destination); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	tmp := destination + ".workmesh-upload.tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		_ = in.Close()
		return err
	}
	_, copyErr := io.Copy(out, in)
	if copyErr == nil {
		copyErr = out.Sync()
	}
	closeOutErr := out.Close()
	closeInErr := in.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeOutErr != nil {
		_ = os.Remove(tmp)
		return closeOutErr
	}
	if closeInErr != nil {
		_ = os.Remove(tmp)
		return closeInErr
	}
	if err := os.Rename(tmp, destination); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Remove(source)
}

// handleChunkDownload 支持 HTTP Range，便于大文件断点续传而无需额外进程。
func handleChunkDownload(w http.ResponseWriter, r *http.Request) {
	var req fileAdvancedRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		fileError(w, http.StatusBadRequest, errors.New("请求体格式无效"))
		return
	}
	path, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		if err == nil {
			err = errors.New("文件不存在")
		}
		fileError(w, 404, err)
		return
	}
	start, end := req.Offset, info.Size()-1
	if req.FileSize > 0 {
		end = start + req.FileSize - 1
	}
	if start < 0 || start >= info.Size() || end < start {
		fileError(w, 400, errors.New("下载范围无效"))
		return
	}
	if end >= info.Size() {
		end = info.Size() - 1
	}
	f, err := os.Open(path)
	if err != nil {
		fileError(w, 500, err)
		return
	}
	defer f.Close()
	if _, err = f.Seek(start, io.SeekStart); err != nil {
		fileError(w, 500, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, info.Size()))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = io.CopyN(w, f, end-start+1)
}
