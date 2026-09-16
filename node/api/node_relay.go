// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/runtime/link"
)

const (
	defaultRelayTimeout = 15 * time.Second
	maxRelayBody        = 8 << 20
	maxRelayResponse    = 8 << 20
	relayClockSkew      = 5 * time.Minute
	relayNonceTTL       = 10 * time.Minute
)

// RelayOptions 配置主节点到次节点的通用请求透传。
type RelayOptions struct {
	// DataDir 必须是服务数据目录，节点清单从其中的 nodes.json 读取。
	DataDir string
	NodeID  string
	Secret  []byte
	HTTP    *http.Client
	Timeout time.Duration
	// MaxRetries 控制可安全重放请求的传输重试次数；每次重试都会重新生成 nonce 和签名。
	MaxRetries int
	// RetryBackoff 是连续重试之间的最小等待时间。
	RetryBackoff time.Duration
	// RoleEpoch 返回当前 fencing epoch；为空时从 role-state.json 读取。
	RoleEpoch func(context.Context) (uint64, error)
	// NodeLookup 从 SQLite 返回节点地址；配置后不再扫描 nodes.json。
	NodeLookup func(context.Context, string) (string, string, bool)
}

// NodeRelay 根据 CurrentNode/operateNode 将请求转发至已登记节点。
// 透传请求和接收请求共用一个中间件，接收端只有在签名、时间戳、nonce 和 epoch
// 校验通过后才会进入本地处理器，避免伪造节点地址或循环转发。
type NodeRelay struct {
	next       http.Handler
	options    RelayOptions
	client     *http.Client
	nonceMu    sync.Mutex
	nonces     map[string]time.Time
	nodesMu    sync.RWMutex
	nodes      map[string]relayNode
	nodesMtime time.Time
}

// forwardedVerifiedKey 用于在节点透传完成签名校验后向下游中间件传递可信标记。
// 标记只存在于当前请求上下文，不会通过网络头部传播，避免调用方伪造。
type forwardedVerifiedKey struct{}

// IsForwardedRequestVerified 判断请求是否已由 NodeRelay 完成内部签名校验。
// 仅供本进程下游鉴权中间件使用，外部请求无法直接设置该上下文值。
func IsForwardedRequestVerified(req *http.Request) bool {
	if req == nil {
		return false
	}
	verified, _ := req.Context().Value(forwardedVerifiedKey{}).(bool)
	return verified
}

type relayNode struct {
	NodeID   string `json:"nodeId"`
	Addr     string `json:"addr"`
	Endpoint string `json:"endpoint"`
}

// NewNodeRelay 创建节点透传中间件。节点清单读取失败时不会猜测地址，而是返回明确错误。
func NewNodeRelay(next http.Handler, options RelayOptions) *NodeRelay {
	if options.Timeout <= 0 {
		options.Timeout = defaultRelayTimeout
	}
	if options.DataDir == "" {
		options.DataDir = "./data"
	}
	if options.HTTP == nil {
		options.HTTP = &http.Client{Timeout: options.Timeout}
	}
	if options.MaxRetries < 0 {
		options.MaxRetries = 0
	}
	if options.RetryBackoff <= 0 {
		options.RetryBackoff = 100 * time.Millisecond
	}
	return &NodeRelay{next: next, options: options, client: options.HTTP, nonces: make(map[string]time.Time), nodes: make(map[string]relayNode)}
}

// ServeHTTP 执行接收端认证或发起远端透传。
func (r *NodeRelay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req == nil {
		writeRelayError(w, http.StatusBadRequest, "请求不能为空")
		return
	}
	if req.Header.Get("X-WorkMesh-Forwarded") == "1" {
		r.serveForwarded(w, req)
		return
	}
	target := relayTarget(req)
	if target == "" || target == "local" || target == "undefined" || target == r.options.NodeID {
		r.next.ServeHTTP(w, req)
		return
	}
	node, ok := r.findNode(target)
	if !ok {
		writeRelayError(w, http.StatusNotFound, "目标节点不存在: "+target)
		return
	}
	if strings.EqualFold(req.Header.Get("Upgrade"), "websocket") {
		if err := r.forwardWebSocket(w, req, node); err != nil {
			writeRelayError(w, http.StatusBadGateway, err.Error())
		}
		return
	}
	if err := r.forward(w, req, node); err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, errRelayBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeRelayError(w, status, err.Error())
	}
}

