// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// runtimeTaskLogPath 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeTaskLogPath(taskID string) string {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	return filepath.Join(root, "logs", "tasks", "runtime", taskID+".log")
}

// runtimeCanonicalInstallPath 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeCanonicalInstallPath(item runtimeRecord) string {
	typ := normalizeRuntimeTypeFilter(item.Type)
	if item.Name == "" || (typ != "php" && typ != "go" && typ != "java" && typ != "node" && typ != "python" && typ != "dotnet") {
		return ""
	}
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	return filepath.Join(root, "runtimes", typ, item.Name)
}

// hydrateRuntimePaths 补齐 1Panel RuntimeDTO 中的 path、Compose 和容器字段，兼容旧记录。
func hydrateRuntimePaths(item *runtimeRecord) {
	if item == nil {
		return
	}
	if item.Container == "" {
		item.Container = runtimeString(item.Params, "CONTAINER_NAME", "containerName")
	}
	installPath := runtimeCanonicalInstallPath(*item)
	if installPath == "" {
		return
	}
	// Persisted paths are authoritative once installation has created them.
	// Only legacy records missing path metadata should be hydrated to the
	// canonical data-directory location; unconditionally replacing an existing
	// path can make operations target another runtime with the same ID.
	if strings.TrimSpace(item.InstallPath) == "" {
		item.InstallPath = installPath
	}
	if hostRuntime(*item) {
		item.ComposePath = ""
		return
	}
	if strings.TrimSpace(item.ComposePath) == "" {
		item.ComposePath = filepath.Join(item.InstallPath, "docker-compose.yml")
	}
}

// persistRuntimeInstallPath 持久化运行时状态，并保持重启恢复一致性。
func persistRuntimeInstallPath(s *runtimeStore, id string, installPath string) error {
	if s == nil || id == "" || installPath == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previousRuntimes := append([]runtimeRecord(nil), s.state.Runtimes...)
	for index := range s.state.Runtimes {
		if s.state.Runtimes[index].ID != id {
			continue
		}
		hydrateRuntimePaths(&s.state.Runtimes[index])
		if s.state.Runtimes[index].InstallPath == "" {
			s.state.Runtimes[index].InstallPath = installPath
			s.state.Runtimes[index].ComposePath = filepath.Join(installPath, "docker-compose.yml")
		}
		if err := s.saveLocked(); err != nil {
			s.state.Runtimes = previousRuntimes
			return err
		}
		return nil
	}
	return errors.New("运行时不存在")
}

// appendRuntimeTaskLog 保留旧调用方签名；新任务流程使用 checked 版本接收错误。
func appendRuntimeTaskLog(taskID, message string) {
	_ = appendRuntimeTaskLogChecked(taskID, message)
}

func appendRuntimeTaskLogChecked(taskID, message string) error {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(message) == "" {
		return nil
	}
	if repository, err := sharedRuntimeRepository(); err == nil {
		writer, err := storage.NewSQLiteTaskLogWriter(repository)
		if err != nil {
			return err
		}
		return writer.Append(context.Background(), storage.TaskLogEntry{TaskID: taskID, Line: message, CreatedAt: time.Now().UTC()})
	}
	path := runtimeTaskLogPath(taskID)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintf(file, "%s %s\n", time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(message))
	return err
}

