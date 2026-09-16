// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

const (
	databaseRuntimeDefaultComposeName = "docker-compose.yml"
	databaseRuntimeDefaultDataDir     = "./data"
)

var databaseRuntimeContainerNames = []string{
	"workmesh-panel-postgres",
	"workmesh-panel-redis",
	"workmesh-panel-mariadb",
}

type databaseRuntimeCommandExecutor interface {
	Execute(context.Context, model.CommandRequest) (model.CommandResult, error)
}

var databaseRuntimeCommands databaseRuntimeCommandExecutor = service.CommandService{}

type databaseRuntimeRequest struct {
	Path string `json:"path"`
}

type databaseRuntimeResponse struct {
	Operation          string                             `json:"operation"`
	Path               string                             `json:"path"`
	Containers         []service.DatabaseRuntimeContainer `json:"containers"`
	ExpectedContainers []string                           `json:"expectedContainers,omitempty"`
	ConfigValidated    bool                               `json:"configValidated,omitempty"`
	Output             string                             `json:"output,omitempty"`
	Stdout             string                             `json:"stdout,omitempty"`
	Stderr             string                             `json:"stderr,omitempty"`
	ExitCode           int                                `json:"exitCode"`
}

func registerDatabaseRuntimeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/databases/runtime/status", handleDatabaseRuntimeStatus)
	mux.HandleFunc("POST /api/v2/databases/runtime/status", handleDatabaseRuntimeStatus)
	mux.HandleFunc("POST /api/v2/databases/runtime/{operation}", handleDatabaseRuntimeOperation)
}

func handleDatabaseRuntimeStatus(w http.ResponseWriter, r *http.Request) {
	req, err := decodeDatabaseRuntimeRequest(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	runtimePath, err := resolveDatabaseRuntimeComposePath(req.Path)
	if err != nil {
		writeDatabaseRuntimeError(w, err)
		return
	}
	if _, result, err := validateDatabaseRuntimeCompose(r.Context(), runtimePath); err != nil {
		writeDatabaseRuntimeCommandError(w, result, err)
		return
	}
	psResult, err := executeDatabaseRuntimeCompose(r.Context(), runtimePath, "ps")
	if err != nil {
		writeDatabaseRuntimeCommandError(w, psResult, err)
		return
	}
	containers, parseErr := service.ParseDatabaseRuntimeComposePS([]byte(psResult.Stdout))
	inspectResult, inspectErr := executeDatabaseRuntimeInspect(r.Context())
	if inspectErr == nil || strings.TrimSpace(inspectResult.Stdout) != "" {
		inspectContainers, err := service.ParseDatabaseRuntimeInspect([]byte(inspectResult.Stdout))
		if err == nil {
			containers = service.MergeDatabaseRuntimeContainers(containers, inspectContainers)
			if parseErr == nil || strings.TrimSpace(inspectResult.Stdout) != "" {
				parseErr = nil
			}
		} else if parseErr != nil {
			parseErr = err
		}
	}
	if parseErr != nil {
		if inspectErr != nil {
			parseErr = fmt.Errorf("%v; inspect: %w", parseErr, inspectErr)
		}
		writeDatabaseRuntimeCommandError(w, psResult, parseErr)
		return
	}
	containers, err = service.CompleteDatabaseRuntimeContainers(containers, databaseRuntimeContainerNames)
	if err != nil {
		writeDatabaseRuntimeCommandError(w, inspectResult, err)
		return
	}
	observedAt := time.Now().UTC()
	for index := range containers {
		containers[index].UpdatedAt = observedAt
	}
	if err := persistDatabaseRuntimeStates(r.Context(), containers, observedAt); err != nil {
		writeDatabaseRuntimeError(w, err)
		return
	}
	stderr := strings.TrimSpace(psResult.Stderr)
	if strings.TrimSpace(inspectResult.Stderr) != "" {
		stderr = strings.TrimSpace(strings.Join([]string{stderr, inspectResult.Stderr}, "\n"))
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{
		"code": 200,
		"data": databaseRuntimeResponse{
			Operation:          "status",
			Path:               runtimePath,
			Containers:         containers,
			ExpectedContainers: append([]string(nil), databaseRuntimeContainerNames...),
			ConfigValidated:    true,
			Output:             psResult.Stdout,
			Stdout:             psResult.Stdout,
			Stderr:             stderr,
			ExitCode:           psResult.ExitCode,
		},
	})
}

func handleDatabaseRuntimeOperation(w http.ResponseWriter, r *http.Request) {
	operation := strings.ToLower(strings.TrimSpace(r.PathValue("operation")))
	if !databaseRuntimeOperationAllowed(operation) {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "不支持的数据库 Compose 操作"})
		return
	}
	req, err := decodeDatabaseRuntimeRequest(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	runtimePath, err := resolveDatabaseRuntimeComposePath(req.Path)
	if err != nil {
		writeDatabaseRuntimeError(w, err)
		return
	}
	var result model.CommandResult
	configValidated := false
	if operation == "config" {
		_, result, err = validateDatabaseRuntimeCompose(r.Context(), runtimePath)
		if err != nil {
			writeDatabaseRuntimeCommandError(w, result, err)
			return
		}
		configValidated = true
	} else if operation == "up" || operation == "stop" || operation == "restart" {
		_, configResult, configErr := validateDatabaseRuntimeCompose(r.Context(), runtimePath)
		if configErr != nil {
			writeDatabaseRuntimeCommandError(w, configResult, configErr)
			return
		}
		configValidated = true
		result, err = executeDatabaseRuntimeCompose(r.Context(), runtimePath, operation)
	}
	if err != nil {
		writeDatabaseRuntimeCommandError(w, result, err)
		return
	}
	containers := databaseRuntimeUnobservedContainers()
	output := result.Stdout
	stdout := result.Stdout
	if operation == "config" {
		// Compose config may contain expanded environment values. Do not return
		// the rendered document through the HTTP response.
		output = "configuration valid"
		stdout = ""
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{
		"code": 200,
		"data": databaseRuntimeResponse{
			Operation:          operation,
			Path:               runtimePath,
			Containers:         containers,
			ExpectedContainers: append([]string(nil), databaseRuntimeContainerNames...),
			ConfigValidated:    configValidated,
			Output:             output,
			Stdout:             stdout,
			Stderr:             result.Stderr,
			ExitCode:           result.ExitCode,
		},
	})
}

