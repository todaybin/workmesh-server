// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
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

// handleProcessWebSocket 按 RFC6455 提供轻量进程快照推送，不引入常驻 WebSocket 库。
// 仅发送服务端文本帧，客户端关闭连接或请求上下文结束后立即释放连接。
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
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "WEBSOCKET_KEY_REQUIRED"}})
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		// 当前传输层不支持 Hijack 时返回依赖不可用，而不是把接口标记为未实现。
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "WEBSOCKET_UNAVAILABLE"}})
		return
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + "\r\n\r\n")
	if err := rw.Flush(); err != nil {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		payload, _ := json.Marshal(map[string]any{"type": "process", "data": dashboardProcesses(), "updatedAt": time.Now().UTC()})
		if err := writeWebSocketTextFrame(conn, payload); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func writeWebSocketTextFrame(conn net.Conn, payload []byte) error {
	// 服务端发送帧不需要掩码；按 RFC6455 编码 7 位、16 位和 64 位长度。
	var header []byte
	switch {
	case len(payload) < 126:
		header = []byte{0x81, byte(len(payload))}
	case len(payload) <= 65535:
		header = []byte{0x81, 126, byte(len(payload) >> 8), byte(len(payload))}
	default:
		if uint64(len(payload)) > ^uint64(0)>>1 {
			return errors.New("进程流帧过大")
		}
		header = []byte{0x81, 127, 0, 0, 0, 0, byte(len(payload) >> 24), byte(len(payload) >> 16), byte(len(payload) >> 8), byte(len(payload))}
	}
	frame := append(header, payload...)
	_, err := conn.Write(frame)
	return err
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
