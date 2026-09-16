// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

type websiteSchemaDB interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
}

// WebsiteSchemaMigration 返回网站和证书领域的版本化关系表迁移。
// 迁移只创建缺失表并补充缺失字段，既有关系数据和废弃物理列均保持原样。
func WebsiteSchemaMigration() storage.Migration {
	sum := sha256.Sum256([]byte("website-relational-v1"))
	return storage.Migration{
		ID:       "0006-website-relational",
		Checksum: hex.EncodeToString(sum[:]),
		Up: func(_ context.Context, tx *sql.Tx) error {
			return ensureWebsiteTables(tx)
		},
	}
}

func ensureWebsiteTables(db websiteSchemaDB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS website_state (state_key TEXT PRIMARY KEY, payload BLOB NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS websites (id INTEGER PRIMARY KEY AUTOINCREMENT, protocol TEXT NOT NULL DEFAULT 'HTTP', primary_domain TEXT NOT NULL UNIQUE, type TEXT NOT NULL DEFAULT 'static', alias TEXT NOT NULL DEFAULT '', remark TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'running', http_config TEXT NOT NULL DEFAULT '', expire_date TEXT NOT NULL DEFAULT '', proxy TEXT NOT NULL DEFAULT '', proxy_type TEXT NOT NULL DEFAULT '', site_dir TEXT NOT NULL DEFAULT '', error_log INTEGER NOT NULL DEFAULT 1, access_log INTEGER NOT NULL DEFAULT 1, default_server INTEGER NOT NULL DEFAULT 0, ipv6 INTEGER NOT NULL DEFAULT 0, rewrite TEXT NOT NULL DEFAULT '', website_group_id INTEGER NOT NULL DEFAULT 0, website_ssl_id INTEGER NOT NULL DEFAULT 0, runtime_id TEXT NOT NULL DEFAULT '', app_install_id INTEGER NOT NULL DEFAULT 0, app_install_ref TEXT NOT NULL DEFAULT '', ftp_id INTEGER NOT NULL DEFAULT 0, parent_website_id INTEGER NOT NULL DEFAULT 0, user TEXT NOT NULL DEFAULT '', "group" TEXT NOT NULL DEFAULT '', group_name TEXT NOT NULL DEFAULT '', db_type TEXT NOT NULL DEFAULT '', db_id INTEGER NOT NULL DEFAULT 0, favorite INTEGER NOT NULL DEFAULT 0, stream_ports TEXT NOT NULL DEFAULT '', udp INTEGER NOT NULL DEFAULT 0, group_id INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_websites_group_status ON websites(group_id, status, id)`,
		`CREATE TABLE IF NOT EXISTS website_domains (id INTEGER PRIMARY KEY AUTOINCREMENT, website_id INTEGER NOT NULL REFERENCES websites(id) ON DELETE CASCADE, domain TEXT NOT NULL, ssl INTEGER NOT NULL DEFAULT 0, port INTEGER NOT NULL DEFAULT 80, created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '', UNIQUE(website_id, domain))`,
		`CREATE INDEX IF NOT EXISTS idx_website_domains_website ON website_domains(website_id, domain)`,
		`CREATE TABLE IF NOT EXISTS website_config_values (website_id INTEGER NOT NULL REFERENCES websites(id) ON DELETE CASCADE, config_type TEXT NOT NULL, content TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL, PRIMARY KEY(website_id, config_type))`,
		`CREATE TABLE IF NOT EXISTS website_settings (website_id INTEGER NOT NULL REFERENCES websites(id) ON DELETE CASCADE, config_type TEXT NOT NULL, content TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL, PRIMARY KEY(website_id, config_type))`,
		`CREATE TABLE IF NOT EXISTS website_default_html (type TEXT PRIMARY KEY, content TEXT NOT NULL, sync INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS website_limit_conn (website_id INTEGER PRIMARY KEY REFERENCES websites(id) ON DELETE CASCADE, enabled INTEGER NOT NULL DEFAULT 0, perserver INTEGER NOT NULL DEFAULT 300, perip INTEGER NOT NULL DEFAULT 25, rate_k INTEGER NOT NULL DEFAULT 512, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS website_dns_accounts (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, provider TEXT NOT NULL, credentials BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS website_openresty_config (id INTEGER PRIMARY KEY CHECK(id=1), version TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 0, default_https INTEGER NOT NULL DEFAULT 0, ssl_reject_handshake INTEGER NOT NULL DEFAULT 0, config_content TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS website_openresty_modules (name TEXT PRIMARY KEY, enabled INTEGER NOT NULL DEFAULT 0, build_mode TEXT NOT NULL DEFAULT '', load_order INTEGER NOT NULL DEFAULT 0, custom INTEGER NOT NULL DEFAULT 0, script TEXT NOT NULL DEFAULT '', packages TEXT NOT NULL DEFAULT '', params TEXT NOT NULL DEFAULT '', provider TEXT NOT NULL DEFAULT '', build_status TEXT NOT NULL DEFAULT '', load_status TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS website_ssls (id INTEGER PRIMARY KEY AUTOINCREMENT, primary_domain TEXT NOT NULL DEFAULT '', private_key TEXT NOT NULL DEFAULT '', pem TEXT NOT NULL DEFAULT '', domains TEXT NOT NULL DEFAULT '', cert_url TEXT NOT NULL DEFAULT '', type TEXT NOT NULL DEFAULT '', provider TEXT NOT NULL DEFAULT '', organization TEXT NOT NULL DEFAULT '', dns_account_id INTEGER NOT NULL DEFAULT 0, acme_account_id INTEGER NOT NULL DEFAULT 0, ca_id INTEGER NOT NULL DEFAULT 0, auto_renew INTEGER NOT NULL DEFAULT 0, expire_date TEXT NOT NULL DEFAULT '', start_date TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '', key_type TEXT NOT NULL DEFAULT '', push_dir INTEGER NOT NULL DEFAULT 0, dir TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', skip_dns INTEGER NOT NULL DEFAULT 0, nameserver1 TEXT NOT NULL DEFAULT '', nameserver2 TEXT NOT NULL DEFAULT '', disable_cname INTEGER NOT NULL DEFAULT 0, exec_shell INTEGER NOT NULL DEFAULT 0, shell TEXT NOT NULL DEFAULT '', master_ssl_id INTEGER NOT NULL DEFAULT 0, nodes TEXT NOT NULL DEFAULT '', push_node INTEGER NOT NULL DEFAULT 0, private_key_path TEXT NOT NULL DEFAULT '', cert_path TEXT NOT NULL DEFAULT '', is_ip INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS website_acme_accounts (id INTEGER PRIMARY KEY AUTOINCREMENT, email TEXT NOT NULL, url TEXT NOT NULL DEFAULT '', private_key TEXT NOT NULL DEFAULT '', type TEXT NOT NULL DEFAULT '', eab_kid TEXT NOT NULL DEFAULT '', eab_hmac_key TEXT NOT NULL DEFAULT '', key_type TEXT NOT NULL DEFAULT '', use_proxy INTEGER NOT NULL DEFAULT 0, ca_dir_url TEXT NOT NULL DEFAULT '', use_eab INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS website_cas (id INTEGER PRIMARY KEY AUTOINCREMENT, csr TEXT NOT NULL DEFAULT '', name TEXT NOT NULL, private_key TEXT NOT NULL DEFAULT '', key_type TEXT NOT NULL DEFAULT '', common_name TEXT NOT NULL DEFAULT '', country TEXT NOT NULL DEFAULT '', organization TEXT NOT NULL DEFAULT '', organization_unit TEXT NOT NULL DEFAULT '', province TEXT NOT NULL DEFAULT '', city TEXT NOT NULL DEFAULT '', certificate TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS website_ca_ssls (id INTEGER PRIMARY KEY AUTOINCREMENT, ca_id INTEGER NOT NULL REFERENCES website_cas(id) ON DELETE RESTRICT, primary_domain TEXT NOT NULL, domains TEXT NOT NULL DEFAULT '', certificate TEXT NOT NULL, private_key TEXT NOT NULL, start_date TEXT NOT NULL, expire_date TEXT NOT NULL, status TEXT NOT NULL, type TEXT NOT NULL, key_type TEXT NOT NULL, auto_renew INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("初始化网站数据库表失败: %w", err)
		}
	}
	// WAF runtime state is file-backed. Remove obsolete SQLite state tables so
	// old installations cannot accidentally become a second source of truth.
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS website_waf_rules`, `DROP TABLE IF EXISTS website_waf_sites`,
		`DROP TABLE IF EXISTS website_waf_access_entries`, `DROP TABLE IF EXISTS website_waf_global`,
		`DROP TABLE IF EXISTS website_waf_sidecar_imports`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("清理旧 WAF 数据表失败: %w", err)
		}
	}
	// One-time compatibility migration; runtime reads and writes use website_settings.
	_, _ = db.Exec(`INSERT OR IGNORE INTO website_settings(website_id,config_type,content,updated_at) SELECT website_id,config_type,content,updated_at FROM website_config_values`)
	// 兼容已经由旧版本创建的 websites 表，逐列补齐而不覆盖用户数据。
	for _, col := range []struct{ name, typ, def string }{
		{"protocol", "TEXT", "'HTTP'"}, {"type", "TEXT", "'static'"}, {"alias", "TEXT", "''"}, {"remark", "TEXT", "''"}, {"http_config", "TEXT", "''"}, {"expire_date", "TEXT", "''"}, {"proxy", "TEXT", "''"}, {"proxy_type", "TEXT", "''"}, {"site_dir", "TEXT", "''"}, {"error_log", "INTEGER", "1"}, {"access_log", "INTEGER", "1"}, {"default_server", "INTEGER", "0"}, {"ipv6", "INTEGER", "0"}, {"rewrite", "TEXT", "''"}, {"website_group_id", "INTEGER", "0"}, {"website_ssl_id", "INTEGER", "0"}, {"runtime_id", "TEXT", "''"}, {"app_install_id", "INTEGER", "0"}, {"app_install_ref", "TEXT", "''"}, {"ftp_id", "INTEGER", "0"}, {"parent_website_id", "INTEGER", "0"}, {"user", "TEXT", "''"}, {"group_name", "TEXT", "''"}, {"db_type", "TEXT", "''"}, {"db_id", "INTEGER", "0"}, {"favorite", "INTEGER", "0"}, {"stream_ports", "TEXT", "''"}, {"udp", "INTEGER", "0"},
	} {
		if err := ensureColumn(db, "websites", col.name, col.typ, col.def); err != nil {
			return err
		}
	}
	if err := ensureColumn(db, "websites", "group", "TEXT", "''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "website_domains", "created_at", "TEXT", "''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "website_domains", "updated_at", "TEXT", "''"); err != nil {
		return err
	}
	for _, col := range []struct{ name, typ, def string }{
		{"custom", "INTEGER", "0"},
		{"script", "TEXT", "''"},
		{"packages", "TEXT", "''"},
		{"params", "TEXT", "''"},
		{"provider", "TEXT", "''"},
		{"build_status", "TEXT", "''"},
		{"load_status", "TEXT", "''"},
		{"last_error", "TEXT", "''"},
	} {
		if err := ensureColumn(db, "website_openresty_modules", col.name, col.typ, col.def); err != nil {
			return err
		}
	}
	// 旧版本以 INTEGER 保存运行时数字 ID；转换为十进制文本，兼容历史引用。
	_, _ = db.Exec(`UPDATE websites SET runtime_id=CAST(runtime_id AS TEXT) WHERE typeof(runtime_id)='integer' AND runtime_id<>0`)
	_, _ = db.Exec(`UPDATE websites SET runtime_id='' WHERE TRIM(CAST(runtime_id AS TEXT))='0'`)
	return nil
}

func ensureColumn(db websiteSchemaDB, table, name, typ, def string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	var found bool
	for rows.Next() {
		var cid int
		var n, t string
		var notnull, pk int
		var d any
		if rows.Scan(&cid, &n, &t, &notnull, &d, &pk) == nil && n == name {
			found = true
			break
		}
	}
	if found {
		return nil
	}
	quoted := name
	if name == "group" {
		quoted = `"group"`
	}
	_, err = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + quoted + " " + typ + " NOT NULL DEFAULT " + def)
	return err
}

// hasColumn 用于识别历史物理表的非空兼容列。该列绝不作为业务数据来源，
// 仅在写入旧表时填充空 BLOB，保证增量升级不因旧约束而中断用户已有关系数据。
func hasColumn(db storage.SQLExecutor, table, name string) (bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var columnName, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if columnName == name {
			return true, nil
		}
	}
	return false, rows.Err()
}

// DNSAccount 是站点管理使用的 DNS provider 账户登记，不包含明文凭据返回值。
type DNSAccount struct {
	ID            uint           `json:"id"`
	Name          string         `json:"name"`
	Provider      string         `json:"provider,omitempty"`
	Type          string         `json:"type,omitempty"`
	Authorization map[string]any `json:"authorization,omitempty"`
	Credentials   map[string]any `json:"-"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}

func (s *WebsiteService) ListDNSAccounts(keyword string, page, pageSize int) (int, []DNSAccount) {
	repository, err := s.sqliteRepository()
	if err != nil {
		return 0, []DNSAccount{}
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	pattern := "%" + strings.ToLower(strings.TrimSpace(keyword)) + "%"
	var total int
	_ = repository.QueryRow(`SELECT COUNT(*) FROM website_dns_accounts WHERE lower(name) LIKE ? OR lower(provider) LIKE ?`, pattern, pattern).Scan(&total)
	rows, err := repository.Query(`SELECT id,name,provider,created_at,updated_at FROM website_dns_accounts WHERE lower(name) LIKE ? OR lower(provider) LIKE ? ORDER BY id DESC LIMIT ? OFFSET ?`, pattern, pattern, pageSize, (page-1)*pageSize)
	if err != nil {
		return total, []DNSAccount{}
	}
	defer rows.Close()
	items := []DNSAccount{}
	for rows.Next() {
		var item DNSAccount
		var created, updated string
		if rows.Scan(&item.ID, &item.Name, &item.Provider, &created, &updated) == nil {
			item.Type = item.Provider
			// 凭据不回传；保留 1Panel DTO 的对象形状，避免编辑表单访问 undefined。
			item.Authorization = map[string]any{}
			item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
			item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
			items = append(items, item)
		}
	}
	return total, items
}

func (s *WebsiteService) UpsertDNSAccount(item DNSAccount) (DNSAccount, error) {
	item.Name = strings.TrimSpace(item.Name)
	if strings.TrimSpace(item.Provider) == "" {
		item.Provider = item.Type
	}
	item.Provider = strings.TrimSpace(item.Provider)
	item.Type = item.Provider
	if item.Name == "" || item.Provider == "" {
		return DNSAccount{}, errors.New("DNS 账户名称和提供商不能为空")
	}
	var cred []byte
	if item.Credentials != nil {
		var err error
		cred, err = json.Marshal(item.Credentials)
		if err != nil {
			return DNSAccount{}, fmt.Errorf("序列化 DNS 账户凭据失败: %w", err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	repository, err := s.sqliteRepository()
	if err != nil {
		return DNSAccount{}, err
	}
	var created string
	if err := repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
		if item.ID == 0 {
			if cred == nil {
				cred = []byte(`{}`)
			}
			res, err := tx.Exec(`INSERT INTO website_dns_accounts(name,provider,credentials,created_at,updated_at) VALUES(?,?,?,?,?)`, item.Name, item.Provider, cred, now, now)
			if err != nil {
				return err
			}
			id, err := res.LastInsertId()
			if err != nil {
				return err
			}
			item.ID = uint(id)
			created = now
			return nil
		}
		if err := tx.QueryRow(`SELECT created_at FROM website_dns_accounts WHERE id=?`, item.ID).Scan(&created); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return os.ErrNotExist
			}
			return err
		}
		if cred == nil {
			_, err = tx.Exec(`UPDATE website_dns_accounts SET name=?,provider=?,updated_at=? WHERE id=?`, item.Name, item.Provider, now, item.ID)
		} else {
			_, err = tx.Exec(`UPDATE website_dns_accounts SET name=?,provider=?,credentials=?,updated_at=? WHERE id=?`, item.Name, item.Provider, cred, now, item.ID)
		}
		return err
	}); err != nil {
		return DNSAccount{}, err
	}
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, now)
	item.Credentials = nil
	item.Authorization = nil
	return item, nil
}

func (s *WebsiteService) DeleteDNSAccount(id uint) error {
	repository, err := s.sqliteRepository()
	if err != nil {
		return err
	}
	return repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
		var references int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM website_ssls WHERE dns_account_id=?`, id).Scan(&references); err != nil {
			return err
		}
		if references > 0 {
			return errors.New("DNS 账户已被证书引用，不能删除")
		}
		res, err := tx.Exec(`DELETE FROM website_dns_accounts WHERE id=?`, id)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return os.ErrNotExist
		}
		return nil
	})
}

// NewWebsiteService 创建服务并从数据目录加载已有状态。
