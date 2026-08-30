// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCoreAuthCurrentAndAPIKey(t *testing.T) {
	mux := http.NewServeMux()
	registerCoreAuthExtras(mux)
	mux.HandleFunc("POST /api/v2/core/auth/login", handleCoreLogin)
	mux.HandleFunc("GET /api/v2/core/auth/current", handleCoreCurrent)
	login := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/core/auth/login", bytes.NewBufferString(`{"name":"admin","password":"admin"}`))
	mux.ServeHTTP(login, req)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d", login.Code)
	}
	cookie := login.Result().Cookies()[0]
	var envelope struct {
		Data struct{ Name, Role, Token string } `json:"data"`
	}
	if err := json.NewDecoder(login.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Name != "admin" || envelope.Data.Role != "ADMIN" || envelope.Data.Token == "" {
		t.Fatalf("登录响应不符合前端契约: %#v", envelope.Data)
	}
	current := httptest.NewRecorder()
	currentReq := httptest.NewRequest(http.MethodGet, "/api/v2/core/auth/current", nil)
	currentReq.AddCookie(cookie)
	mux.ServeHTTP(current, currentReq)
	if current.Code != http.StatusOK {
		t.Fatalf("current status = %d", current.Code)
	}
	var currentBody struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(current.Body).Decode(&currentBody); err != nil {
		t.Fatal(err)
	}
	if currentBody.Data["name"] != "admin" || currentBody.Data["permissions"] == nil {
		t.Fatalf("当前用户字段缺失: %#v", currentBody.Data)
	}
	keyRes := httptest.NewRecorder()
	keyReq := httptest.NewRequest(http.MethodPost, "/api/v2/core/auth/api/generate", nil)
	keyReq.AddCookie(cookie)
	mux.ServeHTTP(keyRes, keyReq)
	if keyRes.Code != http.StatusOK {
		t.Fatalf("api key status = %d", keyRes.Code)
	}
	var keyBody struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(keyRes.Body).Decode(&keyBody); err != nil {
		t.Fatal(err)
	}
	if keyBody.Data == "" {
		t.Fatal("API Key 为空")
	}
	apiCurrent := httptest.NewRecorder()
	apiReq := httptest.NewRequest(http.MethodGet, "/api/v2/core/auth/current", nil)
	apiReq.Header.Set("X-API-Key", keyBody.Data)
	mux.ServeHTTP(apiCurrent, apiReq)
	if apiCurrent.Code != http.StatusOK {
		t.Fatalf("api key current status = %d", apiCurrent.Code)
	}
}
