// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// registerContainerRoutes 注册低开销 Docker 适配器。所有参数作为独立 argv 传递，不经过 shell。
func registerContainerRoutes(mux *http.ServeMux) {
	docker := service.NewDockerService()
	mux.HandleFunc("/api/v2/containers/", func(w http.ResponseWriter, r *http.Request) {
		handleContainerRequest(docker, w, r)
	})
	mux.HandleFunc("POST /api/v2/containers", func(w http.ResponseWriter, r *http.Request) {
		handleContainerRequest(docker, w, r)
	})
	mux.HandleFunc("POST /api/v2/containers/compose/search", handleComposeSearch)
	mux.HandleFunc("POST /api/v2/containers/compose/test", handleComposeTest)
	mux.HandleFunc("POST /api/v2/containers/compose/operate", handleComposeOperate)
	mux.HandleFunc("POST /api/v2/containers/compose", handleComposeOperate)
	mux.HandleFunc("POST /api/v2/containers/compose/update", handleComposeOperate)
	mux.HandleFunc("POST /api/v2/containers/compose/pin", handleComposeOperate)
	mux.HandleFunc("POST /api/v2/containers/compose/env", handleComposeEnv)
	mux.HandleFunc("POST /api/v2/containers/compose/clean/log", handleComposeCleanLog)
}

type composeRequest struct {
	Path      string   `json:"path"`
	Operation string   `json:"operation"`
	Services  []string `json:"services"`
}

var dockerIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:@/-]{0,255}$`)

func validDockerIdentifier(value string) bool {
	return dockerIdentifier.MatchString(strings.TrimSpace(value)) && !strings.ContainsAny(value, "\x00\r\n")
}

func validDockerPath(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.ContainsAny(value, "\x00\r\n") && len(value) <= 4096
}

func composeCommand(r *http.Request, req composeRequest, op string) (model.CommandResult, error) {
	if !validDockerPath(req.Path) {
		return model.CommandResult{}, &containerError{"Compose 文件路径不能为空"}
	}
	allowed := map[string]bool{"up": true, "down": true, "start": true, "stop": true, "restart": true, "ps": true, "config": true, "pull": true}
	if !allowed[op] {
		return model.CommandResult{}, &containerError{"不支持的 Compose 操作"}
	}
	args := []string{"compose", "-f", req.Path, op}
	for _, service := range req.Services {
		if !validDockerIdentifier(service) {
			return model.CommandResult{}, &containerError{"Compose 服务名称无效"}
		}
		args = append(args, service)
	}
	return runDocker(r, args...)
}
func handleComposeSearch(w http.ResponseWriter, r *http.Request) {
	var req composeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	result, err := composeCommand(r, req, "ps")
	writeCommandResult(w, result, err)
}
func handleComposeTest(w http.ResponseWriter, r *http.Request) {
	var req composeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	result, err := composeCommand(r, req, "config")
	writeCommandResult(w, result, err)
}
func handleComposeOperate(w http.ResponseWriter, r *http.Request) {
	var req composeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	result, err := composeCommand(r, req, req.Operation)
	writeCommandResult(w, result, err)
}

func handleComposeEnv(w http.ResponseWriter, r *http.Request) {
	var req composeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	result, err := composeCommand(r, req, "config")
	writeCommandResult(w, result, err)
}

func handleComposeCleanLog(w http.ResponseWriter, _ *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"cleaned": true}})
}

func isContainerRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return path == "/api/v2/containers" || strings.HasPrefix(path, "/api/v2/containers/")
}

func handleContainerRequest(docker service.DockerService, w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/containers")
	path = strings.Trim(path, "/")
	var result model.CommandResult
	var err error

	switch {
	case r.Method == http.MethodGet && (path == "docker/status" || path == "status"):
		result, err = docker.Status(r.Context())
	case r.Method == http.MethodGet && (path == "list" || path == "list/stats"):
		result, err = docker.List(r.Context())
	case r.Method == http.MethodGet && strings.HasPrefix(path, "stats/"):
		id := strings.TrimPrefix(path, "stats/")
		if !validDockerIdentifier(id) {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "容器标识无效"})
			return
		}
		result, err = runDocker(r, "stats", "--no-stream", id)
	case r.Method == http.MethodGet && (path == "image" || path == "image/all"):
		result, err = runDocker(r, "images", "--no-trunc")
	case r.Method == http.MethodGet && path == "network":
		result, err = runDocker(r, "network", "ls")
	case r.Method == http.MethodGet && path == "volume":
		result, err = runDocker(r, "volume", "ls")
	case r.Method == http.MethodGet && path == "network/search":
		result, err = runDocker(r, "network", "ls")
	case r.Method == http.MethodGet && path == "volume/search":
		result, err = runDocker(r, "volume", "ls")
	case r.Method == http.MethodGet && (path == "daemonjson" || path == "daemonjson/file"):
		handleDaemonJSON(w, r)
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
	writeCommandResult(w, result, err)
}

func handleContainerPost(docker service.DockerService, r *http.Request, path string) (model.CommandResult, error) {
	var body struct {
		Container  string `json:"container"`
		ID         string `json:"id"`
		Operation  string `json:"operation"`
		Image      string `json:"image"`
		Name       string `json:"name"`
		Repository string `json:"repository"`
		Tag        string `json:"tag"`
		Content    string `json:"content"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&body); err != nil && !strings.Contains(err.Error(), "EOF") {
			return model.CommandResult{}, err
		}
	}
	container := body.Container
	if container == "" {
		container = body.ID
	}
	switch {
	case path == "operate" || path == "docker/operate":
		return docker.Operate(r.Context(), model.DockerOperationRequest{Container: container, Operation: body.Operation})
	case path == "inspect" || path == "info":
		if container == "" {
			return model.CommandResult{}, errContainerParameter
		}
		return runDocker(r, "inspect", container)
	case path == "prune":
		return runDocker(r, "system", "prune", "-f")
	case path == "clean/log" || path == "download/log":
		return model.CommandResult{ExitCode: 0, Stdout: "", Stderr: ""}, nil
	case path == "daemonjson" || path == "daemonjson/update" || path == "daemonjson/update/byfile":
		return updateDaemonJSON(r)
	case strings.HasPrefix(path, "image/"):
		return handleImageOperation(r, path, body)
	case strings.HasPrefix(path, "network"):
		return handleDockerResourceOperation(r, "network", path, body.Name)
	case strings.HasPrefix(path, "volume"):
		return handleDockerResourceOperation(r, "volume", path, body.Name)
	default:
		// Compose、模板和仓库等路径保留明确的可观测错误，避免伪造执行成功。
		return model.CommandResult{}, unsupportedContainerOperation(path)
	}
}

