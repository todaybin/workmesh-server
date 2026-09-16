// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type containerRequest struct {
	Container     string     `json:"container"`
	ContainerID   string     `json:"containerID"`
	ID            string     `json:"id"`
	Operation     string     `json:"operation"`
	Image         string     `json:"image"`
	Name          string     `json:"name"`
	Repository    string     `json:"repository"`
	Tag           string     `json:"tag"`
	RepoID        any        `json:"repoID"`
	ImageNames    stringList `json:"imageName"`
	SourceID      string     `json:"sourceID"`
	Tags          []string   `json:"tags"`
	Username      string     `json:"username"`
	Password      string     `json:"password"`
	Content       string     `json:"content"`
	From          string     `json:"from"`
	Path          string     `json:"path"`
	HostPath      string     `json:"hostPath"`
	Args          []string   `json:"args"`
	Env           envValues  `json:"env"`
	Command       []string   `json:"command"`
	CPU           string     `json:"cpu"`
	Memory        string     `json:"memory"`
	Paths         []string   `json:"paths"`
	Names         []string   `json:"names"`
	PruneType     string     `json:"pruneType"`
	TaskID        string     `json:"taskID"`
	Since         string     `json:"since"`
	Tail          int        `json:"tail"`
	Timestamp     bool       `json:"timestamp"`
	ContainerType string     `json:"containerType"`
	Compose       string     `json:"compose"`
	WithTagAll    bool       `json:"withTagAll"`
	Dockerfile    string     `json:"dockerfile"`
	TagName       string     `json:"tagName"`
}

// stringList 接受 1Panel 历史请求中的单值和当前请求中的数组格式。
type stringList []string

func (s *stringList) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*s = nil
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*s = list
		return nil
	}
	var single string
	if err := json.Unmarshal(data, &single); err != nil {
		return fmt.Errorf("字符串列表格式无效")
	}
	*s = []string{single}
	return nil
}

// envValues 兼容前端常见的对象和 KEY=VALUE 数组两种环境变量格式。
type envValues map[string]string

func (e *envValues) UnmarshalJSON(data []byte) error {
	var object map[string]string
	if err := json.Unmarshal(data, &object); err == nil {
		*e = object
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	result := make(envValues, len(list))
	for _, item := range list {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 || !validEnvKey(strings.TrimSpace(parts[0])) || strings.ContainsAny(parts[1], "\x00\r\n") {
			return fmt.Errorf("环境变量格式无效: %q", item)
		}
		result[strings.TrimSpace(parts[0])] = parts[1]
	}
	*e = result
	return nil
}

// imageRepository 是镜像仓库配置；密码仅用于受控登录，接口返回时始终脱敏。
type imageRepository struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	DownloadURL string    `json:"downloadUrl,omitempty"`
	Protocol    string    `json:"protocol,omitempty"`
	Username    string    `json:"username,omitempty"`
	Password    string    `json:"password,omitempty"`
	Auth        bool      `json:"auth"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// composeTemplate 保存用户维护的 Compose 模板正文。
type composeTemplate struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type composeRecord struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	AppInstallID string    `json:"appInstallId,omitempty"`
	Pinned       bool      `json:"pinned"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type containerState struct {
	Repositories []imageRepository `json:"repositories"`
	Templates    []composeTemplate `json:"templates"`
	Composes     []composeRecord   `json:"composes"`
	Settings     map[string]any    `json:"settings"`
}

type containerStore struct {
	mu    sync.RWMutex
	path  string
	state containerState
}

func cloneContainerState(source containerState) containerState {
	payload, err := json.Marshal(source)
	if err != nil {
		return source
	}
	var clone containerState
	if err := json.Unmarshal(payload, &clone); err != nil {
		return source
	}
	return clone
}

var containerStoreMu sync.Mutex
var containerStoreInstance *containerStore

func getContainerStore() *containerStore {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	path := filepath.Join(root, "containers.json")
	containerStoreMu.Lock()
	defer containerStoreMu.Unlock()
	if containerStoreInstance != nil && containerStoreInstance.path == path {
		return containerStoreInstance
	}
	s := &containerStore{path: path, state: containerState{Repositories: []imageRepository{}, Templates: []composeTemplate{}, Composes: []composeRecord{}, Settings: map[string]any{}}}
	// 生产进程始终由共享 SQLite 提供状态；旧 containers.json 只允许在
	// 未接入 SQLite 的迁移/单元测试场景读取，避免陈旧文件覆盖真实数据库状态。
	if sharedDB() == nil {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			_ = json.Unmarshal(b, &s.state)
		}
	} else {
		_ = loadJSONState("container_store_state", &s.state)
	}
	if s.state.Repositories == nil {
		s.state.Repositories = []imageRepository{}
	}
	if s.state.Templates == nil {
		s.state.Templates = []composeTemplate{}
	}
	if s.state.Composes == nil {
		s.state.Composes = []composeRecord{}
	}
	if s.state.Settings == nil {
		s.state.Settings = map[string]any{}
	}
	var appState appStoreState
	if loadJSONState("app_store_state", &appState) {
		installByComposePath := make(map[string]string, len(appState.Apps))
		for _, app := range appState.Apps {
			installByComposePath[filepath.Clean(appComposePath(app))] = app.ID
		}
		migrated := false
		for index := range s.state.Composes {
			if s.state.Composes[index].AppInstallID != "" {
				continue
			}
			if installID := installByComposePath[filepath.Clean(s.state.Composes[index].Path)]; installID != "" {
				s.state.Composes[index].AppInstallID = installID
				migrated = true
			}
		}
		if migrated {
			_ = s.saveLocked()
		}
	}
	containerStoreInstance = s
	return s
}

