// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package link

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTimeout    = 10 * time.Second
	defaultRetries    = 2
	defaultRetryDelay = 100 * time.Millisecond
	maxResponseBytes  = 8 << 20
)

// HTTPClient 通过 HTTPS 调用另一台 WorkMesh 节点。
// 每次尝试都会生成新的时间戳和 nonce，防止重试请求复用已消费的签名。
type HTTPClient struct {
	BaseURL      string
	NodeID       string
	Secret       []byte
	HTTP         HTTPDoer
	Timeout      time.Duration
	MaxRetries   int
	RetryBackoff time.Duration
	Clock        Clock
	Nonce        NonceGenerator
}

// NewHTTPClient 创建使用默认超时和重试策略的节点客户端。
func NewHTTPClient(baseURL, nodeID, secret string) *HTTPClient {
	return NewHTTPClientWithOptions(baseURL, HTTPClientOptions{
		NodeID:       nodeID,
		Secret:       []byte(secret),
		MaxRetries:   defaultRetries,
		RetryBackoff: defaultRetryDelay,
	})
}

// NewHTTPClientWithOptions 创建可注入 HTTP 传输、时钟和 nonce 生成器的客户端。
func NewHTTPClientWithOptions(baseURL string, options HTTPClientOptions) *HTTPClient {
	client := &HTTPClient{
		BaseURL:      strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		NodeID:       options.NodeID,
		Secret:       append([]byte(nil), options.Secret...),
		HTTP:         options.HTTP,
		Timeout:      options.Timeout,
		MaxRetries:   options.MaxRetries,
		RetryBackoff: options.RetryBackoff,
		Clock:        options.Clock,
		Nonce:        options.Nonce,
	}
	if client.HTTP == nil {
		client.HTTP = &http.Client{}
	}
	if client.Timeout <= 0 {
		client.Timeout = defaultTimeout
	}
	if client.MaxRetries < 0 {
		client.MaxRetries = 0
	}
	if client.RetryBackoff <= 0 {
		client.RetryBackoff = defaultRetryDelay
	}
	if client.Clock == nil {
		client.Clock = time.Now
	}
	if client.Nonce == nil {
		client.Nonce = randomNonce
	}
	return client
}

// Handshake 向远端发送本节点身份和能力，并返回远端身份。
func (c *HTTPClient) Handshake(ctx context.Context, request Handshake) (Handshake, error) {
	var response Handshake
	if err := c.doJSON(ctx, http.MethodPost, "/api/v2/link/handshake", request, &response); err != nil {
		return Handshake{}, err
	}
	return response, nil
}

// Heartbeat 更新远端节点的在线状态。
func (c *HTTPClient) Heartbeat(ctx context.Context, request Heartbeat) error {
	return c.doJSON(ctx, http.MethodPost, "/api/v2/link/heartbeat", request, nil)
}

// Pull 拉取指定同步流中游标之后的数据。
func (c *HTTPClient) Pull(ctx context.Context, cursor SyncCursor) ([]byte, SyncCursor, error) {
	var response struct {
		Payload []byte     `json:"payload"`
		Cursor  SyncCursor `json:"cursor"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v2/link/sync/pull", cursor, &response); err != nil {
		return nil, SyncCursor{}, err
	}
	return response.Payload, response.Cursor, nil
}

// Push 将一批控制面数据写入远端同步流，并返回远端最新游标。
func (c *HTTPClient) Push(ctx context.Context, cursor SyncCursor, payload []byte) (SyncCursor, error) {
	request := struct {
		Stream  string `json:"stream"`
		Version uint64 `json:"version"`
		Payload []byte `json:"payload"`
	}{Stream: cursor.Stream, Version: cursor.Version, Payload: payload}
	var response struct {
		Cursor SyncCursor `json:"cursor"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v2/link/sync/push", request, &response); err != nil {
		return SyncCursor{}, err
	}
	return response.Cursor, nil
}

