// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupCloudEndpointsDoNotFakeSuccess(t *testing.T) {
	dir := filepath.Join(".tmp", "backup-cloud-test")
	_ = os.RemoveAll(dir)
	t.Setenv("WORKMESH_DATA_DIR", dir)
	mux := http.NewServeMux()
	registerBackupAlertLogSettingsRoutes(mux)
	buckets := httptest.NewRecorder()
	mux.ServeHTTP(buckets, httptest.NewRequest(http.MethodPost, "/api/v2/backups/buckets", bytes.NewBufferString(`{"type":"s3"}`)))
	if buckets.Code != http.StatusServiceUnavailable || !bytes.Contains(buckets.Body.Bytes(), []byte("BACKUP_PROVIDER_UNAVAILABLE")) {
		t.Fatalf("cloud buckets=%d %s", buckets.Code, buckets.Body.String())
	}
	created := httptest.NewRecorder()
	mux.ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/api/v2/backups", bytes.NewBufferString(`{"name":"cloud","type":"onedrive","vars":"{}"}`)))
	var envelope struct {
		Data backupAccount `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil || envelope.Data.ID == "" {
		t.Fatalf("create account: %v %s", err, created.Body.String())
	}
	refresh := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{"id": envelope.Data.ID, "refreshToken": "secret"})
	mux.ServeHTTP(refresh, httptest.NewRequest(http.MethodPost, "/api/v2/backups/refresh/token", bytes.NewReader(body)))
	if refresh.Code != http.StatusServiceUnavailable || !bytes.Contains(refresh.Body.Bytes(), []byte("TOKEN_REFRESH_UNAVAILABLE")) {
		t.Fatalf("refresh=%d %s", refresh.Code, refresh.Body.String())
	}
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.FormValue("grant_type") != "refresh_token" || r.FormValue("refresh_token") != "rt" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt2","expires_in":3600}`))
	}))
	defer oauth.Close()
	configured := httptest.NewRecorder()
	varsBody, _ := json.Marshal(map[string]any{"name": "cloud-oauth", "type": "onedrive", "vars": `{"refresh_url":"` + oauth.URL + `","refresh_token":"rt"}`})
	mux.ServeHTTP(configured, httptest.NewRequest(http.MethodPost, "/api/v2/backups", bytes.NewReader(varsBody)))
	var configuredEnvelope struct {
		Data backupAccount `json:"data"`
	}
	if err := json.Unmarshal(configured.Body.Bytes(), &configuredEnvelope); err != nil || configuredEnvelope.Data.ID == "" {
		t.Fatalf("configured account: %v %s", err, configured.Body.String())
	}
	okRefresh := httptest.NewRecorder()
	okBody, _ := json.Marshal(map[string]any{"id": configuredEnvelope.Data.ID})
	mux.ServeHTTP(okRefresh, httptest.NewRequest(http.MethodPost, "/api/v2/backups/refresh/token", bytes.NewReader(okBody)))
	if okRefresh.Code != http.StatusOK || !bytes.Contains(okRefresh.Body.Bytes(), []byte(`"success"`)) {
		t.Fatalf("oauth refresh=%d %s", okRefresh.Code, okRefresh.Body.String())
	}
}

func TestFunctionalBackupAlertSettings(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	mux := http.NewServeMux()
	registerBackupAlertLogSettingsRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/backups/backup", bytes.NewBufferString(`{"name":"daily"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("backup status = %d", res.Code)
	}
	var envelope struct {
		Code int        `json:"code"`
		Data backupItem `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 200 || envelope.Data.Name != "daily" || envelope.Data.ID == "" {
		t.Fatalf("unexpected backup: %#v", envelope)
	}

	alertReq := httptest.NewRequest(http.MethodPost, "/api/v2/alert/update", bytes.NewBufferString(`{"type":"cpu","name":"CPU","enabled":true}`))
	alertReq.Header.Set("Content-Type", "application/json")
	alertRes := httptest.NewRecorder()
	mux.ServeHTTP(alertRes, alertReq)
	if alertRes.Code != http.StatusOK {
		t.Fatalf("alert status = %d", alertRes.Code)
	}

	settingReq := httptest.NewRequest(http.MethodPost, "/api/v2/config/global", bytes.NewBufferString(`{"language":"en"}`))
	settingRes := httptest.NewRecorder()
	mux.ServeHTTP(settingRes, settingReq)
	if settingRes.Code != http.StatusOK {
		t.Fatalf("settings status = %d", settingRes.Code)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "domains.json")); err != nil {
		t.Fatalf("state file missing: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodPost, "/api/v2/backups/search", bytes.NewBufferString(`{}`))
	listRes := httptest.NewRecorder()
	mux.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list status = %d", listRes.Code)
	}
	var list struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listRes.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if list.Data.Total != 1 {
		t.Fatalf("backup total = %d, want 1", list.Data.Total)
	}
}

func TestSettingsOperationalEndpointsReturnPersistedState(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerBackupAlertLogSettingsRoutes(mux)
	for _, path := range []string{"/api/v2/core/settings/menu/default", "/api/v2/core/settings/terminal/search", "/api/v2/core/settings/ssl/download", "/api/v2/core/settings/ssl/reload"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))
		if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"path"`) {
			t.Fatalf("settings endpoint %s failed: %d %s", path, res.Code, res.Body.String())
		}
	}
}

