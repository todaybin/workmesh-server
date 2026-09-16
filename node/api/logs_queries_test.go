// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// TestLoginLogContractUsesPersistedAddress 验证登录日志返回前端要求的 address 字段。
func TestLoginLogContractUsesPersistedAddress(t *testing.T) {
	store := openLogsTestStore(t)
	db := store.DB()
	if _, err := db.Exec(`INSERT INTO login_logs(ip,user,address,agent,status,message,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, "192.0.2.20", "admin", "北京", "browser", "success", "登录成功", "2026-09-05T01:02:03Z", "2026-09-05T01:02:03Z"); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerLogRoutes(mux, getDomainStore())
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/core/logs/login", bytes.NewBufferString(`{"info":"admin","status":"Success","page":1,"pageSize":20}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("login log status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Items) != 1 || payload.Data.Items[0]["address"] != "北京" {
		t.Fatalf("login contract=%#v", payload.Data.Items)
	}
}

// TestLogSearchUsesSQLiteFilteredPages 验证审计日志搜索在 SQLite 中筛选并限制单页返回量。
func TestLogSearchUsesSQLiteFilteredPages(t *testing.T) {
	store := openLogsTestStore(t)
	db := store.DB()
	base := time.Date(2026, time.September, 5, 0, 0, 0, 0, time.UTC)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 120; index++ {
		created := base.Add(time.Duration(index) * time.Minute).Format(time.RFC3339)
		if _, err := tx.Exec(`INSERT INTO login_logs(ip,user,address,agent,status,message,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, "192.0.2.20", "admin", "杭州", "browser", "Success", fmt.Sprintf("login-%03d", index), created, created); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO operation_logs(source,node,path,method,status,message,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, "websites", "local", "/api/v2/websites/search", "post", "Success", fmt.Sprintf("operation-%03d", index), created, created); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerLogRoutes(mux, getDomainStore())
	login := callLogSearch(mux, "/api/v2/core/logs/login", `{"info":"admin","status":"Success","page":2,"pageSize":7}`)
	if login.Total != 120 || len(login.Items) != 7 || login.Items[0]["message"] != "login-112" {
		t.Fatalf("login SQL page=%#v", login)
	}
	operation := callLogSearch(mux, "/api/v2/core/logs/operation", `{"source":"websites","status":"Success","page":3,"pageSize":5}`)
	if operation.Total != 120 || len(operation.Items) != 5 || operation.Items[0]["message"] != "operation-109" {
		t.Fatalf("operation SQL page=%#v", operation)
	}
}

// TestTaskLogSearchUsesSQLitePages 验证公共任务与运行时任务日志均在 SQLite 中按页读取。
func TestTaskLogSearchUsesSQLitePages(t *testing.T) {
	store := openLogsTestStore(t)
	db := store.DB()
	if _, err := db.Exec(`INSERT INTO app_installs(id,app_key,name,status,created_at,updated_at) VALUES(?,?,?,?,?,?)`, "install-pages", "demo", "Paged App", "running", "2026-09-05T00:00:00Z", "2026-09-05T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, time.September, 5, 0, 0, 0, 0, time.UTC)
	for index := 0; index < 120; index++ {
		created := base.Add(time.Duration(index) * time.Minute).Format(time.RFC3339)
		if _, err := tx.Exec(`INSERT INTO app_install_tasks(id,app_install_id,status,step,progress,message,error,log_path,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, fmt.Sprintf("task-%03d", index), "install-pages", "running", "starting", index, "", "", "", created, created); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO runtime_tasks(id,runtime_id,status,step,progress,message,error,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, "runtime-page", "runtime-1", "running", "starting", 10, "", "", base.Format(time.RFC3339), base.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 130; index++ {
		if _, err := tx.Exec(`INSERT INTO runtime_task_logs(task_id,line,created_at) VALUES(?,?,?)`, "runtime-page", fmt.Sprintf("line-%03d", index), base.Add(time.Duration(index)*time.Second).Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerLogRoutes(mux, getDomainStore())
	tasks := callTaskSearch(mux, `{"type":"app","status":"Success","page":4,"pageSize":9}`)
	if tasks.Total != 120 || len(tasks.Items) != 9 || tasks.Items[0].ID != "task-092" {
		t.Fatalf("task SQL page=%#v", tasks)
	}
	read := httptest.NewRecorder()
	mux.ServeHTTP(read, httptest.NewRequest(http.MethodPost, "/api/v2/logs/tasks/read", bytes.NewBufferString(`{"taskID":"runtime-page","page":3,"pageSize":11}`)))
	var payload struct {
		Data struct {
			Lines      []string `json:"lines"`
			TotalLines int      `json:"totalLines"`
		} `json:"data"`
	}
	if read.Code != http.StatusOK || json.Unmarshal(read.Body.Bytes(), &payload) != nil || payload.Data.TotalLines != 130 || len(payload.Data.Lines) != 11 || payload.Data.Lines[0] != "line-022" {
		t.Fatalf("runtime task SQL page status=%d payload=%s", read.Code, read.Body.String())
	}
}

type logSearchResponse struct {
	Total int              `json:"total"`
	Items []map[string]any `json:"items"`
}

// callLogSearch 执行审计日志 HTTP 查询并提取标准分页数据。
func callLogSearch(mux *http.ServeMux, path, body string) logSearchResponse {
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body)))
	var payload struct {
		Data logSearchResponse `json:"data"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &payload)
	return payload.Data
}

// callTaskSearch 执行任务日志 HTTP 查询并提取标准分页数据。
func callTaskSearch(mux *http.ServeMux, body string) struct {
	Total int             `json:"total"`
	Items []taskLogRecord `json:"items"`
} {
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/logs/tasks/search", bytes.NewBufferString(body)))
	var payload struct {
		Data struct {
			Total int             `json:"total"`
			Items []taskLogRecord `json:"items"`
		} `json:"data"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &payload)
	return payload.Data
}

// TestTaskSearchContractReadsSQLiteRows 验证任务列表读取公共 SQLite 任务表而不是内存日志快照。
func TestTaskSearchContractReadsSQLiteRows(t *testing.T) {
	store := openLogsTestStore(t)
	db := store.DB()
	if _, err := db.Exec(`INSERT INTO app_installs(id,app_key,name,status,created_at,updated_at) VALUES(?,?,?,?,?,?)`, "install-1", "demo", "Demo App", "running", "2026-09-05T01:00:00Z", "2026-09-05T01:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app_install_tasks(id,app_install_id,status,step,progress,message,error,log_path,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, "task-1", "install-1", "running", "starting", 85, "启动完成", "", filepath.Join(t.TempDir(), "task.log"), "2026-09-05T01:00:00Z", "2026-09-05T01:01:00Z"); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerLogRoutes(mux, getDomainStore())
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/logs/tasks/search", bytes.NewBufferString(`{"type":"app","status":"Success","page":1,"pageSize":20}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("task log status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Data struct {
			Items []taskLogRecord `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Items) != 1 || payload.Data.Items[0].Name != "Demo App" || payload.Data.Items[0].ProgressPercent != 85 {
		t.Fatalf("task contract=%#v", payload.Data.Items)
	}
}

// TestLogDetailReadsFreshSQLiteRecord 验证日志详情可读取刚写入数据库而无需重启或刷新内存状态。
func TestLogDetailReadsFreshSQLiteRecord(t *testing.T) {
	store := openLogsTestStore(t)
	if _, err := store.DB().Exec(`INSERT INTO operation_logs(source,path,method,status,message,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, "files", "/api/v2/files/list", "post", "Success", "fresh", "2026-09-05T01:02:03Z", "2026-09-05T01:02:03Z"); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerLogRoutes(mux, getDomainStore())
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/logs/detail", bytes.NewBufferString(`{"id":"1","type":"operation"}`)))
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte(`"message":"fresh"`)) {
		t.Fatalf("log detail status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestLogClearUsesSQLiteTransactionAndValidatesType(t *testing.T) {
	store := openLogsTestStore(t)
	now := "2026-09-05T01:02:03Z"
	if _, err := store.DB().Exec(`INSERT INTO operation_logs(message,created_at,updated_at) VALUES(?,?,?)`, "operation", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().Exec(`INSERT INTO login_logs(message,created_at,updated_at) VALUES(?,?,?)`, "login", now, now); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerLogRoutes(mux, getDomainStore())
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/core/logs/clean", bytes.NewBufferString(`{"logType":"operation"}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("clear status=%d body=%s", res.Code, res.Body.String())
	}
	var operations, logins int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM operation_logs`).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM login_logs`).Scan(&logins); err != nil {
		t.Fatal(err)
	}
	if operations != 0 || logins != 1 {
		t.Fatalf("unexpected counts operation=%d login=%d", operations, logins)
	}
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/core/logs/clean", bytes.NewBufferString(`{"logType":"website"}`)))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("invalid type status=%d body=%s", res.Code, res.Body.String())
	}
}

// openLogsTestStore 创建日志测试使用的共享 SQLite，并在测试结束时释放包级连接。
func openLogsTestStore(t *testing.T) *storage.Store {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	functionalStoreMu.Lock()
	functionalStoreInstance = nil
	functionalStoreMu.Unlock()
	getDomainStore()
	t.Cleanup(func() {
		resetSharedStoreForTest()
		functionalStoreMu.Lock()
		functionalStoreInstance = nil
		functionalStoreMu.Unlock()
		_ = store.Close()
	})
	return store
}
