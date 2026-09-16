// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

const runtimeSchemaSQL = `
CREATE TABLE IF NOT EXISTS runtime_records (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    version TEXT NOT NULL DEFAULT '',
    code_dir TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    remark TEXT NOT NULL DEFAULT '',
    extensions BLOB NOT NULL DEFAULT '[]',
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_runtime_records_type_status ON runtime_records(type, status, name, id);
ALTER TABLE runtime_records ADD COLUMN payload BLOB NOT NULL DEFAULT '{}';
CREATE TABLE IF NOT EXISTS runtime_settings (
    setting_key TEXT PRIMARY KEY,
    payload BLOB NOT NULL,
    updated_at TEXT NOT NULL
);`

const runtimeTaskSchemaSQL = `
CREATE TABLE IF NOT EXISTS runtime_tasks (
    id TEXT PRIMARY KEY,
    runtime_id TEXT NOT NULL,
    status TEXT NOT NULL,
    step TEXT NOT NULL DEFAULT '',
    progress INTEGER NOT NULL DEFAULT 0,
    message TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_runtime_tasks_status ON runtime_tasks(status, updated_at);
CREATE TABLE IF NOT EXISTS runtime_task_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT NOT NULL,
    line TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_runtime_task_logs_task ON runtime_task_logs(task_id, id);`

const runtimeAttributesSQL = `
CREATE TABLE IF NOT EXISTS runtime_attributes (
    runtime_id TEXT NOT NULL,
    attribute_key TEXT NOT NULL,
    attribute_value BLOB NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY(runtime_id, attribute_key)
);`

const maxLegacyRuntimeBytes int64 = 16 << 20

// RuntimeSchemaMigration 返回运行时清单及设置的版本化 SQLite 迁移。
func RuntimeSchemaMigration() storage.Migration {
	return storage.SQLMigration("0009-runtime-state-v2", strings.Replace(runtimeSchemaSQL, "ALTER TABLE runtime_records ADD COLUMN payload BLOB NOT NULL DEFAULT '{}';", "", 1))
}

// RuntimePayloadMigration 执行运行时相关处理并返回可观测错误。
func RuntimePayloadMigration() storage.Migration {
	return storage.SQLMigration("0010-runtime-payload", "ALTER TABLE runtime_records ADD COLUMN payload BLOB NOT NULL DEFAULT '{}';")
}

// RuntimeCreatedAtMigration 为旧运行时记录补充稳定的创建时间列。
func RuntimeCreatedAtMigration() storage.Migration {
	return storage.SQLMigration("0011-runtime-created-at", "ALTER TABLE runtime_records ADD COLUMN created_at TEXT NOT NULL DEFAULT '';")
}

// RuntimeAttributesMigration 执行运行时相关处理并返回可观测错误。
func RuntimeAttributesMigration() storage.Migration {
	return storage.SQLMigration("0012-runtime-task-state", runtimeTaskSchemaSQL+runtimeAttributesSQL)
}

type runtimeRepository struct {
	db         *sql.DB
	repository storage.Transactional
}

func (r runtimeRepository) executor() (storage.Transactional, error) {
	if r.repository != nil {
		return r.repository, nil
	}
	if r.db == nil {
		return nil, errors.New("运行时 SQLite 未初始化")
	}
	repository, err := storage.NewSQLiteRepository(r.db)
	if err != nil {
		return nil, err
	}
	return repository, nil
}

func (r runtimeRepository) identityDB() *sql.DB {
	if r.db != nil {
		return r.db
	}
	if repository, ok := r.repository.(*storage.SQLiteRepository); ok {
		return repository.DB()
	}
	return nil
}

func (r runtimeRepository) taskMessage(taskID string) (string, string, error) {
	repository, err := r.executor()
	if err != nil {
		return "", "", err
	}
	var message, taskError string
	if err := repository.QueryRow(`SELECT message,error FROM runtime_tasks WHERE id=?`, taskID).Scan(&message, &taskError); err != nil {
		return "", "", err
	}
	return message, taskError, nil
}

