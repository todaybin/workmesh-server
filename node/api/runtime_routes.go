// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"sort"
	"strings"
	"time"
)

// registerRuntimeRoutes 注册运行时生命周期接口，并保持 1Panel 请求响应契约。
func registerRuntimeRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("GET /api/v2/runtimes/{id}", runtimeDetailHandler(s))
	mux.HandleFunc("GET /api/v2/runtimes/installed/delete/check/{id}", runtimeDeleteCheckHandler())
	mux.HandleFunc("POST /api/v2/runtimes/search", runtimeListHandler(s))
	mux.HandleFunc("POST /api/v2/runtimes/sync", runtimeSyncHandler(s))
	mux.HandleFunc("POST /api/v2/runtimes", runtimeCreateHandler(s))
	mux.HandleFunc("POST /api/v2/runtimes/operate", runtimeOperateHandler(s))
	mux.HandleFunc("POST /api/v2/runtimes/remark", runtimeRemarkHandler(s))
	mux.HandleFunc("POST /api/v2/runtimes/update", runtimeUpdateHandler(s))
	mux.HandleFunc("POST /api/v2/runtimes/del", runtimeDeleteHandler(s))
	registerRuntimeSubroutes(mux, s)
}

// runtimeListHandler 返回运行时列表查询处理器，并在内存快照上执行兼容分页。
func runtimeListHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		page, pageSize, err := runtimePage(body)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		typeFilter := strings.ToLower(runtimeString(body, "type"))
		nameFilter := strings.ToLower(runtimeString(body, "name"))
		statusFilter := strings.ToLower(runtimeString(body, "status"))
		s.mu.RLock()
		filtered := make([]runtimeRecord, 0, len(s.state.Runtimes))
		for _, item := range s.state.Runtimes {
			hydrateRuntimePaths(&item)
			if typeFilter != "" && typeFilter != "all" && !strings.EqualFold(item.Type, typeFilter) {
				continue
			}
			if nameFilter != "" && !strings.Contains(strings.ToLower(item.Name), nameFilter) {
				continue
			}
			if statusFilter != "" && statusFilter != "all" && !runtimeStatusMatches(item, statusFilter) {
				continue
			}
			filtered = append(filtered, item)
		}
		s.mu.RUnlock()
		sort.SliceStable(filtered, func(i, j int) bool {
			if filtered[i].Name == filtered[j].Name {
				return filtered[i].ID < filtered[j].ID
			}
			return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
		})
		total := len(filtered)
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		items := filtered[start:end]
		if items == nil {
			items = []runtimeRecord{}
		}
		runtimeOK(w, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
	}
}

// runtimeDetailHandler 返回运行时详情查询处理器。
func runtimeDetailHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, v := range s.state.Runtimes {
			hydrateRuntimePaths(&v)
			if v.ID == id {
				runtimeOK(w, v)
				return
			}
		}
		runtimeErr(w, 404, "运行时不存在")
	}
}

// runtimeDeleteCheckHandler 返回运行时删除引用检查处理器。
func runtimeDeleteCheckHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resources, err := runtimeWebsiteReferences(r.Context(), r.PathValue("id"))
		if err != nil {
			runtimeErr(w, http.StatusInternalServerError, "检查运行时引用失败: "+err.Error())
			return
		}
		runtimeOK(w, resources)
	}
}

// runtimeSyncHandler 返回真实容器状态同步处理器。
func runtimeSyncHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := runtimeBody(r); err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := syncRuntimeContainerStatus(s); err != nil {
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		runtimeSuccess(w)
	}
}

