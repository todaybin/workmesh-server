// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"fmt"
	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func handleAppOperation(w http.ResponseWriter, s *appStore, body map[string]any) {
	id := appValue(body, "installId", "installID", "appInstallId", "id")
	operation := strings.ToLower(appValue(body, "operate", "operation"))
	if id == "" {
		if isOpenRestyAppOperation(body, operation) {
			status, err := service.NewWebsiteService("").OperateOpenResty(context.Background(), operation)
			if err != nil {
				runtimeErr(w, http.StatusServiceUnavailable, "OpenResty 操作失败: "+err.Error())
				return
			}
			appOK(w, map[string]any{
				"app":      "openresty",
				"operate":  operation,
				"status":   status.Enabled,
				"running":  status.Available && status.Enabled,
				"accepted": true,
				"runtime":  status,
			})
			return
		}
		runtimeErr(w, http.StatusBadRequest, "应用安装标识不能为空")
		return
	}
	if operation == "" {
		runtimeErr(w, http.StatusBadRequest, "应用操作不能为空")
		return
	}
	s.mu.RLock()
	_, item := findApp(s.state.Apps, id)
	s.mu.RUnlock()
	if item.ID == "" {
		if install, ok := ensureDatabaseInstallLoaded(s, "", id); ok {
			item = install
		}
	}
	if item.ID == "" {
		runtimeErr(w, http.StatusNotFound, "应用不存在: "+id)
		return
	}
	remove := false
	switch operation {
	case "stop", "停止":
		operation = "stop"
	case "start", "启动", "restart", "重启", "reload", "重载":
		if operation == "启动" {
			operation = "start"
		} else if operation == "重启" || operation == "重载" || operation == "reload" {
			operation = "restart"
		}
	case "rebuild", "重建":
		operation = "rebuild"
	case "uninstall", "delete", "卸载":
		remove = true
	default:
		runtimeErr(w, http.StatusBadRequest, "不支持的应用操作: "+operation)
		return
	}
	composePath := appValue(item.Config, "composePath")
	if composePath == "" {
		composePath = appComposePath(item)
	}
	if composePath != "" {
		if _, statErr := os.Stat(composePath); statErr != nil {
			composePath = ""
		}
	}
	if composePath == "" && !remove && len(appContainerNames(item)) == 0 {
		runtimeErr(w, http.StatusBadRequest, "应用 Compose 文件和容器均不存在")
		return
	}
	if composePath != "" {
		op := operation
		if remove {
			op = "down"
		}
		// 原版 start 使用 compose up -d，可创建缺失容器；不能使用 docker start 或 compose start。
		args := []string{"compose"}
		if project := appValue(item.Config, "composeProject", "projectName"); project != "" {
			args = append(args, "--project-name", project)
		}
		args = append(args, "-f", composePath)
		if op == "start" {
			args = append(args, "up", "-d")
		} else if op == "rebuild" {
			downArgs := append([]string{}, args...)
			downArgs = append(downArgs, "down", "--remove-orphans")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			downResult, downErr := (service.CommandService{}).Execute(ctx, model.CommandRequest{Program: service.DockerBinary(), Args: downArgs, Dir: filepath.Dir(composePath), Timeout: 5 * time.Minute})
			if downErr != nil || downResult.ExitCode != 0 {
				cancel()
				message := strings.TrimSpace(downResult.Stderr)
				if message == "" && downErr != nil {
					message = downErr.Error()
				}
				runtimeErr(w, http.StatusBadGateway, "Docker Compose 重建前停止失败: "+message)
				return
			}
			cancel()
			args = append(args, "up", "-d", "--build")
		} else {
			args = append(args, op)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		result, execErr := (service.CommandService{}).Execute(ctx, model.CommandRequest{Program: service.DockerBinary(), Args: args, Dir: filepath.Dir(composePath), Timeout: 5 * time.Minute})
		cancel()
		if execErr != nil || result.ExitCode != 0 {
			message := strings.TrimSpace(result.Stderr)
			if message == "" && execErr != nil {
				message = execErr.Error()
			}
			runtimeErr(w, http.StatusBadGateway, "Docker Compose 操作失败: "+message)
			return
		}
	} else if !remove {
		// 无 Compose 文件的历史安装记录仍按真实容器执行生命周期操作。
		command := operation
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		for _, container := range appContainerNames(item) {
			result, execErr := (service.CommandService{}).Execute(ctx, model.CommandRequest{Program: service.DockerBinary(), Args: []string{command, container}, Timeout: 5 * time.Minute})
			if execErr != nil || result.ExitCode != 0 {
				cancel()
				message := strings.TrimSpace(result.Stderr)
				if message == "" && execErr != nil {
					message = execErr.Error()
				}
				runtimeErr(w, http.StatusBadGateway, "Docker 容器操作失败: "+message)
				return
			}
		}
		cancel()
	}
	if !remove {
		// Compose 文件可能未记录容器名，操作成功后从真实项目状态发现服务容器。
		if composePath != "" {
			if discovered := composeServiceContainers(composePath); discovered != "" {
				s.mu.Lock()
				if index, current := findApp(s.state.Apps, id); index >= 0 {
					current.ContainerName = discovered
					if current.Config == nil {
						current.Config = map[string]any{}
					}
					current.Config["composePath"] = composePath
					s.state.Apps[index] = current
				}
				s.mu.Unlock()
			}
		}
		synced, exists, err := s.syncAppInstallStatus(context.Background(), id, true)
		if err != nil {
			runtimeErr(w, http.StatusBadGateway, "同步应用容器状态失败: "+err.Error())
			return
		}
		if exists {
			item = synced
		}
	}
	s.mu.Lock()
	index, _ := findApp(s.state.Apps, id)
	if index < 0 {
		s.mu.Unlock()
		runtimeErr(w, http.StatusNotFound, "应用不存在: "+id)
		return
	}
	item = s.state.Apps[index]
	item.UpdatedAt = time.Now().UTC()
	if remove {
		removeInstalledDatabaseServer(context.Background(), item)
		s.state.Apps = append(s.state.Apps[:index], s.state.Apps[index+1:]...)
	} else {
		s.state.Apps[index] = item
	}
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		runtimeErr(w, http.StatusInternalServerError, "保存应用状态失败: "+err.Error())
		return
	}
	s.mu.Unlock()
	result := map[string]any{"id": id, "operate": operation, "status": item.Status, "accepted": true}
	if remove {
		result["status"] = "uninstalled"
	}
	appOK(w, result)
}

