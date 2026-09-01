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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	osuser "os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/service"
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
	Path              string            `json:"path"`
	Dst               string            `json:"dst"`
	URL               string            `json:"url"`
	Name              string            `json:"name"`
	Token             string            `json:"token"`
	Key               string            `json:"key"`
	ID                flexibleID        `json:"id"`
	Code              string            `json:"code"`
	Query             string            `json:"query"`
	Paths             []string          `json:"paths"`
	Mode              int64             `json:"mode"`
	User              string            `json:"user"`
	Group             string            `json:"group"`
	WithInit          bool              `json:"withInit"`
	ContainSub        bool              `json:"containSub"`
	MatchCase         bool              `json:"matchCase"`
	WholeWord         bool              `json:"wholeWord"`
	UseRegex          bool              `json:"useRegex"`
	MaxScanFiles      int               `json:"maxScanFiles"`
	MaxFileBytes      int64             `json:"maxFileBytes"`
	MaxHitsPerFile    int               `json:"maxHitsPerFile"`
	MaxTotalHits      int               `json:"maxTotalHits"`
	IgnoreCertificate bool              `json:"ignoreCertificate"`
	Page              int               `json:"page"`
	PageSize          int               `json:"pageSize"`
	UploadID          string            `json:"uploadID"`
	ChunkIndex        int               `json:"chunkIndex"`
	ChunkCount        int               `json:"chunkCount"`
	Offset            int64             `json:"offset"`
	FileSize          int64             `json:"fileSize"`
	Overwrite         bool              `json:"overwrite"`
	DeleteSource      bool              `json:"deleteSource"`
	OutputPath        string            `json:"outputPath"`
	TaskID            string            `json:"taskID"`
	Type              string            `json:"type"`
	Extension         string            `json:"extension"`
	OutputFormat      string            `json:"outputFormat"`
	Files             []fileConvertItem `json:"files"`
	Remark            string            `json:"remark"`
}

// flexibleID 兼容历史客户端将资源 ID 以数字或字符串提交的两种形式。
type flexibleID string

func (id *flexibleID) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*id = flexibleID(text)
		return nil
	}
	var number json.Number
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return fmt.Errorf("id 必须是字符串或数字")
	}
	*id = flexibleID(number.String())
	return nil
}

// fileConvertItem 描述单个媒体文件转换输入与目标格式。
type fileConvertItem struct {
	Path         string `json:"path"`
	Type         string `json:"type"`
	InputFile    string `json:"inputFile"`
	Extension    string `json:"extension"`
	OutputFormat string `json:"outputFormat"`
}

// fileConvertLog 与前端契约保持一致，记录异步转换结果。
type fileConvertLog struct {
	Date    string `json:"date"`
	Type    string `json:"type"`
	Log     string `json:"log"`
	Status  string `json:"status"`
	Message string `json:"message"`
	TaskID  string `json:"taskID,omitempty"`
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

// wgetProgressWriter 在写入目标文件的同时累计字节数，供进度接口实时读取。
type wgetProgressWriter struct {
	dst io.Writer
	key string
}

func (w *wgetProgressWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	if n > 0 {
		fileWgetState.Lock()
		if item := fileWgetState.items[w.key]; item != nil {
			item.Downloaded += int64(n)
		}
		fileWgetState.Unlock()
	}
	return n, err
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
	Favorites   []fileFavorite    `json:"favorites"`
	Recycle     []fileRecycleItem `json:"recycle"`
	Uploads     []fileUploadItem  `json:"uploads"`
	Remarks     map[string]string `json:"remarks,omitempty"`
	History     []fileHistoryItem `json:"history,omitempty"`
	ConvertLogs []fileConvertLog  `json:"convertLogs,omitempty"`
}

type fileHistoryItem struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
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
		dir = "./data"
	}
	return filepath.Join(dir, "files.json")
}

