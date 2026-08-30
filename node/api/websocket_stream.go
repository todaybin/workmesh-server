// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

const maxWebSocketMessage = 1 << 20

const streamWriteTimeout = 30 * time.Second

// streamWebSocket 是流式接口使用的最小 RFC6455 实现，避免为单进程服务引入重量级依赖。
type streamWebSocket struct {
	conn        net.Conn
	read        *bufio.Reader
	writeMu     sync.Mutex
	idleTimeout time.Duration
}

func upgradeStreamWebSocket(w http.ResponseWriter, r *http.Request) (*streamWebSocket, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, errors.New("WEBSOCKET_UPGRADE_REQUIRED")
	}
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		return nil, errors.New("WEBSOCKET_KEY_REQUIRED")
	}
	if !validStreamOrigin(r) {
		return nil, errors.New("WEBSOCKET_ORIGIN_DENIED")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("WEBSOCKET_UNAVAILABLE")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, fmt.Errorf("websocket 握手失败: %w", err)
	}
	acceptSum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	accept := base64.StdEncoding.EncodeToString(acceptSum[:])
	if _, err := rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + "\r\n\r\n"); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &streamWebSocket{conn: conn, read: bufio.NewReader(conn), idleTimeout: terminalIdleTimeout}, nil
}

// validStreamOrigin 拒绝浏览器跨站 WebSocket；非浏览器请求允许不携带 Origin。
func validStreamOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}
	for _, allowed := range strings.Split(os.Getenv("WORKMESH_STREAM_ORIGINS"), ",") {
		if strings.EqualFold(strings.TrimSpace(allowed), origin) {
			return true
		}
	}
	return false
}

func (s *streamWebSocket) close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = writeStreamFrame(s.conn, 0x8, nil)
	return s.conn.Close()
}

func (s *streamWebSocket) writeText(payload []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return writeStreamFrame(s.conn, 0x1, payload)
}

func writeStreamFrame(conn net.Conn, opcode byte, payload []byte) error {
	if len(payload) > maxWebSocketMessage {
		return errors.New("websocket 消息超过 1MiB 限制")
	}
	_ = conn.SetWriteDeadline(time.Now().Add(streamWriteTimeout))
	header := []byte{0x80 | (opcode & 0x0f)}
	switch {
	case len(payload) < 126:
		header = append(header, byte(len(payload)))
	case len(payload) <= 65535:
		header = append(header, 126, byte(len(payload)>>8), byte(len(payload)))
	default:
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(payload)))
		header = append(header, 127)
		header = append(header, size[:]...)
	}
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

// readFrame 读取并解掩码客户端帧，仅接受文本、关闭、Ping 和 Pong 帧。
func (s *streamWebSocket) readFrame() (byte, []byte, error) {
	if s.idleTimeout > 0 {
		_ = s.conn.SetReadDeadline(time.Now().Add(s.idleTimeout))
	}
	first, err := s.read.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	second, err := s.read.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	if first&0x70 != 0 || first&0x80 == 0 {
		return 0, nil, errors.New("websocket 分片或保留位不受支持")
	}
	masked := second&0x80 != 0
	length := uint64(second & 0x7f)
	if length == 126 {
		var size [2]byte
		if _, err := io.ReadFull(s.read, size[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(size[:]))
	} else if length == 127 {
		var size [8]byte
		if _, err := io.ReadFull(s.read, size[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(size[:])
	}
	if length > maxWebSocketMessage {
		return 0, nil, errors.New("websocket 消息超过 1MiB 限制")
	}
	if !masked {
		return 0, nil, errors.New("客户端帧必须掩码")
	}
	var mask [4]byte
	if _, err := io.ReadFull(s.read, mask[:]); err != nil {
		return 0, nil, err
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(s.read, payload); err != nil {
		return 0, nil, err
	}
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
	return first & 0x0f, payload, nil
}

func requireStreamAuth(w http.ResponseWriter, r *http.Request, env ...string) bool {
	for _, name := range env {
		if token := strings.TrimSpace(os.Getenv(name)); token != "" {
			if r.Header.Get("X-WorkMesh-Token") == token {
				return true
			}
			wmhttp.JSON(w, http.StatusUnauthorized, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "STREAM_AUTH_REQUIRED"}, "message": "流式接口需要有效令牌"})
			return false
		}
	}
	// 已登录的本地会话可直接建立流式连接；令牌配置后则必须使用令牌，避免匿名执行命令。
	if sid := coreSessionID(r); sid != "" {
		if _, err := localCore.Current(sid); err == nil {
			return true
		}
	}
	wmhttp.JSON(w, http.StatusUnauthorized, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "STREAM_AUTH_REQUIRED"}, "message": "流式接口需要登录会话或 X-WorkMesh-Token"})
	return false
}