// isOpenRestyAppOperation 兼容旧版 OpenResty 应用卡片没有返回 installId 的请求。
// 只有明确的 OpenResty 动作才允许走真实运行时，避免把通用应用接口降级成任意容器操作。
func isOpenRestyAppOperation(body map[string]any, operation string) bool {
	switch operation {
	case "start", "stop", "restart", "reload":
	default:
		return false
	}
	key := strings.ToLower(strings.TrimSpace(appValue(body, "key", "appKey", "app", "name", "type")))
	return key == "" || key == "openresty" || key == "nginx"
}

func normalizeAppStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running":
		return "Running"
	case "stopped", "exited":
		return "Stopped"
	case "restarting":
		return "ReStarting"
	case "paused":
		return "Paused"
	case "error":
		return "Error"
	case "unhealthy":
		return "UnHealthy"
	default:
		return strings.TrimSpace(status)
	}
}

func appConfiguredContainerName(item appRecord) string {
	if value := strings.TrimSpace(item.ContainerName); value != "" {
		return value
	}
	if value := appValue(item.Config, "containerName", "CONTAINER_NAME", "container"); value != "" {
		return value
	}
	if params, ok := item.Config["params"].(map[string]any); ok {
		return appValue(params, "containerName", "CONTAINER_NAME")
	}
	return ""
}

func appContainerNames(item appRecord) []string {
	seen := map[string]bool{}
	names := make([]string, 0)
	for _, name := range strings.Split(appConfiguredContainerName(item), ",") {
		name = strings.TrimSpace(name)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

func appConfiguredInt(config map[string]any, fallback int, keys ...string) int {
	for _, key := range keys {
		switch value := config[key].(type) {
		case int:
			if value > 0 {
				return value
			}
		case int64:
			if value > 0 {
				return int(value)
			}
		case uint64:
			if value > 0 {
				return int(value)
			}
		case float64:
			if value > 0 {
				return int(value)
			}
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	if params, ok := config["params"].(map[string]any); ok {
		return appConfiguredInt(params, fallback, keys...)
	}
	return fallback
}

func appStatusIsTransient(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "installing", "rebuilding", "upgrading", "uninstalling", "starting", "restarting", "waiting", "syncing":
		return true
	default:
		return false
	}
}

func (s *appStore) syncAppInstallStatus(ctx context.Context, id string, force bool) (appRecord, bool, error) {
	s.mu.RLock()
	_, item := findApp(s.state.Apps, id)
	loader := s.containerStates
	s.mu.RUnlock()
	if item.ID == "" {
		return appRecord{}, false, nil
	}
	if appStatusIsTransient(item.Status) && !force {
		return item, true, nil
	}
	names := appContainerNames(item)
	states := map[string]string{}
	var err error
	if len(names) > 0 {
		states, err = loader(ctx, names)
		if err != nil {
			return item, true, err
		}
	}
	applyAppContainerStates(&item, names, states, force)
	item.ContainerName = strings.Join(names, ",")
	item.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	index, _ := findApp(s.state.Apps, item.ID)
	if index >= 0 {
		s.state.Apps[index] = item
		err = s.saveLocked()
	}
	s.mu.Unlock()
	if err != nil {
		return item, true, fmt.Errorf("保存应用状态失败: %w", err)
	}
	return item, true, nil
}

func applyAppContainerStates(item *appRecord, names []string, states map[string]string, force bool) {
	if len(names) == 0 || len(states) == 0 {
		if normalizeAppStatus(item.Status) == "UpErr" && !force {
			return
		}
		item.Status = "Error"
		item.Message = "未找到应用容器: " + strings.Join(names, ",")
		return
	}
	counts := map[string]int{}
	missing := make([]string, 0)
	exited := make([]string, 0)
	for _, name := range names {
		state, ok := states[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		state = strings.ToLower(strings.TrimSpace(state))
		counts[state]++
		if state == "exited" {
			exited = append(exited, name)
		}
	}
	total := len(names)
	item.Message = ""
	switch {
	case counts["exited"] == total:
		item.Status = "Stopped"
	case counts["running"] == total:
		item.Status = "Running"
	case counts["restarting"] == total:
		item.Status = "ReStarting"
	case counts["paused"] == total:
		item.Status = "Paused"
	case len(missing) == total:
		item.Status = "Error"
		item.Message = "未找到应用容器: " + strings.Join(missing, ",")
	default:
		item.Status = "UnHealthy"
		parts := make([]string, 0, 2)
		if len(exited) > 0 {
			parts = append(parts, "容器已停止: "+strings.Join(exited, ","))
		}
		if len(missing) > 0 {
			parts = append(parts, "未找到应用容器: "+strings.Join(missing, ","))
		}
		if len(parts) == 0 {
			parts = append(parts, "应用容器状态不一致")
		}
		item.Message = strings.Join(parts, "; ")
	}
}
