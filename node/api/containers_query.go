// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// handleContainerOptions 返回前端下拉框所需的真实 Docker 容器名称和状态。
func handleContainerOptions(w http.ResponseWriter, r *http.Request, byImage bool) {
	var req map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req)
	}
	image := strings.TrimSpace(valueString(req, "name", "image"))
	rows, err := dockerJSONLines(r, "ps", "-a", "--format", "{{json .}}")
	if err != nil {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if byImage && image != "" && !strings.EqualFold(valueString(row, "Image"), image) {
			continue
		}
		items = append(items, map[string]any{"name": valueString(row, "Names", "Name"), "state": strings.ToLower(valueString(row, "State"))})
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": items})
}

// handleContainerInfo 将 Docker inspect 的真实配置转换为兼容前端的 data 对象。
func handleContainerInfo(w http.ResponseWriter, r *http.Request) {
	req, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	id := strings.TrimSpace(valueString(req, "name", "id", "container"))
	if !validDockerIdentifier(id) {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "容器名称不能为空"})
		return
	}
	rows, err := dockerJSONLines(r, "inspect", "--format", "{{json .}}", id)
	if err != nil || len(rows) == 0 {
		if err == nil {
			err = errors.New("容器不存在")
		}
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": rows[0]})
}

func handleComposeSearch(w http.ResponseWriter, r *http.Request) {
	req, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	containers, err := dockerContainerRows(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	byProject := map[string][]map[string]any{}
	for _, item := range containers {
		project := dockerLabel(item, "com.docker.compose.project")
		if project != "" {
			byProject[project] = append(byProject[project], item)
		}
	}
	store := getContainerStore()
	store.mu.RLock()
	records := append([]composeRecord(nil), store.state.Composes...)
	store.mu.RUnlock()
	info := strings.ToLower(valueString(req, "info", "name"))
	excludeApps, _ := req["excludeAppStore"].(bool)
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		if info != "" && !strings.Contains(strings.ToLower(record.Name), info) {
			continue
		}
		projectContainers := byProject[record.Name]
		createdBy := "WorkMesh"
		if record.AppInstallID != "" {
			createdBy = "Apps"
		}
		if len(projectContainers) > 0 {
			if label := dockerLabel(projectContainers[0], "createdBy"); label != "" {
				createdBy = label
			}
		}
		if excludeApps && strings.EqualFold(createdBy, "Apps") {
			continue
		}
		composeContainers := make([]map[string]any, 0, len(projectContainers))
		running := 0
		for _, container := range projectContainers {
			state := fmt.Sprint(container["state"])
			if strings.EqualFold(state, "running") {
				running++
			}
			composeContainers = append(composeContainers, map[string]any{
				"containerID": container["containerID"], "name": container["name"], "createTime": container["createTime"],
				"state": state, "ports": container["ports"],
			})
		}
		env, _ := os.ReadFile(filepath.Join(filepath.Dir(record.Path), ".env"))
		_, statErr := os.Stat(record.Path)
		items = append(items, map[string]any{
			"name": record.Name, "createdAt": record.CreatedAt.Format("2006-01-02 15:04:05"), "createdBy": createdBy,
			"containerCount": len(composeContainers), "runningCount": running, "configFile": record.Path,
			"workdir": filepath.Dir(record.Path), "composeFileExists": statErr == nil, "isPinned": record.Pinned,
			"path": record.Path, "containers": composeContainers, "env": string(env), "appInstallId": record.AppInstallID,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, _ := items[i]["isPinned"].(bool)
		right, _ := items[j]["isPinned"].(bool)
		if left != right {
			return left
		}
		return fmt.Sprint(items[i]["createdAt"]) > fmt.Sprint(items[j]["createdAt"])
	})
	pageItems, page, pageSize := paginateMaps(items, intValue(req, "page"), intValue(req, "pageSize"))
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": pageItems, "total": len(items), "page": page, "pageSize": pageSize}})
}
func handleComposeTest(w http.ResponseWriter, r *http.Request) {
	var req composeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	result, err := composeCommand(r, req, "config")
	writeContainerCommandResult(w, result, err)
}
func handleComposeOperate(w http.ResponseWriter, r *http.Request) {
	var req composeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	operation := strings.ToLower(strings.TrimSpace(req.Operation))
	if req.TaskID != "" && !taskIdentifier.MatchString(req.TaskID) {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "任务 ID 无效"})
		return
	}
	if req.TaskID != "" {
		ensureAppTaskLog(req.TaskID, "", "docker-compose", "executing", "开始执行 Compose "+operation)
	}
	result, err := composeCommand(r, req, operation)
	if err != nil || result.ExitCode != 0 {
		if req.TaskID != "" {
			message := strings.TrimSpace(result.Stderr)
			if message == "" && err != nil {
				message = err.Error()
			}
			ensureAppTaskLog(req.TaskID, "", "docker-compose", "failed", message)
			appendAppTaskLog(req.TaskID, "[TASK-END]")
		}
		writeContainerCommandResult(w, result, err)
		return
	}
	if req.TaskID != "" {
		if strings.TrimSpace(result.Stdout) != "" {
			appendAppTaskLog(req.TaskID, result.Stdout)
		}
		ensureAppTaskLog(req.TaskID, "", "docker-compose", "completed", "Compose 操作完成")
		appendAppTaskLog(req.TaskID, "[TASK-END]")
	}
	if operation == "delete" || operation == "remove" {
		store := getContainerStore()
		store.mu.Lock()
		kept := store.state.Composes[:0]
		for _, item := range store.state.Composes {
			if item.Name == req.Name || item.Path == req.Path {
				if req.WithFile {
					_ = os.Remove(item.Path)
					_ = os.Remove(filepath.Join(filepath.Dir(item.Path), ".env"))
				}
				continue
			}
			kept = append(kept, item)
		}
		store.state.Composes = kept
		saveErr := store.saveLocked()
		store.mu.Unlock()
		if saveErr != nil {
			writeContainerCommandResult(w, result, saveErr)
			return
		}
	}
	writeContainerCommandResult(w, result, nil)
}

