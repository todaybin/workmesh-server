// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

const logsSchemaSQL = `
CREATE TABLE IF NOT EXISTS operation_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL DEFAULT 'server',
    user TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    node TEXT NOT NULL DEFAULT 'local',
    path TEXT NOT NULL DEFAULT '',
    method TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    latency INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    detail_zh TEXT NOT NULL DEFAULT '',
    detail_en TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_operation_logs_created ON operation_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_operation_logs_source_status ON operation_logs(source,status,created_at DESC);
CREATE TABLE IF NOT EXISTS login_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ip TEXT NOT NULL DEFAULT '',
    user TEXT NOT NULL DEFAULT '',
    address TEXT NOT NULL DEFAULT '',
    agent TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_login_logs_created ON login_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_login_logs_status ON login_logs(status,created_at DESC)`

// LogsSchemaMigration 为日志表和查询索引提供可追踪的 SQLite 版本迁移。
func LogsSchemaMigration() storage.Migration {
	return storage.SQLMigration("0013-log-audit-v2", logsSchemaSQL)
}

// taskLogRecord 是任务列表接口对齐 1Panel TaskDTO 的稳定输出结构。
// 任务状态和日志内容均来自 SQLite，不从固定 JSON 或内存快照伪造。
type taskLogRecord struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Type            string    `json:"type"`
	Operate         string    `json:"operate"`
	LogFile         string    `json:"logFile"`
	Status          string    `json:"status"`
	ErrorMsg        string    `json:"errorMsg"`
	OperationLogID  uint      `json:"operationLogID"`
	ResourceID      uint      `json:"resourceID"`
	CurrentStep     string    `json:"currentStep"`
	ProgressCurrent int       `json:"progressCurrent"`
	ProgressTotal   int       `json:"progressTotal"`
	ProgressPercent int       `json:"progressPercent"`
	EndAt           time.Time `json:"endAt"`
	CreatedAt       time.Time `json:"createdAt"`
}

// findPersistedLog 按接口请求类型从 SQLite 查找日志详情，优先保证最新请求可立即读取。
func findPersistedLog(db storage.SQLExecutor, id, logType string) (logItem, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return logItem{}, false, nil
	}
	if strings.EqualFold(logType, "login") || strings.EqualFold(logType, "") {
		if item, found, err := findLoginLog(db, id); err != nil || found {
			return item, found, err
		}
	}
	if strings.EqualFold(logType, "operation") || strings.EqualFold(logType, "") {
		if item, found, err := findOperationLog(db, id); err != nil || found {
			return item, found, err
		}
	}
	return logItem{}, false, nil
}

// findLoginLog 按数字主键读取一条登录日志。
func findLoginLog(db storage.SQLExecutor, id string) (logItem, bool, error) {
	number, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return logItem{}, false, nil
	}
	var ip, user, address, agent, status, message, created string
	err = db.QueryRow(`SELECT id,ip,user,address,agent,status,message,created_at FROM login_logs WHERE id=?`, number).Scan(&number, &ip, &user, &address, &agent, &status, &message, &created)
	if err == sql.ErrNoRows {
		return logItem{}, false, nil
	}
	if err != nil {
		return logItem{}, false, err
	}
	normalized := normalizeLoginStatus(status)
	return logItem{ID: strconv.FormatInt(number, 10), Type: "login", Level: normalized, Status: normalized, IP: operationClientIP(ip), User: user, Address: address, UserAgent: agent, Message: message, CreatedAt: parseStoredLogTime(created)}, true, nil
}

// findOperationLog 按数字主键读取一条操作日志。
func findOperationLog(db storage.SQLExecutor, id string) (logItem, bool, error) {
	number, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return logItem{}, false, nil
	}
	var source, user, ip, node, path, method, userAgent, status, message, detailZH, detailEN, created string
	var latency int64
	err = db.QueryRow(`SELECT id,source,user,ip,node,path,method,user_agent,latency,status,message,detail_zh,detail_en,created_at FROM operation_logs WHERE id=?`, number).Scan(&number, &source, &user, &ip, &node, &path, &method, &userAgent, &latency, &status, &message, &detailZH, &detailEN, &created)
	if err == sql.ErrNoRows {
		return logItem{}, false, nil
	}
	if err != nil {
		return logItem{}, false, err
	}
	path = normalizeOperationPath(path)
	method = strings.ToLower(strings.TrimSpace(method))
	if source == "" || strings.EqualFold(source, "server") {
		source = operationSource(path)
	}
	if node == "" {
		node = "local"
	}
	status = operationStatus(status)
	detailZH, detailEN = operationDetails(method, path, detailZH, detailEN)
	return logItem{ID: strconv.FormatInt(number, 10), Type: "operation", Level: status, Status: status, Source: source, User: user, IP: operationClientIP(ip), Node: node, Path: path, Method: method, UserAgent: userAgent, Latency: latency, Message: message, DetailZH: detailZH, DetailEN: detailEN, Meta: map[string]any{"method": method, "path": path}, CreatedAt: parseStoredLogTime(created)}, true, nil
}