// runtimeCreateHandler 返回运行时创建处理器。
func runtimeCreateHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, e := runtimeBody(r)
		if e != nil {
			runtimeErr(w, 400, e.Error())
			return
		}
		item, err := runtimeRecordFromRequest(v)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := hydrateRuntimeFromAppStore(&item); err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if item.ID == "" {
			item.ID = item.Name
		}
		if item.ID == "" {
			runtimeErr(w, 400, "运行时名称不能为空")
			return
		}
		unlock := s.operations.lock(item.ID)
		defer unlock()
		s.mu.Lock()
		if validationErr := validateRuntimeCreateLocked(s.state.Runtimes, item); validationErr != nil {
			s.mu.Unlock()
			runtimeErr(w, http.StatusConflict, validationErr.Error())
			return
		}
		install := runtimeInstallRequested(v, item)
		if install && normalizeRuntimeTypeFilter(item.Type) != "php" {
			if err := validateRuntimeCodeDirectory(item); err != nil {
				s.mu.Unlock()
				runtimeErr(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		if install {
			if strings.TrimSpace(item.TaskID) == "" {
				item.TaskID = idToken()
			}
			item.Status = "Creating"
			item.TaskStatus = "installing"
		} else {
			item.Status = "Running"
		}
		s.state.Runtimes = append(s.state.Runtimes, item)
		saveErr := s.saveLocked()
		s.mu.Unlock()
		if saveErr != nil {
			runtimeErr(w, http.StatusServiceUnavailable, "运行时存储不可用: "+saveErr.Error())
			return
		}
		if install {
			// Create the task record and first log line before returning. This
			// lets the task drawer attach immediately and keeps installation
			// independent from the lifecycle of the creating page.
			if err := persistRuntimeTaskChecked(item); err != nil {
				s.mu.Lock()
				s.state.Runtimes = removeRuntimeByID(s.state.Runtimes, item.ID)
				_ = s.saveLocked()
				s.mu.Unlock()
				runtimeErr(w, http.StatusServiceUnavailable, "运行时任务存储不可用: "+err.Error())
				return
			}
			if err := ensureAppTaskLogChecked(item.TaskID, item.ID, item.Name, "installing", "开始安装运行时"); err != nil {
				s.mu.Lock()
				s.state.Runtimes = removeRuntimeByID(s.state.Runtimes, item.ID)
				_ = s.saveLocked()
				s.mu.Unlock()
				runtimeErr(w, http.StatusServiceUnavailable, "运行时任务日志存储不可用: "+err.Error())
				return
			}
			go runRuntimeInstallTask(s, item)
		}
		if hostRuntime(item) {
			if err := applyHostRuntime(s.commandExecutor(), item, false); err != nil {
				s.mu.Lock()
				s.state.Runtimes = removeRuntimeByID(s.state.Runtimes, item.ID)
				_ = s.saveLocked()
				s.mu.Unlock()
				runtimeErr(w, http.StatusBadGateway, err.Error())
				return
			}
		}
		runtimeOK(w, item)
	}
}

// runtimeOperateHandler 返回运行时容器启停操作处理器。
func runtimeOperateHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		id := runtimeString(body, "ID", "id", "runtimeId")
		unlock := s.operations.lock(id)
		defer unlock()
		item, index := runtimeByID(s, id)
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "运行时不存在")
			return
		}
		operation := runtimeString(body, "operate", "operation")
		if !validRuntimeOperation(operation) {
			runtimeErr(w, http.StatusBadRequest, "运行时操作必须为 up、down 或 restart")
			return
		}
		if err := operateRuntimeContainer(s.commandExecutor(), item, operation); err != nil {
			s.mu.Lock()
			if _, currentIndex := runtimeByIDLocked(s.state.Runtimes, item.ID); currentIndex >= 0 {
				s.state.Runtimes[currentIndex].Status = "Error"
				s.state.Runtimes[currentIndex].TaskStatus = "failed"
				s.state.Runtimes[currentIndex].Message = err.Error()
				s.state.Runtimes[currentIndex].Error = err.Error()
				s.state.Runtimes[currentIndex].UpdatedAt = time.Now().UTC()
				_ = s.saveLocked()
			}
			s.mu.Unlock()
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		s.mu.Lock()
		_, index = runtimeByIDLocked(s.state.Runtimes, item.ID)
		if index < 0 {
			s.mu.Unlock()
			runtimeErr(w, http.StatusConflict, "运行时已被删除")
			return
		}
		item.Status = runtimeStatusForOperation(operation)
		item.Message, item.Error, item.TaskStatus = "", "", ""
		item.UpdatedAt = time.Now().UTC()
		s.state.Runtimes[index] = item
		saveErr := s.saveLocked()
		s.mu.Unlock()
		if saveErr != nil {
			runtimeErr(w, http.StatusServiceUnavailable, "运行时存储不可用: "+saveErr.Error())
			return
		}
		runtimeSuccess(w)
	}
}

// runtimeRemarkHandler 返回运行时备注更新处理器。
func runtimeRemarkHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		id := runtimeString(body, "id", "runtimeId", "ID")
		unlock := s.operations.lock(id)
		defer unlock()
		item, index := runtimeByID(s, id)
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "运行时不存在")
			return
		}
		item.Remark = runtimeString(body, "remark")
		item.UpdatedAt = time.Now().UTC()
		s.mu.Lock()
		_, index = runtimeByIDLocked(s.state.Runtimes, item.ID)
		if index < 0 {
			s.mu.Unlock()
			runtimeErr(w, http.StatusConflict, "运行时已被删除")
			return
		}
		s.state.Runtimes[index] = item
		saveErr := s.saveLocked()
		s.mu.Unlock()
		if saveErr != nil {
			runtimeErr(w, http.StatusServiceUnavailable, "运行时存储不可用: "+saveErr.Error())
			return
		}
		runtimeSuccess(w)
	}
}