// initializeRuntimePersistence 执行运行时相关处理并返回可观测错误。
func initializeRuntimePersistence(db *sql.DB, dataDir string) error {
	if db == nil {
		return errors.New("运行时 SQLite 连接不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := storage.ApplyMigrations(ctx, db, []storage.Migration{RuntimeSchemaMigration(), RuntimePayloadMigration(), RuntimeCreatedAtMigration(), RuntimeAttributesMigration()}); err != nil {
		return fmt.Errorf("执行运行时存储迁移失败: %w", err)
	}
	if err := migrateLegacyRuntimeState(ctx, db, dataDir); err != nil {
		return fmt.Errorf("导入旧运行时数据失败: %w", err)
	}
	return nil
}

// load 执行运行时相关处理并返回可观测错误。
func (r runtimeRepository) load(ctx context.Context) (runtimeState, error) {
	state := runtimeState{Runtimes: []runtimeRecord{}, Settings: map[string]any{}}
	repository, err := r.executor()
	if err != nil {
		return state, err
	}
	rows, err := repository.QueryContext(ctx, `SELECT id,name,type,version,code_dir,status,remark,extensions,created_at,updated_at,payload FROM runtime_records ORDER BY name,id`)
	if err != nil {
		return state, fmt.Errorf("读取运行时清单失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item runtimeRecord
		var extensions []byte
		var createdAt, updatedAt string
		var payload []byte
		if err := rows.Scan(&item.ID, &item.Name, &item.Type, &item.Version, &item.CodeDir, &item.Status, &item.Remark, &extensions, &createdAt, &updatedAt, &payload); err != nil {
			return state, err
		}
		// 仅旧记录回退读取 payload；新记录由关系字段和 runtime_attributes 提供权威数据。
		var attrCount int
		_ = repository.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_attributes WHERE runtime_id=?`, item.ID).Scan(&attrCount)
		if attrCount == 0 && len(payload) > 0 && string(payload) != "{}" {
			_ = json.Unmarshal(payload, &item)
		}
		if attrCount > 0 {
			attrs, attrErr := repository.QueryContext(ctx, `SELECT attribute_key,attribute_value FROM runtime_attributes WHERE runtime_id=?`, item.ID)
			if attrErr != nil {
				return state, attrErr
			}
			values := map[string]any{}
			for attrs.Next() {
				var key string
				var raw []byte
				if err := attrs.Scan(&key, &raw); err != nil {
					attrs.Close()
					return state, err
				}
				var value any
				if json.Unmarshal(raw, &value) == nil {
					values[key] = value
				}
			}
			if err := attrs.Err(); err != nil {
				attrs.Close()
				return state, err
			}
			attrs.Close()
			if encoded, marshalErr := json.Marshal(values); marshalErr == nil {
				_ = json.Unmarshal(encoded, &item)
			}
			// 兼容新增字段：旧 runtime_attributes 没有 mode 时，从完整 payload
			// 补回运行方式，避免宿主机运行时被误判为 Docker。
			if strings.TrimSpace(item.Mode) == "" && len(payload) > 0 && string(payload) != "{}" {
				var persisted runtimeRecord
				if json.Unmarshal(payload, &persisted) == nil {
					item.Mode = persisted.Mode
				}
			}
		}
		if len(extensions) > 0 && json.Unmarshal(extensions, &item.Extensions) != nil {
			return state, fmt.Errorf("运行时 %s 的扩展数据无效", item.ID)
		}
		if item.Extensions == nil {
			item.Extensions = []string{}
		}
		item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return state, err
		}
		if strings.TrimSpace(createdAt) != "" {
			item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
			if err != nil {
				return state, err
			}
		}
		if item.CreatedAt.IsZero() {
			item.CreatedAt = item.UpdatedAt
		}
		state.Runtimes = append(state.Runtimes, item)
	}
	if err := rows.Err(); err != nil {
		return state, err
	}
	settingRows, err := repository.QueryContext(ctx, `SELECT setting_key,payload FROM runtime_settings ORDER BY setting_key`)
	if err != nil {
		return state, err
	}
	defer settingRows.Close()
	for settingRows.Next() {
		var key string
		var payload []byte
		if err := settingRows.Scan(&key, &payload); err != nil {
			return state, err
		}
		var value any
		if err := json.Unmarshal(payload, &value); err != nil {
			return state, err
		}
		state.Settings[key] = value
	}
	if err := settingRows.Err(); err != nil {
		return state, err
	}
	return state, nil
}

// save 持久化运行时状态，并保持重启恢复一致性。
func (r runtimeRepository) save(ctx context.Context, state runtimeState) error {
	repository, err := r.executor()
	if err != nil {
		return err
	}
	return repository.WithTx(ctx, func(tx storage.SQLExecutor) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM runtime_records`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM runtime_attributes`); err != nil {
			return err
		}
		for _, item := range state.Runtimes {
			ext, err := json.Marshal(item.Extensions)
			if err != nil {
				return err
			}
			updated := item.UpdatedAt.UTC()
			if updated.IsZero() {
				updated = time.Now().UTC()
			}
			created := item.CreatedAt.UTC()
			if created.IsZero() {
				created = updated
			}
			payload, err := json.Marshal(item)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_records(id,name,type,version,code_dir,status,remark,extensions,created_at,updated_at,payload) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, item.ID, item.Name, item.Type, item.Version, item.CodeDir, item.Status, item.Remark, ext, created.Format(time.RFC3339Nano), updated.Format(time.RFC3339Nano), payload); err != nil {
				return err
			}
			attrs := runtimeAttributeValues(item)
			for key, value := range attrs {
				encoded, err := json.Marshal(value)
				if err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_attributes(runtime_id,attribute_key,attribute_value,updated_at) VALUES(?,?,?,?) ON CONFLICT(runtime_id,attribute_key) DO UPDATE SET attribute_value=excluded.attribute_value,updated_at=excluded.updated_at`, item.ID, key, encoded, updated.Format(time.RFC3339Nano)); err != nil {
					return err
				}
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM runtime_settings`); err != nil {
			return err
		}
		keys := make([]string, 0, len(state.Settings))
		for k := range state.Settings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		updated := time.Now().UTC().Format(time.RFC3339Nano)
		for _, k := range keys {
			payload, err := json.Marshal(state.Settings[k])
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_settings(setting_key,payload,updated_at) VALUES(?,?,?)`, k, payload, updated); err != nil {
				return err
			}
		}
		return nil
	})
}