var errContainerParameter = &containerError{"容器标识不能为空"}

type containerError struct{ message string }

func (e *containerError) Error() string { return e.message }

func unsupportedContainerOperation(path string) error {
	return &containerError{"容器操作暂未接入 Docker 驱动: " + path}
}

func handleImageOperation(r *http.Request, path string, body struct {
	Container  string `json:"container"`
	ID         string `json:"id"`
	Operation  string `json:"operation"`
	Image      string `json:"image"`
	Name       string `json:"name"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Content    string `json:"content"`
}) (model.CommandResult, error) {
	image := body.Image
	if image == "" {
		image = body.Name
	}
	switch path {
	case "image/pull":
		if !validDockerIdentifier(image) {
			return model.CommandResult{}, &containerError{"镜像名称不能为空"}
		}
		return runDocker(r, "pull", image)
	case "image/remove":
		if !validDockerIdentifier(image) {
			return model.CommandResult{}, &containerError{"镜像名称不能为空"}
		}
		return runDocker(r, "rmi", image)
	case "image/search":
		if !validDockerIdentifier(image) {
			return model.CommandResult{}, &containerError{"镜像关键词不能为空"}
		}
		return runDocker(r, "search", image)
	case "image/tag":
		if image == "" || !validDockerIdentifier(body.Repository) || !validDockerIdentifier(body.Tag) {
			return model.CommandResult{}, &containerError{"镜像名称或标签无效"}
		}
		return runDocker(r, "tag", image, body.Repository+":"+body.Tag)
	case "image/push":
		if image == "" || !validDockerIdentifier(image) {
			return model.CommandResult{}, &containerError{"镜像名称无效"}
		}
		return runDocker(r, "push", image)
	case "image/build":
		if !validDockerPath(body.Name) || !validDockerIdentifier(image) {
			return model.CommandResult{}, &containerError{"构建目录或镜像名称无效"}
		}
		return runDocker(r, "build", "-t", image, body.Name)
	case "image/save":
		if image == "" || !validDockerIdentifier(image) {
			return model.CommandResult{}, &containerError{"镜像名称无效"}
		}
		return runDocker(r, "save", image)
	case "image/load":
		return runDocker(r, "load")
	default:
		return model.CommandResult{}, unsupportedContainerOperation(path)
	}
}

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

func runDocker(r *http.Request, args ...string) (model.CommandResult, error) {
	return (service.CommandService{}).Execute(r.Context(), model.CommandRequest{Program: "docker", Args: args})
}

func daemonJSONPath() string {
	if p := strings.TrimSpace(os.Getenv("WORKMESH_DOCKER_DAEMON_JSON")); p != "" {
		return p
	}
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = ".workmesh-data"
	}
	return filepath.Join(dir, "docker-daemon.json")
}

func handleDaemonJSON(w http.ResponseWriter, _ *http.Request) {
	path := daemonJSONPath()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		b = []byte("{}")
	} else if err != nil {
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": path, "content": string(b)}})
}

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
