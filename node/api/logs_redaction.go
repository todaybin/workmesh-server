// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

const logRedactedValue = "[REDACTED]"

var (
	logSecretValuePattern = regexp.MustCompile(`(?i)(\b(?:token|access[_-]?token|refresh[_-]?token|csrf[_-]?token|authorization|cookie|set-cookie|password|passwd|passphrase|secret(?:[_-]?(?:key|token))?|credential|api[_-]?key|apikey|private[_-]?key|client[_-]?secret|session(?:[_-]?id)?)\b\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;&}]+)`)
	logBearerPattern      = regexp.MustCompile(`(?i)(\bBearer\s+)[A-Za-z0-9._~+/=-]+`)
	logBasicPattern       = regexp.MustCompile(`(?i)(\bBasic\s+)[A-Za-z0-9+/=]+`)
)

// redactLogText 删除日志消息、请求头和任务输出中常见的凭据值，同时保留字段名称便于排障。
func redactLogText(value string) string {
	value = logBearerPattern.ReplaceAllString(value, `${1}`+logRedactedValue)
	value = logBasicPattern.ReplaceAllString(value, `${1}`+logRedactedValue)
	return logSecretValuePattern.ReplaceAllString(value, `${1}`+logRedactedValue)
}

// redactLogItem 脱敏操作和登录日志的可变文本字段，不改变前端字段结构。
func redactLogItem(item logItem) logItem {
	item.Message = redactLogText(item.Message)
	item.DetailZH = redactLogText(item.DetailZH)
	item.DetailEN = redactLogText(item.DetailEN)
	// 路径可能携带 query token；保留路径结构但隐藏凭据值。
	item.Path = redactLogText(item.Path)
	item.UserAgent = redactLogText(item.UserAgent)
	item.Meta = redactLogMap(item.Meta)
	return item
}

// redactLogItems 对日志分页结果逐条脱敏，避免修改共享状态中的原始切片。
func redactLogItems(items []logItem) []logItem {
	if len(items) == 0 {
		return items
	}
	redacted := make([]logItem, len(items))
	for index, item := range items {
		redacted[index] = redactLogItem(item)
	}
	return redacted
}

// redactTaskLogRecord 脱敏任务错误、步骤和日志路径中的凭据值。
func redactTaskLogRecord(item taskLogRecord) taskLogRecord {
	item.Name = redactLogText(item.Name)
	item.Operate = redactLogText(item.Operate)
	item.LogFile = redactLogText(item.LogFile)
	item.ErrorMsg = redactLogText(item.ErrorMsg)
	item.CurrentStep = redactLogText(item.CurrentStep)
	return item
}

// redactTaskLogRecords 对任务搜索结果执行统一脱敏。
func redactTaskLogRecords(items []taskLogRecord) []taskLogRecord {
	if len(items) == 0 {
		return items
	}
	redacted := make([]taskLogRecord, len(items))
	for index, item := range items {
		redacted[index] = redactTaskLogRecord(item)
	}
	return redacted
}

// redactTaskLogLines 脱敏任务日志行并保留行数和顺序。
func redactTaskLogLines(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	redacted := make([]string, len(lines))
	for index, line := range lines {
		redacted[index] = redactLogText(line)
	}
	return redacted
}

// redactSystemLogItems 脱敏系统日志 map 中所有字符串值，兼容 journalctl 和文件来源。
func redactSystemLogItems(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return items
	}
	redacted := make([]map[string]any, 0, len(items))
	for _, item := range items {
		redacted = append(redacted, redactLogMap(item))
	}
	return redacted
}

// redactLogMap 深拷贝并脱敏任意日志对象，避免把数据库或文件读取结果原地改写。
func redactLogMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	result := make(map[string]any, len(value))
	for key, item := range value {
		if isSensitiveLogKey(key) {
			result[key] = logRedactedValue
			continue
		}
		result[key] = redactLogValue(item)
	}
	return result
}

// isSensitiveLogKey 判断结构化日志字段名是否属于凭据、令牌或会话秘密。
func isSensitiveLogKey(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"), " ", "_"))
	switch key {
	case "token", "access_token", "refresh_token", "authorization", "cookie", "set_cookie", "password", "passwd", "passphrase", "secret", "secret_key", "secret_token", "credential", "api_key", "apikey", "private_key", "client_secret", "session", "session_id":
		return true
	default:
		return false
	}
}

