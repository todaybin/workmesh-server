// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/i18n"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

const (
	// sessionCookieName 与浏览器会话 Cookie 名称保持稳定，便于前端和反向代理复用。
	sessionCookieName = "workmesh_session"
	csrfCookieName    = "pcsrftoken"
	csrfHeaderName    = "X-CSRF-Token"
)

// SecuritySettings 是全局安全策略的只读快照。
// 设置由 domains.json 的 settings 对象提供，也可通过环境变量覆盖部署默认值。
type SecuritySettings struct {
	BindDomain           string
	SecurityEntrance     string
	ExpirationDays       int
	ExpirationTime       time.Time
	PasswordChangedAt    time.Time
	AllowUnauthenticated bool
}

// SecurityMiddlewareOptions 配置全局安全包装器。
// Authorize 必须校验本地 Session、Bearer、API Key 或节点令牌；为空时仅启用域名、CSRF 和过期策略。
type SecurityMiddlewareOptions struct {
	DataDir   string
	Authorize func(*http.Request) bool
}

// securitySettingsCache 对低频设置文件进行有界缓存，避免每个请求重复解析 JSON。
type securitySettingsCache struct {
	mu      sync.RWMutex
	path    string
	modTime time.Time
	size    int64
	loaded  bool
	value   SecuritySettings
}

// NewSecurityMiddleware 将 Session、CSRF、域名绑定和密码过期策略统一挂载到 HTTP 链路。
// 健康检查、登录初始化、静态资源和流式接口由各自处理器负责鉴权，不会被重复拦截。
func NewSecurityMiddleware(next http.Handler, options SecurityMiddlewareOptions) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	provider := newSecuritySettingsProvider(options.DataDir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 普通 JSON 请求携带语言元数据，统一错误响应由 runtime/http 按 Accept-Language 渲染。
		// 流式和 WebSocket 请求必须保留原始 ResponseWriter 的 Flusher/Hijacker 能力。
		responseWriter := w
		if r != nil && !isSelfAuthenticatedStream(r.URL.Path) {
			responseWriter = wmhttp.WithLocale(w, i18n.LocaleFromRequest(r))
		}
		if r == nil {
			wmhttp.JSON(responseWriter, http.StatusBadRequest, securityError("INVALID_REQUEST", "请求不能为空"))
			return
		}
		settings := provider.load()
		if !checkBoundDomain(responseWriter, r, settings) {
			return
		}
		// 节点透传请求由 NodeRelay 在更内层完成 HMAC、时间戳、nonce 和 epoch 校验。
		// 在此处跳过本地安全入口、Session、CSRF 和密码过期检查，避免把已认证的
		// 节点调用误判为浏览器请求；未经 NodeRelay 验证的伪造头部仍会被其拒绝。
		if isForwardedRelayRequest(r) {
			next.ServeHTTP(responseWriter, r)
			return
		}
		if !checkSecurityEntrance(responseWriter, r, settings, options.Authorize) {
			return
		}
		if !isAPIRequest(r.URL.Path) {
			next.ServeHTTP(responseWriter, r)
			return
		}
		if isPublicSecurityPath(r) || isStaticAPIPath(r.URL.Path) {
			ensureCSRFToken(responseWriter, r)
			next.ServeHTTP(responseWriter, r)
			return
		}
		// 流接口使用一次性令牌和握手来源校验，不能再要求普通 API Session。
		if isSelfAuthenticatedStream(r.URL.Path) {
			next.ServeHTTP(responseWriter, r)
			return
		}
		if options.Authorize != nil && !options.Authorize(r) {
			writeSecurityError(responseWriter, http.StatusUnauthorized, "LOCAL_AUTH_REQUIRED", "需要有效的本地登录会话")
			return
		}
		if passwordExpired(r, settings, options.Authorize) {
			writeSecurityError(responseWriter, 313, "PASSWORD_EXPIRED", "登录密码已过期，请先重置密码")
			return
		}
		if !checkCSRF(responseWriter, r) {
			return
		}
		ensureCSRFToken(responseWriter, r)
		next.ServeHTTP(responseWriter, r)
	})
}

// isForwardedRelayRequest 仅识别节点透传协议标记，实际可信性由 NodeRelay 验签保证。
func isForwardedRelayRequest(r *http.Request) bool {
	if r == nil || !isAPIRequest(r.URL.Path) || strings.TrimSpace(r.Header.Get("X-WorkMesh-Forwarded")) != "1" {
		return false
	}
	// 控制面（core、gateway、workmesh）路由直接注册在主 mux 上，不经过 NodeRelay；
	// 这些请求必须继续执行本地 Session/CSRF 鉴权，不能仅凭透传头部放行。
	for _, prefix := range []string{"/api/v2/core/", "/api/v2/gateway/", "/api/v2/workmesh/"} {
		if strings.HasPrefix(r.URL.Path, prefix) {
			return false
		}
	}
	return true
}

func newSecuritySettingsProvider(dataDir string) *securitySettingsCache {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		dataDir = strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	}
	if dataDir == "" {
		dataDir = "./data"
	}
	return &securitySettingsCache{path: filepath.Join(dataDir, "domains.json")}
}