// forwardWebSocket 在节点间建立原始 TCP 隧道。HTTP Client 无法转发升级后的双向帧，
// 因此这里显式完成签名握手，再把双方连接字节流双向复制。
func (r *NodeRelay) forwardWebSocket(w http.ResponseWriter, req *http.Request, node relayNode) error {
	base, err := relayBaseURL(node)
	if err != nil {
		return err
	}
	u, err := url.Parse(base)
	if err != nil {
		return err
	}
	query := req.URL.Query()
	query.Del("operateNode")
	path := req.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	requestTarget := pathWithQuery(path, query)
	ctx, cancel := context.WithTimeout(req.Context(), r.options.Timeout)
	defer cancel()
	dialer := &net.Dialer{Timeout: r.options.Timeout}
	var upstream net.Conn
	if strings.EqualFold(u.Scheme, "https") {
		upstream, err = tls.DialWithDialer(dialer, "tcp", u.Host, &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
	} else {
		upstream, err = dialer.DialContext(ctx, "tcp", u.Host)
	}
	if err != nil {
		return fmt.Errorf("连接目标节点失败: %w", err)
	}
	defer upstream.Close()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randomRelayNonce()
	signature := ""
	if len(r.options.Secret) > 0 {
		signature = link.Sign(r.options.Secret, req.Method, requestTarget, timestamp, nonce, nil)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s HTTP/1.1\r\nHost: %s\r\n", req.Method, requestTarget, u.Host)
	for key, values := range req.Header {
		lower := strings.ToLower(key)
		if lower == "host" || strings.HasPrefix(lower, "x-workmesh-") || lower == "content-length" {
			continue
		}
		for _, value := range values {
			fmt.Fprintf(&b, "%s: %s\r\n", key, value)
		}
	}
	b.WriteString("X-WorkMesh-Forwarded: 1\r\n")
	b.WriteString("X-WorkMesh-Node-ID: " + r.options.NodeID + "\r\n")
	b.WriteString("X-WorkMesh-Timestamp: " + timestamp + "\r\n")
	b.WriteString("X-WorkMesh-Nonce: " + nonce + "\r\n")
	b.WriteString("X-WorkMesh-Role-Epoch: ")
	epoch, epochErr := r.currentEpoch(ctx)
	if epochErr != nil {
		return epochErr
	}
	fmt.Fprintf(&b, "%d\r\n", epoch)
	if signature != "" {
		b.WriteString("X-WorkMesh-Signature: " + signature + "\r\n")
	}
	b.WriteString("\r\n")
	if _, err := io.WriteString(upstream, b.String()); err != nil {
		return err
	}
	reader := bufio.NewReader(upstream)
	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		return fmt.Errorf("目标节点 WebSocket 握手失败: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		defer resp.Body.Close()
		return fmt.Errorf("目标节点返回 HTTP %d", resp.StatusCode)
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return errors.New("当前服务器不支持 WebSocket 隧道")
	}
	clientConn, clientRW, err := hijacker.Hijack()
	if err != nil {
		return err
	}
	defer clientConn.Close()
	if _, err := clientRW.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + resp.Header.Get("Sec-WebSocket-Accept") + "\r\n\r\n"); err != nil {
		return err
	}
	if err := clientRW.Flush(); err != nil {
		return err
	}
	// Reader 可能已经预读了握手后的帧，先转发缓冲区再进入双向复制。
	copyConn := func(dst net.Conn, src io.Reader) { _, _ = io.Copy(dst, src) }
	if reader.Buffered() > 0 {
		n := reader.Buffered()
		buf, _ := reader.Peek(n)
		_, _ = clientConn.Write(buf)
		_, _ = reader.Discard(n)
	}
	go copyConn(upstream, clientConn)
	copyConn(clientConn, reader)
	return nil
}

var errRelayBodyTooLarge = errors.New("节点透传请求体超过 8 MiB 限制")

func (r *NodeRelay) forward(w http.ResponseWriter, req *http.Request, node relayNode) error {
	base, err := relayBaseURL(node)
	if err != nil {
		return err
	}
	var source io.Reader = http.NoBody
	if req.Body != nil {
		source = req.Body
	}
	body, err := io.ReadAll(io.LimitReader(source, maxRelayBody+1))
	if err != nil {
		return fmt.Errorf("读取节点透传请求失败: %w", err)
	}
	if len(body) > maxRelayBody {
		return errRelayBodyTooLarge
	}
	attempts := 1
	// SSE 是长连接；连接中断后重放会重复订阅并可能把已经发送的事件重复交给前端。
	if relayRequestReplaySafe(req) && !relayEventStreamRequest(req) {
		attempts += r.options.MaxRetries
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		lastErr = r.forwardAttempt(w, req, base, body)
		if lastErr == nil {
			return nil
		}
		if attempt+1 >= attempts {
			break
		}
		if err := waitRelayRetry(req.Context(), r.options.RetryBackoff, attempt); err != nil {
			return err
		}
	}
	return lastErr
}

// relayRequestReplaySafe 防止远端已接受写请求后连接中断造成副作用重复执行。
func relayRequestReplaySafe(req *http.Request) bool {
	if req == nil {
		return false
	}
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return strings.TrimSpace(req.Header.Get("Idempotency-Key")) != ""
	}
}

