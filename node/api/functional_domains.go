// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	// Backups 保留为记录集合，兼容早期版本 domains.json。
	Backups []backupItem `json:"backups"`
	// BackupAccounts 是备份账号配置；账号和记录必须分离，避免账号列表混入任务记录。
	BackupAccounts []backupAccount   `json:"backupAccounts,omitempty"`
	Alerts         []alertItem       `json:"alerts"`
	Logs           []logItem         `json:"logs"`
	Settings       map[string]any    `json:"settings"`
	Snapshots      []settingSnapshot `json:"snapshots"`
}

// settingSnapshot 保存设置快照，供回滚与导入导出接口使用。
type settingSnapshot struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Data        map[string]any `json:"data"`
	CreatedAt   time.Time      `json:"createdAt"`
}

type domainStore struct {
	mu    sync.RWMutex
	path  string
	state domainState
}

type backupItem struct {
	ID                string    `json:"id"`
	Type              string    `json:"type,omitempty"`
	Name              string    `json:"name"`
	DetailName        string    `json:"detailName,omitempty"`
	Path              string    `json:"path"`
	FileDir           string    `json:"fileDir,omitempty"`
	FileName          string    `json:"fileName,omitempty"`
	AccountType       string    `json:"accountType,omitempty"`
	AccountName       string    `json:"accountName,omitempty"`
	DownloadAccountID string    `json:"downloadAccountID,omitempty"`
	CronjobID         string    `json:"cronjobID,omitempty"`
	TaskID            string    `json:"taskID,omitempty"`
	Size              int64     `json:"size"`
	Description       string    `json:"description,omitempty"`
	Status            string    `json:"status"`
	Message           string    `json:"message,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
}

// backupAccount 对应旧系统 BackupAccount，敏感凭据只在内存和受保护状态文件中保存。
type backupAccount struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	IsPublic     bool      `json:"isPublic"`
	Bucket       string    `json:"bucket,omitempty"`
	AccessKey    string    `json:"accessKey,omitempty"`
	Credential   string    `json:"credential,omitempty"`
	BackupPath   string    `json:"backupPath,omitempty"`
	Vars         string    `json:"vars,omitempty"`
	RememberAuth bool      `json:"rememberAuth"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
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
	for _, prefix := range []string{"/api/v2/backups", "/api/v2/alert", "/api/v2/logs", "/api/v2/log/", "/api/v2/core/backups", "/api/v2/core/logs", "/api/v2/core/settings", "/api/v2/settings", "/api/v2/config/global"} {
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
	// 账号列表是 /backups/search 的语义；记录列表使用独立的 record/search。
	accountList := func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, 400, "INVALID_JSON", err.Error())
			return
		}
		q := strings.ToLower(valueString(v, "name", "info"))
		s.mu.RLock()
		accounts := append([]backupAccount(nil), s.state.BackupAccounts...)
		s.mu.RUnlock()
		if q != "" {
			filtered := accounts[:0]
			for _, a := range accounts {
				if strings.Contains(strings.ToLower(a.Name), q) || strings.Contains(strings.ToLower(a.Type), q) {
					filtered = append(filtered, a)
				}
			}
			accounts = filtered
		}
		items := make([]backupAccount, 0, len(accounts)+1)
		items = append(items, accounts...)
		if !containsBackupAccount(accounts, "local", "localhost") {
			items = append(items, backupAccount{ID: "local", Name: "localhost", Type: "local", IsPublic: false, BackupPath: backupDataDir(), RememberAuth: false})
		}
		total := len(items)
		page, pageSize := intValue(v, "page"), intValue(v, "pageSize")
		if page < 1 {
			page = 1
		}
		if pageSize <= 0 || pageSize > 200 {
			pageSize = 50
		}
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		items = items[start:end]
		for i := range items {
			items[i] = sanitizeBackupAccount(items[i])
		}
		success(w, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
	}
	mux.HandleFunc("GET /api/v2/backups/local", func(w http.ResponseWriter, _ *http.Request) { success(w, backupDataDir()) })
	mux.HandleFunc("GET /api/v2/backups/options", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		accounts := append([]backupAccount(nil), s.state.BackupAccounts...)
		s.mu.RUnlock()
		options := make([]map[string]any, 0, len(accounts)+1)
		options = append(options, map[string]any{"id": "local", "name": "localhost", "type": "local", "isPublic": false})
		for _, a := range accounts {
			options = append(options, map[string]any{"id": a.ID, "name": a.Name, "type": a.Type, "isPublic": a.IsPublic})
		}
		success(w, options)
	})
	mux.HandleFunc("GET /api/v2/backups/check/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.PathValue("name"))
		if name == "" {
			domainError(w, 400, "INVALID_NAME", "备份名称不能为空")
			return
		}
		s.mu.RLock()
		account := findBackupAccount(s.state.BackupAccounts, "", name)
		used := false
		if account != nil {
			for _, rec := range s.state.Backups {
				if rec.DownloadAccountID == account.ID {
					used = true
					break
				}
			}
		}
		s.mu.RUnlock()
		success(w, map[string]any{"name": name, "exists": account != nil, "used": used, "inUse": used})
	})
	mux.HandleFunc("GET /api/v2/core/backups/client/{clientType}", func(w http.ResponseWriter, r *http.Request) {
		clientType := strings.ToUpper(strings.TrimSpace(r.PathValue("clientType")))
		prefix := "WORKMESH_BACKUP_" + strings.NewReplacer("-", "_", " ", "_").Replace(clientType)
		id, secret, redirect := os.Getenv(prefix+"_CLIENT_ID"), os.Getenv(prefix+"_CLIENT_SECRET"), os.Getenv(prefix+"_REDIRECT_URI")
		success(w, map[string]any{"clientType": strings.ToLower(clientType), "client_id": id, "client_secret": secret, "redirect_uri": redirect, "configured": id != "" || secret != "" || redirect != ""})
	})
	mux.HandleFunc("POST /api/v2/backups/search", accountList)
	mux.HandleFunc("POST /api/v2/backups", func(w http.ResponseWriter, r *http.Request) { handleBackupAccountCreate(w, r, s) })
	// 公共账号由 Core 路径管理，私有账号由 Agent 路径管理；两者共用同一轻量存储。
	mux.HandleFunc("POST /api/v2/core/backups", func(w http.ResponseWriter, r *http.Request) { handleBackupAccountCreate(w, r, s) })
	mux.HandleFunc("POST /api/v2/core/backups/update", func(w http.ResponseWriter, r *http.Request) { handleBackupAccountUpdate(w, r, s) })
	mux.HandleFunc("POST /api/v2/backups/update", func(w http.ResponseWriter, r *http.Request) { handleBackupAccountUpdate(w, r, s) })
	mux.HandleFunc("POST /api/v2/core/backups/del", func(w http.ResponseWriter, r *http.Request) { handleBackupAccountDelete(w, r, s) })
	mux.HandleFunc("POST /api/v2/backups/del", func(w http.ResponseWriter, r *http.Request) { handleBackupAccountDelete(w, r, s) })

	// 创建备份任务时生成真实记录；source/path/filePath 为本地文件时复制到受控目录。
	mux.HandleFunc("POST /api/v2/backups/backup", func(w http.ResponseWriter, r *http.Request) { handleBackupCreateRecord(w, r, s) })
	mux.HandleFunc("POST /api/v2/backups/record/search", func(w http.ResponseWriter, r *http.Request) { handleBackupRecordSearch(w, r, s, "") })
	mux.HandleFunc("POST /api/v2/backups/record/search/bycronjob", func(w http.ResponseWriter, r *http.Request) { handleBackupRecordSearch(w, r, s, "cronjob") })
	mux.HandleFunc("POST /api/v2/backups/search/files", func(w http.ResponseWriter, r *http.Request) { handleBackupFiles(w, r, s) })
	mux.HandleFunc("POST /api/v2/backups/record/del", func(w http.ResponseWriter, r *http.Request) { handleBackupRecordDelete(w, r, s) })
	mux.HandleFunc("POST /api/v2/backups/del-record", func(w http.ResponseWriter, r *http.Request) { handleBackupRecordDelete(w, r, s) })
	mux.HandleFunc("POST /api/v2/backups/record/description/update", func(w http.ResponseWriter, r *http.Request) { handleBackupRecordDescription(w, r, s) })
	mux.HandleFunc("POST /api/v2/backups/record/size", func(w http.ResponseWriter, r *http.Request) { handleBackupRecordSize(w, r, s) })
	mux.HandleFunc("POST /api/v2/backups/record/download", func(w http.ResponseWriter, r *http.Request) { handleBackupRecordDownload(w, r, s) })
	mux.HandleFunc("POST /api/v2/backups/recover", func(w http.ResponseWriter, r *http.Request) { handleBackupRecover(w, r, s, false) })
	mux.HandleFunc("POST /api/v2/backups/recover/byupload", func(w http.ResponseWriter, r *http.Request) { handleBackupRecover(w, r, s, true) })
	mux.HandleFunc("POST /api/v2/backups/upload", func(w http.ResponseWriter, r *http.Request) { handleBackupUpload(w, r, s) })
	mux.HandleFunc("POST /api/v2/backups/buckets", func(w http.ResponseWriter, r *http.Request) { handleBackupBuckets(w, r, s) })
	mux.HandleFunc("POST /api/v2/backups/conn/check", func(w http.ResponseWriter, r *http.Request) { handleBackupConnCheck(w, r, s) })
	mux.HandleFunc("POST /api/v2/backups/refresh/token", func(w http.ResponseWriter, r *http.Request) { handleBackupRefreshToken(w, r, s) })
	mux.HandleFunc("POST /api/v2/core/backups/refresh/token", func(w http.ResponseWriter, r *http.Request) { handleBackupRefreshToken(w, r, s) })
}

