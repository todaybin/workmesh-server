// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

func findCatalogDetail(catalog []appRecord, detailID string) (appRecord, appVersionRecord, bool) {
	for _, app := range catalog {
		for _, version := range app.Versions {
			if strings.EqualFold(strings.TrimSpace(version.ID), strings.TrimSpace(detailID)) {
				return app, version, true
			}
		}
	}
	return appRecord{}, appVersionRecord{}, false
}

func selectAppDownloadURL(original string, body map[string]any) string {
	original = strings.TrimSpace(original)
	if original == "" {
		return original
	}
	mirror := appValue(body, "mirror", "mirrorUrl", "accelerateUrl", "downloadMirror")
	if mirror == "" {
		mirror = strings.TrimSpace(os.Getenv("WORKMESH_APP_MIRROR"))
	}
	if mirror == "" {
		return original
	}
	mirror = strings.TrimRight(mirror, "/")
	if strings.Contains(mirror, "{url}") {
		return strings.ReplaceAll(mirror, "{url}", original)
	}
	name := filepath.Base(strings.TrimSpace(strings.SplitN(original, "?", 2)[0]))
	if name == "." || name == "" {
		return original
	}
	return mirror + "/" + name
}

func persistAppInstallRecord(item appRecord, composePath string) error {
	repository, err := SharedRepository()
	if err != nil {
		if sharedDB() == nil {
			return nil
		}
		return err
	}
	if repository == nil {
		return nil
	}
	config, err := json.Marshal(item.Config)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
		_, err := tx.Exec(`INSERT INTO app_installs(id,app_key,name,version,status,install_path,compose_path,compose_project,container_names,config_json,message,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET app_key=excluded.app_key,name=excluded.name,version=excluded.version,status=excluded.status,install_path=excluded.install_path,compose_path=excluded.compose_path,container_names=excluded.container_names,config_json=excluded.config_json,message=excluded.message,updated_at=excluded.updated_at`,
			item.ID, item.Key, item.Name, item.Version, item.Status, appInstallPath(item), composePath, appValue(item.Config, "composeProject", "projectName"), item.ContainerName, config, item.Message, now, now)
		return err
	})
}

func appInstallPath(item appRecord) string {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	name := item.Name
	if !validDockerIdentifier(name) {
		name = item.ID
	}
	return filepath.Join(root, "apps", item.Key, name)
}

func appTaskLogPath(taskID string) string {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	return filepath.Join(root, "logs", "tasks", "app", taskID+".log")
}

func ensureAppTaskLog(taskID, installID, name, status, message string) {
	_ = ensureAppTaskLogChecked(taskID, installID, name, status, message)
}

// ensureAppTaskLogChecked writes task state and its first log line as one
// SQLite transaction, returning storage failures to the owning task.
func ensureAppTaskLogChecked(taskID, installID, name, status, message string) error {
	if strings.TrimSpace(taskID) == "" {
		return errors.New("任务 ID 不能为空")
	}
	terminal := isTerminalTaskStatus(status)
	acquired := false
	if !terminal {
		var ok bool
		acquired, ok = acquireManagedSlot(managedRuntimeSlots.tasks, taskID, nodeRuntimeLimits.tasks)
		if !ok {
			return errors.New("并发任务已达到上限")
		}
	}
	committed := false
	defer func() {
		if acquired && !committed {
			releaseManagedSlot(managedRuntimeSlots.tasks, taskID)
		}
	}()
	path := appTaskLogPath(taskID)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	if repository, repositoryErr := SharedRepository(); repositoryErr == nil {
		now := time.Now().UTC()
		if err := repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
			stateWriter, err := storage.NewSQLiteTaskStateWriter(tx)
			if err != nil {
				return err
			}
			if err := stateWriter.UpsertApp(context.Background(), storage.AppTaskState{
				ID: taskID, InstallID: installID, Status: status, Step: status,
				Progress: appTaskProgress(status), Message: message, Error: appTaskError(status, message),
				LogPath: path, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				return err
			}
			if strings.TrimSpace(message) == "" {
				return nil
			}
			logWriter, err := storage.NewSQLiteTaskLogWriter(tx)
			if err != nil {
				return err
			}
			return logWriter.Append(context.Background(), storage.TaskLogEntry{TaskID: taskID, Line: message, CreatedAt: now})
		}); err != nil {
			return err
		}
	} else if sharedDB() != nil {
		return repositoryErr
	} else if err := appendAppTaskLogChecked(taskID, message); err != nil {
		return err
	}

	committed = true
	if terminal {
		releaseManagedSlot(managedRuntimeSlots.tasks, taskID)
	}
	return nil
}

func appendAppTaskLog(taskID, message string) {
	_ = appendAppTaskLogChecked(taskID, message)
}

func appendAppTaskLogChecked(taskID, message string) error {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(message) == "" {
		return nil
	}
	if repository, repositoryErr := SharedRepository(); repositoryErr == nil {
		writer, err := storage.NewSQLiteTaskLogWriter(repository)
		if err != nil {
			return err
		}
		return writer.Append(context.Background(), storage.TaskLogEntry{TaskID: taskID, Line: message, CreatedAt: time.Now().UTC()})
	} else if sharedDB() != nil {
		return repositoryErr
	}
	path := appTaskLogPath(taskID)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintf(file, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), strings.TrimSpace(message))
	return err
}

func appTaskLevel(status string) string {
	switch strings.ToLower(status) {
	case "running":
		return "Success"
	case "cancelled", "canceled", "stopped":
		return "Canceled"
	case "failed", "error":
		return "Failed"
	default:
		return "Executing"
	}
}

func appTaskProgress(status string) int {
	switch strings.ToLower(status) {
	case "downloading":
		return 20
	case "installing":
		return 40
	case "building":
		return 60
	case "pulling":
		return 65
	case "starting":
		return 85
	case "recreating":
		return 90
	case "running", "failed", "cancelled", "canceled", "stopped":
		return 100
	default:
		return 0
	}
}

func appTaskError(status, message string) string {
	if strings.EqualFold(status, "failed") {
		return message
	}
	return ""
}
