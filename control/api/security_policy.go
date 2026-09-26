// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// isForwardedRelayRequest 识别节点透传请求，真实可信性由 NodeRelay 的签名校验保证。
func isForwardedRelayRequest(r *http.Request) bool {
	if r == nil || !isAPIRequest(r.URL.Path) || strings.TrimSpace(r.Header.Get("X-WorkMesh-Forwarded")) != "1" {
		return false
	}
	// 控制面路由直接注册在主 mux 上，不经过 NodeRelay，仍需执行本地鉴权。
	for _, prefix := range []string{"/api/v2/core/", "/api/v2/gateway/", "/api/v2/workmesh/"} {
		if strings.HasPrefix(r.URL.Path, prefix) {
			return false
		}
	}
	return true
}

// checkBoundDomain 校验请求是否来自绑定域名，并为 API 和网页返回对应错误。
func checkBoundDomain(w http.ResponseWriter, r *http.Request, settings SecuritySettings) bool {
	bound := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(settings.BindDomain)), ".")
	if bound == "" || isLocalRequest(r) {
		return true
	}
	host := strings.ToLower(strings.TrimSuffix(requestHost(r), "."))
	if host == bound {
		return true
	}
	if isAPIRequest(r.URL.Path) {
		writeSecurityError(w, http.StatusForbidden, "DOMAIN_MISMATCH", "请求域名与绑定域名不匹配")
	} else {
		writeSecurityError(w, http.StatusForbidden, "DOMAIN_MISMATCH", "当前访问域名未授权")
	}
	return false
}

// requestHost 提取请求主机名并移除可选端口。
func requestHost(r *http.Request) string {
	host := strings.TrimSpace(r.Host)
	if value, _, err := net.SplitHostPort(host); err == nil {
		return value
	}
	return host
}

// isLocalRequest 识别由本机受信调用标记的请求。
func isLocalRequest(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-WorkMesh-Local")), "1")
}

// checkSecurityEntrance 校验网页请求的安全入口 Cookie，并允许已认证请求继续访问。
func checkSecurityEntrance(w http.ResponseWriter, r *http.Request, settings SecuritySettings, authorize func(*http.Request) bool) bool {
	entrance := strings.Trim(strings.TrimSpace(settings.SecurityEntrance), "/")
	if entrance == "" || r.URL.Path == "/health" || r.URL.Path == "/ready" || isAPIRequest(r.URL.Path) || isStaticAssetPath(r.URL.Path) || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return true
	}
	if strings.Trim(strings.TrimSpace(r.URL.Path), "/") == entrance {
		http.SetCookie(w, &http.Cookie{Name: "SecurityEntrance", Value: base64.StdEncoding.EncodeToString([]byte(entrance)), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		return true
	}
	if cookie, err := r.Cookie("SecurityEntrance"); err == nil {
		if decoded, decodeErr := base64.StdEncoding.DecodeString(cookie.Value); decodeErr == nil && subtle.ConstantTimeCompare(decoded, []byte(entrance)) == 1 {
			return true
		}
	}
	if authorize != nil && authorize(r) {
		return true
	}
	writeSecurityError(w, http.StatusNotFound, "SECURITY_ENTRANCE_REQUIRED", "请通过安全入口访问服务")
	return false
}

// checkCSRF 校验 Cookie 会话写请求的来源和 CSRF 双提交令牌。
func checkCSRF(w http.ResponseWriter, r *http.Request) bool {
	if !unsafeHTTPMethod(r.Method) || hasNonCookieCredential(r) || !hasSessionCookie(r) {
		return true
	}
	if site := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); site == "cross-site" {
		writeSecurityError(w, http.StatusForbidden, "CSRF_INVALID", "跨站请求被拒绝")
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" && !sameOrigin(origin, r) {
		writeSecurityError(w, http.StatusForbidden, "CSRF_INVALID", "请求来源与当前站点不匹配")
		return false
	}
	cookie, cookieErr := r.Cookie(csrfCookieName)
	header := strings.TrimSpace(r.Header.Get(csrfHeaderName))
	if cookieErr != nil || cookie.Value == "" || header == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
		writeSecurityError(w, http.StatusForbidden, "CSRF_INVALID", "CSRF token invalid")
		return false
	}
	return true
}

// sameOrigin 比较来源主机和请求主机，避免把用户可控路径纳入校验。
func sameOrigin(origin string, r *http.Request) bool {
	parsed, err := neturlParse(origin)
	if err != nil || parsed == "" {
		return false
	}
	return strings.EqualFold(parsed, requestHost(r)) || strings.EqualFold(parsed, r.Host)
}

// neturlParse 仅返回 URL 的主机部分，避免将用户可控路径纳入来源比较。
func neturlParse(value string) (string, error) {
	if !strings.Contains(value, "://") {
		return "", errors.New("origin scheme missing")
	}
	parts := strings.SplitN(value, "://", 2)
	if len(parts) != 2 || parts[1] == "" || strings.ContainsAny(parts[1], "/?#\r\n") {
		return "", errors.New("invalid origin")
	}
	return parts[1], nil
}