// appendRuntimeTaskOutput 将 Docker stdout/stderr 同步写入统一任务日志，供任务抽屉实时轮询。
func appendRuntimeTaskOutput(taskID, stream string, data []byte) {
	if strings.TrimSpace(taskID) == "" || len(data) == 0 {
		return
	}
	label := strings.ToUpper(strings.TrimSpace(stream))
	for _, line := range strings.Split(strings.TrimRight(string(data), "\r\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		message := line
		if label != "" {
			message = "[" + label + "] " + line
		}
		if sharedDB() != nil {
			appendRuntimeTaskLog(taskID, message)
		} else {
			appendAppTaskLog(taskID, message)
			appendRuntimeTaskLog(taskID, message)
		}
	}
}

// updateRuntimeTask 保留旧调用方签名；新任务流程使用 checked 版本接收错误。
func updateRuntimeTask(s *runtimeStore, id, status, message string) runtimeRecord {
	item, _ := updateRuntimeTaskChecked(s, id, status, message)
	return item
}

func updateRuntimeTaskChecked(s *runtimeStore, id, status, message string) (runtimeRecord, error) {
	s.mu.Lock()
	for i := range s.state.Runtimes {
		if s.state.Runtimes[i].ID != id {
			continue
		}
		previous := s.state.Runtimes[i]
		item := s.state.Runtimes[i]
		hydrateRuntimePaths(&item)
		item.TaskStatus = strings.ToLower(status)
		item.UpdatedAt = time.Now().UTC()
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "running":
			item.Status, item.Message, item.Error = "Running", "", ""
		case "failed", "error":
			item.Status, item.Message, item.Error = "Error", message, message
		case "building":
			item.Status = "Building"
		case "recreating":
			item.Status = "ReCreating"
		case "restarting":
			item.Status = "Restarting"
		case "stopped":
			item.Status = "Stopped"
		case "downloading", "installing", "pulling", "starting":
			item.Status = "Creating"
		default:
			item.Status = "Creating"
		}
		if strings.TrimSpace(message) != "" && strings.ToLower(strings.TrimSpace(status)) != "running" {
			item.Message = message
		}
		s.state.Runtimes[i] = item
		if err := s.saveLocked(); err != nil {
			s.state.Runtimes[i] = previous
			s.mu.Unlock()
			return runtimeRecord{}, err
		}
		s.mu.Unlock()
		if err := persistRuntimeTaskChecked(item); err != nil {
			s.mu.Lock()
			for index := range s.state.Runtimes {
				if s.state.Runtimes[index].ID == id &&
					s.state.Runtimes[index].TaskStatus == item.TaskStatus &&
					s.state.Runtimes[index].UpdatedAt.Equal(item.UpdatedAt) {
					s.state.Runtimes[index] = previous
					if saveErr := s.saveLocked(); saveErr != nil {
						s.mu.Unlock()
						return runtimeRecord{}, fmt.Errorf("%w; 回滚运行时状态失败: %v", err, saveErr)
					}
					break
				}
			}
			s.mu.Unlock()
			return runtimeRecord{}, err
		}
		return item, nil
	}
	s.mu.Unlock()
	return runtimeRecord{}, errors.New("运行时不存在")
}

// persistRuntimeTask 保留旧调用方签名；新任务流程使用 checked 版本接收错误。
func persistRuntimeTask(item runtimeRecord) {
	_ = persistRuntimeTaskChecked(item)
}

func persistRuntimeTaskChecked(item runtimeRecord) error {
	if item.TaskID == "" {
		return nil
	}
	repository, err := sharedRuntimeRepository()
	if err != nil {
		return nil
	}
	status := strings.ToLower(strings.TrimSpace(item.TaskStatus))
	if status == "" {
		status = "installing"
	}
	writer, err := storage.NewSQLiteTaskStateWriter(repository)
	if err != nil {
		return err
	}
	return writer.UpsertRuntime(context.Background(), storage.RuntimeTaskState{
		ID: item.TaskID, RuntimeID: item.ID, Status: status, Step: status,
		Progress: appTaskProgress(status), Message: item.Message, Error: item.Error,
		CreatedAt: item.UpdatedAt, UpdatedAt: item.UpdatedAt,
	})
}

// runtimeDownloadCompose 下载并解压运行时归档，返回待写入的 Compose 内容。
func runtimeDownloadCompose(root string, item runtimeRecord, installDir string, update func(string, string)) (string, error) {
	return runtimeDownloadComposeChecked(root, item, installDir, func(status, message string) error {
		update(status, message)
		return nil
	})
}

