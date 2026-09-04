// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"strings"

	"github.com/todaybin/workmesh-server/i18n"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// localizedResponseWriter 在响应写出前记录请求语言，流式和 WebSocket 响应不经过该包装。
// 这样既能保持升级连接需要的 Hijacker/Flusher 能力，又能为普通 JSON 错误提供本地化文本。
type localizedResponseWriter struct {
	http.ResponseWriter
	locale string
}

func (w *localizedResponseWriter) Header() http.Header    { return w.ResponseWriter.Header() }
func (w *localizedResponseWriter) WriteHeader(status int) { w.ResponseWriter.WriteHeader(status) }
func (w *localizedResponseWriter) Write(data []byte) (int, error) {
	return w.ResponseWriter.Write(data)
}

// localizeErrorMessage 将稳定错误码映射到内置语言键；未知错误保留带上下文的原文。
func localizeErrorMessage(locale, code, fallback string) string {
	key := map[string]string{
		"INVALID_JSON":                "ErrInvalidParams",
		"ACCOUNT_REQUIRED":            "ErrAgentAccountRequired",
		"ACCOUNT_NOT_FOUND":           "ErrRecordNotFound",
		"ACCOUNT_SAVE_FAILED":         "ErrInternalServer",
		"ACCOUNT_VERIFY_FAILED":       "ErrAgentAccountUnavailable",
		"MODEL_REQUIRED":              "ErrInvalidParams",
		"MODEL_EXISTS":                "ErrRecordExist",
		"MODEL_NOT_FOUND":             "ErrRecordNotFound",
		"MODEL_DISCOVERY_FAILED":      "ErrHttpReqFailed",
		"MCP_SERVER_NOT_FOUND":        "ErrRecordNotFound",
		"MCP_CONNECTION_FAILED":       "ErrHttpReqFailed",
		"AGENT_ID_REQUIRED":           "ErrAgentIDRequired",
		"AGENT_ACCOUNT_ID_REQUIRED":   "ErrAgentAccountIDRequired",
		"AGENT_NOT_FOUND":             "ErrRecordNotFound",
		"AGENT_ROLE_EXISTS":           "ErrRecordExist",
		"AGENT_ROLE_NOT_FOUND":        "ErrRecordNotFound",
		"SESSION_NOT_FOUND":           "ErrRecordNotFound",
		"SESSION_TITLE_REQUIRED":      "ErrInvalidParams",
		"TASK_ID_REQUIRED":            "ErrInvalidParams",
		"SANDBOX_ID_REQUIRED":         "ErrInvalidParams",
		"SANDBOX_NOT_FOUND":           "ErrRecordNotFound",
		"TASK_PROVIDER_UNAVAILABLE":   "ErrInternalServer",
		"CUBESANDBOX_UNAVAILABLE":     "ErrInternalServer",
		"AGENT_RUNTIME_UNAVAILABLE":   "ErrAgentAccountUnavailable",
		"AI_RESOURCE_NOT_FOUND":       "ErrRecordNotFound",
		"OLLAMA_MODEL_NOT_FOUND":      "ErrRecordNotFound",
		"AI_OPERATION_INVALID":        "ErrInvalidParams",
		"PAIRING_PARAMETERS_INVALID":  "ErrInvalidParams",
		"PAIRING_ACCOUNT_INVALID":     "ErrAgentAccountIDRequired",
		"AGENT_TOKEN_FAILED":          "ErrInternalServer",
		"AGENT_SAVE_FAILED":           "ErrInternalServer",
		"AI_CONFIG_SAVE_FAILED":       "ErrInternalServer",
		"AI_RESOURCE_SAVE_FAILED":     "ErrInternalServer",
		"CONNECTION_TEST_UNSUPPORTED": "ErrNotSupportType",
		"SESSION_REQUIRED":            "ErrInvalidParams",
		"SESSION_SAVE_FAILED":         "ErrInternalServer",
		"TASK_AUTH_REQUIRED":          "ErrApiConfigKeyInvalid",
		"INVALID_TASK_REQUEST":        "ErrInvalidParams",
		"TASK_TIMEOUT_INVALID":        "ErrInvalidParams",
		"TASK_CREATE_FAILED":          "ErrInternalServer",
		"TASK_STATE_SAVE_FAILED":      "ErrInternalServer",
		"TASK_OPERATION_NOT_FOUND":    "ErrRecordNotFound",
		"AGENT_PAIRING_FAILED":        "ErrInternalServer",
		"AI_RESOURCE_REQUIRED":        "ErrInvalidParams",
		"MODEL_NAME_REQUIRED":         "ErrInvalidParams",
		"METHOD_NOT_ALLOWED":          "ErrInvalidParams",
	}
	keyName := key[strings.TrimSpace(code)]
	if keyName == "" {
		return fallback
	}
	// 通用模板参数使用原始错误作为 detail，避免丢失资源 ID、上游错误等上下文。
	message, err := i18n.Format(locale, keyName, map[string]any{"detail": fallback, "err": fallback, "name": fallback, "id": fallback})
	if err != nil || strings.TrimSpace(message) == "" {
		return fallback
	}
	return message
}

func writeError(w http.ResponseWriter, status int, err error) {
	message := "请求失败"
	if err != nil {
		message = err.Error()
	}
	code := i18n.ErrorCode(status, message)
	wmhttp.JSON(w, status, map[string]any{
		"code":    "ERR",
		"details": map[string]string{"errCode": code},
		"message": message,
	})
}

// notImplementedError 统一返回明确的未接入错误，避免 handler 用固定成功值掩盖能力缺失。
func notImplementedError(w http.ResponseWriter, message string) {
	runtimeErr(w, http.StatusNotImplemented, message)
}
