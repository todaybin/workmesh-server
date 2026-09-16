// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

// CronjobService 保存计划任务定义并提供调度、执行和记录能力。
type CronjobService struct {
	mu      sync.RWMutex
	initMu  sync.Mutex
	startMu sync.Mutex
	started bool
	items   map[string]model.Cronjob
	cmd     CommandService
	records map[string][]model.CommandResult
	// path 仅作为旧 cronjobs.json 的一次性迁移输入。
	path         string
	fallbackPath string
	running      map[string]context.CancelFunc
	lastTick     map[string]string
	db           *sql.DB
	repository   storage.Transactional
}

// NewCronjobService 创建计划任务服务。
func NewCronjobService() *CronjobService {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = "./data"
	}
	return &CronjobService{
		items: make(map[string]model.Cronjob), records: make(map[string][]model.CommandResult),
		path: filepath.Join(dir, "cronjobs.json"), fallbackPath: filepath.Join(dir, "workmesh.db"),
		running: make(map[string]context.CancelFunc), lastTick: make(map[string]string),
	}
}

// ensureDatabase 在首次业务调用时绑定统一连接；这样包级服务不会在 main 注入数据库前固化旧数据源。
func (s *CronjobService) ensureDatabase(ctx context.Context) error {
	db := SharedDatabase()
	if db == nil {
		var err error
		db, err = sharedFallbackSQLite(s.fallbackPath)
		if err != nil {
			return err
		}
	}
	s.initMu.Lock()
	defer s.initMu.Unlock()
	if s.db == db && s.repository != nil {
		return nil
	}
	if s.db == db {
		repository, err := storage.NewSQLiteRepository(db)
		if err != nil {
			return err
		}
		s.repository = repository
		return nil
	}
	items, records, err := s.initializeDatabase(contextOrBackground(ctx), db)
	if err != nil {
		return err
	}
	repository, err := storage.NewSQLiteRepository(db)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.items, s.records, s.db, s.repository = items, records, db, repository
	s.mu.Unlock()
	return nil
}

func (s *CronjobService) initializeDatabase(ctx context.Context, db *sql.DB) (map[string]model.Cronjob, map[string][]model.CommandResult, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS cronjobs (id TEXT PRIMARY KEY, payload BLOB NOT NULL, records BLOB NOT NULL DEFAULT '[]', updated_at TEXT NOT NULL)`); err != nil {
		return nil, nil, err
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cronjobs`).Scan(&count); err != nil {
		return nil, nil, err
	}
	if count == 0 {
		if payload, err := os.ReadFile(s.path); err == nil {
			var legacy struct {
				Items   map[string]model.Cronjob         `json:"items"`
				Records map[string][]model.CommandResult `json:"records"`
			}
			if json.Unmarshal(payload, &legacy) == nil {
				for id, job := range legacy.Items {
					records, _ := json.Marshal(legacy.Records[id])
					jobPayload, _ := json.Marshal(job)
					if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO cronjobs(id,payload,records,updated_at) VALUES(?,?,?,?)`, id, jobPayload, records, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
						return nil, nil, fmt.Errorf("导入旧计划任务失败: %w", err)
					}
				}
			}
		}
	}
	rows, err := db.QueryContext(ctx, `SELECT id,payload,records FROM cronjobs ORDER BY id LIMIT 10000`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	items := make(map[string]model.Cronjob)
	recordsByID := make(map[string][]model.CommandResult)
	for rows.Next() {
		var id string
		var jobPayload, recordsPayload []byte
		if rows.Scan(&id, &jobPayload, &recordsPayload) != nil {
			continue
		}
		var job model.Cronjob
		var records []model.CommandResult
		if json.Unmarshal(jobPayload, &job) == nil {
			items[id] = job
		}
		if json.Unmarshal(recordsPayload, &records) == nil {
			recordsByID[id] = records
		}
	}
	return items, recordsByID, rows.Err()
}

// Create 新增计划任务，初始状态为 disabled。
func (s *CronjobService) Create(ctx context.Context, job model.Cronjob) (model.Cronjob, error) {
	if err := s.ensureDatabase(ctx); err != nil {
		return model.Cronjob{}, err
	}
	if strings.TrimSpace(job.Name) == "" {
		return model.Cronjob{}, errors.New("计划任务名称不能为空")
	}
	if strings.TrimSpace(job.Type) == "" {
		job.Type = "shell"
	}
	if !validCronType(job.Type) {
		return model.Cronjob{}, fmt.Errorf("不支持的计划任务类型: %s", job.Type)
	}
	if strings.TrimSpace(job.Spec) == "" {
		job.Spec = "* * * * *"
	}
	if job.Type == "shell" && strings.TrimSpace(job.Command) == "" && strings.TrimSpace(job.Script) == "" {
		return model.Cronjob{}, errors.New("脚本或命令不能为空")
	}
	job.ID = newID()
	job.Status = "disabled"
	now := time.Now().UTC().Format(time.RFC3339)
	job.CreatedAt, job.UpdatedAt = now, now
	s.mu.Lock()
	s.items[job.ID] = job
	err := s.persistLocked(ctx, job.ID)
	if err != nil {
		delete(s.items, job.ID)
	}
	s.mu.Unlock()
	if err != nil {
		return model.Cronjob{}, err
	}
	return job, nil
}

// List 返回全部计划任务定义。
func (s *CronjobService) List(ctx context.Context) []model.Cronjob {
	if err := s.ensureDatabase(ctx); err != nil {
		return []model.Cronjob{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]model.Cronjob, 0, len(s.items))
	for _, item := range s.items {
		result = append(result, item)
	}
	return result
}

// ListPage 按页返回计划任务，防止前端一次读取无界数据。
func (s *CronjobService) ListPage(ctx context.Context, page, pageSize int) (int, []model.Cronjob) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	items := s.List(ctx)
	total := len(items)
	start := (page - 1) * pageSize
	if start >= total {
		return total, []model.Cronjob{}
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return total, items[start:end]
}

// Delete 删除指定计划任务。
func (s *CronjobService) Delete(ctx context.Context, id string) error {
	if err := s.ensureDatabase(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return errors.New("计划任务不存在")
	}
	repository, err := s.executor()
	if err != nil {
		return err
	}
	if _, err := repository.ExecContext(contextOrBackground(ctx), `DELETE FROM cronjobs WHERE id=?`, id); err != nil {
		return err
	}
	delete(s.items, id)
	delete(s.records, id)
	return nil
}

// HandleOnce 立即执行计划任务的命令字段。
