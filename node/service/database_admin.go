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
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// DatabaseUser 保存数据库实例的登录用户元数据；密码只保存脱敏标记。
type DatabaseUser struct {
	ID          int64     `json:"id"`
	DatabaseID  int64     `json:"databaseId"`
	Database    string    `json:"database"`
	Type        string    `json:"type"`
	Username    string    `json:"username"`
	Host        string    `json:"host"`
	Description string    `json:"description,omitempty"`
	PasswordSet bool      `json:"passwordSet"`
	CreatedAt   time.Time `json:"createdAt"`
}

// DatabaseGrant 描述用户可访问的数据库和权限集合。
type DatabaseGrant struct {
	ID         int64    `json:"id"`
	Database   string   `json:"database"`
	Username   string   `json:"username"`
	Host       string   `json:"host"`
	Privileges []string `json:"privileges"`
}

// DatabaseVariable 保存经过校验的数据库运行参数。
type DatabaseVariable struct {
	Database  string    `json:"database"`
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// DatabaseAdminStore 是节点侧数据库管理元数据的 SQLite 仓库。
// legacyPath 仅用于启动时一次性导入旧版本文件，正常请求不再读写该文件。
type DatabaseAdminStore struct {
	legacyPath    string
	fallbackPath  string
	initMu        sync.Mutex
	initializedDB *sql.DB
	db            *sql.DB
}

var fallbackSQLiteMu sync.Mutex
var fallbackSQLiteDB = map[string]*sql.DB{}

// NewDatabaseAdminStore 创建数据库管理仓库；正式服务复用统一 SQLite 连接池。
func NewDatabaseAdminStore() *DatabaseAdminStore {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = "./data"
	}
	dir, _ = filepath.Abs(filepath.Clean(dir))
	return &DatabaseAdminStore{legacyPath: filepath.Join(dir, "database-admin.json"), fallbackPath: filepath.Join(dir, "workmesh.db")}
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// sqliteFallback 供未启动 HTTP 服务的离线工具使用，仍只写 SQLite。
func (s *DatabaseAdminStore) sqliteFallback() (*sql.DB, error) {
	return sharedFallbackSQLite(s.fallbackPath)
}

// sharedFallbackSQLite 复用同一路径的连接池，避免离线工具重复打开数据库文件。
func sharedFallbackSQLite(path string) (*sql.DB, error) {
	fallbackSQLiteMu.Lock()
	defer fallbackSQLiteMu.Unlock()
	if db := fallbackSQLiteDB[path]; db != nil {
		return db, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("打开数据库管理 SQLite 失败: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(1)
	fallbackSQLiteDB[path] = db
	return db, nil
}

func (s *DatabaseAdminStore) database(ctx context.Context) (*sql.DB, error) {
	db := SharedDatabase()
	if db == nil {
		db = s.db
	}
	if db == nil {
		var err error
		db, err = s.sqliteFallback()
		if err != nil {
			return nil, err
		}
	}
	if err := s.initializeDatabase(contextOrBackground(ctx), db); err != nil {
		return nil, err
	}
	return db, nil
}

func (s *DatabaseAdminStore) initializeDatabase(ctx context.Context, db *sql.DB) error {
	s.initMu.Lock()
	defer s.initMu.Unlock()
	if s.initializedDB == db {
		return nil
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS database_users (id INTEGER PRIMARY KEY AUTOINCREMENT, database_id INTEGER NOT NULL DEFAULT 0, database_name TEXT NOT NULL, type TEXT NOT NULL DEFAULT '', username TEXT NOT NULL, host TEXT NOT NULL DEFAULT '%', description TEXT NOT NULL DEFAULT '', password_set INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_database_users_identity ON database_users(database_name,username,host)`,
		`CREATE INDEX IF NOT EXISTS idx_database_users_database ON database_users(database_name,id)`,
		`CREATE TABLE IF NOT EXISTS database_grants (id INTEGER PRIMARY KEY AUTOINCREMENT, database_name TEXT NOT NULL, username TEXT NOT NULL, host TEXT NOT NULL DEFAULT '%', privileges BLOB NOT NULL DEFAULT '[]')`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_database_grants_identity ON database_grants(database_name,username,host)`,
		`CREATE INDEX IF NOT EXISTS idx_database_grants_database ON database_grants(database_name,id)`,
		`CREATE TABLE IF NOT EXISTS database_variables (database_name TEXT NOT NULL, name TEXT NOT NULL, value TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(database_name,name))`,
		`CREATE TABLE IF NOT EXISTS database_configs (database_name TEXT PRIMARY KEY, content BLOB NOT NULL, updated_at TEXT NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("初始化数据库管理表失败: %w", err)
		}
	}
	if err := s.importLegacy(ctx, db); err != nil {
		return err
	}
	s.initializedDB = db
	return nil
}

// importLegacy 将旧文件作为一次性输入导入 SQLite，绝不覆盖已有表数据。
func (s *DatabaseAdminStore) importLegacy(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_users`).Scan(&count); err != nil || count != 0 {
		return err
	}
	var payload struct {
		Users     map[int64]DatabaseUser
		Grants    map[int64]DatabaseGrant
		Variables map[string]DatabaseVariable
		Configs   map[string]string
	}
	raw, err := os.ReadFile(s.legacyPath)
	if err != nil || len(raw) == 0 || json.Unmarshal(raw, &payload) != nil {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, user := range payload.Users {
		created := user.CreatedAt.UTC().Format(time.RFC3339Nano)
		if user.CreatedAt.IsZero() {
			created = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO database_users(id,database_id,database_name,type,username,host,description,password_set,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, user.ID, user.DatabaseID, user.Database, user.Type, user.Username, defaultAdminHost(user.Host), user.Description, databaseBoolInt(user.PasswordSet), created); err != nil {
			return fmt.Errorf("导入数据库用户失败: %w", err)
		}
	}
	for _, grant := range payload.Grants {
		privileges, _ := json.Marshal(grant.Privileges)
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO database_grants(id,database_name,username,host,privileges) VALUES(?,?,?,?,?)`, grant.ID, grant.Database, grant.Username, defaultAdminHost(grant.Host), privileges); err != nil {
			return fmt.Errorf("导入数据库授权失败: %w", err)
		}
	}
	for _, variable := range payload.Variables {
		updated := variable.UpdatedAt.UTC().Format(time.RFC3339Nano)
		if variable.UpdatedAt.IsZero() {
			updated = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO database_variables(database_name,name,value,updated_at) VALUES(?,?,?,?)`, variable.Database, variable.Name, variable.Value, updated); err != nil {
			return fmt.Errorf("导入数据库变量失败: %w", err)
		}
	}
	for database, content := range payload.Configs {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO database_configs(database_name,content,updated_at) VALUES(?,?,?)`, database, content, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("导入数据库配置失败: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// 导入成功后归档旧文件，避免后续启动继续把 JSON 当作运行时仓储。
	archiveDir := filepath.Join(filepath.Dir(s.legacyPath), "backups")
	if err := os.MkdirAll(archiveDir, 0o750); err != nil {
		return fmt.Errorf("创建数据库管理旧文件归档目录失败: %w", err)
	}
	archivePath := filepath.Join(archiveDir, "legacy-database-admin-"+time.Now().UTC().Format("20060102T150405.000000000Z")+".json")
	if err := os.Rename(s.legacyPath, archivePath); err != nil {
		return fmt.Errorf("归档数据库管理旧文件失败: %w", err)
	}
	return nil
}

func defaultAdminHost(host string) string {
	if strings.TrimSpace(host) == "" {
		return "%"
	}
	return strings.TrimSpace(host)
}

func parseAdminTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

// ListUsers 按数据库和用户名筛选，最多返回 1000 条。
func (s *DatabaseAdminStore) ListUsers(ctx context.Context, database, username string) []DatabaseUser {
	db, err := s.database(ctx)
	if err != nil {
		return []DatabaseUser{}
	}
	rows, err := db.QueryContext(contextOrBackground(ctx), `SELECT id,database_id,database_name,type,username,host,description,password_set,created_at FROM database_users WHERE (?='' OR lower(database_name)=lower(?)) AND (?='' OR lower(username) LIKE '%'||lower(?)||'%') ORDER BY id LIMIT 1000`, database, database, username, username)
	if err != nil {
		return []DatabaseUser{}
	}
	defer rows.Close()
	out := make([]DatabaseUser, 0)
	for rows.Next() {
		var item DatabaseUser
		var passwordSet int
		var created string
		if rows.Scan(&item.ID, &item.DatabaseID, &item.Database, &item.Type, &item.Username, &item.Host, &item.Description, &passwordSet, &created) == nil {
			item.PasswordSet, item.CreatedAt = passwordSet != 0, parseAdminTime(created)
			out = append(out, item)
		}
	}
	return out
}

// CreateUser 新增数据库用户，唯一索引保证身份不重复。
func (s *DatabaseAdminStore) CreateUser(ctx context.Context, user DatabaseUser) (DatabaseUser, error) {
	if strings.TrimSpace(user.Database) == "" || strings.TrimSpace(user.Username) == "" {
		return DatabaseUser{}, errors.New("数据库和用户名不能为空")
	}
	user.Host, user.CreatedAt = defaultAdminHost(user.Host), time.Now().UTC()
	db, err := s.database(ctx)
	if err != nil {
		return DatabaseUser{}, err
	}
	result, err := db.ExecContext(contextOrBackground(ctx), `INSERT INTO database_users(database_id,database_name,type,username,host,description,password_set,created_at) VALUES(?,?,?,?,?,?,?,?)`, user.DatabaseID, user.Database, user.Type, user.Username, user.Host, user.Description, databaseBoolInt(user.PasswordSet), user.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return DatabaseUser{}, fmt.Errorf("创建数据库用户失败: %w", err)
	}
	user.ID, _ = result.LastInsertId()
	return user, nil
}

// UpdateUser 更新用户元数据，不允许调用方清除原有密码状态。
func (s *DatabaseAdminStore) UpdateUser(ctx context.Context, user DatabaseUser) (DatabaseUser, error) {
	if strings.TrimSpace(user.Username) == "" || strings.TrimSpace(user.Database) == "" {
		return DatabaseUser{}, errors.New("数据库和用户名不能为空")
	}
	db, err := s.database(ctx)
	if err != nil {
		return DatabaseUser{}, err
	}
	var created string
	var passwordSet int
	if err = db.QueryRowContext(contextOrBackground(ctx), `SELECT created_at,password_set FROM database_users WHERE id=?`, user.ID).Scan(&created, &passwordSet); err != nil {
		return DatabaseUser{}, errors.New("数据库用户不存在")
	}
	user.Host = defaultAdminHost(user.Host)
	if _, err = db.ExecContext(contextOrBackground(ctx), `UPDATE database_users SET database_id=?,database_name=?,type=?,username=?,host=?,description=? WHERE id=?`, user.DatabaseID, user.Database, user.Type, user.Username, user.Host, user.Description, user.ID); err != nil {
		return DatabaseUser{}, fmt.Errorf("更新数据库用户失败: %w", err)
	}
	user.CreatedAt, user.PasswordSet = parseAdminTime(created), passwordSet != 0
	return user, nil
}

// DeleteUser 删除数据库用户。
func (s *DatabaseAdminStore) DeleteUser(ctx context.Context, id int64) error {
	db, err := s.database(ctx)
	if err != nil {
		return err
	}
	result, err := db.ExecContext(contextOrBackground(ctx), `DELETE FROM database_users WHERE id=?`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return errors.New("数据库用户不存在")
	}
	return nil
}

// SetPassword 标记用户已经设置密码；实际密码由目标数据库负责保存。
func (s *DatabaseAdminStore) SetPassword(ctx context.Context, id int64) error {
	db, err := s.database(ctx)
	if err != nil {
		return err
	}
	result, err := db.ExecContext(contextOrBackground(ctx), `UPDATE database_users SET password_set=1 WHERE id=?`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return errors.New("数据库用户不存在")
	}
	return nil
}

// ListGrants 查询授权记录，结果固定排序且最多 1000 条。
func (s *DatabaseAdminStore) ListGrants(ctx context.Context, database, username string) []DatabaseGrant {
	db, err := s.database(ctx)
	if err != nil {
		return []DatabaseGrant{}
	}
	rows, err := db.QueryContext(contextOrBackground(ctx), `SELECT id,database_name,username,host,privileges FROM database_grants WHERE (?='' OR lower(database_name)=lower(?)) AND (?='' OR lower(username)=lower(?)) ORDER BY id LIMIT 1000`, database, database, username, username)
	if err != nil {
		return []DatabaseGrant{}
	}
	defer rows.Close()
	out := make([]DatabaseGrant, 0)
	for rows.Next() {
		var item DatabaseGrant
		var raw []byte
		if rows.Scan(&item.ID, &item.Database, &item.Username, &item.Host, &raw) == nil {
			_ = json.Unmarshal(raw, &item.Privileges)
			if item.Privileges == nil {
				item.Privileges = []string{}
			}
			out = append(out, item)
		}
	}
	return out
}

// UpsertGrant 新增或更新数据库授权。
func (s *DatabaseAdminStore) UpsertGrant(ctx context.Context, grant DatabaseGrant) (DatabaseGrant, error) {
	if strings.TrimSpace(grant.Database) == "" || strings.TrimSpace(grant.Username) == "" {
		return DatabaseGrant{}, errors.New("数据库和用户名不能为空")
	}
	grant.Host = defaultAdminHost(grant.Host)
	if grant.Privileges == nil {
		grant.Privileges = []string{}
	}
	privileges, _ := json.Marshal(grant.Privileges)
	db, err := s.database(ctx)
	if err != nil {
		return DatabaseGrant{}, err
	}
	_ = db.QueryRowContext(contextOrBackground(ctx), `SELECT id FROM database_grants WHERE database_name=? AND username=? AND host=?`, grant.Database, grant.Username, grant.Host).Scan(&grant.ID)
	if grant.ID > 0 {
		_, err = db.ExecContext(contextOrBackground(ctx), `UPDATE database_grants SET privileges=? WHERE id=?`, privileges, grant.ID)
	} else {
		var result sql.Result
		result, err = db.ExecContext(contextOrBackground(ctx), `INSERT INTO database_grants(database_name,username,host,privileges) VALUES(?,?,?,?)`, grant.Database, grant.Username, grant.Host, privileges)
		if err == nil {
			grant.ID, _ = result.LastInsertId()
		}
	}
	if err != nil {
		return DatabaseGrant{}, fmt.Errorf("保存数据库授权失败: %w", err)
	}
	return grant, nil
}

// DeleteGrant 删除授权记录。
func (s *DatabaseAdminStore) DeleteGrant(ctx context.Context, id int64) error {
	db, err := s.database(ctx)
	if err != nil {
		return err
	}
	result, err := db.ExecContext(contextOrBackground(ctx), `DELETE FROM database_grants WHERE id=?`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return errors.New("授权记录不存在")
	}
	return nil
}

// Variables 返回数据库变量列表。
func (s *DatabaseAdminStore) Variables(ctx context.Context, database string) []DatabaseVariable {
	db, err := s.database(ctx)
	if err != nil {
		return []DatabaseVariable{}
	}
	rows, err := db.QueryContext(contextOrBackground(ctx), `SELECT database_name,name,value,updated_at FROM database_variables WHERE (?='' OR lower(database_name)=lower(?)) ORDER BY name LIMIT 1000`, database, database)
	if err != nil {
		return []DatabaseVariable{}
	}
	defer rows.Close()
	out := make([]DatabaseVariable, 0)
	for rows.Next() {
		var item DatabaseVariable
		var updated string
		if rows.Scan(&item.Database, &item.Name, &item.Value, &updated) == nil {
			item.UpdatedAt = parseAdminTime(updated)
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// SetVariable 保存数据库变量，并限制单项大小避免异常请求耗尽内存。
func (s *DatabaseAdminStore) SetVariable(ctx context.Context, variable DatabaseVariable) (DatabaseVariable, error) {
	if strings.TrimSpace(variable.Name) == "" || len(variable.Name) > 128 {
		return DatabaseVariable{}, errors.New("变量名称无效")
	}
	if len(variable.Value) > 4096 {
		return DatabaseVariable{}, errors.New("变量值过长")
	}
	variable.Database, variable.Name = strings.TrimSpace(variable.Database), strings.TrimSpace(variable.Name)
	variable.UpdatedAt = time.Now().UTC()
	db, err := s.database(ctx)
	if err != nil {
		return DatabaseVariable{}, err
	}
	_, err = db.ExecContext(contextOrBackground(ctx), `INSERT INTO database_variables(database_name,name,value,updated_at) VALUES(?,?,?,?) ON CONFLICT(database_name,name) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, variable.Database, variable.Name, variable.Value, variable.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return DatabaseVariable{}, fmt.Errorf("保存数据库变量失败: %w", err)
	}
	return variable, nil
}

// SetConfig 保存数据库配置内容，限制为 1 MiB。
func (s *DatabaseAdminStore) SetConfig(ctx context.Context, database, content string) error {
	if len(content) > 1<<20 {
		return errors.New("配置文件超过 1 MiB")
	}
	db, err := s.database(ctx)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(contextOrBackground(ctx), `INSERT INTO database_configs(database_name,content,updated_at) VALUES(?,?,?) ON CONFLICT(database_name) DO UPDATE SET content=excluded.content,updated_at=excluded.updated_at`, database, content, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// Config 读取数据库配置内容。
func (s *DatabaseAdminStore) Config(ctx context.Context, database string) string {
	db, err := s.database(ctx)
	if err != nil {
		return ""
	}
	var content string
	_ = db.QueryRowContext(contextOrBackground(ctx), `SELECT content FROM database_configs WHERE database_name=?`, database).Scan(&content)
	return content
}