func waitRelayRetry(ctx context.Context, delay time.Duration, attempt int) error {
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

func (r *NodeRelay) forwardAttempt(w http.ResponseWriter, req *http.Request, base string, body []byte) error {
	path := req.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	query := req.URL.Query()
	query.Del("operateNode")
	requestURL := base + path
	if encoded := query.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}
	streamRequest := relayEventStreamRequest(req)
	ctx := req.Context()
	cancel := func() {}
	if streamRequest {
		// 流的生命周期由浏览器请求 context 控制，不能沿用普通 API 的短超时。
		ctx, cancel = context.WithCancel(ctx)
	} else {
		ctx, cancel = context.WithTimeout(ctx, r.options.Timeout)
	}
	defer cancel()
	outgoing, err := http.NewRequestWithContext(ctx, req.Method, requestURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("创建节点透传请求失败: %w", err)
	}
	copyRelayHeaders(outgoing.Header, req.Header)
	outgoing.Header.Set("X-WorkMesh-Forwarded", "1")
	outgoing.Header.Set("X-WorkMesh-Node-ID", r.options.NodeID)
	epoch, err := r.currentEpoch(ctx)
	if err != nil {
		return fmt.Errorf("读取角色 epoch 失败: %w", err)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randomRelayNonce()
	outgoing.Header.Set("X-WorkMesh-Role-Epoch", strconv.FormatUint(epoch, 10))
	outgoing.Header.Set("X-WorkMesh-Timestamp", timestamp)
	outgoing.Header.Set("X-WorkMesh-Nonce", nonce)
	if len(r.options.Secret) > 0 {
		outgoing.Header.Set("X-WorkMesh-Signature", link.Sign(r.options.Secret, req.Method, pathWithQuery(path, query), timestamp, nonce, body))
	}
	client := r.client
	if streamRequest && client.Timeout > 0 {
		// http.Client.Timeout 会在响应返回后继续中断 response.Body，必须为 SSE 清除。
		streamClient := *client
		streamClient.Timeout = 0
		client = &streamClient
	}
	response, err := client.Do(outgoing)
	if err != nil {
		return fmt.Errorf("节点透传请求失败: %w", err)
	}
	defer response.Body.Close()
	if streamRequest && strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		return relayEventStream(w, response)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxRelayResponse+1))
	if err != nil {
		return fmt.Errorf("读取节点透传响应失败: %w", err)
	}
	if len(data) > maxRelayResponse {
		return errors.New("节点透传响应超过 8 MiB 限制")
	}
	for key, values := range response.Header {
		if isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(data)
	return nil
}

// relayEventStreamRequest 判断请求是否要求 SSE 响应。
func relayEventStreamRequest(req *http.Request) bool {
	return req != nil && strings.Contains(strings.ToLower(req.Header.Get("Accept")), "text/event-stream")
}

// relayEventStream 将远端 SSE 按块转发并在每块后刷新，避免日志被透传层缓存到连接结束。
func relayEventStream(w http.ResponseWriter, response *http.Response) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("当前服务器不支持 SSE 刷新")
	}
	setDeadline := http.NewResponseController(w).SetWriteDeadline
	for key, values := range response.Header {
		if isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Del("Content-Length")
	w.WriteHeader(response.StatusCode)
	_ = setDeadline(time.Now().Add(30 * time.Second))
	flusher.Flush()
	buffer := make([]byte, 32<<10)
	for {
		size, readErr := response.Body.Read(buffer)
		if size > 0 {
			_ = setDeadline(time.Now().Add(30 * time.Second))
			if _, writeErr := w.Write(buffer[:size]); writeErr != nil {
				// 下游断开后不能再追加 JSON 错误响应。
				return nil
			}
			flusher.Flush()
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			// 状态码和已读数据已经发送，避免把错误 envelope 拼接进 SSE。
			return nil
		}
	}
}

