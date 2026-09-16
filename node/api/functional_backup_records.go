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
)

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
	previousRecords := cloneBackupRecords(s.state.Backups)
	kept := make([]backupItem, 0, len(s.state.Backups))
	removed := 0
	removedItems := make([]backupItem, 0)
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
			removedItems = append(removedItems, item)
			continue
		}
		kept = append(kept, item)
	}
	if removed == 0 {
		s.mu.Unlock()
		domainError(w, 404, "NOT_FOUND", "备份记录不存在")
		return
	}
	s.state.Backups = kept
	if err := s.saveLocked(); err != nil {
		s.state.Backups = previousRecords
		s.mu.Unlock()
		domainError(w, 500, "STATE_SAVE", err.Error())
		return
	}
	accounts := append([]backupAccount(nil), s.state.BackupAccounts...)
	s.mu.Unlock()
	if err := deleteBackupRemote(r.Context(), accounts, removedItems); err != nil {
		s.mu.Lock()
		s.state.Backups = restoreBackupRecords(s.state.Backups, previousRecords, removedItems)
		rollbackErr := s.saveLocked()
		s.mu.Unlock()
		if rollbackErr != nil {
			domainError(w, http.StatusBadGateway, "BACKUP_PROVIDER_DELETE_ROLLBACK", fmt.Sprintf("%v; 恢复备份记录失败: %v", err, rollbackErr))
			return
		}
		domainError(w, http.StatusBadGateway, "BACKUP_PROVIDER_DELETE", err.Error())
		return
	}
	for _, item := range removedItems {
		if !isWithin(item.Path, backupDataDir()) {
			continue
		}
		if err := os.RemoveAll(item.Path); err != nil {
			domainError(w, http.StatusInternalServerError, "BACKUP_STORAGE_DELETE", err.Error())
			return
		}
	}
	success(w, map[string]any{"deleted": removed})
}

func cloneBackupRecords(records []backupItem) []backupItem {
	if records == nil {
		return nil
	}
	return append([]backupItem{}, records...)
}

func restoreBackupRecords(current, previous, removed []backupItem) []backupItem {
	removedIDs := make(map[string]struct{}, len(removed))
	for _, item := range removed {
		removedIDs[item.ID] = struct{}{}
	}
	currentByID := make(map[string]backupItem, len(current))
	for _, item := range current {
		currentByID[item.ID] = item
	}
	restored := make([]backupItem, 0, len(current)+len(removed))
	added := make(map[string]struct{}, len(current)+len(removed))
	for _, item := range previous {
		if currentItem, ok := currentByID[item.ID]; ok {
			restored = append(restored, currentItem)
			added[item.ID] = struct{}{}
			continue
		}
		if _, ok := removedIDs[item.ID]; ok {
			restored = append(restored, item)
			added[item.ID] = struct{}{}
		}
	}
	for _, item := range current {
		if _, ok := added[item.ID]; !ok {
			restored = append(restored, item)
		}
	}
	return restored
}

func valueIDs(v map[string]any, keys ...string) []string {
	var out []string
	for _, key := range keys {
		values, exists := v[key]
		if !exists {
			continue
		}
		switch values := values.(type) {
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
	}
	return out
}

// deleteBackupRemote 删除记录关联的云端对象；未声明删除端点时不执行远端副作用。
func deleteBackupRemote(ctx context.Context, accounts []backupAccount, records []backupItem) error {
	for _, record := range records {
		if record.Path == "" {
			continue
		}
		ids := valueIDs(map[string]any{"ids": record.SourceAccountIDs}, "ids")
		if len(ids) == 0 {
			ids = []string{record.DownloadAccountID}
		}
		for _, id := range ids {
			if id == "" {
				continue
			}
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
			if valueString(vars, "delete_url", "delete_endpoint") == "" {
				continue
			}
			provider, _, err := configuredBackupProvider(*account)
			if err != nil {
				return fmt.Errorf("初始化云备份账号 %s 失败: %w", account.Name, err)
			}
			target := filepath.Join(account.BackupPath, record.FileName)
			if err := provider.Delete(ctx, target); err != nil {
				return fmt.Errorf("删除云备份账号 %s 对象失败: %w", account.Name, err)
			}
		}
	}
	return nil
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
	if !validBackupPath(source) || !isWithin(source, backupDataDir()) {
		domainError(w, 404, "NOT_FOUND", "备份文件不存在")
		return
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
	if !validBackupPath(target) {
		domainError(w, 400, "INVALID_TARGET", "恢复目标路径无效")
		return
	}
	if err := copyBackupFile(source, target, 128<<20); err != nil {
		domainError(w, 500, "RECOVER_WRITE", err.Error())
		return
	}
	success(w, map[string]any{"path": target, "restored": true, "uploaded": byUpload})
}
