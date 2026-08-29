// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package gateway

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
	"strings"
	"time"
)

// HTTPClient 通过 HTTPS 调用 Gateway 注册、授权和心跳接口。
type HTTPClient struct {
	BaseURL     string
	GatewayID   string
	Secret      []byte
	HTTP        *http.Client
	Timeout     time.Duration
	AccessToken string
}

// NewHTTPClient 创建 Gateway 客户端；BaseURL 必须为 https 地址（本地测试可使用 http）。
func NewHTTPClient(baseURL, gatewayID, secret string) *HTTPClient {
	return &HTTPClient{BaseURL: strings.TrimRight(baseURL, "/"), GatewayID: gatewayID, Secret: []byte(secret), HTTP: &http.Client{}, Timeout: 15 * time.Second}
}

// Login 使用 Gateway 账号换取节点授权摘要。
func (c *HTTPClient) Login(ctx context.Context, request LoginRequest) (Authorization, error) {
	var response struct {
		Token     string `json:"token"`
		ExpiresIn int64  `json:"expiresIn"`
	}
	err := c.do(ctx, http.MethodPost, "/workmesh/auth/login", request, &response)
	if err != nil {
		return Authorization{}, err
	}
	c.AccessToken = response.Token
	auth := Authorization{AccessToken: response.Token, Refreshable: response.Token != ""}
	if response.ExpiresIn > 0 {
		auth.ExpiresAt = time.Now().UTC().Add(time.Duration(response.ExpiresIn) * time.Second).Format(time.RFC3339)
	}
	return auth, nil
}

// Register 将当前节点独立注册到 Gateway。
func (c *HTTPClient) Register(ctx context.Context, request RegisterRequest) (Authorization, error) {
	var response struct {
		BindingID string `json:"bindingId"`
	}
	err := c.do(ctx, http.MethodPost, "/workmesh/node/register", request, &response)
	if err != nil {
		return Authorization{}, err
	}
	if response.BindingID == "" {
		response.BindingID = request.NodeID
	}
	return Authorization{BindingID: response.BindingID, AccessToken: c.AccessToken, Refreshable: true}, nil
}

// Heartbeat 上报节点在线状态并保持 Gateway 授权有效。
func (c *HTTPClient) Heartbeat(ctx context.Context, registration Registration) error {
	return c.do(ctx, http.MethodPost, "/workmesh/node/heartbeat", map[string]any{
		"nodeId": registration.NodeID,
		"status": "online",
		"sentAt": time.Now().UTC().Format(time.RFC3339),
	}, nil)
}

// Status 查询节点在 Gateway 的注册和连接状态。
func (c *HTTPClient) Status(ctx context.Context) (Status, error) {
	var response struct {
		Items []struct {
			NodeID string `json:"nodeId"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/workmesh/node/index", nil, &response); err != nil {
		return Status{}, err
	}
	status := Status{Registration: RegistrationUnregistered, GatewayID: c.GatewayID}
	if len(response.Items) > 0 {
		status.Registration = RegistrationRegistered
		status.NodeID = response.Items[0].NodeID
		status.Connected = response.Items[0].Status == "online" || response.Items[0].Status == "ready"
	}
	return status, nil
}

// Refresh 刷新节点云端授权。
func (c *HTTPClient) Refresh(ctx context.Context) (Authorization, error) {
	var response Authorization
	err := c.do(ctx, http.MethodPost, "/api/workmesh/v1/nodes/authorization/refresh", nil, &response)
	return response, err
}

// Revoke 撤销当前节点授权。
func (c *HTTPClient) Revoke(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/workmesh/v1/nodes/authorization/revoke", nil, nil)
}

func (c *HTTPClient) do(ctx context.Context, method, endpoint string, input, output any) error {
	if c.BaseURL == "" {
		return errors.New("Gateway 地址未配置")
	}
	var body []byte
	var err error
	if input != nil {
		body, err = json.Marshal(input)
		if err != nil {
			return err
		}
	}
	requestCtx := ctx
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	request, err := http.NewRequestWithContext(requestCtx, method, c.BaseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-WorkMesh-Protocol-Version", "v1")
	request.Header.Set("X-WorkMesh-Request-Id", randomID())
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	request.Header.Set("X-WorkMesh-Timestamp", timestamp)
	nonce := randomID()
	request.Header.Set("X-WorkMesh-Nonce", nonce)
	request.Header.Set("X-WorkMesh-Gateway-Id", c.GatewayID)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	if len(c.Secret) > 0 {
		mac := hmac.New(sha256.New, c.Secret)
		_, _ = mac.Write([]byte(method + "\n" + endpoint + "\n" + timestamp + "\n" + nonce + "\n" + string(body)))
		request.Header.Set("X-WorkMesh-Signature", hex.EncodeToString(mac.Sum(nil)))
		request.Header.Set("X-Signature", hex.EncodeToString(mac.Sum(nil)))
	}
	if c.AccessToken != "" {
		request.Header.Set("Authorization", "Bearer "+c.AccessToken)
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Gateway 返回 HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	if output == nil || len(data) == 0 {
		return nil
	}
	// Gateway 统一 envelope 的 data 字段承载业务对象；兼容直接返回对象的测试服务。
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &envelope) == nil && len(envelope.Data) > 0 {
		if envelope.Code != 0 && envelope.Code != http.StatusOK {
			return fmt.Errorf("Gateway 业务错误码: %d", envelope.Code)
		}
		return json.Unmarshal(envelope.Data, output)
	}
	return json.Unmarshal(data, output)
}

func randomID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}

var _ ProtocolClient = (*HTTPClient)(nil)
