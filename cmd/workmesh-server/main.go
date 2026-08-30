// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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
	if err := initializeDataDir(cfg.DataDir); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "初始化数据目录失败:", err)
		os.Exit(1)
	}
	if handled, err := runCLI(os.Args[1:], cfg.DataDir); handled {
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	logger := log.New(nil)
	if logWriter, openErr := log.Open(log.Config{Path: filepath.Join(cfg.DataDir, "logs", "server.log")}); openErr == nil {
		defer logWriter.Close()
		logger = log.New(io.MultiWriter(os.Stderr, logWriter))
	}
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
	nodeapi.StartBackgroundTasks(ctx)
	defer scheduler.Stop()

	mux, gatewayStore := httpMux(cfg)
	gatewayStore.Start(ctx, []string{"system", "containers", "files", "databases", "websites", "tasks"})
	// 统一安全包装器位于所有控制面和节点路由外层，避免新增路由遗漏 Session/CSRF、域名绑定和密码过期校验。
	securedMux := controlapi.NewSecurityMiddleware(mux, controlapi.SecurityMiddlewareOptions{
		DataDir: cfg.DataDir, Authorize: nodeapi.AuthorizeControlRequest,
	})
	server := wmhttp.New(cfg.ListenAddr, securedMux, cfg.RequestTimeout)
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

func httpMux(cfg config.Config) (*http.ServeMux, *controlapi.GatewayStateStore) {
	mux := http.NewServeMux()
	// 静态资源必须在 API 兼容层之前命中文件系统，否则浏览器会收到 JSON 错误响应并拒绝执行模块脚本。
	staticRoot := filepath.Join("web", "dist")
	staticFiles := http.StripPrefix("/", http.FileServer(http.Dir(staticRoot)))
	mux.HandleFunc("GET /assets/{filepath...}", func(w http.ResponseWriter, r *http.Request) {
		staticFiles.ServeHTTP(w, r)
	})
	// 旧前端仍会请求 images/static 资源；统一映射到发布包 public 目录并拒绝路径穿越。
	publicRoot := filepath.Join("public")
	servePublic := func(w http.ResponseWriter, r *http.Request) {
		relative := strings.TrimPrefix(r.URL.Path, "/api/v2/")
		relative = strings.TrimPrefix(relative, "images/")
		if strings.HasPrefix(r.URL.Path, "/api/v2/static/") {
			relative = strings.TrimPrefix(r.URL.Path, "/api/v2/static/")
		}
		clean := filepath.Clean(filepath.FromSlash(relative))
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "INVALID_ASSET_PATH"}})
			return
		}
		file := filepath.Join(publicRoot, clean)
		if _, err := os.Stat(file); err != nil {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "ASSET_NOT_FOUND"}})
			return
		}
		http.ServeFile(w, r, file)
	}
	// 兼容旧前端的公开静态入口，路径已移除旧产品品牌前缀。
	mux.HandleFunc("GET /public/{filepath...}", func(w http.ResponseWriter, r *http.Request) {
		relative := strings.TrimPrefix(r.URL.Path, "/public/")
		clean := filepath.Clean(filepath.FromSlash(relative))
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "INVALID_ASSET_PATH"}})
			return
		}
		file := filepath.Join(publicRoot, clean)
		if _, err := os.Stat(file); err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, file)
	})
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		file := filepath.Join(publicRoot, "favicon.ico")
		if _, err := os.Stat(file); err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, file)
	})
	mux.HandleFunc("GET /favicon.ico/{filepath...}", func(w http.ResponseWriter, r *http.Request) {
		relative := strings.TrimPrefix(r.URL.Path, "/favicon.ico/")
		clean := filepath.Clean(filepath.FromSlash(relative))
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		file := filepath.Join(publicRoot, clean)
		if _, err := os.Stat(file); err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, file)
	})
	// Swagger 文档入口保留功能但使用 WorkMesh 无品牌路径；具体文档由发布包提供时再替换响应体。
	mux.HandleFunc("GET /swagger/{any...}", func(w http.ResponseWriter, _ *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"openapi": "3.0.0", "info": map[string]any{"title": "WorkMesh Server API", "version": "v2"}, "servers": []any{map[string]any{"url": "/api/v2"}}})
	})
	mux.HandleFunc("GET /api/v2/images/{filename...}", servePublic)
	mux.HandleFunc("GET /api/v2/static/{filename...}", servePublic)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 发布包包含 web/dist 时由同一进程托管前端，开发环境无构建产物则返回服务信息。
		index := filepath.Join(staticRoot, "index.html")
		if _, err := os.Stat(index); err != nil {
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]string{"service": "workmesh-server"}})
			return
		}
		// API 未命中时必须继续返回 JSON 404，不能把接口请求错误地回退成 HTML。
		if strings.HasPrefix(r.URL.Path, "/api/") || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "ROUTE_NOT_FOUND", "path": r.URL.Path}})
			return
		}
		// Vue Router 使用 history 模式。存在的静态文件照常返回，其他前端路径统一回退到 index.html，
		// 这样直接刷新 /login、/settings/bind 等页面不会被 FileServer 当作物理文件返回 404。
		clean := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		if clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			candidate := filepath.Join(staticRoot, clean)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				http.ServeFile(w, r, candidate)
				return
			}
		}
		http.ServeFile(w, r, index)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]string{"status": "ok"}})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]string{"status": "ready"}})
	})
	mux.HandleFunc("GET /api/v2/health/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]string{"status": "ok"}})
	})
	gatewayStore := controlapi.Register(mux, cfg.NodeID, cfg.Role, nodeapi.AuthorizeControlRequest)
	nodeMux := http.NewServeMux()
	nodeapi.Register(nodeMux)
	// 节点执行面统一经过本机会话鉴权；流接口保留各自的短期 Token 校验。
	mux.Handle("/api/v2/", authenticateNodeAPI(nodeMux))
	return mux, gatewayStore
}

func authenticateNodeAPI(next *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := next.Handler(r)
		if pattern == "" || publicNodeAPIPath(r) || selfAuthenticatedStreamPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !nodeapi.AuthorizeControlRequest(r) {
			wmhttp.JSON(w, http.StatusUnauthorized, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "LOCAL_AUTH_REQUIRED"}, "message": "需要有效的本地登录会话"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func publicNodeAPIPath(r *http.Request) bool {
	if r == nil {
		return false
	}
	path := r.URL.Path
	if r.Method == http.MethodGet {
		switch path {
		case "/api/v2/health", "/api/v2/health/check", "/api/v2/core/health", "/api/v2/core/auth/captcha", "/api/v2/core/auth/setting", "/api/v2/core/auth/welcome", "/api/v2/core/auth/ldap/status", "/api/v2/core/auth/oidc/status", "/api/v2/core/auth/saml2/status":
			return true
		}
	}
	if r.Method == http.MethodPost {
		switch path {
		case "/api/v2/core/auth/login", "/api/v2/core/auth/mfalogin", "/api/v2/core/auth/passkey/finish", "/api/v2/core/auth/oidc/begin", "/api/v2/core/auth/oidc/finish", "/api/v2/core/auth/saml2/begin", "/api/v2/core/auth/saml2/finish":
			return true
		}
	}
	return false
}

func selfAuthenticatedStreamPath(path string) bool {
	switch path {
	case "/api/v2/process/ws", "/api/v2/containers/search/log", "/api/v2/files/wget/process", "/api/v2/hosts/terminal/local", "/api/v2/hosts/terminal/container", "/api/v2/hosts/terminal/ssh":
		return true
	default:
		return false
	}
}
