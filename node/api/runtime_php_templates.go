// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// phpExtensionTemplatesLocked 读取并转换持久化扩展模板；调用方必须持有状态锁。
func phpExtensionTemplatesLocked(s *runtimeStore) ([]phpExtensionTemplate, error) {
	raw, exists := s.state.Settings[phpExtensionTemplatesSetting]
	if !exists {
		return append([]phpExtensionTemplate(nil), defaultPHPExtensionTemplates...), nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var templates []phpExtensionTemplate
	if err := json.Unmarshal(payload, &templates); err != nil {
		return nil, fmt.Errorf("读取 PHP 扩展模板失败: %w", err)
	}
	if templates == nil {
		templates = []phpExtensionTemplate{}
	}
	return templates, nil
}

// registerPHPExtensionTemplateRoutes 注册运行时相关 HTTP 路由，并保持请求响应契约。
func registerPHPExtensionTemplateRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions", phpExtensionTemplateCreateHandler(s))
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/update", phpExtensionTemplateUpdateHandler(s))
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/del", phpExtensionTemplateDeleteHandler(s))
}

// phpExtensionTemplateCreateHandler 创建并持久化 PHP 扩展模板。
func phpExtensionTemplateCreateHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		name := strings.TrimSpace(runtimeString(body, "name"))
		extensions, err := normalizePHPExtensions(body["extensions"])
		if name == "" || len(name) > 64 || strings.ContainsAny(name, "\r\n\x00") {
			runtimeErr(w, http.StatusBadRequest, "扩展模板名称无效")
			return
		}
		if err != nil || extensions == "" {
			if err == nil {
				err = errors.New("扩展模板不能为空")
			}
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		templates, err := phpExtensionTemplatesLocked(s)
		if err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		nextID := 1
		for _, item := range templates {
			if strings.EqualFold(item.Name, name) {
				runtimeErr(w, http.StatusConflict, "扩展模板名称已存在")
				return
			}
			if item.ID >= nextID {
				nextID = item.ID + 1
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		templates = append(templates, phpExtensionTemplate{ID: nextID, Name: name, Extensions: extensions, CreatedAt: now, UpdatedAt: now})
		s.state.Settings[phpExtensionTemplatesSetting] = templates
		if err := s.saveLocked(); err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		runtimeOK(w, templates[len(templates)-1])
	}
}

// phpExtensionTemplateUpdateHandler 更新已有 PHP 扩展模板。
func phpExtensionTemplateUpdateHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		id, idErr := strconv.Atoi(runtimeString(body, "id"))
		extensions, extensionErr := normalizePHPExtensions(body["extensions"])
		if idErr != nil || id < 1 || extensionErr != nil || extensions == "" {
			if extensionErr != nil {
				runtimeErr(w, http.StatusBadRequest, extensionErr.Error())
			} else {
				runtimeErr(w, http.StatusBadRequest, "扩展模板参数无效")
			}
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		templates, err := phpExtensionTemplatesLocked(s)
		if err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		found := false
		for index := range templates {
			if templates[index].ID == id {
				templates[index].Extensions = extensions
				templates[index].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
				found = true
				break
			}
		}
		if !found {
			runtimeErr(w, http.StatusNotFound, "扩展模板不存在")
			return
		}
		s.state.Settings[phpExtensionTemplatesSetting] = templates
		if err := s.saveLocked(); err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		runtimeOK(w, nil)
	}
}

// phpExtensionTemplateDeleteHandler 删除 PHP 扩展模板。
func phpExtensionTemplateDeleteHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		id, idErr := strconv.Atoi(runtimeString(body, "id"))
		if idErr != nil || id < 1 {
			runtimeErr(w, http.StatusBadRequest, "扩展模板 ID 无效")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		templates, err := phpExtensionTemplatesLocked(s)
		if err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		kept := make([]phpExtensionTemplate, 0, len(templates))
		found := false
		for _, item := range templates {
			if item.ID == id {
				found = true
				continue
			}
			kept = append(kept, item)
		}
		if !found {
			runtimeErr(w, http.StatusNotFound, "扩展模板不存在")
			return
		}
		s.state.Settings[phpExtensionTemplatesSetting] = kept
		if err := s.saveLocked(); err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		runtimeOK(w, nil)
	}
}

// registerPHPExtensionOperationRoutes 注册运行时相关 HTTP 路由，并保持请求响应契约。
func registerPHPExtensionOperationRoutes(mux *http.ServeMux, s *runtimeStore) {
	for _, route := range []struct {
		path    string
		install bool
	}{
		{path: "/api/v2/runtimes/php/extensions/install", install: true},
		{path: "/api/v2/runtimes/php/extensions/uninstall", install: false},
	} {
		route := route
		mux.HandleFunc("POST "+route.path, phpExtensionOperationHandler(s, route.install))
	}
}

