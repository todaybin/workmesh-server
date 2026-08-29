// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// registerHostRoutes 注册无数据库依赖的主机信息和诊断接口。
func registerHostRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/hosts/", hostRequest)
	mux.HandleFunc("GET /api/v2/hosts", hostRequest)
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
	case "tree", "search", "test/byinfo", "test/byid":
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": []any{base}, "total": 1, "page": 1, "pageSize": 50}})
	default:
		if r.Method == http.MethodGet {
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": base})
			return
		}
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "HOST_OPERATION_UNSUPPORTED"}, "message": "主机操作需要专用驱动授权"})
	}
}
