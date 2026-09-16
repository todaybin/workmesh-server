// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
				err := copyBackupStream(file, target, 128<<20)
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
	if !validBackupPath(source) || !validBackupPath(targetDir) {
		domainError(w, 400, "INVALID_TARGET", "上传路径无效")
		return
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
	v, err := requestMap(r)
	if err != nil {
		domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	typ := strings.ToLower(valueString(v, "type"))
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
	id, name := valueID(v, "id", "accountId"), valueString(v, "name", "accountName")
	s.mu.RLock()
	account := findBackupAccount(s.state.BackupAccounts, id, name)
	if account == nil {
		s.mu.RUnlock()
		if id == "" && name == "" {
			domainError(w, http.StatusServiceUnavailable, "BACKUP_PROVIDER_UNAVAILABLE", "备份账号未配置 Bucket 查询端点")
			return
		}
		domainError(w, http.StatusNotFound, "NOT_FOUND", "备份账号不存在")
		return
	}
	accountCopy := *account
	s.mu.RUnlock()
	var vars map[string]any
	if err := json.Unmarshal([]byte(accountCopy.Vars), &vars); err != nil || vars == nil {
		vars = map[string]any{}
	}
	endpoint := valueString(vars, "buckets_url", "bucket_url", "endpoint")
	if endpoint == "" {
		domainError(w, http.StatusServiceUnavailable, "BACKUP_PROVIDER_UNAVAILABLE", "备份账号未配置 Bucket 查询端点")
		return
	}
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" {
		domainError(w, http.StatusBadRequest, "BACKUP_PROVIDER_URL_INVALID", "Bucket 查询端点必须是无查询凭据的 HTTP(S) 地址")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		domainError(w, http.StatusBadRequest, "BACKUP_PROVIDER_REQUEST", err.Error())
		return
	}
	if token := valueString(vars, "access_token", "token"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		domainError(w, http.StatusBadGateway, "BACKUP_PROVIDER_FAILED", fmt.Sprintf("Bucket 查询失败: %v", err))
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		domainError(w, http.StatusBadGateway, "BACKUP_PROVIDER_READ", err.Error())
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		domainError(w, http.StatusBadGateway, "BACKUP_PROVIDER_FAILED", fmt.Sprintf("Bucket 端点返回 HTTP %d", resp.StatusCode))
		return
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		domainError(w, http.StatusBadGateway, "BACKUP_PROVIDER_RESPONSE_INVALID", "Bucket 端点返回的 JSON 无效")
		return
	}
	items := normalizeBuckets(payload, typ)
	if items == nil {
		domainError(w, http.StatusBadGateway, "BACKUP_PROVIDER_RESPONSE_INVALID", "Bucket 端点未返回列表")
		return
	}
	success(w, items)
}

// handleBackupBucketsV2 通过统一 Provider 获取云端 Bucket，失败时返回明确错误。
func handleBackupBucketsV2(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, err := requestMap(r)
	if err != nil {
		domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	typ := strings.ToLower(valueString(v, "type"))
	if typ == "local" || typ == "" {
		handleBackupBuckets(w, r, s)
		return
	}
	id, name := valueID(v, "id", "accountId"), valueString(v, "name", "accountName")
	s.mu.RLock()
	account := findBackupAccount(s.state.BackupAccounts, id, name)
	if account == nil {
		s.mu.RUnlock()
		if id == "" && name == "" {
			domainError(w, http.StatusServiceUnavailable, "BACKUP_PROVIDER_UNAVAILABLE", "备份账号未配置 Bucket 查询端点")
			return
		}
		domainError(w, http.StatusNotFound, "NOT_FOUND", "备份账号不存在")
		return
	}
	accountCopy := *account
	s.mu.RUnlock()
	provider, _, err := configuredBackupProvider(accountCopy)
	if err != nil {
		domainError(w, http.StatusServiceUnavailable, "BACKUP_PROVIDER_UNAVAILABLE", err.Error())
		return
	}
	buckets, err := provider.ListBuckets(r.Context())
	if err != nil {
		domainError(w, http.StatusBadGateway, "BACKUP_PROVIDER_FAILED", err.Error())
		return
	}
	items := make([]map[string]any, 0, len(buckets))
	for _, bucket := range buckets {
		item := map[string]any{"name": bucket.Name, "type": typ}
		if bucket.Region != "" {
			item["region"] = bucket.Region
		}
		items = append(items, item)
	}
	success(w, items)
}
