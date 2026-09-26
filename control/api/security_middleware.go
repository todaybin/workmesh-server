// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"net/http"
	"strings"
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
// 设置优先由统一 SQLite 状态提供，旧 domains.json 仅作为迁移兼容来源。
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
	// SelfAuthenticated 只允许精确路由绕过浏览器 Session；对应处理器必须自行验证服务凭据。
	SelfAuthenticated func(*http.Request) bool
	// Settings 返回统一 SQLite 中的安全设置；为空时兼容读取旧 domains.json。
	Settings func() map[string]any
	// OperationLog 在写请求完成后接收统一审计元数据；返回值不影响原始响应。
	OperationLog func(*http.Request, int, []byte, time.Duration)
}

type operationResponseWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

// ResponseLocale 返回底层响应写入器提供的请求语言。
func (w *operationResponseWriter) ResponseLocale() string {
	if localized, ok := w.ResponseWriter.(interface{ ResponseLocale() string }); ok {
		return localized.ResponseLocale()
	}
	return ""
}

// WriteHeader 记录并写出 HTTP 状态码，避免重复写入响应头。
func (w *operationResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Write 缓存有限大小的响应正文后将数据转发给底层写入器。
func (w *operationResponseWriter) Write(payload []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if w.body.Len() < 1<<20 {
		remaining := (1 << 20) - w.body.Len()
		if len(payload) > remaining {
			_, _ = w.body.Write(payload[:remaining])
		} else {
			_, _ = w.body.Write(payload)
		}
	}
	return w.ResponseWriter.Write(payload)
}

// NewSecurityMiddleware 将 Session、CSRF、域名绑定和密码过期策略统一挂载到 HTTP 链路。
// 健康检查、登录初始化、静态资源和流式接口由各自处理器负责鉴权，不会被重复拦截。
func NewSecurityMiddleware(next http.Handler, options SecurityMiddlewareOptions) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	provider := newSecuritySettingsProvider(options.DataDir, options.Settings)
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
		started := time.Now()
		serviceRequest := options.SelfAuthenticated != nil && options.SelfAuthenticated(r)
		logRequest := options.OperationLog != nil && unsafeHTTPMethod(r.Method) && !strings.Contains(strings.ToLower(r.URL.Path), "/search") && !isSelfAuthenticatedStream(r.URL.Path) && !serviceRequest
		var operationWriter *operationResponseWriter
		if logRequest {
			operationWriter = &operationResponseWriter{ResponseWriter: responseWriter}
			responseWriter = operationWriter
		}
		defer func() {
			if logRequest && operationWriter != nil {
				status := operationWriter.status
				if status == 0 {
					status = http.StatusOK
				}
				options.OperationLog(r, status, operationWriter.body.Bytes(), time.Since(started))
			}
		}()
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
		if serviceRequest {
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
