// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// registerHostSupervisorRoutes exposes the host Supervisor tool used by the
// panel.  It operates on the host's real supervisord configuration; no state
// is fabricated when the dependency is absent.
func registerHostSupervisorRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/hosts/tool/status", handleHostSupervisorStatus)
	mux.HandleFunc("POST /api/v2/hosts/tool/config/get", handleHostSupervisorConfigGet)
	mux.HandleFunc("POST /api/v2/hosts/tool/config/set", handleHostSupervisorConfigSet)
	mux.HandleFunc("POST /api/v2/hosts/tool/init", handleHostSupervisorInit)
	mux.HandleFunc("POST /api/v2/hosts/tool/operate", handleHostSupervisorOperate)
	mux.HandleFunc("GET /api/v2/hosts/tool/supervisor/process", handleHostSupervisorProcessList)
	mux.HandleFunc("POST /api/v2/hosts/tool/supervisor/process", handleHostSupervisorProcess)
	mux.HandleFunc("POST /api/v2/hosts/tool/supervisor/process/file", handleHostSupervisorProcessFile)
	mux.HandleFunc("POST /api/v2/hosts/tool/supervisor/process/file/get", handleHostSupervisorProcessFileGet)
}

func supervisorConfigPath() string {
	if path := strings.TrimSpace(os.Getenv("WORKMESH_SUPERVISOR_CONFIG")); path != "" {
		return filepath.Clean(path)
	}
	return "/etc/supervisor/supervisord.conf"
}

func supervisorIncludeDir() string {
	if path := strings.TrimSpace(os.Getenv("WORKMESH_SUPERVISOR_INCLUDE_DIR")); path != "" {
		return filepath.Clean(path)
	}
	return filepath.Join(filepath.Dir(supervisorConfigPath()), "conf.d")
}

func supervisorServiceName() string {
	name := strings.TrimSpace(os.Getenv("WORKMESH_SUPERVISOR_SERVICE"))
	if name == "" {
		name = "supervisord"
	}
	if !supervisorProcessNamePattern.MatchString(name) {
		return "supervisord"
	}
	return name
}

func supervisorMutationAllowed() bool { return os.Getenv("WORKMESH_ALLOW_HOST_MUTATION") == "1" }

func supervisorDependencyError(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s 未安装或不在 PATH 中", name)
	}
	return nil
}

func supervisorCommand(r *http.Request, program string, args ...string) (model.CommandResult, error) {
	return (service.CommandService{}).Execute(r.Context(), model.CommandRequest{Program: program, Args: args, Timeout: 60 * time.Second})
}

func writeSupervisorError(w http.ResponseWriter, status int, err error) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
}

func decodeSupervisorBody(r *http.Request) (map[string]any, error) {
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		return nil, err
	}
	if body == nil {
		body = map[string]any{}
	}
	return body, nil
}

func handleHostSupervisorStatus(w http.ResponseWriter, r *http.Request) {
	ctlErr := supervisorDependencyError("supervisorctl")
	daemonErr := supervisorDependencyError("supervisord")
	if ctlErr != nil && daemonErr != nil {
		writeSupervisorError(w, http.StatusServiceUnavailable, ctlErr)
		return
	}
	result := map[string]any{"type": "supervisord", "configPath": supervisorConfigPath(), "includeDir": supervisorIncludeDir(), "logPath": filepath.Join(filepath.Dir(supervisorConfigPath()), "supervisord.log"), "serviceName": supervisorServiceName(), "ctlExist": ctlErr == nil, "isExist": daemonErr == nil, "init": false, "status": "unknown", "isRunning": false, "version": "", "msg": ""}
	if _, err := os.Stat(supervisorConfigPath()); err == nil {
		result["init"] = true
	}
	if daemonErr == nil {
		if version, err := supervisorCommand(r, "supervisord", "--version"); err == nil && version.ExitCode == 0 {
			result["version"] = strings.TrimSpace(version.Stdout)
		}
	}
	if ctlErr == nil {
		status, err := supervisorCommand(r, "supervisorctl", "-c", supervisorConfigPath(), "status")
		if err == nil && status.ExitCode == 0 {
			result["status"] = "active"
			result["isRunning"] = true
		} else {
			result["status"] = "inactive"
			result["msg"] = strings.TrimSpace(status.Stderr)
		}
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
}

func handleHostSupervisorConfigGet(w http.ResponseWriter, _ *http.Request) {
	content, err := os.ReadFile(supervisorConfigPath())
	if errors.Is(err, os.ErrNotExist) {
		writeSupervisorError(w, http.StatusServiceUnavailable, errors.New("Supervisor 配置文件不存在"))
		return
	}
	if err != nil {
		writeSupervisorError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"type": "supervisord", "content": string(content)}})
}