func loadFileAuxLocked() {
	path := fileAuxPath()
	if fileAux.loaded && fileAux.path == path {
		return
	}
	fileAux.loaded, fileAux.path = true, path
	fileAux.data = fileAuxState{Favorites: []fileFavorite{}, Recycle: []fileRecycleItem{}, Uploads: []fileUploadItem{}, Remarks: map[string]string{}, History: []fileHistoryItem{}, ConvertLogs: []fileConvertLog{}}
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
	if fileAux.data.Remarks == nil {
		fileAux.data.Remarks = map[string]string{}
	}
	if fileAux.data.History == nil {
		fileAux.data.History = []fileHistoryItem{}
	}
	if fileAux.data.ConvertLogs == nil {
		fileAux.data.ConvertLogs = []fileConvertLog{}
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

func fileAuxSearch(path string, req fileAdvancedRequest) ([]any, int, int, int) {
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
	return source[start:end], total, page, size
}

var fileShareState struct {
	sync.Mutex
	loaded bool
	items  map[string]fileShare
}

func fileShareFile() string {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = "./data"
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
	// 旧接口将读取类型编码在路径中（read/:type），统一映射到同一读取实现。
	operationPath := path
	if strings.HasPrefix(path, "read/") {
		operationPath = "read"
	}
	// 分片上传使用 multipart/form-data，必须在 JSON 解码前单独处理。
	if r.Method == http.MethodPost && path == "chunkupload" {
		handleChunkUpload(w, r)
		return
	}
	if r.Method == http.MethodPost && path == "chunkdownload" {
		handleChunkDownload(w, r)
		return
	}
	if r.Method == http.MethodGet {
		switch path {
		case "read/website":
			handleWebsiteFileRead(w, fileAdvancedRequest{ID: flexibleID(r.URL.Query().Get("id")), Path: r.URL.Query().Get("path")})
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
			if path == "wget/process" && strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
				handleWgetProgressStream(w, r)
				return
			}
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
	case "read/website":
		handleWebsiteFileRead(w, req)
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
		key := strings.TrimSpace(string(req.ID))
		if key == "" {
			key = strings.TrimSpace(req.Token)
		}
		if key == "" {
			key = strings.TrimSpace(req.Key)
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
		id := strings.TrimSpace(string(req.ID))
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
		if req.Dst == "" {
			req.Dst = req.Path + ".zip"
		}
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
		items, total, page, size := fileAuxSearch(path, req)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": total, "page": page, "pageSize": size}})
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
		id := strings.TrimSpace(string(req.ID))
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
	case "compress/stop", "decompress/stop", "move/stop":
		fileError(w, http.StatusNotFound, errors.New("异步文件任务不存在或已完成"))
	case "chunkupload/stop":
		id := strings.TrimSpace(req.UploadID)
		if id == "" {
			id = strings.TrimSpace(string(req.ID))
		}
		if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
			fileError(w, http.StatusBadRequest, errors.New("uploadID 无效"))
			return
		}
		root := fileChunkDir()
		if err := os.RemoveAll(filepath.Join(root, id)); err != nil {
			fileError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"stopped": true, "uploadID": id}})
	case "depth/size":
		clean, err := cleanFilePath(req.Path)
		if err != nil {
			fileError(w, 400, err)
			return
		}
		info, err := os.Stat(clean)
		if err != nil || !info.IsDir() {
			if err == nil {
				err = errors.New("路径不是目录")
			}
			fileError(w, 404, err)
			return
		}
		items := make([]map[string]any, 0)
		entries, _ := os.ReadDir(clean)
		for _, entry := range entries {
			p := filepath.Join(clean, entry.Name())
			var size int64
			_ = filepath.Walk(p, func(_ string, i os.FileInfo, e error) error {
				if e == nil && i != nil && !i.IsDir() {
					size += i.Size()
				}
				return nil
			})
			items = append(items, map[string]any{"path": p, "name": entry.Name(), "size": size, "isDir": entry.IsDir()})
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
	case "mode":
		clean, err := cleanFilePath(req.Path)
		if err != nil || req.Mode < 0 || req.Mode > 0o7777 {
			if err == nil {
				err = errors.New("mode 超出范围")
			}
			fileError(w, 400, err)
			return
		}
		if err := os.Chmod(clean, os.FileMode(req.Mode)); err != nil {
			fileError(w, 500, err)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"path": clean, "mode": req.Mode}})
	case "owner":
		clean, err := cleanFilePath(req.Path)
		if err != nil || (req.User == "" && req.Group == "") {
			if err == nil {
				err = errors.New("user 或 group 不能为空")
			}
			fileError(w, http.StatusBadRequest, err)
			return
		}
		if runtime.GOOS == "windows" {
			fileError(w, http.StatusNotImplemented, errors.New("Windows 不支持修改文件 owner"))
			return
		}
		uid, gid := -1, -1
		if req.User != "" {
			u, e := osuser.Lookup(req.User)
			if e != nil {
				fileError(w, http.StatusBadRequest, e)
				return
			}
			uid, _ = strconv.Atoi(u.Uid)
		}
		if req.Group != "" {
			g, e := osuser.LookupGroup(req.Group)
			if e != nil {
				fileError(w, http.StatusBadRequest, e)
				return
			}
			gid, _ = strconv.Atoi(g.Gid)
		}
		if err := os.Chown(clean, uid, gid); err != nil {
			fileError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": clean, "user": req.User, "group": req.Group}})
	case "preview", "content":
		clean, err := cleanFilePath(req.Path)
		if err != nil {
			fileError(w, 400, err)
			return
		}
		f, err := os.Open(clean)
		if err != nil {
			fileError(w, 404, err)
			return
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, 2<<20))
		if err != nil {
			fileError(w, 500, err)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"path": clean, "content": string(b), "size": len(b)}})
	case "read":
		clean, err := cleanFilePath(req.Path)
		if err != nil {
			fileError(w, 400, err)
			return
		}
		b, err := os.ReadFile(clean)
		if err != nil {
			fileError(w, 404, err)
			return
		}
		lines := strings.Split(strings.TrimRight(string(b), "\r\n"), "\n")
		page, size := req.Page, req.PageSize
		if page < 1 {
			page = 1
		}
		if size < 1 || size > 500 {
			size = 100
		}
		start := (page - 1) * size
		if start > len(lines) {
			start = len(lines)
		}
		end := start + size
		if end > len(lines) {
			end = len(lines)
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"path": clean, "lines": lines[start:end], "totalLines": len(lines), "page": page, "pageSize": size, "end": end >= len(lines)}})
	case "remarks":
		fileAux.Lock()
		loadFileAuxLocked()
		out := map[string]string{}
		for _, p := range req.Paths {
			if v, ok := fileAux.data.Remarks[p]; ok {
				out[p] = v
			}
		}
		fileAux.Unlock()
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"remarks": out}})
	case "remark":
		clean, err := cleanFilePath(req.Path)
		if err != nil {
			fileError(w, 400, err)
			return
		}
		fileAux.Lock()
		loadFileAuxLocked()
		if fileAux.data.Remarks == nil {
			fileAux.data.Remarks = map[string]string{}
		}
		fileAux.data.Remarks[clean] = req.Name
		saveErr := saveFileAuxLocked()
		fileAux.Unlock()
		if saveErr != nil {
			fileError(w, 500, saveErr)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"path": clean, "remark": req.Name}})
	case "history/search":
		fileAux.Lock()
		loadFileAuxLocked()
		list := append([]fileHistoryItem(nil), fileAux.data.History...)
		fileAux.Unlock()
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": list, "total": len(list)}})
	case "history/content":
		fileAux.Lock()
		loadFileAuxLocked()
		var found *fileHistoryItem
		for i := range fileAux.data.History {
			if fileAux.data.History[i].ID == string(req.ID) {
				found = &fileAux.data.History[i]
				break
			}
		}
		fileAux.Unlock()
		if found == nil {
			fileError(w, 404, errors.New("历史版本不存在"))
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": found})
	case "history/del":
		fileAux.Lock()
		loadFileAuxLocked()
		kept := fileAux.data.History[:0]
		removed := false
		for _, item := range fileAux.data.History {
			if item.ID == string(req.ID) {
				removed = true
			} else {
				kept = append(kept, item)
			}
		}
		fileAux.data.History = kept
		err := saveFileAuxLocked()
		fileAux.Unlock()
		if err != nil {
			fileError(w, 500, err)
			return
		}
		if !removed {
			fileError(w, 404, errors.New("历史版本不存在"))
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"deleted": true}})
	case "history/restore":
		fileAux.Lock()
		loadFileAuxLocked()
		var found *fileHistoryItem
		for i := range fileAux.data.History {
			if fileAux.data.History[i].ID == string(req.ID) {
				found = &fileAux.data.History[i]
				break
			}
		}
		fileAux.Unlock()
		if found == nil {
			fileError(w, 404, errors.New("历史版本不存在"))
			return
		}
		tmp, e := os.CreateTemp(filepath.Dir(found.Path), ".workmesh-restore-*")
		if e == nil {
			_, e = tmp.WriteString(found.Content)
			_ = tmp.Close()
			if e == nil {
				e = os.Rename(tmp.Name(), found.Path)
			} else {
				_ = os.Remove(tmp.Name())
			}
		}
		if e != nil {
			fileError(w, 500, e)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"path": found.Path, "restored": true}})
	case "share/detail":
		token := strings.TrimSpace(req.Token)
		if token == "" {
			token = strings.TrimSpace(req.Code)
		}
		share, ok := findFileShare(token)
		if !ok {
			fileError(w, http.StatusNotFound, errors.New("分享不存在或文件已删除"))
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": share})
	case "mount":
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": listAlertDisks()})
	case "user/group":
		users := make([]map[string]string, 0, 32)
		if raw, err := os.ReadFile("/etc/passwd"); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				fields := strings.Split(line, ":")
				if len(fields) > 2 {
					users = append(users, map[string]string{"name": fields[0], "uid": fields[2]})
				}
			}
		}
		groups := make([]map[string]string, 0, 32)
		if raw, err := os.ReadFile("/etc/group"); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				fields := strings.Split(line, ":")
				if len(fields) > 2 {
					groups = append(groups, map[string]string{"name": fields[0], "gid": fields[2]})
				}
			}
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"users": users, "groups": groups}})
	case "convert":
		converter := strings.TrimSpace(os.Getenv("WORKMESH_MEDIA_CONVERTER"))
		if converter == "" {
			fileError(w, http.StatusServiceUnavailable, errors.New("未配置媒体转换器，请设置 WORKMESH_MEDIA_CONVERTER"))
			return
		}
		if _, err := exec.LookPath(converter); err != nil {
			fileError(w, http.StatusServiceUnavailable, fmt.Errorf("媒体转换器不可执行: %w", err))
			return
		}
		items := append([]fileConvertItem(nil), req.Files...)
		if len(items) == 0 {
			if req.Path == "" || req.Dst == "" {
				fileError(w, http.StatusBadRequest, errors.New("files 或 path/dst 不能为空"))
				return
			}
			items = []fileConvertItem{{Path: filepath.Dir(req.Path), InputFile: filepath.Base(req.Path), OutputFormat: strings.TrimPrefix(filepath.Ext(req.Dst), "."), Type: req.Type}}
		}
		outputRoot := req.OutputPath
		if outputRoot == "" && req.Dst != "" {
			outputRoot = filepath.Dir(req.Dst)
		}
		outputRoot, err := cleanFilePath(outputRoot)
		if err != nil {
			fileError(w, http.StatusBadRequest, err)
			return
		}
		if err := os.MkdirAll(outputRoot, 0o750); err != nil {
			fileError(w, 500, err)
			return
		}
		taskID := strings.TrimSpace(req.TaskID)
		if taskID == "" {
			taskID = idToken()
		}
		valid := make([]fileConvertItem, 0, len(items))
		for _, item := range items {
			if strings.TrimSpace(item.InputFile) == "" || filepath.Base(item.InputFile) != item.InputFile || strings.ContainsAny(item.InputFile, `/\\`) {
				fileError(w, 400, errors.New("inputFile 无效"))
				return
			}
			base, e := cleanFilePath(filepath.Join(item.Path, item.InputFile))
			if e != nil {
				fileError(w, 400, e)
				return
			}
			if st, e := os.Stat(base); e != nil || !st.Mode().IsRegular() {
				if e == nil {
					e = errors.New("输入文件不是普通文件")
				}
				fileError(w, 404, e)
				return
			}
			if strings.TrimSpace(item.OutputFormat) == "" {
				fileError(w, 400, errors.New("outputFormat 不能为空"))
				return
			}
			valid = append(valid, item)
		}
		for _, item := range valid {
			input := filepath.Join(item.Path, item.InputFile)
			name := strings.TrimSuffix(filepath.Base(item.InputFile), filepath.Ext(item.InputFile)) + "." + strings.TrimPrefix(item.OutputFormat, ".")
			output := filepath.Join(outputRoot, name)
			appendConvertLog(fileConvertLog{Date: time.Now().Format("2006-01-02 15:04:05"), Type: item.Type, Log: fmt.Sprintf("%s -> %s", input, output), Status: "RUNNING", Message: "QUEUED", TaskID: taskID})
			go runMediaConversion(converter, input, output, item.Type, taskID, req.DeleteSource)
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"taskID": taskID, "status": "queued", "total": len(valid)}})
	case "convert/log":
		v, err := requestMap(r)
		if err != nil {
			domainError(w, 400, "INVALID_JSON", err.Error())
			return
		}
		page, size := intValue(v, "page"), intValue(v, "pageSize")
		if page < 1 {
			page = 1
		}
		if size < 1 || size > 200 {
			size = 50
		}
		status, typ, taskID := strings.ToLower(valueString(v, "status")), strings.ToLower(valueString(v, "type")), valueString(v, "taskID")
		fileAux.Lock()
		loadFileAuxLocked()
		all := append([]fileConvertLog(nil), fileAux.data.ConvertLogs...)
		fileAux.Unlock()
		filtered := all[:0]
		for _, item := range all {
			if status != "" && strings.ToLower(item.Status) != status {
				continue
			}
			if typ != "" && strings.ToLower(item.Type) != typ {
				continue
			}
			if taskID != "" && item.TaskID != taskID {
				continue
			}
			filtered = append(filtered, item)
		}
		all = filtered
		total := len(all)
		start := (page - 1) * size
		if start > total {
			start = total
		}
		end := start + size
		if end > total {
			end = total
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": all[start:end], "total": total, "page": page, "pageSize": size}})
	default:
		if operationPath == "read" {
			// 继续复用 read 分页逻辑，路径参数 type 仅用于客户端展示。
			path = "read"
			clean, err := cleanFilePath(req.Path)
			if err != nil {
				fileError(w, http.StatusBadRequest, err)
				return
			}
			b, err := os.ReadFile(clean)
			if err != nil {
				fileError(w, http.StatusNotFound, err)
				return
			}
			lines := strings.Split(strings.TrimRight(string(b), "\r\n"), "\n")
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": clean, "lines": lines, "totalLines": len(lines), "type": strings.TrimPrefix(operationPath, "read/")}})
			return
		}
		fileError(w, http.StatusNotImplemented, fmt.Errorf("文件操作 %q 尚未实现", path))
	}
}