func (p *securitySettingsCache) load() SecuritySettings {
	value := SecuritySettings{BindDomain: strings.TrimSpace(os.Getenv("WORKMESH_BIND_DOMAIN")), SecurityEntrance: strings.Trim(strings.TrimSpace(os.Getenv("WORKMESH_SECURITY_ENTRANCE")), "/")}
	info, err := os.Stat(p.path)
	if err == nil {
		p.mu.RLock()
		cached := p.loaded && p.modTime.Equal(info.ModTime()) && p.size == info.Size()
		cachedValue := p.value
		p.mu.RUnlock()
		if cached {
			return mergeSecuritySettings(value, cachedValue)
		}
		if raw, readErr := os.ReadFile(p.path); readErr == nil {
			var document struct {
				Settings map[string]any `json:"settings"`
			}
			if json.Unmarshal(raw, &document) == nil {
				loaded := parseSecuritySettings(document.Settings)
				p.mu.Lock()
				p.modTime, p.size, p.loaded, p.value = info.ModTime(), info.Size(), true, loaded
				p.mu.Unlock()
				return mergeSecuritySettings(value, loaded)
			}
		}
	}
	return value
}

func mergeSecuritySettings(base, file SecuritySettings) SecuritySettings {
	if base.BindDomain == "" {
		base.BindDomain = file.BindDomain
	}
	if base.SecurityEntrance == "" {
		base.SecurityEntrance = file.SecurityEntrance
	}
	if file.ExpirationDays != 0 {
		base.ExpirationDays = file.ExpirationDays
	}
	if !file.ExpirationTime.IsZero() {
		base.ExpirationTime = file.ExpirationTime
	}
	if !file.PasswordChangedAt.IsZero() {
		base.PasswordChangedAt = file.PasswordChangedAt
	}
	return base
}

func parseSecuritySettings(values map[string]any) SecuritySettings {
	result := SecuritySettings{}
	for key, value := range values {
		switch strings.ToLower(strings.ReplaceAll(key, "_", "")) {
		case "binddomain":
			result.BindDomain = strings.TrimSpace(asString(value))
		case "securityentrance":
			result.SecurityEntrance = strings.Trim(strings.TrimSpace(asString(value)), "/")
		case "expirationdays":
			result.ExpirationDays = asInt(value)
		case "expirationtime", "passwordexpirationtime":
			result.ExpirationTime = parseTime(asString(value))
		case "passwordchangedat", "passwordupdatedat":
			result.PasswordChangedAt = parseTime(asString(value))
		}
	}
	return result
}

func asString(value any) string {
	switch item := value.(type) {
	case string:
		return item
	case json.Number:
		return item.String()
	case float64:
		return strconv.FormatFloat(item, 'f', -1, 64)
	default:
		return ""
	}
}

func asInt(value any) int {
	if number, err := strconv.Atoi(strings.TrimSpace(asString(value))); err == nil {
		return number
	}
	return 0
}

func parseTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

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

func requestHost(r *http.Request) string {
	host := strings.TrimSpace(r.Host)
	if value, _, err := net.SplitHostPort(host); err == nil {
		return value
	}
	return host
}

func isLocalRequest(r *http.Request) bool {
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-WorkMesh-Local")), "1") {
		return true
	}
	return false
}

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

func isPasswordExpirationExempt(r *http.Request) bool {
	path := r.URL.Path
	return strings.HasPrefix(path, "/api/v2/core/auth") || path == "/api/v2/core/settings/search" || path == "/api/v2/core/settings/search/base" || path == "/api/v2/core/auth/expired/reset"
}

func hasSessionCookie(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	return err == nil && strings.TrimSpace(cookie.Value) != ""
}

func hasNonCookieCredential(r *http.Request) bool {
	for _, name := range []string{"Authorization", "X-WorkMesh-Token", "X-API-Key", "X-Panel-Local-Token"} {
		if strings.TrimSpace(r.Header.Get(name)) != "" {
			return true
		}
	}
	return false
}

func unsafeHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

func isAPIRequest(path string) bool { return strings.HasPrefix(path, "/api/v2/") }

func isStaticAPIPath(path string) bool {
	return strings.HasPrefix(path, "/api/v2/images/") || strings.HasPrefix(path, "/api/v2/static/")
}

func isStaticWebPath(path string) bool {
	return path == "/" || path == "/favicon.ico" || strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/public/")
}

func isStaticAssetPath(path string) bool {
	return path == "/favicon.ico" || strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/public/")
}

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

func isSelfAuthenticatedStream(path string) bool {
	switch path {
	case "/api/v2/process/ws", "/api/v2/containers/search/log", "/api/v2/files/wget/process", "/api/v2/hosts/terminal/local", "/api/v2/hosts/terminal/container", "/api/v2/hosts/terminal/ssh":
		return true
	default:
		return false
	}
}

func securityError(code, message string) map[string]any {
	return map[string]any{"code": "ERR", "message": message, "details": map[string]string{"errCode": code}}
}

func writeSecurityError(w http.ResponseWriter, status int, code, message string) {
	wmhttp.JSON(w, status, securityError(code, message))
}
