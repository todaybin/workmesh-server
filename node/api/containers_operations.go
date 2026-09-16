// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// handleContainerPost 分发容器生命周期、镜像、资源和文件操作请求。
// 所有外部命令均通过参数数组执行；路径分支只负责协议适配和输入校验。
func handleContainerPost(docker service.DockerService, r *http.Request, path string) (model.CommandResult, error) {
	var body containerRequest
	if r.Body != nil {
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&body); err != nil && !strings.Contains(err.Error(), "EOF") {
			return model.CommandResult{}, &containerError{"请求参数无效"}
		}
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
	switch {
	case path == "" || path == "create":
		if !validDockerIdentifier(body.Image) || !validDockerIdentifier(body.Name) {
			return model.CommandResult{}, &containerError{"container name and image are required"}
		}
		args := []string{"run", "-d", "--name", body.Name}
		for key, value := range body.Env {
			if !validEnvKey(key) || strings.ContainsAny(value, "\x00\r\n") {
				return model.CommandResult{}, &containerError{"invalid environment variable"}
			}
			args = append(args, "-e", key+"="+value)
		}
		args = append(args, body.Image)
		args = append(args, body.Command...)
		return runDocker(r, args...)
	case path == "update":
		if !validDockerIdentifier(container) {
			return model.CommandResult{}, errContainerParameter
		}
		args := []string{"update"}
		if body.CPU != "" {
			args = append(args, "--cpus", body.CPU)
		}
		if body.Memory != "" {
			args = append(args, "--memory", body.Memory)
		}
		if len(args) == 1 {
			return model.CommandResult{}, &containerError{"cpu or memory is required"}
		}
		args = append(args, container)
		return runDocker(r, args...)
	case path == "upgrade":
		if len(body.Paths) > 0 {
			for _, name := range body.Paths {
				if !validDockerIdentifier(name) {
					return model.CommandResult{}, errContainerParameter
				}
			}
			if !validDockerIdentifier(body.Image) {
				return model.CommandResult{}, &containerError{"镜像名称无效"}
			}
			return runDocker(r, "pull", body.Image)
		}
		if !validDockerIdentifier(body.Image) {
			return model.CommandResult{}, &containerError{"镜像名称无效"}
		}
		return runDocker(r, "pull", body.Image)
	case path == "search" || path == "list":
		args := []string{"ps", "-a", "--no-trunc"}
		if body.Image != "" {
			args = append(args, "--filter", "ancestor="+body.Image)
		}
		if body.Content != "" {
			args = append(args, "--filter", "name="+body.Content)
		}
		return runDocker(r, args...)
	case path == "list/byimage":
		if !validDockerIdentifier(body.Image) {
			return model.CommandResult{}, &containerError{"image is required"}
		}
		return runDocker(r, "ps", "-a", "--filter", "ancestor="+body.Image, "--no-trunc")
	case path == "users":
		if !validDockerIdentifier(container) {
			return model.CommandResult{}, errContainerParameter
		}
		// 容器用户来自容器自身的 passwd 数据，不在宿主机伪造。
		return runDocker(r, "exec", container, "sh", "-c", "cat /etc/passwd")
	case path == "item/stats":
		if !validDockerIdentifier(container) {
			return model.CommandResult{}, errContainerParameter
		}
		return runDocker(r, "inspect", "--size", container)
	case path == "operate" || path == "docker/operate":
		if len(body.Names) > 0 {
			var last model.CommandResult
			for _, name := range body.Names {
				if !validDockerIdentifier(name) {
					return model.CommandResult{}, errContainerParameter
				}
				var runErr error
				last, runErr = docker.Operate(r.Context(), model.DockerOperationRequest{Container: name, Operation: body.Operation})
				if runErr != nil {
					return last, runErr
				}
			}
			return last, nil
		}
		return docker.Operate(r.Context(), model.DockerOperationRequest{Container: container, Operation: body.Operation})
	case path == "inspect" || path == "info":
		if container == "" {
			return model.CommandResult{}, errContainerParameter
		}
		return runDocker(r, "inspect", container)
	case path == "prune":
		pruneType := strings.ToLower(strings.TrimSpace(body.PruneType))
		if pruneType == "" {
			pruneType = strings.ToLower(strings.TrimSpace(body.Operation))
		}
		if pruneType == "" {
			pruneType = strings.ToLower(strings.TrimSpace(body.Name))
		}
		allowed := map[string]bool{"container": true, "image": true, "volume": true, "network": true, "builder": true, "buildcache": true, "system": true}
		if !allowed[pruneType] {
			return model.CommandResult{}, &containerError{"清理类型无效"}
		}
		if pruneType == "buildcache" {
			pruneType = "builder"
		}
		return runContainerPrune(r, body, pruneType)
	case path == "clean/log":
		if !validDockerIdentifier(container) {
			return model.CommandResult{}, errContainerParameter
		}
		return runDocker(r, "logs", "--tail", "0", container)
	case path == "download/log":
		if !validDockerIdentifier(container) {
			return model.CommandResult{}, errContainerParameter
		}
		return runDocker(r, "logs", container)
	case path == "rename":
		if !validDockerIdentifier(container) || !validDockerIdentifier(body.Name) {
			return model.CommandResult{}, &containerError{"invalid container name"}
		}
		return runDocker(r, "rename", container, body.Name)
	case path == "commit":
		if !validDockerIdentifier(container) || !validDockerIdentifier(body.Image) {
			return model.CommandResult{}, &containerError{"container and image are required"}
		}
		return runDocker(r, "commit", container, body.Image)
	case path == "daemonjson" || path == "daemonjson/update" || path == "daemonjson/update/byfile":
		return updateDaemonJSON(r)
	case strings.HasPrefix(path, "image/"):
		return handleImageOperation(r, path, body)
	case strings.HasPrefix(path, "network"):
		return handleDockerResourceOperation(r, "network", path, body.Name)
	case strings.HasPrefix(path, "volume"):
		return handleDockerResourceOperation(r, "volume", path, body.Name)
	case strings.HasPrefix(path, "files/"):
		return handleContainerFileOperation(r, path, container, body.Path, body.HostPath, body.Content)
	default:
		// Compose、模板和仓库等路径保留明确的可观测错误，避免伪造执行成功。
		return model.CommandResult{}, unsupportedContainerOperation(path)
	}
}

