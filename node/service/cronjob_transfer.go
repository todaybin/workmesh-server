// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

func NextRuns(spec string, from time.Time, count int) ([]time.Time, error) {
	if count < 1 || count > 20 {
		count = 5
	}
	out := make([]time.Time, 0, count)
	cur := from
	for i := 0; i < count; i++ {
		next, ok := nextCronRun(spec, cur)
		if !ok {
			return nil, errors.New("无效的 cron 表达式")
		}
		out = append(out, next)
		cur = next
	}
	return out, nil
}
func (s *CronjobService) Get(ctx context.Context, id string) (model.Cronjob, bool) {
	if err := s.ensureDatabase(ctx); err != nil {
		return model.Cronjob{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.items[id]
	return v, ok
}

// executor returns the initialized repository for runtime cronjob state writes.
// Table creation and legacy JSON import intentionally remain in initialization.
func (s *CronjobService) executor() (storage.Transactional, error) {
	if s.repository != nil {
		return s.repository, nil
	}
	if s.db == nil {
		return nil, errors.New("计划任务数据库未初始化")
	}
	repository, err := storage.NewSQLiteRepository(s.db)
	if err != nil {
		return nil, err
	}
	s.repository = repository
	return repository, nil
}

// persistLocked 只更新发生变化的计划任务行，调用方必须已持有 s.mu 写锁。
func (s *CronjobService) persistLocked(ctx context.Context, id string) error {
	repository, err := s.executor()
	if err != nil {
		return err
	}
	job, ok := s.items[id]
	if !ok {
		_, err := repository.ExecContext(contextOrBackground(ctx), `DELETE FROM cronjobs WHERE id=?`, id)
		return err
	}
	jobPayload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	records, err := json.Marshal(s.records[id])
	if err != nil {
		return err
	}
	_, err = repository.ExecContext(contextOrBackground(ctx), `INSERT INTO cronjobs(id,payload,records,updated_at) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,records=excluded.records,updated_at=excluded.updated_at`, id, jobPayload, records, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// Export 返回稳定 JSON，供旧版导入导出接口适配。
func (s *CronjobService) Export(context.Context) ([]byte, error) {
	return json.Marshal(map[string]any{"cronjobs": s.List(context.Background())})
}

// Import 导入 JSON 中的 cronjobs 数组。
func (s *CronjobService) Import(ctx context.Context, payload []byte) error {
	var envelope struct {
		Cronjobs []model.Cronjob `json:"cronjobs"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return err
	}
	for _, job := range envelope.Cronjobs {
		if _, err := s.Create(ctx, job); err != nil {
			return err
		}
	}
	return nil
}

func newID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "local-cronjob"
	}
	return hex.EncodeToString(raw[:])
}