// runtimeUpdateHandler 返回运行时配置更新处理器。
func runtimeUpdateHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		id := runtimeString(body, "id", "runtimeId", "ID")
		unlock := s.operations.lock(id)
		defer unlock()
		current, index := runtimeByID(s, id)
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "运行时不存在")
			return
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
		rollbackPackage, err := refreshRuntimePackageIfNeeded(current, &updated)
		if err != nil {
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		if err := applyRuntimeConfiguration(s.commandExecutor(), current, updated); err != nil {
			rollbackPackage()
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		if hostRuntime(updated) {
			if err := applyHostRuntime(s.commandExecutor(), updated, false); err != nil {
				runtimeErr(w, http.StatusBadGateway, err.Error())
				return
			}
		}
		updated.Status, updated.Message, updated.Error, updated.TaskStatus, updated.UpdatedAt = "Running", "", "", "", time.Now().UTC()
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
			runtimeErr(w, http.StatusServiceUnavailable, "运行时存储不可用: "+saveErr.Error())
			return
		}
		runtimeSuccess(w)
	}
}

// runtimeDeleteHandler 返回运行时删除及外部资源清理处理器。
func runtimeDeleteHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		id := runtimeString(v, "id", "runtimeId", "ID")
		if id == "" {
			runtimeErr(w, http.StatusBadRequest, "运行时 ID 不能为空")
			return
		}
		unlock := s.operations.lock(id)
		defer unlock()
		resources, referenceErr := runtimeWebsiteReferences(r.Context(), id)
		if referenceErr != nil {
			runtimeErr(w, http.StatusInternalServerError, "检查运行时引用失败: "+referenceErr.Error())
			return
		}
		if len(resources) > 0 {
			runtimeErrData(w, http.StatusConflict, "运行时仍被网站引用", resources)
			return
		}
		forceDelete, _ := v["forceDelete"].(bool)
		deleteImage, _ := v["deleteImage"].(bool)
		taskID := runtimeString(v, "taskID", "taskId")
		s.mu.Lock()
		item, index := runtimeByIDLocked(s.state.Runtimes, id)
		if index >= 0 {
			// Docker and filesystem cleanup must happen without the store lock. A
			// second delete request may remove the record while those operations
			// run, so the index is deliberately re-resolved before slicing below.
			s.mu.Unlock()
			if item.ComposePath != "" || item.Container != "" {
				var cleanupErr error
				if hostRuntime(item) {
					cleanupErr = applyHostRuntime(s.commandExecutor(), item, true)
				} else {
					cleanupErr = removeRuntimeContainer(s.commandExecutor(), item)
				}
				if cleanupErr != nil && !forceDelete {
					runtimeErr(w, http.StatusBadGateway, cleanupErr.Error())
					return
				}
				if deleteImage && strings.TrimSpace(item.Image) != "" {
					if _, err := runtimeCommand(s.commandExecutor(), item, 10*time.Minute, "image", "rm", item.Image); err != nil && !forceDelete {
						runtimeErr(w, http.StatusBadGateway, "删除运行时镜像失败: "+err.Error())
						return
					}
				}
				if item.InstallPath != "" {
					if err := removeRuntimeInstallPath(item.InstallPath); err != nil && !forceDelete {
						runtimeErr(w, http.StatusInternalServerError, "删除运行时目录失败: "+err.Error())
						return
					}
				}
			}
			s.mu.Lock()
			_, index = runtimeByIDLocked(s.state.Runtimes, id)
			if index < 0 {
				// Another request completed the deletion while cleanup was running.
				s.mu.Unlock()
				runtimeSuccess(w)
				return
			}
			s.state.Runtimes = append(s.state.Runtimes[:index], s.state.Runtimes[index+1:]...)
			saveErr := s.saveLocked()
			s.mu.Unlock()
			if saveErr != nil {
				runtimeErr(w, http.StatusServiceUnavailable, "运行时存储不可用: "+saveErr.Error())
				return
			}
			if taskID != "" {
				ensureAppTaskLog(taskID, item.ID, item.Name, "running", "运行时删除完成")
				appendRuntimeTaskLog(taskID, "运行时删除完成")
				appendRuntimeTaskLog(taskID, "[TASK-END]")
			}
			runtimeSuccess(w)
			return
		}
		s.mu.Unlock()
		runtimeErr(w, 404, "运行时不存在")
	}
}
