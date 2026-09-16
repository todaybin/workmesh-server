// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// persistedLogPage 是 SQLite 日志检索的稳定分页结果。
type persistedLogPage struct {
	Items []logItem
	Total int
	Page  int
	Size  int
}

// taskLogPage 是公共任务表检索的稳定分页结果。
type taskLogPage struct {
	Items []taskLogRecord
	Total int
	Page  int
	Size  int
}

// sqlLogPageBounds 规范化前端分页参数，并将单页读取限制在合理范围内。
func sqlLogPageBounds(values map[string]any) (page, size, offset int) {
	page, size = intValue(values, "page"), intValue(values, "pageSize")
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 500 {
		size = 50
	}
	return page, size, (page - 1) * size
}

// queryLoginLogPage 在 SQLite 中完成登录日志筛选、计数和分页，避免将全表载入进程内存。
func queryLoginLogPage(db storage.SQLExecutor, values map[string]any) (persistedLogPage, error) {
	where, args := sqlLogWhere(values, "login")
	total, err := sqlLogCount(db, "login_logs", where, args)
	if err != nil {
		return persistedLogPage{}, err
	}
	page, size, offset := sqlLogPageBounds(values)
	rows, err := db.Query(`SELECT id,ip,user,address,agent,status,message,created_at FROM login_logs`+where+` ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`, append(args, size, offset)...)
	if err != nil {
		return persistedLogPage{}, err
	}
	defer rows.Close()
	items := make([]logItem, 0, size)
	for rows.Next() {
		var id int64
		var ip, user, address, agent, status, message, created string
		if err := rows.Scan(&id, &ip, &user, &address, &agent, &status, &message, &created); err != nil {
			return persistedLogPage{}, err
		}
		normalized := normalizeLoginStatus(status)
		items = append(items, logItem{ID: strconv.FormatInt(id, 10), Type: "login", Level: normalized, Status: normalized, IP: operationClientIP(ip), User: user, Address: address, UserAgent: agent, Message: message, CreatedAt: parseStoredLogTime(created)})
	}
	return persistedLogPage{Items: items, Total: total, Page: page, Size: size}, rows.Err()
}

// queryOperationLogPage 在 SQLite 中完成操作日志筛选、计数和分页，避免全量操作审计日志占用内存。
func queryOperationLogPage(db storage.SQLExecutor, values map[string]any) (persistedLogPage, error) {
	where, args := sqlLogWhere(values, "operation")
	total, err := sqlLogCount(db, "operation_logs", where, args)
	if err != nil {
		return persistedLogPage{}, err
	}
	page, size, offset := sqlLogPageBounds(values)
	rows, err := db.Query(`SELECT id,source,user,ip,node,path,method,user_agent,latency,status,message,detail_zh,detail_en,created_at FROM operation_logs`+where+` ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`, append(args, size, offset)...)
	if err != nil {
		return persistedLogPage{}, err
	}
	defer rows.Close()
	items := make([]logItem, 0, size)
	for rows.Next() {
		item, err := scanOperationLog(rows)
		if err != nil {
			return persistedLogPage{}, err
		}
		items = append(items, item)
	}
	return persistedLogPage{Items: items, Total: total, Page: page, Size: size}, rows.Err()
}