// serveForwarded 校验来自其它节点的签名请求，并将可信标记传给本地处理器。
func (r *NodeRelay) serveForwarded(w http.ResponseWriter, req *http.Request) {
	var source io.Reader = http.NoBody
	if req.Body != nil {
		source = req.Body
	}
	body, err := io.ReadAll(io.LimitReader(source, maxRelayBody+1))
	if err != nil || len(body) > maxRelayBody {
		writeRelayError(w, http.StatusRequestEntityTooLarge, "节点透传请求体无效或超过 8 MiB 限制")
		return
	}
	if err := r.authenticateForwarded(req, body); err != nil {
		writeRelayError(w, http.StatusUnauthorized, err.Error())
		return
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	// 清理透传标记，确保本地处理器不会再次触发远端转发。
	req.Header.Del("X-WorkMesh-Forwarded")
	// 将验签结果写入请求上下文，供目标节点外层鉴权中间件放行；不使用可伪造的请求头。
	req = req.WithContext(context.WithValue(req.Context(), forwardedVerifiedKey{}, true))
	r.next.ServeHTTP(w, req)
}

// authenticateForwarded 校验时间戳、nonce、epoch 和 HMAC，阻止重放或越权透传。
func (r *NodeRelay) authenticateForwarded(req *http.Request, body []byte) error {
	if len(r.options.Secret) == 0 {
		return errors.New("节点透传未配置共享密钥")
	}
	nodeID := strings.TrimSpace(req.Header.Get("X-WorkMesh-Node-ID"))
	if nodeID == "" || nodeID == r.options.NodeID {
		return errors.New("节点透传身份无效")
	}
	timestamp := strings.TrimSpace(req.Header.Get("X-WorkMesh-Timestamp"))
	nonce := strings.TrimSpace(req.Header.Get("X-WorkMesh-Nonce"))
	signature := strings.TrimSpace(req.Header.Get("X-WorkMesh-Signature"))
	if timestamp == "" || nonce == "" || signature == "" {
		return errors.New("节点透传签名请求头不完整")
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(seconds, 0)) > relayClockSkew || time.Until(time.Unix(seconds, 0)) > relayClockSkew {
		return errors.New("节点透传时间戳无效或已过期")
	}
	path := req.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	expected := link.Sign(r.options.Secret, req.Method, pathWithQuery(path, req.URL.Query()), timestamp, nonce, body)
	if !secureRelaySignature(signature, expected) {
		return errors.New("节点透传签名无效")
	}
	if epochText := strings.TrimSpace(req.Header.Get("X-WorkMesh-Role-Epoch")); epochText != "" {
		sentEpoch, parseErr := strconv.ParseUint(epochText, 10, 64)
		if parseErr != nil {
			return errors.New("节点透传 role epoch 无效")
		}
		current, epochErr := r.currentEpoch(req.Context())
		if epochErr != nil || sentEpoch != current {
			return errors.New("节点透传 role epoch 冲突")
		}
	} else {
		return errors.New("节点透传缺少 role epoch")
	}
	key := nodeID + ":" + nonce
	r.nonceMu.Lock()
	now := time.Now()
	for oldKey, expires := range r.nonces {
		if !expires.After(now) {
			delete(r.nonces, oldKey)
		}
	}
	if expires, exists := r.nonces[key]; exists && expires.After(now) {
		r.nonceMu.Unlock()
		return errors.New("节点透传 nonce 已使用")
	}
	r.nonces[key] = now.Add(relayNonceTTL)
	r.nonceMu.Unlock()
	return nil
}

// currentEpoch 读取当前角色 fencing epoch，优先使用注入的 SQLite 查询函数。
func (r *NodeRelay) currentEpoch(ctx context.Context) (uint64, error) {
	if r.options.RoleEpoch != nil {
		return r.options.RoleEpoch(ctx)
	}
	path := filepath.Join(r.options.DataDir, "role-state.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 1, nil
		}
		return 0, err
	}
	var state struct {
		RoleEpoch uint64 `json:"roleEpoch"`
	}
	if err := json.Unmarshal(raw, &state); err != nil || state.RoleEpoch == 0 {
		return 0, errors.New("角色状态文件无效")
	}
	return state.RoleEpoch, nil
}

// findNode 按节点 ID 查询缓存清单，文件变化时才重新读取以减少请求路径开销。
func (r *NodeRelay) findNode(nodeID string) (relayNode, bool) {
	if r.options.NodeLookup != nil {
		endpoint, addr, ok := r.options.NodeLookup(context.Background(), nodeID)
		if !ok {
			return relayNode{}, false
		}
		return relayNode{NodeID: nodeID, Endpoint: endpoint, Addr: addr}, true
	}
	path := filepath.Join(r.options.DataDir, "nodes.json")
	info, err := os.Stat(path)
	if err != nil {
		return relayNode{}, false
	}
	r.nodesMu.RLock()
	cached, ok := r.nodes[nodeID]
	mtime := r.nodesMtime
	r.nodesMu.RUnlock()
	if ok && mtime.Equal(info.ModTime()) {
		return cached, true
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return relayNode{}, false
	}
	var rows []relayNode
	if json.Unmarshal(raw, &rows) != nil {
		return relayNode{}, false
	}
	r.nodesMu.Lock()
	r.nodes = make(map[string]relayNode, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.NodeID) != "" {
			r.nodes[row.NodeID] = row
		}
	}
	r.nodesMtime = info.ModTime()
	result, found := r.nodes[nodeID]
	r.nodesMu.Unlock()
	return result, found
}
