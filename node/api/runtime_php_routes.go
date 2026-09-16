// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// registerPHPConfigurationRoutes 注册运行时相关 HTTP 路由，并保持请求响应契约。
func registerPHPConfigurationRoutes(mux *http.ServeMux, s *runtimeStore) {
	registerPHPINIConfigRoutes(mux, s)
	registerPHPConfigFileRoutes(mux, s)
	registerPHPFPMConfigRoutes(mux, s)
	registerPHPContainerRoutes(mux, s)
}

// registerPHPINIConfigRoutes 注册 PHP 主配置的读取和更新接口。
func registerPHPINIConfigRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("GET /api/v2/runtimes/php/config/{id}", phpINIConfigReadHandler(s))
	mux.HandleFunc("POST /api/v2/runtimes/php/config", phpINIConfigUpdateHandler(s))
}

// phpINIConfigReadHandler 读取 PHP 主配置并返回前端兼容字段。
func phpINIConfigReadHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, index := runtimeByID(s, r.PathValue("id"))
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		path, err := phpRuntimeConfigPath(item, "php")
		if err != nil {
			runtimeErr(w, http.StatusConflict, err.Error())
			return
		}
		content, err := readRuntimeConfigFile(path)
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		params := parseRuntimeINI(string(content))
		disabled := []string{}
		for _, value := range strings.Split(params["disable_functions"], ",") {
			if value = strings.TrimSpace(value); value != "" {
				disabled = append(disabled, value)
			}
		}
		runtimeOK(w, map[string]any{"id": item.ID, "params": params, "disableFunctions": disabled, "uploadMaxSize": params["upload_max_filesize"], "maxExecutionTime": params["max_execution_time"]})
	}
}

// phpINIConfigUpdateHandler 校验并原子更新 PHP 主配置。
func phpINIConfigUpdateHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		item, index := runtimeByID(s, runtimeString(body, "id", "runtimeId"))
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		path, err := phpRuntimeConfigPath(item, "php")
		if err != nil {
			runtimeErr(w, http.StatusConflict, err.Error())
			return
		}
		old, err := readRuntimeConfigFile(path)
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		updates := map[string]string{}
		if params, ok := body["params"].(map[string]any); ok {
			for key, value := range params {
				updates[key] = fmt.Sprint(value)
			}
		}
		if raw, exists := body["disableFunctions"]; exists {
			values, ok := raw.([]any)
			if !ok {
				runtimeErr(w, http.StatusBadRequest, "disableFunctions 必须是数组")
				return
			}
			functions := make([]string, 0, len(values))
			for _, value := range values {
				name, ok := value.(string)
				if !ok || !phpINIKeyPattern.MatchString(name) {
					runtimeErr(w, http.StatusBadRequest, "禁用函数名称无效")
					return
				}
				functions = append(functions, name)
			}
			updates["disable_functions"] = strings.Join(functions, ",")
		}
		if value := runtimeString(body, "uploadMaxSize"); value != "" {
			updates["upload_max_filesize"] = value
		}
		if value := runtimeString(body, "maxExecutionTime"); value != "" {
			updates["max_execution_time"] = value
		}
		content, err := updateRuntimeINI(string(old), updates)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := updatePHPFileAndRestart(s.commandExecutor(), item, path, []byte(content)); err != nil {
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		runtimeOK(w, nil)
	}
}

// registerPHPConfigFileRoutes 注册 PHP 与 FPM 原始配置文件接口。
func registerPHPConfigFileRoutes(mux *http.ServeMux, s *runtimeStore) {
	for _, kind := range []string{"php", "fpm"} {
		kind := kind
		mux.HandleFunc("GET /api/v2/runtimes/php/"+kind+"/file/{id}", func(w http.ResponseWriter, r *http.Request) {
			item, index := runtimeByID(s, r.PathValue("id"))
			if index < 0 {
				runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
				return
			}
			path, err := phpRuntimeConfigPath(item, kind)
			if err != nil {
				runtimeErr(w, http.StatusConflict, err.Error())
				return
			}
			content, err := readRuntimeConfigFile(path)
			if err != nil {
				runtimeErr(w, http.StatusNotFound, err.Error())
				return
			}
			runtimeOK(w, map[string]any{"name": filepath.Base(path), "path": path, "content": string(content)})
		})
	}
	mux.HandleFunc("POST /api/v2/runtimes/php/file", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		item, index := runtimeByID(s, runtimeString(body, "id", "runtimeId"))
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		path, err := phpRuntimeConfigPath(item, runtimeString(body, "type"))
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		content, err := readRuntimeConfigFile(path)
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		runtimeOK(w, map[string]any{"name": filepath.Base(path), "path": path, "content": string(content)})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/update", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		item, index := runtimeByID(s, runtimeString(body, "id", "runtimeId"))
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		path, err := phpRuntimeConfigPath(item, runtimeString(body, "type"))
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		content, ok := body["content"].(string)
		if !ok {
			runtimeErr(w, http.StatusBadRequest, "PHP 配置内容不能为空")
			return
		}
		if err := updatePHPFileAndRestart(s.commandExecutor(), item, path, []byte(content)); err != nil {
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		runtimeOK(w, nil)
	})
}

