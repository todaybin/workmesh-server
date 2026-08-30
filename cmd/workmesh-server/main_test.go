// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/config"
	controlapi "github.com/todaybin/workmesh-server/control/api"
	nodeapi "github.com/todaybin/workmesh-server/node/api"
)

func TestHTTPMuxServesJavaScriptAssetsWithModuleMIME(t *testing.T) {
	root := t.TempDir()
	assetDir := filepath.Join(root, "web", "dist", "assets", "js")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "module.js"), []byte("export const ready = true;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })

	mux, _ := httpMux(configForTest())
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/js/module.js", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("静态资源状态码 = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/javascript") && !strings.HasPrefix(contentType, "application/javascript") {
		t.Fatalf("JavaScript MIME 类型错误: %q", contentType)
	}
	if !strings.Contains(recorder.Body.String(), "export const ready") {
		t.Fatalf("静态资源内容错误: %s", recorder.Body.String())
	}
}

func TestHTTPMuxFallsBackToSPAForFrontendRoutes(t *testing.T) {
	root := t.TempDir()
	staticRoot := filepath.Join(root, "web", "dist")
	if err := os.MkdirAll(staticRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticRoot, "index.html"), []byte("<!doctype html><div id=app>workmesh</div>"), 0o644); err != nil {
		t.Fatal(err)
	}
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })

	mux, _ := httpMux(configForTest())
	for _, path := range []string{"/login", "/settings/bind"} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("前端路由 %s 状态码 = %d", path, recorder.Code)
		}
		if !strings.Contains(recorder.Body.String(), "id=app") {
			t.Fatalf("前端路由 %s 未返回 index.html: %s", path, recorder.Body.String())
		}
	}
}

func TestHTTPMuxDoesNotFallbackUnknownAPIToHTML(t *testing.T) {
	root := t.TempDir()
	staticRoot := filepath.Join(root, "web", "dist")
	if err := os.MkdirAll(staticRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticRoot, "index.html"), []byte("<!doctype html><div id=app>workmesh</div>"), 0o644); err != nil {
		t.Fatal(err)
	}
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })

	mux, _ := httpMux(configForTest())
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/unknown", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("未知 API 状态码 = %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "id=app") {
		t.Fatal("未知 API 不应返回 SPA HTML")
	}
}

func TestHTTPMuxProtectsNodeAPIsAndAllowsLogin(t *testing.T) {
	mux, _ := httpMux(configForTest())
	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/api/v2/ai/accounts", bytes.NewBufferString(`{"name":"blocked"}`)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("未登录节点写接口状态码 = %d, body = %s", unauthorized.Code, unauthorized.Body.String())
	}

	login := httptest.NewRecorder()
	mux.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v2/core/auth/login", bytes.NewBufferString(`{"name":"admin","password":"admin"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("登录入口不应被统一鉴权拦截: %d %s", login.Code, login.Body.String())
	}
	var envelope struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(login.Body).Decode(&envelope); err != nil || envelope.Data.Token == "" {
		t.Fatalf("登录响应缺少会话: %v", err)
	}
	authorized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v2/ai/accounts/providers", nil)
	request.Header.Set("Authorization", "Bearer "+envelope.Data.Token)
	mux.ServeHTTP(authorized, request)
	if authorized.Code != http.StatusOK {
		t.Fatalf("有效 Bearer 会话被拒绝: %d %s", authorized.Code, authorized.Body.String())
	}
}

func TestSecurityWrapperProtectsControlAndLeavesHealthPublic(t *testing.T) {
	mux, _ := httpMux(configForTest())
	secured := controlapi.NewSecurityMiddleware(mux, controlapi.SecurityMiddlewareOptions{Authorize: nodeapi.AuthorizeControlRequest})

	health := httptest.NewRecorder()
	secured.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("健康检查不应依赖登录，状态码 = %d", health.Code)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v2/workmesh/gateway/status", nil)
	blocked := httptest.NewRecorder()
	secured.ServeHTTP(blocked, request)
	if blocked.Code != http.StatusUnauthorized {
		t.Fatalf("控制面状态查询应要求登录，状态码 = %d", blocked.Code)
	}
}

func configForTest() config.Config {
	return config.Config{NodeID: "test-node", Role: "secondary"}
}