func runtimeDownloadComposeChecked(root string, item runtimeRecord, installDir string, update func(string, string) error) (string, error) {
	compose := strings.TrimSpace(item.DockerCompose)
	if item.DownloadURL != "" {
		if err := update("downloading", "正在下载运行时归档"); err != nil {
			return "", err
		}
		downloadDir := filepath.Join(root, "runtimes", ".downloads")
		if err := os.MkdirAll(downloadDir, 0o750); err != nil {
			return "", fmt.Errorf("创建下载目录失败: %w", err)
		}
		archivePath := filepath.Join(downloadDir, item.ID+".tar.gz")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		var progressErr error
		err := downloadAppArchiveWithProgress(ctx, item.DownloadURL, archivePath, func(downloaded, total int64) {
			if progressErr != nil {
				return
			}
			if total > 0 {
				progressErr = update("downloading", fmt.Sprintf("正在下载运行时归档: %d%% (%s/%s)", downloaded*100/total, formatRuntimeBytes(downloaded), formatRuntimeBytes(total)))
				return
			}
			progressErr = update("downloading", fmt.Sprintf("正在下载运行时归档: %s", formatRuntimeBytes(downloaded)))
		})
		cancel()
		if err != nil {
			return "", fmt.Errorf("下载运行时归档失败: %w", err)
		}
		if progressErr != nil {
			return "", progressErr
		}
		if err := update("installing", "正在解压运行时归档"); err != nil {
			return "", err
		}
		if err := deployRuntimeArchive(archivePath, installDir, item.Type, item.Version); err != nil {
			return "", fmt.Errorf("解压运行时归档失败: %w", err)
		}
		_ = os.Remove(archivePath)
		packageCompose, err := os.ReadFile(filepath.Join(installDir, "docker-compose.yml"))
		if err != nil {
			return "", fmt.Errorf("运行时包缺少 docker-compose.yml: %w", err)
		}
		compose = string(packageCompose)
	} else if err := os.MkdirAll(installDir, 0o750); err != nil {
		return "", fmt.Errorf("创建运行目录失败: %w", err)
	}
	if compose == "" && item.Image != "" {
		compose = fmt.Sprintf("services:\n  runtime:\n    image: %s\n    container_name: %s\n", item.Image, item.Container)
		if item.Port > 0 {
			compose += fmt.Sprintf("    ports:\n      - \"%d:%d\"\n", item.Port, item.Port)
		}
	}
	if compose == "" {
		return "", errors.New("运行时未提供 Docker Compose 或镜像")
	}
	return compose, nil
}

// persistRuntimeInstallFiles 写入 Compose、环境变量并同步运行时 SQLite 记录。
func persistRuntimeInstallFiles(s *runtimeStore, item runtimeRecord, installDir, compose string) (runtimeRecord, error) {
	composePath := filepath.Join(installDir, "docker-compose.yml")
	if item.DownloadURL == "" {
		if err := writeAtomicRuntimeFile(composePath, []byte(compose)); err != nil {
			return item, fmt.Errorf("写入 Compose 文件失败: %w", err)
		}
	}
	if err := writeRuntimeComposeOverride(installDir, item); err != nil {
		return item, fmt.Errorf("写入 Compose 文件失败: %w", err)
	}
	item.ComposePath, item.DockerCompose = composePath, compose
	env, err := runtimeEnvironment(item)
	if err != nil {
		return item, err
	}
	if normalizeRuntimeTypeFilter(item.Type) == "php" {
		phpVersion := runtimeString(item.Params, "PHP_VERSION")
		if phpVersion == "" {
			return item, errors.New("PHP_VERSION 不能为空")
		}
		item.Image = "1panel-php-fpm:" + phpVersion
		env["IMAGE_NAME"] = item.Image
	}
	if err := writeRuntimeEnv(filepath.Join(installDir, ".env"), env); err != nil {
		return item, fmt.Errorf("写入环境变量失败: %w", err)
	}
	s.mu.Lock()
	previousRuntimes := append([]runtimeRecord(nil), s.state.Runtimes...)
	found := false
	for i := range s.state.Runtimes {
		if s.state.Runtimes[i].ID == item.ID {
			found = true
			s.state.Runtimes[i] = item
			if err := s.saveLocked(); err != nil {
				s.state.Runtimes = previousRuntimes
				s.mu.Unlock()
				return item, err
			}
			break
		}
	}
	s.mu.Unlock()
	if !found {
		return item, errors.New("运行时不存在")
	}
	return item, nil
}