// registerPHPFPMConfigRoutes 注册 FPM 参数和进程状态接口。
func registerPHPFPMConfigRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("GET /api/v2/runtimes/php/fpm/config/{id}", func(w http.ResponseWriter, r *http.Request) {
		item, index := runtimeByID(s, r.PathValue("id"))
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		path, err := phpRuntimeConfigPath(item, "fpm")
		if err != nil {
			runtimeErr(w, http.StatusConflict, err.Error())
			return
		}
		content, err := readRuntimeConfigFile(path)
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		runtimeOK(w, map[string]any{"id": item.ID, "params": parseRuntimeINI(string(content))})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/fpm/config", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		item, index := runtimeByID(s, runtimeString(body, "id", "runtimeId"))
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		params, ok := body["params"].(map[string]any)
		if !ok {
			runtimeErr(w, http.StatusBadRequest, "FPM params 必须是对象")
			return
		}
		path, err := phpRuntimeConfigPath(item, "fpm")
		if err != nil {
			runtimeErr(w, http.StatusConflict, err.Error())
			return
		}
		old, err := readRuntimeConfigFile(path)
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		updates := make(map[string]string, len(params))
		for key, value := range params {
			updates[key] = fmt.Sprint(value)
		}
		content, err := updateRuntimeINI(string(old), updates)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := updatePHPFileAndRestart(s.commandExecutor(), item, path, []byte(content)); err != nil {
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		runtimeOK(w, nil)
	})
	mux.HandleFunc("GET /api/v2/runtimes/php/fpm/status/{id}", func(w http.ResponseWriter, r *http.Request) {
		item, index := runtimeByID(s, r.PathValue("id"))
		if index < 0 || normalizeRuntimeTypeFilter(item.Type) != "php" || item.Port < 1 || item.Port > 65535 {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在或尚未启动")
			return
		}
		status, err := readFastCGIStatus(net.JoinHostPort("127.0.0.1", strconv.Itoa(item.Port)), 10*time.Second)
		if err != nil {
			runtimeErr(w, http.StatusBadGateway, "读取 PHP-FPM 状态失败: "+err.Error())
			return
		}
		runtimeOK(w, status)
	})
}

// registerPHPContainerRoutes 注册 PHP 容器配置读取与更新接口。
func registerPHPContainerRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("GET /api/v2/runtimes/php/container/{id}", func(w http.ResponseWriter, r *http.Request) {
		item, index := runtimeByID(s, r.PathValue("id"))
		if index < 0 || normalizeRuntimeTypeFilter(item.Type) != "php" {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		runtimeOK(w, map[string]any{"id": item.ID, "containerName": item.Container, "exposedPorts": item.ExposedPorts, "environments": item.Environments, "volumes": item.Volumes, "extraHosts": item.ExtraHosts})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/container/update", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		id := runtimeString(body, "id", "runtimeId", "ID")
		unlock := s.operations.lock(id)
		defer unlock()
		current, index := runtimeByID(s, id)
		if index < 0 || normalizeRuntimeTypeFilter(current.Type) != "php" {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		if name := runtimeString(body, "containerName", "container"); name != "" {
			if current.Params == nil {
				current.Params = map[string]any{}
			}
			body["params"] = cloneRuntimeMap(current.Params)
			body["params"].(map[string]any)["CONTAINER_NAME"] = name
		}
		updated, err := mergeRuntimeUpdate(current, body)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		s.mu.RLock()
		_, currentIndex := runtimeByIDLocked(s.state.Runtimes, current.ID)
		if currentIndex < 0 {
			s.mu.RUnlock()
			runtimeErr(w, http.StatusConflict, "运行时已被删除")
			return
		}
		others := append([]runtimeRecord(nil), s.state.Runtimes[:currentIndex]...)
		others = append(others, s.state.Runtimes[currentIndex+1:]...)
		s.mu.RUnlock()
		if err := validateRuntimeCreateLocked(others, updated); err != nil {
			runtimeErr(w, http.StatusConflict, err.Error())
			return
		}
		if err := applyRuntimeConfiguration(s.commandExecutor(), current, updated); err != nil {
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		updated.Status, updated.Message, updated.Error, updated.UpdatedAt = "Running", "", "", time.Now().UTC()
		s.mu.Lock()
		_, index = runtimeByIDLocked(s.state.Runtimes, updated.ID)
		if index < 0 {
			s.mu.Unlock()
			runtimeErr(w, http.StatusConflict, "运行时已被删除")
			return
		}
		s.state.Runtimes[index] = updated
		saveErr := s.saveLocked()
		s.mu.Unlock()
		if saveErr != nil {
			runtimeErr(w, http.StatusServiceUnavailable, saveErr.Error())
			return
		}
		runtimeSuccess(w)
	})
}
