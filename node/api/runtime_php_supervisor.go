// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// readSupervisorFile 读取运行时配置或外部状态，并限制资源使用。
func readSupervisorFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("Supervisor 文件不是普通文件")
	}
	if info.Size() > 2<<20 {
		return "", errors.New("Supervisor 文件超过 2 MiB 限制")
	}
	content, err := os.ReadFile(path)
	return string(content), err
}

// registerPHPSupervisorRoutes 注册运行时相关 HTTP 路由，并保持请求响应契约。
func registerPHPSupervisorRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("GET /api/v2/runtimes/supervisor/process/{id}", supervisorProcessListHandler(s))
	mux.HandleFunc("POST /api/v2/runtimes/supervisor/process", supervisorProcessOperationHandler(s))
	mux.HandleFunc("POST /api/v2/runtimes/supervisor/process/file", supervisorProcessFileHandler(s))
}

// supervisorProcessListHandler 返回 Supervisor 进程列表处理器。
func supervisorProcessListHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, err := supervisorRuntime(s, r.PathValue("id"))
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		processes, err := listSupervisorProcesses(s.commandExecutor(), item)
		if err != nil {
			runtimeErr(w, http.StatusBadGateway, "读取 Supervisor 进程失败: "+err.Error())
			return
		}
		runtimeOK(w, processes)
	}
}

// supervisorProcessOperationHandler 处理 Supervisor 进程生命周期操作。
func supervisorProcessOperationHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		item, err := supervisorRuntime(s, runtimeRequestID(body))
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		operation := strings.ToLower(runtimeString(body, "operate", "operation"))
		name := runtimeString(body, "name")
		if !supervisorProcessNamePattern.MatchString(name) || name == "php-fpm" && operation == "delete" {
			runtimeErr(w, http.StatusBadRequest, "Supervisor 进程名称或操作无效")
			return
		}
		if operation == "create" || operation == "update" {
			config, configErr := supervisorConfigFromBody(body)
			if configErr != nil {
				runtimeErr(w, http.StatusBadRequest, configErr.Error())
				return
			}
			if err := applySupervisorConfig(s.commandExecutor(), item, operation, config); err != nil {
				status := http.StatusBadGateway
				if strings.Contains(err.Error(), "已存在") || strings.Contains(err.Error(), "不存在") {
					status = http.StatusConflict
				}
				runtimeErr(w, status, err.Error())
				return
			}
			runtimeOK(w, nil)
			return
		}
		if operation != "start" && operation != "stop" && operation != "restart" && operation != "delete" {
			runtimeErr(w, http.StatusBadRequest, "Supervisor 操作只允许 create、update、start、stop、restart 或 delete")
			return
		}
		configPath, outLog, errLog, pathErr := supervisorPaths(item, name)
		if pathErr != nil {
			runtimeErr(w, http.StatusBadRequest, pathErr.Error())
			return
		}
		if _, statErr := os.Stat(configPath); statErr != nil {
			runtimeErr(w, http.StatusNotFound, "Supervisor 进程配置不存在")
			return
		}
		if operation != "delete" {
			if _, err := supervisorCtl(s.commandExecutor(), item, operation, name+":*"); err != nil {
				runtimeErr(w, http.StatusBadGateway, "Supervisor 进程操作失败: "+err.Error())
				return
			}
			runtimeOK(w, nil)
			return
		}
		if err := deleteSupervisorProcess(s, item, name, configPath, outLog, errLog); err != nil {
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		runtimeOK(w, nil)
	}
}

