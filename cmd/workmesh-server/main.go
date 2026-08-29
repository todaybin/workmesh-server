// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/todaybin/workmesh-server/config"
	controlapi "github.com/todaybin/workmesh-server/control/api"
	nodeapi "github.com/todaybin/workmesh-server/node/api"
	"github.com/todaybin/workmesh-server/runtime/cache"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"github.com/todaybin/workmesh-server/runtime/log"
	"github.com/todaybin/workmesh-server/runtime/role"
	"github.com/todaybin/workmesh-server/runtime/schedule"
	"github.com/todaybin/workmesh-server/runtime/store"
)

func main() {
	cfg := config.Load()
	logger := log.New(nil)
	stateStore, err := store.Open(filepath.Join(cfg.DataDir, "state.json"))
	if err != nil {
		logger.Error("打开状态存储失败", "error", err)
		os.Exit(1)
	}
	defer stateStore.Close()
	_ = cache.New()
	if _, err := role.New(cfg.NodeID, cfg.Role); err != nil {
		logger.Error("节点角色配置无效", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	scheduler := schedule.New()
	scheduler.Start(ctx)
	defer scheduler.Stop()

	mux := httpMux(cfg)
	server := wmhttp.New(cfg.ListenAddr, mux, cfg.RequestTimeout)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, context.Canceled) {
			logger.Error("HTTP 服务退出", "error", err)
			stop()
		}
	}()
	logger.Info("WorkMesh Server 已启动", "addr", cfg.ListenAddr, "node_id", cfg.NodeID, "role", cfg.Role)
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}

func httpMux(cfg config.Config) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]string{"service": "workmesh-server"}})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]string{"status": "ok"}})
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]string{"status": "ready"}})
	})
	mux.HandleFunc("/api/v2/health/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]string{"status": "ok"}})
	})
	controlapi.Register(mux, cfg.NodeID, cfg.Role)
	nodeapi.Register(mux)
	return mux
}
