// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// domainState 是备份、告警、日志和设置共用的轻量持久化状态。
// 生产环境保存到共享 SQLite，无数据库测试使用原子写入的兼容文件。
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

// domainStore 用读写锁保护功能域缓存；写入和保存必须在同一写锁区间内完成。
type domainStore struct {
	mu    sync.RWMutex
	path  string
	state domainState
}

// cloneDomainState creates an isolated rollback snapshot for compound settings
// and alert updates, including nested maps and slices.
func cloneDomainState(source domainState) domainState {
	payload, err := json.Marshal(source)
	if err != nil {
		return source
	}
	var clone domainState
	if err := json.Unmarshal(payload, &clone); err != nil {
		return source
	}
	return clone
}

// backupItem 保存备份产物及其任务、账号关联，不承载备份账号凭据。
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
	SourceAccountIDs  string    `json:"sourceAccountIDs,omitempty"`
	CronjobID         string    `json:"cronjobID,omitempty"`
	TaskID            string    `json:"taskID,omitempty"`
	Size              int64     `json:"size"`
	Description       string    `json:"description,omitempty"`
	Status            string    `json:"status"`
	Message           string    `json:"message,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
}

// backupAccount 对应旧系统 BackupAccount，敏感凭据只保存在内存和受保护持久化中。
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

// configuredBackupProvider 从账号配置构造云端 Provider；所有云端写操作必须经过此入口。
func configuredBackupProvider(account backupAccount) (*service.HTTPBackupProvider, map[string]any, error) {
	vars := map[string]any{}
	if strings.TrimSpace(account.Vars) != "" {
		if err := json.Unmarshal([]byte(account.Vars), &vars); err != nil {
			return nil, nil, fmt.Errorf("备份账号 Vars 无效: %w", err)
		}
	}
	provider, err := service.NewHTTPBackupProvider(account.Type, vars, account.AccessKey, account.Credential)
	if err != nil {
		return nil, vars, err
	}
	return provider, vars, nil
}

// alertItem 保存告警开关、阈值和类型专属配置，供告警接口恢复用户设置。
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

// logItem 兼容操作、登录等日志字段；对外序列化由 MarshalJSON 处理操作日志差异。
type logItem struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Source    string         `json:"source"`
	User      string         `json:"user"`
	Node      string         `json:"node"`
	IP        string         `json:"ip"`
	Address   string         `json:"address"`
	Path      string         `json:"path"`
	Method    string         `json:"method"`
	UserAgent string         `json:"userAgent"`
	Latency   int64          `json:"latency"`
	Status    string         `json:"status"`
	DetailZH  string         `json:"detailZH"`
	DetailEN  string         `json:"detailEN"`
	Meta      map[string]any `json:"meta,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

// MarshalJSON 保持操作日志 ID 与旧版 API 的数字语义，同时不影响内部字符串任务 ID。
func (item logItem) MarshalJSON() ([]byte, error) {
	type plainLogItem logItem
	payload, err := json.Marshal(plainLogItem(item))
	if err != nil {
		return nil, err
	}
	if item.Type != "operation" {
		return payload, nil
	}
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil {
		return nil, err
	}
	if id, err := strconv.ParseInt(item.ID, 10, 64); err == nil {
		object["id"] = id
	}
	delete(object, "type")
	delete(object, "level")
	delete(object, "meta")
	return json.Marshal(object)
}

// normalizeOperationPath 移除统一 API 前缀并返回以斜杠开头的审计路径。
func normalizeOperationPath(path string) string {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "/api/v2/core") {
		path = strings.TrimPrefix(path, "/api/v2/core")
	} else {
		path = strings.TrimPrefix(path, "/api/v2")
	}
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

// operationSource 从规范化路径推导操作日志所属的功能域。
func operationSource(path string) string {
	clean := strings.TrimPrefix(normalizeOperationPath(path), "/")
	if clean == "" {
		return "server"
	}
	parts := strings.Split(clean, "/")
	if parts[0] == "core" && len(parts) > 1 {
		return parts[1]
	}
	return parts[0]
}

// operationClientIP 从请求远端地址中提取不带端口的客户端 IP。
func operationClientIP(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return strings.Trim(remoteAddr, "[]")
}

// operationStatus 将 failed/error 归为失败，保留空值，其余历史非空状态归为成功。
func operationStatus(status string) string {
	if strings.EqualFold(strings.TrimSpace(status), "failed") || strings.EqualFold(strings.TrimSpace(status), "error") {
		return "Failed"
	}
	if strings.TrimSpace(status) == "" {
		return ""
	}
	return "Success"
}

