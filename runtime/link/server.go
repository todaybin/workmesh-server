// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package link

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"github.com/todaybin/workmesh-server/runtime/role"
)

const (
	defaultProtocolVersion = "v2"
	defaultClockSkew       = 5 * time.Minute
	defaultNonceTTL        = 10 * time.Minute
	maxRequestBytes        = 8 << 20
)

// ErrSyncConflict 表示同步游标落后于远端当前版本，调用方应重新拉取后合并。
var ErrSyncConflict = errors.New("同步游标冲突")

// ServerOptions 配置本机节点链路控制面。
type ServerOptions struct {
	NodeID          string
	Role            string
	ProtocolVersion string
	Capabilities    []string
	Secret          []byte
	RoleManager     *role.Manager
	SyncStore       SyncStore
	Clock           Clock
	ClockSkew       time.Duration
	NonceTTL        time.Duration
}

// SyncStore 是增量同步数据的最小持久化接口。生产环境可替换为数据库实现。
type SyncStore interface {
	Pull(context.Context, SyncCursor) ([]byte, SyncCursor, error)
	Push(context.Context, SyncCursor, []byte) (SyncCursor, error)
}

// MemorySyncStore 是无外部依赖的同步存储，适合单节点部署和测试。
type MemorySyncStore struct {
	mu      sync.RWMutex
	streams map[string]syncRecord
}

type syncRecord struct {
	payload []byte
	version uint64
}

// NewMemorySyncStore 创建内存同步存储。
func NewMemorySyncStore() *MemorySyncStore {
	return &MemorySyncStore{streams: make(map[string]syncRecord)}
}

// Pull 返回指定游标之后的最新快照；没有更新时 payload 为空。
func (s *MemorySyncStore) Pull(ctx context.Context, cursor SyncCursor) ([]byte, SyncCursor, error) {
	if err := ctx.Err(); err != nil {
		return nil, SyncCursor{}, err
	}
	if err := validateStream(cursor.Stream); err != nil {
		return nil, SyncCursor{}, err
	}
	s.mu.RLock()
	record := s.streams[cursor.Stream]
	s.mu.RUnlock()
	current := SyncCursor{Stream: cursor.Stream, Version: record.version}
	if cursor.Version >= record.version {
		return nil, current, nil
	}
	return append([]byte(nil), record.payload...), current, nil
}

