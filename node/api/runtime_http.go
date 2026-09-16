// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// runtimeBody 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeBody(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return map[string]any{}, nil
	}
	var v map[string]any
	decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	if err := decoder.Decode(&v); err != nil {
		if strings.Contains(err.Error(), "EOF") {
			return map[string]any{}, nil
		}
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("请求 JSON 只能包含一个对象")
	}
	if v == nil {
		return nil, errors.New("请求 JSON 必须是对象")
	}
	if v == nil {
		v = map[string]any{}
	}
	return v, nil
}

// runtimeString 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeString(v map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := v[k].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
		if n, ok := v[k].(float64); ok && n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10)
		}
		if n, ok := v[k].(int); ok {
			return strconv.Itoa(n)
		}
	}
	return ""
}

// runtimeOK 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeOK(w http.ResponseWriter, data any) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}

// runtimeSuccess 对齐原 1Panel Success：data 字段存在且为 null，不返回业务对象。
func runtimeSuccess(w http.ResponseWriter) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "message": "success", "data": nil})
}

// runtimeErr 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeErr(w http.ResponseWriter, status int, msg string) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": msg})
}

// runtimeErrData 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeErrData(w http.ResponseWriter, status int, msg string, data any) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": msg, "data": data})
}

// RegisterRuntimeToolboxRoutes 注册运行时、终端、SSH 与工具箱接口。
func RegisterRuntimeToolboxRoutes(mux *http.ServeMux) {
	s := getRuntimeStore()
	registerRuntimeRoutes(mux, s)
	registerTerminalRoutes(mux)
	registerSSHRoutes(mux, s)
	registerToolboxRoutes(mux, s)
}

// isRuntimeToolboxRoute 执行运行时相关处理并返回可观测错误。
func isRuntimeToolboxRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	for _, prefix := range []string{"/api/v2/runtimes", "/api/v2/hosts/terminal", "/api/v2/settings/ssh", "/api/v2/settings/terminal/ai", "/api/v2/toolbox"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// runtimePage 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimePage(body map[string]any) (int, int, error) {
	parse := func(key string, fallback int) (int, error) {
		raw, ok := body[key]
		if !ok || raw == nil {
			return fallback, nil
		}
		value, ok := raw.(float64)
		if !ok || value < 1 || value != float64(int(value)) || value > 10000 {
			return 0, fmt.Errorf("%s 必须是正整数", key)
		}
		return int(value), nil
	}
	page, err := parse("page", 1)
	if err != nil {
		return 0, 0, err
	}
	pageSize, err := parse("pageSize", 20)
	if err != nil {
		return 0, 0, err
	}
	if pageSize > 200 {
		return 0, 0, errors.New("pageSize 不能超过 200")
	}
	return page, pageSize, nil
}

// runtimeStatusMatches 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeStatusMatches(item runtimeRecord, filter string) bool {
	status := strings.ToLower(strings.TrimSpace(item.Status))
	if filter == "normal" && strings.EqualFold(item.Type, "php") {
		// PHP uses Normal as the list's healthy bucket while its actual state is
		// often Running/Building/Stopped.  Keep failures out of this bucket.
		return status != "error" && status != "failed" && status != "unhealthy"
	}
	return status == filter
}
