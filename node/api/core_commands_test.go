// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
)

func TestCoreCommandsLifecycleAndPersistence(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	commandStoreInstance = nil
	mux := http.NewServeMux()
	registerCoreCommandRoutes(mux)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/core/commands", bytes.NewBufferString(`{"name":"List files","command":"ls -la","groupBelong":"System"}`)))
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d", create.Code)
	}
	var created struct {
		Data quickCommand `json:"data"`
	}
	if err := json.NewDecoder(create.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Data.ID == "" {
		t.Fatal("missing command id")
	}
	search := httptest.NewRecorder()
	mux.ServeHTTP(search, httptest.NewRequest(http.MethodPost, "/api/v2/core/commands/search", bytes.NewBufferString(`{"info":"list","page":1,"pageSize":20}`)))
	var result struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.NewDecoder(search.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Total != 1 {
		t.Fatalf("total = %d", result.Data.Total)
	}
	update := httptest.NewRecorder()
	body := `{"id":"` + created.Data.ID + `","name":"List all files","command":"ls -A"}`
	mux.ServeHTTP(update, httptest.NewRequest(http.MethodPost, "/api/v2/core/commands/update", bytes.NewBufferString(body)))
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d", update.Code)
	}
	remove := httptest.NewRecorder()
	mux.ServeHTTP(remove, httptest.NewRequest(http.MethodPost, "/api/v2/core/commands/del", bytes.NewBufferString(`{"ids":[`+created.Data.ID+`]}`)))
	if remove.Code != http.StatusOK {
		t.Fatalf("delete status = %d", remove.Code)
	}
}

func TestCoreCommandsCSVUpload(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	commandStoreInstance = nil
	mux := http.NewServeMux()
	registerCoreCommandRoutes(mux)
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="commands.csv"`)
	h.Set("Content-Type", "text/csv")
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("name,command\nhello,echo hello\n"))
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/core/commands/upload", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("upload status = %d", res.Code)
	}
	var result struct {
		Data []quickCommand `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data) != 1 || result.Data[0].Command != "echo hello" {
		t.Fatalf("unexpected upload: %#v", result.Data)
	}
}