func handleHostSupervisorConfigSet(w http.ResponseWriter, r *http.Request) {
	if !supervisorMutationAllowed() {
		writeSupervisorError(w, http.StatusServiceUnavailable, errors.New("Supervisor 配置写操作需要 WORKMESH_ALLOW_HOST_MUTATION=1"))
		return
	}
	body, err := decodeSupervisorBody(r)
	if err != nil {
		writeSupervisorError(w, http.StatusBadRequest, err)
		return
	}
	content := runtimeString(body, "content", "file")
	if strings.TrimSpace(content) == "" || len(content) > 4<<20 {
		writeSupervisorError(w, http.StatusBadRequest, errors.New("Supervisor 配置内容不能为空或过大"))
		return
	}
	path := supervisorConfigPath()
	old, oldErr := os.ReadFile(path)
	existed := oldErr == nil
	if oldErr != nil && !errors.Is(oldErr, os.ErrNotExist) {
		writeSupervisorError(w, http.StatusInternalServerError, oldErr)
		return
	}
	if err := writeAtomicRuntimeFile(path, []byte(content)); err != nil {
		writeSupervisorError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := supervisorCommand(r, "supervisord", "-t", "-c", path); err != nil {
		restoreSupervisorFile(path, old, existed)
		writeSupervisorError(w, http.StatusBadRequest, fmt.Errorf("Supervisor 配置校验失败，已恢复旧文件: %w", err))
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": path}})
}

func handleHostSupervisorInit(w http.ResponseWriter, r *http.Request) {
	if !supervisorMutationAllowed() {
		writeSupervisorError(w, http.StatusServiceUnavailable, errors.New("Supervisor 初始化需要 WORKMESH_ALLOW_HOST_MUTATION=1"))
		return
	}
	if err := supervisorDependencyError("supervisord"); err != nil {
		writeSupervisorError(w, http.StatusServiceUnavailable, err)
		return
	}
	path := supervisorConfigPath()
	if _, err := os.Stat(path); err == nil {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"initialized": false, "path": path}})
		return
	}
	content := "[unix_http_server]\nfile=/run/supervisor.sock\n[supervisord]\nlogfile=/var/log/supervisord.log\npidfile=/run/supervisord.pid\n[supervisorctl]\nserverurl=unix:///run/supervisor.sock\n[include]\nfiles = " + filepath.Join(supervisorIncludeDir(), "*.ini") + "\n"
	if err := writeAtomicRuntimeFile(path, []byte(content)); err != nil {
		writeSupervisorError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"initialized": true, "path": path}})
}