func decodeDatabaseRuntimeRequest(r *http.Request) (databaseRuntimeRequest, error) {
	var req databaseRuntimeRequest
	if r.Body == nil {
		return req, nil
	}
	err := decodeSingleJSON(r.Body, &req, 1<<20)
	if err != nil {
		// An empty POST body means the default Compose path, matching GET status.
		if errors.Is(err, io.EOF) {
			return req, nil
		}
		return req, errors.New("数据库运行时请求体无效")
	}
	return req, nil
}

func databaseRuntimeOperationAllowed(operation string) bool {
	switch operation {
	case "config", "up", "stop", "restart":
		return true
	default:
		return false
	}
}

func databaseRuntimeUnobservedContainers() []service.DatabaseRuntimeContainer {
	containers := make([]service.DatabaseRuntimeContainer, 0, len(databaseRuntimeContainerNames))
	for _, name := range databaseRuntimeContainerNames {
		containers = append(containers, service.DatabaseRuntimeContainer{Name: name, Status: "not_observed"})
	}
	return containers
}

func resolveDatabaseRuntimeComposePath(requested string) (string, error) {
	dataDir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dataDir == "" {
		dataDir = databaseRuntimeDefaultDataDir
	}
	root, err := filepath.Abs(filepath.Join(dataDir, "database"))
	if err != nil {
		return "", errors.New("数据库 Compose 根目录无效")
	}
	root = filepath.Clean(root)
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		return "", errors.New("数据库 Compose 根目录不存在")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", errors.New("数据库 Compose 根目录不可用")
	}

	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = databaseRuntimeDefaultComposeName
	}
	candidate := requested
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil || !databaseRuntimePathWithin(root, candidate) {
		return "", errors.New("Compose 路径必须位于 WORKMESH_DATA_DIR/database 下")
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return "", errors.New("数据库 Compose 文件不存在")
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("数据库 Compose 路径不是文件")
	}
	realCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil || !databaseRuntimePathWithin(realRoot, realCandidate) {
		return "", errors.New("Compose 路径必须位于 WORKMESH_DATA_DIR/database 下")
	}
	return candidate, nil
}

