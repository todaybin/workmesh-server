// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

func registerDatabaseRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/databases/db", handleDatabaseCreate)
	mux.HandleFunc("POST /api/v2/databases/db/check", handleDatabaseCheck)
	mux.HandleFunc("POST /api/v2/databases/db/del", handleDatabaseDelete)
	mux.HandleFunc("POST /api/v2/databases/db/search", handleDatabaseSearch)
	mux.HandleFunc("POST /api/v2/databases/db/update", handleDatabaseUpdate)
	mux.HandleFunc("/api/v2/databases/", databaseRoute)
}

func isDatabaseRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return path == "/api/v2/databases" || strings.HasPrefix(path, "/api/v2/databases/")
}

func databaseRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/databases/"), "/")
	if r.Method == http.MethodGet {
		typ, name := "", r.URL.Query().Get("name")
		segments := strings.Split(path, "/")
		if len(segments) == 2 && segments[0] == "db" {
			name = segments[1]
		}
		if len(segments) == 3 && segments[0] == "db" && segments[1] == "list" {
			typ = segments[2]
		}
		items := databaseService.Search(r.Context(), typ, name)
		if strings.HasSuffix(path, "/check") {
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"available": true}})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": 50}})
		return
	}
	var payload struct {
		Host    string `json:"host"`
		Port    int    `json:"port"`
		Timeout int    `json:"timeout"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&payload); err != nil && err != io.EOF {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "INVALID_JSON"}, "message": err.Error()})
			return
		}
	}
	if strings.HasSuffix(path, "/check") || path == "redis/check" || path == "status" {
		if payload.Host == "" {
			payload.Host = "127.0.0.1"
		}
		if payload.Port == 0 {
			payload.Port = 3306
		}
		timeout := 2 * time.Second
		if payload.Timeout > 0 && payload.Timeout < 10_000 {
			timeout = time.Duration(payload.Timeout) * time.Millisecond
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(payload.Host, strconv.Itoa(payload.Port)), timeout)
		if err == nil {
			_ = conn.Close()
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"host": payload.Host, "port": payload.Port, "available": err == nil, "error": errorString(err)}})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"accepted": true, "operation": path}})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