func handleHostSupervisorOperate(w http.ResponseWriter, r *http.Request) {
	body, err := decodeSupervisorBody(r)
	if err != nil {
		writeSupervisorError(w, http.StatusBadRequest, err)
		return
	}
	operation := strings.ToLower(strings.TrimSpace(runtimeString(body, "operate", "operation")))
	allowed := map[string]bool{"start": true, "stop": true, "restart": true, "reload": true, "enable": true, "disable": true}
	if !allowed[operation] {
		writeSupervisorError(w, http.StatusBadRequest, errors.New("Supervisor 操作无效"))
		return
	}
	if !supervisorMutationAllowed() {
		writeSupervisorError(w, http.StatusServiceUnavailable, errors.New("Supervisor 操作需要 WORKMESH_ALLOW_HOST_MUTATION=1"))
		return
	}
	if err := supervisorDependencyError("systemctl"); err != nil {
		writeSupervisorError(w, http.StatusServiceUnavailable, err)
		return
	}
	result, runErr := supervisorCommand(r, "systemctl", operation, supervisorServiceName())
	if runErr != nil || result.ExitCode != 0 {
		if runErr == nil {
			runErr = errors.New(strings.TrimSpace(result.Stderr))
		}
		writeSupervisorError(w, http.StatusBadGateway, runErr)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"operation": operation, "service": supervisorServiceName()}})
}

func handleHostSupervisorProcessList(w http.ResponseWriter, r *http.Request) {
	if err := supervisorDependencyError("supervisorctl"); err != nil {
		writeSupervisorError(w, http.StatusServiceUnavailable, err)
		return
	}
	entries, err := os.ReadDir(supervisorIncludeDir())
	if errors.Is(err, os.ErrNotExist) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": []supervisorProcessConfig{}})
		return
	}
	if err != nil {
		writeSupervisorError(w, http.StatusInternalServerError, err)
		return
	}
	status, statusErr := supervisorCommand(r, "supervisorctl", "-c", supervisorConfigPath(), "status")
	if statusErr != nil || status.ExitCode != 0 {
		if statusErr == nil {
			statusErr = errors.New(strings.TrimSpace(status.Stderr))
		}
		writeSupervisorError(w, http.StatusBadGateway, statusErr)
		return
	}
	statuses := parseSupervisorStatus(status.Stdout)
	items := make([]supervisorProcessConfig, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ini") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".ini")
		if !supervisorProcessNamePattern.MatchString(name) {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(supervisorIncludeDir(), entry.Name()))
		if readErr != nil {
			continue
		}
		config, parseErr := parseSupervisorConfig(content, name)
		if parseErr != nil {
			continue
		}
		config.Status = statuses[name]
		if config.Status == nil {
			config.Status = []supervisorProcessItem{}
		}
		items = append(items, config)
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": items})
}