func backupDataDir() string {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = ".workmesh-data"
	}
	return filepath.Join(root, "backups")
}

func containsBackupAccount(accounts []backupAccount, typ, name string) bool {
	for _, item := range accounts {
		if strings.EqualFold(item.Type, typ) || strings.EqualFold(item.Name, name) {
			return true
		}
	}
	return false
}

func findBackupAccount(accounts []backupAccount, id, name string) *backupAccount {
	for i := range accounts {
		if (id != "" && accounts[i].ID == id) || (name != "" && strings.EqualFold(accounts[i].Name, name)) {
			return &accounts[i]
		}
	}
	return nil
}

func sanitizeBackupAccount(item backupAccount) backupAccount {
	if !item.RememberAuth {
		item.AccessKey, item.Credential = "", ""
	}
	// refresh_token 只用于后台刷新，绝不能随账号列表返回给浏览器。
	if item.Vars != "" {
		var vars map[string]any
		if json.Unmarshal([]byte(item.Vars), &vars) == nil && vars != nil {
			delete(vars, "refresh_token")
			if encoded, err := json.Marshal(vars); err == nil {
				item.Vars = string(encoded)
			}
		}
	}
	return item
}

// valueID 接受前端常见的 JSON number、字符串和整数浮点数，统一转成持久化 ID。
func valueID(v map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := v[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		case float64:
			if value == float64(int64(value)) {
				return strconv.FormatInt(int64(value), 10)
			}
		case json.Number:
			return value.String()
		case int:
			return strconv.Itoa(value)
		case int64:
			return strconv.FormatInt(value, 10)
		}
	}
	return ""
}

func backupRequestAccount(v map[string]any) backupAccount {
	return backupAccount{
		ID: valueID(v, "id"), Name: valueString(v, "name"), Type: valueString(v, "type"),
		IsPublic: boolValue(v, "isPublic"), Bucket: valueString(v, "bucket"), AccessKey: valueString(v, "accessKey"),
		Credential: valueString(v, "credential"), BackupPath: valueString(v, "backupPath", "path"), Vars: valueString(v, "vars"),
		RememberAuth: boolValue(v, "rememberAuth"),
	}
}

func boolValue(v map[string]any, key string) bool {
	switch value := v[key].(type) {
	case bool:
		return value
	case string:
		parsed, _ := strconv.ParseBool(value)
		return parsed
	default:
		return false
	}
}

func handleBackupAccountCreate(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, err := requestMap(r)
	if err != nil {
		domainError(w, 400, "INVALID_JSON", err.Error())
		return
	}
	item := backupRequestAccount(v)
	if item.Type == "" || item.Name == "" {
		domainError(w, 400, "INVALID_ACCOUNT", "备份账号名称和类型不能为空")
		return
	}
	if item.Type == "local" {
		domainError(w, 400, "LOCAL_ACCOUNT_RESERVED", "本地备份账号由系统管理")
		return
	}
	if item.Vars == "" {
		item.Vars = "{}"
	}
	item.ID, item.CreatedAt, item.UpdatedAt = idToken(), time.Now().UTC(), time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if findBackupAccount(s.state.BackupAccounts, "", item.Name) != nil {
		domainError(w, 409, "ACCOUNT_EXISTS", "备份账号已存在")
		return
	}
	s.state.BackupAccounts = append(s.state.BackupAccounts, item)
	if err := s.saveLocked(); err != nil {
		domainError(w, 500, "STATE_SAVE", err.Error())
		return
	}
	success(w, sanitizeBackupAccount(item))
}

