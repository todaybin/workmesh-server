// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

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
	mux.HandleFunc("POST /api/v2/backups/buckets", func(w http.ResponseWriter, r *http.Request) { handleBackupBucketsV2(w, r, s) })
	mux.HandleFunc("POST /api/v2/backups/conn/check", func(w http.ResponseWriter, r *http.Request) { handleBackupConnCheckV2(w, r, s) })
	mux.HandleFunc("POST /api/v2/backups/refresh/token", func(w http.ResponseWriter, r *http.Request) { handleBackupRefreshTokenV2(w, r, s) })
	mux.HandleFunc("POST /api/v2/core/backups/refresh/token", func(w http.ResponseWriter, r *http.Request) { handleBackupRefreshTokenV2(w, r, s) })
}

func backupDataDir() string {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
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
	previousAccounts := cloneBackupAccounts(s.state.BackupAccounts)
	s.state.BackupAccounts = append(s.state.BackupAccounts, item)
	if err := s.saveLocked(); err != nil {
		s.state.BackupAccounts = previousAccounts
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
	previousAccounts := cloneBackupAccounts(s.state.BackupAccounts)
	s.state.BackupAccounts[index] = next
	if err := s.saveLocked(); err != nil {
		s.state.BackupAccounts = previousAccounts
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
	previousAccounts := cloneBackupAccounts(s.state.BackupAccounts)
	s.state.BackupAccounts = append(s.state.BackupAccounts[:index], s.state.BackupAccounts[index+1:]...)
	if err := s.saveLocked(); err != nil {
		s.state.BackupAccounts = previousAccounts
		domainError(w, 500, "STATE_SAVE", err.Error())
		return
	}
	success(w, nil)
}

func cloneBackupAccounts(accounts []backupAccount) []backupAccount {
	if accounts == nil {
		return nil
	}
	return append([]backupAccount{}, accounts...)
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
	item := backupItem{ID: idToken(), Type: valueString(v, "type"), Name: name, DetailName: valueString(v, "detailName"), Status: "completed", Description: valueString(v, "description"), TaskID: valueString(v, "taskID", "taskId"), CronjobID: valueID(v, "cronjobID", "cronJobID"), DownloadAccountID: valueID(v, "downloadAccountID", "downloadAccountId"), SourceAccountIDs: strings.Join(valueIDs(v, "sourceAccountIDs", "sourceAccountIds", "accountIDs", "accountIds"), ","), CreatedAt: time.Now().UTC()}
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
		item.Path, item.FileDir = target, filepath.Dir(target)
		// 远端备份对象沿用源文件名，避免用户显示名称（例如“snapshot”）丢失扩展名。
		item.FileName = filepath.Base(source)
		item.Size = backupPathSize(target)
	}
	if source != "" {
		if err := uploadBackupRemote(r.Context(), s, &item, v); err != nil {
			item.Status = "failed"
			item.Message = err.Error()
			s.mu.Lock()
			s.state.Backups = append(s.state.Backups, item)
			_ = s.saveLocked()
			s.mu.Unlock()
			domainError(w, http.StatusBadGateway, "BACKUP_PROVIDER_UPLOAD", err.Error())
			return
		}
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

// uploadBackupRemote 将本地快照上传到账号声明的云端端点；未声明写端点时保持本地备份模式。
func uploadBackupRemote(ctx context.Context, s *domainStore, item *backupItem, values map[string]any) error {
	if item == nil || item.Path == "" {
		return nil
	}
	ids := valueIDs(values, "sourceAccountIDs", "sourceAccountIds", "accountIDs", "accountIds")
	if len(ids) == 0 {
		if id := valueID(values, "downloadAccountID", "downloadAccountId"); id != "" {
			ids = []string{id}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	s.mu.RLock()
	accounts := append([]backupAccount(nil), s.state.BackupAccounts...)
	s.mu.RUnlock()
	for _, id := range ids {
		account := findBackupAccount(accounts, id, "")
		if account == nil {
			return fmt.Errorf("云备份账号 %s 不存在", id)
		}
		vars := map[string]any{}
		if strings.TrimSpace(account.Vars) != "" {
			if err := json.Unmarshal([]byte(account.Vars), &vars); err != nil {
				return fmt.Errorf("云备份账号 %s Vars 无效: %w", account.Name, err)
			}
		}
		if valueString(vars, "upload_url", "upload_endpoint") == "" {
			continue
		}
		provider, _, err := configuredBackupProvider(*account)
		if err != nil {
			return fmt.Errorf("初始化云备份账号 %s 失败: %w", account.Name, err)
		}
		target := filepath.Join(account.BackupPath, item.FileName)
		if err := provider.Upload(ctx, item.Path, target); err != nil {
			return fmt.Errorf("上传到云备份账号 %s 失败: %w", account.Name, err)
		}
		item.AccountType, item.AccountName = account.Type, account.Name
	}
	return nil
}