// runRuntimeInstallTask 执行运行时安装任务，并将进度写入共享任务日志。
func runRuntimeInstallTask(s *runtimeStore, item runtimeRecord) {
	unlock := s.operations.lock(item.ID)
	defer unlock()
	current, index := runtimeByID(s, item.ID)
	if index < 0 || current.TaskID != item.TaskID {
		// 创建返回后可能先发生删除或同名重建，旧任务不得安装到新的资源上。
		return
	}
	taskID := item.TaskID
	if taskID == "" {
		taskID = idToken()
	}
	item.TaskID = taskID
	s.mu.Lock()
	for index := range s.state.Runtimes {
		if s.state.Runtimes[index].ID == item.ID {
			previous := s.state.Runtimes[index]
			s.state.Runtimes[index].TaskID = taskID
			if err := s.saveLocked(); err != nil {
				s.state.Runtimes[index] = previous
				s.mu.Unlock()
				return
			}
			break
		}
	}
	s.mu.Unlock()
	defer appendRuntimeTaskLog(taskID, "[TASK-END]")
	var taskPersistenceErr error
	update := func(status, message string) error {
		if taskPersistenceErr != nil {
			return taskPersistenceErr
		}
		var err error
		item, err = updateRuntimeTaskChecked(s, item.ID, status, message)
		if err != nil {
			taskPersistenceErr = fmt.Errorf("运行时任务状态保存失败: %w", err)
			return taskPersistenceErr
		}
		if err := ensureAppTaskLogChecked(taskID, item.ID, item.Name, status, message); err != nil {
			taskPersistenceErr = fmt.Errorf("运行时任务兼容状态保存失败: %w", err)
			return taskPersistenceErr
		}
		if err := appendRuntimeTaskLogChecked(taskID, message); err != nil {
			taskPersistenceErr = fmt.Errorf("运行时任务日志保存失败: %w", err)
		}
		return taskPersistenceErr
	}
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	installDir := filepath.Join(root, "runtimes", strings.ToLower(item.Type), item.Name)
	item.InstallPath, item.ComposePath = installDir, filepath.Join(installDir, "docker-compose.yml")
	if err := persistRuntimeInstallPath(s, item.ID, installDir); err != nil {
		update("failed", "运行时安装路径保存失败: "+err.Error())
		return
	}
	compose, err := runtimeDownloadComposeChecked(root, item, installDir, update)
	if err != nil {
		if taskPersistenceErr == nil {
			update("failed", err.Error())
		}
		return
	}
	if taskPersistenceErr != nil {
		return
	}
	item, err = persistRuntimeInstallFiles(s, item, installDir, compose)
	if err != nil {
		update("failed", err.Error())
		return
	}
	if taskPersistenceErr != nil {
		return
	}
	if err := ensureRuntimeNetwork(s.commandExecutor(), "1panel-network"); err != nil {
		update("failed", "创建运行时网络失败: "+err.Error())
		return
	}
	if taskPersistenceErr != nil {
		return
	}
	if err := executeRuntimeInstallChecked(s.commandExecutor(), item, func(status, message string) error {
		return update(status, message)
	}); err != nil {
		if taskPersistenceErr == nil {
			update("failed", err.Error())
		}
		return
	}
	if taskPersistenceErr != nil {
		return
	}
	status, err := runtimeContainerStatus(s.commandExecutor(), item)
	if err != nil {
		update("failed", "运行时容器状态检查失败: "+err.Error())
		return
	}
	if taskPersistenceErr != nil {
		return
	}
	if !strings.EqualFold(status, "running") {
		update("failed", fmt.Sprintf("运行时容器未正常运行（当前状态: %s）", status))
		return
	}
	update("running", "运行时安装完成")
}
