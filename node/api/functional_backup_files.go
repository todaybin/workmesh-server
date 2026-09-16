// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// handleBackupRefreshTokenV2 通过统一 Provider 刷新 OAuth，并原子保存新令牌。
func handleBackupRefreshTokenV2(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, err := requestMap(r)
	if err != nil {
		domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	id, name := valueID(v, "id", "accountId"), valueString(v, "name")
	s.mu.Lock()
	index := -1
	for i := range s.state.BackupAccounts {
		if (id != "" && s.state.BackupAccounts[i].ID == id) || (name != "" && strings.EqualFold(s.state.BackupAccounts[i].Name, name)) {
			index = i
			break
		}
	}
	if index < 0 {
		s.mu.Unlock()
		domainError(w, http.StatusNotFound, "NOT_FOUND", "备份账号不存在")
		return
	}
	account := s.state.BackupAccounts[index]
	provider, vars, providerErr := configuredBackupProvider(account)
	refreshToken := valueString(v, "refreshToken", "token")
	if refreshToken == "" {
		refreshToken = valueString(vars, "refresh_token")
	}
	if providerErr != nil || refreshToken == "" {
		s.mu.Unlock()
		if providerErr != nil {
			domainError(w, http.StatusServiceUnavailable, "TOKEN_REFRESH_UNAVAILABLE", providerErr.Error())
		} else {
			domainError(w, http.StatusServiceUnavailable, "TOKEN_REFRESH_UNAVAILABLE", "缺少 OAuth refresh_token")
		}
		return
	}
	s.mu.Unlock()
	result, err := provider.RefreshToken(r.Context(), refreshToken)
	if err != nil {
		domainError(w, http.StatusBadGateway, "TOKEN_REFRESH_FAILED", err.Error())
		return
	}
	vars["access_token"] = result.AccessToken
	if result.RefreshToken != "" {
		vars["refresh_token"] = result.RefreshToken
	}
	if result.TokenType != "" {
		vars["token_type"] = result.TokenType
	}
	if result.ExpiresIn > 0 {
		vars["expires_in"] = result.ExpiresIn
	}
	vars["refresh_status"], vars["refresh_time"] = "success", time.Now().UTC().Format(time.RFC3339)
	encoded, marshalErr := json.Marshal(vars)
	if marshalErr != nil {
		domainError(w, http.StatusInternalServerError, "STATE_SAVE", marshalErr.Error())
		return
	}
	s.mu.Lock()
	index = -1
	for i := range s.state.BackupAccounts {
		if s.state.BackupAccounts[i].ID == account.ID {
			index = i
			break
		}
	}
	if index < 0 {
		s.mu.Unlock()
		domainError(w, http.StatusNotFound, "NOT_FOUND", "备份账号已被删除")
		return
	}
	s.state.BackupAccounts[index].Vars = string(encoded)
	s.state.BackupAccounts[index].UpdatedAt = time.Now().UTC()
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		domainError(w, http.StatusInternalServerError, "STATE_SAVE", err.Error())
		return
	}
	updated := s.state.BackupAccounts[index].ID
	s.mu.Unlock()
	success(w, map[string]any{"updated": true, "id": updated, "status": "success"})
}

// normalizeBuckets 将常见云厂商列表响应转换为统一 DTO；不接受无限制嵌套结构。
func normalizeBuckets(payload any, typ string) []map[string]any {
	var raw []any
	switch value := payload.(type) {
	case []any:
		raw = value
	case map[string]any:
		for _, key := range []string{"buckets", "items", "data"} {
			if list, ok := value[key].([]any); ok {
				raw = list
				break
			}
		}
	default:
		return nil
	}
	if len(raw) > 500 {
		raw = raw[:500]
	}
	items := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		if text, ok := entry.(string); ok && strings.TrimSpace(text) != "" {
			items = append(items, map[string]any{"name": strings.TrimSpace(text), "type": typ})
			continue
		}
		obj, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		name := valueString(obj, "name", "bucket", "id")
		if name == "" {
			continue
		}
		item := map[string]any{"name": name, "type": typ}
		if region := valueString(obj, "region", "location"); region != "" {
			item["region"] = region
		}
		items = append(items, item)
	}
	return items
}

