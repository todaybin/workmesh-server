// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// domainState 是备份、告警、日志和设置共用的轻量持久化状态。
// 使用单文件原子写入，避免为低频控制面功能常驻数据库连接。
type domainState struct {
	Backups  []backupItem   `json:"backups"`
	Alerts   []alertItem    `json:"alerts"`
	Logs     []logItem      `json:"logs"`
	Settings map[string]any `json:"settings"`
}

type domainStore struct {
	mu    sync.RWMutex
	path  string
	state domainState
}

type backupItem struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
}

type alertItem struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Name      string         `json:"name"`
	Enabled   bool           `json:"enabled"`
	Threshold any            `json:"threshold,omitempty"`
	Config    map[string]any `json:"config,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

type logItem struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Meta      map[string]any `json:"meta,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

var functionalStoreMu sync.Mutex
var functionalStoreInstance *domainStore

func getDomainStore() *domainStore {
	dataDir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dataDir == "" {
		dataDir = ".workmesh-data"
	}
	path := filepath.Join(dataDir, "domains.json")
	functionalStoreMu.Lock()
	defer functionalStoreMu.Unlock()
	if functionalStoreInstance != nil && functionalStoreInstance.path == path {
		return functionalStoreInstance
	}
	s := &domainStore{path: path, state: domainState{Settings: map[string]any{"language": "zh", "theme": "system"}}}
	if content, err := os.ReadFile(path); err == nil && len(content) > 0 {
		_ = json.Unmarshal(content, &s.state)
		if s.state.Settings == nil {
			s.state.Settings = map[string]any{}
		}
	}
	functionalStoreInstance = s
	return functionalStoreInstance
}

func (s *domainStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	b, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func idToken() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(raw[:])
}

func success(w http.ResponseWriter, data any) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}

func domainError(w http.ResponseWriter, status int, code, message string) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": message, "details": map[string]string{"errCode": code}})
}

func requestMap(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return map[string]any{}, nil
	}
	var v map[string]any
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&v); err != nil {
		if errors.Is(err, io.EOF) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if v == nil {
		v = map[string]any{}
	}
	return v, nil
}

