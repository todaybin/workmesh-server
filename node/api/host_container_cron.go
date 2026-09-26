// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// BackgroundTasksConfigured 查询 SQLite 中是否存在已启用的计划任务。
// 文件仅作为用户脚本等外部资源，不参与系统任务开关判断。
func BackgroundTasksConfigured() bool {
	repository, err := SharedRepository()
	if err != nil {
		return false
	}
	var count int
	if err := repository.QueryRow(`SELECT COUNT(*) FROM cronjobs WHERE lower(COALESCE(json_extract(payload,'$.status'),''))='enabled'`).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

var sharedCronjobs = service.NewCronjobService()
var (
	backgroundMu      sync.Mutex
	backgroundStarted bool
	retentionStarted  bool
)

// StartBackgroundTasks 启动节点级后台调度任务，调用方应在进程退出时取消 ctx。
func StartBackgroundTasks(ctx context.Context) {
	sharedCronjobs.Start(ctx)
	StartHostMonitor(ctx)
	backgroundMu.Lock()
	if !retentionStarted {
		retentionStarted = true
		go func() {
			defer func() {
				backgroundMu.Lock()
				retentionStarted = false
				backgroundMu.Unlock()
			}()
			startLogRetentionScheduler(ctx)
		}()
	}
	backgroundMu.Unlock()
	if !certificateRenewalConfigured() {
		return
	}
	backgroundMu.Lock()
	if backgroundStarted {
		backgroundMu.Unlock()
		return
	}
	backgroundStarted = true
	backgroundMu.Unlock()
	// 与 1Panel agent/cron/cron.go 一致：证书任务每 6 小时执行一次。
	security := service.NewWebsiteSecurityService("")
	ssls := service.NewSSLService()
	go func() {
		defer func() {
			backgroundMu.Lock()
			backgroundStarted = false
			backgroundMu.Unlock()
		}()
		const interval = 6 * time.Hour
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		security.RenewDueCertificates(ctx, 30*24*time.Hour)
		ssls.RenewDue(ctx, 30*24*time.Hour)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				security.RenewDueCertificates(ctx, 30*24*time.Hour)
				ssls.RenewDue(ctx, 30*24*time.Hour)
			}
		}
	}()
}

func certificateRenewalConfigured() bool {
	repository, err := SharedRepository()
	if err != nil {
		return false
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM website_ssls WHERE auto_renew=1`,
		`SELECT COUNT(*) FROM website_ca_ssls WHERE auto_renew=1`,
	} {
		var count int
		if err := repository.QueryRow(query).Scan(&count); err == nil && count > 0 {
			return true
		}
	}
	return false
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
	registerCronRoutes(mux, cronjobs)
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

// registerHostOperationalRoutes 提供旧主机面板使用的真实本机状态接口。
func registerHostOperationalRoutes(mux *http.ServeMux) {
	registerHostSSHRoutes(mux)
	registerHostSSHCertRoutes(mux)
	registerHostDiskOperationRoutes(mux)
	registerHostSupervisorRoutes(mux)
	registerHostMonitorInterfaces(mux)
	registerHostMonitorSettings(mux)
	mux.HandleFunc("POST /api/v2/hosts/monitor/search", func(w http.ResponseWriter, r *http.Request) {
		var request hostMonitorSearch
		if err := decodeJSON(r, &request); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		data, err := searchHostMonitor(request)
		if err != nil {
			var input monitorInputError
			if errors.As(err, &input) {
				wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
				return
			}
			wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
	})
	mux.HandleFunc("POST /api/v2/hosts/monitor/clean", func(w http.ResponseWriter, r *http.Request) {
		if err := cleanHostMonitor(); err != nil {
			wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": "清理监控记录失败: " + err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"cleaned": true}})
	})
	mux.HandleFunc("GET /api/v2/hosts/disks", func(w http.ResponseWriter, _ *http.Request) {
		data, err := hostDiskInfo()
		if err != nil {
			wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": data})
	})
	mux.HandleFunc("GET /api/v2/hosts/firewall/settings", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": firewallSettings(r.Context())})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/base", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": firewallStatus(r.Context())})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/settings/operate", handleFirewallBackendOperate)
	mux.HandleFunc("POST /api/v2/hosts/firewall/operate", handleFirewallOperate)
	mux.HandleFunc("POST /api/v2/hosts/firewall/filter/operate", handleFirewallBackendOperate)
	mux.HandleFunc("POST /api/v2/hosts/firewall/forward/search", handleForwardRuleSearch)
	mux.HandleFunc("POST /api/v2/hosts/firewall/forward/operate", handleForwardRuleOperate)
	mux.HandleFunc("POST /api/v2/hosts/firewall/forward/base", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": firewallStatus(r.Context())})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/forward/enable", func(w http.ResponseWriter, _ *http.Request) {
		if os.Getenv("WORKMESH_ALLOW_FIREWALL_MUTATION") != "1" {
			wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": "转发启用需要 WORKMESH_ALLOW_FIREWALL_MUTATION=1"})
			return
		}
		if _, err := runFirewallCommand(context.Background(), "sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
			wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"enabled": true}})
	})
	mux.HandleFunc("GET /api/v2/hosts/firewall/docker/ports", handleDockerGuardPorts)
	mux.HandleFunc("GET /api/v2/hosts/firewall/docker/endpoints", handleDockerGuardPorts)
	mux.HandleFunc("POST /api/v2/hosts/firewall/docker/sync", handleDockerGuardSync)
	mux.HandleFunc("POST /api/v2/hosts/firewall/docker/operate", handleDockerGuardOperate)
	mux.HandleFunc("POST /api/v2/hosts/firewall/docker/policies/batch", handleDockerGuardPolicyBatch)
	mux.HandleFunc("POST /api/v2/hosts/firewall/docker/policies/delete/batch", handleDockerGuardPolicyDelete)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/search", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Scope map[string]any `json:"scope"`
		}
		if err := decodeJSON(r, &request); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		inventory, err := firewallInventory(r.Context(), request.Scope)
		if err != nil {
			wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": inventory})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/check", handleFirewallRuleCheck)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules", handleFirewallRuleCreate)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/delete", handleFirewallRuleDelete)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/reset", handleFirewallRuleReset)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/update", handleFirewallRuleUpdate)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/native/detail", handleFirewallNativeDetail)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/sync/preview", handleFirewallSyncPreview)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/sync", handleFirewallSync)
	mux.HandleFunc("GET /api/v2/hosts/firewall/rules/sync/task", handleFirewallSyncTask)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/reorder", handleFirewallRuleReorder)
}

func decodeJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return errors.New("请求体不能为空")
	}
	return decodeSingleJSON(r.Body, target, 2<<20)
}

func writeCommandResult(w http.ResponseWriter, result model.CommandResult, err error) {
	if err != nil {
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error(), "data": result})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
}
