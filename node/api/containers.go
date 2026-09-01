// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

type containerRequest struct {
	Container  string    `json:"container"`
	ID         string    `json:"id"`
	Operation  string    `json:"operation"`
	Image      string    `json:"image"`
	Name       string    `json:"name"`
	Repository string    `json:"repository"`
	Tag        string    `json:"tag"`
	Content    string    `json:"content"`
	Path       string    `json:"path"`
	HostPath   string    `json:"hostPath"`
	Args       []string  `json:"args"`
	Env        envValues `json:"env"`
	Command    []string  `json:"command"`
	CPU        string    `json:"cpu"`
	Memory     string    `json:"memory"`
	Paths      []string  `json:"paths"`
	Names      []string  `json:"names"`
	PruneType  string    `json:"pruneType"`
	Dockerfile string    `json:"dockerfile"`
	TagName    string    `json:"tagName"`
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
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &s.state)
	}
	// 进程接入公共数据库后，数据库状态优先于旧版 JSON 文件。
	_ = loadJSONState("container_store_state", &s.state)
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
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
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
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func handleComposeCreate(w http.ResponseWriter, r *http.Request, s *containerStore) {
	var req composeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	path := composeFilePath(req)
	if err := validateComposeFile(path); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if req.Content == "" {
		req.Content = req.File
	}
	if strings.TrimSpace(req.Content) == "" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "Compose 内容不能为空"})
		return
	}
	if err := writeAtomicFile(path, []byte(req.Content)); err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("写入 Compose 文件失败: %v", err)})
		return
	}
	if req.Env != "" {
		if err := writeAtomicFile(filepath.Join(filepath.Dir(path), ".env"), []byte(req.Env)); err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("写入 Compose 环境文件失败: %v", err)})
			return
		}
	}
	now := time.Now().UTC()
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := composeRecord{ID: idToken(), Name: name, Path: path, CreatedAt: now, UpdatedAt: now}
	s.state.Composes = append(s.state.Composes, item)
	if err := s.saveLocked(); err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("保存 Compose 记录失败: %v", err)})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}

func handleComposeUpdate(w http.ResponseWriter, r *http.Request, s *containerStore) {
	var req composeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	path := composeFilePath(req)
	if err := validateComposeFile(path); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "Compose 内容不能为空"})
		return
	}
	if err := writeAtomicFile(path, []byte(req.Content)); err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("更新 Compose 文件失败: %v", err)})
		return
	}
	if req.Env != "" {
		if err := writeAtomicFile(filepath.Join(filepath.Dir(path), ".env"), []byte(req.Env)); err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("更新 Compose 环境文件失败: %v", err)})
			return
		}
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for i := range s.state.Composes {
		if s.state.Composes[i].Path == path || (req.Name != "" && s.state.Composes[i].Name == req.Name) {
			s.state.Composes[i].Path = path
			if req.Name != "" {
				s.state.Composes[i].Name = req.Name
			}
			s.state.Composes[i].UpdatedAt = now
			found = true
			break
		}
	}
	if !found {
		s.state.Composes = append(s.state.Composes, composeRecord{ID: idToken(), Name: req.Name, Path: path, CreatedAt: now, UpdatedAt: now})
	}
	if err := s.saveLocked(); err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("保存 Compose 记录失败: %v", err)})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"path": path}})
}

func handleComposePin(w http.ResponseWriter, r *http.Request, s *containerStore) {
	var req composeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "Compose 名称不能为空"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Composes {
		if s.state.Composes[i].Name == name || s.state.Composes[i].Path == req.Path {
			s.state.Composes[i].Pinned = req.IsPinned
			s.state.Composes[i].UpdatedAt = time.Now().UTC()
			_ = s.saveLocked()
			wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": s.state.Composes[i]})
			return
		}
	}
	wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "NOT_FOUND"}, "message": "Compose 不存在"})
}

