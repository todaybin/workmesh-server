// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type fileAdvancedRequest struct {
	Path              string          `json:"path"`
	Dst               string          `json:"dst"`
	URL               string          `json:"url"`
	Name              string          `json:"name"`
	Token             string          `json:"token"`
	Key               string          `json:"key"`
	ID                flexibleID      `json:"id"`
	SSLID             flexibleID      `json:"sslID"`
	WebsiteSSLID      flexibleID      `json:"websiteSSLId"`
	Code              string          `json:"code"`
	Query             string          `json:"query"`
	Paths             []string        `json:"paths"`
	IDs               []flexibleID    `json:"ids"`
	Mode              int64           `json:"mode"`
	User              string          `json:"user"`
	Group             string          `json:"group"`
	WithInit          bool            `json:"withInit"`
	ContainSub        bool            `json:"containSub"`
	Sub               bool            `json:"sub"`
	MatchCase         bool            `json:"matchCase"`
	WholeWord         bool            `json:"wholeWord"`
	UseRegex          bool            `json:"useRegex"`
	MaxScanFiles      int             `json:"maxScanFiles"`
	MaxFileBytes      int64           `json:"maxFileBytes"`
	MaxHitsPerFile    int             `json:"maxHitsPerFile"`
	MaxTotalHits      int             `json:"maxTotalHits"`
	IgnoreCertificate bool            `json:"ignoreCertificate"`
	Page              int             `json:"page"`
	PageSize          int             `json:"pageSize"`
	UploadID          string          `json:"uploadID"`
	ChunkIndex        int             `json:"chunkIndex"`
	ChunkCount        int             `json:"chunkCount"`
	Offset            int64           `json:"offset"`
	FileSize          int64           `json:"fileSize"`
	Overwrite         bool            `json:"overwrite"`
	Replace           bool            `json:"replace"`
	DeleteSource      bool            `json:"deleteSource"`
	OutputPath        string          `json:"outputPath"`
	TaskID            string          `json:"taskID"`
	Type              string          `json:"type"`
	Extension         string          `json:"extension"`
	OutputFormat      string          `json:"outputFormat"`
	Files             json.RawMessage `json:"files"`
	Remark            string          `json:"remark"`
	Scope             string          `json:"scope"`
	Operation         string          `json:"operation"`
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
	ID          string    `json:"id"`
	FileID      string    `json:"fileId,omitempty"`
	Path        string    `json:"path"`
	CurrentPath string    `json:"currentPath,omitempty"`
	FileName    string    `json:"fileName,omitempty"`
	Extension   string    `json:"extension,omitempty"`
	FileMode    string    `json:"fileMode,omitempty"`
	Operation   string    `json:"operation,omitempty"`
	Deleted     bool      `json:"deleted,omitempty"`
	ContentSize int64     `json:"contentSize,omitempty"`
	ContentSHA  string    `json:"contentSHA,omitempty"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
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
	if sharedDB() != nil {
		if !loadJSONState("file_aux_state", &fileAux.data) {
			if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 && json.Unmarshal(raw, &fileAux.data) == nil {
				if saveErr := saveJSONState("file_aux_state", fileAux.data); saveErr == nil {
					archiveDir := filepath.Join(filepath.Dir(path), "backups")
					if os.MkdirAll(archiveDir, 0o750) == nil {
						_ = os.Rename(path, filepath.Join(archiveDir, "legacy-files-"+time.Now().UTC().Format("20060102T150405.000000000Z")+".json"))
					}
				}
			}
		}
	} else if raw, err := os.ReadFile(path); err == nil {
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
	if sharedDB() != nil {
		return saveJSONState("file_aux_state", fileAux.data)
	}
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
	if sharedDB() != nil {
		if !loadJSONState("file_shares_state", &fileShareState.items) {
			if b, err := os.ReadFile(fileShareFile()); err == nil && len(b) > 0 && json.Unmarshal(b, &fileShareState.items) == nil {
				if saveErr := saveJSONState("file_shares_state", fileShareState.items); saveErr == nil {
					archiveDir := filepath.Join(filepath.Dir(fileShareFile()), "backups")
					if os.MkdirAll(archiveDir, 0o750) == nil {
						_ = os.Rename(fileShareFile(), filepath.Join(archiveDir, "legacy-file-shares-"+time.Now().UTC().Format("20060102T150405.000000000Z")+".json"))
					}
				}
			}
		}
	} else if b, err := os.ReadFile(fileShareFile()); err == nil {
		_ = json.Unmarshal(b, &fileShareState.items)
	}
}

func saveFileSharesLocked() error {
	if sharedDB() != nil {
		return saveJSONState("file_shares_state", fileShareState.items)
	}
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