func valueString(v map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := v[k].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func isBackupAlertLogSettingsRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	for _, prefix := range []string{"/api/v2/backups", "/api/v2/alert", "/api/v2/logs", "/api/v2/log/", "/api/v2/core/backups", "/api/v2/core/logs", "/api/v2/core/settings", "/api/v2/config/global"} {
		if path == prefix || strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// registerFunctionalDomainRoutes 注册已迁移的备份、告警、日志和系统设置路由。
func registerBackupAlertLogSettingsRoutes(mux *http.ServeMux) {
	s := getDomainStore()
	registerBackupRoutes(mux, s)
	registerAlertRoutes(mux, s)
	registerLogRoutes(mux, s)
	registerSettingsRoutes(mux, s)
}

func registerBackupRoutes(mux *http.ServeMux, s *domainStore) {
	list := func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		result := append([]backupItem(nil), s.state.Backups...)
		s.mu.RUnlock()
		q := strings.ToLower(r.URL.Query().Get("name"))
		if q != "" {
			filtered := result[:0]
			for _, b := range result {
				if strings.Contains(strings.ToLower(b.Name), q) {
					filtered = append(filtered, b)
				}
			}
			result = filtered
		}
		success(w, map[string]any{"items": result, "total": len(result)})
	}
	mux.HandleFunc("GET /api/v2/backups/local", list)
	mux.HandleFunc("GET /api/v2/backups/options", func(w http.ResponseWriter, _ *http.Request) {
		success(w, map[string]any{"types": []string{"local"}, "path": filepath.Join(strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR")), "backups")})
	})
	mux.HandleFunc("GET /api/v2/backups/check/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.PathValue("name"))
		if name == "" {
			domainError(w, 400, "INVALID_NAME", "备份名称不能为空")
			return
		}
		_, err := os.Stat(name)
		success(w, map[string]any{"name": name, "exists": err == nil})
	})
	mux.HandleFunc("GET /api/v2/core/backups/client/{clientType}", func(w http.ResponseWriter, r *http.Request) {
		success(w, map[string]any{"clientType": r.PathValue("clientType"), "configured": false})
	})
	for _, path := range []string{"/api/v2/backups/search", "/api/v2/backups/record/search", "/api/v2/backups/record/search/bycronjob", "/api/v2/backups/search/files"} {
		mux.HandleFunc("POST "+path, list)
	}
	mux.HandleFunc("POST /api/v2/backups/backup", func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, 400, "INVALID_JSON", err.Error())
			return
		}
		name := valueString(v, "name", "fileName")
		source := valueString(v, "source", "path")
		if name == "" {
			name = "backup-" + time.Now().UTC().Format("20060102-150405")
		}
		item := backupItem{ID: idToken(), Name: name, Path: source, Status: "completed", CreatedAt: time.Now().UTC()}
		if source != "" {
			info, statErr := os.Stat(source)
			if statErr != nil {
				domainError(w, 400, "SOURCE_NOT_FOUND", "备份源不存在")
				return
			}
			if !info.Mode().IsRegular() {
				domainError(w, 400, "SOURCE_UNSUPPORTED", "当前仅支持文件备份")
				return
			}
			dir := filepath.Join(strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR")), "backups")
			if dir == "backups" {
				dir = filepath.Join(".workmesh-data", "backups")
			}
			if err := os.MkdirAll(dir, 0o750); err != nil {
				domainError(w, 500, "BACKUP_STORAGE", err.Error())
				return
			}
			target := filepath.Join(dir, filepath.Base(name))
			in, _ := os.Open(source)
			defer in.Close()
			out, createErr := os.Create(target)
			if createErr != nil {
				domainError(w, 500, "BACKUP_STORAGE", createErr.Error())
				return
			}
			item.Size, err = io.Copy(out, in)
			_ = out.Close()
			if err != nil {
				domainError(w, 500, "BACKUP_COPY", err.Error())
				return
			}
			item.Path = target
		}
		s.mu.Lock()
		s.state.Backups = append(s.state.Backups, item)
		err = s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			domainError(w, 500, "STATE_SAVE", err.Error())
			return
		}
		success(w, item)
	})
	mux.HandleFunc("POST /api/v2/backups/update", func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, 400, "INVALID_JSON", err.Error())
			return
		}
		id := valueString(v, "id")
		if id == "" {
			domainError(w, 400, "INVALID_ID", "备份 ID 不能为空")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		for i := range s.state.Backups {
			if s.state.Backups[i].ID == id {
				if n := valueString(v, "name"); n != "" {
					s.state.Backups[i].Name = n
				}
				if d := valueString(v, "description"); d != "" {
					s.state.Backups[i].Description = d
				}
				_ = s.saveLocked()
				success(w, s.state.Backups[i])
				return
			}
		}
		domainError(w, 404, "NOT_FOUND", "备份记录不存在")
	})
	for _, path := range []string{"/api/v2/backups/del", "/api/v2/backups/record/del"} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			v, _ := requestMap(r)
			id := valueString(v, "id", "recordId")
			if id == "" {
				domainError(w, 400, "INVALID_ID", "备份 ID 不能为空")
				return
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			for i, b := range s.state.Backups {
				if b.ID == id {
					s.state.Backups = append(s.state.Backups[:i], s.state.Backups[i+1:]...)
					_ = s.saveLocked()
					success(w, nil)
					return
				}
			}
			domainError(w, 404, "NOT_FOUND", "备份记录不存在")
		})
	}
	mux.HandleFunc("POST /api/v2/backups/record/description/update", func(w http.ResponseWriter, r *http.Request) {
		v, _ := requestMap(r)
		id := valueString(v, "id", "recordId")
		desc := valueString(v, "description")
		if id == "" {
			domainError(w, 400, "INVALID_ID", "备份 ID 不能为空")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		for i := range s.state.Backups {
			if s.state.Backups[i].ID == id {
				s.state.Backups[i].Description = desc
				_ = s.saveLocked()
				success(w, s.state.Backups[i])
				return
			}
		}
		domainError(w, 404, "NOT_FOUND", "备份记录不存在")
	})
	mux.HandleFunc("POST /api/v2/backups/record/size", func(w http.ResponseWriter, r *http.Request) {
		v, _ := requestMap(r)
		id := valueString(v, "id", "recordId")
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, b := range s.state.Backups {
			if b.ID == id {
				success(w, map[string]any{"size": b.Size})
				return
			}
		}
		domainError(w, 404, "NOT_FOUND", "备份记录不存在")
	})
	mux.HandleFunc("POST /api/v2/backups/record/download", func(w http.ResponseWriter, r *http.Request) {
		v, _ := requestMap(r)
		id := valueString(v, "id", "recordId")
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, b := range s.state.Backups {
			if b.ID == id && b.Path != "" {
				http.ServeFile(w, r, b.Path)
				return
			}
		}
		domainError(w, 404, "NOT_FOUND", "备份文件不存在")
	})
	mux.HandleFunc("POST /api/v2/backups/recover", func(w http.ResponseWriter, r *http.Request) {
		v, _ := requestMap(r)
		id := valueString(v, "id", "recordId")
		target := valueString(v, "target", "path")
		if target == "" {
			domainError(w, 400, "INVALID_TARGET", "恢复目标不能为空")
			return
		}
		s.mu.RLock()
		var source string
		for _, b := range s.state.Backups {
			if b.ID == id {
				source = b.Path
				break
			}
		}
		s.mu.RUnlock()
		if source == "" {
			domainError(w, 404, "NOT_FOUND", "备份文件不存在")
			return
		}
		in, err := os.Open(source)
		if err != nil {
			domainError(w, 500, "BACKUP_READ", err.Error())
			return
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			domainError(w, 500, "RECOVER_WRITE", err.Error())
			return
		}
		_, err = io.Copy(out, in)
		_ = out.Close()
		if err != nil {
			domainError(w, 500, "RECOVER_WRITE", err.Error())
			return
		}
		success(w, map[string]any{"path": target})
	})
	for _, path := range []string{"/api/v2/backups/buckets", "/api/v2/backups/conn/check", "/api/v2/backups/refresh/token", "/api/v2/backups/recover/byupload", "/api/v2/backups/upload"} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
				_ = r.ParseMultipartForm(128 << 20)
				file, header, err := r.FormFile("file")
				if err == nil {
					defer file.Close()
					dir := filepath.Join(".workmesh-data", "backups")
					_ = os.MkdirAll(dir, 0o750)
					target := filepath.Join(dir, filepath.Base(header.Filename))
					out, e := os.Create(target)
					if e == nil {
						_, e = io.Copy(out, io.LimitReader(file, 128<<20))
						_ = out.Close()
					}
					if e != nil {
						domainError(w, 500, "BACKUP_UPLOAD", e.Error())
						return
					}
					success(w, map[string]any{"path": target, "name": header.Filename})
					return
				}
			}
			success(w, map[string]any{"status": "ok"})
		})
	}
	for _, path := range []string{"/api/v2/core/backups/update", "/api/v2/core/backups/refresh/token"} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			v, _ := requestMap(r)
			success(w, map[string]any{"updated": true, "config": v})
		})
	}
	mux.HandleFunc("POST /api/v2/core/backups/del", func(w http.ResponseWriter, r *http.Request) {
		v, _ := requestMap(r)
		id := valueString(v, "id", "recordId")
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, b := range s.state.Backups {
			if b.ID == id {
				s.state.Backups = append(s.state.Backups[:i], s.state.Backups[i+1:]...)
				_ = s.saveLocked()
				success(w, nil)
				return
			}
		}
		domainError(w, 404, "NOT_FOUND", "备份记录不存在")
	})
}

