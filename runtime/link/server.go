// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package link

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

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
