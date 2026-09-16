// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestWebsiteTemplateUploadWritesRealZip 验证上传端点会真实落盘并返回可用文件路径。
func TestWebsiteTemplateUploadWritesRealZip(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	part, err := zipWriter.Create("index.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("<h1>{{siteName}}</h1>")); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", "demo.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(archive.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	registerWebsiteExtensionRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/websites/templates/upload", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("upload status=%d body=%s", res.Code, res.Body.String())
	}
	var envelope struct {
		Data struct {
			FilePath string   `json:"filePath"`
			Vars     []string `json:"variables"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid response: %v body=%s", err, res.Body.String())
	}
	if envelope.Data.FilePath == "" || !strings.HasSuffix(envelope.Data.FilePath, "demo.zip") {
		t.Fatalf("unexpected file path: %q", envelope.Data.FilePath)
	}
	if len(envelope.Data.Vars) != 1 || envelope.Data.Vars[0] != "siteName" {
		t.Fatalf("unexpected variables: %#v", envelope.Data.Vars)
	}
	data, err := os.ReadFile(envelope.Data.FilePath)
	if err != nil {
		t.Fatalf("uploaded file is not readable: %v", err)
	}
	if !bytes.Equal(data, archive.Bytes()) {
		t.Fatalf("uploaded archive differs from request")
	}
	if filepath.Dir(envelope.Data.FilePath) != filepath.Join(root, "templates", "files") {
		t.Fatalf("upload escaped data directory: %s", envelope.Data.FilePath)
	}
	// 上传返回的 filePath 应能直接用于 multi 模板创建与真实预览。
	create := httptest.NewRequest(http.MethodPost, "/api/v2/websites/templates", strings.NewReader(`{"name":"uploaded","type":"multi","filePath":"`+envelope.Data.FilePath+`"}`))
	create.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	mux.ServeHTTP(createRes, create)
	if createRes.Code != http.StatusOK {
		t.Fatalf("uploaded template create status=%d body=%s", createRes.Code, createRes.Body.String())
	}
	var created struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil || created.Data.ID == 0 {
		t.Fatalf("invalid uploaded template response: %s", createRes.Body.String())
	}
	preview := httptest.NewRequest(http.MethodPost, "/api/v2/websites/templates/preview", strings.NewReader(`{"templateID":`+strconv.FormatUint(uint64(created.Data.ID), 10)+`,"variableValues":{"siteName":"Uploaded"}}`))
	preview.Header.Set("Content-Type", "application/json")
	previewRes := httptest.NewRecorder()
	mux.ServeHTTP(previewRes, preview)
	var previewEnvelope struct {
		Data struct {
			HTML string `json:"html"`
		} `json:"data"`
	}
	if err := json.Unmarshal(previewRes.Body.Bytes(), &previewEnvelope); err != nil || previewRes.Code != http.StatusOK || previewEnvelope.Data.HTML != "<h1>Uploaded</h1>" {
		t.Fatalf("uploaded template preview failed: %d %s", previewRes.Code, previewRes.Body.String())
	}
}

// TestWebsiteTemplateUploadRejectsNonZip 验证非 ZIP 上传不会伪造成功或留下文件。
func TestWebsiteTemplateUploadRejectsNonZip(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", "demo.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("not zip"))
	_ = form.Close()
	mux := http.NewServeMux()
	registerWebsiteExtensionRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/websites/templates/upload", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d: %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "templates", "files")); !os.IsNotExist(err) {
		t.Fatalf("rejected upload left files behind: %v", err)
	}
}