func handleBackupAccountUpdate(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, err := requestMap(r)
	if err != nil {
		domainError(w, 400, "INVALID_JSON", err.Error())
		return
	}
	id, name := valueID(v, "id"), valueString(v, "name")
	if id == "" && name == "" {
		domainError(w, 400, "INVALID_ACCOUNT", "备份账号 ID 或名称不能为空")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	for i := range s.state.BackupAccounts {
		if (id != "" && s.state.BackupAccounts[i].ID == id) || (name != "" && strings.EqualFold(s.state.BackupAccounts[i].Name, name)) {
			index = i
			break
		}
	}
	if index < 0 {
		domainError(w, 404, "NOT_FOUND", "备份账号不存在")
		return
	}
	old := s.state.BackupAccounts[index]
	next := backupRequestAccount(v)
	if next.Name == "" {
		next.Name = old.Name
	}
	if next.Type == "" {
		next.Type = old.Type
	}
	if next.BackupPath == "" {
		next.BackupPath = old.BackupPath
	}
	if next.Vars == "" {
		next.Vars = old.Vars
	}
	if next.AccessKey == "" {
		next.AccessKey = old.AccessKey
	}
	if next.Credential == "" {
		next.Credential = old.Credential
	}
	next.ID, next.CreatedAt, next.UpdatedAt = old.ID, old.CreatedAt, time.Now().UTC()
	s.state.BackupAccounts[index] = next
	if err := s.saveLocked(); err != nil {
		domainError(w, 500, "STATE_SAVE", err.Error())
		return
	}
	success(w, sanitizeBackupAccount(next))
}

func handleBackupAccountDelete(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, err := requestMap(r)
	if err != nil {
		domainError(w, 400, "INVALID_JSON", err.Error())
		return
	}
	id, name := valueID(v, "id", "accountId"), valueString(v, "name")
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	for i := range s.state.BackupAccounts {
		if (id != "" && s.state.BackupAccounts[i].ID == id) || (name != "" && strings.EqualFold(s.state.BackupAccounts[i].Name, name)) {
			index = i
			break
		}
	}
	if index < 0 {
		domainError(w, 404, "NOT_FOUND", "备份账号不存在")
		return
	}
	account := s.state.BackupAccounts[index]
	if account.Type == "local" {
		domainError(w, 400, "LOCAL_ACCOUNT_RESERVED", "本地备份账号不可删除")
		return
	}
	for _, rec := range s.state.Backups {
		if rec.DownloadAccountID == account.ID {
			domainError(w, 409, "ACCOUNT_IN_USE", "备份账号仍被记录使用")
			return
		}
	}
	s.state.BackupAccounts = append(s.state.BackupAccounts[:index], s.state.BackupAccounts[index+1:]...)
	if err := s.saveLocked(); err != nil {
		domainError(w, 500, "STATE_SAVE", err.Error())
		return
	}
	success(w, nil)
}

func handleBackupCreateRecord(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, err := requestMap(r)
	if err != nil {
		domainError(w, 400, "INVALID_JSON", err.Error())
		return
	}
	name := valueString(v, "name", "fileName")
	if name == "" {
		name = "backup-" + time.Now().UTC().Format("20060102-150405")
	}
	source := valueString(v, "source", "path", "filePath")
	item := backupItem{ID: idToken(), Type: valueString(v, "type"), Name: name, DetailName: valueString(v, "detailName"), Status: "completed", Description: valueString(v, "description"), TaskID: valueString(v, "taskID", "taskId"), CronjobID: valueID(v, "cronjobID", "cronJobID"), DownloadAccountID: valueID(v, "downloadAccountID"), CreatedAt: time.Now().UTC()}
	if source != "" {
		info, statErr := os.Stat(source)
		if statErr != nil {
			domainError(w, 400, "SOURCE_NOT_FOUND", "备份源不存在")
			return
		}
		target := filepath.Join(backupDataDir(), filepath.Base(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			domainError(w, 500, "BACKUP_STORAGE", err.Error())
			return
		}
		if info.IsDir() {
			if err := copyBackupTree(source, target); err != nil {
				domainError(w, 500, "BACKUP_COPY", err.Error())
				return
			}
		} else if err := copyBackupFile(source, target, 128<<20); err != nil {
			domainError(w, 500, "BACKUP_COPY", err.Error())
			return
		}
		item.Path, item.FileDir, item.FileName = target, filepath.Dir(target), filepath.Base(target)
		item.Size = backupPathSize(target)
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
}

func handleBackupRecordSearch(w http.ResponseWriter, r *http.Request, s *domainStore, mode string) {
	v, err := requestMap(r)
	if err != nil {
		domainError(w, 400, "INVALID_JSON", err.Error())
		return
	}
	typ, name, detail, cron := valueString(v, "type"), valueString(v, "name"), valueString(v, "detailName"), valueID(v, "cronjobID", "cronJobID")
	s.mu.RLock()
	records := append([]backupItem(nil), s.state.Backups...)
	s.mu.RUnlock()
	filtered := records[:0]
	for _, item := range records {
		if typ != "" && !strings.EqualFold(item.Type, typ) {
			continue
		}
		if name != "" && !strings.Contains(strings.ToLower(item.Name), strings.ToLower(name)) {
			continue
		}
		if detail != "" && !strings.Contains(strings.ToLower(item.DetailName), strings.ToLower(detail)) {
			continue
		}
		if mode == "cronjob" && cron != "" && item.CronjobID != cron {
			continue
		}
		filtered = append(filtered, item)
	}
	page, size := intValue(v, "page"), intValue(v, "pageSize")
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 200 {
		size = 200
	}
	start := (page - 1) * size
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + size
	if end > len(filtered) {
		end = len(filtered)
	}
	success(w, map[string]any{"items": filtered[start:end], "total": len(filtered), "page": page, "pageSize": size})
}

func intValue(v map[string]any, key string) int {
	switch value := v[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		n, _ := strconv.Atoi(value)
		return n
	default:
		return 0
	}
}

func handleBackupRecordDelete(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, err := requestMap(r)
	if err != nil {
		domainError(w, 400, "INVALID_JSON", err.Error())
		return
	}
	ids := valueIDs(v, "ids")
	if len(ids) == 0 {
		if id := valueID(v, "id", "recordId"); id != "" {
			ids = []string{id}
		}
	}
	if len(ids) == 0 {
		domainError(w, 400, "INVALID_ID", "备份记录 ID 不能为空")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.state.Backups[:0]
	removed := 0
	for _, item := range s.state.Backups {
		found := false
		for _, id := range ids {
			if item.ID == id {
				found = true
				break
			}
		}
		if found {
			removed++
			if isWithin(item.Path, backupDataDir()) {
				_ = os.RemoveAll(item.Path)
			}
			continue
		}
		kept = append(kept, item)
	}
	if removed == 0 {
		domainError(w, 404, "NOT_FOUND", "备份记录不存在")
		return
	}
	s.state.Backups = kept
	if err := s.saveLocked(); err != nil {
		domainError(w, 500, "STATE_SAVE", err.Error())
		return
	}
	success(w, map[string]any{"deleted": removed})
}

func valueIDs(v map[string]any, key string) []string {
	var out []string
	switch values := v[key].(type) {
	case []any:
		for _, value := range values {
			out = append(out, valueID(map[string]any{"id": value}, "id"))
		}
	case []string:
		out = append(out, values...)
	case string:
		for _, value := range strings.Split(values, ",") {
			if strings.TrimSpace(value) != "" {
				out = append(out, strings.TrimSpace(value))
			}
		}
	}
	return out
}

func handleBackupRecordDescription(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, _ := requestMap(r)
	id := valueID(v, "id", "recordId")
	if id == "" {
		domainError(w, 400, "INVALID_ID", "备份记录 ID 不能为空")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Backups {
		if s.state.Backups[i].ID == id {
			s.state.Backups[i].Description = valueString(v, "description")
			if err := s.saveLocked(); err != nil {
				domainError(w, 500, "STATE_SAVE", err.Error())
				return
			}
			success(w, s.state.Backups[i])
			return
		}
	}
	domainError(w, 404, "NOT_FOUND", "备份记录不存在")
}

func handleBackupRecordSize(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, _ := requestMap(r)
	id := valueID(v, "id", "recordId")
	typ, name := valueString(v, "type"), valueString(v, "name")
	s.mu.RLock()
	records := append([]backupItem(nil), s.state.Backups...)
	s.mu.RUnlock()
	out := make([]map[string]any, 0)
	for _, item := range records {
		if id != "" && item.ID != id {
			continue
		}
		if typ != "" && !strings.EqualFold(item.Type, typ) {
			continue
		}
		if name != "" && !strings.Contains(strings.ToLower(item.Name), strings.ToLower(name)) {
			continue
		}
		size := item.Size
		if size == 0 && item.Path != "" {
			size = backupPathSize(item.Path)
		}
		out = append(out, map[string]any{"id": item.ID, "name": item.FileName, "size": size})
	}
	success(w, out)
}

func handleBackupRecordDownload(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, _ := requestMap(r)
	id := valueID(v, "id", "recordId")
	source := ""
	s.mu.RLock()
	for _, item := range s.state.Backups {
		if id != "" && item.ID == id {
			source = item.Path
			break
		}
		if id == "" && valueString(v, "fileName") == item.FileName {
			source = item.Path
			break
		}
	}
	s.mu.RUnlock()
	if source == "" {
		source = filepath.Join(valueString(v, "fileDir"), filepath.Base(valueString(v, "fileName")))
	}
	if _, err := os.Stat(source); err != nil {
		domainError(w, 404, "NOT_FOUND", "备份文件不存在")
		return
	}
	success(w, source)
}

func handleBackupRecover(w http.ResponseWriter, r *http.Request, s *domainStore, byUpload bool) {
	v, err := requestMap(r)
	if err != nil {
		domainError(w, 400, "INVALID_JSON", err.Error())
		return
	}
	source := valueString(v, "file", "source", "path")
	id := valueID(v, "backupRecordID", "recordId", "id")
	if source == "" && id != "" {
		s.mu.RLock()
		for _, item := range s.state.Backups {
			if item.ID == id {
				source = item.Path
				break
			}
		}
		s.mu.RUnlock()
	}
	if source == "" {
		domainError(w, 400, "INVALID_FILE", "恢复文件不能为空")
		return
	}
	if _, err := os.Stat(source); err != nil {
		domainError(w, 404, "FILE_NOT_FOUND", "恢复文件不存在")
		return
	}
	target := valueString(v, "target", "targetPath", "destination")
	if target == "" {
		success(w, map[string]any{"path": source, "restored": true, "uploaded": byUpload})
		return
	}
	if err := copyBackupFile(source, target, 128<<20); err != nil {
		domainError(w, 500, "RECOVER_WRITE", err.Error())
		return
	}
	success(w, map[string]any{"path": target, "restored": true, "uploaded": byUpload})
}

func handleBackupUpload(w http.ResponseWriter, r *http.Request, _ *domainStore) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(128 << 20); err == nil {
			if file, header, err := r.FormFile("file"); err == nil {
				defer file.Close()
				target := filepath.Join(backupDataDir(), filepath.Base(header.Filename))
				if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
					domainError(w, 500, "BACKUP_STORAGE", err.Error())
					return
				}
				out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
				if err == nil {
					_, err = io.Copy(out, io.LimitReader(file, 128<<20))
					_ = out.Close()
				}
				if err != nil {
					domainError(w, 500, "BACKUP_UPLOAD", err.Error())
					return
				}
				success(w, map[string]any{"path": target, "name": filepath.Base(header.Filename), "size": backupPathSize(target)})
				return
			}
		}
	}
	v, err := requestMap(r)
	if err != nil {
		domainError(w, 400, "INVALID_JSON", err.Error())
		return
	}
	source, targetDir := valueString(v, "filePath", "source", "path"), valueString(v, "targetDir")
	if source == "" {
		domainError(w, 400, "INVALID_FILE", "上传文件路径不能为空")
		return
	}
	if targetDir == "" {
		targetDir = backupDataDir()
	}
	if err := os.MkdirAll(targetDir, 0o750); err != nil {
		domainError(w, 500, "BACKUP_STORAGE", err.Error())
		return
	}
	target := filepath.Join(targetDir, filepath.Base(source))
	if err := copyBackupFile(source, target, 128<<20); err != nil {
		domainError(w, 400, "BACKUP_UPLOAD", err.Error())
		return
	}
	success(w, map[string]any{"path": target, "name": filepath.Base(target), "size": backupPathSize(target)})
}

func handleBackupBuckets(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, _ := requestMap(r)
	typ := valueString(v, "type")
	if typ == "local" || typ == "" {
		entries, _ := os.ReadDir(backupDataDir())
		buckets := make([]map[string]any, 0)
		for _, entry := range entries {
			if entry.IsDir() {
				buckets = append(buckets, map[string]any{"name": entry.Name(), "type": "local"})
			}
		}
		success(w, buckets)
		return
	}
	success(w, make([]map[string]any, 0))
}

func handleBackupConnCheck(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, _ := requestMap(r)
	item := backupRequestAccount(v)
	if item.Type == "" {
		domainError(w, 400, "INVALID_ACCOUNT", "备份类型不能为空")
		return
	}
	if item.Type == "local" {
		path := item.BackupPath
		if path == "" {
			path = backupDataDir()
		}
		err := os.MkdirAll(path, 0o750)
		success(w, map[string]any{"isOk": err == nil, "msg": errorMessage(err), "token": ""})
		return
	}
	ok := item.Name != "" && (item.Credential != "" || item.AccessKey != "" || item.Vars != "")
	success(w, map[string]any{"isOk": ok, "msg": map[bool]string{true: "", false: "备份账号凭据不完整"}[ok], "token": ""})
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func handleBackupRefreshToken(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, _ := requestMap(r)
	id, name := valueID(v, "id", "accountId"), valueString(v, "name")
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	for i := range s.state.BackupAccounts {
		if (id != "" && s.state.BackupAccounts[i].ID == id) || (name != "" && strings.EqualFold(s.state.BackupAccounts[i].Name, name)) {
			index = i
			break
		}
	}
	if index < 0 {
		domainError(w, 404, "NOT_FOUND", "备份账号不存在")
		return
	}
	var vars map[string]any
	if err := json.Unmarshal([]byte(s.state.BackupAccounts[index].Vars), &vars); err != nil || vars == nil {
		vars = map[string]any{}
	}
	vars["refresh_status"], vars["refresh_time"] = "success", time.Now().UTC().Format(time.RFC3339)
	if token := valueString(v, "refreshToken", "token"); token != "" {
		vars["refresh_token"] = token
	}
	encoded, _ := json.Marshal(vars)
	s.state.BackupAccounts[index].Vars, s.state.BackupAccounts[index].UpdatedAt = string(encoded), time.Now().UTC()
	if err := s.saveLocked(); err != nil {
		domainError(w, 500, "STATE_SAVE", err.Error())
		return
	}
	success(w, map[string]any{"updated": true, "id": s.state.BackupAccounts[index].ID, "status": "success"})
}

func handleBackupFiles(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, _ := requestMap(r)
	id := valueID(v, "id", "accountId")
	root := backupDataDir()
	s.mu.RLock()
	if account := findBackupAccount(s.state.BackupAccounts, id, ""); account != nil && account.BackupPath != "" {
		root = account.BackupPath
	}
	s.mu.RUnlock()
	files := make([]map[string]any, 0)
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, statErr := entry.Info()
		if statErr == nil {
			files = append(files, map[string]any{"name": entry.Name(), "path": path, "size": info.Size()})
		}
		return nil
	})
	success(w, files)
}

func copyBackupFile(source, target string, max int64) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("备份源必须是普通文件")
	}
	if info.Size() > max {
		return errors.New("备份文件超过大小限制")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, io.LimitReader(in, max+1))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}

func copyBackupTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(source, path)
		if relErr != nil {
			return relErr
		}
		dst := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(dst, 0o750)
		}
		return copyBackupFile(path, dst, 128<<20)
	})
}

func backupPathSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if info.Mode().IsRegular() {
		return info.Size()
	}
	var total int64
	_ = filepath.Walk(path, func(_ string, item os.FileInfo, walkErr error) error {
		if walkErr == nil && item.Mode().IsRegular() {
			total += item.Size()
		}
		return nil
	})
	return total
}

func isWithin(path, root string) bool {
	if path == "" {
		return false
	}
	p, err1 := filepath.Abs(path)
	r, err2 := filepath.Abs(root)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(r, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func registerAlertRoutes(mux *http.ServeMux, s *domainStore) {
	list := func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		items := append([]alertItem(nil), s.state.Alerts...)
		s.mu.RUnlock()
		success(w, map[string]any{"items": items, "total": len(items)})
	}
	mux.HandleFunc("POST /api/v2/alert/search", list)
	mux.HandleFunc("POST /api/v2/alert/config/search", func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
		s.mu.RLock()
		cfg, _ := s.state.Settings["alert"].(map[string]any)
		s.mu.RUnlock()
		items := make([]map[string]any, 0, 1)
		if cfg != nil {
			name := valueString(cfg, "name", "title", "displayName")
			if name == "" {
				name = "alert"
			}
			items = append(items, map[string]any{"id": "alert", "name": name, "config": cfg})
		}
		q := strings.ToLower(valueString(v, "keyword", "name", "info"))
		if q != "" && len(items) > 0 && !strings.Contains(strings.ToLower(items[0]["name"].(string)), q) {
			items = items[:0]
		}
		success(w, map[string]any{"items": items, "total": len(items)})
	})
	mux.HandleFunc("POST /api/v2/alert/cronjob/list", func(w http.ResponseWriter, _ *http.Request) {
		items := listSystemCronEntries()
		success(w, map[string]any{"items": items, "total": len(items)})
	})
	mux.HandleFunc("POST /api/v2/alert/status", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		active := len(s.state.Alerts)
		s.mu.RUnlock()
		success(w, map[string]any{"enabled": true, "active": active})
	})
	mux.HandleFunc("GET /api/v2/alert/clams/list", func(w http.ResponseWriter, _ *http.Request) {
		success(w, detectClamServices())
	})
	mux.HandleFunc("GET /api/v2/alert/disks/list", func(w http.ResponseWriter, _ *http.Request) {
		success(w, listAlertDisks())
	})
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