func (s *containerStore) saveLocked() error {
	// 公共 SQLite 可用时停止写入重复 containers.json；该文件只允许作为一次性迁移输入。
	if sharedDB() == nil {
		if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
			return err
		}
		b, err := json.Marshal(s.state)
		if err != nil {
			return err
		}
		tmp := s.path + ".tmp"
		if err := os.WriteFile(tmp, b, 0o600); err != nil {
			return err
		}
		if err := os.Rename(tmp, s.path); err != nil {
			return err
		}
		return nil
	}
	if err := saveJSONState("container_store_state", s.state); err != nil {
		return err
	}
	return persistContainerRelational(s.state)
}

func sanitizeImageRepository(r imageRepository) imageRepository { r.Password = ""; return r }

// registerContainerRoutes 注册低开销 Docker 适配器。所有参数作为独立 argv 传递，不经过 shell。
func registerContainerRoutes(mux *http.ServeMux) {
	docker := service.NewDockerService()
	store := getContainerStore()
	mux.HandleFunc("/api/v2/containers/", func(w http.ResponseWriter, r *http.Request) {
		handleContainerRequest(docker, w, r)
	})
	// Explicit high-frequency registrations keep the public contract visible to
	// static tooling while sharing the same authenticated dispatcher.
	for _, route := range []string{
		"GET /api/v2/containers/daemonjson",
		"GET /api/v2/containers/daemonjson/file",
		"GET /api/v2/containers/search/log",
		"POST /api/v2/containers/download/log",
		"POST /api/v2/containers/files/content",
		"POST /api/v2/containers/files/download",
		"POST /api/v2/containers/files/search",
		"POST /api/v2/containers/files/size",
	} {
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) { handleContainerRequest(docker, w, r) })
	}
	mux.HandleFunc("POST /api/v2/containers", func(w http.ResponseWriter, r *http.Request) {
		handleContainerRequest(docker, w, r)
	})
	mux.HandleFunc("POST /api/v2/containers/compose/search", handleComposeSearch)
	mux.HandleFunc("GET /api/v2/containers/compose", func(w http.ResponseWriter, _ *http.Request) {
		store.mu.RLock()
		items := append([]composeRecord(nil), store.state.Composes...)
		store.mu.RUnlock()
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
	})
	mux.HandleFunc("POST /api/v2/containers/compose/test", handleComposeTest)
	mux.HandleFunc("POST /api/v2/containers/compose/operate", handleComposeOperate)
	mux.HandleFunc("POST /api/v2/containers/compose", func(w http.ResponseWriter, r *http.Request) { handleComposeCreate(w, r, store) })
	mux.HandleFunc("POST /api/v2/containers/compose/update", func(w http.ResponseWriter, r *http.Request) { handleComposeUpdate(w, r, store) })
	mux.HandleFunc("POST /api/v2/containers/compose/pin", func(w http.ResponseWriter, r *http.Request) { handleComposePin(w, r, store) })
	mux.HandleFunc("POST /api/v2/containers/compose/env", handleComposeEnv)
	mux.HandleFunc("POST /api/v2/containers/compose/clean/log", handleComposeCleanLog)
	mux.HandleFunc("GET /api/v2/containers/stats/{id}", handleContainerStats)
	registerContainerRepositoryRoutes(mux, store)
	registerContainerTemplateRoutes(mux, store)
	registerContainerSettingsRoutes(mux, store)
}

