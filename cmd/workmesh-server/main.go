// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	"github.com/todaybin/workmesh-server/internal/webassets"
	nodeapi "github.com/todaybin/workmesh-server/node/api"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"github.com/todaybin/workmesh-server/runtime/log"
)

func main() {
	cfg, configErr := config.Load()
	if configErr != nil {
		_, _ = fmt.Fprintln(os.Stderr, "加载服务配置失败:", configErr)
		os.Exit(1)
	}
	if err := runServer(cfg); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// runServer 完成单进程服务的存储初始化、路由装配和优雅退出。
func runServer(cfg config.Config) error {
	if err := cfg.ApplyRuntimeEnvironment(); err != nil {
		return fmt.Errorf("同步服务配置失败: %w", err)
	}
	if err := initializeDataDir(cfg.DataDir); err != nil {
		return fmt.Errorf("初始化数据目录失败: %w", err)
	}
	// 安全入口由 functional_domain_state 中的共享 SQLite 持久化状态提供。
	// 启动时不得从旧 domains.json 生成随机入口，否则每次重新部署/重启
	// 都会造成入口变化并覆盖用户看到的地址。
	if handled, err := runCLI(os.Args[1:], cfg.DataDir); handled {
		return err
	}
	logger := log.New(nil)
	if logWriter, openErr := log.Open(log.Config{Path: filepath.Join(cfg.DataDir, "logs", "server.log")}); openErr == nil {
		defer logWriter.Close()
		logger = log.New(io.MultiWriter(os.Stderr, logWriter))
	}
	readiness := newReadinessState()
	readiness.SetNotReady("正在初始化统一存储")
	stateStore, err := initializeServerState(cfg)
	if err != nil {
		logger.Error("初始化统一存储失败", "error", err)
		return err
	}
	defer stateStore.Close()
	return runHTTPService(cfg, readiness, logger)
}

// runHTTPService 启动后台任务、网关和 HTTP 服务，并等待退出信号。
func runHTTPService(cfg config.Config, readiness *readinessState, logger interface {
	Error(string, ...any)
	Info(string, ...any)
}) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	mux, gatewayStore := httpMuxWithReadiness(cfg, readiness)
	gatewayStore.SetResourceSnapshotProvider(nodeapi.RuntimeResourceSnapshot)
	gatewayStore.SetResourcePolicyConsumer(nodeapi.ApplyRemoteResourcePolicy)
	// SSL 自动续期和计划任务需要在没有现存 Cronjob 时也启动扫描器；
	// 具体任务是否到期由各自的 SQLite 状态决定。
	if cfg.BackgroundTasks.Enabled {
		nodeapi.StartBackgroundTasks(ctx)
	}
	gatewayStore.Start(ctx, []string{"system", "containers", "files", "databases", "websites", "tasks"})
	// 统一安全包装器位于所有控制面和节点路由外层，避免新增路由遗漏 Session/CSRF、域名绑定和密码过期校验。
	securedMux := controlapi.NewSecurityMiddleware(mux, controlapi.SecurityMiddlewareOptions{
		DataDir: cfg.DataDir, Authorize: nodeapi.AuthorizeControlRequest, SelfAuthenticated: nodeapi.IsAgentRuntimeRequest, Settings: nodeapi.LoadSecuritySettings, OperationLog: nodeapi.RecordOperationLog,
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
	return nil
}

// unifiedSchemaMigrations 返回统一 SQLite 的稳定启动迁移顺序。
//
// 迁移 ID 和 checksum 是持久化兼容契约：已经应用的迁移不能改写
// SQL 或 checksum。0006 的两个领域迁移必须先于 0007，数据库备份和
// 运行时状态则严格按 0008、0009 顺序追加。
func unifiedSchemaMigrations() []storage.Migration {
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
	return []storage.Migration{
		legacyMigration,
		nodeMigration,
		databaseMigration,
		databaseAdminMigration,
		service.WebsiteSchemaMigration(),
		service.DatabaseContainerNameMigration(),
		service.DatabaseBackupSchemaMigration(),
		service.DatabaseRuntimeStateSchemaMigration(),
		service.WebsiteDefaultHTMLMigration(),
		nodeapi.WebsiteTemplateMigration(),
		controlapi.GatewayBindingMigration(),
		nodeapi.HostMonitorMigration(),
	}
}

func initializeUnifiedSchema(s *storage.Store) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return storage.ApplyMigrations(ctx, s.DB(), unifiedSchemaMigrations())
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

// backgroundTasksConfigured 保留给兼容调用方查询 SQLite 计划任务状态。
func backgroundTasksConfigured(dataDir string) bool {
	_ = dataDir
	return nodeapi.BackgroundTasksConfigured()
}

func httpMux(cfg config.Config) (*http.ServeMux, *controlapi.GatewayStateStore) {
	readiness := newReadinessState()
	readiness.SetReady()
	return httpMuxWithReadiness(cfg, readiness)
}

func httpMuxWithReadiness(cfg config.Config, readiness *readinessState) (*http.ServeMux, *controlapi.GatewayStateStore) {
	return httpMuxWithReadinessAndFrontend(cfg, readiness, webassets.Dist)
}

func httpMuxWithReadinessAndFrontend(cfg config.Config, readiness *readinessState, frontend fs.FS) (*http.ServeMux, *controlapi.GatewayStateStore) {
	mux := http.NewServeMux()
	controlapi.SetControlDatabase(nodeapi.SharedDatabase())
	controlapi.SetGatewayDatabase(nodeapi.SharedDatabase())
	registerStaticRoutesWithFS(mux, frontend)
	registerHealthRoutes(mux, readiness)
	gatewayStore := controlapi.Register(mux, cfg.NodeID, cfg.Role, nodeapi.AuthorizeControlRequest)
	registerNodeRoutes(mux, cfg)
	return mux, gatewayStore
}

func authenticateNodeAPI(next *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := next.Handler(r)
		if pattern == "" || publicNodeAPIPath(r) || selfAuthenticatedStreamPath(r.URL.Path) || nodeapi.IsAgentRuntimeRequest(r) || nodeapi.IsForwardedRequestVerified(r) {
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
	case "/api/v2/process/ws", "/api/v2/containers/search/log", "/api/v2/files/wget/process", "/api/v2/hosts/terminal/local", "/api/v2/hosts/terminal/container", "/api/v2/hosts/terminal/ssh", "/api/v2/core/script/run":
		return true
	default:
		return false
	}
}
