// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/config"
	"github.com/todaybin/workmesh-server/internal/webassets"
	nodeapi "github.com/todaybin/workmesh-server/node/api"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// registerStaticRoutes 注册内置前端静态资源、公开资源和 SPA 回退路由。
func registerStaticRoutes(mux *http.ServeMux) {
	registerStaticRoutesWithFS(mux, webassets.Dist)
}

// registerStaticRoutesWithFS 使用指定前端文件系统注册静态资源。
//
// 生产环境传入 webassets.Dist，保证服务启动目录变化不会影响页面加载；
// 测试可以注入临时文件系统，覆盖资源缺失、SPA 回退和 MIME 类型行为。
func registerStaticRoutesWithFS(mux *http.ServeMux, frontend fs.FS) {
	staticFiles := http.StripPrefix("/", http.FileServer(http.FS(frontend)))
	mux.HandleFunc("GET /assets/{filepath...}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, max-age=2628000, immutable")
		staticFiles.ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /public/{filepath...}", publicAssetHandler(frontend))
	mux.HandleFunc("GET /favicon.ico", faviconHandler(frontend))
	mux.HandleFunc("GET /favicon.ico/{filepath...}", faviconAssetHandler(frontend))
	registerSwaggerRoute(mux)
	mux.HandleFunc("GET /api/v2/images/{filename...}", apiPublicAssetHandler(frontend))
	mux.HandleFunc("GET /api/v2/static/{filename...}", apiStaticAssetHandler(frontend))
	mux.HandleFunc("/", spaFallbackHandler(frontend))
}

// apiPublicAssetHandler 提供旧前端 API 图片入口，并拒绝路径穿越。
func apiPublicAssetHandler(frontend fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relative := strings.TrimPrefix(r.URL.Path, "/api/v2/images/")
		if relative == "favicon" {
			relative = "favicon.png"
		}
		if relative != "favicon.png" {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "ASSET_NOT_FOUND"}})
			return
		}
		serveCheckedEmbeddedAsset(w, r, frontend, relative, true, embeddedAssetOptions{})
	}
}

// apiStaticAssetHandler 提供兼容旧前端的 API 静态资源入口。
func apiStaticAssetHandler(frontend fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relative := strings.TrimPrefix(r.URL.Path, "/api/v2/static/")
		if relative != "china.json" && relative != "world.json" {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "ASSET_NOT_FOUND"}})
			return
		}
		serveCheckedEmbeddedAsset(w, r, frontend, "static/"+relative, true, embeddedAssetOptions{
			cacheControl: "private, max-age=2628000",
			contentType:  "application/json; charset=utf-8",
			includeETag:  true,
		})
	}
}

// publicAssetHandler 提供兼容旧前端的公开静态资源入口。
func publicAssetHandler(frontend fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relative := strings.TrimPrefix(r.URL.Path, "/public/")
		if relative != "favicon.png" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		serveCheckedEmbeddedAsset(w, r, frontend, relative, false, embeddedAssetOptions{})
	}
}

// faviconHandler 返回根 favicon，缺失时维持原有 404 行为。
func faviconHandler(frontend fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serveCheckedEmbeddedAsset(w, r, frontend, "favicon.png", false, embeddedAssetOptions{})
	}
}

// faviconAssetHandler 保留兼容路由，但只允许默认 favicon 文件。
func faviconAssetHandler(frontend fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relative := strings.TrimPrefix(r.URL.Path, "/favicon.ico/")
		if relative != "favicon.png" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		serveCheckedEmbeddedAsset(w, r, frontend, relative, false, embeddedAssetOptions{})
	}
}

type embeddedAssetOptions struct {
	cacheControl string
	contentType  string
	includeETag  bool
}

// serveCheckedEmbeddedAsset 校验资源路径并从内置文件系统返回资源。
func serveCheckedEmbeddedAsset(w http.ResponseWriter, r *http.Request, frontend fs.FS, relative string, jsonErrors bool, options embeddedAssetOptions) {
	clean, ok := cleanAssetPath(relative)
	if !ok {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "INVALID_ASSET_PATH"}})
		return
	}
	file, err := frontend.Open(clean)
	if err != nil {
		if jsonErrors {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "ASSET_NOT_FOUND"}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		if jsonErrors {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "ASSET_NOT_FOUND"}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}
	content, err := fs.ReadFile(frontend, clean)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if options.cacheControl != "" {
		w.Header().Set("Cache-Control", options.cacheControl)
	}
	if options.contentType != "" {
		w.Header().Set("Content-Type", options.contentType)
	}
	if options.includeETag {
		sum := sha256.Sum256(content)
		etag := `"` + hex.EncodeToString(sum[:]) + `"`
		w.Header().Set("ETag", etag)
		if strings.TrimSpace(r.Header.Get("If-None-Match")) == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	http.ServeContent(w, r, clean, info.ModTime(), bytes.NewReader(content))
}

