// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

func registerProcessRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/process/{pid}", handleProcessByID)
	mux.HandleFunc("POST /api/v2/process/stop", handleProcessStop)
	mux.HandleFunc("POST /api/v2/process/listening", handleProcessListening)
}

func handleProcessByID(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.Atoi(r.PathValue("pid"))
	if err != nil || pid <= 0 {
		processError(w, 400, errors.New("进程 ID 无效"))
		return
	}
	data := map[string]any{"pid": pid, "name": "", "cmd": "", "user": "", "memory": 0, "percent": 0}
	if raw, e := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline"); e == nil {
		data["cmd"] = strings.ReplaceAll(string(raw), "\x00", " ")
		data["name"] = strings.Fields(data["cmd"].(string))[0]
	}
	if data["cmd"] == "" {
		processError(w, 404, errors.New("进程不存在或不可访问"))
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": data})
}

func handleProcessStop(w http.ResponseWriter, r *http.Request) {
	if token := os.Getenv("WORKMESH_PROCESS_TOKEN"); token == "" || r.Header.Get("X-WorkMesh-Token") != token {
		processError(w, http.StatusUnauthorized, errors.New("PROCESS_AUTH_REQUIRED"))
		return
	}
	var req struct {
		PID int `json:"pid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PID <= 1 || req.PID == os.Getpid() {
		processError(w, 400, errors.New("进程 ID 无效"))
		return
	}
	p, err := os.FindProcess(req.PID)
	if err != nil {
		processError(w, 404, err)
		return
	}
	if err = p.Kill(); err != nil {
		processError(w, 500, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200})
}

func handleProcessListening(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cmd := exec.CommandContext(ctx, "ss", "-lntup")
	output, err := cmd.Output()
	if err != nil {
		cmd = exec.CommandContext(ctx, "netstat", "-ano")
		output, err = cmd.Output()
	}
	if err != nil {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": []any{}})
		return
	}
	lines := strings.Split(string(output), "\n")
	items := make([]map[string]any, 0)
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			items = append(items, map[string]any{"protocol": fields[0], "localAddress": fields[3], "remoteAddress": fields[4]})
		}
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
}
func processError(w http.ResponseWriter, status int, err error) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
}
