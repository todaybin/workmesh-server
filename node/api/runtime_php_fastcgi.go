// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sort"
	"strings"
	"time"
)

const (
	fastCGIVersion      = 1
	fastCGIBeginRequest = 1
	fastCGIEndRequest   = 3
	fastCGIParams       = 4
	fastCGIStdin        = 5
	fastCGIStdout       = 6
	fastCGIStderr       = 7
)

// writeFastCGIRecord 写入单条 FastCGI 帧，并限制内容长度符合协议上限。
func writeFastCGIRecord(writer io.Writer, recordType byte, requestID uint16, content []byte) error {
	if len(content) > 65535 {
		return errors.New("FastCGI 记录过大")
	}
	padding := byte((8 - len(content)%8) % 8)
	header := []byte{fastCGIVersion, recordType, byte(requestID >> 8), byte(requestID), byte(len(content) >> 8), byte(len(content)), padding, 0}
	if _, err := writer.Write(header); err != nil {
		return err
	}
	if _, err := writer.Write(content); err != nil {
		return err
	}
	if padding > 0 {
		_, err := writer.Write(make([]byte, int(padding)))
		return err
	}
	return nil
}

// appendFastCGILength 编码 FastCGI 名称和值的短长度或长长度字段。
func appendFastCGILength(target []byte, length int) []byte {
	if length < 128 {
		return append(target, byte(length))
	}
	return binary.BigEndian.AppendUint32(target, uint32(length)|1<<31)
}

// encodeFastCGIParams 按稳定顺序编码参数，保证测试和 FastCGI 服务端解析一致。
func encodeFastCGIParams(values map[string]string) []byte {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	content := make([]byte, 0, 512)
	for _, key := range keys {
		value := values[key]
		content = appendFastCGILength(content, len(key))
		content = appendFastCGILength(content, len(value))
		content = append(content, key...)
		content = append(content, value...)
	}
	return content
}

// readFastCGIStatus 请求 PHP-FPM status，并限制网络、响应和 stderr 资源。
func readFastCGIStatus(address string, timeout time.Duration) ([]map[string]any, error) {
	connection, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(timeout))
	if err := writeFastCGIRecord(connection, fastCGIBeginRequest, 1, []byte{0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return nil, err
	}
	_, port, splitErr := net.SplitHostPort(address)
	if splitErr != nil {
		return nil, splitErr
	}
	params := encodeFastCGIParams(map[string]string{"REQUEST_METHOD": "GET", "REQUEST_URI": "/status", "SCRIPT_FILENAME": "/status", "SCRIPT_NAME": "/status", "QUERY_STRING": "", "CONTENT_TYPE": "", "CONTENT_LENGTH": "0", "SERVER_NAME": "localhost", "SERVER_PORT": port, "REMOTE_ADDR": "127.0.0.1", "GATEWAY_INTERFACE": "CGI/1.1"})
	for _, record := range []struct {
		typ     byte
		content []byte
	}{{fastCGIParams, params}, {fastCGIParams, nil}, {fastCGIStdin, nil}} {
		if err := writeFastCGIRecord(connection, record.typ, 1, record.content); err != nil {
			return nil, err
		}
	}
	stdout, stderr := bytes.Buffer{}, bytes.Buffer{}
	for {
		header := make([]byte, 8)
		if _, err := io.ReadFull(connection, header); err != nil {
			return nil, err
		}
		if header[0] != fastCGIVersion || binary.BigEndian.Uint16(header[2:4]) != 1 {
			return nil, errors.New("FastCGI 响应头无效")
		}
		length, padding := int(binary.BigEndian.Uint16(header[4:6])), int(header[6])
		content := make([]byte, length)
		if _, err := io.ReadFull(connection, content); err != nil {
			return nil, err
		}
		if padding > 0 {
			if _, err := io.CopyN(io.Discard, connection, int64(padding)); err != nil {
				return nil, err
			}
		}
		switch header[1] {
		case fastCGIStdout:
			if stdout.Len()+len(content) > 1<<20 {
				return nil, errors.New("FastCGI 状态响应超过 1 MiB 限制")
			}
			_, _ = stdout.Write(content)
		case fastCGIStderr:
			if stderr.Len()+len(content) <= 64<<10 {
				_, _ = stderr.Write(content)
			}
		case fastCGIEndRequest:
			if message := strings.TrimSpace(stderr.String()); message != "" {
				return nil, errors.New(message)
			}
			return parseFastCGIStatusPayload(stdout.String())
		}
	}
}

// parseFastCGIStatusPayload 将 PHP-FPM 文本状态转换为前端使用的键值列表。
func parseFastCGIStatusPayload(payload string) ([]map[string]any, error) {
	if _, body, found := strings.Cut(payload, "\r\n\r\n"); found {
		payload = body
	} else if _, body, found := strings.Cut(payload, "\n\n"); found {
		payload = body
	}
	status := []map[string]any{}
	scanner := bufio.NewScanner(strings.NewReader(payload))
	for scanner.Scan() {
		key, value, found := strings.Cut(strings.TrimSpace(scanner.Text()), ":")
		if found && strings.TrimSpace(key) != "" {
			status = append(status, map[string]any{"key": strings.TrimSpace(key), "value": strings.TrimSpace(value)})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(status) == 0 {
		return nil, errors.New("FastCGI 状态响应为空")
	}
	return status, nil
}
