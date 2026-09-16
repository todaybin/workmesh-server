// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"context"

	"github.com/todaybin/workmesh-server/internal/logsource"
)

const maxSSHLogBytes = 8 << 20

// registerSSHLogRoutes 注册主机 SSH 日志真实读取、清理和导出接口。
func registerSSHLogRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/hosts/ssh/log", loadSSHLogs)
	mux.HandleFunc("POST /api/v2/hosts/ssh/log/clean", cleanSSHLogs)
	mux.HandleFunc("POST /api/v2/hosts/ssh/log/export", exportSSHLogs)
}

func sshLogDir() string {
	if dir := strings.TrimSpace(os.Getenv("WORKMESH_SSH_LOG_DIR")); dir != "" {
		return dir
	}
	return "/var/log"
}

func sshLogFiles() ([]string, error) {
	entries, err := os.ReadDir(sshLogDir())
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !isSSHLogName(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, filepath.Join(sshLogDir(), entry.Name()))
	}
	sort.Strings(files)
	return files, nil
}

func isSSHLogName(name string) bool {
	for _, base := range []string{"auth.log", "secure"} {
		if name == base || strings.HasPrefix(name, base+".") || strings.HasPrefix(name, base+"-") {
			// Rotated files may carry .1/.gz or date suffixes; all remain under
			// the explicit auth.log/secure basename allow-list.
			return true
		}
	}
	return false
}

type sshHistory struct {
	Date     time.Time `json:"date"`
	Area     string    `json:"area"`
	User     string    `json:"user"`
	AuthMode string    `json:"authMode"`
	Address  string    `json:"address"`
	Port     string    `json:"port"`
	Status   string    `json:"status"`
	Message  string    `json:"message"`
}