// deleteSupervisorProcess 删除进程配置，并在 Supervisor 应用失败时恢复旧文件。
func deleteSupervisorProcess(s *runtimeStore, item runtimeRecord, name, configPath, outLog, errLog string) error {
	old, err := os.ReadFile(configPath)
	if err != nil {
		return errors.New("Supervisor 进程配置不存在")
	}
	_, _ = supervisorCtl(s.commandExecutor(), item, "stop", name+":*")
	if err := os.Remove(configPath); err != nil {
		return err
	}
	if _, err := supervisorCtl(s.commandExecutor(), item, "reread"); err != nil {
		_ = writeAtomicRuntimeFile(configPath, old)
		return fmt.Errorf("Supervisor 删除失败，已恢复配置: %w", err)
	}
	if _, err := supervisorCtl(s.commandExecutor(), item, "update"); err != nil {
		_ = writeAtomicRuntimeFile(configPath, old)
		_, _ = supervisorCtl(s.commandExecutor(), item, "reread")
		_, _ = supervisorCtl(s.commandExecutor(), item, "update", name)
		return fmt.Errorf("Supervisor 删除失败，已恢复配置: %w", err)
	}
	_ = os.Remove(outLog)
	_ = os.Remove(errLog)
	return nil
}

// supervisorProcessFileHandler 读取、清理或更新 Supervisor 文件。
func supervisorProcessFileHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		item, err := supervisorRuntime(s, runtimeRequestID(body))
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		name := runtimeString(body, "name")
		operation := strings.ToLower(runtimeString(body, "operate", "operation"))
		kind := strings.ToLower(runtimeString(body, "file"))
		configPath, outLog, errLog, err := supervisorPaths(item, name)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		path := configPath
		if kind == "out.log" {
			path = outLog
		} else if kind == "err.log" {
			path = errLog
		} else if kind != "config" {
			runtimeErr(w, http.StatusBadRequest, "Supervisor 文件类型无效")
			return
		}
		handleSupervisorFileOperation(w, s, item, body, name, kind, operation, path, configPath)
	}
}

// handleSupervisorFileOperation 执行 Supervisor 文件操作并在更新失败时回滚。
func handleSupervisorFileOperation(w http.ResponseWriter, s *runtimeStore, item runtimeRecord, body map[string]any, name, kind, operation, path, configPath string) {
	switch operation {
	case "get":
		content, err := readSupervisorFile(path)
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		runtimeOK(w, content)
	case "clear":
		if kind == "config" {
			runtimeErr(w, http.StatusBadRequest, "Supervisor 配置文件不能清空")
			return
		}
		if err := writeAtomicRuntimeFile(path, []byte{}); err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		runtimeOK(w, "")
	case "update":
		updateSupervisorFile(w, s, item, body, name, kind, configPath)
	default:
		runtimeErr(w, http.StatusBadRequest, "Supervisor 文件操作只允许 get、clear 或 update")
	}
}

// updateSupervisorFile 原子更新 Supervisor 配置并重新加载服务。
func updateSupervisorFile(w http.ResponseWriter, s *runtimeStore, item runtimeRecord, body map[string]any, name, kind, configPath string) {
	if kind != "config" {
		runtimeErr(w, http.StatusBadRequest, "只允许更新 Supervisor 配置文件")
		return
	}
	content, ok := body["content"].(string)
	if !ok || content == "" || len(content) > 2<<20 || strings.IndexByte(content, 0) >= 0 {
		runtimeErr(w, http.StatusBadRequest, "Supervisor 配置内容无效或超过 2 MiB 限制")
		return
	}
	if _, err := parseSupervisorConfig([]byte(content), name); err != nil {
		runtimeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	old, err := os.ReadFile(configPath)
	if err != nil {
		runtimeErr(w, http.StatusNotFound, "Supervisor 配置文件不存在")
		return
	}
	if err := writeAtomicRuntimeFile(configPath, []byte(content)); err != nil {
		runtimeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, applyErr := supervisorCtl(s.commandExecutor(), item, "reread")
	if applyErr == nil {
		_, applyErr = supervisorCtl(s.commandExecutor(), item, "update", name)
	}
	if applyErr != nil {
		_ = writeAtomicRuntimeFile(configPath, old)
		_, _ = supervisorCtl(s.commandExecutor(), item, "reread")
		_, _ = supervisorCtl(s.commandExecutor(), item, "update", name)
		runtimeErr(w, http.StatusBadGateway, "Supervisor 配置应用失败，已恢复旧文件: "+applyErr.Error())
		return
	}
	runtimeOK(w, "")
}