// runContainerPrune 执行受白名单约束的容器清理动作，并将有 taskID 的输出写入任务日志。
func runContainerPrune(r *http.Request, body containerRequest, pruneType string) (model.CommandResult, error) {
	args := []string{pruneType, "prune", "-f"}
	if body.WithTagAll && (pruneType == "image" || pruneType == "builder") {
		args = append(args, "-a")
	}
	taskID := strings.TrimSpace(body.TaskID)
	if taskID != "" && !taskIdentifier.MatchString(taskID) {
		return model.CommandResult{}, &containerError{"任务 ID 无效"}
	}
	if taskID != "" {
		ensureAppTaskLog(taskID, "", "docker-prune", "executing", "开始清理 Docker "+pruneType)
	}
	return runDockerTask(r, taskID, "docker-prune", "开始清理 Docker "+pruneType, "Docker 清理完成", args...)
}

// runDockerTask 为带 taskID 的 Docker 操作统一记录开始、输出、结果和结束标记。
func runDockerTask(r *http.Request, taskID, taskName, startMessage, successMessage string, args ...string) (model.CommandResult, error) {
	if taskID == "" {
		return runDocker(r, args...)
	}
	if !taskIdentifier.MatchString(taskID) {
		return model.CommandResult{}, &containerError{"任务 ID 无效"}
	}
	ensureAppTaskLog(taskID, "", taskName, "executing", startMessage)
	result, err := runDocker(r, args...)
	if output := strings.TrimSpace(result.Stdout); output != "" {
		appendAppTaskLog(taskID, output)
	}
	if output := strings.TrimSpace(result.Stderr); output != "" {
		appendAppTaskLog(taskID, output)
	}
	if err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" && err != nil {
			message = err.Error()
		}
		if message == "" {
			message = "Docker 命令执行失败"
		}
		ensureAppTaskLog(taskID, "", taskName, "failed", message)
		appendAppTaskLog(taskID, "[TASK-END]")
		return result, errors.New(message)
	}
	ensureAppTaskLog(taskID, "", taskName, "running", successMessage)
	appendAppTaskLog(taskID, "[TASK-END]")
	return result, nil
}

