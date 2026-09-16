// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// registerCronRoutes 注册计划任务的 CRUD、执行、记录和导入导出接口。
// 路由保持与 1Panel 前端 v2 契约一致，具体执行委托给共享 CronjobService。
func registerCronRoutes(mux *http.ServeMux, cronjobs *service.CronjobService) {
	registerCronJobCRUDRoutes(mux, cronjobs)
	registerCronJobRecordRoutes(mux, cronjobs)
	registerCronJobUtilityRoutes(mux, cronjobs)
}

// registerCronJobCRUDRoutes 注册计划任务的写入和状态管理接口。
func registerCronJobCRUDRoutes(mux *http.ServeMux, cronjobs *service.CronjobService) {
	registerCronJobCreateListRoutes(mux, cronjobs)
	registerCronJobUpdateStatusRoutes(mux, cronjobs)
	registerCronJobGroupDeleteRoutes(mux, cronjobs)
}

// registerCronJobCreateListRoutes 注册计划任务创建、列表和分页查询接口。
func registerCronJobCreateListRoutes(mux *http.ServeMux, cronjobs *service.CronjobService) {
	mux.HandleFunc("POST /api/v2/cronjobs", func(w http.ResponseWriter, r *http.Request) {
		var job model.Cronjob
		if err := decodeJSON(r, &job); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		created, err := cronjobs.Create(r.Context(), job)
		if err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": created})
	})
	mux.HandleFunc("GET /api/v2/cronjobs", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": cronjobs.List(r.Context())})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/search", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Page     int `json:"page"`
			PageSize int `json:"pageSize"`
		}
		_ = decodeJSON(r, &in)
		total, items := cronjobs.ListPage(r.Context(), in.Page, in.PageSize)
		page, pageSize := normalizeCronPage(in.Page, in.PageSize)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize}})
	})

}

// registerCronJobUpdateStatusRoutes 注册计划任务更新和启停接口。
func registerCronJobUpdateStatusRoutes(mux *http.ServeMux, cronjobs *service.CronjobService) {
	mux.HandleFunc("POST /api/v2/cronjobs/update", func(w http.ResponseWriter, r *http.Request) {
		var job model.Cronjob
		if err := decodeJSON(r, &job); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		updated, err := cronjobs.Update(r.Context(), job)
		if err != nil {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": updated})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/status", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Enable bool   `json:"enable"`
		}
		if err := decodeJSON(r, &in); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		status := in.Status
		if status == "" {
			status = "disabled"
			if in.Enable {
				status = "enabled"
			}
		}
		if err := cronjobs.SetStatus(r.Context(), in.ID, status); err != nil {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
	})

}

// registerCronJobGroupDeleteRoutes 注册计划任务分组更新和删除接口。
func registerCronJobGroupDeleteRoutes(mux *http.ServeMux, cronjobs *service.CronjobService) {
	mux.HandleFunc("POST /api/v2/cronjobs/group/update", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID      string `json:"id"`
			GroupID uint   `json:"groupID"`
		}
		if err := decodeJSON(r, &in); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		job, ok := cronjobs.Get(r.Context(), in.ID)
		if !ok {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": "cronjob not found"})
			return
		}
		job.GroupID = in.GroupID
		updated, err := cronjobs.Update(r.Context(), job)
		if err != nil {
			wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": updated})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/del", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID string `json:"id"`
		}
		if err := decodeJSON(r, &request); err != nil || request.ID == "" {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "计划任务 ID 无效"})
			return
		}
		if err := cronjobs.Delete(r.Context(), request.ID); err != nil {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
	})

}

// registerCronJobRecordRoutes 注册计划任务详情和执行记录查询、清理接口。
func registerCronJobRecordRoutes(mux *http.ServeMux, cronjobs *service.CronjobService) {
	mux.HandleFunc("POST /api/v2/cronjobs/load/info", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		if err := decodeJSON(r, &in); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		job, ok := cronjobs.Get(r.Context(), in.ID)
		if !ok {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": "cronjob not found"})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": job})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/search/records", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID        string `json:"id"`
			CronjobID string `json:"cronjobID"`
			Page      int    `json:"page"`
			PageSize  int    `json:"pageSize"`
		}
		_ = decodeJSON(r, &in)
		id := in.ID
		if id == "" {
			id = in.CronjobID
		}
		total, records := cronjobs.RecordsPage(r.Context(), id, in.Page, in.PageSize)
		page, pageSize := normalizeCronPage(in.Page, in.PageSize)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": records, "total": total, "page": page, "pageSize": pageSize}})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/records/log", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		_ = decodeJSON(r, &in)
		records := cronjobs.Records(r.Context(), in.ID)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": records})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/records/clean", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID        string `json:"id"`
			CronjobID string `json:"cronjobID"`
		}
		_ = decodeJSON(r, &in)
		id := in.ID
		if id == "" {
			id = in.CronjobID
		}
		if err := cronjobs.CleanRecords(r.Context(), id); err != nil {
			wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
	})
}

// registerCronJobUtilityRoutes 注册计划任务执行、导入导出、cron 预览和脚本选项接口。
func registerCronJobUtilityRoutes(mux *http.ServeMux, cronjobs *service.CronjobService) {
	mux.HandleFunc("POST /api/v2/cronjobs/export", func(w http.ResponseWriter, r *http.Request) {
		b, err := cronjobs.Export(r.Context())
		if err != nil {
			wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	})
	mux.HandleFunc("POST /api/v2/cronjobs/import", func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		if err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		if err := cronjobs.Import(r.Context(), b); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
	})
	mux.HandleFunc("GET /api/v2/cronjobs/script/options", func(w http.ResponseWriter, _ *http.Request) {
		options := []map[string]string{{"value": "shell", "label": "Shell"}, {"value": "python", "label": "Python"}}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": options})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/next", handleCronjobNext)
	mux.HandleFunc("POST /api/v2/cronjobs/stop", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		_ = decodeJSON(r, &in)
		if in.ID == "" {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "id is required"})
			return
		}
		if err := cronjobs.Stop(in.ID); err != nil {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/handle", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID string `json:"id"`
		}
		if err := decodeJSON(r, &request); err != nil || request.ID == "" {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "计划任务 ID 无效"})
			return
		}
		result, err := cronjobs.HandleOnce(r.Context(), request.ID)
		writeCommandResult(w, result, err)
	})
}

// handleCronjobNext 将 cron 表达式解析为前端可直接展示的未来执行时间。
func handleCronjobNext(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Spec string `json:"spec"`
	}
	_ = decodeJSON(r, &in)
	if strings.TrimSpace(in.Spec) == "" {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "spec is required"})
		return
	}
	next, err := service.NextRuns(in.Spec, time.Now().UTC(), 5)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "无效的 cron 表达式"})
		return
	}
	formatted := make([]string, len(next))
	for i, runAt := range next {
		formatted[i] = runAt.Format(time.RFC3339)
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": formatted})
}

// normalizeCronPage 统一分页边界，保持服务层查询和响应中的页码一致。
func normalizeCronPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	return page, pageSize
}