// handleBackupConnCheckV2 对云端账号执行真实 HTTP 连通性检查，本地账号保留目录检查。
func handleBackupConnCheckV2(w http.ResponseWriter, r *http.Request, s *domainStore) {
	v, err := requestMap(r)
	if err != nil {
		domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	item := backupRequestAccount(v)
	if item.Type == "local" {
		path := item.BackupPath
		if path == "" {
			path = backupDataDir()
		}
		err := os.MkdirAll(path, 0o750)
		success(w, map[string]any{"isOk": err == nil, "msg": errorMessage(err), "token": ""})
		return
	}
	if item.Type == "" {
		domainError(w, http.StatusBadRequest, "INVALID_ACCOUNT", "备份类型不能为空")
		return
	}
	provider, _, providerErr := configuredBackupProvider(item)
	if providerErr != nil {
		success(w, map[string]any{"isOk": false, "msg": providerErr.Error(), "token": ""})
		return
	}
	checkErr := provider.Check(r.Context())
	success(w, map[string]any{"isOk": checkErr == nil, "msg": errorMessage(checkErr), "token": ""})
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
	refreshToken := valueString(v, "refreshToken", "token")
	if refreshToken == "" {
		if token, ok := vars["refresh_token"].(string); ok {
			refreshToken = strings.TrimSpace(token)
		}
	}
	refreshURL, _ := vars["refresh_url"].(string)
	if refreshToken == "" || strings.TrimSpace(refreshURL) == "" {
		domainError(w, http.StatusServiceUnavailable, "TOKEN_REFRESH_UNAVAILABLE", "备份账号缺少 refresh_token 或 refresh_url")
		return
	}
	u, err := url.Parse(strings.TrimSpace(refreshURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" {
		domainError(w, http.StatusBadRequest, "TOKEN_REFRESH_URL_INVALID", "refresh_url 必须是无查询凭据的 HTTP(S) 地址")
		return
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(form.Encode()))
	if err != nil {
		domainError(w, 400, "TOKEN_REFRESH_REQUEST", err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		domainError(w, http.StatusBadGateway, "TOKEN_REFRESH_FAILED", fmt.Sprintf("刷新备份账号令牌失败: %v", err))
		return
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		domainError(w, 502, "TOKEN_REFRESH_READ", readErr.Error())
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		domainError(w, http.StatusBadGateway, "TOKEN_REFRESH_FAILED", fmt.Sprintf("令牌端点返回 HTTP %d", resp.StatusCode))
		return
	}
	var tokenResponse struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResponse); err != nil || strings.TrimSpace(tokenResponse.AccessToken) == "" {
		domainError(w, http.StatusBadGateway, "TOKEN_REFRESH_RESPONSE_INVALID", "令牌端点未返回 access_token")
		return
	}
	vars["access_token"] = tokenResponse.AccessToken
	if tokenResponse.RefreshToken != "" {
		vars["refresh_token"] = tokenResponse.RefreshToken
	}
	if tokenResponse.ExpiresIn > 0 {
		vars["expires_in"] = tokenResponse.ExpiresIn
	}
	vars["refresh_status"], vars["refresh_time"] = "success", time.Now().UTC().Format(time.RFC3339)
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
	out, err := os.CreateTemp(filepath.Dir(target), ".backup-upload-*")
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer func() { _ = os.Remove(tmp) }()
	_ = out.Chmod(0o640)
	_, copyErr := io.Copy(out, io.LimitReader(in, max+1))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(tmp, target); err != nil {
		return err
	}
	return nil
}

func copyBackupStream(source io.Reader, target string, max int64) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(target), ".backup-upload-*")
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer func() { _ = os.Remove(tmp) }()
	_ = out.Chmod(0o640)
	_, copyErr := io.Copy(out, io.LimitReader(source, max+1))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(tmp, target); err != nil {
		return err
	}
	return nil
}

func copyBackupTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("备份目录不允许包含符号链接")
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
