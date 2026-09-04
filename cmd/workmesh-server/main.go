// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/todaybin/workmesh-server/config"
	controlapi "github.com/todaybin/workmesh-server/control/api"
	"github.com/todaybin/workmesh-server/internal/storage"
	nodeapi "github.com/todaybin/workmesh-server/node/api"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"github.com/todaybin/workmesh-server/runtime/log"
	"github.com/todaybin/workmesh-server/runtime/role"
)

func main() {
	cfg, configErr := config.Load()
	if configErr != nil {
		_, _ = fmt.Fprintln(os.Stderr, "加载服务配置失败:", configErr)
		os.Exit(1)
	}
	if err := cfg.ApplyRuntimeEnvironment(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "同步服务配置失败:", err)
		os.Exit(1)
	}
	if err := initializeDataDir(cfg.DataDir); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "初始化数据目录失败:", err)
		os.Exit(1)
	}
	// 安全入口由 functional_domain_state 中的共享 SQLite 持久化状态提供。
	// 启动时不得从旧 domains.json 生成随机入口，否则每次重新部署/重启
	// 都会造成入口变化并覆盖用户看到的地址。
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
	readiness := newReadinessState()
	readiness.SetNotReady("正在初始化统一存储")
	stateStore, err := storage.Open(filepath.Join(cfg.DataDir, "workmesh.db"))
	if err != nil {
		logger.Error("打开状态存储失败", "error", err)
		os.Exit(1)
	}
	defer stateStore.Close()
	if err := initializeUnifiedSchema(stateStore); err != nil {
		logger.Error("执行统一存储迁移失败", "error", err)
		os.Exit(1)
	}
	if err := service.SetWebsiteDB(stateStore.DB()); err != nil {
		logger.Error("初始化网站公共数据库存储失败", "error", err)
		os.Exit(1)
	}
	if err := service.SetWebsiteSecurityDB(stateStore.DB()); err != nil {
		logger.Error("初始化证书公共数据库存储失败", "error", err)
		os.Exit(1)
	}
	if report, err := importLegacyData(context.Background(), stateStore, cfg.DataDir); err != nil {
		logger.Error("导入旧数据失败", "error", err, "report", report)
		os.Exit(1)
	}
	if err := nodeapi.SetSharedStore(stateStore); err != nil {
		logger.Error("初始化节点公共控制面存储失败", "error", err)
		os.Exit(1)
	}
	if err := nodeapi.SetCoreDatabase(stateStore.DB()); err != nil {
		logger.Error("初始化认证 SQLite 存储失败", "error", err)
		os.Exit(1)
	}
	if err := nodeapi.RecoverDeploymentState(cfg.DataDir); err != nil {
		logger.Error("恢复部署制品状态失败", "error", err)
		os.Exit(1)
	}
	if _, err := role.New(cfg.NodeID, cfg.Role); err != nil {
		logger.Error("节点角色配置无效", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	mux, gatewayStore := httpMuxWithReadiness(cfg, readiness)
	if cfg.BackgroundTasks.Enabled && backgroundTasksConfigured(cfg.DataDir) {
		nodeapi.StartBackgroundTasks(ctx)
	}
	gatewayStore.Start(ctx, []string{"system", "containers", "files", "databases", "websites", "tasks"})
	// 统一安全包装器位于所有控制面和节点路由外层，避免新增路由遗漏 Session/CSRF、域名绑定和密码过期校验。
	securedMux := controlapi.NewSecurityMiddleware(mux, controlapi.SecurityMiddlewareOptions{
		DataDir: cfg.DataDir, Authorize: nodeapi.AuthorizeControlRequest, Settings: nodeapi.LoadSecuritySettings, OperationLog: nodeapi.RecordOperationLog,
	})
	readiness.SetReady()
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
	readiness.SetNotReady("服务正在关闭")
	_ = server.Shutdown(shutdownCtx)
}

func initializeUnifiedSchema(s *storage.Store) error {
	legacyMigration := storage.SQLMigration("0002-legacy-payloads", `CREATE TABLE IF NOT EXISTS legacy_payloads (
		domain TEXT NOT NULL,
		source_path TEXT NOT NULL,
		source_sha256 TEXT NOT NULL,
		payload BLOB NOT NULL,
		imported_at TEXT NOT NULL,
		PRIMARY KEY(domain, source_path, source_sha256)
	)`)
	nodeMigration := storage.SQLMigration("0004-node-terminal-control-plane", `
	CREATE TABLE IF NOT EXISTS node_hosts (id TEXT PRIMARY KEY, name TEXT NOT NULL, address TEXT NOT NULL, port INTEGER NOT NULL, user_name TEXT NOT NULL DEFAULT '', group_id INTEGER NOT NULL DEFAULT 0, payload BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_node_hosts_group_name ON node_hosts(group_id, name, id);
	CREATE TABLE IF NOT EXISTS node_quick_commands (id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL DEFAULT 'command', command TEXT NOT NULL, group_id INTEGER NOT NULL DEFAULT 0, group_belong TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', payload BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_node_quick_commands_type_name ON node_quick_commands(type, name, id);
	CREATE TABLE IF NOT EXISTS node_settings (setting_key TEXT PRIMARY KEY, payload BLOB NOT NULL, updated_at TEXT NOT NULL);`)
	databaseMigration := storage.SQLMigration("0005-database-resources", `
	CREATE TABLE IF NOT EXISTS databases (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'local', app_install_id INTEGER NOT NULL DEFAULT 0, address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 0, initial_db TEXT NOT NULL DEFAULT '', username TEXT NOT NULL DEFAULT '', password TEXT NOT NULL DEFAULT '', ssl INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_databases_type_name ON databases(type, name);
	CREATE INDEX IF NOT EXISTS idx_databases_app_install ON databases(app_install_id);
	CREATE TABLE IF NOT EXISTS database_operations (id TEXT PRIMARY KEY, type TEXT NOT NULL, target TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, message TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_database_operations_created ON database_operations(created_at DESC);
	CREATE TABLE IF NOT EXISTS resource_groups (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL, is_default INTEGER NOT NULL DEFAULT 0, is_delete INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, UNIQUE(type,name));
	CREATE INDEX IF NOT EXISTS idx_resource_groups_type_default ON resource_groups(type,is_default DESC,id);
	CREATE TABLE IF NOT EXISTS cronjobs (id TEXT PRIMARY KEY, payload BLOB NOT NULL, records BLOB NOT NULL DEFAULT '[]', updated_at TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS script_library (id TEXT PRIMARY KEY, name TEXT NOT NULL, script TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', version TEXT NOT NULL DEFAULT '', approved INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);`)
	databaseAdminMigration := storage.SQLMigration("0006-database-admin-metadata", `
	CREATE TABLE IF NOT EXISTS database_users (id INTEGER PRIMARY KEY AUTOINCREMENT, database_id INTEGER NOT NULL DEFAULT 0, database_name TEXT NOT NULL, type TEXT NOT NULL DEFAULT '', username TEXT NOT NULL, host TEXT NOT NULL DEFAULT '%', description TEXT NOT NULL DEFAULT '', password_set INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_database_users_identity ON database_users(database_name,username,host);
	CREATE INDEX IF NOT EXISTS idx_database_users_database ON database_users(database_name,id);
	CREATE TABLE IF NOT EXISTS database_grants (id INTEGER PRIMARY KEY AUTOINCREMENT, database_name TEXT NOT NULL, username TEXT NOT NULL, host TEXT NOT NULL DEFAULT '%', privileges BLOB NOT NULL DEFAULT '[]');
	CREATE UNIQUE INDEX IF NOT EXISTS idx_database_grants_identity ON database_grants(database_name,username,host);
	CREATE INDEX IF NOT EXISTS idx_database_grants_database ON database_grants(database_name,id);
	CREATE TABLE IF NOT EXISTS database_variables (database_name TEXT NOT NULL, name TEXT NOT NULL, value TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(database_name,name));
	CREATE TABLE IF NOT EXISTS database_configs (database_name TEXT PRIMARY KEY, content BLOB NOT NULL, updated_at TEXT NOT NULL);`)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return storage.ApplyMigrations(ctx, s.DB(), []storage.Migration{legacyMigration, nodeMigration, databaseMigration, databaseAdminMigration, service.WebsiteSchemaMigration()})
}

func importLegacyData(ctx context.Context, s *storage.Store, dataDir string) (storage.LegacyImportReport, error) {
	handlers := make([]storage.LegacyJSONHandler, 0, 4)
	for _, domain := range []storage.LegacyDomain{storage.LegacyDomainGroups, storage.LegacyDomainWebsites, storage.LegacyDomainDomains, storage.LegacyDomainSSL} {
		d := domain
		handlers = append(handlers, storage.LegacyJSONHandlerFunc{DomainName: d, ImportFunc: func(ctx context.Context, tx *sql.Tx, source storage.LegacyJSONSource) (int64, error) {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO legacy_payloads(domain, source_path, source_sha256, payload, imported_at) VALUES(?, ?, ?, ?, ?)`, string(source.Domain), source.Path, source.SHA256, []byte(source.Data), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				return 0, err
			}
			stateKey := map[storage.LegacyDomain]string{storage.LegacyDomainWebsites: "websites", storage.LegacyDomainDomains: "website-domains", storage.LegacyDomainSSL: "ssl", storage.LegacyDomainGroups: "groups"}[source.Domain]
			if source.Domain == storage.LegacyDomainSSL {
				switch filepath.Base(source.Path) {
				case "ssl.json":
					stateKey = "ssl-certificates"
				case "website-acme.json":
					stateKey = "website-acme"
				case "website-ca.json":
					stateKey = "website-ca"
				case "website-ca-ssls.json":
					stateKey = "website-ca-ssls"
				}
			}
			if stateKey != "" {
				if _, err := tx.ExecContext(ctx, `INSERT INTO website_state(state_key,payload,updated_at) VALUES(?,?,?) ON CONFLICT(state_key) DO NOTHING`, stateKey, []byte(source.Data), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
					return 0, err
				}
			}
			return 1, nil
		}})
	}
	return s.ImportLegacyJSON(ctx, storage.LegacyJSONImportOptions{SourceDir: dataDir, Handlers: handlers, ArchiveImported: true})
}

// readinessState 保存进程 bootstrap 状态，避免在迁移或关闭阶段错误宣告就绪。
type readinessState struct {
	ready  atomic.Bool
	reason atomic.Value
}

func newReadinessState() *readinessState {
	s := &readinessState{}
	s.reason.Store("服务正在初始化")
	return s
}

func (s *readinessState) SetReady() {
	s.reason.Store("")
	s.ready.Store(true)
}

func (s *readinessState) SetNotReady(reason string) {
	if strings.TrimSpace(reason) == "" {
		reason = "服务尚未完成初始化"
	}
	s.reason.Store(reason)
	s.ready.Store(false)
}

func (s *readinessState) ServeHTTP(w http.ResponseWriter) {
	if s.ready.Load() {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]string{"status": "ready"}})
		return
	}
	reason, _ := s.reason.Load().(string)
	wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "NOT_READY"}, "message": reason})
}

// backgroundTasksConfigured 只在数据目录确实存在可执行任务时启用周期维护。
func backgroundTasksConfigured(dataDir string) bool {
	cronPath := filepath.Join(dataDir, "cronjobs.json")
	if raw, err := os.ReadFile(cronPath); err == nil {
		var payload struct {
			Items map[string]struct {
				Status string `json:"status"`
			} `json:"items"`
		}
		if json.Unmarshal(raw, &payload) == nil {
			for _, item := range payload.Items {
				if strings.EqualFold(strings.TrimSpace(item.Status), "enabled") {
					return true
				}
			}
		}
	}
	return false
}

func httpMux(cfg config.Config) (*http.ServeMux, *controlapi.GatewayStateStore) {
	readiness := newReadinessState()
	readiness.SetReady()
	return httpMuxWithReadiness(cfg, readiness)
}

func httpMuxWithReadiness(cfg config.Config, readiness *readinessState) (*http.ServeMux, *controlapi.GatewayStateStore) {
	mux := http.NewServeMux()
	// 静态资源必须在 API 兼容层之前命中文件系统，否则浏览器会收到 JSON 错误响应并拒绝执行模块脚本。
	staticRoot := filepath.Join("web", "dist")
	staticFiles := http.StripPrefix("/", http.FileServer(http.Dir(staticRoot)))
	serveIndex := func(w http.ResponseWriter, r *http.Request, index string) {
		// SPA 入口不是哈希资源；禁止缓存可避免部署新版本后继续加载旧 chunk。
		w.Header().Set("Cache-Control", "no-store, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		http.ServeFile(w, r, index)
	}
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
		serveIndex(w, r, index)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]string{"status": "ok"}})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		readiness.ServeHTTP(w)
	})
	mux.HandleFunc("GET /api/v2/health/check", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]string{"status": "ok"}})
	})
	gatewayStore := controlapi.Register(mux, cfg.NodeID, cfg.Role, nodeapi.AuthorizeControlRequest)
	nodeMux := http.NewServeMux()
	nodeapi.Register(nodeMux)
	// 节点执行面统一经过本机会话鉴权；流接口保留各自的短期 Token 校验。
	// 节点执行面先进行本机会话校验，再按 CurrentNode/operateNode 透传到已登记节点。
	// 透传请求由共享密钥和 role epoch 保护，目标节点未登记或签名失效时明确返回错误。
	securedNodeMux := authenticateNodeAPI(nodeMux)
	mux.Handle("/api/v2/", nodeapi.NewNodeRelay(securedNodeMux, nodeapi.RelayOptions{
		DataDir: cfg.DataDir,
		NodeID:  cfg.NodeID,
		Secret:  []byte(os.Getenv("WORKMESH_LINK_SECRET")),
		Timeout: cfg.RequestTimeout,
	}))
	return mux, gatewayStore
}

func authenticateNodeAPI(next *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := next.Handler(r)
		if pattern == "" || publicNodeAPIPath(r) || selfAuthenticatedStreamPath(r.URL.Path) || nodeapi.IsForwardedRequestVerified(r) {
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
