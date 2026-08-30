// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

const maxContainerLogTail = 10000

func handleContainerLogStream(w http.ResponseWriter, r *http.Request) {
	if !requireStreamAuth(w, r, "WORKMESH_CONTAINER_TOKEN", "WORKMESH_STREAM_TOKEN") {
		return
	}
	args, follow, err := containerLogArgs(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "CONTAINER_LOG_PARAMETERS_INVALID"}, "message": err.Error()})
		return
	}
	if !strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
		result, runErr := (service.CommandService{}).Execute(r.Context(), model.CommandRequest{Program: "docker", Args: args, Timeout: 30 * time.Second})
		writeCommandResult(w, result, runErr)
		return
	}

	limit := 30 * time.Second
	if follow {
		limit = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(r.Context(), limit)
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SSE_UNAVAILABLE"}})
		return
	}
	stream := &containerSSEWriter{writer: w, flusher: flusher}
	command := exec.CommandContext(ctx, "docker", args...)
	command.Stdout = stream
	command.Stderr = stream
	if err := command.Start(); err != nil {
		stream.event("error", map[string]any{"message": fmt.Sprintf("启动 Docker 日志流失败: %v", err)})
		return
	}
	stream.event("ready", map[string]any{"follow": follow})
	err = command.Wait()
	if ctx.Err() != nil {
		stream.event("close", map[string]any{"reason": ctx.Err().Error()})
		return
	}
	if err != nil {
		stream.event("error", map[string]any{"message": err.Error()})
		return
	}
	stream.event("close", map[string]any{"exitCode": 0})
}

func containerLogArgs(r *http.Request) ([]string, bool, error) {
	query := r.URL.Query()
	container := strings.TrimSpace(query.Get("container"))
	compose := strings.TrimSpace(query.Get("compose"))
	if (container == "") == (compose == "") {
		return nil, false, errors.New("container 和 compose 必须且只能提供一个")
	}
	follow := strings.EqualFold(query.Get("follow"), "true")
	tail := 100
	if raw := strings.TrimSpace(query.Get("tail")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > maxContainerLogTail {
			return nil, false, fmt.Errorf("tail 必须在 0 到 %d 之间", maxContainerLogTail)
		}
		tail = value
	}
	since := strings.TrimSpace(query.Get("since"))
	if len(since) > 64 || strings.ContainsAny(since, "\r\n\x00") {
		return nil, false, errors.New("since 参数无效")
	}
	args := make([]string, 0, 12)
	if compose != "" {
		if !validDockerPath(compose) {
			return nil, false, errors.New("compose 路径无效")
		}
		args = append(args, "compose", "-f", compose, "logs")
	} else {
		if !validDockerIdentifier(container) {
			return nil, false, errors.New("container 参数无效")
		}
		args = append(args, "logs")
	}
	if follow {
		args = append(args, "--follow")
	}
	args = append(args, "--tail", strconv.Itoa(tail))
	if since != "" {
		args = append(args, "--since", since)
	}
	if strings.EqualFold(query.Get("timestamp"), "true") {
		args = append(args, "--timestamps")
	}
	if container != "" {
		args = append(args, container)
	}
	return args, follow, nil
}

type containerSSEWriter struct {
	mu      sync.Mutex
	writer  io.Writer
	flusher http.Flusher
}

func (s *containerSSEWriter) Write(payload []byte) (int, error) {
	originalLen := len(payload)
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(payload) > 0 {
		size := len(payload)
		if size > 64<<10 {
			size = 64 << 10
		}
		chunk := strings.ReplaceAll(strings.ReplaceAll(string(payload[:size]), "\r", ""), "\n", "\ndata: ")
		if _, err := fmt.Fprintf(s.writer, "event: log\ndata: %s\n\n", chunk); err != nil {
			return 0, err
		}
		s.flusher.Flush()
		payload = payload[size:]
	}
	return originalLen, nil
}

func (s *containerSSEWriter) event(name string, value any) {
	payload, _ := json.Marshal(value)
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = fmt.Fprintf(s.writer, "event: %s\ndata: %s\n\n", name, payload)
	s.flusher.Flush()
}
