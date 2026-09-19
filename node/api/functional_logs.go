// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"context"
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
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// sharedLogStorage 返回共享 SQLite 的日志存储边界；无共享数据库时保留文件/内存兼容路径。
func sharedLogStorage() (storage.Transactional, bool) {
	db := sharedDB()
	if db == nil {
		return nil, false
	}
	repo, err := storage.NewSQLiteRepository(db)
	if err != nil {
		return nil, false
	}
	return repo, true
}

// registerLogRoutes 注册面板日志、系统日志和任务日志的 HTTP 入口，并统一复用 SQLite 查询结果。
func registerLogRoutes(mux *http.ServeMux, s *domainStore) {
	search := func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
		if strings.Contains(r.URL.Path, "/logs/tasks/search") {
			searchTaskLogs(w, v, s)
			return
		}
		if db, ok := sharedLogStorage(); ok && strings.Contains(r.URL.Path, "/logs/login") {
			page, queryErr := queryLoginLogPage(db, v)
			if queryErr != nil {
				domainError(w, http.StatusInternalServerError, "LOG_QUERY", queryErr.Error())
				return
			}
			success(w, map[string]any{"items": redactLogItems(page.Items), "total": page.Total, "page": page.Page, "pageSize": page.Size})
			return
		}
		if db, ok := sharedLogStorage(); ok && strings.Contains(r.URL.Path, "/logs/operation") {
			page, queryErr := queryOperationLogPage(db, v)
			if queryErr != nil {
				domainError(w, http.StatusInternalServerError, "LOG_QUERY", queryErr.Error())
				return
			}
			success(w, map[string]any{"items": redactLogItems(page.Items), "total": page.Total, "page": page.Page, "pageSize": page.Size})
			return
		}
		page, size := intValue(v, "page"), intValue(v, "pageSize")
		if page < 1 {
			page = 1
		}
		if size < 1 || size > 500 {
			size = 50
		}
		success(w, map[string]any{"items": []logItem{}, "total": 0, "page": page, "pageSize": size})
	}
	for _, path := range []string{"/api/v2/logs/search", "/api/v2/log/search", "/api/v2/logs/tasks/search", "/api/v2/core/logs/login", "/api/v2/core/logs/operation"} {
		mux.HandleFunc("POST "+path, search)
	}
	mux.HandleFunc("POST /api/v2/logs/detail", func(w http.ResponseWriter, r *http.Request) {
		v, _ := requestMap(r)
		id := valueString(v, "id")
		if db, ok := sharedLogStorage(); ok {
			if item, found, err := findPersistedLog(db, id, valueString(v, "type", "logType")); err != nil {
				domainError(w, http.StatusInternalServerError, "LOG_QUERY", err.Error())
				return
			} else if found {
				success(w, redactLogItem(item))
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
			if logType != "" && logType != "login" && logType != "operation" {
				domainError(w, http.StatusBadRequest, "INVALID_LOG_TYPE", "仅支持清理 login 或 operation 日志")
				return
			}
			if repo, ok := sharedLogStorage(); ok {
				clearErr := repo.WithTx(r.Context(), func(tx storage.SQLExecutor) error {
					if logType == "" {
						if _, err := tx.ExecContext(r.Context(), `DELETE FROM operation_logs`); err != nil {
							return err
						}
						_, err := tx.ExecContext(r.Context(), `DELETE FROM login_logs`)
						return err
					}
					_, err := tx.ExecContext(r.Context(), `DELETE FROM `+logType+`_logs`)
					return err
				})
				if clearErr != nil {
					domainError(w, http.StatusInternalServerError, "LOG_CLEAR", clearErr.Error())
					return
				}
			}
			success(w, nil)
		})
	}
	mux.HandleFunc("POST /api/v2/logs/stat", func(w http.ResponseWriter, _ *http.Request) {
		if db, ok := sharedLogStorage(); ok {
			var operationCount, loginCount int
			if err := db.QueryRow(`SELECT COUNT(*) FROM operation_logs`).Scan(&operationCount); err == nil {
				_ = db.QueryRow(`SELECT COUNT(*) FROM login_logs`).Scan(&loginCount)
				success(w, map[string]any{"total": operationCount + loginCount})
				return
			}
		}
		success(w, map[string]any{"total": 0})
	})
	mux.HandleFunc("POST /api/v2/logs/system/read", func(w http.ResponseWriter, r *http.Request) { readLogFile(w, r) })
	mux.HandleFunc("POST /api/v2/logs/tasks/read", func(w http.ResponseWriter, r *http.Request) { readTaskLog(w, r, s) })
	mux.HandleFunc("GET /api/v2/logs/tasks/read", func(w http.ResponseWriter, r *http.Request) { readTaskLog(w, r, s) })
	mux.HandleFunc("GET /api/v2/logs/system/files", func(w http.ResponseWriter, _ *http.Request) { success(w, listSystemLogFiles()) })
	mux.HandleFunc("GET /api/v2/logs/system/services", func(w http.ResponseWriter, _ *http.Request) { success(w, listRunningSystemServices()) })
	mux.HandleFunc("GET /api/v2/logs/system/status", func(w http.ResponseWriter, _ *http.Request) { success(w, systemLogStatus()) })
	// 执行中任务接口的 data 必须是数字，前端直接将其作为计数器使用。
	mux.HandleFunc("GET /api/v2/logs/tasks/executing/count", func(w http.ResponseWriter, _ *http.Request) {
		if db, ok := sharedLogStorage(); ok {
			var appCount, runtimeCount int
			if err := db.QueryRow(`SELECT COUNT(*) FROM app_install_tasks WHERE lower(status) IN ('executing','running','installing','building','downloading','pulling','starting','recreating')`).Scan(&appCount); err == nil {
				_ = db.QueryRow(`SELECT COUNT(*) FROM runtime_tasks WHERE lower(status) IN ('executing','running','installing','building','downloading','pulling','starting','recreating')`).Scan(&runtimeCount)
				success(w, appCount+runtimeCount)
				return
			}
		}
		success(w, 0)
	})
}

