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
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var sharedCronjobs = service.NewCronjobService()
var (
	backgroundMu      sync.Mutex
	backgroundStarted bool
)

// StartBackgroundTasks 启动节点级后台调度任务，调用方应在进程退出时取消 ctx。
func StartBackgroundTasks(ctx context.Context) {
	sharedCronjobs.Start(ctx)
	backgroundMu.Lock()
	if backgroundStarted {
		backgroundMu.Unlock()
		return
	}
	backgroundStarted = true
	backgroundMu.Unlock()
	// 证书扫描使用小时级周期，避免每个请求触发外部或昂贵的续期逻辑。
	security := service.NewWebsiteSecurityService("")
	go func() {
		defer func() {
			backgroundMu.Lock()
			backgroundStarted = false
			backgroundMu.Unlock()
		}()
		const interval = time.Hour
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		security.RenewDueCertificates(ctx, 30*24*time.Hour)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				security.RenewDueCertificates(ctx, 30*24*time.Hour)
			}
		}
	}()
}

// RegisterHostContainerCronRoutes 注册主机、容器和计划任务接口。
func RegisterHostContainerCronRoutes(mux *http.ServeMux) {
	commands := service.CommandService{}
	docker := service.NewDockerService()
	cronjobs := sharedCronjobs
	// 仪表盘基础系统信息与主机资源采集同属节点运维路由组，单独注册该入口可供模块级挂载使用。
	mux.HandleFunc("GET /api/v2/dashboard/base/os", handleDashboardOS)

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
		if request.TaskID != "" {
			// v2 前端会携带任务标识；同步执行完成后回传同一标识，便于任务面板关联结果。
			if err != nil {
				wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error(), "taskID": request.TaskID, "data": result})
				return
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "taskID": request.TaskID, "data": result})
			return
		}
		writeCommandResult(w, result, err)
	})
	// 镜像导入必须使用 docker load 的文件参数，禁止通过 shell 拼接用户输入。
	mux.HandleFunc("POST /api/v2/containers/image/load", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Path string `json:"path"`
			File string `json:"file"`
		}
		if err := decodeJSON(r, &in); err != nil {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		p := strings.TrimSpace(in.Path)
		if p == "" {
			p = strings.TrimSpace(in.File)
		}
		if p == "" || !filepath.IsAbs(p) || strings.ContainsAny(p, "\x00\r\n") || strings.Contains(p, "..") {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "镜像归档路径无效"})
			return
		}
		result, err := runDocker(r, "load", "-i", p)
		writeCommandResult(w, result, err)
	})

	registerHostOperationalRoutes(mux)

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
	registerGroupRoutes(mux)
	registerCoreCommandRoutes(mux)
	registerFileRoutes(mux)
	RegisterDatabaseAdminRoutes(mux)
	registerDatabaseRoutes(mux)
	registerDeploymentAndProcessRoutes(mux)

	registerBackupAlertLogSettingsRoutes(mux)
	registerWebsiteFunctionalRoutes(mux)
	RegisterLegacyCompatibilityRoutes(mux)
}

// hostOperationalState 保存主机监控采样配置；仅保存用户设置，不缓存无限增长的采样数据。
type hostOperationalState struct {
	Monitor map[string]any `json:"monitor"`
}

var hostOperationalMu sync.Mutex

func hostOperationalStatePath() string {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	return filepath.Join(root, "host-operational.json")
}

func loadHostOperationalState() hostOperationalState {
	hostOperationalMu.Lock()
	defer hostOperationalMu.Unlock()
	return loadHostOperationalStateLocked()
}

func loadHostOperationalStateLocked() hostOperationalState {
	state := hostOperationalState{Monitor: map[string]any{"enabled": true, "interval": 10}}
	// SQLite 已初始化时，主机监控设置只从 node_settings 读取；旧 JSON 仅在首次启动时导入并归档。
	if sharedDB() != nil {
		if loadNodeSetting("host_operational", &state) {
			if state.Monitor == nil {
				state.Monitor = map[string]any{}
			}
			return state
		}
		legacyPath := hostOperationalStatePath()
		if b, err := os.ReadFile(legacyPath); err == nil && len(b) > 0 {
			var legacy hostOperationalState
			if json.Unmarshal(b, &legacy) == nil {
				if legacy.Monitor != nil {
					state = legacy
				}
				if saveErr := saveNodeSetting("host_operational", state); saveErr == nil {
					archiveDir := filepath.Join(filepath.Dir(legacyPath), "backups")
					if os.MkdirAll(archiveDir, 0o750) == nil {
						archivePath := filepath.Join(archiveDir, "legacy-host-operational-"+time.Now().UTC().Format("20060102T150405.000000000Z")+".json")
						_ = os.Rename(legacyPath, archivePath)
					}
				}
			}
		}
		if state.Monitor == nil {
			state.Monitor = map[string]any{}
		}
		return state
	}
	b, err := os.ReadFile(hostOperationalStatePath())
	if err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &state)
	}
	if state.Monitor == nil {
		state.Monitor = map[string]any{}
	}
	return state
}

func saveHostOperationalState(state hostOperationalState) error {
	hostOperationalMu.Lock()
	defer hostOperationalMu.Unlock()
	return saveHostOperationalStateLocked(state)
}

