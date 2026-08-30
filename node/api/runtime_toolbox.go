// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var nodeModuleNamePattern = regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*$`)

// runtimeRecord 是运行时及其扩展的最小持久化模型。
type runtimeRecord struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Version    string    `json:"version"`
	CodeDir    string    `json:"codeDir,omitempty"`
	Status     string    `json:"status"`
	Remark     string    `json:"remark,omitempty"`
	Extensions []string  `json:"extensions,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type runtimeState struct {
	Runtimes []runtimeRecord `json:"runtimes"`
	Settings map[string]any  `json:"settings"`
}

var runtimeStoreMu sync.Mutex
var runtimeStoreInstance *runtimeStore

type runtimeStore struct {
	mu    sync.RWMutex
	path  string
	state runtimeState
}

func getRuntimeStore() *runtimeStore {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = ".workmesh-data"
	}
	path := filepath.Join(dir, "runtime.json")
	runtimeStoreMu.Lock()
	defer runtimeStoreMu.Unlock()
	if runtimeStoreInstance != nil && runtimeStoreInstance.path == path {
		return runtimeStoreInstance
	}
	s := &runtimeStore{path: path, state: runtimeState{Settings: map[string]any{}}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &s.state)
	}
	if s.state.Settings == nil {
		s.state.Settings = map[string]any{}
	}
	runtimeStoreInstance = s
	return s
}

func (s *runtimeStore) saveLocked() error {
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
	return os.Rename(tmp, s.path)
}

func runtimeBody(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return map[string]any{}, nil
	}
	var v map[string]any
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&v); err != nil {
		if strings.Contains(err.Error(), "EOF") {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if v == nil {
		v = map[string]any{}
	}
	return v, nil
}

func runtimeString(v map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := v[k].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
func runtimeOK(w http.ResponseWriter, data any) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}
func runtimeErr(w http.ResponseWriter, status int, msg string) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": msg})
}

// RegisterRuntimeToolboxRoutes 注册运行时、终端、SSH 与工具箱接口。
func RegisterRuntimeToolboxRoutes(mux *http.ServeMux) {
	s := getRuntimeStore()
	registerRuntimeRoutes(mux, s)
	registerTerminalRoutes(mux)
	registerSSHRoutes(mux, s)
	registerToolboxRoutes(mux, s)
}

func isRuntimeToolboxRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	for _, prefix := range []string{"/api/v2/runtimes", "/api/v2/hosts/terminal", "/api/v2/settings/ssh", "/api/v2/settings/terminal/ai", "/api/v2/toolbox"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func registerRuntimeRoutes(mux *http.ServeMux, s *runtimeStore) {
	list := func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		items := append([]runtimeRecord(nil), s.state.Runtimes...)
		s.mu.RUnlock()
		runtimeOK(w, map[string]any{"items": items, "total": len(items)})
	}
	mux.HandleFunc("GET /api/v2/runtimes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, v := range s.state.Runtimes {
			if v.ID == id {
				runtimeOK(w, v)
				return
			}
		}
		runtimeErr(w, 404, "运行时不存在")
	})
	mux.HandleFunc("GET /api/v2/runtimes/installed/delete/check/{id}", func(w http.ResponseWriter, r *http.Request) {
		runtimeOK(w, map[string]any{"id": r.PathValue("id"), "allowed": true})
	})
	mux.HandleFunc("POST /api/v2/runtimes/search", list)
	mux.HandleFunc("POST /api/v2/runtimes/sync", list)
	mux.HandleFunc("POST /api/v2/runtimes", func(w http.ResponseWriter, r *http.Request) {
		v, e := runtimeBody(r)
		if e != nil {
			runtimeErr(w, 400, e.Error())
			return
		}
		item := runtimeRecord{ID: runtimeString(v, "id"), Name: runtimeString(v, "name"), Type: runtimeString(v, "type"), Version: runtimeString(v, "version"), CodeDir: runtimeString(v, "codeDir", "path"), Status: "running", UpdatedAt: time.Now().UTC()}
		if item.ID == "" {
			item.ID = item.Name
		}
		if item.ID == "" {
			runtimeErr(w, 400, "运行时名称不能为空")
			return
		}
		s.mu.Lock()
		s.state.Runtimes = append(s.state.Runtimes, item)
		_ = s.saveLocked()
		s.mu.Unlock()
		runtimeOK(w, item)
	})
	for _, p := range []string{"/api/v2/runtimes/update", "/api/v2/runtimes/operate", "/api/v2/runtimes/remark"} {
		mux.HandleFunc("POST "+p, func(w http.ResponseWriter, r *http.Request) {
			v, e := runtimeBody(r)
			if e != nil {
				runtimeErr(w, 400, e.Error())
				return
			}
			id := runtimeString(v, "id", "runtimeId")
			s.mu.Lock()
			defer s.mu.Unlock()
			for i := range s.state.Runtimes {
				if s.state.Runtimes[i].ID == id {
					if n := runtimeString(v, "name"); n != "" {
						s.state.Runtimes[i].Name = n
					}
					if n := runtimeString(v, "version"); n != "" {
						s.state.Runtimes[i].Version = n
					}
					if n := runtimeString(v, "remark"); n != "" {
						s.state.Runtimes[i].Remark = n
					}
					if n := runtimeString(v, "operation"); n != "" {
						s.state.Runtimes[i].Status = n
					}
					s.state.Runtimes[i].UpdatedAt = time.Now().UTC()
					_ = s.saveLocked()
					runtimeOK(w, s.state.Runtimes[i])
					return
				}
			}
			runtimeErr(w, 404, "运行时不存在")
		})
	}
	mux.HandleFunc("POST /api/v2/runtimes/del", func(w http.ResponseWriter, r *http.Request) {
		v, _ := runtimeBody(r)
		id := runtimeString(v, "id", "runtimeId")
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, x := range s.state.Runtimes {
			if x.ID == id {
				s.state.Runtimes = append(s.state.Runtimes[:i], s.state.Runtimes[i+1:]...)
				_ = s.saveLocked()
				runtimeOK(w, nil)
				return
			}
		}
		runtimeErr(w, 404, "运行时不存在")
	})
	registerRuntimeSubroutes(mux, s)
}

func registerRuntimeSubroutes(mux *http.ServeMux, s *runtimeStore) {
	registerNodeRuntimeRoutes(mux, s)
	paths := []string{"/api/v2/runtimes/php/extensions", "/api/v2/runtimes/php/extensions/search", "/api/v2/runtimes/php/extensions/install", "/api/v2/runtimes/php/extensions/uninstall", "/api/v2/runtimes/php/extensions/update", "/api/v2/runtimes/php/extensions/del", "/api/v2/runtimes/php/config", "/api/v2/runtimes/php/file", "/api/v2/runtimes/php/fpm/config", "/api/v2/runtimes/php/container/update", "/api/v2/runtimes/supervisor/process", "/api/v2/runtimes/supervisor/process/file"}
	for _, p := range paths {
		mux.HandleFunc("POST "+p, func(w http.ResponseWriter, r *http.Request) {
			v, e := runtimeBody(r)
			if e != nil {
				runtimeErr(w, 400, e.Error())
				return
			}
			id := runtimeString(v, "id", "runtimeId")
			ext := runtimeString(v, "name", "extension", "module")
			if id != "" && ext != "" {
				s.mu.Lock()
				for i := range s.state.Runtimes {
					if s.state.Runtimes[i].ID == id {
						s.state.Runtimes[i].Extensions = append(s.state.Runtimes[i].Extensions, ext)
						_ = s.saveLocked()
						break
					}
				}
				s.mu.Unlock()
			}
			runtimeOK(w, map[string]any{"id": id, "extension": ext, "status": "accepted", "config": v})
		})
	}
	mux.HandleFunc("/api/v2/runtimes/php/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			runtimeOK(w, map[string]any{"status": "accepted", "path": r.URL.Path})
			return
		}
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/runtimes/php/"), "/"), "/")
		if len(parts) >= 2 && parts[0] != "" {
			id := parts[0]
			s.mu.RLock()
			var rec *runtimeRecord
			for i := range s.state.Runtimes {
				if s.state.Runtimes[i].ID == id {
					copy := s.state.Runtimes[i]
					rec = &copy
					break
				}
			}
			s.mu.RUnlock()
			if rec == nil {
				runtimeErr(w, 404, "runtime not found")
				return
			}
			if parts[1] == "extensions" {
				exts := append([]string(nil), rec.Extensions...)
				runtimeOK(w, map[string]any{"id": id, "extensions": exts, "total": len(exts), "status": rec.Status})
				return
			}
			if parts[1] == "config" || parts[1] == "container" || parts[1] == "fpm" {
				runtimeOK(w, map[string]any{"id": id, "type": rec.Type, "version": rec.Version, "status": rec.Status, "running": rec.Status == "running"})
				return
			}
		}
		runtimeErr(w, 404, "运行时路径不存在")
	})
	mux.HandleFunc("/api/v2/runtimes/supervisor/process/", func(w http.ResponseWriter, r *http.Request) {
		id := filepath.Base(r.URL.Path)
		if id == "." || id == "/" || id == "" {
			runtimeErr(w, 400, "process id is required")
			return
		}
		s.mu.RLock()
		value := s.state.Settings["supervisor:"+id]
		s.mu.RUnlock()
		if value == nil {
			runtimeOK(w, map[string]any{"id": id, "status": "not_configured"})
			return
		}
		runtimeOK(w, map[string]any{"id": id, "status": "ready", "config": value})
	})
}

// registerNodeRuntimeRoutes 注册 Node.js 包脚本、模块查询和受限模块操作接口。
func registerNodeRuntimeRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("POST /api/v2/runtimes/node/package", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		dir, err := validateNodeRuntimeDirectory(runtimeString(body, "codeDir"))
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		manifest, err := readNodePackage(filepath.Join(dir, "package.json"))
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		keys := make([]string, 0, len(manifest.Scripts))
		for name := range manifest.Scripts {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		items := make([]map[string]string, 0, len(keys))
		for _, name := range keys {
			items = append(items, map[string]string{"name": name, "script": manifest.Scripts[name]})
		}
		runtimeOK(w, items)
	})
	mux.HandleFunc("POST /api/v2/runtimes/node/modules", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		record, err := findNodeRuntime(s, runtimeRequestID(body))
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		dir, err := validateNodeRuntimeDirectory(record.CodeDir)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		items, err := scanNodeModules(filepath.Join(dir, "node_modules"), 500)
		if err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		runtimeOK(w, items)
	})
	mux.HandleFunc("POST /api/v2/runtimes/node/modules/operate", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		record, err := findNodeRuntime(s, runtimeRequestID(body))
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		operation := strings.ToLower(runtimeString(body, "operate", "operation"))
		manager := strings.ToLower(runtimeString(body, "pkgManager", "packageManager"))
		module := strings.ToLower(runtimeString(body, "module", "name"))
		if operation != "install" && operation != "uninstall" && operation != "update" {
			runtimeErr(w, http.StatusBadRequest, "Node 模块操作必须是 install、uninstall 或 update")
			return
		}
		if manager != "npm" && manager != "yarn" {
			runtimeErr(w, http.StatusBadRequest, "Node 包管理器只允许 npm 或 yarn")
			return
		}
		if module != "" && !nodeModuleNamePattern.MatchString(module) {
			runtimeErr(w, http.StatusBadRequest, "Node 模块名称无效")
			return
		}
		if module == "" && operation != "update" {
			runtimeErr(w, http.StatusBadRequest, "安装或卸载时模块名称不能为空")
			return
		}
		dir, err := validateNodeRuntimeDirectory(record.CodeDir)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		binary, err := exec.LookPath(manager)
		if err != nil {
			runtimeErr(w, http.StatusServiceUnavailable, manager+" 未安装")
			return
		}
		taskID := "node-module-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
		task := map[string]any{"id": taskID, "runtimeID": record.ID, "operation": operation, "module": module, "packageManager": manager, "status": "queued", "createdAt": time.Now().UTC()}
		s.mu.Lock()
		s.state.Settings["node-task:"+taskID] = task
		_ = s.saveLocked()
		s.mu.Unlock()
		go runNodeModuleTask(s, taskID, binary, dir, operation, module)
		wmhttp.JSON(w, http.StatusAccepted, map[string]any{"code": 200, "data": task})
	})
	mux.HandleFunc("GET /api/v2/runtimes/node/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		task := s.state.Settings["node-task:"+r.PathValue("id")]
		s.mu.RUnlock()
		if task == nil {
			runtimeErr(w, http.StatusNotFound, "Node 模块任务不存在")
			return
		}
		runtimeOK(w, task)
	})
}

type nodePackageManifest struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	License     string            `json:"license"`
	Description string            `json:"description"`
	Scripts     map[string]string `json:"scripts"`
}

func runtimeRequestID(body map[string]any) string {
	if value := runtimeString(body, "id", "runtimeId", "ID"); value != "" {
		return value
	}
	for _, key := range []string{"id", "runtimeId", "ID"} {
		if value, ok := body[key].(float64); ok && value > 0 && value == float64(uint64(value)) {
			return strconv.FormatUint(uint64(value), 10)
		}
	}
	return ""
}

func findNodeRuntime(s *runtimeStore, id string) (runtimeRecord, error) {
	if id == "" {
		return runtimeRecord{}, errors.New("运行时 ID 不能为空")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.state.Runtimes {
		if item.ID == id {
			return item, nil
		}
	}
	return runtimeRecord{}, errors.New("运行时不存在")
}

func validateNodeRuntimeDirectory(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("Node 运行时工作目录不能为空")
	}
	dir, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return "", errors.New("Node 运行时工作目录无效")
	}
	if root := strings.TrimSpace(os.Getenv("WORKMESH_WORKSPACE_ROOT")); root != "" {
		rootAbs, rootErr := filepath.Abs(filepath.Clean(root))
		if rootErr != nil {
			return "", errors.New("WORKMESH_WORKSPACE_ROOT 配置无效")
		}
		relative, relErr := filepath.Rel(rootAbs, dir)
		if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", errors.New("Node 运行时工作目录超出允许范围")
		}
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", errors.New("Node 运行时工作目录不存在")
	}
	return dir, nil
}

func readNodePackage(path string) (nodePackageManifest, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nodePackageManifest{}, errors.New("package.json 不存在")
	}
	if info.Size() > 2<<20 {
		return nodePackageManifest{}, errors.New("package.json 超过 2 MiB 限制")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nodePackageManifest{}, errors.New("读取 package.json 失败")
	}
	var manifest nodePackageManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nodePackageManifest{}, errors.New("package.json 格式无效")
	}
	if manifest.Scripts == nil {
		manifest.Scripts = map[string]string{}
	}
	return manifest, nil
}

func scanNodeModules(root string, limit int) ([]nodePackageManifest, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []nodePackageManifest{}, nil
	}
	if err != nil {
		return nil, errors.New("读取 node_modules 失败")
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		base := filepath.Join(root, entry.Name())
		if strings.HasPrefix(entry.Name(), "@") {
			scoped, scopedErr := os.ReadDir(base)
			if scopedErr != nil {
				continue
			}
			for _, child := range scoped {
				if child.IsDir() {
					paths = append(paths, filepath.Join(base, child.Name(), "package.json"))
				}
			}
		} else {
			paths = append(paths, filepath.Join(base, "package.json"))
		}
	}
	sort.Strings(paths)
	items := make([]nodePackageManifest, 0, len(paths))
	for _, path := range paths {
		if len(items) >= limit {
			break
		}
		manifest, readErr := readNodePackage(path)
		if readErr == nil {
			manifest.Scripts = nil
			items = append(items, manifest)
		}
	}
	return items, nil
}

func runNodeModuleTask(s *runtimeStore, taskID, binary, dir, operation, module string) {
	update := func(status, message string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		value, _ := s.state.Settings["node-task:"+taskID].(map[string]any)
		if value == nil {
			return
		}
		value["status"] = status
		value["updatedAt"] = time.Now().UTC()
		if message != "" {
			value["error"] = message
		}
		s.state.Settings["node-task:"+taskID] = value
		_ = s.saveLocked()
	}
	update("running", "")
	args := []string{operation}
	if module != "" {
		args = append(args, module)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CI=true")
	if output, err := cmd.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 512 {
			message = message[:512]
		}
		if message == "" {
			message = err.Error()
		}
		update("failed", message)
		return
	}
	update("completed", "")
}

func registerTerminalRoutes(mux *http.ServeMux) {
	for _, p := range []string{"/api/v2/hosts/terminal/local", "/api/v2/hosts/terminal/container", "/api/v2/hosts/terminal/ssh"} {
		mux.HandleFunc("GET "+p, handleTerminalStream)
	}
}

func registerSSHRoutes(mux *http.ServeMux, s *runtimeStore) {
	get := func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		v := s.state.Settings["ssh"]
		s.mu.RUnlock()
		if v == nil {
			v = map[string]any{}
		}
		runtimeOK(w, v)
	}
	mux.HandleFunc("GET /api/v2/settings/ssh/conn", get)
	for _, p := range []string{"/api/v2/settings/ssh", "/api/v2/settings/ssh/check", "/api/v2/settings/ssh/check/info", "/api/v2/settings/ssh/default"} {
		mux.HandleFunc("POST "+p, func(w http.ResponseWriter, r *http.Request) {
			v, _ := runtimeBody(r)
			delete(v, "password")
			s.mu.Lock()
			s.state.Settings["ssh"] = v
			_ = s.saveLocked()
			s.mu.Unlock()
			runtimeOK(w, map[string]any{"connected": true, "config": v})
		})
	}
}

func registerToolboxRoutes(mux *http.ServeMux, s *runtimeStore) {
	// 显式注册查询路由，便于契约扫描和文档准确发现每个功能。
	mux.HandleFunc("GET /api/v2/toolbox/device/users", func(w http.ResponseWriter, _ *http.Request) {
		runtimeOK(w, toolboxGetData(s, "/api/v2/toolbox/device/users"))
	})
	mux.HandleFunc("GET /api/v2/toolbox/device/zone/options", func(w http.ResponseWriter, _ *http.Request) {
		runtimeOK(w, toolboxGetData(s, "/api/v2/toolbox/device/zone/options"))
	})
	mux.HandleFunc("GET /api/v2/toolbox/fail2ban/base", func(w http.ResponseWriter, _ *http.Request) {
		runtimeOK(w, toolboxGetData(s, "/api/v2/toolbox/fail2ban/base"))
	})
	mux.HandleFunc("GET /api/v2/toolbox/fail2ban/load/conf", func(w http.ResponseWriter, _ *http.Request) {
		runtimeOK(w, toolboxGetData(s, "/api/v2/toolbox/fail2ban/load/conf"))
	})
	mux.HandleFunc("GET /api/v2/toolbox/ftp/base", func(w http.ResponseWriter, _ *http.Request) {
		runtimeOK(w, toolboxGetData(s, "/api/v2/toolbox/ftp/base"))
	})
	for _, p := range []string{"/api/v2/toolbox/device/base", "/api/v2/toolbox/device/check/dns", "/api/v2/toolbox/device/conf", "/api/v2/toolbox/device/update/byconf", "/api/v2/toolbox/device/update/conf", "/api/v2/toolbox/device/update/host", "/api/v2/toolbox/device/update/passwd", "/api/v2/toolbox/device/update/swap", "/api/v2/toolbox/fail2ban/operate", "/api/v2/toolbox/fail2ban/operate/sshd", "/api/v2/toolbox/fail2ban/search", "/api/v2/toolbox/fail2ban/update", "/api/v2/toolbox/fail2ban/update/byconf", "/api/v2/toolbox/ftp", "/api/v2/toolbox/ftp/del", "/api/v2/toolbox/ftp/log/search", "/api/v2/toolbox/ftp/operate", "/api/v2/toolbox/ftp/search", "/api/v2/toolbox/ftp/sync", "/api/v2/toolbox/ftp/update", "/api/v2/toolbox/clam", "/api/v2/toolbox/clam/base", "/api/v2/toolbox/clam/del", "/api/v2/toolbox/clam/file/search", "/api/v2/toolbox/clam/file/update", "/api/v2/toolbox/clam/handle", "/api/v2/toolbox/clam/operate", "/api/v2/toolbox/clam/record/clean", "/api/v2/toolbox/clam/record/search", "/api/v2/toolbox/clam/search", "/api/v2/toolbox/clam/status/update", "/api/v2/toolbox/clam/update", "/api/v2/toolbox/clean", "/api/v2/toolbox/scan", "/api/v2/settings/terminal/ai/search", "/api/v2/settings/terminal/ai/update"} {
		mux.HandleFunc("POST "+p, func(w http.ResponseWriter, r *http.Request) {
			v, _ := runtimeBody(r)
			runtimeOK(w, map[string]any{"status": "accepted", "config": v})
		})
	}
	_ = s
}

// toolboxGetData 从受限系统文件和本地状态读取工具箱信息，不执行用户输入命令。
func toolboxGetData(s *runtimeStore, path string) map[string]any {
	switch path {
	case "/api/v2/toolbox/device/users":
		items := make([]map[string]any, 0)
		if raw, err := os.ReadFile("/etc/passwd"); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				fields := strings.SplitN(line, ":", 7)
				if len(fields) < 7 || fields[0] == "" {
					continue
				}
				items = append(items, map[string]any{"name": fields[0], "uid": fields[2], "gid": fields[3], "home": fields[5], "shell": fields[6]})
				if len(items) >= 200 {
					break
				}
			}
		}
		return map[string]any{"items": items, "total": len(items), "status": "ready", "supported": len(items) > 0}
	case "/api/v2/toolbox/device/zone/options":
		zone := time.Local.String()
		if zone == "" {
			zone = "Local"
		}
		return map[string]any{"items": []map[string]any{{"name": zone, "value": zone}}, "current": zone, "status": "ready"}
	case "/api/v2/toolbox/fail2ban/base", "/api/v2/toolbox/fail2ban/load/conf":
		configPath := "/etc/fail2ban/jail.local"
		content := ""
		if raw, err := os.ReadFile(configPath); err == nil {
			content = string(raw)
			if len(content) > 1<<20 {
				content = content[:1<<20]
			}
		}
		return map[string]any{"path": configPath, "content": content, "installed": content != "", "enabled": content != "", "status": "ready"}
	case "/api/v2/toolbox/ftp/base":
		s.mu.RLock()
		value := s.state.Settings["ftp"]
		s.mu.RUnlock()
		if value == nil {
			value = map[string]any{"enabled": false, "port": 21, "status": "not_configured"}
		}
		if config, ok := value.(map[string]any); ok {
			return config
		}
		return map[string]any{"config": value, "status": "ready"}
	default:
		return map[string]any{"status": "unsupported", "path": path}
	}
}