// phpExtensionOperationHandler 返回 PHP 扩展安装或卸载处理器。
func phpExtensionOperationHandler(s *runtimeStore, install bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		id, extension := runtimeString(body, "id", "runtimeId"), strings.ToLower(runtimeString(body, "name", "extension"))
		if !phpExtensionNamePattern.MatchString(extension) {
			runtimeErr(w, http.StatusBadRequest, "PHP 扩展名称无效")
			return
		}
		item, index := runtimeByID(s, id)
		if index < 0 || normalizeRuntimeTypeFilter(item.Type) != "php" {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		definition := phpExtensionDefinitionForName(extension)
		extensions := updatedPHPExtensionNames(item.Extensions, definition, install)
		updated := item
		updated.Extensions = extensions
		updated.Params = cloneRuntimeMap(item.Params)
		updated.Params["PHP_EXTENSIONS"] = strings.Join(extensions, ",")
		taskID := runtimeString(body, "taskID", "taskId")
		if taskID == "" {
			taskID = idToken()
		}
		startMessage := "开始更新 PHP 扩展 " + extension
		ensureAppTaskLog(taskID, item.ID, item.Name, "installing", startMessage)
		run := func() error {
			if install {
				if err := installPHPExtensionWithOutput(s.commandExecutor(), item, definition.Name, func(stream string, data []byte) {
					appendRuntimeTaskOutput(taskID, stream, data)
				}); err != nil {
					return err
				}
				values, err := runtimeEnvironment(updated)
				if err != nil {
					return err
				}
				if strings.TrimSpace(updated.InstallPath) == "" {
					return errors.New("PHP 运行时安装目录不存在")
				}
				if err := writeRuntimeEnv(filepath.Join(updated.InstallPath, ".env"), values); err != nil {
					return fmt.Errorf("保存 PHP 扩展环境变量失败: %w", err)
				}
				return nil
			}
			return uninstallPHPExtension(s.commandExecutor(), item, updated, definition)
		}
		finish := func() error {
			s.mu.Lock()
			defer s.mu.Unlock()
			updated.Status, updated.Message, updated.Error, updated.TaskStatus, updated.UpdatedAt = "Running", "", "", "success", time.Now().UTC()
			for runtimeIndex := range s.state.Runtimes {
				if s.state.Runtimes[runtimeIndex].ID == updated.ID {
					s.state.Runtimes[runtimeIndex] = updated
					return s.saveLocked()
				}
			}
			return errors.New("PHP 运行时已被删除")
		}
		if !install {
			if err := run(); err != nil {
				runtimeErr(w, http.StatusBadGateway, err.Error())
				return
			}
			if err := finish(); err != nil {
				runtimeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			runtimeOK(w, map[string]any{"id": id, "extension": extension, "status": "success"})
			return
		}
		runPHPExtensionAsync(s, item, taskID, startMessage, run, finish)
		wmhttp.JSON(w, http.StatusAccepted, map[string]any{"code": 200, "data": map[string]any{"id": id, "extension": extension, "taskID": taskID, "status": "queued"}})
	}
}

// runPHPExtensionAsync 在后台执行安装任务，并统一记录成功、失败和结束标记。
func runPHPExtensionAsync(s *runtimeStore, item runtimeRecord, taskID, startMessage string, run func() error, finish func() error) {
	go func() {
		appendRuntimeTaskLog(taskID, startMessage)
		if err := run(); err != nil {
			updateRuntimeTask(s, item.ID, "failed", err.Error())
			ensureAppTaskLog(taskID, item.ID, item.Name, "failed", err.Error())
			appendRuntimeTaskLog(taskID, err.Error())
			appendRuntimeTaskLog(taskID, "[TASK-END]")
			return
		}
		if err := finish(); err != nil {
			updateRuntimeTask(s, item.ID, "failed", err.Error())
			ensureAppTaskLog(taskID, item.ID, item.Name, "failed", err.Error())
			appendRuntimeTaskLog(taskID, err.Error())
			appendRuntimeTaskLog(taskID, "[TASK-END]")
			return
		}
		ensureAppTaskLog(taskID, item.ID, item.Name, "running", "PHP 扩展更新完成")
		appendRuntimeTaskLog(taskID, "PHP 扩展更新完成")
		appendRuntimeTaskLog(taskID, "[TASK-END]")
	}()
}
