// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

func registerProcessRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/process/{pid}", handleProcessByID)
	mux.HandleFunc("GET /api/v2/process/ws", handleProcessWebSocket)
	mux.HandleFunc("POST /api/v2/process/stop", handleProcessStop)
	mux.HandleFunc("POST /api/v2/process/listening", handleProcessListening)
}

// writeWebSocketTextFrame 保留测试和旧内部调用的兼容名称。
func writeWebSocketTextFrame(conn net.Conn, payload []byte) error {
	return writeStreamFrame(conn, 0x1, payload)
}

// handleProcessWebSocket 按原 Agent 协议处理 ps/net 请求并返回数组结果。
func handleProcessWebSocket(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"websocket": true, "upgradeRequired": true}})
		return
	}
	if !requireStreamAuth(w, r, "WORKMESH_PROCESS_TOKEN", "WORKMESH_STREAM_TOKEN") {
		return
	}
	if !validStreamOrigin(r) {
		wmhttp.JSON(w, http.StatusForbidden, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "WEBSOCKET_ORIGIN_DENIED"}})
		return
	}
	ws, err := upgradeStreamWebSocket(w, r)
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "WEBSOCKET_ORIGIN_DENIED" {
			status = http.StatusForbidden
		}
		if err.Error() == "WEBSOCKET_UNAVAILABLE" {
			status = http.StatusServiceUnavailable
		}
		wmhttp.JSON(w, status, map[string]any{"code": "ERR", "details": map[string]string{"errCode": err.Error()}})
		return
	}
	defer ws.close()
	for {
		opcode, payload, readErr := ws.readFrame()
		if readErr != nil {
			return
		}
		switch opcode {
		case 0x8:
			return
		case 0x9:
			_ = ws.writeControl(0xA, payload)
		case 0x1:
			var request struct {
				Type        string `json:"type"`
				PID         int    `json:"pid"`
				Name        string `json:"name"`
				Username    string `json:"username"`
				ProcessID   int    `json:"processID"`
				ProcessName string `json:"processName"`
				Port        int    `json:"port"`
			}
			if json.Unmarshal(payload, &request) != nil {
				continue
			}
			var data any
			switch request.Type {
			case "ps":
				data = dashboardProcessList(request.PID, request.Name, request.Username)
			case "net":
				data = dashboardNetList(request.ProcessID, request.ProcessName, request.Port)
			default:
				continue
			}
			encoded, _ := json.Marshal(data)
			if err := ws.writeText(encoded); err != nil {
				return
			}
		}
	}
}

func handleProcessByID(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.Atoi(r.PathValue("pid"))
	if err != nil || pid <= 0 {
		processError(w, 400, errors.New("进程 ID 无效"))
		return
	}
	data := map[string]any{"pid": pid, "name": "", "cmd": "", "user": "", "memory": int64(0), "percent": 0.0}
	if raw, e := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline"); e == nil {
		data["cmd"] = strings.ReplaceAll(string(raw), "\x00", " ")
		fields := strings.Fields(data["cmd"].(string))
		if len(fields) > 0 {
			data["name"] = fields[0]
		}
		readProcessDetails(pid, data)
	}
	if data["cmd"] == "" {
		processError(w, 404, errors.New("进程不存在或不可访问"))
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": data})
}

func dashboardProcessList(pid int, name, username string) []map[string]any {
	items := dashboardProcesses()
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if pid > 0 && processIntValue(item["pid"]) != pid {
			continue
		}
		if name != "" && !strings.Contains(strings.ToLower(fmt.Sprint(item["name"])), strings.ToLower(name)) {
			continue
		}
		if username != "" && !strings.Contains(strings.ToLower(fmt.Sprint(item["user"])), strings.ToLower(username)) {
			continue
		}
		result = append(result, item)
	}
	return result
}

func dashboardNetList(processID int, processName string, port int) []map[string]any {
	items := make([]map[string]any, 0)
	for _, proto := range []string{"tcp", "udp"} {
		data, err := os.ReadFile("/proc/net/" + proto)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n")[1:] {
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			local := procSocketAddr(fields[1])
			remote := procSocketAddr(fields[2])
			if port > 0 && local["port"] != port && remote["port"] != port {
				continue
			}
			status := "NONE"
			if len(fields) > 3 {
				switch fields[3] {
				case "01":
					status = "ESTABLISHED"
				case "0A":
					status = "LISTEN"
				case "06":
					status = "TIME_WAIT"
				case "08":
					status = "CLOSE_WAIT"
				}
			}
			item := map[string]any{"type": proto, "status": status, "localaddr": local, "remoteaddr": remote, "PID": 0, "name": ""}
			items = append(items, item)
			if len(items) >= 2048 {
				return items
			}
		}
	}
	_ = processID
	_ = processName
	return items
}

func procSocketAddr(value string) map[string]any {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return map[string]any{"ip": "", "port": 0}
	}
	port, _ := strconv.ParseInt(parts[1], 16, 32)
	ip := parts[0]
	if len(ip) == 8 {
		if raw, err := hex.DecodeString(ip); err == nil {
			ip = net.IPv4(raw[3], raw[2], raw[1], raw[0]).String()
		}
	}
	return map[string]any{"ip": ip, "port": int(port)}
}

func processIntValue(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case uint64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// readProcessDetails 从 procfs 读取进程内存和用户，失败时保留可解释的默认值。
func readProcessDetails(pid int, data map[string]any) {
	status, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(status), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "VmRSS":
			if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				data["memory"] = kb * 1024
			}
		case "Uid":
			if uid, err := user.LookupId(fields[1]); err == nil {
				data["user"] = uid.Username
			}
		}
	}
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
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ss", "-lntup")
	output, err := cmd.Output()
	if err != nil {
		cmd = exec.CommandContext(ctx, "netstat", "-ano")
		output, err = cmd.Output()
	}
	if err != nil {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "LISTENING_COMMAND_UNAVAILABLE"}, "message": "ss 和 netstat 均不可用"})
		return
	}
	items := parseListeningOutput(string(output))
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
}

// parseListeningOutput 将 ss/netstat 输出转换为有界 DTO，避免把系统命令原始文本直接暴露给前端。
func parseListeningOutput(output string) []map[string]any {
	lines := strings.Split(output, "\n")
	items := make([]map[string]any, 0, minInt(len(lines), 1024))
	for _, line := range lines[1:] {
		if len(items) >= 1024 {
			break
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		addresses := make([]string, 0, 2)
		addressIndexes := make([]int, 0, 2)
		for index := 1; index < len(fields) && len(addresses) < 2; index++ {
			// ss/netstat 的队列长度是纯数字，地址字段通常包含冒号；跳过状态和队列列。
			if !strings.Contains(fields[index], ":") || isNumericToken(fields[index]) {
				continue
			}
			addresses = append(addresses, fields[index])
			addressIndexes = append(addressIndexes, index)
		}
		if len(addresses) < 2 {
			continue
		}
		item := map[string]any{"protocol": fields[0], "localAddress": addresses[0], "remoteAddress": addresses[1]}
		if len(fields) > addressIndexes[1]+1 {
			item["process"] = strings.TrimSpace(strings.Join(fields[addressIndexes[1]+1:], " "))
		}
		items = append(items, item)
	}
	return items
}

func isNumericToken(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func processError(w http.ResponseWriter, status int, err error) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
}
