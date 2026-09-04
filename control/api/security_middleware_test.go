// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSecurityMiddlewareSessionAndCSRF(t *testing.T) {
	handler := NewSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), SecurityMiddlewareOptions{Authorize: func(r *http.Request) bool {
		cookie, err := r.Cookie(sessionCookieName)
		return err == nil && cookie.Value == "session-1"
	}})

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v2/settings/search", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("未登录请求应返回 401，实际 %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v2/settings/update", strings.NewReader(`{"key":"theme","value":"dark"}`))
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-1"})
	request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf-1"})
	request.Header.Set(csrfHeaderName, "invalid")
	forbidden := httptest.NewRecorder()
	handler.ServeHTTP(forbidden, request)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("CSRF token 错误应返回 403，实际 %d", forbidden.Code)
	}

	request.Header.Set(csrfHeaderName, "csrf-1")
	ok := httptest.NewRecorder()
	handler.ServeHTTP(ok, request)
	if ok.Code != http.StatusNoContent {
		t.Fatalf("有效 Session/CSRF 请求应放行，实际 %d", ok.Code)
	}
}

func TestSecurityMiddlewareAllowsSignedRelayToReachNodeRelay(t *testing.T) {
	authorizedCalled := false
	handler := NewSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), SecurityMiddlewareOptions{Authorize: func(*http.Request) bool {
		authorizedCalled = true
		return false
	}})
	request := httptest.NewRequest(http.MethodPost, "/api/v2/files/search", strings.NewReader(`{}`))
	request.Header.Set("X-WorkMesh-Forwarded", "1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("已标记透传请求应交由 NodeRelay 验签，实际状态 %d", response.Code)
	}
	if authorizedCalled {
		t.Fatal("透传请求不应触发本地 Session 鉴权")
	}
}

func TestSecurityMiddlewareMintsCSRFCookie(t *testing.T) {
	// 使用真实布尔授权器，确保响应中的 token 可被后续请求复用。
	handler := NewSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), SecurityMiddlewareOptions{Authorize: func(r *http.Request) bool {
		cookie, err := r.Cookie(sessionCookieName)
		return err == nil && cookie.Value == "session-1"
	}})
	request := httptest.NewRequest(http.MethodGet, "/api/v2/settings/search", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-1"})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("授权查询应放行，实际 %d", recorder.Code)
	}
	csrf := recorder.Result().Cookies()
	if len(csrf) == 0 || csrf[0].Name != csrfCookieName || csrf[0].Value == "" {
		t.Fatalf("授权响应未生成 CSRF Cookie: %#v", csrf)
	}
}

func TestSecurityMiddlewareBoundDomain(t *testing.T) {
	root := t.TempDir()
	writeSecurityDomains(t, root, map[string]any{"bindDomain": "panel.example.test"})
	handler := NewSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }), SecurityMiddlewareOptions{DataDir: root})

	bad := httptest.NewRequest(http.MethodGet, "/api/v2/health", nil)
	bad.Host = "other.example.test"
	badRecorder := httptest.NewRecorder()
	handler.ServeHTTP(badRecorder, bad)
	if badRecorder.Code != http.StatusForbidden {
		t.Fatalf("错误域名应返回 403，实际 %d", badRecorder.Code)
	}

	good := httptest.NewRequest(http.MethodGet, "/api/v2/health", nil)
	good.Host = "panel.example.test:443"
	goodRecorder := httptest.NewRecorder()
	handler.ServeHTTP(goodRecorder, good)
	if goodRecorder.Code != http.StatusNoContent {
		t.Fatalf("正确域名应放行，实际 %d", goodRecorder.Code)
	}
}

func TestSecurityMiddlewarePasswordExpiration(t *testing.T) {
	root := t.TempDir()
	writeSecurityDomains(t, root, map[string]any{"expirationDays": 1, "expirationTime": time.Now().Add(-time.Hour).Format(time.RFC3339)})
	handler := NewSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }), SecurityMiddlewareOptions{DataDir: root, Authorize: func(*http.Request) bool { return true }})
	request := httptest.NewRequest(http.MethodGet, "/api/v2/websites", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != 313 {
		t.Fatalf("密码过期应返回 313，实际 %d", recorder.Code)
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response["message"] == nil {
		t.Fatalf("过期响应应包含上下文错误: %s", recorder.Body.String())
	}

	reset := httptest.NewRecorder()
	handler.ServeHTTP(reset, httptest.NewRequest(http.MethodPost, "/api/v2/core/auth/expired/reset", nil))
	if reset.Code != http.StatusNoContent {
		t.Fatalf("密码过期重置入口必须放行，实际 %d", reset.Code)
	}
}

func TestSecurityMiddlewareSecurityEntrance(t *testing.T) {
	root := t.TempDir()
	writeSecurityDomains(t, root, map[string]any{"securityEntrance": "safe-entry"})
	handler := NewSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }), SecurityMiddlewareOptions{DataDir: root})
	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, httptest.NewRequest(http.MethodGet, "/login", nil))
	if blocked.Code != http.StatusNotFound {
		t.Fatalf("未通过安全入口的前端路径应返回 404，实际 %d", blocked.Code)
	}
	allowed := httptest.NewRecorder()
	handler.ServeHTTP(allowed, httptest.NewRequest(http.MethodGet, "/safe-entry", nil))
	if allowed.Code != http.StatusNoContent {
		t.Fatalf("安全入口路径应放行，实际 %d", allowed.Code)
	}
}

