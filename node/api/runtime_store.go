// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var runtimeStoreMu sync.Mutex
var runtimeStoreInstance *runtimeStore

func sharedRuntimeRepository() (*storage.SQLiteRepository, error) {
	db := sharedDB()
	if db == nil {
		return nil, errors.New("运行时 SQLite 未初始化")
	}
	return storage.NewSQLiteRepository(db)
}

type runtimeStore struct {
	mu         sync.RWMutex
	operations runtimeOperationLocks
	path       string
	repository runtimeRepository
	commands   runtimeCommandExecutor
	state      runtimeState
	loadErr    error
}

type runtimeCommandExecutor interface {
	Execute(context.Context, model.CommandRequest) (model.CommandResult, error)
}

// getRuntimeStore 执行运行时相关处理并返回可观测错误。
func getRuntimeStore() *runtimeStore {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = "./data"
	}
	// path 仅用于缓存实例身份；业务状态始终来自共享 SQLite。
	path := filepath.Join(dir, "workmesh.db")
	runtimeStoreMu.Lock()
	defer runtimeStoreMu.Unlock()
	if runtimeStoreInstance != nil && runtimeStoreInstance.path == path && runtimeStoreInstance.repository.identityDB() == sharedDB() {
		return runtimeStoreInstance
	}
	repository, repositoryErr := storage.NewSQLiteRepository(sharedDB())
	s := &runtimeStore{path: path, repository: runtimeRepository{db: sharedDB()}, commands: service.CommandService{}, state: runtimeState{Runtimes: []runtimeRecord{}, Settings: map[string]any{}}}
	if repositoryErr != nil {
		s.loadErr = errors.New("运行时 SQLite 未初始化")
	} else {
		s.repository.repository = repository
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		s.state, s.loadErr = s.repository.load(ctx)
		cancel()
	}
	if s.state.Settings == nil {
		s.state.Settings = map[string]any{}
	}
	interrupted := recoverInterruptedRuntimeTasks(s)
	if interrupted && s.loadErr == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		s.loadErr = s.repository.save(ctx, s.state)
		cancel()
	}
	if s.loadErr == nil {
		for _, item := range s.state.Runtimes {
			if strings.TrimSpace(item.TaskID) != "" && strings.EqualFold(item.Status, "Error") {
				persistRuntimeTask(item)
			}
		}
	}
	if _, exists := s.state.Settings[phpExtensionTemplatesSetting]; !exists {
		s.state.Settings[phpExtensionTemplatesSetting] = append([]phpExtensionTemplate(nil), defaultPHPExtensionTemplates...)
		if s.loadErr == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			s.loadErr = s.repository.save(ctx, s.state)
			cancel()
		}
	}
	runtimeStoreInstance = s
	return s
}

// recoverInterruptedRuntimeTasks 将重启时未完成的运行时任务收敛为可审计失败状态。
func recoverInterruptedRuntimeTasks(s *runtimeStore) bool {
	interrupted := false
	for index := range s.state.Runtimes {
		current := &s.state.Runtimes[index]
		status := strings.ToLower(strings.TrimSpace(current.Status))
		taskStatus := strings.ToLower(strings.TrimSpace(current.TaskStatus))
		if (taskStatus == "failed" || taskStatus == "error") && status != "error" {
			if current.TaskID != "" {
				var message, taskError string
				if messageValue, errorValue, err := s.repository.taskMessage(current.TaskID); err == nil {
					message, taskError = messageValue, errorValue
					if strings.TrimSpace(current.Message) == "" {
						current.Message = message
					}
					if strings.TrimSpace(current.Error) == "" {
						current.Error = taskError
					}
				}
			}
			current.Status = "Error"
			if strings.TrimSpace(current.Message) == "" {
				current.Message = current.Error
			}
			if strings.TrimSpace(current.Error) == "" {
				current.Error = current.Message
			}
			current.UpdatedAt = time.Now().UTC()
			interrupted = true
			continue
		}
		switch status {
		case "creating", "building", "recreating", "installing", "downloading", "pulling", "starting":
			current.Status = "Error"
			current.TaskStatus = "failed"
			current.Message = "系统重启导致任务中断"
			current.Error = "系统重启导致任务中断"
			current.UpdatedAt = time.Now().UTC()
			interrupted = true
		}
	}
	return interrupted
}

// saveLocked 持久化运行时状态，并保持重启恢复一致性。
func (s *runtimeStore) saveLocked() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.repository.save(ctx, s.state); err != nil {
		if restored, loadErr := s.repository.load(ctx); loadErr == nil {
			s.state = restored
		}
		return err
	}
	s.loadErr = nil
	return nil
}

// commandExecutor 执行运行时相关处理并返回可观测错误。
func (s *runtimeStore) commandExecutor() runtimeCommandExecutor {
	if s.commands != nil {
		return s.commands
	}
	return service.CommandService{}
}
