// SPDX-License-Identifier: LicenseRef-WorkMesh-Pending
// Copyright (c) 2026 WorkMesh contributors

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Server 是控制面与节点执行面共享的 HTTP 生命周期封装。
type Server struct {
	server *http.Server
}

// New 创建统一 HTTP Server；业务模块通过 Register 注册路由。
func New(addr string, handler http.Handler, timeout time.Duration) *Server {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Server{server: &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: timeout, ReadTimeout: timeout, WriteTimeout: timeout, IdleTimeout: 90 * time.Second}}
}

// ListenAndServe 启动服务。
func (s *Server) ListenAndServe() error { return s.server.ListenAndServe() }

// Shutdown 在给定上下文截止前优雅停止服务。
func (s *Server) Shutdown(ctx context.Context) error { return s.server.Shutdown(ctx) }

// JSON 将统一响应编码为 JSON。
func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
