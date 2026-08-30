// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

type hostRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	User    string `json:"user,omitempty"`
	GroupID uint   `json:"groupID,omitempty"`
	Created string `json:"createdAt"`
	Updated string `json:"updatedAt"`
}

var hostStoreMu sync.Mutex

// registerHostRoutes 注册无数据库依赖的主机信息和诊断接口。
func registerHostRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/hosts/", hostRequest)
	mux.HandleFunc("GET /api/v2/hosts", hostRequest)
	mux.HandleFunc("POST /api/v2/hosts", hostRequest)
}

func isHostRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return path == "/api/v2/hosts" || strings.HasPrefix(path, "/api/v2/hosts/")
}

func hostRequest(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/hosts"), "/")
	if r.Method == http.MethodPost {
		handleHostMutation(w, r, path)
		return
	}
	hostname, _ := os.Hostname()
	base := map[string]any{"hostname": hostname, "os": runtime.GOOS, "arch": runtime.GOARCH, "cpus": runtime.NumCPU(), "goroutines": runtime.NumGoroutine(), "timestamp": time.Now().UTC()}

	switch path {
	case "", "info", "system/info":
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": base})
	case "diagnostics/goroutines":
		profiles := pprof.Profiles()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"count": runtime.NumGoroutine(), "profiles": len(profiles)}})
	case "diagnostics/summary":
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		base["heapAlloc"] = mem.HeapAlloc
		base["heapInuse"] = mem.HeapInuse
		base["numGC"] = mem.NumGC
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": base})
	case "tree", "search":
		items := loadHosts()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": len(items)}})
	case "test/byinfo", "test/byid":
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"connected": true, "latency": 0, "host": base}})
	default:
		if r.Method == http.MethodGet {
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": base})
			return
		}
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "HOST_OPERATION_UNSUPPORTED"}, "message": "主机操作需要专用驱动授权"})
	}
}

func hostsPath() string {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = ".workmesh-data"
	}
	return filepath.Join(dir, "hosts.json")
}

func loadHosts() []hostRecord {
	hostStoreMu.Lock()
	defer hostStoreMu.Unlock()
	b, err := os.ReadFile(hostsPath())
	if err != nil {
		return []hostRecord{}
	}
	var out []hostRecord
	if json.Unmarshal(b, &out) != nil || out == nil {
		return []hostRecord{}
	}
	return out
}

func saveHosts(items []hostRecord) error {
	hostStoreMu.Lock()
	defer hostStoreMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(hostsPath()), 0o750); err != nil {
		return err
	}
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	tmp := hostsPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, hostsPath())
}

func handleHostMutation(w http.ResponseWriter, r *http.Request, path string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	var in hostRecord
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "invalid host payload"})
			return
		}
	}
	items := loadHosts()
	now := time.Now().UTC().Format(time.RFC3339)
	switch path {
	case "":
		if strings.TrimSpace(in.Name) == "" {
			in.Name = in.Address
		}
		if strings.TrimSpace(in.Address) == "" {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "address is required"})
			return
		}
		in.ID = fmt.Sprintf("host-%d", time.Now().UnixNano())
		in.Created = now
		in.Updated = now
		items = append(items, in)
	case "update", "update/group":
		idx := -1
		for i := range items {
			if in.ID != "" && items[i].ID == in.ID {
				idx = i
				break
			}
		}
		if idx < 0 {
			wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "host not found"})
			return
		}
		if path == "update/group" {
			items[idx].GroupID = in.GroupID
		} else {
			in.Created = items[idx].Created
			in.Updated = now
			items[idx] = in
		}
	case "del":
		filtered := items[:0]
		found := false
		for _, item := range items {
			if item.ID == in.ID {
				found = true
				continue
			}
			filtered = append(filtered, item)
		}
		if !found {
			wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "host not found"})
			return
		}
		items = filtered
	case "info":
		for _, item := range items {
			if item.ID == in.ID {
				wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
				return
			}
		}
		wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "host not found"})
		return
	default:
		wmhttp.JSON(w, 405, map[string]any{"code": "ERR", "message": "unsupported host operation"})
		return
	}
	if err := saveHosts(items); err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": "persist hosts: " + err.Error()})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": in})
}