// redactLogValue 递归处理日志响应中的字符串、对象和数组值。
func redactLogValue(value any) any {
	switch typed := value.(type) {
	case string:
		return redactLogText(typed)
	case map[string]any:
		return redactLogMap(typed)
	case map[string]string:
		result := make(map[string]string, len(typed))
		for key, item := range typed {
			if isSensitiveLogKey(key) {
				result[key] = logRedactedValue
			} else {
				result[key] = redactLogText(item)
			}
		}
		return result
	case []any:
		items := make([]any, len(typed))
		for index, item := range typed {
			items[index] = redactLogValue(item)
		}
		return items
	case []string:
		items := make([]string, len(typed))
		for index, item := range typed {
			items[index] = redactLogText(item)
		}
		return items
	default:
		return value
	}
}

// redactWebsiteLogResult 仅处理网站日志接口的 content，避免原始 access/error 日志回显凭据。
func redactWebsiteLogResult(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	result := make(map[string]any, len(value))
	for key, item := range value {
		if key == "content" {
			if content, ok := item.(string); ok {
				result[key] = redactLogText(content)
				continue
			}
		}
		result[key] = redactLogValue(item)
	}
	return result
}

// redactSSHHistory 脱敏 SSH 日志消息和认证模式字段，地址、用户等审计定位字段保持可检索。
func redactSSHHistory(item sshHistory) sshHistory {
	item.AuthMode = redactLogText(item.AuthMode)
	item.Message = redactLogText(item.Message)
	return item
}

// redactSSHHistories 对主机 SSH 日志执行统一脱敏。
func redactSSHHistories(items []sshHistory) []sshHistory {
	redacted := make([]sshHistory, len(items))
	for index, item := range items {
		redacted[index] = redactSSHHistory(item)
	}
	return redacted
}

// redactAnalyticsRecord 脱敏网站访问日志中的 Referer、User-Agent 和 URI 查询参数。
func redactAnalyticsRecord(item map[string]any) map[string]any {
	return redactLogMap(item)
}

// logRetentionPolicy 描述一种日志数据源的默认年龄和容量上限。
type logRetentionPolicy struct {
	Name       string
	Table      string
	TimeColumn string
	MaxAge     time.Duration
	MaxRows    int
}

// defaultLogRetentionPolicies 返回正式环境默认日志保留策略；文件型日志由外部轮转器执行。
func defaultLogRetentionPolicies() []logRetentionPolicy {
	return []logRetentionPolicy{
		{Name: "operation", Table: "operation_logs", TimeColumn: "created_at", MaxAge: 180 * 24 * time.Hour, MaxRows: 100000},
		{Name: "login", Table: "login_logs", TimeColumn: "created_at", MaxAge: 180 * 24 * time.Hour, MaxRows: 50000},
		{Name: "app_task", Table: "app_install_tasks", TimeColumn: "created_at", MaxAge: 90 * 24 * time.Hour, MaxRows: 100000},
		{Name: "runtime_task", Table: "runtime_tasks", TimeColumn: "created_at", MaxAge: 90 * 24 * time.Hour, MaxRows: 100000},
		{Name: "runtime_task_log", Table: "runtime_task_logs", TimeColumn: "created_at", MaxAge: 90 * 24 * time.Hour, MaxRows: 500000},
	}
}

// pruneRetainedSQLiteLogs 清理过期和超容量日志；调用方应在低峰期、备份后执行。
func pruneRetainedSQLiteLogs(ctx context.Context, db *sql.DB, now time.Time) (map[string]int64, error) {
	if db == nil {
		return nil, sql.ErrConnDone
	}
	repo, err := storage.NewSQLiteRepository(db)
	if err != nil {
		return nil, err
	}
	return pruneRetainedSQLiteLogsWithRepository(ctx, repo, now)
}

