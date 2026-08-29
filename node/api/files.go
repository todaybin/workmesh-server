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
	if err = os.WriteFile(path, []byte(req.Content), 0600); err != nil {
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
		err = os.Remove(path)
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