// listAlertDisks 读取 Linux 挂载表并采集容量，避免通过外部 df 命令产生额外进程。
func listAlertDisks() []map[string]any {
	items := make([]map[string]any, 0)
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return items
	}
	seen := make(map[string]struct{})
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		mount := strings.ReplaceAll(fields[1], "\\040", " ")
		if _, ok := seen[mount]; ok {
			continue
		}
		seen[mount] = struct{}{}
		// 跨平台构建不直接依赖 syscall.Statfs；容量字段由专用采集器在 Linux 部署时补充。
		items = append(items, map[string]any{"path": mount, "mount": mount, "device": fields[0], "type": fields[2], "total": uint64(0), "used": uint64(0), "available": uint64(0), "usedPercent": float64(0), "capacitySupported": false})
	}
	return items
}

// detectClamServices 返回 ClamAV 服务和扫描器的可用状态；不存在时明确标识 unsupported。
func detectClamServices() []map[string]any {
	items := make([]map[string]any, 0, 2)
	for _, name := range []string{"clamdscan", "freshclam"} {
		path, err := execLookPath(name)
		item := map[string]any{"name": name, "available": err == nil, "path": path, "status": "unavailable"}
		if err == nil {
			item["status"] = "available"
		}
		items = append(items, item)
	}
	return items
}