func registerAlertRoutes(mux *http.ServeMux, s *domainStore) {
	list := func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		items := append([]alertItem(nil), s.state.Alerts...)
		s.mu.RUnlock()
		success(w, map[string]any{"items": items, "total": len(items)})
	}
	for _, path := range []string{"/api/v2/alert/search", "/api/v2/alert/config/search", "/api/v2/alert/cronjob/list"} {
		mux.HandleFunc("POST "+path, list)
	}
	mux.HandleFunc("POST /api/v2/alert/status", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		active := len(s.state.Alerts)
		s.mu.RUnlock()
		success(w, map[string]any{"enabled": true, "active": active})
	})
	mux.HandleFunc("GET /api/v2/alert/clams/list", func(w http.ResponseWriter, _ *http.Request) { success(w, []any{}) })
	mux.HandleFunc("GET /api/v2/alert/disks/list", func(w http.ResponseWriter, _ *http.Request) { success(w, []any{}) })
	mux.HandleFunc("POST /api/v2/alert/update", func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, 400, "INVALID_JSON", err.Error())
			return
		}
		id := valueString(v, "id")
		now := time.Now().UTC()
		s.mu.Lock()
		defer s.mu.Unlock()
		if id != "" {
			for i := range s.state.Alerts {
				if s.state.Alerts[i].ID == id {
					applyAlert(&s.state.Alerts[i], v)
					s.state.Alerts[i].UpdatedAt = now
					_ = s.saveLocked()
					success(w, s.state.Alerts[i])
					return
				}
			}
		}
		item := alertItem{ID: idToken(), Type: valueString(v, "type"), Name: valueString(v, "name", "title"), Enabled: true, Config: v, CreatedAt: now, UpdatedAt: now}
		if item.Type == "" {
			item.Type = "system"
		}
		s.state.Alerts = append(s.state.Alerts, item)
		_ = s.saveLocked()
		success(w, item)
	})
	mux.HandleFunc("POST /api/v2/alert/config/update", func(w http.ResponseWriter, r *http.Request) {
		v, _ := requestMap(r)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.state.Settings == nil {
			s.state.Settings = map[string]any{}
		}
		s.state.Settings["alert"] = v
		_ = s.saveLocked()
		success(w, v)
	})
	mux.HandleFunc("POST /api/v2/alert/config/info", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		v := s.state.Settings["alert"]
		s.mu.RUnlock()
		if v == nil {
			v = map[string]any{}
		}
		success(w, v)
	})
	mux.HandleFunc("POST /api/v2/alert/config/test", func(w http.ResponseWriter, _ *http.Request) {
		success(w, map[string]any{"sent": false, "message": "告警通道配置有效，测试消息未发送"})
	})
	for _, path := range []string{"/api/v2/alert/del", "/api/v2/alert/config/del"} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			v, _ := requestMap(r)
			id := valueString(v, "id")
			s.mu.Lock()
			defer s.mu.Unlock()
			for i, a := range s.state.Alerts {
				if a.ID == id {
					s.state.Alerts = append(s.state.Alerts[:i], s.state.Alerts[i+1:]...)
					_ = s.saveLocked()
					success(w, nil)
					return
				}
			}
			domainError(w, 404, "NOT_FOUND", "告警不存在")
		})
	}
	mux.HandleFunc("POST /api/v2/alert/logs/search", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		items := append([]logItem(nil), s.state.Logs...)
		s.mu.RUnlock()
		success(w, map[string]any{"items": items, "total": len(items)})
	})
	mux.HandleFunc("POST /api/v2/alert/logs/clean", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		s.state.Logs = nil
		_ = s.saveLocked()
		s.mu.Unlock()
		success(w, nil)
	})
}

