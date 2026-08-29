// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

type executionState struct {
	mu       sync.RWMutex
	accounts []map[string]any
	agents   []map[string]any
	mcp      []map[string]any
	tasks    map[string]map[string]any
}

var aiState = executionState{tasks: make(map[string]map[string]any)}

func registerAIExecutionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/ai/", aiHandler)
	mux.HandleFunc("/api/v2/cubesandbox/", sandboxHandler)
	mux.HandleFunc("/api/v2/workmesh/tasks/", taskHandler)
}

func isAIExecutionRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return strings.HasPrefix(path, "/api/v2/ai/") || strings.HasPrefix(path, "/api/v2/cubesandbox/") || strings.HasPrefix(path, "/api/v2/workmesh/tasks/")
}

func aiHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/ai/"), "/")
	if r.Method == http.MethodGet {
		switch path {
		case "accounts/providers":
			providers := []string{}
			for _, item := range strings.Split(os.Getenv("WORKMESH_AI_PROVIDERS"), ",") {
				if item = strings.TrimSpace(item); item != "" {
					providers = append(providers, item)
				}
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"providers": providers}})
		case "gpu/load":
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"available": false, "reason": "未检测到 GPU 驱动", "heapAlloc": mem.HeapAlloc, "cpus": runtime.NumCPU()}})
		case "gpu/options":
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"devices": []any{}, "backends": []string{"cpu"}}})
		case "mcp/domain/get", "domain/get":
			aiState.mu.RLock()
			items := append([]map[string]any(nil), aiState.mcp...)
			aiState.mu.RUnlock()
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
		default:
			writeAIList(w, path)
		}
		return
	}
	var body map[string]any
	if r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "INVALID_JSON"}, "message": err.Error()})
			return
		}
	}
	if body == nil {
		body = map[string]any{}
	}
	collection := &aiState.accounts
	if strings.HasPrefix(path, "agents") {
		collection = &aiState.agents
	} else if strings.HasPrefix(path, "mcp/") || strings.HasPrefix(path, "domain/") {
		collection = &aiState.mcp
	}
	if strings.HasSuffix(path, "/delete") || strings.HasSuffix(path, "/del") {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"deleted": true}})
		return
	}
	if strings.HasSuffix(path, "/search") || strings.HasSuffix(path, "/list") || strings.HasSuffix(path, "/counts") || strings.HasSuffix(path, "/overview") {
		writeAIList(w, path)
		return
	}
	if _, ok := body["id"]; !ok {
		body["id"] = "ai-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	body["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	aiState.mu.Lock()
	*collection = append(*collection, body)
	aiState.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": body})
}

func writeAIList(w http.ResponseWriter, path string) {
	aiState.mu.RLock()
	items := []map[string]any{}
	if strings.HasPrefix(path, "agents") {
		items = append(items, aiState.agents...)
	} else if strings.HasPrefix(path, "mcp/") || strings.HasPrefix(path, "domain/") {
		items = append(items, aiState.mcp...)
	} else {
		items = append(items, aiState.accounts...)
	}
	aiState.mu.RUnlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": 50}})
}

func sandboxHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/cubesandbox/"), "/")
	available := runtime.GOOS == "linux"
	if available {
		if _, err := os.Stat("/dev/kvm"); err != nil {
			available = false
		}
	}
	if r.Method == http.MethodGet {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"status": map[bool]string{true: "ready", false: "degraded"}[available], "available": available, "path": path}})
		return
	}
	if !available {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "CUBESANDBOX_UNAVAILABLE"}, "message": "当前主机缺少 KVM，无法启动 MicroVM"})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"accepted": true, "operation": path}})
}

func taskHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/workmesh/tasks/"), "/")
	if r.Method != http.MethodPost {
		wmhttp.JSON(w, http.StatusMethodNotAllowed, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "METHOD_NOT_ALLOWED"}})
		return
	}
	var body struct {
		ID      string   `json:"id"`
		Program string   `json:"program"`
		Args    []string `json:"args"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
	}
	if body.ID == "" {
		body.ID = "task-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	aiState.mu.Lock()
	if path == "create" || path == "start" {
		aiState.tasks[body.ID] = map[string]any{"id": body.ID, "status": "created", "createdAt": time.Now().UTC()}
	}
	task := aiState.tasks[body.ID]
	if task == nil {
		task = map[string]any{"id": body.ID, "status": "unknown"}
	}
	if path == "cancel" || path == "destroy" {
		task["status"] = path + "ed"
	}
	aiState.mu.Unlock()
	if path == "exec" {
		token := os.Getenv("WORKMESH_TASK_TOKEN")
		if token == "" || r.Header.Get("X-WorkMesh-Token") != token {
			wmhttp.JSON(w, http.StatusUnauthorized, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "TASK_AUTH_REQUIRED"}})
			return
		}
		result, err := (service.CommandService{}).Execute(r.Context(), model.CommandRequest{Program: body.Program, Args: body.Args})
		writeCommandResult(w, result, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": task})
}
