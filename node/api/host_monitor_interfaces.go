// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"os"
	"sort"
	"strings"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// registerHostMonitorInterfaces 注册网卡和磁盘选项查询，数据直接来自本机 proc 文件系统。
func registerHostMonitorInterfaces(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/hosts/monitor/netoptions", func(w http.ResponseWriter, _ *http.Request) {
		items := []string{"all"}
		seen := map[string]bool{"all": true}
		if data, err := os.ReadFile("/proc/net/dev"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) != 2 {
					continue
				}
				name := strings.TrimSpace(parts[0])
				if name != "" && !seen[name] {
					seen[name] = true
					items = append(items, name)
				}
			}
		}
		sort.Strings(items[1:])
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
	})
	mux.HandleFunc("GET /api/v2/hosts/monitor/iooptions", func(w http.ResponseWriter, _ *http.Request) {
		items := []string{"all"}
		if data, err := os.ReadFile("/proc/diskstats"); err == nil {
			seen := map[string]bool{"all": true}
			for _, line := range strings.Split(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) < 3 {
					continue
				}
				name := fields[2]
				if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || seen[name] {
					continue
				}
				seen[name] = true
				items = append(items, name)
			}
			sort.Strings(items[1:])
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
	})
}

// registerHostMonitorSettings 注册监控设置的读取和更新接口，字段与 1Panel 设置页一致。
func registerHostMonitorSettings(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/hosts/monitor/setting", func(w http.ResponseWriter, _ *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": loadHostMonitorSettings()})
	})
	mux.HandleFunc("POST /api/v2/hosts/monitor/setting/update", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := decodeJSON(r, &request); err != nil {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		updated, err := updateHostMonitorSetting(request.Key, request.Value)
		if err != nil {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		restartHostMonitorLoop()
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": updated})
	})
}