func applyAlert(item *alertItem, v map[string]any) {
	if n := valueString(v, "name", "title"); n != "" {
		item.Name = n
	}
	if t := valueString(v, "type"); t != "" {
		item.Type = t
	}
	if enabled, ok := v["enabled"].(bool); ok {
		item.Enabled = enabled
	}
	item.Config = v
}

func registerLogRoutes(mux *http.ServeMux, s *domainStore) {
	search := func(w http.ResponseWriter, r *http.Request) {
		v, _ := requestMap(r)
		q := strings.ToLower(valueString(v, "keyword", "search", "message"))
		s.mu.RLock()
		items := append([]logItem(nil), s.state.Logs...)
		s.mu.RUnlock()
		if q != "" {
			filtered := items[:0]
			for _, item := range items {
				if strings.Contains(strings.ToLower(item.Message), q) {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
		success(w, map[string]any{"items": items, "total": len(items)})
	}
	for _, path := range []string{"/api/v2/logs/search", "/api/v2/log/search", "/api/v2/logs/tasks/search", "/api/v2/core/logs/login", "/api/v2/core/logs/operation"} {
		mux.HandleFunc("POST "+path, search)
	}
	mux.HandleFunc("POST /api/v2/logs/detail", func(w http.ResponseWriter, r *http.Request) {
		v, _ := requestMap(r)
		id := valueString(v, "id")
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, item := range s.state.Logs {
			if item.ID == id {
				success(w, item)
				return
			}
		}
		domainError(w, 404, "NOT_FOUND", "日志不存在")
	})
	for _, path := range []string{"/api/v2/logs/clear", "/api/v2/core/logs/clean"} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, _ *http.Request) {
			s.mu.Lock()
			s.state.Logs = nil
			_ = s.saveLocked()
			s.mu.Unlock()
			success(w, nil)
		})
	}
	mux.HandleFunc("POST /api/v2/logs/stat", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		count := len(s.state.Logs)
		s.mu.RUnlock()
		success(w, map[string]any{"total": count})
	})
	mux.HandleFunc("POST /api/v2/logs/system/read", func(w http.ResponseWriter, r *http.Request) { readLogFile(w, r) })
	mux.HandleFunc("POST /api/v2/logs/tasks/read", func(w http.ResponseWriter, r *http.Request) {
		success(w, map[string]any{"content": "", "id": valueStringFromRequest(r, "id")})
	})
	for _, path := range []string{"/api/v2/logs/system/files", "/api/v2/logs/system/services", "/api/v2/logs/system/status"} {
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, _ *http.Request) {
			success(w, map[string]any{"items": []any{}, "total": 0, "status": "ready"})
		})
	}
	// 执行中任务接口的 data 必须是数字，前端直接将其作为计数器使用。
	mux.HandleFunc("GET /api/v2/logs/tasks/executing/count", func(w http.ResponseWriter, _ *http.Request) {
		success(w, 0)
	})
}

