// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
	"strings"
)

// registerOpenRestyRoutes 注册 OpenResty 配置、模块、缓存和 HTTPS 管理接口。
func registerOpenRestyRoutes(mux *http.ServeMux, svc *service.WebsiteService) {
	mux.HandleFunc("GET /api/v2/openresty", func(w http.ResponseWriter, r *http.Request) {
		content, err := svc.OpenRestyFile()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		cfg := svc.GetOpenResty()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"content": content, "version": cfg.Version, "enabled": cfg.Enabled, "updatedAt": cfg.UpdatedAt}})
	})
	mux.HandleFunc("GET /api/v2/openresty/status", func(w http.ResponseWriter, r *http.Request) {
		status := svc.ProbeOpenResty(r.Context())
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": status})
	})
	mux.HandleFunc("GET /api/v2/openresty/modules", func(w http.ResponseWriter, r *http.Request) {
		modules := openRestyModuleData(svc.GetOpenResty().Modules)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"mirror": "", "modules": modules, "dynamicSupported": true}})
	})
	mux.HandleFunc("GET /api/v2/openresty/https", func(w http.ResponseWriter, r *http.Request) {
		cfg := svc.GetOpenResty()
		status := svc.ProbeOpenResty(r.Context())
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"https": cfg.DefaultHTTPS, "open": cfg.DefaultHTTPS, "enabled": cfg.DefaultHTTPS, "sslRejectHandshake": cfg.SSLRejectHandshake, "available": status.Available, "configValid": status.ConfigValid}})
	})
	mux.HandleFunc("POST /api/v2/openresty/operate", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Operate   string `json:"operate"`
			Operation string `json:"operation"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, 400, err)
			return
		}
		if in.Operation == "" {
			in.Operation = in.Operate
		}
		status, err := svc.OperateOpenResty(r.Context(), in.Operation)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": status})
	})
	for _, path := range []string{"/api/v2/openresty/clear", "/api/v2/openresty/cache/clear"} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			if err := svc.ClearOpenRestyCache(); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"cleared": true}})
		})
	}
	mux.HandleFunc("POST /api/v2/openresty/update", openRestyUpdate(svc))
	mux.HandleFunc("POST /api/v2/openresty/file", openRestyUpdate(svc))
	mux.HandleFunc("GET /api/v2/openresty/scope", func(w http.ResponseWriter, r *http.Request) {
		scope := strings.TrimSpace(r.URL.Query().Get("scope"))
		result, err := svc.OpenRestyScope(scope)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
	})
	mux.HandleFunc("POST /api/v2/openresty/scope", openRestyUpdate(svc))
	mux.HandleFunc("POST /api/v2/openresty/build", openRestyUpdate(svc))
	mux.HandleFunc("POST /api/v2/openresty/modules", openRestyUpdate(svc))
	mux.HandleFunc("POST /api/v2/openresty/modules/update", openRestyUpdate(svc))
	mux.HandleFunc("POST /api/v2/openresty/https", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Operate            string `json:"operate"`
			SSLRejectHandshake *bool  `json:"sslRejectHandshake"`
		}
		if err := decodeJSON(r, &req); err != nil || (req.Operate != "enable" && req.Operate != "disable") {
			writeError(w, http.StatusBadRequest, errors.New("HTTPS 操作无效"))
			return
		}
		cfg := svc.GetOpenResty()
		cfg.DefaultHTTPS = req.Operate == "enable"
		if req.SSLRejectHandshake != nil {
			cfg.SSLRejectHandshake = *req.SSLRejectHandshake
		}
		result, err := svc.UpdateOpenResty(cfg)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
	})
}

// openRestyUpdate 返回 OpenResty 多路径更新处理器，并保持各历史路径响应契约。
func openRestyUpdate(svc *service.WebsiteService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var raw map[string]any
		if err := decodeJSON(r, &raw); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if openRestyFileUpdate(svc, raw, r.URL.Path, w) || openRestyScopeUpdate(svc, raw, r, w) || openRestyBuild(svc, raw, r, w) || openRestyModuleUpdate(svc, raw, r.URL.Path, w) {
			return
		}
		var req model.OpenRestyConfig
		encoded, _ := json.Marshal(raw)
		if err := json.Unmarshal(encoded, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		cfg := svc.GetOpenResty()
		if req.Version != "" {
			cfg.Version = req.Version
		}
		if req.ConfigContent != "" {
			cfg.ConfigContent = req.ConfigContent
		}
		if req.Modules != nil {
			seen := make(map[string]struct{}, len(req.Modules))
			for _, module := range req.Modules {
				name := strings.TrimSpace(module.Name)
				if name == "" {
					writeError(w, http.StatusBadRequest, errors.New("OpenResty module name is required"))
					return
				}
				if _, ok := seen[name]; ok {
					writeError(w, http.StatusBadRequest, errors.New("duplicate OpenResty module"))
					return
				}
				seen[name] = struct{}{}
			}
			cfg.Modules = req.Modules
		}
		result, err := svc.UpdateOpenResty(cfg)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
	}
}

func openRestyModuleData(modules []model.OpenRestyModule) []map[string]any {
	result := make([]map[string]any, 0, len(modules))
	for _, module := range modules {
		buildMode := module.BuildMode
		if buildMode == "" {
			buildMode = "dynamic"
		}
		provider := module.Provider
		if provider == "" {
			provider = "local"
		}
		buildStatus := module.BuildStatus
		if buildStatus == "" {
			buildStatus = "ready"
		}
		loadStatus := module.LoadStatus
		if loadStatus == "" {
			if module.Enabled {
				loadStatus = "enabled"
			} else {
				loadStatus = "disabled"
			}
		}
		result = append(result, map[string]any{
			"name": module.Name, "enabled": module.Enabled, "enable": module.Enabled,
			"custom": module.Custom, "script": module.Script, "packages": module.Packages,
			"params": module.Params, "buildMode": buildMode, "provider": provider,
			"loadOrder": module.LoadOrder, "buildStatus": buildStatus,
			"loadStatus": loadStatus, "lastError": module.LastError,
		})
	}
	return result
}

func openRestyModuleUpdate(svc *service.WebsiteService, raw map[string]any, path string, w http.ResponseWriter) bool {
	if !strings.HasSuffix(path, "/modules") && !strings.HasSuffix(path, "/modules/update") {
		return false
	}
	name, _ := raw["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 || strings.ContainsAny(name, " /\\\x00\r\n") {
		writeError(w, http.StatusBadRequest, errors.New("OpenResty module name is required and must be valid"))
		return true
	}
	operation, _ := raw["operate"].(string)
	operation = strings.ToLower(strings.TrimSpace(operation))
	if operation == "" {
		operation = "update"
	}
	if operation != "create" && operation != "update" && operation != "delete" {
		writeError(w, http.StatusBadRequest, errors.New("OpenResty module operation is invalid"))
		return true
	}
	cfg := svc.GetOpenResty()
	modules := append([]model.OpenRestyModule(nil), cfg.Modules...)
	index := -1
	for i := range modules {
		if modules[i].Name == name {
			index = i
			break
		}
	}
	if operation == "create" && index >= 0 {
		writeError(w, http.StatusConflict, errors.New("OpenResty module already exists"))
		return true
	}
	if operation == "update" && index < 0 {
		writeError(w, http.StatusNotFound, errors.New("OpenResty module does not exist"))
		return true
	}
	if operation == "delete" {
		if index >= 0 && !modules[index].Custom {
			writeError(w, http.StatusBadRequest, errors.New("built-in OpenResty module cannot be deleted"))
			return true
		}
		if index >= 0 {
			modules = append(modules[:index], modules[index+1:]...)
		}
		cfg.Modules = modules
		result, err := svc.UpdateOpenResty(cfg)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return true
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"deleted": name, "modules": openRestyModuleData(result.Modules)}})
		return true
	}

	module := model.OpenRestyModule{Name: name, Custom: operation == "create"}
	if index >= 0 {
		module = modules[index]
	}
	module.Name = name
	module.Custom = module.Custom || operation == "create"
	if value, ok, err := openRestyBool(raw, "enable", "enabled"); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return true
	} else if ok {
		module.Enabled = value
	}
	if value, ok := openRestyString(raw, "script"); ok {
		module.Script = value
	}
	if value, ok := openRestyString(raw, "packages"); ok {
		module.Packages = value
	}
	if value, ok := openRestyString(raw, "params"); ok {
		module.Params = value
	}
	if value, ok := openRestyString(raw, "buildMode"); ok {
		if value != "dynamic" && value != "static" {
			writeError(w, http.StatusBadRequest, errors.New("OpenResty module buildMode is invalid"))
			return true
		}
		module.BuildMode = value
	}
	if module.BuildMode == "" {
		module.BuildMode = "dynamic"
	}
	if value, ok := openRestyString(raw, "provider"); ok {
		if value != "local" && value != "prebuilt" {
			writeError(w, http.StatusBadRequest, errors.New("OpenResty module provider is invalid"))
			return true
		}
		module.Provider = value
	}
	if module.Provider == "" {
		module.Provider = "local"
	}
	if value, ok, err := openRestyInt(raw, "loadOrder"); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return true
	} else if ok {
		if value < 0 || value > 9999 {
			writeError(w, http.StatusBadRequest, errors.New("OpenResty module loadOrder is invalid"))
			return true
		}
		module.LoadOrder = value
	}
	if module.BuildStatus == "" {
		module.BuildStatus = "pending"
	}
	if module.Enabled {
		module.LoadStatus = "enabled"
	} else {
		module.LoadStatus = "disabled"
	}
	if index < 0 {
		modules = append(modules, module)
	} else {
		modules[index] = module
	}
	cfg.Modules = modules
	result, err := svc.UpdateOpenResty(cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return true
	}
	for _, item := range result.Modules {
		if item.Name == name {
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": openRestyModuleData([]model.OpenRestyModule{item})[0]})
			return true
		}
	}
	writeError(w, http.StatusInternalServerError, errors.New("OpenResty module persistence failed"))
	return true
}

func openRestyString(raw map[string]any, key string) (string, bool) {
	value, ok := raw[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return strings.TrimSpace(text), ok
}

func openRestyBool(raw map[string]any, keys ...string) (bool, bool, error) {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		boolean, ok := value.(bool)
		if !ok {
			return false, false, fmt.Errorf("%s must be a boolean", key)
		}
		return boolean, true, nil
	}
	return false, false, nil
}

func openRestyInt(raw map[string]any, key string) (int, bool, error) {
	value, ok := raw[key]
	if !ok {
		return 0, false, nil
	}
	switch value := value.(type) {
	case float64:
		if value != float64(int(value)) {
			return 0, false, fmt.Errorf("%s must be an integer", key)
		}
		return int(value), true, nil
	case int:
		return value, true, nil
	default:
		return 0, false, fmt.Errorf("%s must be an integer", key)
	}
}

// openRestyFileUpdate 处理完整配置文件写入路径。
func openRestyFileUpdate(svc *service.WebsiteService, raw map[string]any, path string, w http.ResponseWriter) bool {
	if !strings.HasSuffix(path, "/file") {
		return false
	}
	content, _ := raw["content"].(string)
	backup, _ := raw["backup"].(bool)
	if err := svc.UpdateOpenRestyFile(content, backup); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return true
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"updated": true, "backup": backup}})
	return true
}

// openRestyScopeUpdate 处理 OpenResty 作用域读取和参数更新路径。
func openRestyScopeUpdate(svc *service.WebsiteService, raw map[string]any, r *http.Request, w http.ResponseWriter) bool {
	scopePath := strings.HasSuffix(r.URL.Path, "/scope")
	legacyUpdatePath := strings.HasSuffix(r.URL.Path, "/update") && strings.TrimSpace(openRestyStringValue(raw["scope"])) != ""
	if !scopePath && !legacyUpdatePath {
		return false
	}
	scope := strings.TrimSpace(openRestyStringValue(raw["scope"]))
	params := openRestyScopeParams(raw["params"])
	backup, _ := raw["backup"].(bool)
	if r.Method == http.MethodPost && len(params) > 0 {
		if err := svc.UpdateOpenRestyScope(scope, params, backup); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return true
		}
		result, err := svc.OpenRestyScope(scope)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return true
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
		return true
	}
	if legacyUpdatePath {
		writeError(w, http.StatusBadRequest, errors.New("OpenResty 作用域参数不能为空"))
		return true
	}
	result, err := svc.OpenRestyScope(scope)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return true
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
	return true
}

func openRestyStringValue(value any) string {
	text, _ := value.(string)
	return text
}

// openRestyScopeParams 接受 1Panel 的对象格式，也兼容旧客户端发送的指令数组。
func openRestyScopeParams(raw any) map[string]string {
	result := map[string]string{}
	switch values := raw.(type) {
	case map[string]any:
		for key, value := range values {
			if text, ok := value.(string); ok {
				result[strings.TrimSpace(key)] = strings.TrimSpace(text)
			}
		}
	case []any:
		for _, item := range values {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			name := strings.TrimSpace(openRestyStringValue(entry["name"]))
			if name == "" {
				if value, ok := entry["limit_conn"].(string); ok {
					result["limit_conn"] = strings.TrimSpace(value)
				}
				if value, ok := entry["limit_rate"].(string); ok {
					result["limit_rate"] = strings.TrimSpace(value)
				}
				continue
			}
			switch params := entry["params"].(type) {
			case []any:
				parts := make([]string, 0, len(params))
				for _, param := range params {
					if text, ok := param.(string); ok {
						parts = append(parts, strings.TrimSpace(text))
					}
				}
				if len(parts) > 0 {
					result[name] = strings.Join(parts, " ")
				}
			case string:
				result[name] = strings.TrimSpace(params)
			}
		}
	}
	return result
}

// openRestyBuild 处理模块构建请求并返回真实探针状态。
func openRestyBuild(svc *service.WebsiteService, raw map[string]any, r *http.Request, w http.ResponseWriter) bool {
	if !strings.HasSuffix(r.URL.Path, "/build") {
		return false
	}
	var modules []string
	if values, ok := raw["modules"].([]any); ok {
		for _, value := range values {
			if name, ok := value.(string); ok {
				modules = append(modules, name)
			}
		}
	}
	status, err := svc.BuildOpenResty(r.Context(), modules)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return true
	}
	wmhttp.JSON(w, http.StatusAccepted, map[string]any{"code": 200, "data": map[string]any{"status": "validated", "probe": status}})
	return true
}
