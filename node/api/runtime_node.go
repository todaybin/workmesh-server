// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// phpInstalledExtensions 处理 PHP 运行时配置或扩展操作。
func phpInstalledExtensions(executor runtimeCommandExecutor, item runtimeRecord) ([]string, error) {
	result, err := runtimeCommand(executor, item, 20*time.Second, "exec", "-i", item.Container, "php", "-m")
	if err != nil {
		return nil, err
	}
	seen := map[string]string{}
	for _, line := range strings.Split(result.Stdout, "\n") {
		value := strings.TrimSpace(line)
		if value == "" || value == "[PHP Modules]" || value == "[Zend Modules]" {
			continue
		}
		seen[strings.ToLower(value)] = value
	}
	items := make([]string, 0, len(seen))
	for _, value := range seen {
		items = append(items, value)
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i]) < strings.ToLower(items[j]) })
	return items, nil
}

// registerNodeRuntimeRoutes 注册 Node.js 包脚本、模块查询和受限模块操作接口。
func registerNodeRuntimeRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("POST /api/v2/runtimes/node/package", nodePackageHandler())
	mux.HandleFunc("POST /api/v2/runtimes/node/modules", nodeModulesHandler(s))
	mux.HandleFunc("POST /api/v2/runtimes/node/modules/operate", nodeModuleOperationHandler(s))
	mux.HandleFunc("GET /api/v2/runtimes/node/tasks/{id}", nodeTaskHandler(s))
}

// nodePackageHandler 返回 package.json 脚本查询处理器。
func nodePackageHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
	}
}

// nodeModulesHandler 返回 Node 模块扫描处理器。
func nodeModulesHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
	}
}

// nodeModuleOperationHandler 返回 Node 模块异步操作处理器。
func nodeModuleOperationHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		if manager != "npm" && manager != "yarn" && manager != "pnpm" {
			runtimeErr(w, http.StatusBadRequest, "Node 包管理器只允许 npm、yarn 或 pnpm")
			return
		}
		if manager == "pnpm" && nodeRuntimeMajor(record.Version) < 18 {
			runtimeErr(w, http.StatusBadRequest, "Node 18 以下版本不支持 pnpm")
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
		if strings.TrimSpace(record.Container) == "" {
			runtimeErr(w, http.StatusConflict, "Node 运行时尚未关联容器")
			return
		}
		taskID := "node-module-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
		task := map[string]any{"id": taskID, "runtimeID": record.ID, "operation": operation, "module": module, "packageManager": manager, "status": "queued", "createdAt": time.Now().UTC()}
		s.mu.Lock()
		previous, hadPrevious := s.state.Settings["node-task:"+taskID]
		s.state.Settings["node-task:"+taskID] = task
		saveErr := s.saveLocked()
		if saveErr != nil {
			if hadPrevious {
				s.state.Settings["node-task:"+taskID] = previous
			} else {
				delete(s.state.Settings, "node-task:"+taskID)
			}
		}
		s.mu.Unlock()
		if saveErr != nil {
			runtimeErr(w, http.StatusServiceUnavailable, "Node 模块任务存储不可用: "+saveErr.Error())
			return
		}
		go runNodeModuleTask(s, taskID, s.commandExecutor(), record, manager, operation, module)
		wmhttp.JSON(w, http.StatusAccepted, map[string]any{"code": 200, "data": task})
	}
}

// nodeTaskHandler 返回 Node 模块任务查询处理器。
func nodeTaskHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		task := s.state.Settings["node-task:"+r.PathValue("id")]
		s.mu.RUnlock()
		if task == nil {
			runtimeErr(w, http.StatusNotFound, "Node 模块任务不存在")
			return
		}
		runtimeOK(w, task)
	}
}

type nodePackageManifest struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	License     string            `json:"license"`
	Description string            `json:"description"`
	Scripts     map[string]string `json:"scripts"`
}

// runtimeRequestID 处理运行时业务规则，并保持 SQLite 与外部资源一致。
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

// findNodeRuntime 执行运行时相关处理并返回可观测错误。
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

// validateNodeRuntimeDirectory 校验运行时参数和外部资源边界。
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

// readNodePackage 读取运行时配置或外部状态，并限制资源使用。
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

// scanNodeModules 执行运行时相关处理并返回可观测错误。
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

// nodeRuntimeMajor 执行运行时相关处理并返回可观测错误。
func nodeRuntimeMajor(version string) int {
	major, _ := strconv.Atoi(strings.SplitN(strings.TrimSpace(version), ".", 2)[0])
	return major
}

// runNodeModuleTask 执行运行时相关处理并返回可观测错误。
func runNodeModuleTask(s *runtimeStore, taskID string, executor runtimeCommandExecutor, record runtimeRecord, manager, operation, module string) {
	update := func(status, message string) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		key := "node-task:" + taskID
		value, _ := s.state.Settings[key].(map[string]any)
		if value == nil {
			return errors.New("Node 模块任务不存在")
		}
		previous := make(map[string]any, len(value))
		for key, item := range value {
			previous[key] = item
		}
		next := make(map[string]any, len(value)+2)
		for key, item := range value {
			next[key] = item
		}
		next["status"] = status
		next["updatedAt"] = time.Now().UTC()
		if message != "" {
			next["error"] = message
		}
		s.state.Settings[key] = next
		if err := s.saveLocked(); err != nil {
			s.state.Settings[key] = previous
			return err
		}
		return nil
	}
	if err := update("running", ""); err != nil {
		appendRuntimeTaskLog(taskID, "Node 模块任务状态保存失败，已停止执行: "+err.Error())
		return
	}
	command := operation
	if manager == "yarn" {
		switch operation {
		case "install":
			command = "add"
		case "uninstall":
			command = "remove"
		case "update":
			command = "upgrade"
		}
	} else if manager == "pnpm" {
		switch operation {
		case "install":
			command = "add"
		case "uninstall":
			command = "remove"
		}
	}
	args := []string{command}
	if module != "" {
		args = append(args, module)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	requestArgs := append([]string{"exec", "-i", record.Container, manager}, args...)
	result, err := executor.Execute(ctx, model.CommandRequest{Program: service.DockerBinary(), Args: requestArgs, Timeout: 20 * time.Minute})
	if err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = strings.TrimSpace(result.Stdout)
		}
		if len(message) > 512 {
			message = message[:512]
		}
		if message == "" && err != nil {
			message = err.Error()
		}
		if updateErr := update("failed", message); updateErr != nil {
			appendRuntimeTaskLog(taskID, "Node 模块任务失败状态保存失败: "+updateErr.Error())
		}
		return
	}
	if updateErr := update("completed", ""); updateErr != nil {
		appendRuntimeTaskLog(taskID, "Node 模块任务完成状态保存失败: "+updateErr.Error())
	}
}
