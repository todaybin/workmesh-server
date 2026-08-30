// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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
	Path              string   `json:"path"`
	Dst               string   `json:"dst"`
	URL               string   `json:"url"`
	Name              string   `json:"name"`
	Token             string   `json:"token"`
	ID                string   `json:"id"`
	Code              string   `json:"code"`
	Query             string   `json:"query"`
	Paths             []string `json:"paths"`
	Mode              int64    `json:"mode"`
	User              string   `json:"user"`
	Group             string   `json:"group"`
	WithInit          bool     `json:"withInit"`
	ContainSub        bool     `json:"containSub"`
	MatchCase         bool     `json:"matchCase"`
	WholeWord         bool     `json:"wholeWord"`
	UseRegex          bool     `json:"useRegex"`
	MaxScanFiles      int      `json:"maxScanFiles"`
	MaxFileBytes      int64    `json:"maxFileBytes"`
	MaxHitsPerFile    int      `json:"maxHitsPerFile"`
	MaxTotalHits      int      `json:"maxTotalHits"`
	IgnoreCertificate bool     `json:"ignoreCertificate"`
	Page              int      `json:"page"`
	PageSize          int      `json:"pageSize"`
}

// fileWgetProcess 保存远程下载任务状态；状态仅保留有限字段，避免无界内存增长。
type fileWgetProcess struct {
	Key        string     `json:"key"`
	URL        string     `json:"url"`
	Path       string     `json:"path"`
	Status     string     `json:"status"`
	Downloaded int64      `json:"downloaded"`
	Total      int64      `json:"total"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

var fileWgetState struct {
	sync.RWMutex
	items  map[string]*fileWgetProcess
	cancel map[string]context.CancelFunc
}

func initFileWgetState() {
	fileWgetState.Lock()
	if fileWgetState.items == nil {
		fileWgetState.items = make(map[string]*fileWgetProcess)
		fileWgetState.cancel = make(map[string]context.CancelFunc)
	}
	fileWgetState.Unlock()
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
			token := strings.TrimSpace(r.URL.Query().Get("token"))
			if token == "" {
				token = strings.TrimSpace(r.URL.Query().Get("code"))
			}
			if token == "" {
				fileError(w, http.StatusBadRequest, errors.New("分享 code/token 不能为空"))
				return
			}
			share, ok := findFileShare(token)
			if path == "share/check" && !ok {
				fileError(w, http.StatusNotFound, errors.New("分享不存在或文件已删除"))
				return
			}
			if path == "share/qrcode" {
				// 当前轻量服务返回可编码的分享 URL，避免引入图像库和额外进程。
				wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"url": "/s/" + url.PathEscape(token), "token": token, "exists": ok}})
				return
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": share.Path, "exists": ok, "token": share.Token, "code": share.Token, "createdAt": share.CreatedAt}})
		case "share/download":
			share, ok := findFileShare(r.URL.Query().Get("token"))
			if !ok {
				fileError(w, http.StatusNotFound, errors.New("分享不存在或文件已删除"))
				return
			}
			w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(share.Path)+"\"")
			http.ServeFile(w, r, share.Path)
		case "wget/process", "wget/process/keys":
			initFileWgetState()
			fileWgetState.RLock()
			items := make([]fileWgetProcess, 0, len(fileWgetState.items))
			keys := make([]string, 0, len(fileWgetState.items))
			for key, item := range fileWgetState.items {
				if path == "wget/process" && r.URL.Query().Get("key") != "" && key != r.URL.Query().Get("key") {
					continue
				}
				items = append(items, *item)
				keys = append(keys, key)
			}
			fileWgetState.RUnlock()
			if path == "wget/process/keys" {
				wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"keys": keys}})
			} else {
				wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
			}
		default:
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": path, "operation": path}})
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
	case "check":
		clean, err := cleanFilePath(req.Path)
		if err != nil {
			fileError(w, http.StatusBadRequest, err)
			return
		}
		_, statErr := os.Stat(clean)
		exists := statErr == nil
		if !exists && req.WithInit {
			if err := os.MkdirAll(clean, 0o750); err != nil {
				fileError(w, http.StatusInternalServerError, err)
				return
			}
			exists = true
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"exist": exists, "path": clean}})
	case "batch/check":
		if len(req.Paths) == 0 || len(req.Paths) > 500 {
			fileError(w, http.StatusBadRequest, errors.New("paths 数量必须为 1-500"))
			return
		}
		items := make([]map[string]any, 0, len(req.Paths))
		for _, raw := range req.Paths {
			clean, err := cleanFilePath(raw)
			if err != nil {
				continue
			}
			if info, err := os.Stat(clean); err == nil {
				items = append(items, map[string]any{"name": info.Name(), "path": clean, "size": info.Size(), "modTime": info.ModTime(), "isDir": info.IsDir()})
			}
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": items})
	case "batch/del":
		if len(req.Paths) == 0 || len(req.Paths) > 200 {
			fileError(w, http.StatusBadRequest, errors.New("paths 数量必须为 1-200"))
			return
		}
		deleted := make([]string, 0, len(req.Paths))
		failures := make([]map[string]string, 0)
		for _, raw := range req.Paths {
			clean, err := cleanFilePath(raw)
			if err != nil {
				failures = append(failures, map[string]string{"path": raw, "error": err.Error()})
				continue
			}
			if err := os.RemoveAll(clean); err != nil {
				failures = append(failures, map[string]string{"path": clean, "error": err.Error()})
				continue
			}
			deleted = append(deleted, clean)
		}
		if len(deleted) == 0 {
			fileError(w, http.StatusBadRequest, errors.New("没有文件删除成功"))
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"deleted": deleted, "failures": failures, "count": len(deleted)}})
	case "batch/role":
		if len(req.Paths) == 0 || len(req.Paths) > 200 {
			fileError(w, http.StatusBadRequest, errors.New("paths 数量必须为 1-200"))
			return
		}
		if req.Mode < 0 || req.Mode > 0o7777 {
			fileError(w, http.StatusBadRequest, errors.New("mode 超出范围"))
			return
		}
		updated := make([]string, 0, len(req.Paths))
		failures := make([]map[string]string, 0)
		for _, raw := range req.Paths {
			clean, err := cleanFilePath(raw)
			if err != nil {
				failures = append(failures, map[string]string{"path": raw, "error": err.Error()})
				continue
			}
			if _, err := os.Stat(clean); err != nil {
				failures = append(failures, map[string]string{"path": clean, "error": "文件不存在"})
				continue
			}
			if err := os.Chmod(clean, os.FileMode(req.Mode)); err != nil {
				failures = append(failures, map[string]string{"path": clean, "error": err.Error()})
				continue
			}
			updated = append(updated, clean)
		}
		if len(updated) == 0 {
			fileError(w, http.StatusBadRequest, errors.New("没有文件权限更新成功"))
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"updated": updated, "failures": failures, "mode": req.Mode, "user": req.User, "group": req.Group}})
	case "ai-search":
		handleFileAISearch(w, r, req)
		return
	case "wget":
		handleFileWget(w, r, req)
		return
	case "wget/stop":
		key := strings.TrimSpace(req.ID)
		if key == "" {
			key = strings.TrimSpace(req.Token)
		}
		if key == "" {
			fileError(w, http.StatusBadRequest, errors.New("下载任务 key 不能为空"))
			return
		}
		initFileWgetState()
		fileWgetState.Lock()
		cancel := fileWgetState.cancel[key]
		item := fileWgetState.items[key]
		if cancel != nil {
			cancel()
		}
		if item != nil {
			item.Status = "cancelled"
		}
		fileWgetState.Unlock()
		if cancel == nil {
			fileError(w, http.StatusNotFound, errors.New("下载任务不存在"))
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"key": key, "stopped": true}})
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
	case "compress/stop", "decompress/stop", "chunkupload/stop", "move/stop":
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"stopped": true}})
	default:
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"accepted": true, "operation": path}})
	}
}

// handleFileAISearch 在指定目录内执行受限内容搜索，避免将搜索请求转发为占位响应。
func handleFileAISearch(w http.ResponseWriter, r *http.Request, req fileAdvancedRequest) {
	root, err := cleanFilePath(req.Path)
	if err != nil || strings.TrimSpace(req.Query) == "" {
		if err == nil {
			err = errors.New("query 不能为空")
		}
		fileError(w, http.StatusBadRequest, err)
		return
	}
	if st, statErr := os.Stat(root); statErr != nil || !st.IsDir() {
		if statErr == nil {
			statErr = errors.New("搜索路径不是目录")
		}
		fileError(w, http.StatusNotFound, statErr)
		return
	}
	maxFiles := req.MaxScanFiles
	if maxFiles <= 0 || maxFiles > 1000 {
		maxFiles = 500
	}
	maxHits := req.MaxTotalHits
	if maxHits <= 0 || maxHits > 5000 {
		maxHits = 500
	}
	matcher := func(string) bool { return false }
	if req.UseRegex {
		rx, compileErr := regexp.Compile(req.Query)
		if compileErr != nil {
			fileError(w, http.StatusBadRequest, compileErr)
			return
		}
		matcher = rx.MatchString
	} else {
		needle := req.Query
		if !req.MatchCase {
			needle = strings.ToLower(needle)
		}
		matcher = func(s string) bool {
			target := s
			if !req.MatchCase {
				target = strings.ToLower(target)
			}
			if req.WholeWord {
				for _, f := range strings.FieldsFunc(target, func(r rune) bool { return r < '0' || (r > '9' && r < 'A') || (r > 'Z' && r < 'a') || r > 'z' }) {
					if f == needle {
						return true
					}
				}
				return false
			}
			return strings.Contains(target, needle)
		}
	}
	type hit struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	hits := make([]hit, 0)
	scanned := 0
	truncated := false
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			if !req.ContainSub && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if scanned >= maxFiles {
			truncated = true
			return filepath.SkipAll
		}
		if info.Size() > 8<<20 {
			return nil
		}
		scanned++
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if matcher(line) {
				hits = append(hits, hit{Path: path, Line: i + 1, Text: line})
				if len(hits) >= maxHits {
					truncated = true
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"mode": "grep", "summary": "", "hits": hits, "contentScannedFiles": scanned, "contentHitsTruncated": truncated, "truncated": truncated, "itemCount": len(hits)}})
}

// handleFileWget 启动带超时和取消能力的远程文件下载任务。
func handleFileWget(w http.ResponseWriter, r *http.Request, req fileAdvancedRequest) {
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		fileError(w, http.StatusBadRequest, errors.New("url 必须为有效的 http(s) 地址"))
		return
	}
	dir, err := cleanFilePath(req.Path)
	if err != nil || strings.TrimSpace(req.Name) == "" {
		if err == nil {
			err = errors.New("name 不能为空")
		}
		fileError(w, http.StatusBadRequest, err)
		return
	}
	if strings.ContainsAny(req.Name, `/\\`) || req.Name == "." || req.Name == ".." {
		fileError(w, http.StatusBadRequest, errors.New("name 包含非法路径"))
		return
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	key := fileAuxID("wget")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	initFileWgetState()
	proc := &fileWgetProcess{Key: key, URL: u.String(), Path: filepath.Join(dir, req.Name), Status: "running", StartedAt: time.Now().UTC()}
	fileWgetState.Lock()
	if len(fileWgetState.items) >= 200 {
		for oldKey, oldItem := range fileWgetState.items {
			if oldItem.Status == "completed" || oldItem.Status == "failed" || oldItem.Status == "cancelled" {
				delete(fileWgetState.items, oldKey)
				if len(fileWgetState.items) < 200 {
					break
				}
			}
		}
	}
	fileWgetState.items[key] = proc
	fileWgetState.cancel[key] = cancel
	fileWgetState.Unlock()
	go func() {
		defer cancel()
		client := &http.Client{Timeout: 30 * time.Minute}
		request, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if reqErr == nil {
			var resp *http.Response
			resp, reqErr = client.Do(request)
			if reqErr == nil {
				defer resp.Body.Close()
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					reqErr = errors.New(resp.Status)
				} else {
					tmp := proc.Path + ".tmp"
					var out *os.File
					out, reqErr = os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
					if reqErr == nil {
						var n int64
						n, reqErr = io.Copy(out, resp.Body)
						fileWgetState.Lock()
						proc.Total, proc.Downloaded = resp.ContentLength, n
						fileWgetState.Unlock()
						_ = out.Close()
						if reqErr == nil {
							reqErr = os.Rename(tmp, proc.Path)
						} else {
							_ = os.Remove(tmp)
						}
					}
				}
			}
		}
		fileWgetState.Lock()
		now := time.Now().UTC()
		proc.FinishedAt = &now
		if ctx.Err() == context.Canceled || ctx.Err() == context.DeadlineExceeded {
			proc.Status = "cancelled"
		} else if reqErr != nil {
			proc.Status = "failed"
			proc.Error = reqErr.Error()
		} else {
			proc.Status = "completed"
		}
		delete(fileWgetState.cancel, key)
		fileWgetState.Unlock()
	}()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"key": key, "path": proc.Path}})
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
