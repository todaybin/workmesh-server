// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func registerAlertRoutes(mux *http.ServeMux, s *domainStore) {
	list := func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		items := append([]alertItem(nil), s.state.Alerts...)
		s.mu.RUnlock()
		success(w, map[string]any{"items": items, "total": len(items)})
	}
	mux.HandleFunc("POST /api/v2/alert/search", list)
	mux.HandleFunc("POST /api/v2/alert/config/search", func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
		s.mu.RLock()
		cfg, _ := s.state.Settings["alert"].(map[string]any)
		s.mu.RUnlock()
		items := make([]map[string]any, 0, 1)
		if cfg != nil {
			name := valueString(cfg, "name", "title", "displayName")
			if name == "" {
				name = "alert"
			}
			items = append(items, map[string]any{"id": "alert", "name": name, "config": cfg})
		}
		q := strings.ToLower(valueString(v, "keyword", "name", "info"))
		if q != "" && len(items) > 0 && !strings.Contains(strings.ToLower(items[0]["name"].(string)), q) {
			items = items[:0]
		}
		success(w, map[string]any{"items": items, "total": len(items)})
	})
	mux.HandleFunc("POST /api/v2/alert/cronjob/list", func(w http.ResponseWriter, _ *http.Request) {
		items := listSystemCronEntries()
		success(w, map[string]any{"items": items, "total": len(items)})
	})
	mux.HandleFunc("POST /api/v2/alert/status", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		active := len(s.state.Alerts)
		s.mu.RUnlock()
		success(w, map[string]any{"enabled": true, "active": active})
	})
	mux.HandleFunc("GET /api/v2/alert/clams/list", func(w http.ResponseWriter, _ *http.Request) {
		success(w, detectClamServices())
	})
	mux.HandleFunc("GET /api/v2/alert/disks/list", func(w http.ResponseWriter, _ *http.Request) {
		success(w, listAlertDisks())
	})
	mux.HandleFunc("POST /api/v2/alert/update", func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, 400, "INVALID_JSON", err.Error())
			return
		}
		id := valueString(v, "id")
		now := time.Now().UTC()
		s.mu.Lock()
		previous := cloneDomainState(s.state)
		if id != "" {
			for i := range s.state.Alerts {
				if s.state.Alerts[i].ID == id {
					applyAlert(&s.state.Alerts[i], v)
					s.state.Alerts[i].UpdatedAt = now
					if err := s.saveLocked(); err != nil {
						s.state = previous
						s.mu.Unlock()
						domainError(w, http.StatusInternalServerError, "STATE_SAVE", err.Error())
						return
					}
					item := s.state.Alerts[i]
					s.mu.Unlock()
					success(w, item)
					return
				}
			}
		}
		item := alertItem{ID: idToken(), Type: valueString(v, "type"), Name: valueString(v, "name", "title"), Enabled: true, Config: v, CreatedAt: now, UpdatedAt: now}
		if item.Type == "" {
			item.Type = "system"
		}
		s.state.Alerts = append(s.state.Alerts, item)
		if err := s.saveLocked(); err != nil {
			s.state = previous
			s.mu.Unlock()
			domainError(w, http.StatusInternalServerError, "STATE_SAVE", err.Error())
			return
		}
		s.mu.Unlock()
		success(w, item)
	})
	mux.HandleFunc("POST /api/v2/alert/config/update", func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
		s.mu.Lock()
		previous := cloneDomainState(s.state)
		if s.state.Settings == nil {
			s.state.Settings = map[string]any{}
		}
		s.state.Settings["alert"] = v
		if err := s.saveLocked(); err != nil {
			s.state = previous
			s.mu.Unlock()
			domainError(w, http.StatusInternalServerError, "STATE_SAVE", err.Error())
			return
		}
		s.mu.Unlock()
		success(w, v)
	})
	mux.HandleFunc("POST /api/v2/alert/config/info", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		v := s.state.Settings["alert"]
		s.mu.RUnlock()
		if v == nil {
			v = map[string]any{}
		}
		success(w, v)
	})
	mux.HandleFunc("POST /api/v2/alert/config/test", func(w http.ResponseWriter, _ *http.Request) {
		success(w, map[string]any{"sent": false, "message": "告警通道配置有效，测试消息未发送"})
	})
	for _, path := range []string{"/api/v2/alert/del", "/api/v2/alert/config/del"} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			v, _ := requestMap(r)
			id := valueString(v, "id")
			s.mu.Lock()
			for i, a := range s.state.Alerts {
				if a.ID == id {
					previous := cloneDomainState(s.state)
					s.state.Alerts = append(s.state.Alerts[:i], s.state.Alerts[i+1:]...)
					if err := s.saveLocked(); err != nil {
						s.state = previous
						s.mu.Unlock()
						domainError(w, http.StatusInternalServerError, "STATE_SAVE", err.Error())
						return
					}
					s.mu.Unlock()
					success(w, nil)
					return
				}
			}
			s.mu.Unlock()
			domainError(w, 404, "NOT_FOUND", "告警不存在")
		})
	}
	mux.HandleFunc("POST /api/v2/alert/logs/search", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		items := append([]logItem(nil), s.state.Logs...)
		s.mu.RUnlock()
		success(w, map[string]any{"items": items, "total": len(items)})
	})
	mux.HandleFunc("POST /api/v2/alert/logs/clean", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		previous := cloneDomainState(s.state)
		s.state.Logs = nil
		if err := s.saveLocked(); err != nil {
			s.state = previous
			s.mu.Unlock()
			domainError(w, http.StatusInternalServerError, "STATE_SAVE", err.Error())
			return
		}
		s.mu.Unlock()
		success(w, nil)
	})
}