// filterTaskLogs applies the 1Panel task search contract before pagination。
func filterTaskLogs(items []taskLogRecord, values map[string]any) []taskLogRecord {
	typ, status, taskID := strings.ToLower(valueString(values, "type")), strings.ToLower(valueString(values, "status")), strings.TrimSpace(valueString(values, "taskID", "id"))
	filtered := items[:0]
	for _, item := range items {
		if taskID != "" && item.ID != taskID {
			continue
		}
		if typ != "" && !strings.EqualFold(item.Type, typ) {
			continue
		}
		if status != "" && !strings.EqualFold(item.Status, status) && !strings.EqualFold(item.Operate, status) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// searchTaskLogs 输出 SQLite 中的任务分页列表；任务表是唯一事实来源。
func searchTaskLogs(w http.ResponseWriter, values map[string]any, _ *domainStore) {
	if db := sharedDB(); db != nil {
		page, err := queryTaskLogPage(db, values)
		if err != nil {
			domainError(w, 500, "TASK_QUERY", err.Error())
			return
		}
		success(w, map[string]any{"items": redactTaskLogRecords(page.Items), "total": page.Total, "page": page.Page, "pageSize": page.Size})
		return
	}
	page, size := intValue(values, "page"), intValue(values, "pageSize")
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 500 {
		size = 50
	}
	success(w, map[string]any{"items": []taskLogRecord{}, "total": 0, "page": page, "pageSize": size})
}

// normalizeLogItems applies the shared text/status/date filters used by login and operation logs。
func normalizeLogItems(items []logItem, values map[string]any) []logItem {
	keyword := strings.ToLower(strings.TrimSpace(valueString(values, "keyword", "search", "message", "operation", "info")))
	typ := strings.ToLower(valueString(values, "type", "logType"))
	level := strings.ToLower(valueString(values, "level", "status"))
	source := strings.ToLower(valueString(values, "source"))
	node := strings.ToLower(valueString(values, "node"))
	start, end := parseAnalyticsTime(values["startTime"]), parseAnalyticsTime(values["endTime"])
	filtered := items[:0]
	for _, item := range items {
		searchText := strings.ToLower(strings.Join([]string{item.Message, item.DetailZH, item.DetailEN, item.Path, item.Method, item.User, item.Address, item.UserAgent}, " "))
		if keyword != "" && !strings.Contains(searchText, keyword) {
			continue
		}
		if typ != "" && !strings.EqualFold(item.Type, typ) {
			continue
		}
		if level != "" && !strings.EqualFold(item.Level, level) && !strings.EqualFold(item.Status, level) {
			continue
		}
		if source != "" && !strings.EqualFold(item.Source, source) {
			continue
		}
		if node != "" && !strings.EqualFold(item.Node, node) {
			continue
		}
		if !start.IsZero() && item.CreatedAt.Before(start) || !end.IsZero() && item.CreatedAt.After(end) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// parseStoredLogTime 解析新旧版本可能使用的日志时间格式。
func parseStoredLogTime(value string) time.Time {
	if parsed := parseAnalyticsTime(value); !parsed.IsZero() {
		return parsed
	}
	return time.Time{}
}

// normalizeLoginStatus 把旧版大小写和失败别名转换为前端状态值。
func normalizeLoginStatus(status string) string {
	if strings.EqualFold(strings.TrimSpace(status), "failed") || strings.EqualFold(strings.TrimSpace(status), "error") {
		return "Failed"
	}
	if strings.TrimSpace(status) == "" {
		return ""
	}
	return "Success"
}

// appTaskType 为没有独立 type 列的公共任务记录推导稳定类型。
func appTaskType(name, installID string) string {
	if strings.TrimSpace(installID) != "" {
		return "app"
	}
	name = strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(name, "docker"):
		return "container"
	case strings.HasPrefix(name, "file"):
		return "file"
	case strings.HasPrefix(name, "runtime"):
		return "runtime"
	default:
		return "task"
	}
}

// numericResourceID 将兼容 ID 安全转换为前端旧接口的数字字段。
func numericResourceID(value string) uint {
	number, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
	if err != nil {
		return 0
	}
	return uint(number)
}

// isTerminalTaskStatus 判断任务是否已结束，用于计算 EndAt 兼容值。
func isTerminalTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "completed", "complete", "failed", "error", "stopped", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

// paginateLogItems 统一处理分页边界，避免负数页码触发切片越界。
func paginateLogItems[T any](items []T, values map[string]any) ([]T, int, int, int) {
	page, size := intValue(values, "page"), intValue(values, "pageSize")
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 500 {
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
	return items[start:end], total, page, size
}
