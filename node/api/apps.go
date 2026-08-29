// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type appRecord struct {
	ID        string         `json:"id"`
	Key       string         `json:"key"`
	Name      string         `json:"name"`
	Version   string         `json:"version"`
	Status    string         `json:"status"`
	Config    map[string]any `json:"config,omitempty"`
	UpdatedAt time.Time      `json:"updatedAt"`
}
type appStore struct {
	mu   sync.RWMutex
	path string
	Apps []appRecord `json:"apps"`
}

var appStoreMu sync.Mutex
var appStoreInstance *appStore

func getAppStore() *appStore {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = ".workmesh-data"
	}
	path := filepath.Join(dir, "apps.json")
	appStoreMu.Lock()
	defer appStoreMu.Unlock()
	if appStoreInstance != nil && appStoreInstance.path == path {
		return appStoreInstance
	}
	s := &appStore{path: path}
	if b, e := os.ReadFile(path); e == nil {
		_ = json.Unmarshal(b, s)
	}
	appStoreInstance = s
	return s
}
func (s *appStore) saveLocked() error {
	if e := os.MkdirAll(filepath.Dir(s.path), 0o750); e != nil {
		return e
	}
	b, e := json.Marshal(s)
	if e != nil {
		return e
	}
	tmp := s.path + ".tmp"
	if e = os.WriteFile(tmp, b, 0o600); e != nil {
		return e
	}
	return os.Rename(tmp, s.path)
}
func appBody(r *http.Request) map[string]any {
	var v map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&v)
	}
	if v == nil {
		v = map[string]any{}
	}
	return v
}
func appValue(v map[string]any, keys ...string) string {
	for _, k := range keys {
		if x, ok := v[k].(string); ok && strings.TrimSpace(x) != "" {
			return strings.TrimSpace(x)
		}
	}
	return ""
}
func appOK(w http.ResponseWriter, d any) { runtimeOK(w, d) }

// RegisterAppRoutes 注册应用目录和已安装应用兼容接口。
func RegisterAppRoutes(mux *http.ServeMux) {
	s := getAppStore()
	list := func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		a := append([]appRecord(nil), s.Apps...)
		s.mu.RUnlock()
		appOK(w, map[string]any{"items": a, "total": len(a)})
	}
	for _, p := range []string{"/api/v2/apps/installed/list", "/api/v2/apps/installed/search", "/api/v2/apps/search", "/api/v2/apps/sync/local", "/api/v2/apps/sync/remote"} {
		mux.HandleFunc("POST "+p, list)
	}
	for _, p := range []string{"/api/v2/apps/checkupdate", "/api/v2/apps/ignored/detail", "/api/v2/apps/tags"} {
		mux.HandleFunc("GET "+p, func(w http.ResponseWriter, _ *http.Request) { appOK(w, map[string]any{"items": []any{}, "total": 0}) })
	}
	mux.HandleFunc("GET /api/v2/apps/installed/list", list)
	mux.HandleFunc("GET /api/v2/apps/installed/info/{appInstallId}", func(w http.ResponseWriter, r *http.Request) { appGet(w, s, r.PathValue("appInstallId")) })
	mux.HandleFunc("GET /api/v2/apps/installed/params/{appInstallId}", func(w http.ResponseWriter, r *http.Request) { appGet(w, s, r.PathValue("appInstallId")) })
	mux.HandleFunc("GET /api/v2/apps/installed/delete/check/{appInstallId}", func(w http.ResponseWriter, r *http.Request) {
		appOK(w, map[string]any{"id": r.PathValue("appInstallId"), "allowed": true})
	})
	mux.HandleFunc("GET /api/v2/apps/{key}", func(w http.ResponseWriter, r *http.Request) {
		appOK(w, map[string]any{"key": r.PathValue("key"), "available": true})
	})
	for _, p := range []string{"/api/v2/apps/detail/{appId}/{version}/{type}", "/api/v2/apps/detail/node/{appKey}/{version}", "/api/v2/apps/details/{id}", "/api/v2/apps/services/{key}", "/api/v2/apps/icon/{key}"} {
		mux.HandleFunc("GET "+p, func(w http.ResponseWriter, r *http.Request) {
			appOK(w, map[string]any{"path": r.URL.Path, "available": true})
		})
	}
	for _, p := range []string{"/api/v2/apps/install", "/api/v2/apps/installed/check", "/api/v2/apps/installed/conf", "/api/v2/apps/installed/config/update", "/api/v2/apps/installed/conninfo", "/api/v2/apps/installed/ignore", "/api/v2/apps/installed/loadport", "/api/v2/apps/installed/op", "/api/v2/apps/installed/params/update", "/api/v2/apps/installed/port/change", "/api/v2/apps/installed/sort/update", "/api/v2/apps/installed/sync", "/api/v2/apps/installed/update/versions", "/api/v2/apps/ignored/cancel"} {
		mux.HandleFunc("POST "+p, func(w http.ResponseWriter, r *http.Request) {
			v := appBody(r)
			id := appValue(v, "id", "appId", "appInstallId", "key")
			if id == "" {
				id = appValue(v, "name")
			}
			if p == "/api/v2/apps/install" && id != "" {
				s.mu.Lock()
				s.Apps = append(s.Apps, appRecord{ID: id, Key: id, Name: appValue(v, "name"), Version: appValue(v, "version"), Status: "running", Config: v, UpdatedAt: time.Now().UTC()})
				_ = s.saveLocked()
				s.mu.Unlock()
			}
			appOK(w, map[string]any{"id": id, "status": "accepted", "config": v})
		})
	}
}
func appGet(w http.ResponseWriter, s *appStore, id string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.Apps {
		if a.ID == id || a.Key == id {
			appOK(w, a)
			return
		}
	}
	appOK(w, map[string]any{"id": id, "status": "not_installed"})
}
func isAppRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	p := pattern
	if len(parts) == 2 {
		p = parts[1]
	}
	return p == "/api/v2/apps" || strings.HasPrefix(p, "/api/v2/apps/")
}