func registerContainerRepositoryRoutes(mux *http.ServeMux, s *containerStore) {
	list := func(w http.ResponseWriter, r *http.Request) {
		var q map[string]any
		if r != nil && r.Body != nil {
			_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&q)
		}
		keyword := strings.ToLower(valueString(q, "name", "keyword"))
		s.mu.RLock()
		items := make([]imageRepository, 0, len(s.state.Repositories))
		for _, item := range s.state.Repositories {
			if keyword == "" || strings.Contains(strings.ToLower(item.Name), keyword) || strings.Contains(strings.ToLower(item.DownloadURL), keyword) {
				items = append(items, sanitizeImageRepository(item))
			}
		}
		s.mu.RUnlock()
		sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
	}
	mux.HandleFunc("GET /api/v2/containers/repo", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		items := make([]imageRepository, 0, len(s.state.Repositories))
		for _, item := range s.state.Repositories {
			items = append(items, sanitizeImageRepository(item))
		}
		s.mu.RUnlock()
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
	})
	mux.HandleFunc("POST /api/v2/containers/repo/search", list)
	mux.HandleFunc("POST /api/v2/containers/repo", func(w http.ResponseWriter, r *http.Request) {
		var in imageRepository
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "仓库名称不能为空"})
			return
		}
		now := time.Now().UTC()
		in.ID = idToken()
		in.CreatedAt = now
		in.UpdatedAt = now
		s.mu.Lock()
		s.state.Repositories = append(s.state.Repositories, in)
		err := s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": sanitizeImageRepository(in)})
	})
	mux.HandleFunc("POST /api/v2/containers/repo/update", func(w http.ResponseWriter, r *http.Request) {
		var in imageRepository
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil || strings.TrimSpace(in.ID) == "" {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "仓库 ID 无效"})
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		for i := range s.state.Repositories {
			if s.state.Repositories[i].ID == in.ID {
				if in.Name != "" {
					s.state.Repositories[i].Name = in.Name
				}
				if in.DownloadURL != "" {
					s.state.Repositories[i].DownloadURL = in.DownloadURL
				}
				if in.Protocol != "" {
					s.state.Repositories[i].Protocol = in.Protocol
				}
				if in.Username != "" {
					s.state.Repositories[i].Username = in.Username
				}
				if in.Password != "" {
					s.state.Repositories[i].Password = in.Password
				}
				s.state.Repositories[i].Auth = in.Auth
				s.state.Repositories[i].UpdatedAt = time.Now().UTC()
				_ = s.saveLocked()
				wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": sanitizeImageRepository(s.state.Repositories[i])})
				return
			}
		}
		wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "仓库不存在"})
	})
	for _, path := range []string{"/api/v2/containers/repo/del"} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				ID  string   `json:"id"`
				IDs []string `json:"ids"`
			}
			_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in)
			ids := in.IDs
			if in.ID != "" {
				ids = append(ids, in.ID)
			}
			if len(ids) == 0 {
				wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "仓库 ID 不能为空"})
				return
			}
			s.mu.Lock()
			kept := s.state.Repositories[:0]
			removed := 0
			for _, item := range s.state.Repositories {
				found := false
				for _, id := range ids {
					if id == item.ID {
						found = true
					}
				}
				if found {
					removed++
				} else {
					kept = append(kept, item)
				}
			}
			s.state.Repositories = kept
			_ = s.saveLocked()
			s.mu.Unlock()
			wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"deleted": removed}})
		})
	}
	mux.HandleFunc("POST /api/v2/containers/repo/status", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in)
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, item := range s.state.Repositories {
			if item.ID == in.ID {
				wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"id": item.ID, "configured": item.DownloadURL != "", "auth": item.Auth}})
				return
			}
		}
		wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "仓库不存在"})
	})
}

