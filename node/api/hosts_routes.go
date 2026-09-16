// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"io"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// hostRequestTest 处理主机连通性测试路由，返回是否已经写出响应。
func hostRequestTest(w http.ResponseWriter, r *http.Request, path string) bool {
	if r.Method != http.MethodPost || (path != "test/byinfo" && path != "test/byid") {
		return false
	}
	host, err := hostForConnectionTest(r, path)
	if err != nil {
		writeHostError(w, http.StatusBadRequest, "INVALID_HOST_TEST", err)
		return true
	}
	connected, latency, probeErr := probeHost(host.Address, host.Port)
	result := map[string]any{"connected": connected, "latency": latency, "address": host.Address, "addr": host.Address, "port": host.Port}
	if probeErr != nil {
		result["error"] = probeErr.Error()
	}
	writeHostJSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
	return true
}

// hostRequestCollection 处理主机搜索和树形分组路由，返回是否已经写出响应。
func hostRequestCollection(w http.ResponseWriter, r *http.Request, path string) bool {
	if r.Method != http.MethodPost || (path != "search" && path != "tree") {
		return false
	}
	if path == "tree" {
		writeHostTree(w)
		return true
	}
	query, err := decodeHostSearchQuery(r)
	if err != nil {
		writeHostError(w, http.StatusBadRequest, "", err)
		return true
	}
	writeHostSearch(w, query)
	return true
}

// hostRequestRead 处理主机查询、诊断和树形视图等只读路由。
func hostRequestRead(w http.ResponseWriter, r *http.Request, path string) {
	hostname, _ := os.Hostname()
	base := map[string]any{"hostname": hostname, "os": runtime.GOOS, "arch": runtime.GOARCH, "cpus": runtime.NumCPU(), "goroutines": runtime.NumGoroutine(), "timestamp": time.Now().UTC()}
	switch path {
	case "":
		items, err := listHosts("", 0, 0, 0)
		if err != nil {
			writeHostError(w, http.StatusInternalServerError, "", err)
			return
		}
		writeHostJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": len(items)}})
	case "system/info":
		writeHostJSON(w, http.StatusOK, map[string]any{"code": 200, "data": base})
	case "info":
		writeHostInfo(w, r.URL.Query().Get("id"))
	case "diagnostics/goroutines":
		writeHostJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"count": runtime.NumGoroutine(), "profiles": len(pprof.Profiles())}})
	case "diagnostics/summary":
		writeHostDiagnostics(w, base)
	case "tree":
		writeHostTree(w)
	case "search":
		query, err := decodeHostSearchQuery(r)
		if err != nil {
			writeHostError(w, http.StatusBadRequest, "", err)
			return
		}
		writeHostSearch(w, query)
	default:
		writeHostError(w, http.StatusNotFound, "HOST_ROUTE_NOT_FOUND", errors.New("host route not found"))
	}
}

// hostSearchQuery 描述主机列表搜索的分页和分组参数。
type hostSearchQuery struct {
	Info     string `json:"info"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	GroupID  uint   `json:"groupID"`
}

// decodeHostSearchQuery 解码并规范化主机搜索请求参数。
func decodeHostSearchQuery(r *http.Request) (hostSearchQuery, error) {
	var query hostSearchQuery
	if err := decodeJSON(r, &query); err != nil && err != io.EOF {
		return query, err
	}
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 200 {
		query.PageSize = 20
	}
	return query, nil
}

// writeHostSearch 写出主机搜索结果及总数，保持前端分页响应字段。
func writeHostSearch(w http.ResponseWriter, query hostSearchQuery) {
	items, total, err := searchHosts(query.Info, query.GroupID, query.Page, query.PageSize)
	if err != nil {
		writeHostError(w, http.StatusInternalServerError, "", err)
		return
	}
	writeHostJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": total, "page": query.Page, "pageSize": query.PageSize}})
}

// writeHostTree 将主机按分组组织为树形结构。
func writeHostTree(w http.ResponseWriter) {
	items, err := listHosts("", 0, 0, 0)
	if err != nil {
		writeHostError(w, http.StatusInternalServerError, "", err)
		return
	}
	groups := map[uint]map[string]any{}
	for _, item := range items {
		group := groups[item.GroupID]
		if group == nil {
			group = map[string]any{"id": item.GroupID, "label": item.GroupBelong, "children": []hostRecord{}}
			groups[item.GroupID] = group
		}
		group["children"] = append(group["children"].([]hostRecord), item)
	}
	out := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		out = append(out, group)
	}
	writeHostJSON(w, http.StatusOK, map[string]any{"code": 200, "data": out})
}

// writeHostInfo 查询单台主机的公开详情，不加载认证凭据。
func writeHostInfo(w http.ResponseWriter, id string) {
	item, ok, err := findHost(id)
	if err != nil {
		writeHostError(w, http.StatusInternalServerError, "", err)
		return
	}
	if !ok {
		writeHostError(w, http.StatusNotFound, "", errors.New("host not found"))
		return
	}
	writeHostJSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
}

// writeHostDiagnostics 写出当前进程的内存和运行时诊断信息。
func writeHostDiagnostics(w http.ResponseWriter, base map[string]any) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	base["heapAlloc"] = mem.HeapAlloc
	base["heapInuse"] = mem.HeapInuse
	base["numGC"] = mem.NumGC
	writeHostJSON(w, http.StatusOK, map[string]any{"code": 200, "data": base})
}

// writeHostJSON 输出统一主机接口响应。
func writeHostJSON(w http.ResponseWriter, status int, payload map[string]any) {
	wmhttp.JSON(w, status, payload)
}

// writeHostError 输出统一主机接口错误响应。
func writeHostError(w http.ResponseWriter, status int, code string, err error) {
	response := map[string]any{"code": "ERR", "message": err.Error()}
	if strings.TrimSpace(code) != "" {
		response["details"] = map[string]string{"errCode": code}
	}
	writeHostJSON(w, status, response)
}