// Push 以 compare-and-set 方式写入同步快照，拒绝旧 epoch/游标覆盖新数据。
func (s *MemorySyncStore) Push(ctx context.Context, cursor SyncCursor, payload []byte) (SyncCursor, error) {
	if err := ctx.Err(); err != nil {
		return SyncCursor{}, err
	}
	if err := validateStream(cursor.Stream); err != nil {
		return SyncCursor{}, err
	}
	if len(payload) > maxRequestBytes {
		return SyncCursor{}, errors.New("同步数据超过 8 MiB 限制")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.streams[cursor.Stream]
	if cursor.Version < record.version {
		// 网络重试可能重复提交同一请求；相同内容视为幂等成功。
		if bytes.Equal(record.payload, payload) {
			return SyncCursor{Stream: cursor.Stream, Version: record.version}, nil
		}
		return SyncCursor{Stream: cursor.Stream, Version: record.version}, ErrSyncConflict
	}
	if cursor.Version > record.version {
		return SyncCursor{Stream: cursor.Stream, Version: record.version}, ErrSyncConflict
	}
	record.version++
	record.payload = append([]byte(nil), payload...)
	s.streams[cursor.Stream] = record
	return SyncCursor{Stream: cursor.Stream, Version: record.version}, nil
}

func validateStream(stream string) error {
	if stream == "" || len(stream) > 128 {
		return errors.New("同步流名称无效")
	}
	for _, char := range stream {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '.' && char != '_' && char != '-' {
			return errors.New("同步流名称包含非法字符")
		}
	}
	return nil
}

// Server 提供握手、心跳、同步和 fencing 控制面接口。
type Server struct {
	options ServerOptions
	manager *role.Manager
	store   SyncStore
	clock   Clock

	mu      sync.Mutex
	pending role.Transition
	peers   map[string]Heartbeat
	nonces  map[string]time.Time
}

// NewServer 创建本机链路控制面；角色管理器为空时自动创建一个内存管理器。
func NewServer(options ServerOptions) (*Server, error) {
	if options.NodeID == "" {
		return nil, errors.New("节点 ID 不能为空")
	}
	if options.Role == "" {
		options.Role = role.Secondary
	}
	if options.ProtocolVersion == "" {
		options.ProtocolVersion = defaultProtocolVersion
	}
	manager := options.RoleManager
	if manager == nil {
		var err error
		manager, err = role.New(options.NodeID, options.Role)
		if err != nil {
			return nil, err
		}
	}
	if state := manager.State(context.Background()); state.NodeID != options.NodeID {
		return nil, errors.New("角色管理器节点身份与链路节点不一致")
	}
	if options.SyncStore == nil {
		options.SyncStore = NewMemorySyncStore()
	}
	if options.ClockSkew <= 0 {
		options.ClockSkew = defaultClockSkew
	}
	if options.NonceTTL <= 0 {
		options.NonceTTL = defaultNonceTTL
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	options.Secret = append([]byte(nil), options.Secret...)
	return &Server{
		options: options,
		manager: manager,
		store:   options.SyncStore,
		clock:   clock,
		peers:   make(map[string]Heartbeat),
		nonces:  make(map[string]time.Time),
	}, nil
}

// Manager 返回本机使用的角色管理器，便于路由层共享角色状态。
func (s *Server) Manager() *role.Manager { return s.manager }

// Register 将链路控制面注册到统一 HTTP ServeMux。
func (s *Server) Register(mux *http.ServeMux) {
	if mux == nil {
		return
	}
	for _, pattern := range []string{"POST /api/v2/link/handshake", "POST /api/v2/workmesh/link/handshake"} {
		mux.HandleFunc(pattern, s.handshake)
	}
	for _, pattern := range []string{"POST /api/v2/link/heartbeat", "POST /api/v2/workmesh/link/heartbeat"} {
		mux.HandleFunc(pattern, s.heartbeat)
	}
	for _, pattern := range []string{"GET /api/v2/link/status", "GET /api/v2/workmesh/link/status"} {
		mux.HandleFunc(pattern, s.status)
	}
	for _, pattern := range []string{"POST /api/v2/link/sync/pull", "POST /api/v2/workmesh/link/sync/pull"} {
		mux.HandleFunc(pattern, s.pull)
	}
	for _, pattern := range []string{"POST /api/v2/link/sync/push", "POST /api/v2/workmesh/link/sync/push"} {
		mux.HandleFunc(pattern, s.push)
	}
	for _, action := range []string{"check", "prepare", "commit", "abort"} {
		mux.HandleFunc("POST /api/v2/link/fencing/"+action, s.fencing(action))
		mux.HandleFunc("POST /api/v2/link/fence/"+action, s.fencing(action))
	}
}

func (s *Server) handshake(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeLinkError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.authenticate(r, body); err != nil {
		writeLinkError(w, authStatus(err), err)
		return
	}
	var request Handshake
	if err := json.Unmarshal(body, &request); err != nil {
		writeLinkError(w, http.StatusBadRequest, err)
		return
	}
	if request.NodeID == "" || request.NodeID == s.options.NodeID {
		writeLinkError(w, http.StatusBadRequest, errors.New("握手节点身份无效"))
		return
	}
	state := s.manager.State(r.Context())
	response := Handshake{NodeID: state.NodeID, Role: state.Role, RoleEpoch: state.RoleEpoch, ProtocolVersion: s.options.ProtocolVersion, Capabilities: append([]string(nil), s.options.Capabilities...), Nonce: newResponseNonce()}
	s.mu.Lock()
	s.peers[request.NodeID] = Heartbeat{NodeID: request.NodeID, RoleEpoch: request.RoleEpoch, Version: request.ProtocolVersion, SentAt: s.clock().UTC().Format(time.RFC3339)}
	s.mu.Unlock()
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": response})
}

func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeLinkError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.authenticate(r, body); err != nil {
		writeLinkError(w, authStatus(err), err)
		return
	}
	var request Heartbeat
	if err := json.Unmarshal(body, &request); err != nil || request.NodeID == "" {
		writeLinkError(w, http.StatusBadRequest, errors.New("心跳节点身份无效"))
		return
	}
	s.mu.Lock()
	s.peers[request.NodeID] = request
	s.mu.Unlock()
	state := s.manager.State(r.Context())
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"accepted": true, "current": state}})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	state := s.manager.State(r.Context())
	s.mu.Lock()
	peers := make([]Heartbeat, 0, len(s.peers))
	for _, peer := range s.peers {
		peers = append(peers, peer)
	}
	s.mu.Unlock()
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"current": state, "peers": peers}})
}

func (s *Server) pull(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeLinkError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.authenticate(r, body); err != nil {
		writeLinkError(w, authStatus(err), err)
		return
	}
	var cursor SyncCursor
	if err := json.Unmarshal(body, &cursor); err != nil {
		writeLinkError(w, http.StatusBadRequest, err)
		return
	}
	payload, next, err := s.store.Pull(r.Context(), cursor)
	if err != nil {
		writeLinkError(w, syncStatus(err), err)
		return
	}
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"payload": payload, "cursor": next}})
}

