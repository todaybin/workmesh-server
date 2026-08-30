// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var sharedCronjobs = service.NewCronjobService()

// StartBackgroundTasks 启动节点级后台调度任务，调用方应在进程退出时取消 ctx。
func StartBackgroundTasks(ctx context.Context) {
	sharedCronjobs.Start(ctx)
}

// RegisterHostContainerCronRoutes 注册主机、容器和计划任务接口。
func RegisterHostContainerCronRoutes(mux *http.ServeMux) {
	commands := service.CommandService{}
	docker := service.NewDockerService()
	cronjobs := sharedCronjobs

	mux.HandleFunc("POST /api/v2/system/command", func(w http.ResponseWriter, r *http.Request) {
		if token := os.Getenv("WORKMESH_COMMAND_TOKEN"); token == "" || r.Header.Get("X-WorkMesh-Token") != token {
			wmhttp.JSON(w, http.StatusUnauthorized, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "COMMAND_AUTH_REQUIRED"}})
			return
		}
		var request model.CommandRequest
		if err := decodeJSON(r, &request); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		result, err := commands.Execute(r.Context(), request)
		if err != nil {
			wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error(), "data": result})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
	})

	mux.HandleFunc("GET /api/v2/hosts/components/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.PathValue("name"))
		if name == "" {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "组件名称不能为空"})
			return
		}
		_, err := exec.LookPath(name)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"name": name, "exists": err == nil}})
	})
	mux.HandleFunc("GET /api/v2/hosts/system/info", func(w http.ResponseWriter, r *http.Request) {
		host, _ := os.Hostname()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"hostname": host, "os": runtime.GOOS, "arch": runtime.GOARCH, "cpus": runtime.NumCPU()}})
	})

	mux.HandleFunc("GET /api/v2/containers/docker/status", func(w http.ResponseWriter, r *http.Request) {
		status, err := docker.StatusInfo(r.Context())
		if err != nil {
			wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": status})
	})
	mux.HandleFunc("GET /api/v2/containers/list", func(w http.ResponseWriter, r *http.Request) {
		result, err := docker.List(r.Context())
		writeCommandResult(w, result, err)
	})
	mux.HandleFunc("POST /api/v2/containers/operate", func(w http.ResponseWriter, r *http.Request) {
		var request model.DockerOperationRequest
		if err := decodeJSON(r, &request); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		result, err := docker.Operate(r.Context(), request)
		writeCommandResult(w, result, err)
	})

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
		if in.Page < 1 {
			in.Page = 1
		}
		if in.PageSize < 1 || in.PageSize > 200 {
			in.PageSize = 20
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": total, "page": in.Page, "pageSize": in.PageSize}})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/update", func(w http.ResponseWriter, r *http.Request) {
		var job model.Cronjob
		if err := decodeJSON(r, &job); err != nil {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		updated, err := cronjobs.Update(r.Context(), job)
		if err != nil {
			wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": updated})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/status", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Enable bool   `json:"enable"`
		}
		if err := decodeJSON(r, &in); err != nil {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		status := in.Status
		if status == "" {
			if in.Enable {
				status = "enabled"
			} else {
				status = "disabled"
			}
		}
		if err := cronjobs.SetStatus(r.Context(), in.ID, status); err != nil {
			wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/load/info", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		if err := decodeJSON(r, &in); err != nil {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		job, ok := cronjobs.Get(r.Context(), in.ID)
		if !ok {
			wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "cronjob not found"})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": job})
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
		if in.Page < 1 {
			in.Page = 1
		}
		if in.PageSize < 1 || in.PageSize > 200 {
			in.PageSize = 20
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": records, "total": total, "page": in.Page, "pageSize": in.PageSize}})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/records/log", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		_ = decodeJSON(r, &in)
		records := cronjobs.Records(r.Context(), in.ID)
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": records})
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
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/export", func(w http.ResponseWriter, r *http.Request) {
		b, err := cronjobs.Export(r.Context())
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(b)
	})
	mux.HandleFunc("POST /api/v2/cronjobs/import", func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		if err != nil {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		if err := cronjobs.Import(r.Context(), b); err != nil {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200})
	})
	mux.HandleFunc("GET /api/v2/cronjobs/script/options", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": []map[string]string{{"value": "shell", "label": "Shell"}, {"value": "python", "label": "Python"}}})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/next", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Spec string `json:"spec"`
		}
		_ = decodeJSON(r, &in)
		if strings.TrimSpace(in.Spec) == "" {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "spec is required"})
			return
		}
		next, err := service.NextRuns(in.Spec, time.Now().UTC(), 5)
		if err != nil {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "无效的 cron 表达式"})
			return
		}
		formatted := make([]string, len(next))
		for i, t := range next {
			formatted[i] = t.Format(time.RFC3339)
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": formatted})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/stop", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		_ = decodeJSON(r, &in)
		if in.ID == "" {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "id is required"})
			return
		}
		if err := cronjobs.Stop(in.ID); err != nil {
			wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200})
	})
	mux.HandleFunc("POST /api/v2/cronjobs/group/update", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID      string `json:"id"`
			GroupID uint   `json:"groupID"`
		}
		if err := decodeJSON(r, &in); err != nil {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		job, ok := cronjobs.Get(r.Context(), in.ID)
		if !ok {
			wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "cronjob not found"})
			return
		}
		job.GroupID = in.GroupID
		updated, err := cronjobs.Update(r.Context(), job)
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": updated})
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
	// Go 1.22 路径参数使用大括号；同时保留旧的冒号字面量契约以兼容历史客户端。
	mux.HandleFunc("GET /api/v2/dashboard/base/{ioOption}/{netOption}", handleDashboardBase)
	mux.HandleFunc("GET /api/v2/dashboard/current/{ioOption}/{netOption}", handleDashboardCurrent)
	mux.HandleFunc("POST /api/v2/dashboard/system/restart/{operation}", handleDashboardRestart)
	mux.HandleFunc("POST /api/v2/files", handleFilesCreate)
	/*
		// 尚未接入专用处理器的网站子路径统一走兼容入口；更具体路由会优先匹配。
		mux.HandleFunc("/api/v2/websites/{rest...}", websiteFallbackHandler)
		// 网站接口由 website.go 中的专用处理器注册；不再注册无界总兜底。
		// 网站接口由 website.go 中的专用处理器注册；不再注册无界总兜底。
	*/
	registerContainerRoutes(mux)
	registerHostRoutes(mux)
	registerAIExecutionRoutes(mux)
	registerCoreResourceRoutes(mux)
	registerCoreCommandRoutes(mux)
	registerFileRoutes(mux)
	RegisterDatabaseAdminRoutes(mux)
	registerDatabaseRoutes(mux)
	registerDeploymentAndProcessRoutes(mux)

	registerBackupAlertLogSettingsRoutes(mux)
	registerWebsiteFunctionalRoutes(mux)
	RegisterLegacyCompatibilityRoutes(mux)
}