func TestSecurityMiddlewareUsesSQLiteSettingsProvider(t *testing.T) {
	handler := NewSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), SecurityMiddlewareOptions{
		DataDir: t.TempDir(),
		Settings: func() map[string]any {
			return map[string]any{"securityEntrance": "sqlite-entry", "bindDomain": "panel.example.test"}
		},
	})
	blocked := httptest.NewRequest(http.MethodGet, "/login", nil)
	blocked.Host = "panel.example.test"
	blockedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(blockedRecorder, blocked)
	if blockedRecorder.Code != http.StatusNotFound {
		t.Fatalf("SQLite 安全入口未生效: %d", blockedRecorder.Code)
	}
	allowed := httptest.NewRequest(http.MethodGet, "/sqlite-entry", nil)
	allowed.Host = "panel.example.test"
	allowedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(allowedRecorder, allowed)
	if allowedRecorder.Code != http.StatusNoContent {
		t.Fatalf("SQLite 安全入口路径未放行: %d", allowedRecorder.Code)
	}
}

func TestSecurityMiddlewareLocalizesSecurityErrors(t *testing.T) {
	root := t.TempDir()
	writeSecurityDomains(t, root, map[string]any{"bindDomain": "panel.example.test"})
	handler := NewSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), SecurityMiddlewareOptions{DataDir: root, Authorize: func(*http.Request) bool { return false }})

	unauthorized := httptest.NewRequest(http.MethodGet, "/api/v2/websites", nil)
	unauthorized.Host = "panel.example.test"
	unauthorized.Header.Set("Accept-Language", "en-US,en;q=0.8")
	unauthorizedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedRecorder, unauthorized)
	if unauthorizedRecorder.Code != http.StatusUnauthorized || strings.Contains(unauthorizedRecorder.Body.String(), "本地登录") {
		t.Fatalf("未授权错误未按英文返回: status=%d body=%s", unauthorizedRecorder.Code, unauthorizedRecorder.Body.String())
	}

	csrf := httptest.NewRequest(http.MethodPost, "/api/v2/websites", strings.NewReader(`{}`))
	csrf.Host = "panel.example.test"
	csrf.Header.Set("Accept-Language", "en")
	csrf.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-1"})
	csrf.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf-1"})
	csrf.Header.Set(csrfHeaderName, "bad")
	csrfRecorder := httptest.NewRecorder()
	// 授权器只验证会话，确保请求能走到 CSRF 校验分支。
	allowSession := NewSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }), SecurityMiddlewareOptions{DataDir: root, Authorize: func(r *http.Request) bool {
		cookie, err := r.Cookie(sessionCookieName)
		return err == nil && cookie.Value == "session-1"
	}})
	allowSession.ServeHTTP(csrfRecorder, csrf)
	if csrfRecorder.Code != http.StatusForbidden || strings.Contains(csrfRecorder.Body.String(), "CSRF token invalid") {
		t.Fatalf("CSRF 错误未本地化: status=%d body=%s", csrfRecorder.Code, csrfRecorder.Body.String())
	}

	domain := httptest.NewRequest(http.MethodGet, "/api/v2/health", nil)
	domain.Host = "other.example.test"
	domain.Header.Set("Accept-Language", "en")
	domainRecorder := httptest.NewRecorder()
	handler.ServeHTTP(domainRecorder, domain)
	if domainRecorder.Code != http.StatusForbidden || strings.Contains(domainRecorder.Body.String(), "域名") {
		t.Fatalf("域名错误未本地化: status=%d body=%s", domainRecorder.Code, domainRecorder.Body.String())
	}
}

func writeSecurityDomains(t *testing.T, root string, settings map[string]any) {
	t.Helper()
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"settings": settings})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "domains.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
