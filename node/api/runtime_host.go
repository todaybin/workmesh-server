package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
)

func hostRuntime(item runtimeRecord) bool { return normalizeRuntimeMode(item.Mode) == "host" }

func hostRuntimeName(item runtimeRecord) string {
	name := strings.TrimSpace(item.Container)
	if name == "" {
		name = strings.TrimSpace(item.Name)
	}
	return name
}

func hostRuntimeConfigPath(item runtimeRecord) string {
	return filepath.Join(supervisorIncludeDir(), hostRuntimeName(item)+".conf")
}

func hostRuntimeConfig(item runtimeRecord) (supervisorProcessConfig, error) {
	command := strings.TrimSpace(runtimeString(item.Params, "EXEC_SCRIPT"))
	dir := strings.TrimSpace(item.CodeDir)
	if command == "" || dir == "" {
		return supervisorProcessConfig{}, errors.New("宿主机运行时需要工作目录和启动命令")
	}
	if !filepath.IsAbs(dir) {
		return supervisorProcessConfig{}, errors.New("宿主机运行时工作目录必须是绝对路径")
	}
	if _, err := os.Stat(dir); err != nil {
		return supervisorProcessConfig{}, fmt.Errorf("宿主机运行时工作目录不可用: %w", err)
	}
	env := make([]string, 0, len(item.Environments)+4)
	for _, raw := range item.Environments {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		key := runtimeString(entry, "key")
		if validEnvKey(key) {
			env = append(env, key+"=\""+strings.ReplaceAll(fmt.Sprint(entry["value"]), "\"", "\\\"")+"\"")
		}
	}
	for _, key := range []string{"WORKMESH_HOME", "WORKMESH_FRONTEND_ROOT", "WORKMESH_PUBLIC_ROOT", "WORKMESH_RUNTIME_ROOT"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			env = append(env, key+"=\""+strings.ReplaceAll(value, "\"", "\\\"")+"\"")
		}
	}
	return supervisorProcessConfig{
		Name: hostRuntimeName(item), Command: command, User: "workmesh", Dir: dir,
		Numprocs: "1", AutoRestart: "true", AutoStart: "true", Environment: strings.Join(env, ","),
	}, nil
}

func hostSupervisorCommand(ctx context.Context, args ...string) (model.CommandResult, error) {
	return (service.CommandService{}).Execute(ctx, model.CommandRequest{Program: "supervisorctl", Args: append([]string{"-c", supervisorConfigPath()}, args...), Timeout: 30 * time.Second})
}

func applyHostRuntime(executor runtimeCommandExecutor, item runtimeRecord, remove bool) error {
	if !hostRuntime(item) {
		return nil
	}
	name := hostRuntimeName(item)
	if !supervisorProcessNamePattern.MatchString(name) {
		return errors.New("宿主机运行时名称无效")
	}
	path := hostRuntimeConfigPath(item)
	if remove {
		_, _ = hostSupervisorCommand(context.Background(), "stop", name+":*")
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else {
		config, err := hostRuntimeConfig(item)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		if err := writeAtomicRuntimeFile(path, renderSupervisorConfig(config)); err != nil {
			return err
		}
	}
	if _, err := hostSupervisorCommand(context.Background(), "reread"); err != nil {
		return fmt.Errorf("Supervisor 重新读取失败: %w", err)
	}
	if _, err := hostSupervisorCommand(context.Background(), "update"); err != nil {
		return fmt.Errorf("Supervisor 应用运行时失败: %w", err)
	}
	if !remove {
		if _, err := hostSupervisorCommand(context.Background(), "start", name+":*"); err != nil {
			return fmt.Errorf("Supervisor 启动运行时失败: %w", err)
		}
	}
	return nil
}

func operateHostRuntime(item runtimeRecord, operation string) error {
	name := hostRuntimeName(item)
	if !supervisorProcessNamePattern.MatchString(name) {
		return errors.New("宿主机运行时名称无效")
	}
	op := strings.ToLower(strings.TrimSpace(operation))
	if op == "up" {
		op = "start"
	}
	if op == "down" {
		op = "stop"
	}
	if op != "start" && op != "stop" && op != "restart" {
		return errors.New("宿主机运行时操作无效")
	}
	result, err := hostSupervisorCommand(context.Background(), op, name+":*")
	if err != nil || result.ExitCode != 0 {
		if err == nil {
			err = errors.New(strings.TrimSpace(result.Stderr))
		}
		return fmt.Errorf("Supervisor 操作失败: %w", err)
	}
	return nil
}

func inspectHostRuntime(item runtimeRecord) runtimeContainerSnapshot {
	value := runtimeContainerSnapshot{status: "Stopped", version: item.UpdatedAt}
	result, err := hostSupervisorCommand(context.Background(), "status", hostRuntimeName(item))
	if err == nil && result.ExitCode == 0 {
		line := strings.TrimSpace(result.Stdout)
		if strings.Contains(line, "RUNNING") {
			value.status = "Running"
		} else if strings.Contains(line, "STARTING") {
			value.status = "Starting"
		}
		return value
	}
	value.err = strings.TrimSpace(result.Stderr)
	if value.err == "" && err != nil {
		value.err = err.Error()
	}
	return value
}