// valueStringFromRequest 从请求体中读取指定字符串字段，供日志读取接口兼容多种参数命名。
func valueStringFromRequest(r *http.Request, key string) string {
	v, _ := requestMap(r)
	return valueString(v, key)
}

// readLogFile 读取系统日志并返回前端 v2 契约；path 仍保留旧版文件读取兼容。
func readLogFile(w http.ResponseWriter, r *http.Request) {
	v, err := requestMap(r)
	if err != nil {
		domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	pageSize := intValue(v, "pageSize")
	if pageSize < 1 {
		pageSize = 100
	}
	if pageSize > 500 {
		pageSize = 500
	}
	cursor, err := decodeSystemLogReadCursor(valueString(v, "cursor"))
	if err != nil {
		domainError(w, http.StatusBadRequest, "INVALID_CURSOR", err.Error())
		return
	}
	path := valueString(v, "path")
	if path == "" && !hasSystemLogReadOptions(v) {
		domainError(w, http.StatusBadRequest, "INVALID_PATH", "日志读取参数不能为空")
		return
	}
	if path != "" {
		if !allowedLogPath(path) {
			domainError(w, http.StatusForbidden, "PATH_FORBIDDEN", "日志路径不在允许目录内")
			return
		}
		items, content, readErr := readSystemLogFile(path, v)
		if readErr != nil {
			status := http.StatusInternalServerError
			if os.IsNotExist(readErr) {
				status = http.StatusNotFound
			}
			domainError(w, status, "LOG_READ", readErr.Error())
			return
		}
		page := paginateSystemLogItems(items, pageSize, cursor)
		// Keep the legacy content field bounded to the returned page. The v2
		// contract consumes items/cursor, while older callers may still render
		// content directly.
		page.items = redactSystemLogItems(page.items)
		content = systemLogPageContent(page.items)
		response := map[string]any{
			"source": "file", "items": page.items, "hasMore": page.hasMore, "nextCursor": page.nextCursor,
			// path/content 保持旧版面板调用兼容，content 仅包含当前页且有大小上限。
			"path": path, "content": content,
		}
		success(w, response)
		return
	}
	items, source, readErr := collectSystemLogItems(v)
	if readErr != nil {
		domainError(w, http.StatusInternalServerError, "LOG_READ", readErr.Error())
		return
	}
	page := paginateSystemLogItems(items, pageSize, cursor)
	page.items = redactSystemLogItems(page.items)
	success(w, map[string]any{"source": source, "items": page.items, "hasMore": page.hasMore, "nextCursor": page.nextCursor})
}

func formatSystemLogTimestamp(value string) string {
	if micros, err := strconv.ParseInt(value, 10, 64); err == nil && micros > 0 {
		return time.UnixMicro(micros).UTC().Format(time.RFC3339)
	}
	return value
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

// logDataDir 返回 WorkMesh 运行数据目录，未配置环境变量时使用相对 data 目录。
func logDataDir() string {
	if dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR")); dir != "" {
		return dir
	}
	return "./data"
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

// listRunningSystemServices 按操作系统枚举当前运行中的服务名称，供系统日志页面展示。
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

// systemLogStatus 探测系统日志来源及 journalctl 版本，返回前端可直接展示的状态信息。
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
	if path == "" && id != "" {
		path = storedTaskLogPath(id)
	}
	if id != "" {
		if handled, err := readTaskLogSQLite(w, v, id); handled {
			if err != nil {
				domainError(w, http.StatusInternalServerError, "TASK_LOG_READ", err.Error())
			}
			return
		}
	}
	taskStatus := ""
	if path == "" && id != "" {
		candidate := appTaskLogPath(id)
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
			path = candidate
		}
	}
	if path == "" && id != "" {
		candidate := runtimeTaskLogPath(id)
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
			path = candidate
		}
	}
	if path == "" {
		domainError(w, 400, "INVALID_TASK", "任务日志路径或任务 ID 不能为空")
		return
	}
	if !allowedLogPath(path) {
		domainError(w, 403, "PATH_FORBIDDEN", "日志路径不在允许目录内")
		return
	}
	page, size := intValue(v, "page"), intValue(v, "pageSize")
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 500 {
		size = 100
	}
	lines, totalLines, err := readTaskLogPage(r.Context(), path, page, size, boolValue(v, "latest"), runtimeMaxLogBytes())
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			domainError(w, http.StatusRequestTimeout, "LOG_READ", err.Error())
			return
		}
		if errors.Is(err, os.ErrNotExist) {
			domainError(w, http.StatusNotFound, "LOG_NOT_FOUND", "日志文件不存在")
			return
		}
		domainError(w, http.StatusInternalServerError, "LOG_READ", err.Error())
		return
	}
	lines = redactTaskLogLines(lines)
	if boolValue(v, "latest") && totalLines > 0 {
		page = (totalLines + size - 1) / size
	}
	success(w, map[string]any{"path": path, "lines": lines, "totalLines": totalLines, "total": (totalLines + size - 1) / size, "end": page*size >= totalLines, "scope": "page", "taskStatus": taskStatus})
}

