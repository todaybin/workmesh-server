// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestRedactLogTextCoversCredentialForms 验证常见认证头、令牌和密码格式不会原样回显。
func TestRedactLogTextCoversCredentialForms(t *testing.T) {
	input := `Authorization: Bearer abc.def; password="secret pass" cookie=session=abc; api_key=key123 Basic dXNlcjpwYXNz refresh_token=rt-1`
	output := redactLogText(input)
	for _, secret := range []string{"abc.def", "secret pass", "session=abc", "key123", "dXNlcjpwYXNz", "rt-1"} {
		if strings.Contains(output, secret) {
			t.Fatalf("secret %q leaked in %q", secret, output)
		}
	}
	if strings.Count(output, logRedactedValue) < 6 {
		t.Fatalf("expected redactions in %q", output)
	}
	structured := redactLogMap(map[string]any{"password": "plain-secret", "nested": map[string]any{"access_token": "token-secret"}})
	if structured["password"] != logRedactedValue || structured["nested"].(map[string]any)["access_token"] != logRedactedValue {
		t.Fatalf("structured secrets were not redacted: %#v", structured)
	}
}

// TestLogResponsesRedactRealSQLiteValues 验证操作、登录和任务接口从真实 SQLite 读取后再脱敏。
func TestLogResponsesRedactRealSQLiteValues(t *testing.T) {
	store := openLogsTestStore(t)
	db := store.DB()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO operation_logs(source,path,method,status,message,detail_zh,detail_en,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, "server", "/api/v2/test", "post", "Success", "Authorization: Bearer op-secret password=op-pass", "cookie=session-op", "", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO login_logs(ip,user,address,agent,status,message,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, "192.0.2.1", "admin", "test", "Cookie: sid=login-cookie", "Success", "password=login-pass", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app_installs(id,app_key,name,status,created_at,updated_at) VALUES(?,?,?,?,?,?)`, "redact-install", "demo", "Redact App", "running", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app_install_tasks(id,app_install_id,status,step,error,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, "redact-task", "redact-install", "failed", "password=task-pass", "secret=task-secret", now, now); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerLogRoutes(mux, getDomainStore())
	for path, body := range map[string]string{
		"/api/v2/core/logs/operation": `{"page":1,"pageSize":10}`,
		"/api/v2/core/logs/login":     `{"page":1,"pageSize":10}`,
		"/api/v2/logs/tasks/search":   `{"type":"app","page":1,"pageSize":10}`,
	} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body)))
		if res.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, res.Code, res.Body.String())
		}
		if strings.Contains(res.Body.String(), "op-secret") || strings.Contains(res.Body.String(), "op-pass") || strings.Contains(res.Body.String(), "login-pass") || strings.Contains(res.Body.String(), "task-secret") || strings.Contains(res.Body.String(), "task-pass") {
			t.Fatalf("%s leaked secret: %s", path, res.Body.String())
		}
	}
	detail := httptest.NewRecorder()
	mux.ServeHTTP(detail, httptest.NewRequest(http.MethodPost, "/api/v2/logs/detail", bytes.NewBufferString(`{"id":"1","type":"operation"}`)))
	if detail.Code != http.StatusOK || strings.Contains(detail.Body.String(), "op-secret") || strings.Contains(detail.Body.String(), "op-pass") {
		t.Fatalf("detail leaked secret: status=%d body=%s", detail.Code, detail.Body.String())
	}
}

// TestSystemAndSSHLogResponsesRedactSecrets 验证系统文件和 SSH 文件日志输出、导出均经过脱敏。
func TestSystemAndSSHLogResponsesRedactSecrets(t *testing.T) {
	dataDir, sshDir := t.TempDir(), t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	t.Setenv("WORKMESH_SSH_LOG_DIR", sshDir)
	logDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		t.Fatal(err)
	}
	systemPath := filepath.Join(logDir, "server.log")
	if err := os.WriteFile(systemPath, []byte("2026-09-05T00:00:00Z password=system-pass Authorization: Bearer system-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sshPath := filepath.Join(sshDir, "auth.log")
	if err := os.WriteFile(sshPath, []byte("Jan  2 03:04:05 host sshd[1]: Accepted password for admin from 192.0.2.9 port 22 password=ssh-pass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerLogRoutes(mux, getDomainStore())
	registerSSHLogRoutes(mux)
	read := httptest.NewRecorder()
	mux.ServeHTTP(read, httptest.NewRequest(http.MethodPost, "/api/v2/logs/system/read", bytes.NewBufferString(`{"path":"`+systemPath+`"}`)))
	if read.Code != http.StatusOK || strings.Contains(read.Body.String(), "system-pass") || strings.Contains(read.Body.String(), "system-token") {
		t.Fatalf("system log leaked: status=%d body=%s", read.Code, read.Body.String())
	}
	export := httptest.NewRecorder()
	mux.ServeHTTP(export, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/ssh/log/export", bytes.NewBufferString(`{}`)))
	if export.Code != http.StatusOK {
		t.Fatalf("ssh export status=%d body=%s", export.Code, export.Body.String())
	}
	var envelope struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(export.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(envelope.Data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "ssh-pass") {
		t.Fatalf("ssh export leaked secret: %s", b)
	}
}

// TestPruneRetainedSQLiteLogsUsesAgeAndRowBounds 验证保留清理只删除过期记录并保留近期真实数据。
func TestPruneRetainedSQLiteLogsUsesAgeAndRowBounds(t *testing.T) {
	store := openLogsTestStore(t)
	db := store.DB()
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	fresh := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO operation_logs(message,created_at,updated_at) VALUES(?,?,?)`, "old password=gone", old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO operation_logs(message,created_at,updated_at) VALUES(?,?,?)`, "fresh password=kept", fresh, fresh); err != nil {
		t.Fatal(err)
	}
	removed, err := pruneRetainedSQLiteLogs(context.Background(), db, time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC))
	if err != nil || removed["operation"] != 1 {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM operation_logs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("fresh operation row was removed, count=%d", count)
	}
	if names := retentionPolicyNames(); len(names) != 5 || names[0] != "app_task" {
		t.Fatalf("policy names=%v", names)
	}
}

// TestRedactionCoversWebsiteContentAndNestedCollections 验证网站日志正文、路径和嵌套集合均不会泄漏凭据。
func TestRedactionCoversWebsiteContentAndNestedCollections(t *testing.T) {
	website := redactWebsiteLogResult(map[string]any{
		"path":    "/srv/site/access.log",
		"content": "GET /download?access_token=url-secret HTTP/1.1\nAuthorization: Bearer header-secret",
		"meta": map[string]any{
			"headers": map[string]string{"Cookie": "sid=cookie-secret"},
			"tokens":  []string{"refresh_token=list-secret"},
		},
	})
	raw, _ := json.Marshal(website)
	for _, secret := range []string{"url-secret", "header-secret", "cookie-secret", "list-secret"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatalf("website log secret %q leaked: %s", secret, raw)
		}
	}
	if website["path"] != "/srv/site/access.log" {
		t.Fatalf("website path changed: %#v", website["path"])
	}
	item := redactLogItem(logItem{Path: "/api/v2/files/get?token=path-secret"})
	if strings.Contains(item.Path, "path-secret") {
		t.Fatalf("operation path leaked: %q direct=%q", item.Path, redactLogText(item.Path))
	}
}

// TestPruneRetainedSQLiteLogsSkipsUninstalledDomains 验证仅安装部分日志迁移时清理仍可提交事务。
func TestPruneRetainedSQLiteLogsSkipsUninstalledDomains(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE operation_logs (id INTEGER PRIMARY KEY AUTOINCREMENT, created_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO operation_logs(created_at) VALUES(?)`, "2020-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	removed, err := pruneRetainedSQLiteLogs(context.Background(), db, time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC))
	if err != nil || removed["operation"] != 1 {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	for _, name := range []string{"login", "app_task", "runtime_task", "runtime_task_log"} {
		if removed[name] != 0 {
			t.Fatalf("missing table %s reported removal=%d", name, removed[name])
		}
	}
}

func TestRunLogRetentionCycleRecordsAuditAndIsIdempotent(t *testing.T) {
	store := openLogsTestStore(t)
	old := time.Now().UTC().Add(-365 * 24 * time.Hour).Format(time.RFC3339Nano)
	if _, err := store.DB().Exec(`INSERT INTO operation_logs(source,path,method,status,message,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, "test", "/old", "post", "Success", "old", old, old); err != nil {
		t.Fatal(err)
	}
	if err := runLogRetentionCycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	var oldCount, auditCount int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM operation_logs WHERE path='/old'`).Scan(&oldCount); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM operation_logs WHERE path='/internal/log-retention' AND status='Success'`).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if oldCount != 0 || auditCount != 1 {
		t.Fatalf("oldCount=%d auditCount=%d", oldCount, auditCount)
	}
	if err := runLogRetentionCycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM operation_logs WHERE path='/internal/log-retention' AND status='Success'`).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 {
		t.Fatalf("second cycle should be auditable, count=%d", auditCount)
	}
}