// ensureCSRFToken 为已有会话补发浏览器可读取的 CSRF Cookie。
func ensureCSRFToken(w http.ResponseWriter, r *http.Request) {
	if !hasSessionCookie(r) {
		return
	}
	if _, err := r.Cookie(csrfCookieName); err == nil {
		return
	}
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return
	}
	token := hex.EncodeToString(raw[:])
	http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: token, Path: "/", HttpOnly: false, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil})
}

// passwordExpired 判断当前 Cookie 会话是否超过安全设置中的密码有效期。
func passwordExpired(r *http.Request, settings SecuritySettings, authorize func(*http.Request) bool) bool {
	if settings.ExpirationDays <= 0 || isPasswordExpirationExempt(r) || hasNonCookieCredential(r) {
		return false
	}
	expires := settings.ExpirationTime
	if expires.IsZero() && !settings.PasswordChangedAt.IsZero() {
		expires = settings.PasswordChangedAt.AddDate(0, 0, settings.ExpirationDays)
	}
	if expires.IsZero() {
		return false
	}
	return time.Now().After(expires)
}

// isPasswordExpirationExempt 列出登录、密码重置和设置查询等免过期检查路径。
func isPasswordExpirationExempt(r *http.Request) bool {
	path := r.URL.Path
	return strings.HasPrefix(path, "/api/v2/core/auth") || path == "/api/v2/core/settings/search" || path == "/api/v2/core/settings/search/base" || path == "/api/v2/core/auth/expired/reset"
}

// hasSessionCookie 判断请求是否包含非空的本地会话 Cookie。
func hasSessionCookie(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	return err == nil && strings.TrimSpace(cookie.Value) != ""
}

// hasNonCookieCredential 判断请求是否携带令牌类认证凭据。
func hasNonCookieCredential(r *http.Request) bool {
	for _, name := range []string{"Authorization", "X-WorkMesh-Token", "X-API-Key", "X-Panel-Local-Token"} {
		if strings.TrimSpace(r.Header.Get(name)) != "" {
			return true
		}
	}
	return false
}

// unsafeHTTPMethod 判断 HTTP 方法是否可能改变服务端状态。
func unsafeHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

// isAPIRequest 判断路径是否属于统一的 v2 API。
func isAPIRequest(path string) bool { return strings.HasPrefix(path, "/api/v2/") }

// isStaticAPIPath 判断路径是否属于 API 静态资源。
func isStaticAPIPath(path string) bool {
	return strings.HasPrefix(path, "/api/v2/images/") || strings.HasPrefix(path, "/api/v2/static/")
}

// isStaticWebPath 判断路径是否属于网站静态入口或资源目录。
func isStaticWebPath(path string) bool {
	return path == "/" || path == "/favicon.ico" || strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/public/")
}

// isStaticAssetPath 判断路径是否属于静态资源文件。
func isStaticAssetPath(path string) bool {
	return path == "/favicon.ico" || strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/public/")
}

// isPublicSecurityPath 列出无需本地会话即可访问的认证和健康检查接口。
func isPublicSecurityPath(r *http.Request) bool {
	if r.Method == http.MethodGet {
		switch r.URL.Path {
		case "/api/v2/health", "/api/v2/health/check", "/api/v2/core/health", "/api/v2/core/auth/captcha", "/api/v2/core/auth/setting", "/api/v2/core/auth/welcome", "/api/v2/core/auth/ldap/status", "/api/v2/core/auth/oidc/status", "/api/v2/core/auth/saml2/status":
			return true
		}
	}
	if r.Method == http.MethodPost {
		switch r.URL.Path {
		case "/api/v2/core/auth/login", "/api/v2/core/auth/mfalogin", "/api/v2/core/auth/passkey/finish", "/api/v2/core/auth/oidc/begin", "/api/v2/core/auth/oidc/finish", "/api/v2/core/auth/saml2/begin", "/api/v2/core/auth/saml2/finish":
			return true
		}
	}
	return false
}

// isSelfAuthenticatedStream 列出使用一次性令牌和握手来源校验的流式接口。
func isSelfAuthenticatedStream(path string) bool {
	switch path {
	case "/api/v2/process/ws", "/api/v2/containers/search/log", "/api/v2/files/wget/process", "/api/v2/hosts/terminal/local", "/api/v2/hosts/terminal/container", "/api/v2/hosts/terminal/ssh", "/api/v2/core/script/run":
		return true
	default:
		return false
	}
}

// securityError 构造统一安全错误响应的业务 envelope。
func securityError(code, message string) map[string]any {
	return map[string]any{"code": "ERR", "message": message, "details": map[string]string{"errCode": code}}
}

// writeSecurityError 使用统一 HTTP 响应格式写出安全策略错误。
func writeSecurityError(w http.ResponseWriter, status int, code, message string) {
	wmhttp.JSON(w, status, securityError(code, message))
}
