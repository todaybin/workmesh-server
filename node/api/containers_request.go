// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// handleContainerRequest 分派容器查询、管理和 Docker 命令请求。
func handleContainerRequest(docker service.DockerService, w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/containers")
	path = strings.Trim(path, "/")
	if handleContainerQuery(w, r, path) {
		return
	}
	if r.Method == http.MethodPost && handleContainerHighFrequency(w, r, path) {
		return
	}
	var result model.CommandResult
	var err error

	switch {
	case r.Method == http.MethodGet && path == "docker/status":
		status, statusErr := docker.StatusInfo(r.Context())
		if statusErr != nil {
			wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": statusErr.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": status})
		return
	case r.Method == http.MethodGet && path == "list":
		result, err = docker.List(r.Context())
	case r.Method == http.MethodGet && strings.HasPrefix(path, "stats/"):
		id := strings.TrimPrefix(path, "stats/")
		if !validDockerIdentifier(id) {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "容器标识无效"})
			return
		}
		result, err = runDocker(r, "stats", "--no-stream", id)
	case r.Method == http.MethodGet && path == "image":
		handleImageOptions(w, r)
		return
	case r.Method == http.MethodGet && path == "image/all":
		handleImageAll(w, r)
		return
	case r.Method == http.MethodGet && path == "network":
		handleResourceOptions(w, r, "network")
		return
	case r.Method == http.MethodGet && path == "volume":
		handleResourceOptions(w, r, "volume")
		return
	case r.Method == http.MethodGet && (path == "daemonjson" || path == "daemonjson/file"):
		handleDaemonJSON(w, r, path == "daemonjson/file")
		return
	case r.Method == http.MethodGet && path == "search/log":
		handleContainerLogStream(w, r)
		return
	case r.Method == http.MethodGet && path == "limit":
		result, err = runDocker(r, "info")
	case r.Method == http.MethodPost:
		result, err = handleContainerPost(docker, r, path)
	default:
		wmhttp.JSON(w, http.StatusMethodNotAllowed, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "METHOD_NOT_ALLOWED"}})
		return
	}
	writeContainerCommandResult(w, result, err)
}

// handleContainerHighFrequency 提供前端文件浏览和日志下载所需的专用响应。
// 这些接口不能直接暴露 CommandResult：前端分别期待结构化文件信息和 Blob。
func handleContainerHighFrequency(w http.ResponseWriter, r *http.Request, path string) bool {
	switch path {
	case "download/log", "files/content", "files/size", "files/search", "files/download":
	default:
		return false
	}
	var body containerRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&body); err != nil && !strings.Contains(err.Error(), "EOF") {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return true
	}
	container := body.Container
	if container == "" {
		container = body.ContainerID
	}
	if container == "" {
		container = body.ID
	}
	if container == "" {
		container = body.Name
	}
	if path == "download/log" {
		result, err := containerLogDownloadResult(r, body, container)
		if err != nil || result.ExitCode != 0 {
			writeContainerCommandResult(w, result, err)
			return true
		}
		name := strings.ReplaceAll(container, "/", "-") + ".log"
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		_, _ = w.Write([]byte(result.Stdout))
		return true
	}
	if path == "files/download" {
		if !validDockerIdentifier(container) || !validContainerPath(body.Path) {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "容器标识或文件路径无效"})
			return true
		}
		streamContainerFileDownload(w, r, container, body.Path)
		return true
	}
	if !validDockerIdentifier(container) || !validContainerPath(body.Path) {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "容器标识或文件路径无效"})
		return true
	}
	result, err := runDocker(r, "exec", container, "cat", body.Path)
	if path == "files/content" {
		if err != nil || result.ExitCode != 0 {
			writeContainerCommandResult(w, result, err)
			return true
		}
		content := result.Stdout
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{
			"content": content, "size": len([]byte(content)), "truncated": len(content) >= 1<<20,
			"isBinary": strings.IndexByte(content, 0) >= 0,
		}})
		return true
	}
	if path == "files/size" {
		result, err = runDocker(r, "exec", container, "du", "-sb", body.Path)
		if err != nil || result.ExitCode != 0 {
			writeContainerCommandResult(w, result, err)
			return true
		}
		fields := strings.Fields(result.Stdout)
		size := int64(0)
		if len(fields) > 0 {
			size, _ = strconv.ParseInt(fields[0], 10, 64)
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": size})
		return true
	}
	if path == "files/search" {
		// BusyBox-based images do not implement GNU find -printf.  List paths
		// portably first, then query metadata with the container's stat binary.
		result, err = runDocker(r, "exec", container, "find", body.Path, "-maxdepth", "1", "-mindepth", "1", "-print")
		if err != nil || result.ExitCode != 0 {
			writeContainerCommandResult(w, result, err)
			return true
		}
		items := make([]map[string]any, 0)
		for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
			itemPath := strings.TrimSpace(line)
			if itemPath == "" || !validContainerPath(itemPath) {
				continue
			}
			meta, metaErr := runDocker(r, "exec", container, "stat", "-c", "%F\t%s", itemPath)
			if metaErr != nil || meta.ExitCode != 0 {
				continue
			}
			parts := strings.SplitN(strings.TrimSpace(meta.Stdout), "\t", 2)
			if len(parts) != 2 {
				continue
			}
			size, _ := strconv.ParseInt(parts[1], 10, 64)
			kind := strings.ToLower(parts[0])
			items = append(items, map[string]any{"path": itemPath, "name": filepath.Base(itemPath), "isDir": strings.Contains(kind, "directory"), "isLink": strings.Contains(kind, "symbolic link"), "size": size})
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": items})
		return true
	}
	return false
}