func TestFunctionalLogValidation(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerBackupAlertLogSettingsRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/logs/system/read", bytes.NewBufferString(`{}`))
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	if body["code"] != "ERR" {
		t.Fatalf("unexpected envelope: %#v", body)
	}
}

func TestAlertDiskAndClamDiscovery(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerBackupAlertLogSettingsRoutes(mux)
	for _, path := range []string{"/api/v2/alert/disks/list", "/api/v2/alert/clams/list"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, res.Code)
		}
		var envelope struct {
			Code int              `json:"code"`
			Data []map[string]any `json:"data"`
		}
		if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Code != 200 || envelope.Data == nil {
			t.Fatalf("%s returned invalid data: %#v", path, envelope)
		}
	}
}

func TestAlertConfigSearchAndCronListAreDynamic(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerBackupAlertLogSettingsRoutes(mux)
	update := httptest.NewRequest(http.MethodPost, "/api/v2/alert/config/update", bytes.NewBufferString(`{"name":"mail"}`))
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, update)
	if res.Code != http.StatusOK {
		t.Fatalf("config update status = %d", res.Code)
	}
	search := httptest.NewRecorder()
	mux.ServeHTTP(search, httptest.NewRequest(http.MethodPost, "/api/v2/alert/config/search", bytes.NewBufferString(`{"keyword":"mail"}`)))
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), "mail") {
		t.Fatalf("config search did not return persisted config: %s", search.Body.String())
	}
	cron := httptest.NewRecorder()
	mux.ServeHTTP(cron, httptest.NewRequest(http.MethodPost, "/api/v2/alert/cronjob/list", nil))
	if cron.Code != http.StatusOK || !strings.Contains(cron.Body.String(), `"code":200`) {
		t.Fatalf("cron list failed: %s", cron.Body.String())
	}
}

