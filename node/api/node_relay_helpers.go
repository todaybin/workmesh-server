// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"crypto/hmac"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// relayTarget 从请求头和查询参数解析目标节点，并统一去除空白。
func relayTarget(req *http.Request) string {
	if value := strings.TrimSpace(req.URL.Query().Get("operateNode")); value != "" {
		if decoded, err := url.QueryUnescape(value); err == nil {
			return strings.TrimSpace(decoded)
		}
	}
	value := strings.TrimSpace(req.Header.Get("CurrentNode"))
	if decoded, err := url.QueryUnescape(value); err == nil {
		value = decoded
	}
	return value
}

// relayBaseURL 校验节点地址协议和主机，拒绝凭据、查询和片段注入。
func relayBaseURL(node relayNode) (string, error) {
	value := strings.TrimSpace(node.Endpoint)
	if value == "" {
		value = strings.TrimSpace(node.Addr)
	}
	parsed, err := url.Parse(strings.TrimRight(value, "/"))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("目标节点地址必须是无凭据 HTTP(S) URL")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// pathWithQuery 组合经过 URL 编码的路径和查询参数，供签名与转发共同使用。
func pathWithQuery(path string, query url.Values) string {
	if encoded := query.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

// copyRelayHeaders 复制允许透传的响应头，过滤 hop-by-hop 和内部控制头。
func copyRelayHeaders(dst, src http.Header) {
	for key, values := range src {
		lower := strings.ToLower(key)
		if lower == "host" || strings.HasPrefix(lower, "x-workmesh-") || isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

// isHopByHopHeader 判断连接级头部，避免把上游连接控制泄露到下游。
func isHopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

// secureRelaySignature 使用常量时间比较 HMAC，避免签名校验侧信道。
func secureRelaySignature(got, expected string) bool {
	got = strings.TrimSpace(got)
	return hmac.Equal([]byte(got), []byte(expected))
}

// randomRelayNonce 生成透传请求 nonce，供接收端防止短期重放。
func randomRelayNonce() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
}

// writeRelayError 使用统一错误 envelope 返回透传失败原因。
func writeRelayError(w http.ResponseWriter, status int, message string) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": message, "details": map[string]any{"errCode": "NODE_RELAY_FAILED"}})
}
