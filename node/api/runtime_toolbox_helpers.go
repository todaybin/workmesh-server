// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"os"
	"strings"
	"time"
)

// pageRecordsGeneric 执行运行时相关处理并返回可观测错误。
func pageRecordsGeneric(items []map[string]any, body map[string]any) map[string]any {
	page, size := 1, 100
	if n, ok := body["page"].(float64); ok && n >= 1 {
		page = int(n)
	}
	if n, ok := body["pageSize"].(float64); ok && n >= 1 {
		size = int(n)
	}
	if size > 500 {
		size = 500
	}
	start := (page - 1) * size
	if start > len(items) {
		start = len(items)
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	return map[string]any{"items": items[start:end], "total": len(items), "page": page, "pageSize": size}
}

// cloneRuntimeMap 复制运行时数据，避免调用方共享可变状态。
func cloneRuntimeMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

// toolboxGetData 从受限系统文件和本地状态读取工具箱信息，不执行用户输入命令。
func toolboxGetData(s *runtimeStore, path string) map[string]any {
	switch path {
	case "/api/v2/toolbox/device/users":
		items := make([]map[string]any, 0)
		if raw, err := os.ReadFile("/etc/passwd"); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				fields := strings.SplitN(line, ":", 7)
				if len(fields) < 7 || fields[0] == "" {
					continue
				}
				items = append(items, map[string]any{"name": fields[0], "uid": fields[2], "gid": fields[3], "home": fields[5], "shell": fields[6]})
				if len(items) >= 200 {
					break
				}
			}
		}
		return map[string]any{"items": items, "total": len(items), "status": "ready", "supported": len(items) > 0}
	case "/api/v2/toolbox/device/zone/options":
		zone := time.Local.String()
		if zone == "" {
			zone = "Local"
		}
		return map[string]any{"items": []map[string]any{{"name": zone, "value": zone}}, "current": zone, "status": "ready"}
	case "/api/v2/toolbox/fail2ban/base", "/api/v2/toolbox/fail2ban/load/conf":
		configPath := "/etc/fail2ban/jail.local"
		content := ""
		if raw, err := os.ReadFile(configPath); err == nil {
			content = string(raw)
			if len(content) > 1<<20 {
				content = content[:1<<20]
			}
		}
		return map[string]any{"path": configPath, "content": content, "installed": content != "", "enabled": content != "", "status": "ready"}
	case "/api/v2/toolbox/ftp/base":
		s.mu.RLock()
		value := s.state.Settings["ftp"]
		s.mu.RUnlock()
		if value == nil {
			value = map[string]any{"enabled": false, "port": 21, "status": "not_configured"}
		}
		if config, ok := value.(map[string]any); ok {
			return config
		}
		return map[string]any{"config": value, "status": "ready"}
	default:
		return map[string]any{"status": "unsupported", "path": path}
	}
}