func registerContainerTemplateRoutes(mux *http.ServeMux, s *containerStore) {
	list := func(w http.ResponseWriter, r *http.Request) {
		var q map[string]any
		if r != nil && r.Body != nil {
			_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&q)
		}
		keyword := strings.ToLower(valueString(q, "name", "keyword", "description"))
		s.mu.RLock()
		items := make([]composeTemplate, 0, len(s.state.Templates))
		for _, item := range s.state.Templates {
			if keyword == "" || strings.Contains(strings.ToLower(item.Name), keyword) || strings.Contains(strings.ToLower(item.Description), keyword) {
				items = append(items, item)
			}
		}
		s.mu.RUnlock()
		sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
	}
	mux.HandleFunc("GET /api/v2/containers/template", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		items := append([]composeTemplate(nil), s.state.Templates...)
		s.mu.RUnlock()
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
	})
	mux.HandleFunc("POST /api/v2/containers/template/search", list)
	mux.HandleFunc("POST /api/v2/containers/template", func(w http.ResponseWriter, r *http.Request) {
		var in composeTemplate
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "模板名称不能为空"})
			return
		}
		if len(in.Content) > 4<<20 {
			wmhttp.JSON(w, 413, map[string]any{"code": "ERR", "message": "模板内容超过限制"})
			return
		}
		now := time.Now().UTC()
		in.ID = idToken()
		in.CreatedAt = now
		in.UpdatedAt = now
		s.mu.Lock()
		s.state.Templates = append(s.state.Templates, in)
		err := s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": in})
	})
	mux.HandleFunc("POST /api/v2/containers/template/update", func(w http.ResponseWriter, r *http.Request) {
		var in composeTemplate
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&in); err != nil || strings.TrimSpace(in.ID) == "" {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "模板 ID 无效"})
			return
		}
		if len(in.Content) > 4<<20 {
			wmhttp.JSON(w, 413, map[string]any{"code": "ERR", "message": "模板内容超过限制"})
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		for i := range s.state.Templates {
			if s.state.Templates[i].ID == in.ID {
				if in.Name != "" {
					s.state.Templates[i].Name = in.Name
				}
				if in.Description != "" {
					s.state.Templates[i].Description = in.Description
				}
				if in.Content != "" {
					s.state.Templates[i].Content = in.Content
				}
				s.state.Templates[i].UpdatedAt = time.Now().UTC()
				_ = s.saveLocked()
				wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": s.state.Templates[i]})
				return
			}
		}
		wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "模板不存在"})
	})
	mux.HandleFunc("POST /api/v2/containers/template/batch", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Templates []composeTemplate `json:"templates"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&in); err != nil || len(in.Templates) == 0 {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "模板列表不能为空"})
			return
		}
		now := time.Now().UTC()
		s.mu.Lock()
		for _, item := range in.Templates {
			if strings.TrimSpace(item.Name) == "" {
				continue
			}
			item.ID = idToken()
			item.CreatedAt = now
			item.UpdatedAt = now
			s.state.Templates = append(s.state.Templates, item)
		}
		err := s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"count": len(in.Templates)}})
	})
	mux.HandleFunc("POST /api/v2/containers/template/del", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID  string   `json:"id"`
			IDs []string `json:"ids"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in)
		ids := in.IDs
		if in.ID != "" {
			ids = append(ids, in.ID)
		}
		if len(ids) == 0 {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "模板 ID 不能为空"})
			return
		}
		s.mu.Lock()
		kept := s.state.Templates[:0]
		removed := 0
		for _, item := range s.state.Templates {
			found := false
			for _, id := range ids {
				if item.ID == id {
					found = true
				}
			}
			if found {
				removed++
			} else {
				kept = append(kept, item)
			}
		}
		s.state.Templates = kept
		_ = s.saveLocked()
		s.mu.Unlock()
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"deleted": removed}})
	})
}

