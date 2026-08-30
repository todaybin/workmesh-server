// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/i18n"
)

// Server 是控制面与节点执行面共享的 HTTP 生命周期封装。
type Server struct {
	server *http.Server
}

// New 创建统一 HTTP Server；业务模块通过 Register 注册路由。
func New(addr string, handler http.Handler, timeout time.Duration) *Server {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Server{server: &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: timeout, ReadTimeout: timeout, WriteTimeout: timeout, IdleTimeout: 90 * time.Second}}
}

// ListenAndServe 启动服务。
func (s *Server) ListenAndServe() error { return s.server.ListenAndServe() }

// Shutdown 在给定上下文截止前优雅停止服务。
func (s *Server) Shutdown(ctx context.Context) error { return s.server.Shutdown(ctx) }

// JSON 将统一响应编码为 JSON。
func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// 统一处理错误语言和内部状态标识，避免把实现阶段术语暴露给客户端。
	_ = json.NewEncoder(w).Encode(sanitizeResponse(localizeResponse(value, status, LocaleOf(w))))
}

// localeResponseWriter 为普通 HTTP 响应携带请求语言；流式和 WebSocket 处理器可继续使用原始 writer。
// 该包装器只增加语言元数据，不缓存响应体，也不会改变连接生命周期。
type localeResponseWriter struct {
	http.ResponseWriter
	locale string
}

// WithLocale 返回带语言元数据的响应写入器。
func WithLocale(w http.ResponseWriter, locale string) http.ResponseWriter {
	if w == nil {
		return nil
	}
	if existing, ok := w.(*localeResponseWriter); ok {
		if strings.TrimSpace(locale) != "" {
			existing.locale = i18n.NormalizeLocale(locale)
		}
		return existing
	}
	return &localeResponseWriter{ResponseWriter: w, locale: i18n.NormalizeLocale(locale)}
}

// LocaleOf 读取响应写入器绑定的语言；未绑定时返回默认中文。
func LocaleOf(w http.ResponseWriter) string {
	if localized, ok := w.(interface{ ResponseLocale() string }); ok {
		return localized.ResponseLocale()
	}
	return i18n.NormalizeLocale("")
}

// ResponseLocale 返回当前响应的语言代码。
func (w *localeResponseWriter) ResponseLocale() string { return w.locale }

func localizeResponse(value any, status int, locale string) any {
	item, ok := value.(map[string]any)
	if !ok || strings.TrimSpace(fmtString(item["code"])) != "ERR" {
		return value
	}
	result := make(map[string]any, len(item)+1)
	for key, child := range item {
		result[key] = child
	}
	code := ""
	if details, ok := item["details"].(map[string]string); ok {
		code = strings.TrimSpace(details["errCode"])
	}
	if details, ok := item["details"].(map[string]any); ok && code == "" {
		code = strings.TrimSpace(fmtString(details["errCode"]))
	}
	fallback := strings.TrimSpace(fmtString(item["message"]))
	if code == "" {
		code = i18n.ErrorCode(status, fallback)
		result["details"] = map[string]string{"errCode": code}
	}
	if fallback != "" {
		result["message"] = i18n.LocalizeError(locale, code, fallback)
	}
	return result
}

func fmtString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

// sanitizeResponse 将内部状态名称转换为稳定的公开错误码；复制容器，避免修改调用方持有的数据。
func sanitizeResponse(value any) any {
	switch item := value.(type) {
	case string:
		if item == "MIGRATION_PENDING" {
			return "FEATURE_UNAVAILABLE"
		}
		if strings.Contains(item, "该接口正在迁移") {
			return "该功能暂不可用"
		}
		return item
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, child := range item {
			result[key] = sanitizeResponse(child)
		}
		return result
	case map[string]string:
		result := make(map[string]string, len(item))
		for key, child := range item {
			sanitized, _ := sanitizeResponse(child).(string)
			result[key] = sanitized
		}
		return result
	case []any:
		result := make([]any, len(item))
		for index, child := range item {
			result[index] = sanitizeResponse(child)
		}
		return result
	case []string:
		result := make([]string, len(item))
		for index, child := range item {
			result[index], _ = sanitizeResponse(child).(string)
		}
		return result
	default:
		return value
	}
}
