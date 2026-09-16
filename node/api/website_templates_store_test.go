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
	"strconv"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// TestWebsiteTemplateLegacyBlobMigratesOnce 验证旧 JSON 状态可导入关系表且重复初始化幂等。
func TestWebsiteTemplateLegacyBlobMigratesOnce(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	dbPath := filepath.Join(root, "workmesh.db")
	store, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().Exec(`CREATE TABLE IF NOT EXISTS website_extension_state (id INTEGER PRIMARY KEY CHECK(id=1), payload BLOB NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	payload := `{"templates":[{"id":"11","name":"legacy","type":"single","content":"Hi {{name}}","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}],"outputs":[{"id":"21","name":"legacy-output","templateID":"11","templateType":"single","variableValues":"{\"name\":\"old\"}","outputPath":"","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]}`
	if _, err := store.DB().Exec(`INSERT INTO website_extension_state(id,payload,updated_at) VALUES(1,?,?)`, []byte(payload), "2026-01-01T00:00:00Z"); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	registerWebsiteExtensionRoutes(mux)
	search := websiteTemplateTestRequest(mux, http.MethodPost, "/api/v2/websites/templates/search", `{"page":1,"pageSize":20}`)
	if search.Code != http.StatusOK || !bytes.Contains(search.Body.Bytes(), []byte(`"legacy"`)) {
		t.Fatalf("legacy template missing: %d %s", search.Code, search.Body.String())
	}
	outputs := websiteTemplateTestRequest(mux, http.MethodPost, "/api/v2/websites/templates/outputs/search", `{"page":1,"pageSize":20}`)
	if outputs.Code != http.StatusOK || !bytes.Contains(outputs.Body.Bytes(), []byte(`"legacy-output"`)) {
		t.Fatalf("legacy output missing: %d %s", outputs.Code, outputs.Body.String())
	}

	verify, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer verify.Close()
	var templateCount, outputCount, migrationCount int
	if err := verify.DB().QueryRow(`SELECT COUNT(*) FROM website_templates`).Scan(&templateCount); err != nil {
		t.Fatal(err)
	}
	if err := verify.DB().QueryRow(`SELECT COUNT(*) FROM website_template_outputs`).Scan(&outputCount); err != nil {
		t.Fatal(err)
	}
	if err := verify.DB().QueryRow(`SELECT COUNT(*) FROM website_template_migrations WHERE migration_key='legacy-json-v1'`).Scan(&migrationCount); err != nil {
		t.Fatal(err)
	}
	if templateCount != 1 || outputCount != 1 || migrationCount != 1 {
		t.Fatalf("unexpected migration counts: templates=%d outputs=%d migrations=%d", templateCount, outputCount, migrationCount)
	}
	if err := ensureWebsiteTemplateTables(verify.DB()); err != nil {
		t.Fatal(err)
	}
	if err := verify.DB().QueryRow(`SELECT COUNT(*) FROM website_templates`).Scan(&templateCount); err != nil {
		t.Fatal(err)
	}
	if templateCount != 1 {
		t.Fatalf("repeated migration duplicated template: %d", templateCount)
	}
}

// TestWebsiteTemplatePreviewUsesPersistedTemplate 验证预览读取 SQLite 模板而不是回显请求正文。
func TestWebsiteTemplatePreviewUsesPersistedTemplate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	mux := http.NewServeMux()
	registerWebsiteExtensionRoutes(mux)
	created := websiteTemplateTestRequest(mux, http.MethodPost, "/api/v2/websites/templates", `{"name":"preview","type":"single","content":"Hello {{name}}"}`)
	if created.Code != http.StatusOK {
		t.Fatalf("create template: %d %s", created.Code, created.Body.String())
	}
	var envelope struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil || envelope.Data.ID == 0 {
		t.Fatalf("invalid create response: %s", created.Body.String())
	}
	body := `{"templateID":` + itoaTemplateID(envelope.Data.ID) + `,"content":"fake","variableValues":{"name":"WorkMesh"}}`
	preview := websiteTemplateTestRequest(mux, http.MethodPost, "/api/v2/websites/templates/preview", body)
	if preview.Code != http.StatusOK || !bytes.Contains(preview.Body.Bytes(), []byte(`"html":"Hello WorkMesh"`)) || bytes.Contains(preview.Body.Bytes(), []byte("fake")) {
		t.Fatalf("preview did not use persisted template: %d %s", preview.Code, preview.Body.String())
	}
}

// TestWebsiteTemplateOutputCRUDUsesSQLite 验证产物创建、查询、分页、真实渲染和删除均来自 SQLite。
func TestWebsiteTemplateOutputCRUDUsesSQLite(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	mux := http.NewServeMux()
	registerWebsiteExtensionRoutes(mux)
	created := websiteTemplateTestRequest(mux, http.MethodPost, "/api/v2/websites/templates", `{"name":"output-template","type":"single","content":"{{title}}"}`)
	var templateEnvelope struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &templateEnvelope); err != nil || templateEnvelope.Data.ID == 0 {
		t.Fatalf("invalid template response: %s", created.Body.String())
	}
	id := itoaTemplateID(templateEnvelope.Data.ID)
	output := websiteTemplateTestRequest(mux, http.MethodPost, "/api/v2/websites/templates/outputs", `{"templateID":`+id+`,"name":"generated","variableValues":{"title":"real"}}`)
	if output.Code != http.StatusOK || !bytes.Contains(output.Body.Bytes(), []byte(`"outputPath"`)) {
		t.Fatalf("create output failed: %d %s", output.Code, output.Body.String())
	}
	var outputEnvelope struct {
		Data struct {
			ID         uint   `json:"id"`
			OutputPath string `json:"outputPath"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output.Body.Bytes(), &outputEnvelope); err != nil || outputEnvelope.Data.ID == 0 {
		t.Fatalf("invalid output response: %s", output.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(outputEnvelope.Data.OutputPath, "index.html"))
	if err != nil || string(content) != "real" {
		t.Fatalf("output was not rendered: %v %q", err, content)
	}

	mux2 := http.NewServeMux()
	registerWebsiteExtensionRoutes(mux2)
	search := websiteTemplateTestRequest(mux2, http.MethodPost, "/api/v2/websites/templates/outputs/search", `{"templateID":`+id+`,"page":1,"pageSize":1}`)
	if search.Code != http.StatusOK || !bytes.Contains(search.Body.Bytes(), []byte(`"generated"`)) {
		t.Fatalf("output not loaded from SQLite: %d %s", search.Code, search.Body.String())
	}
	deleteBody := `{"id":` + itoaTemplateID(outputEnvelope.Data.ID) + `}`
	deleted := websiteTemplateTestRequest(mux2, http.MethodPost, "/api/v2/websites/templates/outputs/del", deleteBody)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete output failed: %d %s", deleted.Code, deleted.Body.String())
	}
	searchEmpty := websiteTemplateTestRequest(mux2, http.MethodPost, "/api/v2/websites/templates/outputs/search", `{"templateID":`+id+`,"page":1,"pageSize":10}`)
	if searchEmpty.Code != http.StatusOK || !bytes.Contains(searchEmpty.Body.Bytes(), []byte(`"total":0`)) {
		t.Fatalf("deleted output remains: %d %s", searchEmpty.Code, searchEmpty.Body.String())
	}
}

// websiteTemplateTestRequest 发送 JSON 请求并设置兼容的内容类型。
func websiteTemplateTestRequest(mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// itoaTemplateID 将测试中的 ID 转换为 JSON 数字文本。
func itoaTemplateID(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