// sqlLogCount 读取筛选后的数量；列表查询始终使用相同条件避免 total 与 items 不一致。
func sqlLogCount(db storage.SQLExecutor, table, where string, args []any) (int, error) {
	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM `+table+where, args...).Scan(&total)
	return total, err
}

// sqlLogWhere 将公开日志查询字段转换为固定 SQL 片段和绑定参数，不能拼接用户输入。
func sqlLogWhere(values map[string]any, kind string) (string, []any) {
	clauses, args := []string{" WHERE 1=1"}, make([]any, 0, 10)
	if requested := strings.ToLower(valueString(values, "type", "logType")); requested != "" && requested != kind {
		clauses = append(clauses, " AND 1=0")
	}
	if keyword := valueString(values, "keyword", "search", "message", "operation", "info"); keyword != "" {
		pattern := "%" + strings.ToLower(keyword) + "%"
		if kind == "operation" {
			clauses = append(clauses, " AND lower(message || ' ' || detail_zh || ' ' || detail_en || ' ' || path || ' ' || method || ' ' || user || ' ' || ip || ' ' || user_agent) LIKE ?")
		} else {
			clauses = append(clauses, " AND lower(ip || ' ' || user || ' ' || address || ' ' || agent || ' ' || status || ' ' || message) LIKE ?")
		}
		args = append(args, pattern)
	}
	if status := strings.ToLower(valueString(values, "level", "status")); status != "" {
		clause, ok := sqlAuditStatusClause(status)
		if !ok {
			clauses = append(clauses, " AND 1=0")
		} else {
			clauses = append(clauses, " AND "+clause)
		}
	}
	if kind == "operation" {
		if source := valueString(values, "source"); source != "" {
			clauses, args = append(clauses, " AND lower(source)=lower(?)"), append(args, source)
		}
		if node := valueString(values, "node"); node != "" {
			clauses, args = append(clauses, " AND lower(node)=lower(?)"), append(args, node)
		}
	}
	if start := sqlLogFilterTime(values["startTime"]); start != "" {
		clauses, args = append(clauses, " AND created_at>=?"), append(args, start)
	}
	if end := sqlLogFilterTime(values["endTime"]); end != "" {
		clauses, args = append(clauses, " AND created_at<=?"), append(args, end)
	}
	return strings.Join(clauses, ""), args
}

// sqlAuditStatusClause 复用原有状态归一化规则，将前端状态下推为 SQLite 条件。
func sqlAuditStatusClause(status string) (string, bool) {
	switch status {
	case "failed":
		return "lower(trim(status)) IN ('failed','error')", true
	case "success":
		return "trim(status)<>'' AND lower(trim(status)) NOT IN ('failed','error')", true
	default:
		return "", false
	}
}

// sqlLogFilterTime 只接受可解析的时间，保持非法时间与旧内存筛选一致地不生效。
func sqlLogFilterTime(value any) string {
	parsed := parseAnalyticsTime(value)
	if parsed.IsZero() {
		return ""
	}
	return parsed.UTC().Format(time.RFC3339Nano)
}

// scanOperationLog 将一行操作审计记录转换为既有 API 输出。
func scanOperationLog(rows *sql.Rows) (logItem, error) {
	var id, latency int64
	var source, user, ip, node, path, method, userAgent, status, message, detailZH, detailEN, created string
	if err := rows.Scan(&id, &source, &user, &ip, &node, &path, &method, &userAgent, &latency, &status, &message, &detailZH, &detailEN, &created); err != nil {
		return logItem{}, err
	}
	path, method = normalizeOperationPath(path), strings.ToLower(strings.TrimSpace(method))
	if source == "" || strings.EqualFold(source, "server") {
		source = operationSource(path)
	}
	if strings.TrimSpace(node) == "" {
		node = "local"
	}
	status = operationStatus(status)
	detailZH, detailEN = operationDetails(method, path, detailZH, detailEN)
	return logItem{ID: strconv.FormatInt(id, 10), Type: "operation", Level: status, Status: status, Source: source, User: user, IP: operationClientIP(ip), Node: node, Path: path, Method: method, UserAgent: userAgent, Latency: latency, Message: message, DetailZH: detailZH, DetailEN: detailEN, Meta: map[string]any{"method": method, "path": path}, CreatedAt: parseStoredLogTime(created)}, nil
}

const taskSearchCTE = `WITH tasks (id,name,type,operate,log_file,status,error_msg,resource_id,current_step,progress,created_at,updated_at) AS (
SELECT t.id,COALESCE(NULLIF(i.name,''),t.step), 'app',t.step,t.log_path,t.status,t.error,t.app_install_id,t.step,t.progress,t.created_at,t.updated_at
FROM app_install_tasks t LEFT JOIN app_installs i ON i.id=t.app_install_id
UNION ALL
SELECT t.id,COALESCE(NULLIF(r.name,''),t.runtime_id), 'runtime',t.step,'',t.status,t.error,t.runtime_id,t.step,t.progress,t.created_at,t.updated_at
FROM runtime_tasks t LEFT JOIN runtime_records r ON r.id=t.runtime_id
)`

// queryTaskLogPage 在 SQLite 中合并应用和运行时任务，并只读取当前页。
func queryTaskLogPage(db storage.SQLExecutor, values map[string]any) (taskLogPage, error) {
	where, args := sqlTaskWhere(values)
	var total int
	if err := db.QueryRow(taskSearchCTE+` SELECT COUNT(*) FROM tasks`+where, args...).Scan(&total); err != nil {
		return taskLogPage{}, err
	}
	page, size, offset := sqlLogPageBounds(values)
	query := taskSearchCTE + ` SELECT id,name,type,operate,log_file,status,error_msg,resource_id,current_step,progress,created_at,updated_at FROM tasks` + where + ` ORDER BY created_at DESC,id DESC,type DESC LIMIT ? OFFSET ?`
	rows, err := db.Query(query, append(args, size, offset)...)
	if err != nil {
		return taskLogPage{}, err
	}
	defer rows.Close()
	items := make([]taskLogRecord, 0, size)
	for rows.Next() {
		item, err := scanTaskLog(rows)
		if err != nil {
			return taskLogPage{}, err
		}
		items = append(items, item)
	}
	return taskLogPage{Items: items, Total: total, Page: page, Size: size}, rows.Err()
}

// sqlTaskWhere 将任务列表筛选限制在固定列和绑定参数内。
func sqlTaskWhere(values map[string]any) (string, []any) {
	clauses, args := []string{" WHERE 1=1"}, make([]any, 0, 3)
	if taskID := valueString(values, "taskID", "id"); taskID != "" {
		clauses, args = append(clauses, " AND id=?"), append(args, taskID)
	}
	if typ := strings.ToLower(valueString(values, "type")); typ != "" {
		clauses, args = append(clauses, " AND type=?"), append(args, typ)
	}
	if status := strings.ToLower(valueString(values, "status")); status != "" {
		switch status {
		case "success":
			clauses = append(clauses, " AND lower(status)='running'")
		case "failed":
			clauses = append(clauses, " AND lower(status) IN ('failed','error')")
		case "executing":
			clauses = append(clauses, " AND lower(status) NOT IN ('running','failed','error')")
		case "canceled", "cancelled":
			clauses = append(clauses, " AND lower(status) IN ('canceled','cancelled')")
		default:
			clauses = append(clauses, " AND 1=0")
		}
	}
	return strings.Join(clauses, ""), args
}

// scanTaskLog 转换任务检索行，同时保持 TaskDTO 的状态和时间字段。
func scanTaskLog(rows *sql.Rows) (taskLogRecord, error) {
	var id, name, typ, operate, logFile, status, taskError, resourceID, step, created, updated string
	var progress int
	if err := rows.Scan(&id, &name, &typ, &operate, &logFile, &status, &taskError, &resourceID, &step, &progress, &created, &updated); err != nil {
		return taskLogRecord{}, err
	}
	if typ == "runtime" {
		logFile = runtimeTaskLogPath(id)
	}
	createdAt, endAt := parseStoredLogTime(created), parseStoredLogTime(updated)
	if endAt.IsZero() && isTerminalTaskStatus(status) {
		endAt = createdAt
	}
	return taskLogRecord{ID: id, Name: name, Type: typ, Operate: operate, LogFile: logFile, Status: appTaskLevel(status), ErrorMsg: taskError, ResourceID: numericResourceID(resourceID), CurrentStep: step, ProgressCurrent: progress, ProgressTotal: 100, ProgressPercent: progress, EndAt: endAt, CreatedAt: createdAt}, nil
}

// queryRuntimeTaskLogLinesPage 从 SQLite 读取某个任务的一页日志行，避免读取完整任务输出。
func queryRuntimeTaskLogLinesPage(db storage.SQLExecutor, taskID string, values map[string]any) ([]string, int, int, int, error) {
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runtime_task_logs WHERE task_id=?`, taskID).Scan(&total); err != nil {
		return nil, 0, 0, 0, err
	}
	page, size, offset := sqlTaskLinePageBounds(values)
	if boolValue(values, "latest") && total > 0 {
		page = (total + size - 1) / size
		offset = (page - 1) * size
	}
	rows, err := db.Query(`SELECT line FROM runtime_task_logs WHERE task_id=? ORDER BY id LIMIT ? OFFSET ?`, taskID, size, offset)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer rows.Close()
	lines := make([]string, 0, size)
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, 0, 0, 0, err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, 0, err
	}
	return lines, total, page, size, nil
}

// sqlTaskLinePageBounds 保留任务明细接口历史的每页 100 行默认值。
func sqlTaskLinePageBounds(values map[string]any) (page, size, offset int) {
	page, size = intValue(values, "page"), intValue(values, "pageSize")
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 500 {
		size = 100
	}
	return page, size, (page - 1) * size
}

// formatSQLPageTotal 返回旧任务日志接口使用的总页数。
func formatSQLPageTotal(total, size int) int {
	if size < 1 {
		return 0
	}
	return (total + size - 1) / size
}
