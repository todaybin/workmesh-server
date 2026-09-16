// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// mcpProbeTarget 保存一次 MCP 连通性探测已经校验过的协议参数。
type mcpProbeTarget struct {
	endpoint  string
	transport string
	protocol  string
}

// testMCPConnection 使用保存的 MCP 配置或当前请求参数执行一次真实网络探测。
// testMCPConnection 使用真实 HTTP 请求验证 MCP 服务端点和传输协议。
func testMCPConnection(s *executionState, body map[string]any) (map[string]any, error) {
	server, err := resolveMCPProbeServer(s, body)
	if err != nil {
		return nil, err
	}
	target, err := parseMCPProbeTarget(server)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := newMCPProbeRequest(ctx, target)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 MCP 服务失败: %w", err)
	}
	defer resp.Body.Close()
	if err := validateMCPProbeResponse(resp, target.transport); err != nil {
		return nil, err
	}
	return map[string]any{"success": true, "endpoint": target.endpoint, "outputTransport": target.transport, "protocolVersion": target.protocol, "message": "连接成功"}, nil
}

// resolveMCPProbeServer 优先按资源 ID 读取持久化配置，未指定 ID 时使用请求中的临时配置。
func resolveMCPProbeServer(s *executionState, body map[string]any) (map[string]any, error) {
	id := aiID(body, "id", "serverId")
	if id == "" {
		return body, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.data.MCP {
		if aiID(item, "id", "serverId") == id {
			return cloneMap(item), nil
		}
	}
	return nil, errors.New("MCP 服务不存在")
}

// parseMCPProbeTarget 校验地址、传输类型和协议版本，并生成最终探测端点。
func parseMCPProbeTarget(server map[string]any) (mcpProbeTarget, error) {
	base := strings.TrimRight(aiString(server, "baseUrl", "baseURL", "url"), "/")
	if base == "" {
		return mcpProbeTarget{}, errors.New("MCP 服务地址不能为空")
	}
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return mcpProbeTarget{}, errors.New("MCP 服务地址必须是无凭据的 HTTP(S) 地址")
	}
	if mcpURLContainsCredential(parsed) {
		return mcpProbeTarget{}, errors.New("MCP 服务地址不得在查询参数中携带凭据")
	}
	transport := strings.ToLower(aiString(server, "outputTransport", "transport"))
	if transport == "" {
		transport = "streamablehttp"
	}
	if transport != "sse" && transport != "streamablehttp" && transport != "streamable-http" {
		return mcpProbeTarget{}, fmt.Errorf("MCP 传输类型不受支持: %s", transport)
	}
	if transport == "streamable-http" {
		transport = "streamablehttp"
	}
	endpoint := mcpProbeEndpoint(base, transport, server)
	parsed, err = url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return mcpProbeTarget{}, errors.New("MCP 服务端点地址无效")
	}
	protocol := aiString(server, "protocolVersion")
	if protocol == "" {
		protocol = "2025-06-18"
	}
	return mcpProbeTarget{endpoint: endpoint, transport: transport, protocol: protocol}, nil
}

// mcpURLContainsCredential 拒绝在查询参数名称中携带常见凭据，避免探测错误泄露密钥。
func mcpURLContainsCredential(parsed *url.URL) bool {
	for key := range parsed.Query() {
		name := strings.ToLower(key)
		if strings.Contains(name, "token") || strings.Contains(name, "secret") || strings.Contains(name, "password") || strings.Contains(name, "apikey") || strings.Contains(name, "api_key") {
			return true
		}
	}
	return false
}

// mcpProbeEndpoint 按传输类型拼接对应路径，空路径继续探测基础地址。
func mcpProbeEndpoint(base, transport string, server map[string]any) string {
	suffix := strings.TrimSpace(aiString(server, "streamableHttpPath", "streamableHTTPPath"))
	if transport == "sse" {
		suffix = strings.TrimSpace(aiString(server, "ssePath"))
	}
	if suffix == "" {
		return base
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(suffix, "/")
}

// newMCPProbeRequest 构造 SSE 握手或 Streamable HTTP initialize 请求。
func newMCPProbeRequest(ctx context.Context, target mcpProbeTarget) (*http.Request, error) {
	var req *http.Request
	var err error
	if target.transport == "sse" {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, target.endpoint, nil)
		if err == nil {
			req.Header.Set("Accept", "text/event-stream")
		}
	} else {
		payload := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": target.protocol, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "workmesh-server", "version": "1.0.0"}}}
		var raw []byte
		raw, err = json.Marshal(payload)
		if err == nil {
			req, err = http.NewRequestWithContext(ctx, http.MethodPost, target.endpoint, strings.NewReader(string(raw)))
		}
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
		}
	}
	if err != nil {
		return nil, err
	}
	return req, nil
}

// validateMCPProbeResponse 检查状态码及 SSE 内容类型，并限制错误响应的读取大小。
func validateMCPProbeResponse(resp *http.Response, transport string) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		if strings.TrimSpace(string(message)) == "" {
			return fmt.Errorf("MCP 服务返回 HTTP %d", resp.StatusCode)
		}
		return fmt.Errorf("MCP 服务返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	if transport == "sse" && !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return fmt.Errorf("MCP SSE 响应类型无效: %s", resp.Header.Get("Content-Type"))
	}
	return nil
}