// operationDetails 补齐操作日志缺失的中英文详情，同时保留调用方提供的文案。
func operationDetails(method, path, detailZH, detailEN string) (string, string) {
	method = strings.ToLower(strings.TrimSpace(method))
	path = normalizeOperationPath(path)
	if strings.TrimSpace(detailZH) == "" {
		detailZH = fmt.Sprintf("%s %s", strings.ToUpper(method), path)
	}
	if strings.TrimSpace(detailEN) == "" {
		detailEN = fmt.Sprintf("%s %s", strings.ToUpper(method), path)
	}
	return detailZH, detailEN
}

// RecordOperationLog 将统一 HTTP 链路的写请求审计信息写入 SQLite，无共享库时跳过。
// 不保存原始请求体；响应 message 和详情头仍会入库，调用方必须避免其中包含凭据。
func RecordOperationLog(r *http.Request, status int, response []byte, latency time.Duration) {
	if r == nil {
		return
	}
	repository, err := SharedRepository()
	if err != nil {
		return
	}
	resultStatus := "Success"
	message := ""
	var envelope struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
	}
	if len(response) > 0 {
		_ = json.Unmarshal(response, &envelope)
		message = strings.TrimSpace(envelope.Message)
	}
	if status >= http.StatusBadRequest {
		resultStatus = "Failed"
	}
	switch code := envelope.Code.(type) {
	case string:
		if strings.EqualFold(code, "ERR") {
			resultStatus = "Failed"
		}
	case float64:
		if code != 200 {
			resultStatus = "Failed"
		}
	}
	user := ""
	sessionID := coreSessionID(r)
	if cookie, err := r.Cookie("workmesh_session"); err == nil {
		sessionID = strings.TrimSpace(cookie.Value)
	}
	if sessionID != "" {
		if account, currentErr := localCore.Current(sessionID); currentErr == nil {
			user = account.Name
		}
	}
	node := strings.TrimSpace(r.Header.Get("CurrentNode"))
	if decoded, err := url.QueryUnescape(node); err == nil {
		node = strings.TrimSpace(decoded)
	}
	if node == "" {
		node = "local"
	}
	detailZH := strings.TrimSpace(r.Header.Get("X-Operation-Detail-ZH"))
	detailEN := strings.TrimSpace(r.Header.Get("X-Operation-Detail-EN"))
	path := normalizeOperationPath(r.URL.Path)
	method := strings.ToLower(strings.TrimSpace(r.Method))
	detailZH, detailEN = operationDetails(method, path, detailZH, detailEN)
	writer, err := storage.NewSQLiteAuditLogWriter(repository)
	if err != nil {
		return
	}
	_ = writer.RecordOperation(r.Context(), storage.OperationAuditEntry{
		Source: operationSource(r.URL.Path), User: user, IP: operationClientIP(r.RemoteAddr), Node: node,
		Path: path, Method: method, UserAgent: r.UserAgent(), LatencyNsec: latency.Nanoseconds(),
		Status: resultStatus, Message: message, DetailZH: detailZH, DetailEN: detailEN, CreatedAt: time.Now().UTC(),
	})
}

var functionalStoreMu sync.Mutex
var functionalStoreInstance *domainStore

