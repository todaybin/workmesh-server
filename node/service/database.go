// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

type Database struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	Version       string    `json:"version,omitempty"`
	From          string    `json:"from,omitempty"`
	AppInstallID  int64     `json:"appInstallID,omitempty"`
	ContainerName string    `json:"containerName,omitempty"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	InitialDB     string    `json:"initialDB,omitempty"`
	Username      string    `json:"username"`
	Password      string    `json:"-"`
	SSL           bool      `json:"ssl,omitempty"`
	Description   string    `json:"description,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}
type DatabaseRepository struct {
	db         *sql.DB
	repository storage.Transactional
}

// DatabaseSchemaStore is the small SQL surface shared by SQLite migrations and
// control-store bootstrap checks.
type DatabaseSchemaStore interface {
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

var sharedDatabaseMu sync.RWMutex
var sharedDatabase *sql.DB

// EnsureDatabaseContainerNameColumn upgrades an existing databases table
// without changing data or failing when the column is already present.
func EnsureDatabaseContainerNameColumn(db DatabaseSchemaStore) error {
	if db == nil {
		return errors.New("数据库资源表连接不能为空")
	}
	rows, err := db.Query(`PRAGMA table_info(databases)`)
	if err != nil {
		return fmt.Errorf("检查数据库资源表 container_name 字段失败: %w", err)
	}
	hasColumn := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return fmt.Errorf("读取数据库资源表结构失败: %w", err)
		}
		if name == "container_name" {
			hasColumn = true
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("检查数据库资源表 container_name 字段失败: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("关闭数据库资源表结构游标失败: %w", err)
	}
	if hasColumn {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE databases ADD COLUMN container_name TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("升级数据库资源表失败: 增加 container_name 字段: %w", err)
	}
	return nil
}

// DatabaseContainerNameMigration is the versioned upgrade for legacy
// databases tables. The marker is intentionally stable because applied
// migration checksums are part of the SQLite compatibility contract.
func DatabaseContainerNameMigration() storage.Migration {
	sum := sha256.Sum256([]byte("database-container-name-v1"))
	return storage.Migration{
		ID:       "0007-database-container-name",
		Checksum: hex.EncodeToString(sum[:]),
		Up: func(_ context.Context, tx *sql.Tx) error {
			return EnsureDatabaseContainerNameColumn(tx)
		},
	}
}

// SetSharedDatabase 注入统一 SQLite 连接池；正式服务启动后数据库仓库不再写文件。
func SetSharedDatabase(db *sql.DB) {
	sharedDatabaseMu.Lock()
	sharedDatabase = db
	sharedDatabaseMu.Unlock()
}

func currentSharedDatabase() *sql.DB {
	sharedDatabaseMu.RLock()
	defer sharedDatabaseMu.RUnlock()
	return sharedDatabase
}

// SharedDatabase 返回进程统一 SQLite 连接；领域服务使用该连接避免重复打开数据库。
func SharedDatabase() *sql.DB { return currentSharedDatabase() }

func NewDatabaseRepository() *DatabaseRepository {
	db := currentSharedDatabase()
	repository, _ := storage.NewSQLiteRepository(db)
	return &DatabaseRepository{db: db, repository: repository}
}

func (r *DatabaseRepository) sqlDB() *sql.DB {
	if db := currentSharedDatabase(); db != nil {
		return db
	}
	return r.db
}

func (r *DatabaseRepository) executor() (storage.Transactional, error) {
	if current := currentSharedDatabase(); current != nil {
		if r.repository == nil || r.db != current {
			repository, err := storage.NewSQLiteRepository(current)
			if err != nil {
				return nil, err
			}
			r.repository, r.db = repository, current
		}
		return r.repository, nil
	}
	if r.repository != nil {
		return r.repository, nil
	}
	if r.db == nil {
		return nil, errors.New("公共数据库未初始化")
	}
	repository, err := storage.NewSQLiteRepository(r.db)
	if err != nil {
		return nil, err
	}
	r.repository = repository
	return repository, nil
}

func (r *DatabaseRepository) List(ctx context.Context, typ, name string) []Database {
	if repository, repositoryErr := r.executor(); repositoryErr == nil {
		query := `SELECT id,name,type,version,source,app_install_id,container_name,address,port,initial_db,username,description,created_at,updated_at FROM databases WHERE (?='' OR lower(type)=lower(?)) AND (?='' OR lower(name) LIKE '%'||lower(?)||'%') ORDER BY id`
		rows, err := repository.QueryContext(ctx, query, typ, typ, name, name)
		legacy := false
		if err != nil {
			if !isMissingDatabaseContainerName(err) {
				return []Database{}
			}
			legacy = true
			rows, err = repository.QueryContext(ctx, `SELECT id,name,type,version,source,app_install_id,address,port,initial_db,username,description,created_at,updated_at FROM databases WHERE (?='' OR lower(type)=lower(?)) AND (?='' OR lower(name) LIKE '%'||lower(?)||'%') ORDER BY id`, typ, typ, name, name)
			if err != nil {
				return []Database{}
			}
		}
		defer rows.Close()
		out := make([]Database, 0)
		for rows.Next() {
			var item Database
			var created, updated string
			var err error
			if legacy {
				err = rows.Scan(&item.ID, &item.Name, &item.Type, &item.Version, &item.From, &item.AppInstallID, &item.Host, &item.Port, &item.InitialDB, &item.Username, &item.Description, &created, &updated)
			} else {
				err = rows.Scan(&item.ID, &item.Name, &item.Type, &item.Version, &item.From, &item.AppInstallID, &item.ContainerName, &item.Host, &item.Port, &item.InitialDB, &item.Username, &item.Description, &created, &updated)
			}
			if err != nil {
				continue
			}
			item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
			item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
			out = append(out, item)
		}
		return out
	}
	return []Database{}
}
func (r *DatabaseRepository) Create(ctx context.Context, item Database) (Database, error) {
	if repository, repositoryErr := r.executor(); repositoryErr == nil {
		if item.Name == "" || item.Type == "" {
			return Database{}, errors.New("数据库名称和类型不能为空")
		}
		now := time.Now().UTC()
		item.CreatedAt, item.UpdatedAt = now, now
		res, err := repository.ExecContext(ctx, `INSERT INTO databases(name,type,version,source,app_install_id,container_name,address,port,initial_db,username,password,ssl,description,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, item.Name, item.Type, item.Version, item.From, item.AppInstallID, item.ContainerName, item.Host, item.Port, item.InitialDB, item.Username, item.Password, databaseBoolInt(item.SSL), item.Description, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
		if err != nil && isMissingDatabaseContainerName(err) {
			res, err = repository.ExecContext(ctx, `INSERT INTO databases(name,type,version,source,app_install_id,address,port,initial_db,username,password,ssl,description,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, item.Name, item.Type, item.Version, item.From, item.AppInstallID, item.Host, item.Port, item.InitialDB, item.Username, item.Password, databaseBoolInt(item.SSL), item.Description, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
		}
		if err != nil {
			return Database{}, err
		}
		item.ID, _ = res.LastInsertId()
		return item, nil
	}
	return Database{}, errors.New("公共数据库未初始化")
}
func (r *DatabaseRepository) Delete(ctx context.Context, id int64) error {
	if repository, repositoryErr := r.executor(); repositoryErr == nil {
		res, err := repository.ExecContext(ctx, `DELETE FROM databases WHERE id=?`, id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return errors.New("数据库不存在")
		}
		return nil
	}
	return errors.New("公共数据库未初始化")
}

// Update 修改已登记的数据库连接信息，不会覆盖创建时间。
func (r *DatabaseRepository) Update(ctx context.Context, item Database) (Database, error) {
	if repository, repositoryErr := r.executor(); repositoryErr == nil {
		if item.Name == "" || item.Type == "" {
			return Database{}, errors.New("数据库名称和类型不能为空")
		}
		current := r.findSQL(ctx, item.ID)
		if current.ID == 0 {
			return Database{}, errors.New("数据库不存在")
		}
		item.CreatedAt, item.UpdatedAt = current.CreatedAt, time.Now().UTC()
		_, err := repository.ExecContext(ctx, `UPDATE databases SET name=?,type=?,version=?,source=?,app_install_id=?,container_name=?,address=?,port=?,initial_db=?,username=?,password=CASE WHEN ?='' THEN password ELSE ? END,ssl=?,description=?,updated_at=? WHERE id=?`, item.Name, item.Type, item.Version, item.From, item.AppInstallID, item.ContainerName, item.Host, item.Port, item.InitialDB, item.Username, item.Password, item.Password, databaseBoolInt(item.SSL), item.Description, item.UpdatedAt.Format(time.RFC3339Nano), item.ID)
		if err != nil && isMissingDatabaseContainerName(err) {
			_, err = repository.ExecContext(ctx, `UPDATE databases SET name=?,type=?,version=?,source=?,app_install_id=?,address=?,port=?,initial_db=?,username=?,password=CASE WHEN ?='' THEN password ELSE ? END,ssl=?,description=?,updated_at=? WHERE id=?`, item.Name, item.Type, item.Version, item.From, item.AppInstallID, item.Host, item.Port, item.InitialDB, item.Username, item.Password, item.Password, databaseBoolInt(item.SSL), item.Description, item.UpdatedAt.Format(time.RFC3339Nano), item.ID)
		}
		if err != nil {
			return Database{}, err
		}
		if item.Password == "" {
			item.Password = current.Password
		}
		return item, nil
	}
	return Database{}, errors.New("公共数据库未初始化")
}

func (r *DatabaseRepository) findSQL(ctx context.Context, id int64) Database {
	repository, repositoryErr := r.executor()
	if repositoryErr != nil {
		return Database{}
	}
	var item Database
	var created, updated string
	var ssl int
	err := repository.QueryRowContext(ctx, `SELECT id,name,type,version,source,app_install_id,container_name,address,port,initial_db,username,password,ssl,description,created_at,updated_at FROM databases WHERE id=?`, id).Scan(&item.ID, &item.Name, &item.Type, &item.Version, &item.From, &item.AppInstallID, &item.ContainerName, &item.Host, &item.Port, &item.InitialDB, &item.Username, &item.Password, &ssl, &item.Description, &created, &updated)
	if err != nil && isMissingDatabaseContainerName(err) {
		err = repository.QueryRowContext(ctx, `SELECT id,name,type,version,source,app_install_id,address,port,initial_db,username,password,ssl,description,created_at,updated_at FROM databases WHERE id=?`, id).Scan(&item.ID, &item.Name, &item.Type, &item.Version, &item.From, &item.AppInstallID, &item.Host, &item.Port, &item.InitialDB, &item.Username, &item.Password, &ssl, &item.Description, &created, &updated)
	}
	if err != nil {
		return Database{}
	}
	item.SSL = ssl != 0
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return item
}

func isMissingDatabaseContainerName(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such column: container_name") ||
		strings.Contains(message, "has no column named container_name")
}

type DatabaseService struct{ repo *DatabaseRepository }

func NewDatabaseService(repo *DatabaseRepository) *DatabaseService {
	if repo == nil {
		repo = NewDatabaseRepository()
	}
	return &DatabaseService{repo: repo}
}
func (s *DatabaseService) Search(ctx context.Context, typ, name string) []Database {
	return s.repo.List(ctx, typ, name)
}

// FindConnection 返回应用安装参数所需的完整连接信息，调用方必须自行脱敏输出。
func (s *DatabaseService) FindConnection(ctx context.Context, typ, name string) (Database, bool) {
	typ = strings.ToLower(strings.TrimSpace(typ))
	name = strings.TrimSpace(name)
	for _, item := range s.repo.List(ctx, typ, name) {
		if strings.EqualFold(item.Name, name) {
			item = s.repo.findSQL(ctx, item.ID)
			return item, true
		}
	}
	return Database{}, false
}

// Find 按资源 ID 返回完整数据库登记信息，供管理路由复用已持久化的连接参数。
func (s *DatabaseService) Find(ctx context.Context, id int64) (Database, bool) {
	if id <= 0 {
		return Database{}, false
	}
	item := s.repo.findSQL(ctx, id)
	return item, item.ID != 0
}

// FindByName 按类型和名称返回数据库登记信息，避免管理接口回退到未经确认的本机地址。
func (s *DatabaseService) FindByName(ctx context.Context, typ, name string) (Database, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Database{}, false
	}
	for _, item := range s.repo.List(ctx, typ, name) {
		if strings.EqualFold(item.Name, name) {
			return s.repo.findSQL(ctx, item.ID), true
		}
	}
	return Database{}, false
}
func (s *DatabaseService) Create(ctx context.Context, item Database) (Database, error) {
	if item.Port < 0 || item.Port > 65535 {
		return Database{}, errors.New("数据库端口无效")
	}
	return s.repo.Create(ctx, item)
}
func (s *DatabaseService) Delete(ctx context.Context, id int64) error { return s.repo.Delete(ctx, id) }

// Update 修改数据库资源登记并保留原始创建时间。
func (s *DatabaseService) Update(ctx context.Context, item Database) (Database, error) {
	if item.Port < 0 || item.Port > 65535 {
		return Database{}, errors.New("数据库端口无效")
	}
	if strings.TrimSpace(item.Host) == "" {
		return Database{}, errors.New("数据库主机不能为空")
	}
	return s.repo.Update(ctx, item)
}
func (s *DatabaseService) Check(ctx context.Context, item Database) bool {
	if strings.TrimSpace(item.Host) == "" || item.Port <= 0 || item.Port > 65535 {
		return false
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(checkCtx, "tcp", net.JoinHostPort(item.Host, strconv.Itoa(item.Port)))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func databaseBoolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
