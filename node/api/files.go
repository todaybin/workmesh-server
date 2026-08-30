// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

type fileRequest struct {
	Path        string   `json:"path"`
	Content     string   `json:"content"`
	Name        string   `json:"name"`
	IsDir       bool     `json:"isDir"`
	ForceDelete bool     `json:"forceDelete"`
	Paths       []string `json:"paths"`
	Dst         string   `json:"dst"`
	ShowHidden  bool     `json:"showHidden"`
}

func decodeFileRequest(r *http.Request) (fileRequest, error) {
	var req fileRequest
	if r.Body == nil {
		return req, errors.New("请求体不能为空")
	}
	err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req)
	return req, err
}
func cleanFilePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("文件路径不能为空")
	}
	clean := filepath.Clean(path)
	if clean == "." || strings.ContainsRune(clean, 0) {
		return "", errors.New("文件路径无效")
	}
	return clean, nil
}
func fileInfo(path string, info os.FileInfo) map[string]any {
	return map[string]any{"path": path, "name": info.Name(), "size": info.Size(), "isDir": info.IsDir(), "isSymlink": info.Mode()&os.ModeSymlink != 0, "mode": info.Mode().Perm(), "modTime": info.ModTime(), "updateTime": info.ModTime(), "mimeType": mime.TypeByExtension(filepath.Ext(info.Name()))}
}

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
		info, e := entry.Info()
		if e == nil {
			items = append(items, fileInfo(filepath.Join(root, entry.Name()), info))
		}
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
}

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
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"path": path, "content": string(data)}})
}
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
	// 临时文件与目标位于同一目录，确保 rename 在同一文件系统内原子替换，
	// 避免服务中断或并发读取时看到半写入内容。
	if err = os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
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
	wmhttp.JSON(w, 200, map[string]any{"code": 200})
}
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
	info, _ := os.Stat(path)
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": fileInfo(path, info)})
}
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
		// 非强制删除进入本服务自己的回收目录，保留原路径以支持恢复。
		info, statErr := os.Stat(path)
		if statErr != nil {
			fileError(w, http.StatusNotFound, statErr)
			return
		}
		trashRoot := filepath.Join(filepath.Dir(fileAuxPath()), "recycle")
		if err = os.MkdirAll(trashRoot, 0o750); err == nil {
			trashPath := filepath.Join(trashRoot, fileAuxID("item"))
			err = os.Rename(path, trashPath)
			if err == nil {
				fileAux.Lock()
				loadFileAuxLocked()
				fileAux.data.Recycle = append(fileAux.data.Recycle, fileRecycleItem{ID: fileAuxID("recycle"), OriginalPath: path, TrashPath: trashPath, Name: info.Name(), Size: info.Size(), IsDir: info.IsDir(), DeletedAt: time.Now().UTC()})
				err = saveFileAuxLocked()
				fileAux.Unlock()
			}
		}
	}
	if err != nil {
		fileError(w, 404, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200})
}
func handleFilesRename(w http.ResponseWriter, r *http.Request) {
	req, err := decodeFileRequest(r)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	old, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	if req.Name == "" {
		fileError(w, 400, errors.New("新名称不能为空"))
		return
	}
	dst := filepath.Join(filepath.Dir(old), filepath.Base(req.Name))
	if err = os.Rename(old, dst); err != nil {
		fileError(w, 500, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"path": dst}})
}
func handleFilesMove(w http.ResponseWriter, r *http.Request) {
	req, err := decodeFileRequest(r)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	src, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	dst, err := cleanFilePath(req.Dst)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	if err = os.Rename(src, dst); err != nil {
		fileError(w, 500, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200})
}
func handleFilesSize(w http.ResponseWriter, r *http.Request) {
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
	var total int64
	filepath.Walk(path, func(_ string, info os.FileInfo, e error) error {
		if e == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"path": path, "size": total}})
}

func handleFilesTree(w http.ResponseWriter, r *http.Request) {
	req, err := decodeFileRequest(r)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	root, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	var walk func(string, int) []map[string]any
	walk = func(path string, depth int) []map[string]any {
		if depth > 4 {
			return nil
		}
		entries, _ := os.ReadDir(path)
		result := make([]map[string]any, 0, len(entries))
		for _, entry := range entries {
			if !req.ShowHidden && strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			full := filepath.Join(path, entry.Name())
			info, e := entry.Info()
			if e != nil {
				continue
			}
			item := fileInfo(full, info)
			if info.IsDir() {
				item["children"] = walk(full, depth+1)
			}
			result = append(result, item)
		}
		return result
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": walk(root, 0)})
}

func handleFilesUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		fileError(w, 400, err)
		return
	}
	dir, err := cleanFilePath(r.FormValue("path"))
	if err != nil {
		fileError(w, 400, err)
		return
	}
	for _, headers := range r.MultipartForm.File {
		for _, header := range headers {
			in, e := header.Open()
			if e != nil {
				fileError(w, 400, e)
				return
			}
			dst := filepath.Join(dir, filepath.Base(header.Filename))
			out, e := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if e == nil {
				_, e = io.Copy(out, in)
				_ = out.Close()
			}
			_ = in.Close()
			if e != nil {
				fileError(w, 500, e)
				return
			}
			if info, statErr := os.Stat(dst); statErr == nil {
				fileAux.Lock()
				loadFileAuxLocked()
				fileAux.data.Uploads = append(fileAux.data.Uploads, fileUploadItem{ID: fileAuxID("upload"), Path: dst, Name: info.Name(), Size: info.Size(), CreatedAt: time.Now().UTC()})
				_ = saveFileAuxLocked()
				fileAux.Unlock()
			}
		}
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200})
}
func handleFilesDownload(w http.ResponseWriter, r *http.Request) {
	path, err := cleanFilePath(r.URL.Query().Get("path"))
	if err != nil {
		fileError(w, 400, err)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(path)+"\"")
	http.ServeFile(w, r, path)
}
func fileError(w http.ResponseWriter, status int, err error) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
}