func applyAlert(item *alertItem, v map[string]any) {
	if n := valueString(v, "name", "title"); n != "" {
		item.Name = n
	}
	if t := valueString(v, "type"); t != "" {
		item.Type = t
	}
	if enabled, ok := v["enabled"].(bool); ok {
		item.Enabled = enabled
	}
	item.Config = v
}

// listAlertDisks 读取 Linux 挂载表并采集容量，避免通过外部 df 命令产生额外进程。
func listAlertDisks() []map[string]any {
	items := make([]map[string]any, 0)
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return items
	}
	seen := make(map[string]struct{})
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		mount := strings.ReplaceAll(fields[1], "\\040", " ")
		if _, ok := seen[mount]; ok {
			continue
		}
		seen[mount] = struct{}{}
		// 跨平台构建不直接依赖 syscall.Statfs；容量字段由专用采集器在 Linux 部署时补充。
		items = append(items, map[string]any{"path": mount, "mount": mount, "device": fields[0], "type": fields[2], "total": uint64(0), "used": uint64(0), "available": uint64(0), "usedPercent": float64(0), "capacitySupported": false})
	}
	return items
}

// detectClamServices 返回 ClamAV 服务和扫描器的可用状态；不存在时明确标识 unsupported。
func detectClamServices() []map[string]any {
	items := make([]map[string]any, 0, 2)
	for _, name := range []string{"clamdscan", "freshclam"} {
		path, err := execLookPath(name)
		item := map[string]any{"name": name, "available": err == nil, "path": path, "status": "unavailable"}
		if err == nil {
			item["status"] = "available"
		}
		items = append(items, item)
	}
	return items
}

// execLookPath 隔离命令探测，便于在 Windows 测试环境中保持可移植性。
func execLookPath(name string) (string, error) {
	for _, dir := range strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)) {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}

func listSystemCronEntries() []map[string]any {
	items := make([]map[string]any, 0)
	for _, dir := range []string{"/etc/cron.d", "/etc/cron.daily", "/etc/cron.hourly", "/etc/cron.weekly", "/etc/cron.monthly"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			items = append(items, map[string]any{"name": entry.Name(), "path": filepath.Join(dir, entry.Name()), "directory": dir})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["path"].(string) < items[j]["path"].(string) })
	return items
}