// SignaturePayload 返回签名所覆盖的规范化内容，服务端和客户端必须使用相同格式。
func SignaturePayload(method, path, timestamp, nonce string, body []byte) []byte {
	return []byte(method + "\n" + path + "\n" + timestamp + "\n" + nonce + "\n" + string(body))
}

// Sign 使用 HMAC-SHA256 对链路请求签名，返回小写十六进制摘要。
func Sign(secret []byte, method, path, timestamp, nonce string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(SignaturePayload(method, path, timestamp, nonce, body))
	return hex.EncodeToString(mac.Sum(nil))
}

func (c *HTTPClient) doJSON(ctx context.Context, method, endpoint string, input, output any) error {
	if c.BaseURL == "" {
		return errors.New("节点地址未配置")
	}
	var body []byte
	var err error
	if input != nil {
		body, err = json.Marshal(input)
		if err != nil {
			return fmt.Errorf("编码链路请求: %w", err)
		}
	}
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		data, status, requestErr := c.doAttempt(ctx, method, endpoint, body)
		if requestErr == nil && status >= 200 && status < 300 {
			if output == nil || len(data) == 0 {
				return nil
			}
			if err := decodeEnvelope(data, output); err != nil {
				return fmt.Errorf("解析链路响应: %w", err)
			}
			return nil
		}
		if requestErr != nil {
			lastErr = requestErr
		} else {
			lastErr = fmt.Errorf("节点返回 HTTP %d: %s", status, strings.TrimSpace(string(data)))
		}
		if !retryable(ctx, status, requestErr) || attempt == c.MaxRetries {
			return lastErr
		}
		if err := waitRetry(ctx, c.RetryBackoff, attempt); err != nil {
			return err
		}
	}
	return lastErr
}

func (c *HTTPClient) doAttempt(ctx context.Context, method, endpoint string, body []byte) ([]byte, int, error) {
	requestCtx := ctx
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	path := endpoint
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	nonce, err := c.Nonce()
	if err != nil {
		return nil, 0, fmt.Errorf("生成链路 nonce: %w", err)
	}
	timestamp := strconv.FormatInt(c.Clock().Unix(), 10)
	request, err := http.NewRequestWithContext(requestCtx, method, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("创建链路请求: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(HeaderNodeID, c.NodeID)
	request.Header.Set(HeaderTimestamp, timestamp)
	request.Header.Set(HeaderNonce, nonce)
	if len(c.Secret) > 0 {
		request.Header.Set(HeaderSignature, Sign(c.Secret, method, path, timestamp, nonce, body))
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, 0, err
	}
	if response == nil {
		return nil, 0, errors.New("节点传输返回空响应")
	}
	if response.Body == nil {
		return nil, response.StatusCode, nil
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, response.StatusCode, fmt.Errorf("读取链路响应: %w", err)
	}
	if len(data) > maxResponseBytes {
		return nil, response.StatusCode, errors.New("链路响应超过 8 MiB 限制")
	}
	return data, response.StatusCode, nil
}

func decodeEnvelope(data []byte, output any) error {
	var envelope struct {
		Code    json.RawMessage `json:"code"`
		Data    json.RawMessage `json:"data"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && len(envelope.Data) > 0 {
		if code := strings.Trim(string(envelope.Code), `"`); code != "" && code != "200" {
			if envelope.Message != "" {
				return errors.New(envelope.Message)
			}
			return fmt.Errorf("业务错误码 %s", code)
		}
		return json.Unmarshal(envelope.Data, output)
	}
	return json.Unmarshal(data, output)
}

func retryable(ctx context.Context, status int, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	if err != nil {
		return true
	}
	return status == http.StatusRequestTimeout || status == http.StatusTooEarly || status == http.StatusTooManyRequests || status >= 500
}

func waitRetry(ctx context.Context, delay time.Duration, attempt int) error {
	if attempt > 0 {
		delay *= time.Duration(1 << min(attempt, 6))
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func randomNonce() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

var _ Client = (*HTTPClient)(nil)
