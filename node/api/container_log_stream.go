// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

const maxContainerLogTail = 10000

const (
	maxContainerSSEPending   = 128 << 10
	maxContainerSSEReplay    = 256
	maxContainerSSEStreams   = 128
	containerSSEWriteTimeout = 10 * time.Second
)

// containerLogCommand 允许测试注入受控的 Docker 进程构造器；生产环境始终执行 docker 二进制。
var containerLogCommand = func(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, service.DockerBinary(), args...)
}

func handleContainerLogStream(w http.ResponseWriter, r *http.Request) {
	if !requireStreamAuth(w, r, "WORKMESH_CONTAINER_TOKEN", "WORKMESH_STREAM_TOKEN") {
		return
	}
	if !strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": nil})
		return
	}
	releaseStream, ok := tryRuntimeSlotFor("streams", nodeRuntimeLimits.streams)
	if !ok {
		wmhttp.JSON(w, http.StatusTooManyRequests, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "SSE_LIMIT_REACHED"}, "message": "日志流并发数已达到上限"})
		return
	}
	defer releaseStream()
	args, follow, err := containerLogArgs(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "CONTAINER_LOG_PARAMETERS_INVALID"}, "message": err.Error()})
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
	resumeID := parseLastEventID(r.Header.Get("Last-Event-ID"))
	stream := newContainerSSEWriter(w, flusher, cancel, strings.Join(args, "\x00"), resumeID)
	stream.setDeadline = http.NewResponseController(w).SetWriteDeadline
	command := containerLogCommand(ctx, args...)
	command.Stdout = stream
	command.Stderr = stream
	if err := command.Start(); err != nil {
		stream.event("error", map[string]any{"message": fmt.Sprintf("启动 Docker 日志流失败: %v", err)})
		return
	}
	// 先重放有界历史，再发送 ready，客户端可用 Last-Event-ID 补齐断线期间的数据。
	if err := stream.replay(resumeID); err != nil {
		return
	}
	if err := stream.event("ready", map[string]any{"follow": follow, "resumeFrom": resumeID}); err != nil {
		return
	}
	// 长时间没有日志时仍发送心跳，确保反向代理不会回收 SSE；请求取消会同时终止该协程。
	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := stream.event("heartbeat", map[string]any{"timestamp": time.Now().UTC()}); err != nil {
					return
				}
			case <-heartbeatDone:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	err = command.Wait()
	// Docker 可能在最后一行不带换行，关闭前补发暂存内容，避免日志尾部丢失。
	if flushErr := stream.flushPending(); flushErr != nil {
		err = flushErr
	}
	if ctx.Err() != nil {
		_ = stream.event("close", map[string]any{"reason": ctx.Err().Error()})
		return
	}
	if err != nil {
		_ = stream.event("error", map[string]any{"message": err.Error()})
		return
	}
	_ = stream.event("close", map[string]any{"exitCode": 0})
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
		paths := strings.Split(compose, ",")
		if len(paths) > 20 {
			return nil, false, errors.New("compose 文件数量不能超过 20 个")
		}
		args = append(args, "compose")
		for _, rawPath := range paths {
			path := strings.TrimSpace(rawPath)
			if !validComposeLogPath(path) {
				return nil, false, errors.New("compose 路径无效")
			}
			args = append(args, "-f", path)
		}
		args = append(args, "logs")
	} else {
		if !validDockerIdentifier(container) {
			return nil, false, errors.New("container 参数无效")
		}
		args = append(args, "logs")
	}
	if follow {
		args = append(args, "--follow")
	}
	if tail > 0 {
		args = append(args, "--tail", strconv.Itoa(tail))
	}
	if since != "" && !strings.EqualFold(since, "all") {
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

// validComposeLogPath 限制日志跟随使用的 Compose 文件，避免路径穿越和空文件段。
func validComposeLogPath(value string) bool {
	value = strings.TrimSpace(value)
	if !validDockerPath(value) || value == "." || value == ".." {
		return false
	}
	clean := filepath.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return false
	}
	for _, part := range strings.FieldsFunc(filepath.ToSlash(value), func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return false
		}
	}
	return true
}

type containerSSEWriter struct {
	mu          sync.Mutex
	writer      io.Writer
	flusher     http.Flusher
	onError     func()
	setDeadline func(time.Time) error
	pending     []byte
	replayKey   string
	// nextID 按连接单调递增；断线重连通过 Last-Event-ID 继续编号，便于客户端去重。
	nextID int64
}

type containerSSEEvent struct {
	id   int64
	name string
	data []byte
}

type containerSSEReplayBuffer struct {
	events []containerSSEEvent
}

var containerSSEReplay = struct {
	sync.Mutex
	items map[string]*containerSSEReplayBuffer
}{items: make(map[string]*containerSSEReplayBuffer)}