func databaseRuntimePathWithin(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	return candidate == root || strings.HasPrefix(candidate, root+string(os.PathSeparator))
}

func executeDatabaseRuntimeCompose(ctx context.Context, composePath, operation string) (model.CommandResult, error) {
	args := []string{"compose", "-f", composePath}
	switch operation {
	case "config":
		args = append(args, "config", "--format", "json")
	case "ps":
		args = append(args, "ps", "--all", "--format", "json")
	case "stop", "restart":
		args = append(args, operation)
	case "up":
		args = append(args, "up", "-d")
	default:
		return model.CommandResult{}, errors.New("不支持的数据库 Compose 操作")
	}
	result, err := databaseRuntimeCommands.Execute(ctx, model.CommandRequest{
		Program: service.DockerBinary(),
		Args:    args,
		Dir:     filepath.Dir(composePath),
		Timeout: 5 * time.Minute,
	})
	if err != nil {
		return result, err
	}
	if result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = fmt.Sprintf("Docker Compose 命令退出码为 %d", result.ExitCode)
		}
		return result, errors.New(message)
	}
	return result, nil
}

func validateDatabaseRuntimeCompose(ctx context.Context, composePath string) (bool, model.CommandResult, error) {
	result, err := executeDatabaseRuntimeCompose(ctx, composePath, "config")
	if err != nil {
		return false, result, err
	}
	if err := service.ValidateDatabaseRuntimeComposeConfig([]byte(result.Stdout), databaseRuntimeContainerNames); err != nil {
		return false, result, err
	}
	return true, result, nil
}

func executeDatabaseRuntimeInspect(ctx context.Context) (model.CommandResult, error) {
	args := append([]string{"inspect", "--type", "container"}, databaseRuntimeContainerNames...)
	result, err := databaseRuntimeCommands.Execute(ctx, model.CommandRequest{
		Program: service.DockerBinary(),
		Args:    args,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return result, err
	}
	if result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = fmt.Sprintf("Docker inspect 命令退出码为 %d", result.ExitCode)
		}
		return result, errors.New(message)
	}
	return result, nil
}

func persistDatabaseRuntimeStates(ctx context.Context, containers []service.DatabaseRuntimeContainer, observedAt time.Time) error {
	repository, repositoryErr := sharedRuntimeRepository()
	if repositoryErr != nil {
		return nil
	}
	states := make([]service.DatabaseRuntimeState, 0, len(containers))
	for _, container := range containers {
		states = append(states, service.DatabaseRuntimeState{
			ContainerName: container.Name,
			Status:        container.Status,
			Health:        container.Health,
			Image:         container.Image,
			Ports:         container.Ports,
			Error:         container.Error,
			ExitCode:      container.ExitCode,
			ObservedAt:    observedAt,
		})
	}
	store := service.SQLiteDatabaseRuntimeStateStore{Repository: repository}
	if err := store.UpsertDatabaseRuntimeStates(ctx, states); err != nil {
		return fmt.Errorf("保存数据库容器运行状态失败: %w", err)
	}
	return nil
}

func writeDatabaseRuntimeError(w http.ResponseWriter, err error) {
	wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
}

func writeDatabaseRuntimeCommandError(w http.ResponseWriter, result model.CommandResult, err error) {
	message := strings.TrimSpace(result.Stderr)
	if message == "" {
		message = strings.TrimSpace(err.Error())
	}
	wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{
		"code":    "ERR",
		"message": message,
		"data": map[string]any{
			"stdout":   result.Stdout,
			"stderr":   result.Stderr,
			"exitCode": result.ExitCode,
		},
	})
}