// validEnvKey 校验容器环境变量名，拒绝换行和 shell 元字符。
func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i, c := range key {
		if !(c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || (i > 0 && c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// validContainerPath 校验容器内路径，拒绝空值和路径穿越。
func validContainerPath(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && strings.HasPrefix(value, "/") && !strings.ContainsAny(value, "\x00\r\n") && !strings.Contains(value, "..") && len(value) <= 4096
}

// handleContainerFileOperation 执行容器文件读写，统一限制路径、内容大小和命令参数。
func handleContainerFileOperation(r *http.Request, path, container, filePath, hostPath, content string) (model.CommandResult, error) {
	if !validDockerIdentifier(container) || !validContainerPath(filePath) {
		return model.CommandResult{}, &containerError{"invalid container or file path"}
	}
	switch path {
	case "files/search":
		return runDocker(r, "exec", container, "find", filePath, "-maxdepth", "1", "-printf", "%p\\n")
	case "files/content":
		return runDocker(r, "exec", container, "cat", filePath)
	case "files/size":
		return runDocker(r, "exec", container, "du", "-sb", filePath)
	case "files/del":
		paths := []string{filePath}
		for _, item := range strings.Fields(hostPath) {
			if !validContainerPath(item) {
				return model.CommandResult{}, &containerError{"容器文件路径无效"}
			}
			paths = append(paths, item)
		}
		args := []string{"exec", container, "rm", "-rf"}
		args = append(args, paths...)
		return runDocker(r, args...)
	case "files/download":
		if !validDockerPath(hostPath) {
			return model.CommandResult{}, &containerError{"invalid target path"}
		}
		return runDocker(r, "cp", container+":"+filePath, hostPath)
	case "files/upload":
		if !validDockerPath(hostPath) {
			return model.CommandResult{}, &containerError{"invalid source path"}
		}
		return runDocker(r, "cp", hostPath, container+":"+filePath)
	default:
		return model.CommandResult{}, unsupportedContainerOperation(path)
	}
}

// errContainerParameter 统一表示缺少或非法的容器标识，避免把用户输入拼入错误消息。
var errContainerParameter = &containerError{"容器标识不能为空"}

// containerError 表示容器接口的可预期业务错误，不携带完整 Docker 命令或敏感参数。
type containerError struct{ message string }

// Error 返回容器操作的业务错误摘要，不暴露敏感命令参数。
func (e *containerError) Error() string { return e.message }

// unsupportedContainerOperation 为未列入白名单的容器动作返回明确错误。
func unsupportedContainerOperation(path string) error {
	return &containerError{"容器操作暂未接入 Docker 驱动: " + path}
}

func imageOperationNames(body containerRequest) []string {
	for _, values := range [][]string{body.Names, []string(body.ImageNames)} {
		names := make([]string, 0, len(values))
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				names = append(names, value)
			}
		}
		if len(names) > 0 {
			return names
		}
	}
	for _, value := range []string{body.Image, body.Name} {
		if value = strings.TrimSpace(value); value != "" {
			return []string{value}
		}
	}
	return nil
}

// handleImageOperation 分发镜像拉取、删除和查询操作，使用参数数组调用 Docker。
func handleImageOperation(r *http.Request, path string, body containerRequest) (model.CommandResult, error) {
	image := strings.TrimSpace(body.Image)
	if image == "" {
		image = strings.TrimSpace(body.Name)
	}
	switch path {
	case "image/pull":
		images := imageOperationNames(body)
		if len(images) == 0 {
			return model.CommandResult{}, &containerError{"镜像名称不能为空"}
		}
		repo, err := containerRepository(body.RepoID)
		if err != nil {
			return model.CommandResult{}, err
		}
		registry := ""
		if repo != nil {
			registry, err = normalizeRegistryReference(repo.DownloadURL)
			if err != nil {
				return model.CommandResult{}, err
			}
		}
		var combined strings.Builder
		for _, name := range images {
			name = strings.TrimSpace(name)
			if !validDockerIdentifier(name) {
				return model.CommandResult{}, &containerError{"镜像名称无效"}
			}
			if registry != "" && !strings.Contains(name, "/") {
				name = registry + "/" + name
			}
			result, pullErr := runDocker(r, "pull", name)
			if pullErr != nil || result.ExitCode != 0 {
				return result, pullErr
			}
			combined.WriteString(result.Stdout)
		}
		return model.CommandResult{ExitCode: 0, Stdout: combined.String()}, nil
	case "image/remove":
		images := imageOperationNames(body)
		if len(images) == 0 {
			return model.CommandResult{}, &containerError{"镜像名称不能为空"}
		}
		args := []string{"rmi"}
		for _, name := range images {
			if !validDockerIdentifier(name) {
				return model.CommandResult{}, &containerError{"镜像名称无效"}
			}
			args = append(args, name)
		}
		return runDockerTask(r, strings.TrimSpace(body.TaskID), "docker-image-remove", "开始删除 Docker 镜像", "Docker 镜像删除完成", args...)
	case "image/search":
		if !validDockerIdentifier(image) {
			return model.CommandResult{}, &containerError{"镜像关键词不能为空"}
		}
		return runDocker(r, "search", image)
	case "image/tag":
		source := strings.TrimSpace(body.SourceID)
		if source == "" {
			source = image
		}
		if !validDockerIdentifier(source) {
			return model.CommandResult{}, &containerError{"源镜像无效"}
		}
		tags := append([]string(nil), body.Tags...)
		if len(tags) == 0 && validDockerIdentifier(body.Repository) && validDockerIdentifier(body.Tag) {
			tags = []string{body.Repository + ":" + body.Tag}
		}
		if len(tags) == 0 {
			return model.CommandResult{}, &containerError{"镜像标签不能为空"}
		}
		var last model.CommandResult
		for _, tag := range tags {
			if !validDockerIdentifier(tag) {
				return model.CommandResult{}, &containerError{"镜像标签无效"}
			}
			var tagErr error
			last, tagErr = runDocker(r, "tag", source, tag)
			if tagErr != nil || last.ExitCode != 0 {
				return last, tagErr
			}
		}
		return last, nil
	case "image/push":
		repo, err := containerRepository(body.RepoID)
		if err != nil {
			return model.CommandResult{}, err
		}
		local := strings.TrimSpace(body.TagName)
		if local == "" {
			local = strings.TrimSpace(body.Image)
		}
		if local == "" {
			local = strings.TrimSpace(body.Name)
		}
		if !validDockerIdentifier(local) {
			return model.CommandResult{}, &containerError{"镜像名称无效"}
		}
		if repo == nil || strings.TrimSpace(repo.DownloadURL) == "" {
			return model.CommandResult{}, &containerError{"推送仓库不存在或未配置地址"}
		}
		registry, err := normalizeRegistryReference(repo.DownloadURL)
		if err != nil {
			return model.CommandResult{}, err
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			name = local
		}
		name = strings.TrimPrefix(name, registry+"/")
		if !validDockerIdentifier(name) {
			return model.CommandResult{}, &containerError{"目标镜像名称无效"}
		}
		target := registry + "/" + name
		if strings.HasPrefix(local, registry+"/") {
			target = local
		}
		tagCreated := false
		if target != local {
			// Docker pushes a repository-qualified tag. Create that tag from the
			// local source before pushing; the API must not rely on callers having
			// performed a separate tag operation.
			tagResult, tagErr := runDocker(r, "tag", local, target)
			if tagErr != nil || tagResult.ExitCode != 0 {
				return tagResult, tagErr
			}
			tagCreated = true
		}
		if repo.Auth {
			user, password := repo.Username, repo.Password
			if body.Username != "" {
				user = body.Username
			}
			if body.Password != "" {
				password = body.Password
			}
			if user == "" || password == "" {
				return model.CommandResult{}, &containerError{"认证仓库缺少用户名或密码"}
			}
			login, loginErr := runDockerWithStdin(r, []string{"login", registry, "--username", user, "--password-stdin"}, password+"\n")
			if loginErr != nil || login.ExitCode != 0 {
				return login, loginErr
			}
			defer func() { _, _ = runDocker(r, "logout", registry) }()
		}
		result, pushErr := runDocker(r, "push", target)
		if tagCreated && (pushErr != nil || result.ExitCode != 0) {
			// A failed push must not leave an automatically-created local tag.
			_, _ = runDocker(r, "rmi", target)
		}
		return result, pushErr
	case "image/build":
		// Older 1Panel clients sent the build context in `name` and the
		// resulting image tag in `image`. Keep that payload compatible while
		// retaining the current contract (`name` + `dockerfile`).
		buildName := strings.TrimSpace(body.Name)
		dockerfile := strings.TrimSpace(body.Dockerfile)
		if dockerfile == "" && strings.TrimSpace(body.Image) != "" && validDockerPath(buildName) {
			body.From = "path"
			dockerfile = filepath.Join(buildName, "Dockerfile")
			buildName = strings.TrimSpace(body.Image)
		}
		if !validDockerIdentifier(buildName) {
			return model.CommandResult{}, &containerError{"镜像名称无效"}
		}
		if dockerfile == "" {
			return model.CommandResult{}, &containerError{"Dockerfile 路径或内容不能为空"}
		}
		args := []string{"build", "-t", buildName}
		contextDir := ""
		cleanup := func() {}
		if strings.EqualFold(strings.TrimSpace(body.From), "edit") {
			tmp, err := os.MkdirTemp("", "workmesh-dockerfile-")
			if err != nil {
				return model.CommandResult{}, err
			}
			cleanup = func() { _ = os.RemoveAll(tmp) }
			defer cleanup()
			file := filepath.Join(tmp, "Dockerfile")
			if err := os.WriteFile(file, []byte(dockerfile), 0o600); err != nil {
				return model.CommandResult{}, err
			}
			contextDir = tmp
		} else {
			if !validDockerPath(dockerfile) || strings.Contains(dockerfile, "..") {
				return model.CommandResult{}, &containerError{"Dockerfile 路径无效"}
			}
			contextDir = filepath.Dir(filepath.Clean(dockerfile))
			args = append(args, "-f", filepath.Clean(dockerfile))
		}
		for _, buildArg := range body.Args {
			if !validBuildArgument(buildArg) {
				return model.CommandResult{}, &containerError{"构建参数无效"}
			}
			args = append(args, "--build-arg", buildArg)
		}
		for _, tag := range body.Tags {
			if !validDockerIdentifier(tag) {
				return model.CommandResult{}, &containerError{"镜像标签无效"}
			}
			args = append(args, "-t", tag)
		}
		args = append(args, contextDir)
		return runDocker(r, args...)
	case "image/save":
		if image == "" {
			image = body.TagName
		}
		if image == "" || !validDockerIdentifier(image) {
			return model.CommandResult{}, &containerError{"镜像名称无效"}
		}
		path := strings.TrimSpace(body.Path)
		if path != "" && body.Name != "" {
			path = filepath.Join(path, body.Name)
		}
		if !validDockerPath(path) || strings.Contains(path, "..") {
			return model.CommandResult{}, &containerError{"镜像导出路径无效"}
		}
		return runDocker(r, "save", "-o", path, image)
	case "image/load":
		paths := append([]string(nil), body.Paths...)
		if len(paths) == 0 && body.Path != "" {
			paths = []string{body.Path}
		}
		if len(paths) == 0 {
			return model.CommandResult{}, &containerError{"镜像导入路径不能为空"}
		}
		var last model.CommandResult
		for _, path := range paths {
			if !validDockerPath(path) || strings.Contains(path, "..") {
				return model.CommandResult{}, &containerError{"镜像导入路径无效"}
			}
			var loadErr error
			last, loadErr = runDocker(r, "load", "-i", path)
			if loadErr != nil || last.ExitCode != 0 {
				return last, loadErr
			}
		}
		return last, nil
	default:
		return model.CommandResult{}, unsupportedContainerOperation(path)
	}
}

// containerRepository resolves repository credentials from the persistent
// container store.  Credentials never leave this process in a response.
// containerRepository 从持久化容器状态解析仓库凭据。
func containerRepository(value any) (*imageRepository, error) {
	id := strings.TrimSpace(fmt.Sprint(value))
	if id == "" || id == "0" || id == "<nil>" {
		return nil, nil
	}
	store := getContainerStore()
	store.mu.RLock()
	defer store.mu.RUnlock()
	for _, item := range store.state.Repositories {
		if item.ID == fmt.Sprint(id) {
			copy := item
			return &copy, nil
		}
	}
	return nil, &containerError{"镜像仓库不存在"}
}

// validBuildArgument 校验 Docker build-arg 的键值格式和长度。
func validBuildArgument(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	key, _, ok := strings.Cut(value, "=")
	return ok && validEnvKey(strings.TrimSpace(key)) && len(value) <= 2048
}

// normalizeRegistryReference 规范化并校验 registry 地址。
func normalizeRegistryReference(value string) (string, error) {
	raw := strings.TrimSpace(value)
	if raw == "" || strings.ContainsAny(raw, "\x00\r\n \t;&|`$<>") {
		return "", &containerError{"镜像仓库地址无效"}
	}
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return "", &containerError{"镜像仓库地址无效"}
		}
		raw = parsed.Host + strings.TrimSuffix(parsed.Path, "/")
	}
	raw = strings.TrimSuffix(raw, "/")
	if !validDockerIdentifier(raw) || strings.HasPrefix(raw, "/") || strings.HasSuffix(raw, "/") {
		return "", &containerError{"镜像仓库地址无效"}
	}
	return raw, nil
}