func readTaskLogPage(ctx context.Context, path string, page, size int, latest bool, maxBytes int64) ([]string, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	reader := bufio.NewScanner(&io.LimitedReader{R: file, N: maxBytes})
	reader.Buffer(make([]byte, 4096), 1<<20)
	lines := make([]string, 0, size)
	total := 0
	start, end := (page-1)*size, page*size
	for reader.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, total, err
		}
		if latest {
			if len(lines) < size {
				lines = append(lines, reader.Text())
			} else {
				copy(lines, lines[1:])
				lines[len(lines)-1] = reader.Text()
			}
		} else if total >= start && total < end {
			lines = append(lines, reader.Text())
		}
		total++
	}
	if err := reader.Err(); err != nil {
		return nil, total, err
	}
	return lines, total, nil
}

// storedTaskLogPath 读取任务表中的日志路径，保证重启后不依赖内存任务快照。
func storedTaskLogPath(taskID string) string {
	db, ok := sharedLogStorage()
	if !ok || strings.TrimSpace(taskID) == "" {
		return ""
	}
	var path string
	if err := db.QueryRow(`SELECT log_path FROM app_install_tasks WHERE id=?`, taskID).Scan(&path); err == nil && strings.TrimSpace(path) != "" {
		return path
	}
	return runtimeTaskLogPath(taskID)
}

// readTaskLogSQLite 从任务日志表读取持久化日志；没有记录时才兼容外部日志文件资源。
func readTaskLogSQLite(w http.ResponseWriter, v map[string]any, taskID string) (bool, error) {
	db, ok := sharedLogStorage()
	if !ok {
		return false, nil
	}
	lines, count, page, size, err := queryRuntimeTaskLogLinesPage(db, taskID, v)
	if err != nil {
		return false, err
	}
	var taskStatus string
	_ = db.QueryRow(`SELECT status FROM runtime_tasks WHERE id=?`, taskID).Scan(&taskStatus)
	if count == 0 {
		return false, nil
	}
	if taskStatus == "" {
		_ = db.QueryRow(`SELECT status FROM app_install_tasks WHERE id=?`, taskID).Scan(&taskStatus)
	}
	success(w, map[string]any{"lines": redactTaskLogLines(lines), "totalLines": count, "total": formatSQLPageTotal(count, size), "end": page*size >= count, "scope": "sqlite", "taskStatus": appTaskStatus(taskStatus)})
	return true, nil
}

// appTaskStatus 将应用安装任务状态映射为日志页面使用的统一状态名称。
func appTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "success", "completed":
		return "Success"
	case "failed", "error":
		return "Failed"
	default:
		return "Executing"
	}
}