// runtimeAttributeValues 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeAttributeValues(item runtimeRecord) map[string]any {
	return map[string]any{"mode": item.Mode, "workDir": item.WorkDir, "resource": item.Resource, "source": item.Source, "downloadUrl": item.DownloadURL, "packageModified": item.PackageModified, "image": item.Image, "dockerCompose": item.DockerCompose, "params": item.Params, "env": item.Env, "appDetailID": item.AppDetailID, "appID": item.AppID, "port": item.Port, "container": item.Container, "exposedPorts": item.ExposedPorts, "environments": item.Environments, "volumes": item.Volumes, "extraHosts": item.ExtraHosts, "taskID": item.TaskID, "taskStatus": item.TaskStatus, "message": item.Message, "error": item.Error, "path": item.InstallPath, "composePath": item.ComposePath}
}

// migrateLegacyRuntimeState 执行运行时相关处理并返回可观测错误。
func migrateLegacyRuntimeState(ctx context.Context, db *sql.DB, dataDir string) error {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "./data"
	}
	path, err := filepath.Abs(filepath.Join(dataDir, "runtime.json"))
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxLegacyRuntimeBytes {
		return errors.New("旧 runtime.json 不是普通文件或超过 16 MiB 限制")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var legacy runtimeState
	if err := json.Unmarshal(payload, &legacy); err != nil {
		return fmt.Errorf("解析旧 runtime.json 失败: %w", err)
	}
	if legacy.Settings == nil {
		legacy.Settings = map[string]any{}
	}
	digestBytes := sha256.Sum256(payload)
	digest := hex.EncodeToString(digestBytes[:])
	var imported int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM legacy_imports WHERE source_type='json' AND source_path=? AND source_sha256=? AND domain='runtime' AND status='imported'`, path, digest).Scan(&imported); err != nil {
		return err
	}
	if imported == 0 {
		var records, settings int
		if err := db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM runtime_records),(SELECT COUNT(*) FROM runtime_settings)`).Scan(&records, &settings); err != nil {
			return err
		}
		if records == 0 && settings == 0 {
			if err := (runtimeRepository{db: db}).save(ctx, legacy); err != nil {
				return err
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		report, _ := json.Marshal(map[string]any{"source": path, "runtimes": len(legacy.Runtimes), "settings": len(legacy.Settings)})
		if _, err := db.ExecContext(ctx, `INSERT INTO legacy_imports(source_type,source_path,source_sha256,domain,status,imported_count,report_json,error,started_at,finished_at) VALUES('json',?,?, 'runtime','imported',?,?, '',?,?) ON CONFLICT(source_type,source_path,source_sha256,domain) DO UPDATE SET status='imported',imported_count=excluded.imported_count,report_json=excluded.report_json,error='',finished_at=excluded.finished_at`, path, digest, len(legacy.Runtimes)+len(legacy.Settings), string(report), now, now); err != nil {
			return err
		}
	}
	targetDir := filepath.Join(filepath.Dir(path), "backups", "legacy-runtime")
	if err := os.MkdirAll(targetDir, 0o750); err != nil {
		return err
	}
	target := filepath.Join(targetDir, filepath.Base(path))
	if _, err := os.Stat(target); err == nil {
		target = filepath.Join(targetDir, "runtime-"+time.Now().UTC().Format("20060102T150405.000000000Z")+".json")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(path, target); err != nil {
		return fmt.Errorf("归档旧 runtime.json 失败: %w", err)
	}
	return nil
}
