package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

var (
	controlStoreMu sync.RWMutex
	controlStoreDB *sql.DB
)

// SetSharedStore 注入服务进程唯一的公共数据库连接。
func SetSharedStore(store *storage.Store) error {
	if store == nil || store.DB() == nil {
		return errors.New("公共数据库连接不能为空")
	}
	db := store.DB()
	if err := ensureControlTables(db); err != nil {
		return err
	}
	controlStoreMu.Lock()
	controlStoreDB = db
	controlStoreMu.Unlock()
	return importLegacyControlState(db)
}

func sharedDB() *sql.DB {
	controlStoreMu.RLock()
	defer controlStoreMu.RUnlock()
	return controlStoreDB
}

func ensureControlTables(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS app_store_state (id INTEGER PRIMARY KEY CHECK(id=1), payload BLOB NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS container_store_state (id INTEGER PRIMARY KEY CHECK(id=1), payload BLOB NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS app_installs (id TEXT PRIMARY KEY, app_key TEXT NOT NULL, name TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, install_path TEXT NOT NULL DEFAULT '', compose_path TEXT NOT NULL DEFAULT '', compose_project TEXT NOT NULL DEFAULT '', container_names TEXT NOT NULL DEFAULT '', config_json BLOB NOT NULL DEFAULT '{}', message TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_app_installs_status ON app_installs(status, updated_at)`,
		`CREATE TABLE IF NOT EXISTS app_install_tasks (id TEXT PRIMARY KEY, app_install_id TEXT NOT NULL, status TEXT NOT NULL, step TEXT NOT NULL DEFAULT '', progress INTEGER NOT NULL DEFAULT 0, message TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '', log_path TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_app_install_tasks_install ON app_install_tasks(app_install_id, updated_at)`,
		`CREATE TABLE IF NOT EXISTS container_compose_projects (id TEXT PRIMARY KEY, name TEXT NOT NULL, path TEXT NOT NULL, app_install_id TEXT NOT NULL DEFAULT '', project_name TEXT NOT NULL DEFAULT '', pinned INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, UNIQUE(path))`,
		`CREATE TABLE IF NOT EXISTS image_repositories (id TEXT PRIMARY KEY, name TEXT NOT NULL, download_url TEXT NOT NULL DEFAULT '', protocol TEXT NOT NULL DEFAULT '', username TEXT NOT NULL DEFAULT '', password TEXT NOT NULL DEFAULT '', auth INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS compose_templates (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', content BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS docker_settings (id INTEGER PRIMARY KEY CHECK(id=1), payload BLOB NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS node_hosts (id TEXT PRIMARY KEY, name TEXT NOT NULL, address TEXT NOT NULL, port INTEGER NOT NULL, user_name TEXT NOT NULL DEFAULT '', group_id INTEGER NOT NULL DEFAULT 0, payload BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_node_hosts_group_name ON node_hosts(group_id, name, id)`,
		`CREATE TABLE IF NOT EXISTS node_quick_commands (id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL DEFAULT 'command', command TEXT NOT NULL, group_id INTEGER NOT NULL DEFAULT 0, group_belong TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', payload BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_node_quick_commands_type_name ON node_quick_commands(type, name, id)`,
		`CREATE TABLE IF NOT EXISTS node_settings (setting_key TEXT PRIMARY KEY, payload BLOB NOT NULL, updated_at TEXT NOT NULL)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("初始化公共控制面表失败: %w", err)
		}
	}
	return nil
}

func importLegacyControlState(db *sql.DB) error {
	dataDir := os.Getenv("WORKMESH_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	for _, item := range []struct {
		name string
		path string
	}{
		{name: "apps", path: filepath.Join(dataDir, "apps.json")},
		{name: "containers", path: filepath.Join(dataDir, "containers.json")},
	} {
		payload, err := os.ReadFile(item.path)
		if err != nil || len(payload) == 0 {
			continue
		}
		table := "app_store_state"
		if item.name == "containers" {
			table = "container_store_state"
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			if _, err := db.Exec("INSERT INTO "+table+"(id,payload,updated_at) VALUES(1,?,?)", payload, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				return fmt.Errorf("导入 %s 失败: %w", item.path, err)
			}
		}
	}
	return nil
}

func loadJSONState(table string, target any) bool {
	db := sharedDB()
	if db == nil {
		return false
	}
	var payload []byte
	if err := db.QueryRowContext(context.Background(), "SELECT payload FROM "+table+" WHERE id=1").Scan(&payload); err != nil {
		return false
	}
	return json.Unmarshal(payload, target) == nil
}

func saveJSONState(table string, value any) error {
	db := sharedDB()
	if db == nil {
		return nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO "+table+"(id,payload,updated_at) VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at", payload, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func loadNodeSetting(key string, target any) bool {
	db := sharedDB()
	if db == nil {
		return false
	}
	var payload []byte
	if err := db.QueryRow("SELECT payload FROM node_settings WHERE setting_key = ?", key).Scan(&payload); err != nil {
		return false
	}
	return json.Unmarshal(payload, target) == nil
}

func saveNodeSetting(key string, value any) error {
	db := sharedDB()
	if db == nil {
		return errors.New("公共数据库未初始化")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO node_settings(setting_key,payload,updated_at) VALUES(?,?,?) ON CONFLICT(setting_key) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at", key, payload, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func persistContainerRelational(state containerState) error {
	db := sharedDB()
	if db == nil {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM container_compose_projects"); err != nil {
		return err
	}
	for _, item := range state.Composes {
		if _, err = tx.Exec(`INSERT OR REPLACE INTO container_compose_projects(id,name,path,app_install_id,project_name,pinned,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, item.ID, item.Name, item.Path, item.AppInstallID, filepath.Base(filepath.Dir(item.Path)), boolInt(item.Pinned), item.CreatedAt.UTC().Format(time.RFC3339Nano), item.UpdatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("DELETE FROM image_repositories"); err != nil {
		return err
	}
	for _, item := range state.Repositories {
		if _, err = tx.Exec(`INSERT OR REPLACE INTO image_repositories(id,name,download_url,protocol,username,password,auth,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, item.ID, item.Name, item.DownloadURL, item.Protocol, item.Username, item.Password, boolInt(item.Auth), item.CreatedAt.UTC().Format(time.RFC3339Nano), item.UpdatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("DELETE FROM compose_templates"); err != nil {
		return err
	}
	for _, item := range state.Templates {
		if _, err = tx.Exec(`INSERT OR REPLACE INTO compose_templates(id,name,description,content,created_at,updated_at) VALUES(?,?,?,?,?,?)`, item.ID, item.Name, item.Description, item.Content, item.CreatedAt.UTC().Format(time.RFC3339Nano), item.UpdatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func persistAppRelational(state appStoreState) error {
	db := sharedDB()
	if db == nil {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range state.Apps {
		config, _ := json.Marshal(item.Config)
		now := item.UpdatedAt.UTC().Format(time.RFC3339Nano)
		if now == "0001-01-01T00:00:00Z" {
			now = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if _, err = tx.Exec(`INSERT INTO app_installs(id,app_key,name,version,status,install_path,compose_path,compose_project,container_names,config_json,message,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET app_key=excluded.app_key,name=excluded.name,version=excluded.version,status=excluded.status,install_path=excluded.install_path,compose_path=excluded.compose_path,container_names=excluded.container_names,config_json=excluded.config_json,message=excluded.message,updated_at=excluded.updated_at`, item.ID, item.Key, item.Name, item.Version, item.Status, appInstallPath(item), appComposePath(item), appValue(item.Config, "composeProject", "projectName"), item.ContainerName, config, item.Message, now, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