func handleComposeEnv(w http.ResponseWriter, r *http.Request) {
	var req composeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	path := composeFilePath(req)
	if err := validateComposeFile(path); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	// Compose 环境优先读取项目目录 .env；变量值原样返回，避免执行 shell 或 Docker。
	envPath := filepath.Join(filepath.Dir(path), ".env")
	data := map[string]string{}
	if b, err := os.ReadFile(envPath); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			kv := strings.SplitN(line, "=", 2)
			if len(kv) == 2 && validEnvKey(strings.TrimSpace(kv[0])) {
				data[strings.TrimSpace(kv[0])] = strings.Trim(strings.TrimSpace(kv[1]), "\"")
			}
		}
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": data})
}

func handleComposeCleanLog(w http.ResponseWriter, r *http.Request) {
	var req composeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil || !validDockerPath(req.Path) {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "compose path is required"})
		return
	}
	ids, err := runDocker(r, "compose", "-f", req.Path, "ps", "-q")
	if err != nil {
		writeContainerCommandResult(w, ids, err)
		return
	}
	cleaned := 0
	for _, id := range strings.Fields(ids.Stdout) {
		if !validDockerIdentifier(id) {
			continue
		}
		logPath, e := runDocker(r, "inspect", "--format", "{{.LogPath}}", id)
		if e != nil {
			continue
		}
		p := strings.TrimSpace(logPath.Stdout)
		if p == "" || !filepath.IsAbs(p) || strings.Contains(p, "..") {
			continue
		}
		if e := os.Truncate(p, 0); e == nil {
			cleaned++
		}
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"cleaned": cleaned}})
}

func isContainerRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return path == "/api/v2/containers" || strings.HasPrefix(path, "/api/v2/containers/")
}