func registerContainerSettingsRoutes(mux *http.ServeMux, s *containerStore) {
	for _, path := range []string{"/api/v2/containers/logoption/update", "/api/v2/containers/ipv6option/update"} {
		p := path
		mux.HandleFunc("POST "+p, func(w http.ResponseWriter, r *http.Request) {
			var in map[string]any
			if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
				wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
				return
			}
			s.mu.Lock()
			if s.state.Settings == nil {
				s.state.Settings = map[string]any{}
			}
			key := strings.TrimPrefix(strings.TrimPrefix(p, "/api/v2/containers/"), "/update")
			s.state.Settings[key] = in
			err := s.saveLocked()
			s.mu.Unlock()
			if err != nil {
				wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
				return
			}
			wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": in})
		})
	}
}
func handleComposeSearch(w http.ResponseWriter, r *http.Request) {
	req, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	containers, err := dockerContainerRows(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
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
		writeCommandResult(w, ids, err)
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

func handleContainerRequest(docker service.DockerService, w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/containers")
	path = strings.Trim(path, "/")
	if r.Method == http.MethodPost && path == "search" {
		handleContainerSearch(w, r)
		return
	}
	if r.Method == http.MethodGet && path == "list/stats" {
		handleContainerListStats(w, r)
		return
	}
	if r.Method == http.MethodGet && path == "status" {
		handleContainerStatus(w, r)
		return
	}
	if r.Method == http.MethodPost && path == "image/search" {
		handleImageSearch(w, r)
		return
	}
	if r.Method == http.MethodPost && path == "network/search" {
		handleNetworkSearch(w, r)
		return
	}
	if r.Method == http.MethodPost && path == "volume/search" {
		handleVolumeSearch(w, r)
		return
	}
	if r.Method == http.MethodPost && path == "item/stats" {
		handleContainerItemStats(w, r)
		return
	}
	if r.Method == http.MethodPost && path == "inspect" {
		handleContainerInspect(w, r)
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
	writeCommandResult(w, result, err)
}

func handleContainerStatus(w http.ResponseWriter, r *http.Request) {
	items, err := dockerContainerRows(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
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

func dockerJSONLines(r *http.Request, args ...string) ([]map[string]any, error) {
	result, err := runDocker(r, args...)
	if err != nil || result.ExitCode != 0 {
		if err != nil {
			return nil, err
		}
		return nil, errors.New(strings.TrimSpace(result.Stderr))
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

func parseDockerCreated(value string) time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05 -0700 MST", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

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

func handleImageSearch(w http.ResponseWriter, r *http.Request) {
	req, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	rows, err := dockerJSONLines(r, "image", "ls", "--no-trunc", "--format", "{{json .}}")
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	keyword := strings.ToLower(valueString(req, "name", "info"))
	byID := map[string]map[string]any{}
	used := map[string]bool{}
	containerRows, _ := dockerJSONLines(r, "ps", "-a", "--format", "{{json .}}")
	for _, row := range containerRows {
		used[strings.ToLower(valueString(row, "Image"))] = true
	}
	for _, row := range rows {
		id := valueString(row, "ID", "Id")
		if id == "" {
			continue
		}
		repository, tag := valueString(row, "Repository"), valueString(row, "Tag")
		label := repository
		if tag != "" && tag != "<none>" {
			label += ":" + tag
		}
		if keyword != "" && !strings.Contains(strings.ToLower(label), keyword) {
			continue
		}
		item := byID[id]
		if item == nil {
			item = map[string]any{"id": id, "tags": []string{}, "size": int64(parseDockerBytes(valueString(row, "Size"))), "createdAt": parseDockerCreated(valueString(row, "CreatedAt")), "isUsed": used[strings.ToLower(label)]}
			byID[id] = item
		}
		if label != "" && !strings.Contains(label, "<none>") {
			item["tags"] = append(item["tags"].([]string), label)
		}
	}
	store := getContainerStore()
	items := make([]map[string]any, 0, len(byID))
	for id, item := range byID {
		description, pinned := resourceDescription(store, "image", strings.TrimPrefix(id, "sha256:"))
		item["description"], item["isPinned"] = description, pinned
		items = append(items, item)
	}
	orderBy, order := valueString(req, "orderBy"), valueString(req, "order")
	sort.SliceStable(items, func(i, j int) bool {
		less := fmt.Sprint(items[i]["createdAt"]) < fmt.Sprint(items[j]["createdAt"])
		if orderBy == "size" {
			less = items[i]["size"].(int64) < items[j]["size"].(int64)
		} else if orderBy == "tags" {
			less = fmt.Sprint(items[i]["tags"]) < fmt.Sprint(items[j]["tags"])
		} else if orderBy == "isUsed" {
			less = !items[i]["isUsed"].(bool) && items[j]["isUsed"].(bool)
		}
		if strings.EqualFold(order, "descending") {
			return !less
		}
		return less
	})
	pageItems, page, pageSize := paginateMaps(items, intValue(req, "page"), intValue(req, "pageSize"))
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": pageItems, "total": len(items), "page": page, "pageSize": pageSize}})
}

func handleImageOptions(w http.ResponseWriter, r *http.Request) {
	rows, err := dockerJSONLines(r, "image", "ls", "--format", "{{json .}}")
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	seen := map[string]bool{}
	items := make([]map[string]any, 0)
	for _, row := range rows {
		repo, tag := valueString(row, "Repository"), valueString(row, "Tag")
		if repo == "" || tag == "<none>" {
			continue
		}
		name := repo + ":" + tag
		if !seen[name] {
			seen[name] = true
			items = append(items, map[string]any{"option": name})
		}
	}
	sort.Slice(items, func(i, j int) bool { return fmt.Sprint(items[i]["option"]) < fmt.Sprint(items[j]["option"]) })
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
}

func handleImageAll(w http.ResponseWriter, r *http.Request) {
	req := map[string]any{"page": 1, "pageSize": 10000, "orderBy": "createdAt", "order": "descending"}
	body, _ := json.Marshal(req)
	r2 := r.Clone(r.Context())
	r2.Body = io.NopCloser(strings.NewReader(string(body)))
	result := httptest.NewRecorder()
	handleImageSearch(result, r2)
	var envelope map[string]any
	if json.Unmarshal(result.Body.Bytes(), &envelope) != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": "镜像列表解析失败"})
		return
	}
	data, _ := envelope["data"].(map[string]any)
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": data["items"]})
}

func handleResourceOptions(w http.ResponseWriter, r *http.Request, resource string) {
	rows, err := dockerJSONLines(r, resource, "ls", "--format", "{{json .}}")
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		name := valueString(row, "Name")
		if name != "" {
			items = append(items, map[string]any{"option": name})
		}
	}
	sort.Slice(items, func(i, j int) bool { return fmt.Sprint(items[i]["option"]) < fmt.Sprint(items[j]["option"]) })
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
}

func handleNetworkSearch(w http.ResponseWriter, r *http.Request) {
	req, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	rows, err := dockerJSONLines(r, "network", "ls", "--no-trunc", "--format", "{{json .}}")
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	keyword := strings.ToLower(valueString(req, "info", "name"))
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		name := valueString(row, "Name")
		if keyword != "" && !strings.Contains(strings.ToLower(name), keyword) {
			continue
		}
		inspect, inspectErr := dockerJSONLines(r, "network", "inspect", name, "--format", "{{json .}}")
		if inspectErr != nil || len(inspect) == 0 {
			continue
		}
		detail := inspect[0]
		ipam, _ := detail["IPAM"].(map[string]any)
		configs, _ := ipam["Config"].([]any)
		subnet, gateway := "", ""
		if len(configs) > 0 {
			if c, ok := configs[0].(map[string]any); ok {
				subnet, gateway = valueString(c, "Subnet"), valueString(c, "Gateway")
			}
		}
		labels := make([]string, 0)
		if values, ok := detail["Labels"].(map[string]any); ok {
			for k, v := range values {
				labels = append(labels, k+"="+fmt.Sprint(v))
			}
			sort.Strings(labels)
		}
		created := parseDockerCreated(valueString(detail, "Created"))
		items = append(items, map[string]any{"id": valueString(detail, "Id", "ID"), "name": name, "labels": labels, "driver": valueString(detail, "Driver"), "ipamDriver": valueString(ipam, "Driver"), "subnet": subnet, "gateway": gateway, "createdAt": created, "attachable": dockerBoolValue(detail, "Attachable")})
	}
	pageItems, page, pageSize := paginateMaps(items, intValue(req, "page"), intValue(req, "pageSize"))
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": pageItems, "total": len(items), "page": page, "pageSize": pageSize}})
}

func handleVolumeSearch(w http.ResponseWriter, r *http.Request) {
	req, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	rows, err := dockerJSONLines(r, "volume", "ls", "--format", "{{json .}}")
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	keyword := strings.ToLower(valueString(req, "info", "name"))
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		name := valueString(row, "Name")
		if keyword != "" && !strings.Contains(strings.ToLower(name), keyword) {
			continue
		}
		inspect, inspectErr := dockerJSONLines(r, "volume", "inspect", name, "--format", "{{json .}}")
		if inspectErr != nil || len(inspect) == 0 {
			continue
		}
		detail := inspect[0]
		labels := make([]map[string]any, 0)
		if values, ok := detail["Labels"].(map[string]any); ok {
			for k, v := range values {
				labels = append(labels, map[string]any{"key": k, "value": fmt.Sprint(v)})
			}
		}
		options := make([]map[string]any, 0)
		if values, ok := detail["Options"].(map[string]any); ok {
			for k, v := range values {
				options = append(options, map[string]any{"key": k, "value": fmt.Sprint(v)})
			}
		}
		items = append(items, map[string]any{"name": name, "labels": labels, "driver": valueString(detail, "Driver"), "mountpoint": valueString(detail, "Mountpoint"), "createdAt": parseDockerCreated(valueString(detail, "CreatedAt")), "options": options})
	}
	pageItems, page, pageSize := paginateMaps(items, intValue(req, "page"), intValue(req, "pageSize"))
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": pageItems, "total": len(items), "page": page, "pageSize": pageSize}})
}

func handleContainerItemStats(w http.ResponseWriter, r *http.Request) {
	req, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	name := valueString(req, "name", "id")
	if name == "" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "INVALID_NAME"}, "message": "容器名称不能为空"})
		return
	}
	if name != "system" {
		rows, runErr := dockerJSONLines(r, "inspect", "--size", "--format", "{{json .}}", name)
		if runErr != nil || len(rows) == 0 {
			if runErr == nil {
				runErr = errors.New("容器不存在")
			}
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": runErr.Error()})
			return
		}
		item := rows[0]
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"sizeRw": numberValue(item, "SizeRw"), "sizeRootFs": numberValue(item, "SizeRootFs")}})
		return
	}
	rows, runErr := dockerJSONLines(r, "system", "df", "--format", "{{json .}}")
	if runErr != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": runErr.Error()})
		return
	}
	data := map[string]any{"containerUsage": int64(0), "containerReclaimable": int64(0), "imageUsage": int64(0), "imageReclaimable": int64(0), "volumeUsage": int64(0), "volumeReclaimable": int64(0), "buildCacheUsage": int64(0), "buildCacheReclaimable": int64(0)}
	for _, row := range rows {
		typ := strings.ToLower(valueString(row, "Type"))
		size := int64(parseDockerBytes(valueString(row, "Size")))
		reclaim := int64(parseDockerBytes(strings.TrimSpace(strings.Split(valueString(row, "Reclaimable"), "(")[0])))
		switch typ {
		case "images":
			data["imageUsage"] = data["imageUsage"].(int64) + size
			data["imageReclaimable"] = data["imageReclaimable"].(int64) + reclaim
		case "containers":
			data["containerUsage"] = data["containerUsage"].(int64) + size
			data["containerReclaimable"] = data["containerReclaimable"].(int64) + reclaim
		case "local volumes", "volumes":
			data["volumeUsage"] = data["volumeUsage"].(int64) + size
			data["volumeReclaimable"] = data["volumeReclaimable"].(int64) + reclaim
		case "build cache":
			data["buildCacheUsage"] = data["buildCacheUsage"].(int64) + size
			data["buildCacheReclaimable"] = data["buildCacheReclaimable"].(int64) + reclaim
		}
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": data})
}

