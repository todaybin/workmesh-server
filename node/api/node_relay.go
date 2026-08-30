// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
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
	// RoleEpoch 返回当前 fencing epoch；为空时从 role-state.json 读取。
	RoleEpoch func(context.Context) (uint64, error)
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
		options.DataDir = ".workmesh-data"
	}
	if options.HTTP == nil {
		options.HTTP = &http.Client{Timeout: options.Timeout}
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
	if err := r.forward(w, req, node); err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, errRelayBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeRelayError(w, status, err.Error())
	}
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
	ctx, cancel := context.WithTimeout(req.Context(), r.options.Timeout)
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
	response, err := r.client.Do(outgoing)
	if err != nil {
		return fmt.Errorf("节点透传请求失败: %w", err)
	}
	defer response.Body.Close()
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

func (r *NodeRelay) findNode(nodeID string) (relayNode, bool) {
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

func pathWithQuery(path string, query url.Values) string {
	if encoded := query.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

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

func isHopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func secureRelaySignature(got, expected string) bool {
	got = strings.TrimSpace(got)
	if hmac.Equal([]byte(got), []byte(expected)) {
		return true
	}
	return false
}

func randomRelayNonce() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
}

func writeRelayError(w http.ResponseWriter, status int, message string) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": message, "details": map[string]any{"errCode": "NODE_RELAY_FAILED"}})
}