// pruneRetainedSQLiteLogsWithRepository 在统一事务边界内清理日志，供后台维护任务使用。
func pruneRetainedSQLiteLogsWithRepository(ctx context.Context, repo storage.Transactional, now time.Time) (map[string]int64, error) {
	if repo == nil {
		return nil, sql.ErrConnDone
	}
	policies := defaultLogRetentionPolicies()
	result := make(map[string]int64, len(policies))
	err := repo.WithTx(ctx, func(tx storage.SQLExecutor) error {
		for _, policy := range policies {
			if !sqliteTableExists(ctx, tx, policy.Table) {
				// 可选领域迁移尚未启用时跳过该表，避免清理任务因单表缺失整体失败。
				result[policy.Name] = 0
				continue
			}
			cutoff := now.Add(-policy.MaxAge).UTC().Format(time.RFC3339Nano)
			deleteOld, err := tx.ExecContext(ctx, `DELETE FROM `+policy.Table+` WHERE julianday(`+policy.TimeColumn+`) < julianday(?)`, cutoff)
			if err != nil {
				return err
			}
			removed, err := deleteOld.RowsAffected()
			if err != nil {
				return err
			}
			deleteOverflow, err := tx.ExecContext(ctx, `DELETE FROM `+policy.Table+` WHERE id IN (SELECT id FROM `+policy.Table+` ORDER BY `+policy.TimeColumn+` DESC,id DESC LIMIT -1 OFFSET ?)`, policy.MaxRows)
			if err != nil {
				return err
			}
			overflow, err := deleteOverflow.RowsAffected()
			if err != nil {
				return err
			}
			result[policy.Name] = removed + overflow
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// logRetentionInterval controls the maintenance cadence. The environment
// override is intentionally only a duration, so deployments can tune the
// schedule without changing the set of tables or retention limits.
func logRetentionInterval() time.Duration {
	if value := strings.TrimSpace(os.Getenv("WORKMESH_LOG_RETENTION_INTERVAL")); value != "" {
		if interval, err := time.ParseDuration(value); err == nil && interval > 0 {
			return interval
		}
	}
	return 24 * time.Hour
}

// runLogRetentionCycle performs one transactional SQLite cleanup and records
// the result as an operation log. It is safe to call when the shared database
// has not been initialized yet; the scheduler then simply waits for the next
// cycle.
func runLogRetentionCycle(ctx context.Context) error {
	repository, err := SharedRepository()
	if err != nil {
		if sharedDB() == nil {
			return nil
		}
		return err
	}
	if repository == nil {
		return nil
	}
	result, pruneErr := pruneRetainedSQLiteLogsWithRepository(ctx, repository, time.Now().UTC())
	auditErr := recordLogRetentionAudit(ctx, repository, result, pruneErr)
	if pruneErr != nil {
		return pruneErr
	}
	return auditErr
}

func recordLogRetentionAudit(ctx context.Context, db storage.SQLExecutor, result map[string]int64, operationErr error) error {
	if db == nil {
		return sql.ErrConnDone
	}
	status := "Success"
	message := "日志保留清理完成"
	if operationErr != nil {
		status = "Failed"
		message = "日志保留清理失败: " + operationErr.Error()
	}
	if result != nil {
		if encoded, err := json.Marshal(result); err == nil {
			message += "; removed=" + string(encoded)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `INSERT INTO operation_logs(source,path,method,status,message,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, "system", "/internal/log-retention", "maintenance", status, message, now, now)
	return err
}

// startLogRetentionScheduler runs independently from certificate renewal so
// a slow or unavailable firewall/ACME dependency cannot delay log cleanup.
func startLogRetentionScheduler(ctx context.Context) {
	ticker := time.NewTicker(logRetentionInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = runLogRetentionCycle(ctx)
		}
	}
}

// sqliteTableExists 判断迁移表是否已存在；表名来自固定策略清单，不接收外部输入。
func sqliteTableExists(ctx context.Context, tx storage.SQLExecutor, table string) bool {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count)
	return err == nil && count > 0
}

// retentionPolicyNames 返回排序后的策略名称，供状态页和单测稳定展示。
func retentionPolicyNames() []string {
	policies := defaultLogRetentionPolicies()
	names := make([]string, 0, len(policies))
	for _, policy := range policies {
		names = append(names, policy.Name)
	}
	sort.Strings(names)
	return names
}

// retentionPolicySummary 返回不含敏感运行数据的默认保留摘要。
func retentionPolicySummary() []map[string]any {
	policies := defaultLogRetentionPolicies()
	items := make([]map[string]any, 0, len(policies)+2)
	for _, policy := range policies {
		items = append(items, map[string]any{"name": policy.Name, "source": "sqlite", "maxAgeDays": int(policy.MaxAge / (24 * time.Hour)), "maxRows": policy.MaxRows})
	}
	items = append(items,
		map[string]any{"name": "system", "source": "journalctl-or-file", "maxAgeDays": 30, "maxBytes": 2 << 30},
		map[string]any{"name": "website-and-ssh", "source": "openresty-and-host-files", "maxAgeDays": 30, "maxBytes": 2 << 30},
	)
	return items
}