// handleWebsiteFileRead 以网站 ID 解析根目录，并拒绝读取站点目录以外的任何路径。
func handleWebsiteFileRead(w http.ResponseWriter, req fileAdvancedRequest) {
	idText := strings.TrimSpace(string(req.ID))
	id, err := strconv.ParseUint(idText, 10, 32)
	if err != nil || id == 0 {
		fileError(w, http.StatusBadRequest, errors.New("网站 ID 无效"))
		return
	}
	site, err := service.NewWebsiteService("").Get(uint(id))
	if err != nil {
		fileError(w, http.StatusNotFound, errors.New("网站不存在"))
		return
	}
	root := filepath.Clean(site.SiteDir)
	target := root
	if requested := strings.TrimSpace(req.Path); requested != "" {
		if filepath.IsAbs(requested) {
			target = filepath.Clean(requested)
		} else {
			target = filepath.Join(root, requested)
		}
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		fileError(w, http.StatusForbidden, errors.New("路径必须位于网站目录内"))
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		fileError(w, http.StatusNotFound, err)
		return
	}
	if info.IsDir() {
		entries, err := os.ReadDir(target)
		if err != nil {
			fileError(w, http.StatusForbidden, err)
			return
		}
		items := make([]map[string]any, 0, len(entries))
		for _, entry := range entries {
			entryInfo, statErr := entry.Info()
			if statErr != nil {
				continue
			}
			items = append(items, map[string]any{"name": entry.Name(), "path": filepath.Join(target, entry.Name()), "isDir": entry.IsDir(), "size": entryInfo.Size(), "modTime": entryInfo.ModTime()})
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"websiteID": id, "root": root, "path": target, "isDir": true, "items": items}})
		return
	}
	if info.Size() > 4<<20 {
		fileError(w, http.StatusRequestEntityTooLarge, errors.New("文件超过 4 MiB 读取限制"))
		return
	}
	data, err := os.ReadFile(target)
	if err != nil {
		fileError(w, http.StatusForbidden, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"websiteID": id, "root": root, "path": target, "isDir": false, "content": string(data), "size": info.Size()}})
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
						fileWgetState.Lock()
						proc.Total, proc.Downloaded = resp.ContentLength, 0
						fileWgetState.Unlock()
						// 每次写入后更新已下载字节，WebSocket 进度查询可实时反映长任务状态。
						counter := &wgetProgressWriter{dst: out, key: key}
						_, reqErr = io.Copy(counter, resp.Body)
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
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".workmesh-zip-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	out := tmp
	zw := zip.NewWriter(out)
	err = filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
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
	if closeErr := zw.Close(); err == nil {
		err = closeErr
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, destination)
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
		if item.FileInfo().Mode()&os.ModeSymlink != 0 {
			return errors.New("压缩包不允许包含符号链接")
		}
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
		// 每个条目先写同目录临时文件，完成后原子替换目标，避免中断留下半文件。
		tmp, err := os.CreateTemp(filepath.Dir(target), ".workmesh-unzip-*")
		if err == nil {
			_, err = io.Copy(tmp, in)
			if closeErr := tmp.Close(); err == nil {
				err = closeErr
			}
			if err == nil {
				err = os.Rename(tmp.Name(), target)
			} else {
				_ = os.Remove(tmp.Name())
			}
		}
		_ = in.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

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
		if err := os.Rename(partPath, dstFile); err != nil {
			fileError(w, 500, err)
			return
		}
		_ = os.RemoveAll(workDir)
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"uploadID": uploadID, "chunkIndex": chunkIndex, "chunkCount": chunkCount, "completed": done, "path": dstFile}})
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