func numberValue(item map[string]any, key string) int64 {
	switch value := item[key].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case json.Number:
		v, _ := value.Int64()
		return v
	}
	return 0
}
func dockerBoolValue(item map[string]any, key string) bool {
	value, _ := item[key].(bool)
	return value
}

func handleContainerInspect(w http.ResponseWriter, r *http.Request) {
	req, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	id, typ, detail := valueString(req, "id", "name"), strings.ToLower(valueString(req, "type")), valueString(req, "detail")
	if id == "" || typ == "" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "id 和 type 不能为空"})
		return
	}
	if typ == "compose" {
		store := getContainerStore()
		store.mu.RLock()
		records := append([]composeRecord(nil), store.state.Composes...)
		store.mu.RUnlock()
		for _, record := range records {
			if record.Name == id || record.Path == id {
				path := record.Path
				if detail != "" {
					path = detail
				}
				content, readErr := os.ReadFile(path)
				if readErr != nil {
					wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": readErr.Error()})
					return
				}
				wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": string(content)})
				return
			}
		}
		wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "Compose 不存在"})
		return
	}
	if !validDockerIdentifier(id) {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "对象标识无效"})
		return
	}
	rows, runErr := dockerJSONLines(r, "inspect", id)
	if typ == "image" {
		rows, runErr = dockerJSONLines(r, "image", "inspect", id)
	} else if typ == "network" {
		rows, runErr = dockerJSONLines(r, "network", "inspect", id)
	} else if typ == "volume" {
		rows, runErr = dockerJSONLines(r, "volume", "inspect", id)
	}
	if runErr != nil || len(rows) == 0 {
		if runErr == nil {
			runErr = errors.New("对象不存在")
		}
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": runErr.Error()})
		return
	}
	b, _ := json.Marshal(rows[0])
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": string(b)})
}