func (s *Server) push(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeLinkError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.authenticate(r, body); err != nil {
		writeLinkError(w, authStatus(err), err)
		return
	}
	var request struct {
		Stream  string `json:"stream"`
		Version uint64 `json:"version"`
		Payload []byte `json:"payload"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		writeLinkError(w, http.StatusBadRequest, err)
		return
	}
	next, err := s.store.Push(r.Context(), SyncCursor{Stream: request.Stream, Version: request.Version}, request.Payload)
	if err != nil {
		writeLinkError(w, syncStatus(err), err)
		return
	}
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"cursor": next}})
}

func (s *Server) fencing(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := readBody(r)
		if err != nil {
			writeLinkError(w, http.StatusBadRequest, err)
			return
		}
		if err := s.authenticate(r, body); err != nil {
			writeLinkError(w, authStatus(err), err)
			return
		}
		var request role.Transition
		if len(body) > 0 && string(body) != "null" {
			if err := json.Unmarshal(body, &request); err != nil {
				writeLinkError(w, http.StatusBadRequest, err)
				return
			}
		}
		switch action {
		case "check":
			s.fencingCheck(w, r, request)
		case "prepare":
			s.fencingPrepare(w, r, request)
		case "commit":
			s.fencingCommit(w, r, request)
		case "abort":
			s.fencingAbort(w, r, request)
		}
	}
}

func (s *Server) fencingCheck(w http.ResponseWriter, r *http.Request, request role.Transition) {
	state := s.manager.State(r.Context())
	if err := validateTransition(state, request, false); err != nil {
		writeLinkError(w, http.StatusConflict, err)
		return
	}
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"ready": true, "current": state}})
}

func (s *Server) fencingPrepare(w http.ResponseWriter, r *http.Request, request role.Transition) {
	state := s.manager.State(r.Context())
	if err := validateTransition(state, request, true); err != nil {
		writeLinkError(w, http.StatusConflict, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending.OperationID != "" && s.pending.OperationID != request.OperationID {
		writeLinkError(w, http.StatusConflict, errors.New("已有角色切换操作正在准备"))
		return
	}
	s.pending = request
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]string{"status": "prepared", "operationId": request.OperationID}})
}

func (s *Server) fencingCommit(w http.ResponseWriter, r *http.Request, request role.Transition) {
	s.mu.Lock()
	pending := s.pending
	s.mu.Unlock()
	if request.OperationID != "" && request.OperationID != pending.OperationID {
		writeLinkError(w, http.StatusConflict, errors.New("角色切换操作不匹配"))
		return
	}
	if pending.OperationID == "" {
		writeLinkError(w, http.StatusConflict, errors.New("没有待提交的角色切换"))
		return
	}
	state, err := s.manager.Switch(r.Context(), pending.ExpectedEpoch, pending.To)
	if err != nil {
		writeLinkError(w, http.StatusConflict, err)
		return
	}
	s.mu.Lock()
	s.pending = role.Transition{}
	s.mu.Unlock()
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": state})
}

func (s *Server) fencingAbort(w http.ResponseWriter, _ *http.Request, request role.Transition) {
	s.mu.Lock()
	if request.OperationID != "" && s.pending.OperationID != "" && request.OperationID != s.pending.OperationID {
		s.mu.Unlock()
		writeLinkError(w, http.StatusConflict, errors.New("角色切换操作不匹配"))
		return
	}
	s.pending = role.Transition{}
	s.mu.Unlock()
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]string{"status": "aborted"}})
}

func validateTransition(state role.State, request role.Transition, requireOperation bool) error {
	if requireOperation && request.OperationID == "" {
		return errors.New("操作 ID 不能为空")
	}
	if request.NodeID != "" && request.NodeID != state.NodeID {
		return errors.New("节点身份冲突")
	}
	if request.From != "" && request.From != state.Role {
		return errors.New("源角色冲突")
	}
	if request.ExpectedEpoch != state.RoleEpoch {
		return ErrSyncConflict
	}
	if request.To != role.Primary && request.To != role.Secondary {
		return errors.New("目标角色无效")
	}
	return nil
}

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

func parseTimestamp(value string) (time.Time, error) {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err == nil {
		return time.Unix(seconds, 0), nil
	}
	return time.Parse(time.RFC3339, value)
}

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

func firstHeader(r *http.Request, names ...string) string {
	for _, name := range names {
		if value := r.Header.Get(name); value != "" {
			return value
		}
	}
	return ""
}

func authStatus(err error) int {
	if strings.Contains(err.Error(), "nonce") {
		return http.StatusConflict
	}
	return http.StatusUnauthorized
}

func syncStatus(err error) int {
	if errors.Is(err, ErrSyncConflict) {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}

func writeLinkJSON(w http.ResponseWriter, status int, value any) {
	wmhttp.JSON(w, status, value)
}

func writeLinkError(w http.ResponseWriter, status int, err error) {
	message := "链路请求失败"
	if err != nil {
		message = err.Error()
	}
	writeLinkJSON(w, status, map[string]any{"code": "ERR", "message": message})
}

func newResponseNonce() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(raw[:])
}