func handleHostSupervisorProcess(w http.ResponseWriter, r *http.Request) {
	body, err := decodeSupervisorBody(r)
	if err != nil {
		writeSupervisorError(w, http.StatusBadRequest, err)
		return
	}
	name := strings.TrimSpace(runtimeString(body, "name"))
	operation := strings.ToLower(strings.TrimSpace(runtimeString(body, "operate", "operation")))
	if !supervisorProcessNamePattern.MatchString(name) {
		writeSupervisorError(w, http.StatusBadRequest, errors.New("Supervisor 进程名称无效"))
		return
	}
	if !supervisorMutationAllowed() {
		writeSupervisorError(w, http.StatusServiceUnavailable, errors.New("Supervisor 进程操作需要 WORKMESH_ALLOW_HOST_MUTATION=1"))
		return
	}
	if err := supervisorDependencyError("supervisorctl"); err != nil {
		writeSupervisorError(w, http.StatusServiceUnavailable, err)
		return
	}
	path := filepath.Join(supervisorIncludeDir(), name+".ini")
	if operation == "create" || operation == "update" {
		config, parseErr := supervisorConfigFromBody(body)
		if parseErr != nil {
			writeSupervisorError(w, http.StatusBadRequest, parseErr)
			return
		}
		old, oldErr := os.ReadFile(path)
		existed := oldErr == nil
		if oldErr != nil && !errors.Is(oldErr, os.ErrNotExist) {
			writeSupervisorError(w, 500, oldErr)
			return
		}
		if operation == "create" && existed {
			writeSupervisorError(w, 409, errors.New("Supervisor 进程已存在"))
			return
		}
		if operation == "update" && !existed {
			writeSupervisorError(w, 404, errors.New("Supervisor 进程不存在"))
			return
		}
		if err := writeAtomicRuntimeFile(path, renderSupervisorConfig(config)); err != nil {
			writeSupervisorError(w, 500, err)
			return
		}
		if _, err := supervisorCommand(r, "supervisorctl", "-c", supervisorConfigPath(), "reread"); err != nil {
			restoreSupervisorFile(path, old, existed)
			writeSupervisorError(w, 502, err)
			return
		}
		if _, err := supervisorCommand(r, "supervisorctl", "-c", supervisorConfigPath(), "update", name); err != nil {
			restoreSupervisorFile(path, old, existed)
			writeSupervisorError(w, 502, err)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": config})
		return
	}
	if operation == "delete" {
		old, readErr := os.ReadFile(path)
		if readErr != nil {
			writeSupervisorError(w, 404, readErr)
			return
		}
		if err := os.Remove(path); err != nil {
			writeSupervisorError(w, 500, err)
			return
		}
		if _, err := supervisorCommand(r, "supervisorctl", "-c", supervisorConfigPath(), "reread"); err != nil {
			_ = writeAtomicRuntimeFile(path, old)
			writeSupervisorError(w, 502, err)
			return
		}
		if _, err := supervisorCommand(r, "supervisorctl", "-c", supervisorConfigPath(), "update", name); err != nil {
			_ = writeAtomicRuntimeFile(path, old)
			_, _ = supervisorCommand(r, "supervisorctl", "-c", supervisorConfigPath(), "reread")
			writeSupervisorError(w, 502, err)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"name": name, "deleted": true}})
		return
	}
	if operation != "start" && operation != "stop" && operation != "restart" {
		writeSupervisorError(w, 400, errors.New("Supervisor 进程操作无效"))
		return
	}
	result, runErr := supervisorCommand(r, "supervisorctl", "-c", supervisorConfigPath(), operation, name+":*")
	if runErr != nil || result.ExitCode != 0 {
		if runErr == nil {
			runErr = errors.New(strings.TrimSpace(result.Stderr))
		}
		writeSupervisorError(w, 502, runErr)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"name": name, "operation": operation}})
}

func handleHostSupervisorProcessFile(w http.ResponseWriter, r *http.Request) {
	handleHostSupervisorProcessFileCommon(w, r, false)
}
func handleHostSupervisorProcessFileGet(w http.ResponseWriter, r *http.Request) {
	handleHostSupervisorProcessFileCommon(w, r, true)
}

func handleHostSupervisorProcessFileCommon(w http.ResponseWriter, r *http.Request, getOnly bool) {
	body, err := decodeSupervisorBody(r)
	if err != nil {
		writeSupervisorError(w, 400, err)
		return
	}
	name := strings.TrimSpace(runtimeString(body, "name"))
	if !supervisorProcessNamePattern.MatchString(name) {
		writeSupervisorError(w, 400, errors.New("Supervisor 进程名称无效"))
		return
	}
	file := strings.ToLower(strings.TrimSpace(runtimeString(body, "file")))
	if file != "config" && file != "stdout" && file != "stderr" {
		writeSupervisorError(w, 400, errors.New("Supervisor 文件类型无效"))
		return
	}
	path := filepath.Join(supervisorIncludeDir(), name+".ini")
	if file != "config" {
		path = filepath.Join(supervisorIncludeDir(), "..", "log", name+"."+file+".log")
	}
	if getOnly || strings.EqualFold(runtimeString(body, "operate"), "get") {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			writeSupervisorError(w, 404, readErr)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"name": name, "file": file, "content": string(content)}})
		return
	}
	if !supervisorMutationAllowed() {
		writeSupervisorError(w, 503, errors.New("Supervisor 文件写操作需要 WORKMESH_ALLOW_HOST_MUTATION=1"))
		return
	}
	content := runtimeString(body, "content")
	if len(content) > 4<<20 {
		writeSupervisorError(w, 400, errors.New("Supervisor 文件过大"))
		return
	}
	if err := writeAtomicRuntimeFile(path, []byte(content)); err != nil {
		writeSupervisorError(w, 500, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"path": path}})
}