func newContainerSSEWriter(w io.Writer, flusher http.Flusher, onError func(), key string, resumeID int64) *containerSSEWriter {
	if resumeID < 0 {
		resumeID = 0
	}
	s := &containerSSEWriter{writer: w, flusher: flusher, onError: onError, replayKey: key, nextID: resumeID}
	if key == "" {
		return s
	}
	containerSSEReplay.Lock()
	if history := containerSSEReplay.items[key]; history != nil && len(history.events) > 0 {
		latest := history.events[len(history.events)-1].id
		if latest > s.nextID {
			s.nextID = latest
		}
	}
	containerSSEReplay.Unlock()
	return s
}

func (s *containerSSEWriter) rememberLocked(event containerSSEEvent) {
	if s.replayKey == "" {
		return
	}
	data := append([]byte(nil), event.data...)
	containerSSEReplay.Lock()
	history := containerSSEReplay.items[s.replayKey]
	if history == nil {
		if len(containerSSEReplay.items) >= maxContainerSSEStreams {
			for key := range containerSSEReplay.items {
				delete(containerSSEReplay.items, key)
				break
			}
		}
		history = &containerSSEReplayBuffer{}
		containerSSEReplay.items[s.replayKey] = history
	}
	history.events = append(history.events, containerSSEEvent{id: event.id, name: event.name, data: data})
	if len(history.events) > maxContainerSSEReplay {
		history.events = history.events[len(history.events)-maxContainerSSEReplay:]
	}
	containerSSEReplay.Unlock()
}

func (s *containerSSEWriter) writeLocked(format string, args ...any) error {
	if s.setDeadline != nil {
		if err := s.setDeadline(time.Now().Add(containerSSEWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			if s.onError != nil {
				s.onError()
			}
			return fmt.Errorf("SSE 写入超时设置失败: %w", err)
		}
	}
	_, err := fmt.Fprintf(s.writer, format, args...)
	if s.setDeadline != nil {
		_ = s.setDeadline(time.Time{})
	}
	if err != nil && s.onError != nil {
		s.onError()
	}
	return err
}

func parseLastEventID(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 0 {
		return 0
	}
	return id
}

func (s *containerSSEWriter) Write(payload []byte) (int, error) {
	originalLen := len(payload)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(payload) > maxContainerSSEPending || len(s.pending)+len(payload) > maxContainerSSEPending {
		if s.onError != nil {
			s.onError()
		}
		return 0, errors.New("SSE 输出积压超过 128KiB 限制")
	}
	s.pending = append(s.pending, payload...)
	for {
		index := bytes.IndexByte(s.pending, '\n')
		if index < 0 {
			if len(s.pending) > 64<<10 {
				if err := s.writeDataLineLocked(s.pending[:64<<10]); err != nil {
					return 0, err
				}
				s.pending = s.pending[64<<10:]
				continue
			}
			break
		}
		line := s.pending[:index]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if err := s.writeDataLineLocked(line); err != nil {
			return 0, err
		}
		s.pending = s.pending[index+1:]
	}
	return originalLen, nil
}

func (s *containerSSEWriter) writeDataLineLocked(line []byte) error {
	s.nextID++
	clean := []byte(strings.ReplaceAll(string(line), "\r", ""))
	if err := s.writeLocked("id: %d\ndata: %s\n\n", s.nextID, clean); err != nil {
		return err
	}
	s.rememberLocked(containerSSEEvent{id: s.nextID, data: clean})
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}

func (s *containerSSEWriter) flushPending() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) == 0 {
		return nil
	}
	line := s.pending
	s.pending = nil
	return s.writeDataLineLocked(line)
}

func (s *containerSSEWriter) event(name string, value any) error {
	payload, _ := json.Marshal(value)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) > 0 {
		line := s.pending
		s.pending = nil
		if err := s.writeDataLineLocked(line); err != nil {
			return err
		}
	}
	s.nextID++
	if err := s.writeLocked("id: %d\nevent: %s\ndata: %s\n\n", s.nextID, name, payload); err != nil {
		return err
	}
	s.rememberLocked(containerSSEEvent{id: s.nextID, name: name, data: payload})
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}

// replay 将 Last-Event-ID 之后的有界事件重放给重连客户端。
func (s *containerSSEWriter) replay(lastID int64) error {
	if s.replayKey == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	containerSSEReplay.Lock()
	history := containerSSEReplay.items[s.replayKey]
	events := make([]containerSSEEvent, 0)
	if history != nil {
		for _, event := range history.events {
			if event.id > lastID {
				events = append(events, containerSSEEvent{id: event.id, name: event.name, data: append([]byte(nil), event.data...)})
			}
		}
	}
	containerSSEReplay.Unlock()
	for _, event := range events {
		if event.name == "" {
			if err := s.writeLocked("id: %d\ndata: %s\n\n", event.id, event.data); err != nil {
				return err
			}
		} else if err := s.writeLocked("id: %d\nevent: %s\ndata: %s\n\n", event.id, event.name, event.data); err != nil {
			return err
		}
		if event.id > s.nextID {
			s.nextID = event.id
		}
		if s.flusher != nil {
			s.flusher.Flush()
		}
	}
	return nil
}
