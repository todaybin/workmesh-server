// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Database struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	Version      string    `json:"version,omitempty"`
	From         string    `json:"from,omitempty"`
	AppInstallID int64     `json:"appInstallID,omitempty"`
	Host         string    `json:"host"`
	Port         int       `json:"port"`
	InitialDB    string    `json:"initialDB,omitempty"`
	Username     string    `json:"username"`
	Password     string    `json:"-"`
	SSL          bool      `json:"ssl,omitempty"`
	Description  string    `json:"description,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
type DatabaseRepository struct {
	db *sql.DB
}

var sharedDatabaseMu sync.RWMutex
var sharedDatabase *sql.DB

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
	return &DatabaseRepository{db: currentSharedDatabase()}
}

func (r *DatabaseRepository) sqlDB() *sql.DB {
	if db := currentSharedDatabase(); db != nil {
		return db
	}
	return r.db
}

func (r *DatabaseRepository) List(ctx context.Context, typ, name string) []Database {
	if db := r.sqlDB(); db != nil {
		query := `SELECT id,name,type,version,source,app_install_id,address,port,initial_db,username,description,created_at,updated_at FROM databases WHERE (?='' OR lower(type)=lower(?)) AND (?='' OR lower(name) LIKE '%'||lower(?)||'%') ORDER BY id`
		rows, err := db.QueryContext(ctx, query, typ, typ, name, name)
		if err != nil {
			return []Database{}
		}
		defer rows.Close()
		out := make([]Database, 0)
		for rows.Next() {
			var item Database
			var created, updated string
			if rows.Scan(&item.ID, &item.Name, &item.Type, &item.Version, &item.From, &item.AppInstallID, &item.Host, &item.Port, &item.InitialDB, &item.Username, &item.Description, &created, &updated) != nil {
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
	if db := r.sqlDB(); db != nil {
		if item.Name == "" || item.Type == "" {
			return Database{}, errors.New("数据库名称和类型不能为空")
		}
		now := time.Now().UTC()
		item.CreatedAt, item.UpdatedAt = now, now
		res, err := db.ExecContext(ctx, `INSERT INTO databases(name,type,version,source,app_install_id,address,port,initial_db,username,password,ssl,description,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, item.Name, item.Type, item.Version, item.From, item.AppInstallID, item.Host, item.Port, item.InitialDB, item.Username, item.Password, databaseBoolInt(item.SSL), item.Description, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
		if err != nil {
			return Database{}, err
		}
		item.ID, _ = res.LastInsertId()
		return item, nil
	}
	return Database{}, errors.New("公共数据库未初始化")
}
func (r *DatabaseRepository) Delete(ctx context.Context, id int64) error {
	if db := r.sqlDB(); db != nil {
		res, err := db.ExecContext(ctx, `DELETE FROM databases WHERE id=?`, id)
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
	if db := r.sqlDB(); db != nil {
		if item.Name == "" || item.Type == "" {
			return Database{}, errors.New("数据库名称和类型不能为空")
		}
		current := r.findSQL(ctx, item.ID)
		if current.ID == 0 {
			return Database{}, errors.New("数据库不存在")
		}
		item.CreatedAt, item.UpdatedAt = current.CreatedAt, time.Now().UTC()
		_, err := db.ExecContext(ctx, `UPDATE databases SET name=?,type=?,version=?,source=?,app_install_id=?,address=?,port=?,initial_db=?,username=?,password=CASE WHEN ?='' THEN password ELSE ? END,ssl=?,description=?,updated_at=? WHERE id=?`, item.Name, item.Type, item.Version, item.From, item.AppInstallID, item.Host, item.Port, item.InitialDB, item.Username, item.Password, item.Password, databaseBoolInt(item.SSL), item.Description, item.UpdatedAt.Format(time.RFC3339Nano), item.ID)
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
	db := r.sqlDB()
	if db == nil {
		return Database{}
	}
	var item Database
	var created, updated string
	var ssl int
	err := db.QueryRowContext(ctx, `SELECT id,name,type,version,source,app_install_id,address,port,initial_db,username,password,ssl,description,created_at,updated_at FROM databases WHERE id=?`, id).Scan(&item.ID, &item.Name, &item.Type, &item.Version, &item.From, &item.AppInstallID, &item.Host, &item.Port, &item.InitialDB, &item.Username, &item.Password, &ssl, &item.Description, &created, &updated)
	if err != nil {
		return Database{}
	}
	item.SSL = ssl != 0
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return item
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
			if db := s.repo.sqlDB(); db != nil {
				item = s.repo.findSQL(ctx, item.ID)
			}
			return item, true
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