func handleContainerStats(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validDockerIdentifier(id) {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "容器标识无效"})
		return
	}
	result, err := runDocker(r, "stats", "--no-stream", "--format", "{{json .}}", id)
	if err != nil || result.ExitCode != 0 {
		if err == nil {
			err = errors.New(result.Stderr)
		}
		status := http.StatusInternalServerError
		if isDockerUnavailable(result, err) {
			status = http.StatusServiceUnavailable
		}
		wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &raw); err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	memory, cache := parseDockerMemoryUsage(valueString(raw, "MemUsage"))
	ioRead, ioWrite := parsePairBytes(valueString(raw, "BlockIO"))
	rx, tx := parsePairBytes(valueString(raw, "NetIO"))
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"cpuPercent": parseDockerPercent(valueString(raw, "CPUPerc")), "memory": float64(memory), "cache": float64(cache), "ioRead": ioRead, "ioWrite": ioWrite, "networkRX": rx, "networkTX": tx, "shotTime": time.Now().UTC()}})
}

func parsePairBytes(value string) (float64, float64) {
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	return float64(parseDockerBytes(strings.TrimSpace(parts[0]))) / 1024 / 1024, float64(parseDockerBytes(strings.TrimSpace(parts[1]))) / 1024 / 1024
}

type composeRequest struct {
	Path      string   `json:"path"`
	Operation string   `json:"operation"`
	Services  []string `json:"services"`
	Name      string   `json:"name"`
	DirName   string   `json:"dirName"`
	From      string   `json:"from"`
	File      string   `json:"file"`
	Content   string   `json:"content"`
	Env       string   `json:"env"`
	Template  string   `json:"template"`
	IsPinned  bool     `json:"isPinned"`
	TaskID    string   `json:"taskID,omitempty"`
	WithFile  bool     `json:"withFile,omitempty"`
	Force     bool     `json:"force,omitempty"`
}

var dockerIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:@/-]{0,255}$`)
var taskIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

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
	op = strings.ToLower(strings.TrimSpace(op))
	allowed := map[string]bool{"up": true, "down": true, "start": true, "stop": true, "restart": true, "ps": true, "config": true, "pull": true, "delete": true, "remove": true, "rebuild": true}
	if !allowed[op] {
		return model.CommandResult{}, &containerError{"不支持的 Compose 操作"}
	}
	if op == "delete" || op == "remove" {
		op = "down"
	}
	if op == "rebuild" {
		for _, service := range req.Services {
			if !validDockerIdentifier(service) {
				return model.CommandResult{}, &containerError{"Compose 服务名称无效"}
			}
		}
		downArgs := []string{"compose", "-f", req.Path, "down", "--remove-orphans"}
		if req.Force {
			downArgs = append(downArgs, "--volumes")
		}
		downResult, err := runDocker(r, downArgs...)
		if err != nil || downResult.ExitCode != 0 {
			return downResult, err
		}
		upArgs := []string{"compose", "-f", req.Path, "up", "-d", "--build"}
		for _, service := range req.Services {
			upArgs = append(upArgs, service)
		}
		upResult, err := runDocker(r, upArgs...)
		upResult.Stdout = joinCommandOutput(downResult.Stdout, upResult.Stdout)
		upResult.Stderr = joinCommandOutput(downResult.Stderr, upResult.Stderr)
		upResult.Duration += downResult.Duration
		return upResult, err
	}
	args := []string{"compose", "-f", req.Path, op}
	// Panel lifecycle actions are asynchronous from the user's perspective;
	// keep the request detached so `up` does not block on service logs.
	if op == "up" {
		args = append(args, "--detach")
	}
	if strings.EqualFold(req.Operation, "delete") || strings.EqualFold(req.Operation, "remove") {
		args = append(args, "--remove-orphans")
		if req.Force {
			args = append(args, "--volumes")
		}
	}
	for _, service := range req.Services {
		if !validDockerIdentifier(service) {
			return model.CommandResult{}, &containerError{"Compose 服务名称无效"}
		}
		args = append(args, service)
	}
	return runDocker(r, args...)
}

func joinCommandOutput(parts ...string) string {
	joined := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			joined = append(joined, trimmed)
		}
	}
	return strings.Join(joined, "\n")
}

func composeFilePath(req composeRequest) string {
	if strings.TrimSpace(req.Path) != "" {
		return filepath.Clean(strings.TrimSpace(req.Path))
	}
	if strings.EqualFold(strings.TrimSpace(req.From), "path") {
		return ""
	}
	name := strings.TrimSpace(req.DirName)
	if name == "" {
		name = strings.TrimSpace(req.Name)
	}
	if !validDockerIdentifier(name) {
		return ""
	}
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	return filepath.Join(root, "docker", "compose", name, "docker-compose.yml")
}

func validateComposeFile(path string) error {
	if !validDockerPath(path) || strings.Contains(path, "..") {
		return errors.New("Compose 文件路径无效")
	}
	return nil
}

func writeAtomicFile(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".workmesh-compose-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