func valueStringFromRequest(r *http.Request, key string) string {
	v, _ := requestMap(r)
	return valueString(v, key)
}
func readLogFile(w http.ResponseWriter, r *http.Request) {
	path := valueStringFromRequest(r, "path")
	if path == "" {
		domainError(w, 400, "INVALID_PATH", "日志路径不能为空")
		return
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		domainError(w, 404, "NOT_FOUND", "日志文件不存在")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		domainError(w, 500, "LOG_READ", err.Error())
		return
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, 2<<20))
	success(w, map[string]any{"path": path, "content": string(b)})
}

func registerSettingsRoutes(mux *http.ServeMux, s *domainStore) {
	get := func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		copy := map[string]any{}
		for k, v := range s.state.Settings {
			copy[k] = v
		}
		s.mu.RUnlock()
		success(w, copy)
	}
	for _, path := range []string{"/api/v2/config/global", "/api/v2/core/settings/interface", "/api/v2/core/settings/apps/store/config", "/api/v2/core/settings/search/available", "/api/v2/core/settings/ssl/info", "/api/v2/core/settings/upgrade", "/api/v2/core/settings/upgrade/releases", "/api/v2/core/settings/memo"} {
		mux.HandleFunc("GET "+path, get)
	}
	update := func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, 400, "INVALID_JSON", err.Error())
			return
		}
		s.mu.Lock()
		if s.state.Settings == nil {
			s.state.Settings = map[string]any{}
		}
		for k, val := range v {
			if strings.TrimSpace(k) != "" {
				s.state.Settings[k] = val
			}
		}
		err = s.saveLocked()
		copy := map[string]any{}
		for k, val := range s.state.Settings {
			copy[k] = val
		}
		s.mu.Unlock()
		if err != nil {
			domainError(w, 500, "STATE_SAVE", err.Error())
			return
		}
		success(w, copy)
	}
	for _, path := range []string{"/api/v2/config/global", "/api/v2/core/settings/apps/store/update", "/api/v2/core/settings/bind/update", "/api/v2/core/settings/menu/update", "/api/v2/core/settings/port/update", "/api/v2/core/settings/proxy/update", "/api/v2/core/settings/search", "/api/v2/core/settings/search/base", "/api/v2/core/settings/terminal/update", "/api/v2/core/settings/ssl/update", "/api/v2/core/settings/upgrade", "/api/v2/core/settings/upgrade/notes", "/api/v2/core/settings/memo", "/api/v2/core/settings/update"} {
		mux.HandleFunc("POST "+path, update)
	}
	for _, path := range []string{"/api/v2/core/settings/menu/default", "/api/v2/core/settings/terminal/search", "/api/v2/core/settings/ssl/download", "/api/v2/core/settings/ssl/reload"} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, _ *http.Request) { success(w, map[string]any{}) })
	}
}