// cleanAssetPath 将 URL 相对路径转换为安全的嵌入文件路径。
func cleanAssetPath(relative string) (string, bool) {
	clean := pathClean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

// pathClean 只使用 URL 的斜杠，避免平台路径规则影响嵌入文件访问。
func pathClean(value string) string {
	value = strings.ReplaceAll(value, `\`, "/")
	parts := strings.Split(value, "/")
	stack := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(stack) == 0 {
				return ".."
			}
			stack = stack[:len(stack)-1]
		default:
			stack = append(stack, part)
		}
	}
	if len(stack) == 0 {
		return "."
	}
	return strings.Join(stack, "/")
}

// registerSwaggerRoute 注册兼容的 OpenAPI 文档入口。
func registerSwaggerRoute(mux *http.ServeMux) {
	// 具体文档由发布包提供时再替换响应体。
	mux.HandleFunc("GET /swagger/{any...}", func(w http.ResponseWriter, _ *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"openapi": "3.0.0", "info": map[string]any{"title": "WorkMesh Server API", "version": "v2"}, "servers": []any{map[string]any{"url": "/api/v2"}}})
	})
}

// spaFallbackHandler 提供前端 history 路由回退，并保护 API 404 响应为 JSON。
func spaFallbackHandler(frontend fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(frontend, "index.html"); err != nil {
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]string{"service": "workmesh-server"}})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "ROUTE_NOT_FOUND", "path": r.URL.Path}})
			return
		}
		if serveExistingEmbeddedFile(w, r, frontend) {
			return
		}
		serveSPAIndex(w, r, frontend)
	}
}

// serveExistingEmbeddedFile 优先返回发布包中实际存在的静态文件。
func serveExistingEmbeddedFile(w http.ResponseWriter, r *http.Request, frontend fs.FS) bool {
	clean := pathClean(strings.TrimPrefix(r.URL.Path, "/"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return false
	}
	info, err := fs.Stat(frontend, clean)
	if err != nil || info.IsDir() {
		return false
	}
	content, err := fs.ReadFile(frontend, clean)
	if err != nil {
		return false
	}
	http.ServeContent(w, r, clean, info.ModTime(), bytes.NewReader(content))
	return true
}

// serveSPAIndex 返回不可缓存的 SPA 入口，避免部署后继续使用旧 chunk。
func serveSPAIndex(w http.ResponseWriter, r *http.Request, frontend fs.FS) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	content, err := fs.ReadFile(frontend, "index.html")
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	http.ServeContent(w, r, "index.html", fileModTime(frontend, "index.html"), bytes.NewReader(content))
}

func fileModTime(frontend fs.FS, name string) (modTime time.Time) {
	info, err := fs.Stat(frontend, name)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// registerHealthRoutes 注册进程健康、就绪和兼容健康检查路由。
func registerHealthRoutes(mux *http.ServeMux, readiness *readinessState) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]string{"status": "ok"}})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		readiness.ServeHTTP(w)
	})
	mux.HandleFunc("GET /api/v2/health/check", func(w http.ResponseWriter, _ *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]string{"status": "ok"}})
	})
}

// registerNodeRoutes 注册节点执行面，并挂载本地鉴权和主次节点透传。
func registerNodeRoutes(mux *http.ServeMux, cfg config.Config) {
	nodeMux := http.NewServeMux()
	nodeapi.Register(nodeMux)
	securedNodeMux := authenticateNodeAPI(nodeMux)
	options := nodeapi.RelayOptions{
		DataDir:    cfg.DataDir,
		NodeID:     cfg.NodeID,
		Secret:     []byte(os.Getenv("WORKMESH_LINK_SECRET")),
		Timeout:    cfg.RequestTimeout,
		MaxRetries: 1,
		RoleEpoch:  roleEpochLookup(cfg.NodeID),
		NodeLookup: func(ctx context.Context, nodeID string) (string, string, bool) {
			return lookupNodeEndpoint(ctx, nodeID)
		},
	}
	mux.Handle("/api/v2/", nodeapi.NewNodeRelay(securedNodeMux, options))
}

// roleEpochLookup 返回读取当前节点角色 epoch 的回调。
func roleEpochLookup(nodeID string) func(context.Context) (uint64, error) {
	return func(ctx context.Context) (uint64, error) {
		var epoch uint64
		repository, err := nodeapi.SharedRepository()
		if err != nil {
			return 1, nil
		}
		err = repository.QueryRowContext(ctx, `SELECT role_epoch FROM role_state WHERE node_id=?`, nodeID).Scan(&epoch)
		if err == sql.ErrNoRows {
			return 1, nil
		}
		return epoch, err
	}
}

// lookupNodeEndpoint 从统一 SQLite 查询主次节点透传地址。
func lookupNodeEndpoint(ctx context.Context, nodeID string) (string, string, bool) {
	var endpoint, addr string
	repository, err := nodeapi.SharedRepository()
	if err != nil {
		return "", "", false
	}
	err = repository.QueryRowContext(ctx, `SELECT endpoint,addr FROM role_nodes WHERE node_id=?`, nodeID).Scan(&endpoint, &addr)
	return endpoint, addr, err == nil
}