func handleContainerListStats(w http.ResponseWriter, r *http.Request) {
	result, err := runDocker(r, "stats", "--no-stream", "--no-trunc", "--format", "{{json .}}")
	if err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" && err != nil {
			message = err.Error()
		}
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": message})
		return
	}
	items := make([]map[string]any, 0)
	for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": fmt.Sprintf("解析 Docker 统计失败: %v", err)})
			return
		}
		usage, limit := parseDockerMemoryUsage(valueString(raw, "MemUsage"))
		items = append(items, map[string]any{
			"containerID": valueString(raw, "ID", "Container"), "cpuTotalUsage": 0, "systemUsage": 0,
			"cpuPercent": parseDockerPercent(valueString(raw, "CPUPerc")), "percpuUsage": 0, "memoryCache": 0,
			"memoryUsage": usage, "memoryLimit": limit, "memoryPercent": parseDockerPercent(valueString(raw, "MemPerc")),
		})
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": items})
}

func parseDockerPercent(value string) float64 {
	parsed, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "%")), 64)
	return parsed
}

func parseDockerMemoryUsage(value string) (uint64, uint64) {
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	return parseDockerBytes(parts[0]), parseDockerBytes(parts[1])
}

func parseDockerBytes(value string) uint64 {
	value = strings.TrimSpace(value)
	units := []struct {
		suffix string
		factor float64
	}{{"TiB", 1 << 40}, {"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}, {"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"B", 1}}
	for _, unit := range units {
		if strings.HasSuffix(value, unit.suffix) {
			number, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, unit.suffix)), 64)
			if number > 0 {
				return uint64(number * unit.factor)
			}
			return 0
		}
	}
	return 0
}