// handleDockerResourceOperation 处理网络、卷和编排资源操作，校验资源名称后执行。
func handleDockerResourceOperation(r *http.Request, resource, path, name string) (model.CommandResult, error) {
	switch {
	case path == resource:
		if !validDockerIdentifier(name) {
			return model.CommandResult{}, &containerError{resource + "名称不能为空"}
		}
		return runDocker(r, resource, "create", name)
	case path == resource+"/del":
		if !validDockerIdentifier(name) {
			return model.CommandResult{}, &containerError{resource + "名称不能为空"}
		}
		return runDocker(r, resource, "rm", name)
	default:
		return runDocker(r, resource, "ls")
	}
}

// runDocker 通过参数数组执行 Docker CLI，并应用超时和输出上限。
func runDocker(r *http.Request, args ...string) (model.CommandResult, error) {
	// 统一使用 Docker CLI 路径发现逻辑，避免服务进程 PATH 精简导致 stop/start 失败。
	return (service.CommandService{}).Execute(r.Context(), model.CommandRequest{Program: service.DockerBinary(), Args: args, Timeout: 5 * time.Minute})
}

// runDockerWithStdin feeds sensitive values through stdin instead of exposing
// them in argv or a shell command line.
// runDockerWithStdin 通过标准输入传递敏感数据并执行 Docker 命令。
func runDockerWithStdin(r *http.Request, args []string, input string) (model.CommandResult, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, service.DockerBinary(), args...)
	command.Stdin = strings.NewReader(input)
	var stdout, stderr limitedDockerBuffer
	stdout.limit, stderr.limit = 1<<20, 1<<20
	command.Stdout, command.Stderr = &stdout, &stderr
	started := time.Now()
	err := command.Run()
	result := model.CommandResult{ExitCode: 0, Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(started).Milliseconds()}
	if err != nil {
		result.ExitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return result, context.DeadlineExceeded
		}
		return result, err
	}
	if stdout.exceeded || stderr.exceeded {
		return result, errors.New("Docker 命令输出超过限制")
	}
	return result, nil
}

type limitedDockerBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

// Write 将 Docker 输出限制在固定大小以内，避免异常命令耗尽内存。
func (b *limitedDockerBuffer) Write(data []byte) (int, error) {
	if b.limit > 0 && b.Len()+len(data) > b.limit {
		b.exceeded = true
		return len(data), io.ErrShortBuffer
	}
	return b.Buffer.Write(data)
}

// daemonJSONPath 返回受数据目录约束的 Docker daemon 配置路径。
func daemonJSONPath() string {
	if p := strings.TrimSpace(os.Getenv("WORKMESH_DOCKER_DAEMON_JSON")); p != "" {
		return p
	}
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = "./data"
	}
	return filepath.Join(dir, "docker-daemon.json")
}

// handleDaemonJSON 读取或更新 daemon.json，并使用原子文件替换保存配置。
func handleDaemonJSON(w http.ResponseWriter, _ *http.Request, fileOnly bool) {
	path := daemonJSONPath()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		b = []byte("{}")
	} else if err != nil {
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if fileOnly {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": string(b)})
		return
	}
	var raw map[string]any
	_ = json.Unmarshal(b, &raw)
	conf := map[string]any{
		"isSwarm": false, "version": "-", "registryMirrors": stringSlice(raw["registry-mirrors"]), "insecureRegistries": stringSlice(raw["insecure-registries"]),
		"liveRestore": boolValue(raw, "live-restore"), "iptables": true, "cgroupDriver": "cgroupfs", "ipv6": boolValue(raw, "ipv6"),
		"fixedCidrV6": valueString(raw, "fixed-cidr-v6"), "ip6Tables": boolValue(raw, "ip6tables"), "experimental": boolValue(raw, "experimental"),
	}
	if value, ok := raw["iptables"].(bool); ok {
		conf["iptables"] = value
	}
	if opts, ok := raw["exec-opts"].([]any); ok {
		for _, opt := range opts {
			if text, ok := opt.(string); ok && strings.HasPrefix(text, "native.cgroupdriver=") {
				conf["cgroupDriver"] = strings.TrimPrefix(text, "native.cgroupdriver=")
			}
		}
	}
	if logs, ok := raw["log-opts"].(map[string]any); ok {
		conf["logMaxSize"], conf["logMaxFile"] = valueString(logs, "max-size"), valueString(logs, "max-file")
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": conf})
}

// stringSlice 将 JSON 数组安全转换为字符串切片并过滤空值。
func stringSlice(value any) []string {
	result := []string{}
	if values, ok := value.([]any); ok {
		for _, item := range values {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
	}
	return result
}

// updateDaemonJSON 校验 daemon 配置字段后原子写入真实 Docker 配置文件。
func updateDaemonJSON(r *http.Request) (model.CommandResult, error) {
	var req struct {
		Content string `json:"content"`
		File    string `json:"file"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil && !strings.Contains(err.Error(), "EOF") {
			return model.CommandResult{}, err
		}
	}
	content := req.Content
	if content == "" {
		content = req.File
	}
	if content == "" {
		content = "{}"
	}
	var parsed any
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return model.CommandResult{}, &containerError{"daemon.json 内容不是有效 JSON"}
	}
	path := daemonJSONPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return model.CommandResult{}, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return model.CommandResult{}, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return model.CommandResult{}, err
	}
	return model.CommandResult{ExitCode: 0, Stdout: path}, nil
}