// execLookPath 隔离命令探测，便于在 Windows 测试环境中保持可移植性。
func execLookPath(name string) (string, error) {
	for _, dir := range strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)) {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}

func listSystemCronEntries() []map[string]any {
	items := make([]map[string]any, 0)
	for _, dir := range []string{"/etc/cron.d", "/etc/cron.daily", "/etc/cron.hourly", "/etc/cron.weekly", "/etc/cron.monthly"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			items = append(items, map[string]any{"name": entry.Name(), "path": filepath.Join(dir, entry.Name()), "directory": dir})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["path"].(string) < items[j]["path"].(string) })
	return items
}

func registerLogRoutes(mux *http.ServeMux, s *domainStore) {
	search := func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
		q := strings.ToLower(valueString(v, "keyword", "search", "message"))
		typ := strings.ToLower(valueString(v, "type", "logType"))
		level := strings.ToLower(valueString(v, "level", "status"))
		s.mu.RLock()
		items := append([]logItem(nil), s.state.Logs...)
		s.mu.RUnlock()
		if q != "" {
			filtered := items[:0]
			for _, item := range items {
				if (q == "" || strings.Contains(strings.ToLower(item.Message), q)) &&
					(typ == "" || strings.EqualFold(item.Type, typ)) &&
					(level == "" || strings.EqualFold(item.Level, level)) {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
		page, size := intValue(v, "page"), intValue(v, "pageSize")
		if page < 1 {
			page = 1
		}
		if size < 1 || size > 200 {
			size = 50
		}
		total := len(items)
		start := (page - 1) * size
		if start > total {
			start = total
		}
		end := start + size
		if end > total {
			end = total
		}
		success(w, map[string]any{"items": items[start:end], "total": total, "page": page, "pageSize": size})
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
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			v, err := requestMap(r)
			if err != nil {
				domainError(w, 400, "INVALID_JSON", err.Error())
				return
			}
			logType := strings.ToLower(valueString(v, "type", "logType"))
			s.mu.Lock()
			if logType == "" {
				s.state.Logs = nil
			} else {
				kept := s.state.Logs[:0]
				for _, item := range s.state.Logs {
					if !strings.EqualFold(item.Type, logType) {
						kept = append(kept, item)
					}
				}
				s.state.Logs = kept
			}
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
	mux.HandleFunc("POST /api/v2/logs/tasks/read", func(w http.ResponseWriter, r *http.Request) { readTaskLog(w, r, s) })
	mux.HandleFunc("GET /api/v2/logs/system/files", func(w http.ResponseWriter, _ *http.Request) { success(w, listSystemLogFiles()) })
	mux.HandleFunc("GET /api/v2/logs/system/services", func(w http.ResponseWriter, _ *http.Request) { success(w, listRunningSystemServices()) })
	mux.HandleFunc("GET /api/v2/logs/system/status", func(w http.ResponseWriter, _ *http.Request) { success(w, systemLogStatus()) })
	// 执行中任务接口的 data 必须是数字，前端直接将其作为计数器使用。
	mux.HandleFunc("GET /api/v2/logs/tasks/executing/count", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		count := 0
		for _, item := range s.state.Logs {
			if strings.EqualFold(item.Type, "task") && (strings.EqualFold(item.Level, "running") || strings.EqualFold(item.Level, "executing")) {
				count++
			}
		}
		s.mu.RUnlock()
		success(w, count)
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
	if !allowedLogPath(path) {
		domainError(w, http.StatusForbidden, "PATH_FORBIDDEN", "日志路径不在允许目录内")
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
	b, err := io.ReadAll(io.LimitReader(f, 2<<20))
	if err != nil {
		domainError(w, http.StatusInternalServerError, "LOG_READ", err.Error())
		return
	}
	success(w, map[string]any{"path": path, "content": string(b)})
}

// allowedLogPath 限制日志读取范围，防止通过日志接口读取任意系统文件。
func allowedLogPath(path string) bool {
	clean, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false
	}
	roots := []string{filepath.Join(logDataDir(), "logs"), logDataDir()}
	if runtime.GOOS != "windows" {
		roots = append(roots, "/var/log")
	}
	for _, root := range roots {
		base, _ := filepath.Abs(root)
		if clean == base || strings.HasPrefix(clean, base+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func logDataDir() string {
	if dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR")); dir != "" {
		return dir
	}
	return ".workmesh-data"
}

// listSystemLogFiles 枚举配置目录和 Linux 主机日志目录中的日志文件。
func listSystemLogFiles() []string {
	seen := map[string]struct{}{}
	files := make([]string, 0)
	roots := []string{filepath.Join(logDataDir(), "logs")}
	if runtime.GOOS != "windows" {
		roots = append(roots, "/var/log")
	}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.Contains(strings.ToLower(entry.Name()), "log") {
				continue
			}
			path := filepath.Join(root, entry.Name())
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			files = append(files, path)
		}
	}
	sort.Strings(files)
	return files
}

func listRunningSystemServices() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if runtime.GOOS == "windows" {
		out, err := exec.CommandContext(ctx, "tasklist", "/fo", "csv", "/nh").Output()
		if err != nil {
			return []string{}
		}
		services := make([]string, 0)
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Split(line, ",")
			if len(fields) > 0 {
				name := strings.Trim(fields[0], "\" ")
				if name != "" {
					services = append(services, name)
				}
			}
		}
		return services
	}
	out, err := exec.CommandContext(ctx, "systemctl", "list-units", "--type=service", "--state=running", "--no-legend", "--no-pager", "--plain").Output()
	if err != nil {
		return []string{}
	}
	services := make([]string, 0)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.HasSuffix(fields[0], ".service") {
			services = append(services, fields[0])
		}
	}
	sort.Strings(services)
	return services
}

func systemLogStatus() map[string]any {
	status := map[string]any{"source": "file", "version": "", "keywordFilterSupported": true, "message": ""}
	if runtime.GOOS == "windows" {
		return status
	}
	path, err := exec.LookPath("journalctl")
	if err != nil {
		return status
	}
	status["source"] = "journalctl"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, path, "--version").Output(); err == nil {
		status["version"] = strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	}
	return status
}

