// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// SetDatabase 注入认证领域共用的 SQLite 数据库，并执行核心表初始化。
// core_users 为空时仅导入一次旧用户文件，后续读写完全使用 SQLite。
func (s *CoreService) SetDatabase(db *sql.DB) error {
	if db == nil {
		return errors.New("SQLite 数据库不能为空")
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS core_users (id TEXT PRIMARY KEY, payload BLOB NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS core_passkeys (id TEXT PRIMARY KEY, name TEXT NOT NULL, credential_id TEXT NOT NULL UNIQUE, created_at TEXT NOT NULL, last_used_at TEXT NOT NULL DEFAULT '')`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS core_groups (id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL); CREATE TABLE IF NOT EXISTS core_settings (setting_key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		return err
	}
	repository, err := storage.NewSQLiteRepository(db)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.db, s.repository = db, repository
	s.mu.Unlock()
	var payload []byte
	err = db.QueryRow(`SELECT payload FROM core_users ORDER BY id LIMIT 1`).Scan(&payload)
	if err == nil {
		var users map[string]persistedUser
		if json.Unmarshal(payload, &users) == nil && len(users) > 0 {
			s.mu.Lock()
			s.users = usersFromPersisted(users)
			s.mu.Unlock()
			if err := s.loadPasskeysFromDB(repository); err != nil {
				return err
			}
			s.loadGroupsSettingsFromDB(repository)
			return nil
		}
	}
	if err != sql.ErrNoRows && err != nil {
		return err
	}
	// 核心用户表为空时才读取旧文件作为一次性导入输入；已有 SQLite 数据绝不回读 JSON。
	s.loadUsers()
	s.loadPasskeys()
	if err := s.loadPasskeysFromDB(repository); err != nil {
		return err
	}
	s.loadGroupsSettingsFromDB(repository)
	return s.saveUsersToDB()
}

// loadPasskeysFromDB 从 SQLite 恢复 Passkey，并在关系表为空时导入兼容文件数据。
func (s *CoreService) loadPasskeysFromDB(repository storage.SQLExecutor) error {
	s.mu.Lock()
	legacy := s.passkeys
	s.passkeys = make(map[string]Passkey)
	s.mu.Unlock()
	rows, rowsErr := repository.Query(`SELECT id,name,credential_id,created_at,last_used_at FROM core_passkeys`)
	if rowsErr != nil {
		return rowsErr
	}
	defer rows.Close()
	s.mu.Lock()
	for rows.Next() {
		var item Passkey
		var created, last string
		if rows.Scan(&item.ID, &item.Name, &item.CredentialID, &created, &last) == nil {
			item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
			if last != "" {
				item.LastUsedAt, _ = time.Parse(time.RFC3339Nano, last)
			}
			s.passkeys[item.ID] = item
		}
	}
	s.mu.Unlock()
	if err := rows.Err(); err != nil {
		return err
	}
	var count int
	if err := repository.QueryRow(`SELECT COUNT(*) FROM core_passkeys`).Scan(&count); err == nil && count == 0 && len(legacy) > 0 {
		s.mu.Lock()
		s.passkeys = legacy
		err = s.savePasskeysLocked()
		s.mu.Unlock()
		return err
	}
	return nil
}

// shouldLoadLegacyFiles 判断当前数据目录是否仍处于 SQLite 初始化前的兼容阶段。
func shouldLoadLegacyFiles(dataDir string) bool {
	info, err := os.Stat(filepath.Join(dataDir, "workmesh.db"))
	return os.IsNotExist(err) || err != nil || info.Size() == 0
}

// loadUsers 读取本地用户哈希；文件损坏或不存在时保留首次启动管理员。
func (s *CoreService) loadUsers() {
	raw, err := os.ReadFile(s.usersPath)
	if err != nil {
		return
	}
	var users map[string]persistedUser
	if json.Unmarshal(raw, &users) != nil || len(users) == 0 {
		return
	}
	s.users = usersFromPersisted(users)
}

// persistedUser 是旧用户 JSON 和 SQLite payload 的内部表示，包含密码哈希但不直接对外返回。
type persistedUser struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	Password string    `json:"password"`
	Groups   []string  `json:"groups,omitempty"`
	MFA      bool      `json:"mfa"`
	API      APIConfig `json:"api,omitempty"`
}

// usersFromPersisted 将持久化用户转换为运行时模型，集中保证字段映射一致。
func usersFromPersisted(items map[string]persistedUser) map[string]User {
	users := make(map[string]User, len(items))
	for key, item := range items {
		users[key] = User{ID: item.ID, Name: item.Name, Role: item.Role, Password: item.Password, Groups: item.Groups, MFA: item.MFA, API: item.API}
	}
	return users
}

// saveUsersLocked 在持有用户锁时优先写 SQLite，无数据库时原子写入兼容文件。
func (s *CoreService) saveUsersLocked() error {
	if s.repository != nil {
		return s.saveUsersToDBLocked()
	}
	if err := os.MkdirAll(filepath.Dir(s.usersPath), 0o700); err != nil {
		return err
	}
	items := make(map[string]persistedUser, len(s.users))
	for key, user := range s.users {
		items[key] = persistedUser{ID: user.ID, Name: user.Name, Role: user.Role, Password: user.Password, Groups: user.Groups, MFA: user.MFA, API: user.API}
	}
	raw, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.usersPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.usersPath)
}

// saveUsersToDB 获取读锁后将当前用户快照写入 SQLite。
func (s *CoreService) saveUsersToDB() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.saveUsersToDBLocked()
}

// saveUsersToDBLocked 将包含凭据哈希的用户快照保存到核心用户记录。
func (s *CoreService) saveUsersToDBLocked() error {
	if s.repository == nil {
		return errors.New("SQLite 数据库未初始化")
	}
	items := make(map[string]persistedUser, len(s.users))
	for key, user := range s.users {
		items[key] = persistedUser{ID: user.ID, Name: user.Name, Role: user.Role, Password: user.Password, Groups: user.Groups, MFA: user.MFA, API: user.API}
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return err
	}
	_, err = s.repository.Exec(`INSERT INTO core_users(id,payload,updated_at) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`, "local", raw, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// loadPasskeys 从兼容文件读取 Passkey 元数据，非法或不完整记录会被忽略。
func (s *CoreService) loadPasskeys() {
	raw, err := os.ReadFile(s.passkeyPath)
	if err != nil {
		return
	}
	var items []Passkey
	if json.Unmarshal(raw, &items) != nil {
		return
	}
	for _, item := range items {
		if item.ID != "" && item.CredentialID != "" {
			s.passkeys[item.ID] = item
		}
	}
}

// savePasskeysLocked 在持有锁时持久化 Passkey；SQLite 可用时不写兼容文件。
func (s *CoreService) savePasskeysLocked() error {
	items := make([]Passkey, 0, len(s.passkeys))
	for _, item := range s.passkeys {
		items = append(items, item)
	}
	if s.repository != nil {
		return s.repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
			if _, err := tx.Exec(`DELETE FROM core_passkeys`); err != nil {
				return err
			}
			for _, item := range items {
				lastUsed := ""
				if !item.LastUsedAt.IsZero() {
					lastUsed = item.LastUsedAt.UTC().Format(time.RFC3339Nano)
				}
				if _, err := tx.Exec(`INSERT INTO core_passkeys(id,name,credential_id,created_at,last_used_at) VALUES(?,?,?,?,?)`, item.ID, item.Name, item.CredentialID, item.CreatedAt.UTC().Format(time.RFC3339Nano), lastUsed); err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err := os.MkdirAll(filepath.Dir(s.passkeyPath), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.passkeyPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.passkeyPath)
}
