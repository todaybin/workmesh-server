// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/logsource"
)

// systemLogPageContent renders only the current page for the legacy content field.
func systemLogPageContent(items []map[string]any) string {
	var content strings.Builder
	for _, item := range items {
		raw, ok := item["raw"].(string)
		if !ok || raw == "" {
			continue
		}
		if content.Len()+len(raw)+1 > maxSystemLogReadBytes {
			break
		}
		content.WriteString(raw)
		content.WriteByte('\n')
	}
	return content.String()
}

// hasSystemLogReadOptions 判断请求是否显式提供了 v2 日志读取参数。
func hasSystemLogReadOptions(values map[string]any) bool {
	for _, key := range []string{"pageSize", "cursor", "startTime", "endTime", "keyword", "priority", "service"} {
		if value, ok := values[key]; ok && strings.TrimSpace(fmt.Sprint(value)) != "" {
			return true
		}
	}
	return false
}

const maxSystemLogReadBytes = 8 << 20
const maxSystemLogReadItems = 100000

type systemLogReadPage struct {
	items      []map[string]any
	hasMore    bool
	nextCursor string
}

type systemLogReadCursor struct {
	Offset int `json:"offset"`
}

// decodeSystemLogReadCursor 解码无状态游标并限制跳过范围。
func decodeSystemLogReadCursor(value string) (*systemLogReadCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid system log cursor")
	}
	var cursor systemLogReadCursor
	if json.Unmarshal(b, &cursor) != nil || cursor.Offset < 0 || cursor.Offset > maxSystemLogReadItems {
		return nil, fmt.Errorf("invalid system log cursor")
	}
	return &cursor, nil
}

func encodeSystemLogReadCursor(offset int) string {
	b, _ := json.Marshal(systemLogReadCursor{Offset: offset})
	return base64.RawURLEncoding.EncodeToString(b)
}

func paginateSystemLogItems(items []map[string]any, pageSize int, cursor *systemLogReadCursor) systemLogReadPage {
	offset := 0
	if cursor != nil {
		offset = cursor.Offset
	}
	if offset > len(items) {
		offset = len(items)
	}
	end := offset + pageSize
	if end > len(items) {
		end = len(items)
	}
	page := systemLogReadPage{items: items[offset:end], hasMore: end < len(items)}
	if page.hasMore {
		page.nextCursor = encodeSystemLogReadCursor(end)
	}
	return page
}

// readSystemLogFile 读取受限文件并按请求条件转换为统一日志条目。
func readSystemLogFile(path string, values map[string]any) ([]map[string]any, string, error) {
	result, err := (logsource.FileSource{}).Read(context.Background(), logsource.Request{
		Paths: []string{path}, MaxBytesPerFile: maxSystemLogReadBytes, MaxLines: maxSystemLogReadItems, FirstAvailable: true,
	})
	if err != nil {
		return nil, "", err
	}
	items := make([]map[string]any, 0, 256)
	var content strings.Builder
	for _, entry := range result.Lines {
		line := entry.Text
		if item, ok := parseSystemLogLine(line, values); ok {
			items = append(items, item)
			if content.Len() < maxSystemLogReadBytes {
				content.WriteString(line)
				content.WriteByte('\n')
			}
		}
		if len(items) >= maxSystemLogReadItems {
			break
		}
	}
	return items, content.String(), nil
}

func parseSystemLogLine(line string, values map[string]any) (map[string]any, bool) {
	if strings.TrimSpace(line) == "" {
		return nil, false
	}
	priority, service, message := "6", "", strings.TrimSpace(line)
	if idx := strings.Index(message, " "); idx > 0 {
		first := message[:idx]
		if _, err := strconv.Atoi(first); err == nil && len(first) <= 1 {
			priority, message = first, strings.TrimSpace(message[idx:])
		}
	}
	keyword := strings.ToLower(valueString(values, "keyword"))
	if keyword != "" && !strings.Contains(strings.ToLower(line), keyword) {
		return nil, false
	}
	if requested := strings.TrimSpace(valueString(values, "priority")); requested != "" && requested != priority {
		return nil, false
	}
	if requested := strings.TrimSpace(valueString(values, "service")); requested != "" && !strings.EqualFold(requested, service) {
		return nil, false
	}
	logTime := ""
	if fields := strings.Fields(message); len(fields) > 0 {
		if parsed, err := time.Parse(time.RFC3339, fields[0]); err == nil {
			logTime = parsed.UTC().Format(time.RFC3339)
			if start := parseSystemLogTime(valueString(values, "startTime")); !start.IsZero() && parsed.Before(start) {
				return nil, false
			}
			if end := parseSystemLogTime(valueString(values, "endTime")); !end.IsZero() && parsed.After(end) {
				return nil, false
			}
		}
	}
	return map[string]any{"time": logTime, "priority": priority, "service": service, "message": message, "raw": line}, true
}

func parseSystemLogTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	parsed, _ := time.Parse(time.RFC3339, strings.TrimSpace(value))
	return parsed
}

func journalValue(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

// collectSystemLogItems 优先使用 journalctl，缺失时读取受限的系统日志文件。
func collectSystemLogItems(values map[string]any) ([]map[string]any, string, error) {
	if journalctl, err := exec.LookPath("journalctl"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		args := []string{"--no-pager", "--output=json", "--reverse", "--lines=100000"}
		if service := valueString(values, "service"); service != "" {
			args = append(args, "-u", service)
		}
		if priority := valueString(values, "priority"); priority != "" {
			args = append(args, "--priority", priority+".."+priority)
		}
		if start := valueString(values, "startTime"); start != "" {
			args = append(args, "--since", start)
		}
		if end := valueString(values, "endTime"); end != "" {
			args = append(args, "--until", end)
		}
		result, runErr := (logsource.CommandSource{}).Read(ctx, logsource.CommandRequest{
			Program: journalctl, Args: args, MaxBytes: maxSystemLogReadBytes, MaxLines: maxSystemLogReadItems,
		})
		if runErr == nil {
			items := make([]map[string]any, 0, 256)
			for _, line := range result.Lines {
				var raw map[string]any
				if json.Unmarshal([]byte(line.Text), &raw) != nil {
					continue
				}
				message := journalValue(raw, "MESSAGE")
				if keyword := strings.ToLower(valueString(values, "keyword")); keyword != "" && !strings.Contains(strings.ToLower(message), keyword) {
					continue
				}
				items = append(items, map[string]any{"time": formatSystemLogTimestamp(journalValue(raw, "__REALTIME_TIMESTAMP")), "priority": journalValue(raw, "PRIORITY"), "service": journalValue(raw, "_SYSTEMD_UNIT"), "message": message, "raw": line.Text})
				if len(items) >= maxSystemLogReadItems {
					break
				}
			}
			return items, "journalctl", nil
		}
	}
	for _, path := range []string{"/var/log/syslog", "/var/log/messages", "/var/log/system.log"} {
		if _, err := os.Stat(path); err != nil || !allowedLogPath(path) {
			continue
		}
		items, _, readErr := readSystemLogFile(path, values)
		if readErr == nil {
			return items, "file", nil
		}
	}
	return []map[string]any{}, "file", nil
}
