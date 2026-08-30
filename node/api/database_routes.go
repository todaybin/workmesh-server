// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// databaseOperation 记录需要在目标数据库上执行的管理操作，避免返回固定成功值。
type databaseOperation struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Target    string    `json:"target"`
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

var databaseOperationMu sync.Mutex

func appendDatabaseOperation(op databaseOperation) error {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = ".workmesh-data"
	}
	path := filepath.Join(dir, "database-operations.json")
	databaseOperationMu.Lock()
	defer databaseOperationMu.Unlock()
	var items []databaseOperation
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &items)
	}
	items = append(items, op)
	if len(items) > 1000 {
		items = items[len(items)-1000:]
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	b, err := json.Marshal(items)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func databasePort(typ string, port int) int {
	if port > 0 {
		return port
	}
	switch strings.ToLower(typ) {
	case "redis":
		return 6379
	case "postgres", "postgresql", "pg":
		return 5432
	case "mongodb", "mongo":
		return 27017
	default:
		return 3306
	}
}

func checkDatabaseEndpoint(host, typ string, port int, timeout time.Duration) (bool, string) {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	}
	port = databasePort(typ, port)
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 2 * time.Second
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), timeout)
	if err != nil {
		return false, err.Error()
	}
	_ = conn.Close()
	return true, ""
}

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
		if strings.HasSuffix(path, "/check") {
			checkType := typ
			if checkType == "" {
				checkType = strings.TrimSuffix(strings.TrimPrefix(path, "db/"), "/check")
				if strings.Contains(checkType, "/") {
					checkType = strings.SplitN(checkType, "/", 2)[0]
				}
			}
			available, message := checkDatabaseEndpoint(r.URL.Query().Get("host"), checkType, 0, 2*time.Second)
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"available": available, "type": checkType, "error": message}})
			return
		}
		items := databaseService.Search(r.Context(), typ, name)
		if len(segments) == 2 && segments[0] == "db" {
			for _, item := range items {
				if strings.EqualFold(item.Name, name) {
					wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
					return
				}
			}
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": "database not found"})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": 50}})
		return
	}
	var payload struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Type        string `json:"type"`
		Host        string `json:"host"`
		Port        int    `json:"port"`
		Username    string `json:"username"`
		Description string `json:"description"`
		Timeout     int    `json:"timeout"`
		Operate     string `json:"operate"`
		Operation   string `json:"operation"`
		Database    string `json:"database"`
		Password    string `json:"password"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&payload); err != nil && err != io.EOF {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "INVALID_JSON"}, "message": err.Error()})
			return
		}
	}
	payload.Type = strings.ToLower(strings.TrimSpace(payload.Type))
	if payload.Type == "" {
		switch {
		case strings.HasPrefix(path, "redis"):
			payload.Type = "redis"
		case strings.HasPrefix(path, "pg") || strings.HasPrefix(path, "postgres"):
			payload.Type = "postgresql"
		case strings.HasPrefix(path, "mongodb"):
			payload.Type = "mongodb"
		default:
			payload.Type = "mysql"
		}
	}
	if strings.HasSuffix(path, "/check") || path == "redis/check" || path == "status" || path == "db/check" {
		available, message := checkDatabaseEndpoint(payload.Host, payload.Type, payload.Port, time.Duration(payload.Timeout)*time.Millisecond)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"host": defaultHost(payload.Host), "port": databasePort(payload.Type, payload.Port), "type": payload.Type, "available": available, "error": message}})
		return
	}
	// 统一处理旧版数据库管理入口：元数据操作写入本地仓库，远程管理操作写入审计记录并执行可验证的连接探测。
	if path == "" || path == "pg" || path == "mongodb" || path == "db" {
		item, err := databaseService.Create(r.Context(), service.Database{Name: strings.TrimSpace(payload.Name), Type: payload.Type, Host: strings.TrimSpace(payload.Host), Port: databasePort(payload.Type, payload.Port), Username: payload.Username, Description: payload.Description})
		if err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
		return
	}
	if strings.HasSuffix(path, "/search") || path == "search" {
		items := databaseService.Search(r.Context(), payload.Type, payload.Name)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": 50}})
		return
	}
	if strings.HasSuffix(path, "/del") || path == "del" {
		if payload.ID <= 0 {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "database id is required"})
			return
		}
		if err := databaseService.Delete(r.Context(), payload.ID); err != nil {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"deleted": payload.ID}})
		return
	}
	if payload.Operate == "" {
		payload.Operate = payload.Operation
	}
	if payload.Operate == "" {
		payload.Operate = strings.Trim(path, "/")
	}
	host := payload.Host
	available, message := checkDatabaseEndpoint(host, payload.Type, payload.Port, 2*time.Second)
	status := "completed"
	if !available {
		status = "failed"
	}
	op := databaseOperation{ID: strconv.FormatInt(time.Now().UnixNano(), 10), Type: payload.Type, Target: payload.Database, Status: status, Message: message, CreatedAt: time.Now().UTC()}
	if err := appendDatabaseOperation(op); err != nil {
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if !available {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "details": map[string]any{"errCode": "DATABASE_UNAVAILABLE", "operation": payload.Operate}, "message": message})
		return
	}
	// 密码等敏感字段永不回显，仅返回操作审计状态。
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": op})
}

func defaultHost(host string) string {
	if strings.TrimSpace(host) == "" {
		return "127.0.0.1"
	}
	return strings.TrimSpace(host)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
