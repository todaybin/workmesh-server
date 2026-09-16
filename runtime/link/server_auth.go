// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package link

import (
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// authenticate 校验链路签名、时间窗口和 nonce，阻止伪造及重放请求。
func (s *Server) authenticate(r *http.Request, body []byte) error {
	if len(s.options.Secret) == 0 {
		return nil
	}
	timestampText := firstHeader(r, HeaderTimestamp, "X-Timestamp")
	nonce := firstHeader(r, HeaderNonce, "X-Nonce")
	signature := firstHeader(r, HeaderSignature, "X-Signature")
	if timestampText == "" || nonce == "" || signature == "" {
		return errors.New("链路签名请求头不完整")
	}
	timestamp, err := parseTimestamp(timestampText)
	if err != nil {
		return errors.New("链路时间戳无效")
	}
	now := s.clock()
	if now.Sub(timestamp) > s.options.ClockSkew || timestamp.Sub(now) > s.options.ClockSkew {
		return errors.New("链路请求已过期")
	}
	expected := Sign(s.options.Secret, r.Method, r.URL.RequestURI(), timestampText, nonce, body)
	if !secureEqualSignature(signature, expected) {
		return errors.New("链路签名无效")
	}
	nodeID := firstHeader(r, HeaderNodeID, "X-Node-ID")
	if nodeID == "" {
		return errors.New("链路节点身份缺失")
	}
	key := nodeID + ":" + nonce
	s.mu.Lock()
	defer s.mu.Unlock()
	for oldKey, expires := range s.nonces {
		if !expires.After(now) {
			delete(s.nonces, oldKey)
		}
	}
	if expires, exists := s.nonces[key]; exists && expires.After(now) {
		return errors.New("链路 nonce 已使用")
	}
	s.nonces[key] = now.Add(s.options.NonceTTL)
	return nil
}

// parseTimestamp 解析 Unix 秒数或 RFC3339 时间戳，兼容两类链路客户端。
func parseTimestamp(value string) (time.Time, error) {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err == nil {
		return time.Unix(seconds, 0), nil
	}
	return time.Parse(time.RFC3339, value)
}

// secureEqualSignature 兼容十六进制和 Base64 签名，并使用恒定时间比较。
func secureEqualSignature(got, expected string) bool {
	got = strings.TrimSpace(got)
	if decoded, err := hex.DecodeString(got); err == nil {
		want, _ := hex.DecodeString(expected)
		return hmac.Equal(decoded, want)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(got)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(got)
	}
	if err != nil {
		return false
	}
	want, _ := hex.DecodeString(expected)
	return hmac.Equal(decoded, want)
}

// readBody 按统一大小上限读取 HTTP 请求体，避免链路请求耗尽进程内存。
func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, errors.New("请求体不能为空")
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRequestBytes {
		return nil, errors.New("请求体超过 8 MiB 限制")
	}
	return data, nil
}

// firstHeader 按兼容顺序读取第一个非空请求头。
func firstHeader(r *http.Request, names ...string) string {
	for _, name := range names {
		if value := r.Header.Get(name); value != "" {
			return value
		}
	}
	return ""
}

// authStatus 将认证错误映射为 HTTP 状态码，nonce 重放使用冲突状态。
func authStatus(err error) int {
	if strings.Contains(err.Error(), "nonce") {
		return http.StatusConflict
	}
	return http.StatusUnauthorized
}

// syncStatus 将同步存储错误映射为冲突或请求错误状态。
func syncStatus(err error) int {
	if errors.Is(err, ErrSyncConflict) {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}

// writeLinkJSON 使用统一 HTTP 响应编码器返回链路结果。
func writeLinkJSON(w http.ResponseWriter, status int, value any) {
	wmhttp.JSON(w, status, value)
}

// writeLinkError 返回保持统一 envelope 的链路错误响应。
func writeLinkError(w http.ResponseWriter, status int, err error) {
	message := "链路请求失败"
	if err != nil {
		message = err.Error()
	}
	writeLinkJSON(w, status, map[string]any{"code": "ERR", "message": message})
}

// newResponseNonce 生成握手响应使用的随机 nonce，随机源失败时回退到时间值。
func newResponseNonce() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(raw[:])
}