// TestBackupAccountAndRecordLifecycle 覆盖账号、记录、上传和恢复的端到端最小闭环。
func TestBackupAccountAndRecordLifecycle(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	mux := http.NewServeMux()
	registerBackupAlertLogSettingsRoutes(mux)
	post := func(path, payload string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(res, req)
		return res
	}
	created := post("/api/v2/backups", `{"name":"archive","type":"s3","vars":"{}","accessKey":"key","credential":"secret"}`)
	if created.Code != http.StatusOK {
		t.Fatalf("create account status=%d body=%s", created.Code, created.Body.String())
	}
	var accountEnvelope struct {
		Data backupAccount `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &accountEnvelope); err != nil || accountEnvelope.Data.ID == "" {
		t.Fatalf("account response=%s err=%v", created.Body.String(), err)
	}
	if accountEnvelope.Data.AccessKey != "" || accountEnvelope.Data.Credential != "" {
		t.Fatalf("敏感字段未脱敏: %#v", accountEnvelope.Data)
	}

	source := filepath.Join(dataDir, "source.txt")
	if err := os.WriteFile(source, []byte("backup-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	recordBody, _ := json.Marshal(map[string]any{"type": "website", "name": "site", "source": source, "downloadAccountID": accountEnvelope.Data.ID})
	record := post("/api/v2/backups/backup", string(recordBody))
	if record.Code != http.StatusOK {
		t.Fatalf("create record status=%d body=%s", record.Code, record.Body.String())
	}
	var recordEnvelope struct {
		Data backupItem `json:"data"`
	}
	if err := json.Unmarshal(record.Body.Bytes(), &recordEnvelope); err != nil || recordEnvelope.Data.ID == "" || recordEnvelope.Data.Size != int64(len("backup-content")) {
		t.Fatalf("record response=%s err=%v", record.Body.String(), err)
	}

	search := post("/api/v2/backups/record/search", `{"type":"website","page":1,"pageSize":20}`)
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), recordEnvelope.Data.ID) {
		t.Fatalf("record search=%d body=%s", search.Code, search.Body.String())
	}
	sizePayload, _ := json.Marshal(map[string]any{"id": recordEnvelope.Data.ID})
	sizes := post("/api/v2/backups/record/size", string(sizePayload))
	if sizes.Code != http.StatusOK || !strings.Contains(sizes.Body.String(), `"size":14`) {
		t.Fatalf("record size=%d body=%s", sizes.Code, sizes.Body.String())
	}

	target := filepath.Join(dataDir, "restored.txt")
	recoverPayload, _ := json.Marshal(map[string]any{"backupRecordID": recordEnvelope.Data.ID, "target": target})
	recovered := post("/api/v2/backups/recover", string(recoverPayload))
	if recovered.Code != http.StatusOK {
		t.Fatalf("recover status=%d body=%s", recovered.Code, recovered.Body.String())
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "backup-content" {
		t.Fatalf("recovered content=%q err=%v", got, err)
	}

	updatePayload, _ := json.Marshal(map[string]any{"id": accountEnvelope.Data.ID, "name": "archive-updated"})
	updated := post("/api/v2/backups/update", string(updatePayload))
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), "archive-updated") {
		t.Fatalf("update account=%d body=%s", updated.Code, updated.Body.String())
	}
	deletePayload, _ := json.Marshal(map[string]any{"ids": []string{recordEnvelope.Data.ID}})
	deleted := post("/api/v2/backups/record/del", string(deletePayload))
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete record=%d body=%s", deleted.Code, deleted.Body.String())
	}
	if deletedAccount := post("/api/v2/core/backups/del", `{"name":"archive-updated"}`); deletedAccount.Code != http.StatusOK {
		t.Fatalf("delete account=%d body=%s", deletedAccount.Code, deletedAccount.Body.String())
	}
}

func TestBackupUploadAndConnectionChecks(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	mux := http.NewServeMux()
	registerBackupAlertLogSettingsRoutes(mux)
	source := filepath.Join(dataDir, "upload.tar.gz")
	if err := os.WriteFile(source, []byte("uploaded"), 0o600); err != nil {
		t.Fatal(err)
	}
	uploadBody, _ := json.Marshal(map[string]string{"filePath": source, "targetDir": filepath.Join(dataDir, "incoming")})
	body := bytes.NewBuffer(uploadBody)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/backups/upload", body)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "upload.tar.gz") {
		t.Fatalf("upload=%d body=%s", res.Code, res.Body.String())
	}
	checkBody, _ := json.Marshal(map[string]string{"type": "local", "backupPath": filepath.Join(dataDir, "local")})
	check := httptest.NewRecorder()
	checkReq := httptest.NewRequest(http.MethodPost, "/api/v2/backups/conn/check", bytes.NewBuffer(checkBody))
	checkReq.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(check, checkReq)
	if check.Code != http.StatusOK || !strings.Contains(check.Body.String(), `"isOk":true`) {
		t.Fatalf("connection check=%d body=%s", check.Code, check.Body.String())
	}
}

func TestCoreSettingsExposeContractFieldsAndKeyValueUpdate(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerBackupAlertLogSettingsRoutes(mux)
	search := httptest.NewRecorder()
	mux.ServeHTTP(search, httptest.NewRequest(http.MethodPost, "/api/v2/core/settings/search/base", bytes.NewBufferString(`{}`)))
	if search.Code != http.StatusOK {
		t.Fatalf("search status = %d", search.Code)
	}
	var body struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(search.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 200 || body.Data["panelName"] == nil || body.Data["serverPort"] == nil {
		t.Fatalf("缺少基础设置字段: %#v", body.Data)
	}
	update := httptest.NewRecorder()
	mux.ServeHTTP(update, httptest.NewRequest(http.MethodPost, "/api/v2/core/settings/update", bytes.NewBufferString(`{"key":"PanelName","value":"Demo"}`)))
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d", update.Code)
	}
	verify := httptest.NewRecorder()
	mux.ServeHTTP(verify, httptest.NewRequest(http.MethodPost, "/api/v2/core/settings/search", bytes.NewBufferString(`{}`)))
	var result struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(verify.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Data["panelName"] != "Demo" {
		t.Fatalf("键值更新未生效: %#v", result.Data["panelName"])
	}
}