func loadSSHLogs(w http.ResponseWriter, r *http.Request) {
	values, err := requestMap(r)
	if err != nil {
		domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	items, err := collectSSHLogs(values)
	if err != nil {
		domainError(w, http.StatusInternalServerError, "SSH_LOG_READ", err.Error())
		return
	}
	page, size := intValue(values, "page"), intValue(values, "pageSize")
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 500 {
		size = 500
	}
	start := (page - 1) * size
	if start > len(items) {
		start = len(items)
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	success(w, map[string]any{"items": redactSSHHistories(items[start:end]), "total": len(items), "page": page, "pageSize": size})
}

func collectSSHLogs(values map[string]any) ([]sshHistory, error) {
	files, err := sshLogFiles()
	if err != nil {
		return nil, err
	}
	result, err := (logsource.FileSource{}).Read(context.Background(), logsource.Request{
		Paths: files, MaxBytesPerFile: maxSSHLogBytes, MaxLines: 100000,
	})
	if err != nil {
		return nil, err
	}
	var items []sshHistory
	for _, line := range result.Lines {
		if item, ok := parseSSHHistory(line.Text); ok && matchSSHHistory(item, values) {
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Date.After(items[j].Date) })
	return items, nil
}

func parseSSHHistory(line string) (sshHistory, bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return sshHistory{}, false
	}
	year := time.Now().Year()
	date, err := time.Parse("Jan 2 15:04:05 2006", fmt.Sprintf("%s %s %s %d", fields[0], fields[1], fields[2], year))
	if err != nil {
		return sshHistory{}, false
	}
	msg := strings.Join(fields[3:], " ")
	item := sshHistory{Date: date, Area: "", Port: "22", Message: msg, Status: "failed"}
	if i := strings.Index(msg, "sshd["); i >= 0 {
		item.Area = "sshd"
	}
	if strings.Contains(msg, "Accepted ") {
		item.Status = "success"
	}
	if strings.Contains(msg, "Accepted password") {
		item.AuthMode = "password"
	}
	if strings.Contains(msg, "Accepted publickey") {
		item.AuthMode = "publickey"
	}
	parts := strings.Fields(msg)
	for i, part := range parts {
		switch part {
		case "for":
			if i+1 < len(parts) {
				item.User = parts[i+1]
			}
		case "from":
			if i+1 < len(parts) {
				item.Address = parts[i+1]
			}
		case "port":
			if i+1 < len(parts) {
				item.Port = parts[i+1]
			}
		}
	}
	return item, true
}

func matchSSHHistory(item sshHistory, values map[string]any) bool {
	info := strings.ToLower(valueString(values, "info"))
	status := strings.ToLower(valueString(values, "status"))
	if info != "" && !strings.Contains(strings.ToLower(item.Message), info) && !strings.Contains(strings.ToLower(item.Address), info) && !strings.Contains(strings.ToLower(item.User), info) {
		return false
	}
	if status != "" && status != item.Status {
		return false
	}
	if start := parseSystemLogTime(valueString(values, "startTime")); !start.IsZero() && item.Date.Before(start) {
		return false
	}
	if end := parseSystemLogTime(valueString(values, "endTime")); !end.IsZero() && item.Date.After(end) {
		return false
	}
	return true
}

func cleanSSHLogs(w http.ResponseWriter, _ *http.Request) {
	files, err := sshLogFiles()
	if err != nil {
		domainError(w, http.StatusInternalServerError, "SSH_LOG_CLEAN", err.Error())
		return
	}
	truncatePaths := make([]string, 0, len(files))
	removePaths := make([]string, 0, len(files))
	for _, path := range files {
		base := filepath.Base(path)
		if base == "auth.log" || base == "secure" {
			truncatePaths = append(truncatePaths, path)
		} else {
			removePaths = append(removePaths, path)
		}
	}
	if len(truncatePaths) > 0 {
		if _, err = (logsource.FileMaintenance{}).Cleanup(context.Background(), logsource.CleanupRequest{
			Paths: truncatePaths, Mode: logsource.CleanupTruncate,
		}); err != nil {
			domainError(w, http.StatusInternalServerError, "SSH_LOG_CLEAN", err.Error())
			return
		}
	}
	if len(removePaths) > 0 {
		if _, err = (logsource.FileMaintenance{}).Cleanup(context.Background(), logsource.CleanupRequest{
			Paths: removePaths, Mode: logsource.CleanupRemove,
		}); err != nil {
			domainError(w, http.StatusInternalServerError, "SSH_LOG_CLEAN", err.Error())
			return
		}
	}
	success(w, nil)
}

func exportSSHLogs(w http.ResponseWriter, r *http.Request) {
	values, err := requestMap(r)
	if err != nil {
		domainError(w, 400, "INVALID_JSON", err.Error())
		return
	}
	items, err := collectSSHLogs(values)
	if err != nil {
		domainError(w, 500, "SSH_LOG_READ", err.Error())
		return
	}
	if len(items) == 0 {
		domainError(w, http.StatusNotFound, "SSH_LOG_EMPTY", "没有可导出的 SSH 日志")
		return
	}
	dir := filepath.Join(logDataDir(), "exports")
	if err = os.MkdirAll(dir, 0o750); err != nil {
		domainError(w, 500, "SSH_LOG_EXPORT", err.Error())
		return
	}
	path := filepath.Join(dir, fmt.Sprintf("ssh-log-%d.csv", time.Now().UnixNano()))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		domainError(w, 500, "SSH_LOG_EXPORT", err.Error())
		return
	}
	cw := csv.NewWriter(f)
	_ = cw.Write([]string{"address", "area", "port", "authMode", "user", "status", "date", "message"})
	for _, item := range redactSSHHistories(items) {
		_ = cw.Write([]string{item.Address, item.Area, item.Port, item.AuthMode, item.User, item.Status, item.Date.Format(time.RFC3339), item.Message})
	}
	cw.Flush()
	err = cw.Error()
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		domainError(w, 500, "SSH_LOG_EXPORT", err.Error())
		return
	}
	success(w, path)
}
