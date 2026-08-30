// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
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

type fileAdvancedRequest struct {
	Path     string `json:"path"`
	Dst      string `json:"dst"`
	URL      string `json:"url"`
	Token    string `json:"token"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

type fileShare struct {
	Token     string    `json:"token"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
}

type fileFavorite struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	IsDir     bool      `json:"isDir"`
	CreatedAt time.Time `json:"createdAt"`
}

type fileRecycleItem struct {
	ID           string    `json:"id"`
	OriginalPath string    `json:"originalPath"`
	TrashPath    string    `json:"trashPath"`
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	IsDir        bool      `json:"isDir"`
	DeletedAt    time.Time `json:"deletedAt"`
}

type fileUploadItem struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
}

type fileAuxState struct {
	Favorites []fileFavorite    `json:"favorites"`
	Recycle   []fileRecycleItem `json:"recycle"`
	Uploads   []fileUploadItem  `json:"uploads"`
}

var fileAux struct {
	sync.Mutex
	loaded bool
	path   string
	data   fileAuxState
}

func fileAuxPath() string {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = ".workmesh-data"
	}
	return filepath.Join(dir, "files.json")
}

func loadFileAuxLocked() {
	path := fileAuxPath()
	if fileAux.loaded && fileAux.path == path {
		return
	}
	fileAux.loaded, fileAux.path = true, path
	fileAux.data = fileAuxState{Favorites: []fileFavorite{}, Recycle: []fileRecycleItem{}, Uploads: []fileUploadItem{}}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &fileAux.data)
	}
	if fileAux.data.Favorites == nil {
		fileAux.data.Favorites = []fileFavorite{}
	}
	if fileAux.data.Recycle == nil {
		fileAux.data.Recycle = []fileRecycleItem{}
	}
	if fileAux.data.Uploads == nil {
		fileAux.data.Uploads = []fileUploadItem{}
	}
}

func saveFileAuxLocked() error {
	if err := os.MkdirAll(filepath.Dir(fileAux.path), 0o750); err != nil {
		return err
	}
	raw, err := json.Marshal(fileAux.data)
	if err != nil {
		return err
	}
	tmp := fileAux.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, fileAux.path)
}

func fileAuxID(prefix string) string {
	return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func fileAuxSearch(path string, req fileAdvancedRequest) ([]any, int) {
	fileAux.Lock()
	defer fileAux.Unlock()
	loadFileAuxLocked()
	page, size := req.Page, req.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 50
	}
	var source []any
	switch path {
	case "recycle/search":
		for _, item := range fileAux.data.Recycle {
			source = append(source, item)
		}
	case "favorite/search":
		for _, item := range fileAux.data.Favorites {
			source = append(source, item)
		}
	case "upload/search":
		for _, item := range fileAux.data.Uploads {
			source = append(source, item)
		}
	}
	total := len(source)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return source[start:end], total
}

var fileShareState struct {
	sync.Mutex
	loaded bool
	items  map[string]fileShare
}

func fileShareFile() string {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = ".workmesh-data"
	}
	return filepath.Join(dir, "file-shares.json")
}

func loadFileSharesLocked() {
	if fileShareState.loaded {
		return
	}
	fileShareState.loaded = true
	fileShareState.items = make(map[string]fileShare)
	b, err := os.ReadFile(fileShareFile())
	if err == nil {
		_ = json.Unmarshal(b, &fileShareState.items)
	}
}

func saveFileSharesLocked() error {
	if err := os.MkdirAll(filepath.Dir(fileShareFile()), 0o750); err != nil {
		return err
	}
	b, err := json.Marshal(fileShareState.items)
	if err != nil {
		return err
	}
	tmp := fileShareFile() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, fileShareFile())
}

func findFileShare(token string) (fileShare, bool) {
	fileShareState.Lock()
	defer fileShareState.Unlock()
	loadFileSharesLocked()
	item, ok := fileShareState.items[strings.TrimSpace(token)]
	if !ok {
		return fileShare{}, false
	}
	if _, err := os.Stat(item.Path); err != nil {
		return item, false
	}
	return item, true
}

func fileAdvancedHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/files/"), "/")
	if r.Method == http.MethodGet {
		switch path {
		case "recycle/status":
			fileAux.Lock()
			loadFileAuxLocked()
			count := len(fileAux.data.Recycle)
			fileAux.Unlock()
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"enabled": true, "items": count}})
		case "share/check", "share/info", "share/qrcode":
			share, ok := findFileShare(r.URL.Query().Get("token"))
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": share.Path, "exists": ok, "token": share.Token, "createdAt": share.CreatedAt}})
		case "share/download":
			share, ok := findFileShare(r.URL.Query().Get("token"))
			if !ok {
				fileError(w, http.StatusNotFound, errors.New("分享不存在或文件已删除"))
				return
			}
			w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(share.Path)+"\"")
			http.ServeFile(w, r, share.Path)
		case "wget/process", "wget/process/keys":
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": r.URL.Query().Get("path"), "exists": false, "items": []any{}}})
		default:
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": path, "items": []any{}}})
		}
		return
	}
	var req fileAdvancedRequest
	if r.Body != nil {
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "INVALID_JSON"}, "message": err.Error()})
			return
		}
	}
	switch path {
	case "favorite":
		clean, err := cleanFilePath(req.Path)
		if err != nil {
			fileError(w, http.StatusBadRequest, err)
			return
		}
		info, err := os.Stat(clean)
		if err != nil {
			fileError(w, http.StatusNotFound, err)
			return
		}
		fileAux.Lock()
		loadFileAuxLocked()
		for _, item := range fileAux.data.Favorites {
			if item.Path == clean {
				fileAux.Unlock()
				wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
				return
			}
		}
		item := fileFavorite{ID: fileAuxID("favorite"), Path: clean, Name: info.Name(), IsDir: info.IsDir(), CreatedAt: time.Now().UTC()}
		fileAux.data.Favorites = append(fileAux.data.Favorites, item)
		err = saveFileAuxLocked()
		fileAux.Unlock()
		if err != nil {
			fileError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	case "favorite/del":
		fileAux.Lock()
		loadFileAuxLocked()
		id := strings.TrimSpace(req.ID)
		if id == "" {
			id = strings.TrimSpace(req.Token)
		}
		pathValue := strings.TrimSpace(req.Path)
		kept := fileAux.data.Favorites[:0]
		removed := false
		for _, item := range fileAux.data.Favorites {
			if (id != "" && item.ID == id) || (pathValue != "" && item.Path == pathValue) {
				removed = true
				continue
			}
			kept = append(kept, item)
		}
		fileAux.data.Favorites = kept
		err := saveFileAuxLocked()
		fileAux.Unlock()
		if err != nil {
			fileError(w, http.StatusInternalServerError, err)
			return
		}
		if !removed {
			fileError(w, http.StatusNotFound, errors.New("收藏不存在"))
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"deleted": true}})
	case "compress":
		if err := zipPath(req.Path, req.Dst); err != nil {
			fileError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": req.Dst}})
	case "decompress":
		if err := unzipPath(req.Path, req.Dst); err != nil {
			fileError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": req.Dst}})
	case "share/create":
		clean, err := cleanFilePath(req.Path)
		if err != nil {
			fileError(w, http.StatusBadRequest, err)
			return
		}
		if _, err := os.Stat(clean); err != nil {
			fileError(w, http.StatusNotFound, err)
			return
		}
		var raw [12]byte
		if _, err := rand.Read(raw[:]); err != nil {
			fileError(w, http.StatusInternalServerError, err)
			return
		}
		share := fileShare{Token: hex.EncodeToString(raw[:]), Path: clean, CreatedAt: time.Now().UTC()}
		fileShareState.Lock()
		loadFileSharesLocked()
		fileShareState.items[share.Token] = share
		err = saveFileSharesLocked()
		fileShareState.Unlock()
		if err != nil {
			fileError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": share})
	case "share/del":
		token := strings.TrimSpace(req.Token)
		if token == "" {
			fileError(w, http.StatusBadRequest, errors.New("分享 token 不能为空"))
			return
		}
		fileShareState.Lock()
		loadFileSharesLocked()
		if _, ok := fileShareState.items[token]; !ok {
			fileShareState.Unlock()
			fileError(w, http.StatusNotFound, errors.New("分享不存在"))
			return
		}
		delete(fileShareState.items, token)
		err := saveFileSharesLocked()
		fileShareState.Unlock()
		if err != nil {
			fileError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"deleted": true, "token": token}})
	case "share/search":
		fileShareState.Lock()
		loadFileSharesLocked()
		items := make([]fileShare, 0, len(fileShareState.items))
		for _, item := range fileShareState.items {
			if _, err := os.Stat(item.Path); err == nil {
				items = append(items, item)
			}
		}
		fileShareState.Unlock()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
	case "recycle/search", "favorite/search", "upload/search":
		items, total := fileAuxSearch(path, req)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": total, "page": 1, "pageSize": len(items)}})
	case "recycle/clear":
		fileAux.Lock()
		loadFileAuxLocked()
		var firstErr error
		for _, item := range fileAux.data.Recycle {
			if err := os.RemoveAll(item.TrashPath); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		fileAux.data.Recycle = []fileRecycleItem{}
		err := saveFileAuxLocked()
		fileAux.Unlock()
		if firstErr != nil {
			fileError(w, http.StatusInternalServerError, firstErr)
			return
		}
		if err != nil {
			fileError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"cleared": true}})
	case "recycle/reduce":
		id := strings.TrimSpace(req.ID)
		if id == "" {
			id = strings.TrimSpace(req.Token)
		}
		if id == "" {
			id = strings.TrimSpace(req.Path)
		}
		fileAux.Lock()
		loadFileAuxLocked()
		index := -1
		for i, item := range fileAux.data.Recycle {
			if item.ID == id || item.OriginalPath == id {
				index = i
				break
			}
		}
		if index < 0 {
			fileAux.Unlock()
			fileError(w, http.StatusNotFound, errors.New("回收站文件不存在"))
			return
		}
		item := fileAux.data.Recycle[index]
		if _, statErr := os.Stat(item.OriginalPath); statErr == nil {
			fileAux.Unlock()
			fileError(w, http.StatusConflict, errors.New("原路径已存在，无法恢复"))
			return
		}
		if err := os.MkdirAll(filepath.Dir(item.OriginalPath), 0o750); err != nil {
			fileAux.Unlock()
			fileError(w, http.StatusInternalServerError, err)
			return
		}
		if err := os.Rename(item.TrashPath, item.OriginalPath); err != nil {
			fileAux.Unlock()
			fileError(w, http.StatusInternalServerError, err)
			return
		}
		fileAux.data.Recycle = append(fileAux.data.Recycle[:index], fileAux.data.Recycle[index+1:]...)
		err := saveFileAuxLocked()
		fileAux.Unlock()
		if err != nil {
			fileError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	case "wget/stop", "compress/stop", "decompress/stop", "chunkupload/stop", "move/stop":
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"stopped": true}})
	default:
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"accepted": true, "operation": path}})
	}
}

func zipPath(source, destination string) error {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(destination) == "" {
		return errors.New("压缩源和目标不能为空")
	}
	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	defer zw.Close()
	return filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(filepath.Dir(source), path)
		if err != nil {
			return err
		}
		entry, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(entry, in)
		_ = in.Close()
		return copyErr
	})
}

func unzipPath(source, destination string) error {
	items, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer items.Close()
	root := filepath.Clean(destination)
	if err := os.MkdirAll(root, 0o750); err != nil {
		return err
	}
	for _, item := range items.File {
		target := filepath.Join(root, filepath.FromSlash(item.Name))
		if !strings.HasPrefix(target, root+string(os.PathSeparator)) && target != root {
			return errors.New("压缩包包含非法路径")
		}
		if item.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		in, err := item.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err == nil {
			_, err = io.Copy(out, in)
			_ = out.Close()
		}
		_ = in.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