// getDomainStore 在全局锁内按数据目录复用仓库，优先恢复 SQLite，再尝试旧文件导入。
// 最多合并最近 1000 条操作日志；旧文件仅在导入保存成功后尝试归档。
func getDomainStore() *domainStore {
	dataDir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dataDir == "" {
		dataDir = "./data"
	}
	path := filepath.Join(dataDir, "domains.json")
	functionalStoreMu.Lock()
	defer functionalStoreMu.Unlock()
	if functionalStoreInstance != nil && functionalStoreInstance.path == path {
		return functionalStoreInstance
	}
	s := &domainStore{path: path, state: domainState{Settings: map[string]any{"language": "zh", "theme": "system"}}}
	var persistedOperationLogs []logItem
	if repository, repositoryErr := SharedRepository(); repositoryErr == nil {
		if rows, queryErr := repository.Query(`SELECT id,source,user,ip,node,path,method,user_agent,latency,status,message,detail_zh,detail_en,created_at FROM operation_logs ORDER BY id DESC LIMIT 1000`); queryErr == nil {
			for rows.Next() {
				var id int64
				var source, user, ip, node, path, method, userAgent, status, message, detailZH, detailEN, created string
				var latency int64
				if rows.Scan(&id, &source, &user, &ip, &node, &path, &method, &userAgent, &latency, &status, &message, &detailZH, &detailEN, &created) == nil {
					t, _ := time.Parse(time.RFC3339Nano, created)
					path = normalizeOperationPath(path)
					method = strings.ToLower(strings.TrimSpace(method))
					if source == "" || strings.EqualFold(source, "server") {
						source = operationSource(path)
					}
					ip = operationClientIP(ip)
					if node == "" {
						node = "local"
					}
					status = operationStatus(status)
					detailZH, detailEN = operationDetails(method, path, detailZH, detailEN)
					persistedOperationLogs = append(persistedOperationLogs, logItem{ID: strconv.FormatInt(id, 10), Type: "operation", Level: status, Status: status, Source: source, User: user, IP: ip, Node: node, Path: path, Method: method, UserAgent: userAgent, Latency: latency, Message: message, DetailZH: detailZH, DetailEN: detailEN, Meta: map[string]any{"method": method, "path": path}, CreatedAt: t})
				}
			}
			rows.Close()
		}
	}
	if db := sharedDB(); db != nil {
		if !loadJSONState("functional_domain_state", &s.state) {
			if content, err := os.ReadFile(path); err == nil && len(content) > 0 && json.Unmarshal(content, &s.state) == nil {
				if saveErr := saveJSONState("functional_domain_state", s.state); saveErr == nil {
					archiveDir := filepath.Join(filepath.Dir(path), "backups")
					if os.MkdirAll(archiveDir, 0o750) == nil {
						_ = os.Rename(path, filepath.Join(archiveDir, "legacy-domains-"+time.Now().UTC().Format("20060102T150405.000000000Z")+".json"))
					}
				}
			}
		}
	} else if content, err := os.ReadFile(path); err == nil && len(content) > 0 {
		_ = json.Unmarshal(content, &s.state)
		if s.state.Settings == nil {
			s.state.Settings = map[string]any{}
		}
	}
	if len(persistedOperationLogs) > 0 {
		s.state.Logs = append(persistedOperationLogs, s.state.Logs...)
	}
	functionalStoreInstance = s
	return functionalStoreInstance
}

// saveLocked 要求调用方持有仓库写锁；有共享库时只保存 SQLite 状态。
// 无数据库的兼容路径使用同目录临时文件替换，不在此方法内重复获取仓库锁。
func (s *domainStore) saveLocked() error {
	if sharedDB() != nil {
		return saveJSONState("functional_domain_state", s.state)
	}
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
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	return nil
}

// validBackupPath 拒绝空值、NUL/换行、过长路径和清理后仍以 .. 开头的相对路径。
// 此检查允许绝对路径，不验证授权根目录或符号链接，不能替代文件访问权限校验。
func validBackupPath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\x00\r\n") || len(value) > 4096 {
		return false
	}
	clean := filepath.Clean(value)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

// formatTimeForLog 保持审计日志统一使用 UTC RFC3339Nano 文本格式。
func formatTimeForLog(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

// idToken 生成备份、告警和设置快照使用的随机字符串 ID。
func idToken() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(raw[:])
}

// success 使用统一成功 envelope 返回功能域数据。
func success(w http.ResponseWriter, data any) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}

// domainError 使用统一错误 envelope 返回功能域错误码和消息。
func domainError(w http.ResponseWriter, status int, code, message string) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": message, "details": map[string]string{"errCode": code}})
}

// requestMap 最多读取 4 MiB 请求体，兼容空体/null，再补入未被 JSON 覆盖的查询字段。
// 查询参数只取首个非空值；保留现有单次 Decode 语义，不在此检查尾随 JSON。
func requestMap(r *http.Request) (map[string]any, error) {
	v := map[string]any{}
	if r.Body != nil {
		if err := decodeSingleJSON(r.Body, &v, 4<<20); err != nil {
			if errors.Is(err, io.EOF) {
				v = map[string]any{}
			} else {
				return nil, err
			}
		}
	}
	if v == nil {
		v = map[string]any{}
	}
	for key, values := range r.URL.Query() {
		if len(values) > 0 && strings.TrimSpace(values[0]) != "" {
			if _, exists := v[key]; !exists {
				v[key] = values[0]
			}
		}
	}
	return v, nil
}

// valueString 按兼容字段顺序读取首个非空字符串值。
func valueString(v map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := v[k].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// isBackupAlertLogSettingsRoute 判断路由是否属于备份、告警、日志或设置领域。
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

// registerBackupAlertLogSettingsRoutes 注册已迁移的备份、告警、日志和系统设置路由。
func registerBackupAlertLogSettingsRoutes(mux *http.ServeMux) {
	s := getDomainStore()
	registerBackupRoutes(mux, s)
	registerAlertRoutes(mux, s)
	registerLogRoutes(mux, s)
	registerSettingsRoutes(mux, s)
}
