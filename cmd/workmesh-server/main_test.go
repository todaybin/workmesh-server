// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/todaybin/workmesh-server/config"
	controlapi "github.com/todaybin/workmesh-server/control/api"
	nodeapi "github.com/todaybin/workmesh-server/node/api"
)

func TestSignedNodeRelayPassesOuterAndNodeSessionMiddleware(t *testing.T) {
	dataDir := t.TempDir()
	const secret = "relay-secret"
	remoteMux := http.NewServeMux()
	remoteMux.HandleFunc("/api/v2/files/search", func(w http.ResponseWriter, r *http.Request) {
		if !nodeapi.IsForwardedRequestVerified(r) {
			t.Fatal("目标节点处理器未收到已验签上下文")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	remoteRelay := nodeapi.NewNodeRelay(authenticateNodeAPI(remoteMux), nodeapi.RelayOptions{
		DataDir: dataDir, NodeID: "secondary", Secret: []byte(secret),
		RoleEpoch: func(context.Context) (uint64, error) { return 1, nil },
	})
	remoteServer := httptest.NewServer(controlapi.NewSecurityMiddleware(remoteRelay, controlapi.SecurityMiddlewareOptions{
		DataDir: dataDir, Authorize: func(*http.Request) bool { return false },
	}))
	defer remoteServer.Close()
	if err := os.WriteFile(filepath.Join(dataDir, "nodes.json"), []byte(`[{"nodeId":"secondary","addr":"`+remoteServer.URL+`"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	localMux := http.NewServeMux()
	localMux.HandleFunc("/api/v2/files/search", func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("本地处理器不应被调用")
	})
	localRelay := nodeapi.NewNodeRelay(authenticateNodeAPI(localMux), nodeapi.RelayOptions{
		DataDir: dataDir, NodeID: "primary", Secret: []byte(secret),
		RoleEpoch: func(context.Context) (uint64, error) { return 1, nil },
	})
	handler := controlapi.NewSecurityMiddleware(localRelay, controlapi.SecurityMiddlewareOptions{
		DataDir: dataDir, Authorize: func(r *http.Request) bool {
			cookie, err := r.Cookie("workmesh_session")
			return err == nil && cookie.Value == "local-session"
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v2/files/search?operateNode=secondary", strings.NewReader(`{}`))
	request.AddCookie(&http.Cookie{Name: "workmesh_session", Value: "local-session"})
	request.AddCookie(&http.Cookie{Name: "pcsrftoken", Value: "csrf"})
	request.Header.Set("X-CSRF-Token", "csrf")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("签名透传不应被 Session 中间件拦截: %d %s", response.Code, response.Body.String())
	}
}

func TestHTTPMuxServesJavaScriptAssetsWithModuleMIME(t *testing.T) {
	frontend := fstest.MapFS{
		"assets/js/module.js": &fstest.MapFile{Data: []byte("export const ready = true;\n")},
	}
	mux, _ := httpMuxWithReadinessAndFrontend(configForTest(), newReadinessState(), frontend)
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
	if cache := recorder.Header().Get("Cache-Control"); cache != "private, max-age=2628000, immutable" {
		t.Fatalf("哈希静态资源缓存头错误: %q", cache)
	}
}

func TestHTTPMuxFallsBackToSPAForFrontendRoutes(t *testing.T) {
	frontend := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><div id=app>workmesh</div>")},
	}
	mux, _ := httpMuxWithReadinessAndFrontend(configForTest(), newReadinessState(), frontend)
	for _, path := range []string{"/login", "/settings/bind"} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("前端路由 %s 状态码 = %d", path, recorder.Code)
		}
		if !strings.Contains(recorder.Body.String(), "id=app") {
			t.Fatalf("前端路由 %s 未返回 index.html: %s", path, recorder.Body.String())
		}
		if cache := recorder.Header().Get("Cache-Control"); cache != "no-store, max-age=0" {
			t.Fatalf("前端入口必须禁止缓存，实际为 %q", cache)
		}
	}
}

func TestHTTPMuxDoesNotFallbackUnknownAPIToHTML(t *testing.T) {
	frontend := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><div id=app>workmesh</div>")},
	}
	mux, _ := httpMuxWithReadinessAndFrontend(configForTest(), newReadinessState(), frontend)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/unknown", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("未知 API 状态码 = %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "id=app") {
		t.Fatal("未知 API 不应返回 SPA HTML")
	}
}

func TestHTTPMuxServesStaticJSONWithETag(t *testing.T) {
	frontend := fstest.MapFS{
		"index.html":        &fstest.MapFile{Data: []byte("<!doctype html><div id=app>workmesh</div>")},
		"static/china.json": &fstest.MapFile{Data: []byte(`{"name":"china"}`)},
		"static/world.json": &fstest.MapFile{Data: []byte(`{"name":"world"}`)},
		"favicon.png":       &fstest.MapFile{Data: []byte("PNG")},
		"assets/js/main.js": &fstest.MapFile{Data: []byte("export {}")},
	}
	mux, _ := httpMuxWithReadinessAndFrontend(configForTest(), newReadinessState(), frontend)

	first := httptest.NewRecorder()
	mux.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v2/static/china.json", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("地图资源状态码 = %d, body = %s", first.Code, first.Body.String())
	}
	if contentType := first.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("地图资源 MIME 类型错误: %q", contentType)
	}
	if cache := first.Header().Get("Cache-Control"); cache != "private, max-age=2628000" {
		t.Fatalf("地图资源缓存头错误: %q", cache)
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("地图资源应包含 ETag")
	}

	second := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v2/static/china.json", nil)
	request.Header.Set("If-None-Match", etag)
	mux.ServeHTTP(second, request)
	if second.Code != http.StatusNotModified {
		t.Fatalf("相同 ETag 应返回 304，实际为 %d", second.Code)
	}
}

func TestHTTPMuxLimitsEmbeddedPublicAssets(t *testing.T) {
	frontend := fstest.MapFS{
		"index.html":       &fstest.MapFile{Data: []byte("<!doctype html><div id=app>workmesh</div>")},
		"favicon.png":      &fstest.MapFile{Data: []byte("PNG")},
		"assets/secret.js": &fstest.MapFile{Data: []byte("secret")},
	}
	mux, _ := httpMuxWithReadinessAndFrontend(configForTest(), newReadinessState(), frontend)
	for _, path := range []string{
		"/public/assets/secret.js",
		"/favicon.ico/assets/secret.js",
		"/api/v2/images/logo",
		"/api/v2/static/unknown.json",
	} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s 应拒绝嵌入目录穿透，实际状态码 = %d", path, recorder.Code)
		}
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

func TestReadinessReturnsUnavailableBeforeBootstrap(t *testing.T) {
	state := newReadinessState()
	mux, _ := httpMuxWithReadiness(configForTest(), state)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("bootstrap 未完成时应返回 503，实际为 %d", recorder.Code)
	}
	state.SetReady()
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("bootstrap 完成后应返回 200，实际为 %d", recorder.Code)
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