func registerUnmigratedRoutes(mux *http.ServeMux) {
	paths := []string{
		"/api/v2/hosts", "/api/v2/hosts/test/byinfo", "/api/v2/hosts/test/byid", "/api/v2/hosts/tree", "/api/v2/hosts/search", "/api/v2/hosts/del", "/api/v2/hosts/update", "/api/v2/hosts/update/group", "/api/v2/hosts/info",
		"/api/v2/containers/search", "/api/v2/containers/users", "/api/v2/containers/files/search", "/api/v2/containers/files/upload", "/api/v2/containers/files/content", "/api/v2/containers/files/size", "/api/v2/containers/files/del", "/api/v2/containers/files/download", "/api/v2/containers/list", "/api/v2/containers/list/byimage", "/api/v2/containers/status", "/api/v2/containers/compose/search", "/api/v2/containers/compose/test", "/api/v2/containers/compose", "/api/v2/containers/compose/operate", "/api/v2/containers/update", "/api/v2/containers/info", "/api/v2/containers/limit", "/api/v2/containers/list/stats", "/api/v2/containers/item/stats", "/api/v2/containers", "/api/v2/containers/upgrade", "/api/v2/containers/prune", "/api/v2/containers/clean/log", "/api/v2/containers/compose/clean/log", "/api/v2/containers/rename", "/api/v2/containers/commit", "/api/v2/containers/operate", "/api/v2/containers/inspect", "/api/v2/containers/download/log", "/api/v2/containers/network/search", "/api/v2/containers/network", "/api/v2/containers/network/del", "/api/v2/containers/volume/search", "/api/v2/containers/volume", "/api/v2/containers/volume/del", "/api/v2/containers/compose/update", "/api/v2/containers/compose/pin", "/api/v2/containers/compose/env", "/api/v2/containers/search/log", "/api/v2/containers/daemonjson/file", "/api/v2/containers/daemonjson", "/api/v2/containers/daemonjson/update", "/api/v2/containers/logoption/update", "/api/v2/containers/ipv6option/update", "/api/v2/containers/daemonjson/update/byfile", "/api/v2/containers/docker/operate",
		"/api/v2/cronjobs/load/info", "/api/v2/cronjobs/export", "/api/v2/cronjobs/import", "/api/v2/cronjobs/script/options", "/api/v2/cronjobs/next", "/api/v2/cronjobs/search/records", "/api/v2/cronjobs/records/log", "/api/v2/cronjobs/records/clean", "/api/v2/cronjobs/stop", "/api/v2/cronjobs/update", "/api/v2/cronjobs/group/update", "/api/v2/cronjobs/status",
	}
	for _, path := range paths {
		for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
			if (path == "/api/v2/containers/list" && method == "GET") ||
				(path == "/api/v2/containers/operate" && method == "POST") ||
				(path == "/api/v2/cronjobs" && method == "POST") ||
				(path == "/api/v2/cronjobs/del" && method == "POST") {
				continue
			}
			pattern := method + " " + path
			mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
				wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "MIGRATION_PENDING"}, "message": "该接口正在迁移"})
			})
		}
	}
	mux.HandleFunc("GET /api/v2/containers/stats/{id}", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusNotImplemented, map[string]any{"code": "ERR", "message": "该接口正在迁移"})
	})
}

func decodeJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return errors.New("请求体不能为空")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	return decoder.Decode(target)
}

func writeCommandResult(w http.ResponseWriter, result model.CommandResult, err error) {
	if err != nil {
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error(), "data": result})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
}
