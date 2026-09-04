// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package gateway

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
	BaseURL   string
	GatewayID string
	// NodeID 是注册后用于状态查询和心跳关联的节点标识。
	NodeID      string
	Secret      []byte
	HTTP        *http.Client
	Timeout     time.Duration
	AccessToken string
	privateKey  ed25519.PrivateKey
	publicKey   ed25519.PublicKey
}

// NewHTTPClient 创建 Gateway 客户端；BaseURL 必须为 https 地址（本地测试可使用 http）。
func NewHTTPClient(baseURL, gatewayID, secret string) *HTTPClient {
	seed := sha256.Sum256([]byte("workmesh-node:" + gatewayID + ":" + secret))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	return &HTTPClient{BaseURL: strings.TrimRight(baseURL, "/"), GatewayID: gatewayID, Secret: []byte(secret), HTTP: &http.Client{Timeout: 15 * time.Second}, Timeout: 15 * time.Second, privateKey: privateKey, publicKey: privateKey.Public().(ed25519.PublicKey)}
}

// NewHTTPClientWithIdentity 创建使用持久化节点身份的 Gateway 客户端。
// identity 为空时回退到兼容密钥，仅用于没有数据目录的开发环境。
func NewHTTPClientWithIdentity(baseURL, gatewayID, secret string, identity *Identity) *HTTPClient {
	client := NewHTTPClient(baseURL, gatewayID, secret)
	if identity != nil && len(identity.PrivateKey) == ed25519.PrivateKeySize && len(identity.PublicKey) == ed25519.PublicKeySize {
		client.privateKey = append(ed25519.PrivateKey(nil), identity.PrivateKey...)
		client.publicKey = append(ed25519.PublicKey(nil), identity.PublicKey...)
	}
	return client
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
		BindingID   string   `json:"bindingId"`
		NodeID      string   `json:"nodeId"`
		ID          string   `json:"id"`
		AccessToken string   `json:"token"`
		ExpiresAt   string   `json:"expiresAt"`
		Refreshable bool     `json:"refreshable"`
		Scopes      []string `json:"scopes"`
		Item        struct {
			BindingID string `json:"bindingId"`
			NodeID    string `json:"nodeId"`
			ID        string `json:"id"`
		} `json:"item"`
	}
	c.NodeID = strings.TrimSpace(request.NodeID)
	request.PublicKey = base64.RawStdEncoding.EncodeToString(c.publicKey)
	err := c.do(ctx, http.MethodPost, "/workmesh/node/register", request, &response)
	if err != nil {
		return Authorization{}, err
	}
	if response.BindingID == "" {
		response.BindingID = response.Item.BindingID
	}
	if response.BindingID == "" {
		response.BindingID = response.Item.NodeID
	}
	if response.BindingID == "" {
		// 第三方网关节点视图使用 item.id 承载稳定 runner_key。
		response.BindingID = response.Item.ID
	}
	if response.BindingID == "" {
		response.BindingID = response.NodeID
	}
	if response.BindingID == "" {
		response.BindingID = response.ID
	}
	if response.BindingID == "" {
		return Authorization{}, errors.New("Gateway 注册响应缺少绑定标识")
	}
	if response.AccessToken != "" {
		c.AccessToken = response.AccessToken
	}
	return Authorization{BindingID: response.BindingID, AccessToken: c.AccessToken, Scopes: response.Scopes, ExpiresAt: response.ExpiresAt, Refreshable: response.Refreshable || c.AccessToken != ""}, nil
}

// Heartbeat 上报节点在线状态并保持 Gateway 授权有效。
func (c *HTTPClient) Heartbeat(ctx context.Context, registration Registration) error {
	return c.do(ctx, http.MethodPost, "/workmesh/node/heartbeat", map[string]any{
		"nodeId":       registration.NodeID,
		"bindingId":    registration.BindingID,
		"role":         registration.Role,
		"capabilities": registration.Capabilities,
		"status":       "online",
		"sentAt":       time.Now().UTC().Format(time.RFC3339),
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
	for _, item := range response.Items {
		if c.NodeID != "" && item.NodeID != c.NodeID {
			continue
		}
		status.Registration = RegistrationRegistered
		status.NodeID = item.NodeID
		status.Connected = item.Status == "online" || item.Status == "ready" || item.Status == "busy"
		break
	}
	return status, nil
}

// Refresh 刷新节点云端授权。
func (c *HTTPClient) Refresh(ctx context.Context) (Authorization, error) {
	var response struct {
		Token       string   `json:"token"`
		ExpiresAt   string   `json:"expiresAt"`
		ExpiresIn   int64    `json:"expiresIn"`
		BindingID   string   `json:"bindingId"`
		Scopes      []string `json:"scopes"`
		Refreshable bool     `json:"refreshable"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/workmesh/v2/nodes/authorization/refresh", nil, &response); err != nil {
		return Authorization{}, err
	}
	if response.Token != "" {
		c.AccessToken = response.Token
	}
	if response.ExpiresAt == "" && response.ExpiresIn > 0 {
		response.ExpiresAt = time.Now().UTC().Add(time.Duration(response.ExpiresIn) * time.Second).Format(time.RFC3339)
	}
	return Authorization{BindingID: response.BindingID, Scopes: response.Scopes, ExpiresAt: response.ExpiresAt, Refreshable: response.Refreshable || c.AccessToken != "", AccessToken: c.AccessToken}, nil
}

// Revoke 撤销当前节点授权。
func (c *HTTPClient) Revoke(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/workmesh/v2/nodes/authorization/revoke", nil, nil)
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
	request.Header.Set("X-WorkMesh-Protocol-Version", "v2")
	request.Header.Set("X-WorkMesh-Request-Id", randomID())
	now := time.Now().UTC()
	protocolTimestamp := fmt.Sprintf("%d", now.Unix())
	// Gateway 通用 HMAC 协议使用 Unix 秒；Runner Ed25519 验签使用 RFC3339，二者不能复用同一时间字符串。
	runnerTimestamp := now.Format(time.RFC3339)
	request.Header.Set("X-WorkMesh-Timestamp", protocolTimestamp)
	nonce := randomID()
	request.Header.Set("X-WorkMesh-Nonce", nonce)
	request.Header.Set("X-WorkMesh-Gateway-Id", c.GatewayID)
	request.Header.Set("X-Timestamp", runnerTimestamp)
	request.Header.Set("X-Nonce", nonce)
	if len(c.Secret) > 0 {
		mac := hmac.New(sha256.New, c.Secret)
		_, _ = mac.Write([]byte(method + "\n" + endpoint + "\n" + protocolTimestamp + "\n" + nonce + "\n" + string(body)))
		request.Header.Set("X-WorkMesh-Signature", hex.EncodeToString(mac.Sum(nil)))
	}
	if len(c.privateKey) == ed25519.PrivateKeySize {
		bodyHash := sha256.Sum256(body)
		message := strings.Join([]string{strings.ToUpper(method), endpoint, hex.EncodeToString(bodyHash[:]), runnerTimestamp, nonce}, "\n")
		signature := base64.RawStdEncoding.EncodeToString(ed25519.Sign(c.privateKey, []byte(message)))
		request.Header.Set("X-Signature", signature)
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