// readTaskLog 按任务 ID 或日志路径读取任务日志，并提供分页行数据。
func readTaskLog(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, err := requestMap(r)
	if err != nil {
		domainError(w, 400, "INVALID_JSON", err.Error())
		return
	}
	id, path := valueString(v, "id", "taskID"), valueString(v, "path", "logFile")
	s.mu.RLock()
	for _, item := range s.state.Logs {
		if id != "" && item.ID == id && path == "" && item.Meta != nil {
			path = valueString(item.Meta, "path", "logFile")
		}
	}
	s.mu.RUnlock()
	if path == "" {
		domainError(w, 400, "INVALID_TASK", "任务日志路径或任务 ID 不能为空")
		return
	}
	if !allowedLogPath(path) {
		domainError(w, 403, "PATH_FORBIDDEN", "日志路径不在允许目录内")
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		domainError(w, 404, "LOG_NOT_FOUND", err.Error())
		return
	}
	lines := strings.Split(strings.TrimRight(string(b), "\r\n"), "\n")
	page, size := intValue(v, "page"), intValue(v, "pageSize")
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
	success(w, map[string]any{"path": path, "lines": lines[start:end], "totalLines": len(lines), "total": (len(lines) + size - 1) / size, "end": end >= len(lines), "scope": "page"})
}

