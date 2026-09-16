// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
	"strings"
)

func handleContainerSearch(w http.ResponseWriter, r *http.Request) {
	req, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	items, err := dockerContainerRows(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	name := strings.ToLower(valueString(req, "name", "info"))
	state := strings.ToLower(valueString(req, "state"))
	excludeApps, _ := req["excludeAppStore"].(bool)
	filtered := items[:0]
	for _, item := range items {
		if name != "" && !strings.Contains(strings.ToLower(fmt.Sprint(item["name"])), name) {
			continue
		}
		if state != "" && state != "all" && !strings.EqualFold(fmt.Sprint(item["state"]), state) {
			continue
		}
		if excludeApps && item["isFromApp"] == true {
			continue
		}
		filtered = append(filtered, item)
	}
	pageItems, page, pageSize := paginateMaps(filtered, intValue(req, "page"), intValue(req, "pageSize"))
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": pageItems, "total": len(filtered), "page": page, "pageSize": pageSize}})
}

func dockerContainerRows(r *http.Request) ([]map[string]any, error) {
	result, err := runDocker(r, "ps", "-a", "--no-trunc", "--format", "{{json .}}")
	if err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" && err != nil {
			message = err.Error()
		}
		if message == "" {
			message = "Docker 命令执行失败"
		}
		return nil, errors.New(message)
	}
	appContainers := map[string]string{}
	appStore := getAppStore()
	appStore.mu.RLock()
	for _, app := range appStore.state.Apps {
		for _, name := range strings.Split(app.ContainerName, ",") {
			if name = strings.TrimSpace(name); name != "" {
				appContainers[name] = app.Name
			}
		}
	}
	appStore.mu.RUnlock()
	return parseDockerContainerRows(result.Stdout, appContainers)
}

func parseDockerContainerRows(output string, appContainers map[string]string) ([]map[string]any, error) {
	items := make([]map[string]any, 0)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return nil, fmt.Errorf("解析 Docker 容器列表失败: %w", err)
		}
		name := valueString(raw, "Names", "Name")
		appName, isFromApp := appContainers[name]
		ports := []string{}
		if value := valueString(raw, "Ports"); value != "" {
			for _, port := range strings.Split(value, ",") {
				ports = append(ports, strings.TrimSpace(port))
			}
		}
		labels := valueString(raw, "Labels")
		items = append(items, map[string]any{
			"containerID": valueString(raw, "ID"), "name": name, "imageName": valueString(raw, "Image"),
			"createTime": valueString(raw, "CreatedAt"), "state": strings.ToLower(valueString(raw, "State")),
			"runTime": valueString(raw, "Status"), "network": []string{}, "ports": ports,
			"isFromApp": isFromApp, "isFromCompose": strings.Contains(labels, "com.docker.compose.project="),
			"appName": appName, "appInstallName": appName, "labels": labels,
		})
	}
	return items, nil
}

func dockerLabel(item map[string]any, key string) string {
	for _, label := range strings.Split(fmt.Sprint(item["labels"]), ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(label), "=")
		if ok && name == key {
			return value
		}
	}
	return ""
}

func paginateMaps(items []map[string]any, page, pageSize int) ([]map[string]any, int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 1000 {
		pageSize = 100
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []map[string]any{}, page, pageSize
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], page, pageSize
}