func containerLogDownloadResult(r *http.Request, body containerRequest, container string) (model.CommandResult, error) {
	if !validDockerIdentifier(container) {
		return model.CommandResult{}, &containerError{"容器标识无效"}
	}
	args := []string{"logs"}
	if body.Tail > 0 {
		args = append(args, "--tail", strconv.Itoa(body.Tail))
	}
	if strings.TrimSpace(body.Since) != "" && !strings.EqualFold(strings.TrimSpace(body.Since), "all") {
		if len(body.Since) > 64 || strings.ContainsAny(body.Since, "\r\n\x00") {
			return model.CommandResult{}, &containerError{"since 参数无效"}
		}
		args = append(args, "--since", body.Since)
	}
	if body.Timestamp {
		args = append(args, "--timestamps")
	}
	return runDocker(r, append(args, container)...)
}

// streamContainerFileDownload copies a container path to a private temporary
// directory, then streams it as a regular file or tar.gz for directories.
// docker cp writes binary data to disk, avoiding CommandResult's text/output
// truncation limit and preserving the Blob contract used by the frontend.
func streamContainerFileDownload(w http.ResponseWriter, r *http.Request, container, filePath string) {
	tmpDir, err := os.MkdirTemp("", "workmesh-container-download-")
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	defer os.RemoveAll(tmpDir)
	target := filepath.Join(tmpDir, "payload")
	result, err := runDocker(r, "cp", container+":"+filePath, target)
	if err != nil || result.ExitCode != 0 {
		writeContainerCommandResult(w, result, err)
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	name := filepath.Base(filepath.Clean(filePath))
	if info.IsDir() {
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.tar.gz"`)
		gz := gzip.NewWriter(w)
		tarWriter := tar.NewWriter(gz)
		walkErr := filepath.Walk(target, func(path string, fi os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, relErr := filepath.Rel(target, path)
			if relErr != nil {
				return relErr
			}
			if rel == "." {
				return nil
			}
			header, headerErr := tar.FileInfoHeader(fi, "")
			if headerErr != nil {
				return headerErr
			}
			header.Name = filepath.ToSlash(filepath.Join(name, rel))
			if err := tarWriter.WriteHeader(header); err != nil {
				return err
			}
			if fi.Mode().IsRegular() {
				file, openErr := os.Open(path)
				if openErr != nil {
					return openErr
				}
				_, copyErr := io.Copy(tarWriter, file)
				closeErr := file.Close()
				if copyErr != nil {
					return copyErr
				}
				return closeErr
			}
			return nil
		})
		_ = tarWriter.Close()
		_ = gz.Close()
		if walkErr != nil {
			return
		}
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	file, err := os.Open(target)
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	defer file.Close()
	_, _ = io.Copy(w, file)
}

// writeContainerCommandResult 输出 Docker 命令结果；客户端不存在或守护进程不可连接时返回 503。
func writeContainerCommandResult(w http.ResponseWriter, result model.CommandResult, err error) {
	if err != nil || result.ExitCode != 0 {
		status := http.StatusInternalServerError
		var businessErr *containerError
		if errors.As(err, &businessErr) {
			status = http.StatusBadRequest
		}
		if isDockerUnavailable(result, err) {
			status = http.StatusServiceUnavailable
		}
		message := strings.TrimSpace(result.Stderr)
		if message == "" && err != nil {
			message = err.Error()
		}
		if message == "" {
			message = "Docker 命令执行失败"
		}
		wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": message, "data": result})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
}

// isDockerUnavailable 区分 Docker CLI 缺失或 daemon 不可用，避免错误伪装成普通业务失败。
func isDockerUnavailable(result model.CommandResult, err error) bool {
	if err != nil && errors.Is(err, exec.ErrNotFound) {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(result.Stderr + " " + containerErrorString(err)))
	if result.ExitCode == -1 && strings.TrimSpace(result.Stderr) == "" {
		return true
	}
	for _, marker := range []string{"executable file not found", "cannot connect to the docker daemon", "failed to connect to the docker api", "is the docker daemon running", "docker daemon is not running", "connect: no such file or directory"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	// Docker CLI reports daemon/socket authorization failures as a human
	// readable permission error rather than a stable exit code.
	for _, marker := range []string{"permission denied while trying to connect to the docker api", "permission denied while trying to connect to the docker daemon"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func containerErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// handleContainerQuery 处理无需 Docker 命令的容器查询路由。
func handleContainerQuery(w http.ResponseWriter, r *http.Request, path string) bool {
	if r.Method == http.MethodPost && (path == "list" || path == "list/byimage") {
		handleContainerOptions(w, r, path == "list/byimage")
		return true
	}
	if r.Method == http.MethodPost && path == "info" {
		handleContainerInfo(w, r)
		return true
	}
	if r.Method == http.MethodPost && path == "search" {
		handleContainerSearch(w, r)
		return true
	}
	if r.Method == http.MethodGet && path == "list/stats" {
		handleContainerListStats(w, r)
		return true
	}
	if r.Method == http.MethodGet && path == "status" {
		handleContainerStatus(w, r)
		return true
	}
	if r.Method == http.MethodPost && path == "image/search" {
		handleImageSearch(w, r)
		return true
	}
	if r.Method == http.MethodPost && path == "network/search" {
		handleNetworkSearch(w, r)
		return true
	}
	if r.Method == http.MethodPost && path == "volume/search" {
		handleVolumeSearch(w, r)
		return true
	}
	if r.Method == http.MethodPost && path == "item/stats" {
		handleContainerItemStats(w, r)
		return true
	}
	if r.Method == http.MethodPost && path == "inspect" {
		handleContainerInspect(w, r)
		return true
	}
	return false
}

// handleContainerStatus 汇总容器、Compose、镜像、网络和卷的当前状态。
func handleContainerStatus(w http.ResponseWriter, r *http.Request) {
	items, err := dockerContainerRows(r)
	if err != nil {
		// Docker 守护进程不可用是外部依赖故障，使用 503 让前端区分业务错误并支持重试。
		status := http.StatusInternalServerError
		if strings.Contains(strings.ToLower(err.Error()), "cannot connect") || strings.Contains(strings.ToLower(err.Error()), "daemon") || strings.Contains(strings.ToLower(err.Error()), "docker") {
			status = http.StatusServiceUnavailable
		}
		wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	data := map[string]any{
		"isExist": true, "isActive": true, "containerCount": len(items), "created": 0, "running": 0,
		"paused": 0, "restarting": 0, "removing": 0, "exited": 0, "dead": 0,
	}
	composeProjects := map[string]struct{}{}
	for _, item := range items {
		state := strings.ToLower(fmt.Sprint(item["state"]))
		if _, ok := data[state]; ok {
			data[state] = data[state].(int) + 1
		}
		if project := dockerLabel(item, "com.docker.compose.project"); project != "" {
			composeProjects[project] = struct{}{}
		}
	}
	store := getContainerStore()
	store.mu.RLock()
	data["composeCount"] = len(composeProjects)
	if len(store.state.Composes) > len(composeProjects) {
		data["composeCount"] = len(store.state.Composes)
	}
	data["composeTemplateCount"] = len(store.state.Templates)
	data["repoCount"] = len(store.state.Repositories)
	store.mu.RUnlock()
	for key, args := range map[string][]string{
		"imageCount": {"images", "-q"}, "networkCount": {"network", "ls", "-q"}, "volumeCount": {"volume", "ls", "-q"},
	} {
		result, runErr := runDocker(r, args...)
		if runErr == nil && result.ExitCode == 0 {
			data[key] = countNonEmptyLines(result.Stdout)
		} else {
			data[key] = 0
		}
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}

// dockerJSONLines 执行 Docker 查询并解析逐行 JSON 结果。
func dockerJSONLines(r *http.Request, args ...string) ([]map[string]any, error) {
	result, err := runDocker(r, args...)
	if err != nil || result.ExitCode != 0 {
		if err != nil {
			return nil, fmt.Errorf("docker command failed: %w", err)
		}
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = "Docker 命令执行失败"
		}
		return nil, errors.New(message)
	}
	items := make([]map[string]any, 0)
	for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item map[string]any
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("解析 Docker JSON 失败: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

// parseDockerCreated 将 Docker 创建时间字段转换为 UTC 时间。
func parseDockerCreated(value string) time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05 -0700 MST", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

// resourceDescription 读取网络或卷资源的稳定展示名称。
func resourceDescription(store *containerStore, typ, id string) (string, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	descriptions, _ := store.state.Settings["descriptions"].(map[string]any)
	if descriptions == nil {
		return "", false
	}
	item, _ := descriptions[typ+":"+id].(map[string]any)
	if item == nil {
		return "", false
	}
	description, _ := item["description"].(string)
	pinned, _ := item["isPinned"].(bool)
	return description, pinned
}