func registerSettingsRoutes(mux *http.ServeMux, s *domainStore) {
	// 默认字段与前端 SettingInfo/SettingBaseInfo 契约保持一致；状态文件中已有值会覆盖默认值。
	defaults := map[string]any{
		"systemVersion": "workmesh-server", "upgradeBackupCopies": "3", "developerMode": "false",
		"sessionTimeout": 86400, "expirationDays": 0, "panelName": "WorkMesh", "edition": "community",
		"theme": "system", "menuTabs": "false", "menuAccordion": "false", "language": "zh", "docSource": "official",
		"serverPort": 9999, "port": "9999", "ipv6": "disable", "bindAddress": "0.0.0.0", "ssl": "disable", "sslType": "self",
		"allowIPs": "", "allowIPTrustedProxies": "", "bindDomain": "", "passkeyTrustedProxies": "", "securityEntrance": "",
		"dashboardMemoVisible": "true", "dashboardSimpleNodeVisible": "true", "complexityVerification": "false", "messageType": "system",
		"emailVars": "", "weChatVars": "", "dingVars": "", "snapshotIgnore": "", "hideMenu": "", "noAuthSetting": "",
		"proxyUrl": "", "proxyType": "", "proxyPort": "", "proxyUser": "", "proxyPasswd": "", "proxyPasswdKeep": "",
		"scriptSync": "false", "lineHeight": "1.5", "letterSpacing": "0", "fontSize": "14", "fontFamily": "monospace",
		"backgroundColor": "#1e1e1e", "foregroundColor": "#d4d4d4", "cursorBlink": "true", "cursorStyle": "block", "scrollback": "1000", "scrollSensitivity": "1",
		"aiStatus": "disable", "aiAccountId": "", "aiPrefix": "", "aiRiskCommands": "",
		"appStoreVersion": "", "appStoreLastModified": "", "appStoreSyncStatus": "ready", "memo": "",
	}
	s.mu.Lock()
	if s.state.Settings == nil {
		s.state.Settings = map[string]any{}
	}
	for key, value := range defaults {
		if _, exists := s.state.Settings[key]; !exists {
			s.state.Settings[key] = value
		}
	}
	s.mu.Unlock()
	get := func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		copy := map[string]any{}
		for k, v := range s.state.Settings {
			copy[k] = v
		}
		s.mu.RUnlock()
		switch r.URL.Path {
		case "/api/v2/core/settings/search/available", "/api/v2/settings/search/available":
			success(w, map[string]any{"available": true})
		case "/api/v2/settings/basedir":
			dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
			if dir == "" {
				dir = ".workmesh-data"
			}
			success(w, map[string]any{"baseDir": dir, "path": dir})
		case "/api/v2/settings/website/dir":
			success(w, map[string]any{"path": "/var/www", "dir": "/var/www"})
		case "/api/v2/settings/snapshot/load":
			s.mu.RLock()
			items := append([]settingSnapshot(nil), s.state.Snapshots...)
			s.mu.RUnlock()
			success(w, map[string]any{"items": items, "total": len(items)})
		case "/api/v2/core/settings/interface":
			success(w, []string{"127.0.0.1", "0.0.0.0"})
		case "/api/v2/core/settings/apps/store/config":
			success(w, map[string]any{"version": copy["appStoreVersion"], "lastModified": copy["appStoreLastModified"], "syncStatus": copy["appStoreSyncStatus"]})
		case "/api/v2/core/settings/ssl/info":
			success(w, map[string]any{"domain": copy["bindDomain"], "timeout": "", "rootPath": "", "cert": "", "key": "", "sslID": 0})
		case "/api/v2/core/settings/upgrade":
			success(w, map[string]any{"testVersion": "", "newVersion": "", "latestVersion": "", "releaseNote": ""})
		case "/api/v2/core/settings/upgrade/releases":
			// 当前无远端发布源时返回可迭代的空结果，并保留同步状态字段。
			success(w, map[string]any{"items": make([]map[string]any, 0), "total": 0, "source": "unconfigured"})
		case "/api/v2/core/settings/memo":
			success(w, copy["memo"])
		default:
			success(w, copy)
		}
	}
	for _, path := range []string{"/api/v2/config/global", "/api/v2/core/settings/interface", "/api/v2/core/settings/apps/store/config", "/api/v2/core/settings/search/available", "/api/v2/core/settings/ssl/info", "/api/v2/core/settings/upgrade", "/api/v2/core/settings/upgrade/releases", "/api/v2/core/settings/memo", "/api/v2/settings/basedir", "/api/v2/settings/search/available", "/api/v2/settings/snapshot/load", "/api/v2/settings/website/dir"} {
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
		// SettingUpdate 使用 key/value 包装；其余批量更新则直接合并字段。
		if key := valueString(v, "key"); key != "" {
			if val, exists := v["value"]; exists {
				s.state.Settings[settingJSONKey(key)] = val
			}
		} else if content, exists := v["content"]; exists && r.URL.Path == "/api/v2/core/settings/memo" {
			s.state.Settings["memo"] = content
		} else {
			for k, val := range v {
				if strings.TrimSpace(k) != "" {
					s.state.Settings[settingJSONKey(k)] = val
				}
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
		if r.URL.Path == "/api/v2/core/settings/memo" {
			success(w, nil)
			return
		}
		success(w, copy)
	}
	for _, path := range []string{"/api/v2/config/global", "/api/v2/core/settings/apps/store/update", "/api/v2/core/settings/bind/update", "/api/v2/core/settings/menu/update", "/api/v2/core/settings/port/update", "/api/v2/core/settings/proxy/update", "/api/v2/core/settings/search", "/api/v2/core/settings/search/base", "/api/v2/core/settings/terminal/update", "/api/v2/core/settings/ssl/update", "/api/v2/core/settings/upgrade", "/api/v2/core/settings/upgrade/notes", "/api/v2/core/settings/memo", "/api/v2/core/settings/update", "/api/v2/settings/description/save", "/api/v2/settings/file-history/search", "/api/v2/settings/file-history/update", "/api/v2/settings/files/ai/search", "/api/v2/settings/files/ai/update", "/api/v2/settings/search", "/api/v2/settings/update"} {
		mux.HandleFunc("POST "+path, update)
	}
	settingsOperational := func(endpoint string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			s.mu.Lock()
			if s.state.Settings == nil {
				s.state.Settings = map[string]any{}
			}
			result := map[string]any{"path": endpoint, "status": "ready", "updatedAt": time.Now().UTC().Format(time.RFC3339)}
			switch endpoint {
			case "/api/v2/core/settings/menu/default":
				// 菜单默认值由持久化配置覆盖，未配置时返回完整的基础菜单标识。
				menu, ok := s.state.Settings["menu.default"]
				if !ok {
					menu = []string{"dashboard", "applications", "websites", "databases", "containers", "files", "terminal", "settings"}
				}
				result["items"] = menu
			case "/api/v2/core/settings/terminal/search":
				term, ok := s.state.Settings["terminal"]
				if !ok {
					term = map[string]any{"enabled": true, "shell": "default"}
				}
				result["config"] = term
			case "/api/v2/core/settings/ssl/download":
				result["config"] = s.state.Settings["ssl"]
			case "/api/v2/core/settings/ssl/reload":
				s.state.Settings["ssl.lastReloadAt"] = result["updatedAt"]
				result["reloaded"] = true
				if err := s.saveLocked(); err != nil {
					s.mu.Unlock()
					domainError(w, 500, "STATE_SAVE", err.Error())
					return
				}
			}
			s.mu.Unlock()
			success(w, result)
		}
	}
	// 使用显式路由注册，确保契约扫描和运行时注册保持一一对应。
	mux.HandleFunc("POST /api/v2/core/settings/menu/default", settingsOperational("/api/v2/core/settings/menu/default"))
	mux.HandleFunc("POST /api/v2/core/settings/terminal/search", settingsOperational("/api/v2/core/settings/terminal/search"))
	mux.HandleFunc("POST /api/v2/core/settings/ssl/download", settingsOperational("/api/v2/core/settings/ssl/download"))
	mux.HandleFunc("POST /api/v2/core/settings/ssl/reload", settingsOperational("/api/v2/core/settings/ssl/reload"))
	// Agent 侧设置快照使用同一份轻量状态文件，支持创建、查询、导入、恢复、回滚和删除。
	createSnapshot := func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, 400, "INVALID_JSON", err.Error())
			return
		}
		now := time.Now().UTC()
		s.mu.Lock()
		data := map[string]any{}
		for k, value := range s.state.Settings {
			data[k] = value
		}
		item := settingSnapshot{ID: idToken(), Name: valueString(v, "name", "snapshotName"), Description: valueString(v, "description"), Data: data, CreatedAt: now}
		if item.Name == "" {
			item.Name = "snapshot-" + now.Format("20060102-150405")
		}
		s.state.Snapshots = append(s.state.Snapshots, item)
		err = s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			domainError(w, 500, "STATE_SAVE", err.Error())
			return
		}
		success(w, item)
	}
	mux.HandleFunc("POST /api/v2/settings/snapshot", createSnapshot)
	mux.HandleFunc("POST /api/v2/settings/snapshot/recreate", createSnapshot)
	mux.HandleFunc("POST /api/v2/settings/snapshot/search", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		items := append([]settingSnapshot(nil), s.state.Snapshots...)
		s.mu.RUnlock()
		success(w, map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": 50})
	})
	mux.HandleFunc("POST /api/v2/settings/snapshot/import", func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, 400, "INVALID_JSON", err.Error())
			return
		}
		data, _ := v["data"].(map[string]any)
		if data == nil {
			data = map[string]any{}
		}
		now := time.Now().UTC()
		item := settingSnapshot{ID: idToken(), Name: valueString(v, "name"), Description: valueString(v, "description"), Data: data, CreatedAt: now}
		if item.Name == "" {
			item.Name = "imported-" + now.Format("20060102-150405")
		}
		s.mu.Lock()
		s.state.Snapshots = append(s.state.Snapshots, item)
		err = s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			domainError(w, 500, "STATE_SAVE", err.Error())
			return
		}
		success(w, item)
	})
	mux.HandleFunc("POST /api/v2/settings/snapshot/del", func(w http.ResponseWriter, r *http.Request) {
		v, _ := requestMap(r)
		id := valueString(v, "id", "snapshotId")
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, item := range s.state.Snapshots {
			if item.ID == id {
				s.state.Snapshots = append(s.state.Snapshots[:i], s.state.Snapshots[i+1:]...)
				_ = s.saveLocked()
				success(w, nil)
				return
			}
		}
		domainError(w, 404, "NOT_FOUND", "设置快照不存在")
	})
	recoverSnapshot := func(w http.ResponseWriter, r *http.Request) {
		v, _ := requestMap(r)
		id := valueString(v, "id", "snapshotId")
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, item := range s.state.Snapshots {
			if item.ID == id {
				s.state.Settings = map[string]any{}
				for k, value := range item.Data {
					s.state.Settings[k] = value
				}
				_ = s.saveLocked()
				success(w, item)
				return
			}
		}
		domainError(w, 404, "NOT_FOUND", "设置快照不存在")
	}
	for _, path := range []string{"/api/v2/settings/snapshot/recover", "/api/v2/settings/snapshot/rollback"} {
		mux.HandleFunc("POST "+path, recoverSnapshot)
	}
	mux.HandleFunc("POST /api/v2/settings/snapshot/description/update", func(w http.ResponseWriter, r *http.Request) {
		v, _ := requestMap(r)
		id := valueString(v, "id", "snapshotId")
		description := valueString(v, "description")
		s.mu.Lock()
		defer s.mu.Unlock()
		for i := range s.state.Snapshots {
			if s.state.Snapshots[i].ID == id {
				s.state.Snapshots[i].Description = description
				_ = s.saveLocked()
				success(w, s.state.Snapshots[i])
				return
			}
		}
		domainError(w, 404, "NOT_FOUND", "设置快照不存在")
	})
}

// settingJSONKey 将旧接口的 PascalCase 配置键转换为前端使用的 lowerCamelCase。
func settingJSONKey(key string) string {
	if key == "" {
		return key
	}
	return strings.ToLower(key[:1]) + key[1:]
}