func saveHostOperationalStateLocked(state hostOperationalState) error {
	if sharedDB() != nil {
		if state.Monitor == nil {
			state.Monitor = map[string]any{}
		}
		return saveNodeSetting("host_operational", state)
	}
	if err := os.MkdirAll(filepath.Dir(hostOperationalStatePath()), 0o750); err != nil {
		return err
	}
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmp := hostOperationalStatePath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, hostOperationalStatePath())
}

// registerHostOperationalRoutes 提供旧主机面板使用的真实本机状态接口。
func registerHostOperationalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/hosts/monitor/netoptions", func(w http.ResponseWriter, _ *http.Request) {
		// 直接读取 /proc/net/dev，避免容器内缺少 netlink 权限时无法列出节点网卡。
		items := []string{"all"}
		seen := map[string]bool{"all": true}
		if data, err := os.ReadFile("/proc/net/dev"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) != 2 {
					continue
				}
				name := strings.TrimSpace(parts[0])
				if name != "" && !seen[name] {
					seen[name] = true
					items = append(items, name)
				}
			}
		}
		sort.Strings(items[1:])
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
	})
	mux.HandleFunc("GET /api/v2/hosts/monitor/iooptions", func(w http.ResponseWriter, _ *http.Request) {
		items := []string{"all"}
		if data, err := os.ReadFile("/proc/diskstats"); err == nil {
			seen := map[string]bool{"all": true}
			for _, line := range strings.Split(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) < 3 {
					continue
				}
				name := fields[2]
				if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || dashboardIsPartition(name) || seen[name] {
					continue
				}
				seen[name] = true
				items = append(items, name)
			}
			sort.Strings(items[1:])
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
	})
	mux.HandleFunc("GET /api/v2/hosts/monitor/setting", func(w http.ResponseWriter, _ *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": loadHostOperationalState().Monitor})
	})
	mux.HandleFunc("POST /api/v2/hosts/monitor/setting/update", func(w http.ResponseWriter, r *http.Request) {
		var monitor map[string]any
		if err := decodeJSON(r, &monitor); err != nil {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		if v, ok := monitor["interval"].(float64); ok && (v < 1 || v > 3600) {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "监控间隔必须在 1-3600 秒之间"})
			return
		}
		hostOperationalMu.Lock()
		state := loadHostOperationalStateLocked()
		state.Monitor = monitor
		err := saveHostOperationalStateLocked(state)
		hostOperationalMu.Unlock()
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": "保存监控设置失败: " + err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200})
	})
	mux.HandleFunc("POST /api/v2/hosts/monitor/search", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		data := hostRuntimeMetrics(ctx)
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": data})
	})
	mux.HandleFunc("POST /api/v2/hosts/monitor/clean", func(w http.ResponseWriter, r *http.Request) {
		_ = r
		hostOperationalMu.Lock()
		state := loadHostOperationalStateLocked()
		state.Monitor["lastCleanAt"] = time.Now().UTC()
		err := saveHostOperationalStateLocked(state)
		hostOperationalMu.Unlock()
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": "清理监控记录失败: " + err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"cleaned": true}})
	})
	mux.HandleFunc("GET /api/v2/hosts/disks", func(w http.ResponseWriter, _ *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": hostDiskInfo()})
	})
	mux.HandleFunc("GET /api/v2/hosts/firewall/settings", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": firewallStatus(r.Context())})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/base", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": firewallStatus(r.Context())})
	})
	mux.HandleFunc("POST /api/v2/hosts/tool/status", func(w http.ResponseWriter, r *http.Request) {
		typ := "supervisor"
		var in map[string]any
		_ = decodeJSON(r, &in)
		if v, ok := in["type"].(string); ok && strings.TrimSpace(v) != "" {
			typ = strings.TrimSpace(v)
		}
		_, err := exec.LookPath(typ)
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"type": typ, "installed": err == nil}})
	})
}

func hostRuntimeMetrics(ctx context.Context) map[string]any {
	result := map[string]any{"timestamp": time.Now().UTC(), "cpus": runtime.NumCPU(), "goroutines": runtime.NumGoroutine()}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	result["heapAlloc"] = mem.HeapAlloc
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(b))
		if len(fields) > 0 {
			result["load1"] = fields[0]
		}
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 && (f[0] == "MemTotal:" || f[0] == "MemAvailable:") {
				result[strings.TrimSuffix(f[0], ":")] = f[1]
			}
		}
	}
	select {
	case <-ctx.Done():
		result["timedOut"] = true
	default:
	}
	return result
}

func hostDiskInfo() []map[string]any {
	paths := []string{"/", "."}
	if runtime.GOOS == "windows" {
		paths = []string{"."}
	}
	items := make([]map[string]any, 0, len(paths))
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil {
			items = append(items, map[string]any{"path": p, "available": true, "directory": info.IsDir()})
		}
	}
	return items
}

func firewallStatus(parent context.Context) map[string]any {
	_ = parent
	for _, name := range []string{"ufw", "firewall-cmd", "iptables"} {
		if p, err := exec.LookPath(name); err == nil {
			return map[string]any{"available": true, "provider": name, "path": p}
		}
	}
	return map[string]any{"available": false, "provider": "", "reason": "未检测到防火墙命令"}
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