func countNonEmptyLines(value string) int {
	count := 0
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func handleContainerSearch(w http.ResponseWriter, r *http.Request) {
	req, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	items, err := dockerContainerRows(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
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

func handleContainerPost(docker service.DockerService, r *http.Request, path string) (model.CommandResult, error) {
	var body containerRequest
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
		return runDocker(r, pruneType, "prune", "-f")
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

func validContainerPath(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && strings.HasPrefix(value, "/") && !strings.ContainsAny(value, "\x00\r\n") && !strings.Contains(value, "..") && len(value) <= 4096
}

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

var errContainerParameter = &containerError{"容器标识不能为空"}

type containerError struct{ message string }

func (e *containerError) Error() string { return e.message }

func unsupportedContainerOperation(path string) error {
	return &containerError{"容器操作暂未接入 Docker 驱动: " + path}
}

func handleImageOperation(r *http.Request, path string, body containerRequest) (model.CommandResult, error) {
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
		if !validDockerPath(body.Name) || strings.Contains(body.Name, "..") || !validDockerIdentifier(image) {
			return model.CommandResult{}, &containerError{"构建目录或镜像名称无效"}
		}
		args := []string{"build", "-t", image}
		if body.Dockerfile != "" {
			if !validDockerPath(body.Dockerfile) || strings.Contains(body.Dockerfile, "..") {
				return model.CommandResult{}, &containerError{"Dockerfile 路径无效"}
			}
			args = append(args, "-f", body.Dockerfile)
		}
		args = append(args, body.Name)
		return runDocker(r, args...)
	case "image/save":
		if image == "" || !validDockerIdentifier(image) {
			return model.CommandResult{}, &containerError{"镜像名称无效"}
		}
		if !validDockerPath(body.Path) || strings.Contains(body.Path, "..") {
			return model.CommandResult{}, &containerError{"镜像导出路径无效"}
		}
		return runDocker(r, "save", "-o", body.Path, image)
	case "image/load":
		if !validDockerPath(body.Path) || strings.Contains(body.Path, "..") {
			return model.CommandResult{}, &containerError{"镜像导入路径无效"}
		}
		return runDocker(r, "load", "-i", body.Path)
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
		dir = "./data"
	}
	return filepath.Join(dir, "docker-daemon.json")
}

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
