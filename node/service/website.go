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
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

var domainPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)

const defaultWebsiteIndexHTML = `<!doctype html>
<html>
<head><meta charset="utf-8"><meta name="robots" content="noindex,nofollow"><title>站点创建成功</title></head>
<body><h1>恭喜，站点创建成功！</h1><p>这是系统自动生成的默认 index.html。</p></body>
</html>
`

const defaultWebsite404HTML = `<html>
<head><title>404 Not Found</title></head>
<body><center><h1>404 Not Found</h1></center><hr><center>nginx</center></body>
</html>
`

// OpenRestyStatus 描述节点上 OpenResty 的真实探测结果。
type OpenRestyStatus struct {
	Available    bool                    `json:"available"`
	Binary       string                  `json:"binary,omitempty"`
	Version      string                  `json:"version,omitempty"`
	ConfigValid  bool                    `json:"configValid"`
	Enabled      bool                    `json:"enabled"`
	DefaultHTTPS bool                    `json:"defaultHttps"`
	Modules      []model.OpenRestyModule `json:"modules"`
	ProcessID    int                     `json:"processId,omitempty"`
	Cgroup       string                  `json:"cgroup,omitempty"`
	ConfigPath   string                  `json:"configPath,omitempty"`
	Listening    []int                   `json:"listening,omitempty"`
	Active       int                     `json:"active"`
	Accepts      int64                   `json:"accepts"`
	Handled      int64                   `json:"handled"`
	Requests     int64                   `json:"requests"`
	Reading      int                     `json:"reading"`
	Writing      int                     `json:"writing"`
	Waiting      int                     `json:"waiting"`
	Error        string                  `json:"error,omitempty"`
}

// OpenRestyScopeParams 是按配置作用域读取或更新的指令集合。
type OpenRestyScopeParams struct {
	Scope  string            `json:"scope"`
	Params map[string]string `json:"params"`
}

// WebsiteService 提供网站、WAF 和 OpenResty 的轻量本地控制面。
// 文件采用原子替换保存，节点未配置数据库时重启仍能保留配置。
type WebsiteService struct {
	mu        sync.RWMutex
	root      string
	websites  []model.Website
	wafSites  map[uint]model.WAFSite
	global    model.WAFGlobalConfig
	lists     model.WAFAccessLists
	openresty model.OpenRestyConfig
	domains   map[uint][]model.WebsiteDomain
	configs   map[uint]map[string]any
	db        *sql.DB
	owner     *storage.Store
}

var (
	websiteDBMu sync.RWMutex
	websiteDB   *sql.DB
)

// SetWebsiteDB 注入进程唯一的公共数据库连接。
func SetWebsiteDB(db *sql.DB) error {
	if db == nil {
		return errors.New("网站公共数据库连接不能为空")
	}
	if err := ensureWebsiteTables(db); err != nil {
		return err
	}
	websiteDBMu.Lock()
	websiteDB = db
	websiteDBMu.Unlock()
	return nil
}

func currentWebsiteDB() *sql.DB {
	websiteDBMu.RLock()
	defer websiteDBMu.RUnlock()
	return websiteDB
}

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
		`CREATE TABLE IF NOT EXISTS websites (id INTEGER PRIMARY KEY AUTOINCREMENT, protocol TEXT NOT NULL DEFAULT 'HTTP', primary_domain TEXT NOT NULL UNIQUE, type TEXT NOT NULL DEFAULT 'static', alias TEXT NOT NULL DEFAULT '', remark TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'running', http_config TEXT NOT NULL DEFAULT '', expire_date TEXT NOT NULL DEFAULT '', proxy TEXT NOT NULL DEFAULT '', proxy_type TEXT NOT NULL DEFAULT '', site_dir TEXT NOT NULL DEFAULT '', error_log INTEGER NOT NULL DEFAULT 1, access_log INTEGER NOT NULL DEFAULT 1, default_server INTEGER NOT NULL DEFAULT 0, ipv6 INTEGER NOT NULL DEFAULT 0, rewrite TEXT NOT NULL DEFAULT '', website_group_id INTEGER NOT NULL DEFAULT 0, website_ssl_id INTEGER NOT NULL DEFAULT 0, runtime_id TEXT NOT NULL DEFAULT '', app_install_id INTEGER NOT NULL DEFAULT 0, ftp_id INTEGER NOT NULL DEFAULT 0, parent_website_id INTEGER NOT NULL DEFAULT 0, user TEXT NOT NULL DEFAULT '', "group" TEXT NOT NULL DEFAULT '', group_name TEXT NOT NULL DEFAULT '', db_type TEXT NOT NULL DEFAULT '', db_id INTEGER NOT NULL DEFAULT 0, favorite INTEGER NOT NULL DEFAULT 0, stream_ports TEXT NOT NULL DEFAULT '', udp INTEGER NOT NULL DEFAULT 0, group_id INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_websites_group_status ON websites(group_id, status, id)`,
		`CREATE TABLE IF NOT EXISTS website_domains (id INTEGER PRIMARY KEY AUTOINCREMENT, website_id INTEGER NOT NULL REFERENCES websites(id) ON DELETE CASCADE, domain TEXT NOT NULL, ssl INTEGER NOT NULL DEFAULT 0, port INTEGER NOT NULL DEFAULT 80, created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '', UNIQUE(website_id, domain))`,
		`CREATE INDEX IF NOT EXISTS idx_website_domains_website ON website_domains(website_id, domain)`,
		`CREATE TABLE IF NOT EXISTS website_config_values (website_id INTEGER NOT NULL REFERENCES websites(id) ON DELETE CASCADE, config_type TEXT NOT NULL, content TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL, PRIMARY KEY(website_id, config_type))`,
		`CREATE TABLE IF NOT EXISTS website_dns_accounts (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, provider TEXT NOT NULL, credentials BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS website_waf_sites (website_id INTEGER PRIMARY KEY REFERENCES websites(id) ON DELETE CASCADE, enabled INTEGER NOT NULL DEFAULT 1, mode TEXT NOT NULL DEFAULT 'observe', updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS website_waf_rules (id INTEGER PRIMARY KEY AUTOINCREMENT, website_id INTEGER NOT NULL REFERENCES websites(id) ON DELETE CASCADE, name TEXT NOT NULL, location TEXT NOT NULL, rule_key TEXT NOT NULL DEFAULT '', operator TEXT NOT NULL, value TEXT NOT NULL, action TEXT NOT NULL, priority INTEGER NOT NULL DEFAULT 0, enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS website_waf_access_entries (kind TEXT NOT NULL, value TEXT NOT NULL, position INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL, PRIMARY KEY(kind, value))`,
		`CREATE TABLE IF NOT EXISTS website_waf_global (id INTEGER PRIMARY KEY CHECK(id=1), enabled INTEGER NOT NULL DEFAULT 1, standard_rules INTEGER NOT NULL DEFAULT 1, mode TEXT NOT NULL DEFAULT 'observe', paranoia_level INTEGER NOT NULL DEFAULT 1, inbound_threshold INTEGER NOT NULL DEFAULT 5, request_body_limit INTEGER NOT NULL DEFAULT 1048576, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS website_openresty_config (id INTEGER PRIMARY KEY CHECK(id=1), version TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 0, default_https INTEGER NOT NULL DEFAULT 0, ssl_reject_handshake INTEGER NOT NULL DEFAULT 0, config_content TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS website_openresty_modules (name TEXT PRIMARY KEY, enabled INTEGER NOT NULL DEFAULT 0, build_mode TEXT NOT NULL DEFAULT '', load_order INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS website_ssls (id INTEGER PRIMARY KEY AUTOINCREMENT, primary_domain TEXT NOT NULL DEFAULT '', private_key TEXT NOT NULL DEFAULT '', pem TEXT NOT NULL DEFAULT '', domains TEXT NOT NULL DEFAULT '', cert_url TEXT NOT NULL DEFAULT '', type TEXT NOT NULL DEFAULT '', provider TEXT NOT NULL DEFAULT '', organization TEXT NOT NULL DEFAULT '', dns_account_id INTEGER NOT NULL DEFAULT 0, acme_account_id INTEGER NOT NULL DEFAULT 0, ca_id INTEGER NOT NULL DEFAULT 0, auto_renew INTEGER NOT NULL DEFAULT 0, expire_date TEXT NOT NULL DEFAULT '', start_date TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '', key_type TEXT NOT NULL DEFAULT '', push_dir INTEGER NOT NULL DEFAULT 0, dir TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', skip_dns INTEGER NOT NULL DEFAULT 0, nameserver1 TEXT NOT NULL DEFAULT '', nameserver2 TEXT NOT NULL DEFAULT '', disable_cname INTEGER NOT NULL DEFAULT 0, exec_shell INTEGER NOT NULL DEFAULT 0, shell TEXT NOT NULL DEFAULT '', master_ssl_id INTEGER NOT NULL DEFAULT 0, nodes TEXT NOT NULL DEFAULT '', push_node INTEGER NOT NULL DEFAULT 0, private_key_path TEXT NOT NULL DEFAULT '', cert_path TEXT NOT NULL DEFAULT '', is_ip INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS website_acme_accounts (id INTEGER PRIMARY KEY AUTOINCREMENT, email TEXT NOT NULL, url TEXT NOT NULL DEFAULT '', private_key TEXT NOT NULL DEFAULT '', type TEXT NOT NULL DEFAULT '', eab_kid TEXT NOT NULL DEFAULT '', eab_hmac_key TEXT NOT NULL DEFAULT '', key_type TEXT NOT NULL DEFAULT '', use_proxy INTEGER NOT NULL DEFAULT 0, ca_dir_url TEXT NOT NULL DEFAULT '', use_eab INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS website_cas (id INTEGER PRIMARY KEY AUTOINCREMENT, csr TEXT NOT NULL DEFAULT '', name TEXT NOT NULL, private_key TEXT NOT NULL DEFAULT '', key_type TEXT NOT NULL DEFAULT '', common_name TEXT NOT NULL DEFAULT '', country TEXT NOT NULL DEFAULT '', organization TEXT NOT NULL DEFAULT '', organization_unit TEXT NOT NULL DEFAULT '', province TEXT NOT NULL DEFAULT '', city TEXT NOT NULL DEFAULT '', certificate TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS website_ca_ssls (id INTEGER PRIMARY KEY AUTOINCREMENT, ca_id INTEGER NOT NULL REFERENCES website_cas(id) ON DELETE RESTRICT, primary_domain TEXT NOT NULL, domains TEXT NOT NULL DEFAULT '', certificate TEXT NOT NULL, private_key TEXT NOT NULL, start_date TEXT NOT NULL, expire_date TEXT NOT NULL, status TEXT NOT NULL, type TEXT NOT NULL, key_type TEXT NOT NULL, auto_renew INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("初始化网站数据库表失败: %w", err)
		}
	}
	// 兼容已经由旧版本创建的 websites 表，逐列补齐而不覆盖用户数据。
	for _, col := range []struct{ name, typ, def string }{
		{"protocol", "TEXT", "'HTTP'"}, {"type", "TEXT", "'static'"}, {"alias", "TEXT", "''"}, {"remark", "TEXT", "''"}, {"http_config", "TEXT", "''"}, {"expire_date", "TEXT", "''"}, {"proxy", "TEXT", "''"}, {"proxy_type", "TEXT", "''"}, {"site_dir", "TEXT", "''"}, {"error_log", "INTEGER", "1"}, {"access_log", "INTEGER", "1"}, {"default_server", "INTEGER", "0"}, {"ipv6", "INTEGER", "0"}, {"rewrite", "TEXT", "''"}, {"website_group_id", "INTEGER", "0"}, {"website_ssl_id", "INTEGER", "0"}, {"runtime_id", "TEXT", "''"}, {"app_install_id", "INTEGER", "0"}, {"ftp_id", "INTEGER", "0"}, {"parent_website_id", "INTEGER", "0"}, {"user", "TEXT", "''"}, {"group_name", "TEXT", "''"}, {"db_type", "TEXT", "''"}, {"db_id", "INTEGER", "0"}, {"favorite", "INTEGER", "0"}, {"stream_ports", "TEXT", "''"}, {"udp", "INTEGER", "0"},
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
func hasColumn(db websiteSchemaDB, table, name string) (bool, error) {
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
	ID          uint           `json:"id"`
	Name        string         `json:"name"`
	Provider    string         `json:"provider"`
	Credentials map[string]any `json:"-"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

func (s *WebsiteService) ListDNSAccounts(keyword string, page, pageSize int) (int, []DNSAccount) {
	if s.db == nil {
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
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM website_dns_accounts WHERE lower(name) LIKE ? OR lower(provider) LIKE ?`, pattern, pattern).Scan(&total)
	rows, err := s.db.Query(`SELECT id,name,provider,created_at,updated_at FROM website_dns_accounts WHERE lower(name) LIKE ? OR lower(provider) LIKE ? ORDER BY id DESC LIMIT ? OFFSET ?`, pattern, pattern, pageSize, (page-1)*pageSize)
	if err != nil {
		return total, []DNSAccount{}
	}
	defer rows.Close()
	items := []DNSAccount{}
	for rows.Next() {
		var item DNSAccount
		var created, updated string
		if rows.Scan(&item.ID, &item.Name, &item.Provider, &created, &updated) == nil {
			item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
			item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
			items = append(items, item)
		}
	}
	return total, items
}

func (s *WebsiteService) UpsertDNSAccount(item DNSAccount) (DNSAccount, error) {
	if s.db == nil {
		return DNSAccount{}, errors.New("网站公共数据库未初始化")
	}
	item.Name, item.Provider = strings.TrimSpace(item.Name), strings.TrimSpace(item.Provider)
	if item.Name == "" || item.Provider == "" {
		return DNSAccount{}, errors.New("DNS 账户名称和提供商不能为空")
	}
	cred, _ := json.Marshal(item.Credentials)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if item.ID == 0 {
		res, err := s.db.Exec(`INSERT INTO website_dns_accounts(name,provider,credentials,created_at,updated_at) VALUES(?,?,?,?,?)`, item.Name, item.Provider, cred, now, now)
		if err != nil {
			return DNSAccount{}, err
		}
		id, _ := res.LastInsertId()
		item.ID = uint(id)
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	} else {
		if _, err := s.db.Exec(`UPDATE website_dns_accounts SET name=?,provider=?,credentials=?,updated_at=? WHERE id=?`, item.Name, item.Provider, cred, now, item.ID); err != nil {
			return DNSAccount{}, err
		}
	}
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, now)
	item.Credentials = nil
	return item, nil
}

func (s *WebsiteService) DeleteDNSAccount(id uint) error {
	if s.db == nil {
		return errors.New("网站公共数据库未初始化")
	}
	res, err := s.db.Exec(`DELETE FROM website_dns_accounts WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return os.ErrNotExist
	}
	return nil
}

// NewWebsiteService 创建服务并从数据目录加载已有状态。
func NewWebsiteService(root string) *WebsiteService {
	if strings.TrimSpace(root) == "" {
		root = os.Getenv("WORKMESH_DATA_DIR")
	}
	if strings.TrimSpace(root) == "" {
		root = "./data"
	}
	db := currentWebsiteDB()
	var owner *storage.Store
	if db == nil {
		if opened, err := storage.Open(filepath.Join(root, "workmesh.db")); err == nil {
			owner = opened
			db = opened.DB()
		}
	}
	if db != nil {
		_ = ensureWebsiteTables(db)
	}
	s := &WebsiteService{root: root, db: db, owner: owner, wafSites: map[uint]model.WAFSite{}, domains: map[uint][]model.WebsiteDomain{}, configs: map[uint]map[string]any{}}
	s.load()
	return s
}

func (s *WebsiteService) load() {
	// 正式数据从关系表加载；只有旧数据库没有任何关系记录时才读取旧 blob 迁移数据。
	if s.db != nil {
		if rows, err := s.db.Query(`SELECT id,protocol,primary_domain,type,alias,remark,status,http_config,expire_date,proxy,proxy_type,site_dir,error_log,access_log,default_server,ipv6,rewrite,website_group_id,website_ssl_id,runtime_id,app_install_id,ftp_id,parent_website_id,user,"group",db_type,db_id,favorite,stream_ports,udp,created_at,updated_at FROM websites ORDER BY id`); err == nil {
			for rows.Next() {
				var item model.Website
				var id, groupID, sslID, appID, ftpID, parentID, dbID int64
				var runtimeID string
				var errLog, accessLog, defaultServer, ipv6, favorite, udp int
				var expire, created, updated string
				if rows.Scan(&id, &item.Protocol, &item.PrimaryDomain, &item.Type, &item.Alias, &item.Remark, &item.Status, &item.HttpConfig, &expire, &item.Proxy, &item.ProxyType, &item.SiteDir, &errLog, &accessLog, &defaultServer, &ipv6, &item.Rewrite, &groupID, &sslID, &runtimeID, &appID, &ftpID, &parentID, &item.User, &item.Group, &item.DbType, &dbID, &favorite, &item.StreamPorts, &udp, &created, &updated) != nil {
					continue
				}
				item.ID = uint(id)
				item.WebsiteGroupID = uint(groupID)
				item.WebsiteSSLID = uint(sslID)
				item.RuntimeID = strings.TrimSpace(runtimeID)
				item.AppInstallID = uint(appID)
				item.FtpID = uint(ftpID)
				item.ParentWebsiteID = uint(parentID)
				item.DbID = uint(dbID)
				item.ErrorLog = errLog != 0
				item.AccessLog = accessLog != 0
				item.DefaultServer = defaultServer != 0
				item.IPV6 = ipv6 != 0
				item.Favorite = favorite != 0
				item.UDP = udp != 0
				item.ExpireDate, _ = time.Parse(time.RFC3339Nano, expire)
				item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
				item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
				s.websites = append(s.websites, item)
			}
			rows.Close()
		}
	}
	// 网站状态只来自关系表；数据库为空时网站列表就是空集合。
	var sites []model.WAFSite
	if s.db != nil {
		if rows, err := s.db.Query(`SELECT website_id,enabled,mode FROM website_waf_sites ORDER BY website_id`); err == nil {
			for rows.Next() {
				var id int64
				var enabled int
				var mode string
				if rows.Scan(&id, &enabled, &mode) == nil {
					sites = append(sites, model.WAFSite{WebsiteID: uint(id), Enabled: enabled != 0, Mode: mode, Rules: []model.WAFRule{}})
				}
			}
			rows.Close()
			for i := range sites {
				rr, _ := s.db.Query(`SELECT id,name,location,rule_key,operator,value,action,priority,enabled FROM website_waf_rules WHERE website_id=? ORDER BY priority,id`, sites[i].WebsiteID)
				if rr != nil {
					for rr.Next() {
						var id, priority int64
						var enabled int
						var rule model.WAFRule
						if rr.Scan(&id, &rule.Name, &rule.Location, &rule.Key, &rule.Operator, &rule.Value, &rule.Action, &priority, &enabled) == nil {
							rule.ID = strconv.FormatInt(id, 10)
							rule.Priority = int(priority)
							rule.Enabled = enabled != 0
							sites[i].Rules = append(sites[i].Rules, rule)
						}
					}
					rr.Close()
				}
			}
		}
	}
	for _, site := range sites {
		if site.Rules == nil {
			site.Rules = []model.WAFRule{}
		}
		s.wafSites[site.WebsiteID] = site
	}
	// 域名目录中的 WAF 文件是站点配置的权威副本；数据库仅作兼容索引。
	for _, site := range s.websites {
		path := s.SitePath(site, "waf")
		if data, readErr := os.ReadFile(filepath.Join(path, "site.json")); readErr == nil {
			var cfg model.WAFSite
			if json.Unmarshal(data, &cfg) == nil && cfg.WebsiteID == site.ID {
				s.wafSites[site.ID] = cfg
			}
		}
	}
	s.global = model.WAFGlobalConfig{Enabled: true, StandardRules: true, Mode: "observe", ParanoiaLevel: 1, InboundThreshold: 5, RequestBodyLimit: 1 << 20}
	s.lists = model.WAFAccessLists{Whitelist: []string{}, Blacklist: []string{}}
	globalDir := strings.TrimSpace(os.Getenv("WORKMESH_WAF_GLOBAL_DIR"))
	if globalDir == "" {
		globalDir = filepath.Join(s.root, "waf")
	}
	if data, readErr := os.ReadFile(filepath.Join(globalDir, "global.json")); readErr == nil {
		_ = json.Unmarshal(data, &s.global)
	}
	if s.db != nil {
		var enabled, standard int
		if s.db.QueryRow(`SELECT enabled,standard_rules,mode,paranoia_level,inbound_threshold,request_body_limit FROM website_waf_global WHERE id=1`).Scan(&enabled, &standard, &s.global.Mode, &s.global.ParanoiaLevel, &s.global.InboundThreshold, &s.global.RequestBodyLimit) == nil {
			s.global.Enabled = enabled != 0
			s.global.StandardRules = standard != 0
		}
		for kind, target := range map[string]*[]string{"whitelist": &s.lists.Whitelist, "blacklist": &s.lists.Blacklist} {
			rows, _ := s.db.Query(`SELECT value FROM website_waf_access_entries WHERE kind=? ORDER BY position,value`, kind)
			if rows != nil {
				for rows.Next() {
					var value string
					if rows.Scan(&value) == nil {
						*target = append(*target, value)
					}
				}
				rows.Close()
			}
		}
	}
	s.openresty = model.OpenRestyConfig{Version: "1.27.1", Enabled: true, Modules: []model.OpenRestyModule{}}
	if s.db != nil {
		var enabled, defaultHTTPS, reject int
		var updated string
		if s.db.QueryRow(`SELECT version,enabled,default_https,ssl_reject_handshake,config_content,updated_at FROM website_openresty_config WHERE id=1`).Scan(&s.openresty.Version, &enabled, &defaultHTTPS, &reject, &s.openresty.ConfigContent, &updated) == nil {
			s.openresty.Enabled = enabled != 0
			s.openresty.DefaultHTTPS = defaultHTTPS != 0
			s.openresty.SSLRejectHandshake = reject != 0
			s.openresty.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		}
		rows, _ := s.db.Query(`SELECT name,enabled,build_mode,load_order FROM website_openresty_modules ORDER BY load_order,name`)
		if rows != nil {
			for rows.Next() {
				var m model.OpenRestyModule
				var enabled int
				if rows.Scan(&m.Name, &enabled, &m.BuildMode, &m.LoadOrder) == nil {
					m.Enabled = enabled != 0
					s.openresty.Modules = append(s.openresty.Modules, m)
				}
			}
			rows.Close()
		}
	}
	if s.db != nil {
		if rows, err := s.db.Query(`SELECT id,website_id,domain,port,ssl FROM website_domains ORDER BY id`); err == nil {
			for rows.Next() {
				var idRaw string
				var websiteID, port int64
				var domain string
				var ssl int
				if rows.Scan(&idRaw, &websiteID, &domain, &port, &ssl) == nil {
					if _, err := strconv.ParseInt(idRaw, 10, 64); err != nil && idRaw == "" {
						idRaw = strconv.FormatInt(time.Now().UnixNano(), 10)
					}
					s.domains[uint(websiteID)] = append(s.domains[uint(websiteID)], model.WebsiteDomain{ID: idRaw, WebsiteID: uint(websiteID), Domain: domain, Port: int(port), SSL: ssl != 0})
				}
			}
			rows.Close()
		}
	}
	// 域名和配置只从当前关系表读取。
	if s.domains == nil {
		s.domains = map[uint][]model.WebsiteDomain{}
	}
	domainsBackfilled := false
	for _, site := range s.websites {
		if strings.EqualFold(site.Type, "stream") || strings.TrimSpace(site.PrimaryDomain) == "" {
			continue
		}
		found := false
		for _, item := range s.domains[site.ID] {
			if strings.EqualFold(strings.TrimSpace(item.Domain), strings.TrimSpace(site.PrimaryDomain)) {
				found = true
				break
			}
		}
		if !found {
			primary := model.WebsiteDomain{WebsiteID: site.ID, Domain: strings.TrimSpace(site.PrimaryDomain), Port: 80}
			if strings.EqualFold(site.Protocol, "HTTPS") {
				primary.Port, primary.SSL = 443, true
			}
			s.domains[site.ID] = append([]model.WebsiteDomain{primary}, s.domains[site.ID]...)
			domainsBackfilled = true
		}
	}
	if domainsBackfilled {
		_ = s.persist("website-domains", s.domains)
	}
	if s.configs == nil {
		s.configs = map[uint]map[string]any{}
	}
	if s.db != nil {
		if rows, err := s.db.Query(`SELECT website_id,config_type,content FROM website_config_values`); err == nil {
			for rows.Next() {
				var id int64
				var typ, content string
				if rows.Scan(&id, &typ, &content) == nil {
					var value map[string]any
					if json.Unmarshal([]byte(content), &value) == nil {
						if s.configs[uint(id)] == nil {
							s.configs[uint(id)] = map[string]any{}
						}
						s.configs[uint(id)][typ] = value
					}
				}
			}
			rows.Close()
		}
	}
	if s.websites == nil {
		s.websites = []model.Website{}
	}
}

func (s *WebsiteService) persist(name string, value any) error {
	if s.db == nil {
		return errors.New("网站公共数据库未初始化")
	}
	key := strings.TrimSpace(name)
	// 网站和域名正式写入关系表，禁止写入旧状态 blob。
	if key == "websites" {
		items, ok := value.([]model.Website)
		if !ok {
			return errors.New("网站数据类型无效")
		}
		legacyPayload, err := hasColumn(s.db, "websites", "payload")
		if err != nil {
			return fmt.Errorf("读取网站表结构失败: %w", err)
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				_ = tx.Rollback()
			}
		}()
		for _, item := range items {
			args := []any{item.ID, item.Protocol, item.PrimaryDomain, item.Type, item.Alias, item.Remark, item.Status, item.HttpConfig, formatTime(item.ExpireDate), item.Proxy, item.ProxyType, item.SiteDir, boolInt(item.ErrorLog), boolInt(item.AccessLog), boolInt(item.DefaultServer), boolInt(item.IPV6), item.Rewrite, item.WebsiteGroupID, item.WebsiteSSLID, item.RuntimeID, item.AppInstallID, item.FtpID, item.ParentWebsiteID, item.User, item.Group, item.DbType, item.DbID, boolInt(item.Favorite), item.StreamPorts, boolInt(item.UDP), formatTime(item.CreatedAt), formatTime(item.UpdatedAt)}
			statement := `INSERT INTO websites(id,protocol,primary_domain,type,alias,remark,status,http_config,expire_date,proxy,proxy_type,site_dir,error_log,access_log,default_server,ipv6,rewrite,website_group_id,website_ssl_id,runtime_id,app_install_id,ftp_id,parent_website_id,user,"group",db_type,db_id,favorite,stream_ports,udp,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET protocol=excluded.protocol,primary_domain=excluded.primary_domain,type=excluded.type,alias=excluded.alias,remark=excluded.remark,status=excluded.status,http_config=excluded.http_config,expire_date=excluded.expire_date,proxy=excluded.proxy,proxy_type=excluded.proxy_type,site_dir=excluded.site_dir,error_log=excluded.error_log,access_log=excluded.access_log,default_server=excluded.default_server,ipv6=excluded.ipv6,rewrite=excluded.rewrite,website_group_id=excluded.website_group_id,website_ssl_id=excluded.website_ssl_id,runtime_id=excluded.runtime_id,app_install_id=excluded.app_install_id,ftp_id=excluded.ftp_id,parent_website_id=excluded.parent_website_id,user=excluded.user,"group"=excluded."group",db_type=excluded.db_type,db_id=excluded.db_id,favorite=excluded.favorite,stream_ports=excluded.stream_ports,udp=excluded.udp,updated_at=excluded.updated_at`
			if legacyPayload {
				statement = `INSERT INTO websites(id,protocol,primary_domain,type,alias,remark,status,http_config,expire_date,proxy,proxy_type,site_dir,error_log,access_log,default_server,ipv6,rewrite,website_group_id,website_ssl_id,runtime_id,app_install_id,ftp_id,parent_website_id,user,"group",db_type,db_id,favorite,stream_ports,udp,created_at,updated_at,payload) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET protocol=excluded.protocol,primary_domain=excluded.primary_domain,type=excluded.type,alias=excluded.alias,remark=excluded.remark,status=excluded.status,http_config=excluded.http_config,expire_date=excluded.expire_date,proxy=excluded.proxy,proxy_type=excluded.proxy_type,site_dir=excluded.site_dir,error_log=excluded.error_log,access_log=excluded.access_log,default_server=excluded.default_server,ipv6=excluded.ipv6,rewrite=excluded.rewrite,website_group_id=excluded.website_group_id,website_ssl_id=excluded.website_ssl_id,runtime_id=excluded.runtime_id,app_install_id=excluded.app_install_id,ftp_id=excluded.ftp_id,parent_website_id=excluded.parent_website_id,user=excluded.user,"group"=excluded."group",db_type=excluded.db_type,db_id=excluded.db_id,favorite=excluded.favorite,stream_ports=excluded.stream_ports,udp=excluded.udp,updated_at=excluded.updated_at`
				args = append(args, []byte{})
			}
			_, err = tx.Exec(statement, args...)
			if err != nil {
				return err
			}
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		return nil
	}
	if key == "website-domains" {
		domains, ok := value.(map[uint][]model.WebsiteDomain)
		if !ok {
			return errors.New("网站域名数据类型无效")
		}
		legacyPayload, err := hasColumn(s.db, "website_domains", "payload")
		if err != nil {
			return fmt.Errorf("读取网站域名表结构失败: %w", err)
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				_ = tx.Rollback()
			}
		}()
		for websiteID, items := range domains {
			for index, item := range items {
				var id any = nil
				if parsed, e := strconv.ParseInt(item.ID, 10, 64); e == nil && parsed > 0 {
					id = parsed
				}
				now := formatTime(time.Now())
				if id == nil {
					statement := `INSERT INTO website_domains(website_id,domain,port,ssl,created_at,updated_at) VALUES(?,?,?,?,?,?)`
					args := []any{websiteID, item.Domain, item.Port, boolInt(item.SSL), now, now}
					var generatedID int64
					if legacyPayload {
						// 旧表使用 TEXT PRIMARY KEY，不能依赖 SQLite rowid 回填主键。
						if err := tx.QueryRow(`SELECT COALESCE(MAX(CAST(id AS INTEGER)),0)+1 FROM website_domains`).Scan(&generatedID); err != nil {
							return err
						}
						statement = `INSERT INTO website_domains(id,website_id,domain,port,ssl,created_at,updated_at,payload) VALUES(?,?,?,?,?,?,?,?)`
						args = []any{strconv.FormatInt(generatedID, 10), websiteID, item.Domain, item.Port, boolInt(item.SSL), now, now, []byte{}}
					}
					result, execErr := tx.Exec(statement, args...)
					if execErr != nil {
						return execErr
					}
					if !legacyPayload {
						var idErr error
						generatedID, idErr = result.LastInsertId()
						if idErr != nil {
							return idErr
						}
					}
					items[index].ID = strconv.FormatInt(generatedID, 10)
					domains[websiteID] = items
				} else {
					statement := `INSERT INTO website_domains(id,website_id,domain,port,ssl,created_at,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET website_id=excluded.website_id,domain=excluded.domain,port=excluded.port,ssl=excluded.ssl,updated_at=excluded.updated_at`
					args := []any{id, websiteID, item.Domain, item.Port, boolInt(item.SSL), now, now}
					if legacyPayload {
						statement = `INSERT INTO website_domains(id,website_id,domain,port,ssl,created_at,updated_at,payload) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET website_id=excluded.website_id,domain=excluded.domain,port=excluded.port,ssl=excluded.ssl,updated_at=excluded.updated_at`
						args = append(args, []byte{})
					}
					if _, err = tx.Exec(statement, args...); err != nil {
						return err
					}
				}
			}
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		return nil
	}
	if key == "website-configs" {
		return nil
	}
	if key == "waf-global" {
		cfg, ok := value.(model.WAFGlobalConfig)
		if !ok {
			return errors.New("WAF 全局数据类型无效")
		}
		_, err := s.db.Exec(`INSERT INTO website_waf_global(id,enabled,standard_rules,mode,paranoia_level,inbound_threshold,request_body_limit,updated_at) VALUES(1,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET enabled=excluded.enabled,standard_rules=excluded.standard_rules,mode=excluded.mode,paranoia_level=excluded.paranoia_level,inbound_threshold=excluded.inbound_threshold,request_body_limit=excluded.request_body_limit,updated_at=excluded.updated_at`, boolInt(cfg.Enabled), boolInt(cfg.StandardRules), cfg.Mode, cfg.ParanoiaLevel, cfg.InboundThreshold, cfg.RequestBodyLimit, formatTime(time.Now()))
		return err
	}
	if key == "waf-access-lists" {
		lists, ok := value.(model.WAFAccessLists)
		if !ok {
			return errors.New("WAF 访问列表数据类型无效")
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				_ = tx.Rollback()
			}
		}()
		if _, err = tx.Exec(`DELETE FROM website_waf_access_entries`); err != nil {
			return err
		}
		for kind, items := range map[string][]string{"whitelist": lists.Whitelist, "blacklist": lists.Blacklist} {
			for i, item := range items {
				if _, err = tx.Exec(`INSERT INTO website_waf_access_entries(kind,value,position,updated_at) VALUES(?,?,?,?)`, kind, item, i, formatTime(time.Now())); err != nil {
					return err
				}
			}
		}
		return tx.Commit()
	}
	if key == "openresty" {
		cfg, ok := value.(model.OpenRestyConfig)
		if !ok {
			return errors.New("OpenResty 数据类型无效")
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				_ = tx.Rollback()
			}
		}()
		_, err = tx.Exec(`INSERT INTO website_openresty_config(id,version,enabled,default_https,ssl_reject_handshake,config_content,updated_at) VALUES(1,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET version=excluded.version,enabled=excluded.enabled,default_https=excluded.default_https,ssl_reject_handshake=excluded.ssl_reject_handshake,config_content=excluded.config_content,updated_at=excluded.updated_at`, cfg.Version, boolInt(cfg.Enabled), boolInt(cfg.DefaultHTTPS), boolInt(cfg.SSLRejectHandshake), cfg.ConfigContent, formatTime(cfg.UpdatedAt))
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM website_openresty_modules`); err != nil {
			return err
		}
		for _, m := range cfg.Modules {
			if _, err = tx.Exec(`INSERT INTO website_openresty_modules(name,enabled,build_mode,load_order) VALUES(?,?,?,?)`, m.Name, boolInt(m.Enabled), m.BuildMode, m.LoadOrder); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
	return errors.New("不支持的站点状态类型")
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// List 返回网站列表；limit 始终有界，避免管理端请求消耗无界内存。
func (s *WebsiteService) List(name string, offset, limit int) []model.Website {
	s.mu.RLock()
	defer s.mu.RUnlock()
	name = strings.ToLower(strings.TrimSpace(name))
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	result := make([]model.Website, 0, len(s.websites))
	for _, site := range s.websites {
		if name != "" && !strings.Contains(strings.ToLower(site.PrimaryDomain+" "+site.Alias), name) {
			continue
		}
		item := s.decorateWebsite(site)
		item.Domains = append([]model.WebsiteDomain(nil), s.domains[site.ID]...)
		result = append(result, item)
	}
	if offset >= len(result) {
		return []model.Website{}
	}
	end := offset + limit
	if end > len(result) {
		end = len(result)
	}
	return append([]model.Website(nil), result[offset:end]...)
}

// Count 返回匹配站点总数，用于分页响应的 total 字段。
func (s *WebsiteService) Count(name string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	name = strings.ToLower(strings.TrimSpace(name))
	count := 0
	for _, site := range s.websites {
		if name == "" || strings.Contains(strings.ToLower(site.PrimaryDomain+" "+site.Alias), name) {
			count++
		}
	}
	return count
}

// Get 根据 ID 查询网站。
func (s *WebsiteService) Get(id uint) (model.Website, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, site := range s.websites {
		if site.ID == id {
			site.Domains = append([]model.WebsiteDomain(nil), s.domains[id]...)
			return s.decorateWebsite(site), nil
		}
	}
	return model.Website{}, os.ErrNotExist
}

func (s *WebsiteService) decorateWebsite(site model.Website) model.Website {
	base := strings.TrimSpace(site.SiteDir)
	if base == "" {
		base = s.siteDirPath(site.PrimaryDomain)
	}
	site.SiteDir = filepath.Clean(base)
	site.SitePath = site.SiteDir
	site.Root = filepath.Join(site.SiteDir, "app")
	if cfg, ok := s.configs[site.ID]["dir"].(map[string]any); ok {
		if rel, ok := cfg["dir"].(string); ok {
			candidate := filepath.Join(site.SiteDir, "app", filepath.FromSlash(rel))
			if withinPath(filepath.Join(site.SiteDir, "app"), candidate) {
				site.Root = candidate
			}
		}
	}
	if site.WebsiteSSLID != 0 && s.db != nil {
		var raw string
		if s.db.QueryRow(`SELECT expire_date FROM website_ssls WHERE id=?`, site.WebsiteSSLID).Scan(&raw) == nil {
			if value, err := time.Parse(time.RFC3339Nano, raw); err == nil {
				site.SSLExpireDate = &value
			}
		}
	}
	return site
}

// siteRootPath 返回网站目录根；新站点配置与入口文件均按域名隔离。
func (s *WebsiteService) siteRootPath() string {
	if configured := strings.TrimSpace(os.Getenv("WORKMESH_WEBSITE_ROOT")); configured != "" {
		return filepath.Clean(configured)
	}
	return "/www/wwwroot"
}

func (s *WebsiteService) siteDirPath(domain string) string {
	return filepath.Join(s.siteRootPath(), domain)
}

// SitePath 返回站点域名目录下的标准路径，供 API 和文件操作统一使用。
func (s *WebsiteService) SitePath(site model.Website, kind string) string {
	base := strings.TrimSpace(site.SiteDir)
	if base == "" {
		base = s.siteDirPath(site.PrimaryDomain)
	}
	switch kind {
	case "root", "site":
		return base
	case "app":
		return filepath.Join(base, "app")
	case "nginx":
		return filepath.Join(base, "nginx")
	case "site.conf":
		return filepath.Join(base, "nginx", "site.conf")
	case "stream.conf":
		return filepath.Join(base, "nginx", "stream.conf")
	case "rewrite":
		return filepath.Join(base, "nginx", "rewrite", site.PrimaryDomain+".conf")
	case "waf":
		return filepath.Join(base, "waf")
	case "runtime":
		return filepath.Join(base, ".workmesh", "runtime")
	case "cache":
		return filepath.Join(base, ".workmesh", "cache")
	case "ssl":
		return filepath.Join(base, "ssl")
	case "logs":
		return filepath.Join(base, "logs")
	case "proxy":
		return filepath.Join(base, "nginx", "proxy")
	case "redirect":
		return filepath.Join(base, "nginx", "redirect")
	case "auth_basic":
		return filepath.Join(base, "nginx", "auth_basic")
	case "path_auth":
		return filepath.Join(base, "nginx", "path_auth")
	case "upstream":
		return filepath.Join(base, "nginx", "upstream")
	case "cors":
		return filepath.Join(base, "nginx", "cors.conf")
	}
	return base
}

func (s *WebsiteService) ensureSiteLayout(site model.Website) error {
	for _, kind := range []string{"app", "nginx", "proxy", "redirect", "auth_basic", "path_auth", "upstream", "rewrite", "waf", "logs", "ssl", "runtime", "cache"} {
		p := s.SitePath(site, kind)
		if kind == "rewrite" || kind == "cors" {
			p = filepath.Dir(p)
		}
		if err := os.MkdirAll(p, 0o755); err != nil {
			return err
		}
	}
	// 旧版本部分调用方仍访问 config/basic，保留空兼容目录但不在其中保存新配置。
	if err := os.MkdirAll(filepath.Join(s.SitePath(site, "site"), "config", "basic"), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(s.SitePath(site, "site.conf")); errors.Is(err, os.ErrNotExist) {
		content := fmt.Sprintf("server {\n    server_name %s;\n    root %s;\n    access_log %s;\n    error_log %s;\n    index index.html index.htm;\n    error_page 404 /404.html;\n}\n", site.PrimaryDomain, s.SitePath(site, "app"), s.websiteLogPath(site, "access.log"), s.websiteLogPath(site, "error.log"))
		if err := os.WriteFile(s.SitePath(site, "site.conf"), []byte(content), 0o644); err != nil {
			return err
		}
	}
	for name, content := range map[string]string{"index.html": defaultWebsiteIndexHTML, "404.html": defaultWebsite404HTML} {
		path := filepath.Join(s.SitePath(site, "app"), name)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	if _, err := os.Stat(s.SitePath(site, "stream.conf")); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(s.SitePath(site, "stream.conf"), nil, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Create 创建网站并初始化对应 WAF 配置。
func (s *WebsiteService) Create(req model.WebsiteCreateRequest) (model.Website, error) {
	domain := strings.TrimSpace(req.PrimaryDomain)
	if domain == "" && len(req.Domains) > 0 {
		domain = strings.TrimSpace(req.Domains[0].Domain)
	}
	if domain == "" {
		domain = strings.TrimSpace(req.Name)
	}
	if !domainPattern.MatchString(domain) || strings.Contains(domain, "..") {
		return model.Website{}, errors.New("主域名格式无效")
	}
	if strings.ContainsRune(req.SiteDir, 0) || strings.Contains(filepath.Clean(req.SiteDir), "..") || len(req.SiteDir) > 4096 {
		return model.Website{}, errors.New("站点目录无效")
	}
	// 已能探测到 OpenResty 时，创建站点前必须通过真实配置语法检查；未安装时由预检接口返回明确状态。
	if status := s.ProbeOpenResty(context.Background()); status.Available && !status.ConfigValid && !strings.Contains(status.Error, "无法进入") && !strings.Contains(status.Error, "无权限") {
		return model.Website{}, fmt.Errorf("OpenResty 配置语法检查失败: %s", status.Error)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, site := range s.websites {
		if strings.EqualFold(site.PrimaryDomain, domain) {
			return model.Website{}, errors.New("主域名已存在")
		}
	}
	var id uint = 1
	for _, site := range s.websites {
		if site.ID >= id {
			id = site.ID + 1
		}
	}
	now := time.Now().UTC()
	protocol := strings.ToUpper(strings.TrimSpace(req.Protocol))
	if protocol == "" {
		protocol = "HTTP"
	}
	if protocol != "HTTP" && protocol != "HTTPS" {
		return model.Website{}, errors.New("协议类型无效")
	}
	sslID := req.WebsiteSSLID
	if sslID == 0 {
		sslID = req.SSLID
	}
	errorLog, accessLog := true, true
	if req.ErrorLog != nil {
		errorLog = *req.ErrorLog
	}
	if req.AccessLog != nil {
		accessLog = *req.AccessLog
	}
	typeName := strings.ToLower(strings.TrimSpace(req.Type))
	if typeName == "" {
		typeName = strings.ToLower(strings.TrimSpace(req.AppType))
	}
	if typeName == "" {
		typeName = "static"
	}
	runtimeID := strings.TrimSpace(req.RuntimeID)
	if typeName != "runtime" {
		runtimeID = ""
	} else if runtimeID == "" {
		return model.Website{}, errors.New("运行时站点必须选择运行时")
	} else if s.db != nil {
		var exists int
		if err := s.db.QueryRow(`SELECT 1 FROM runtime_records WHERE id=? LIMIT 1`, runtimeID).Scan(&exists); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return model.Website{}, errors.New("运行时不存在")
		} else if err == nil && exists != 1 {
			return model.Website{}, errors.New("运行时不存在")
		}
	}
	site := model.Website{ID: id, PrimaryDomain: domain, Alias: strings.TrimSpace(req.Alias), Type: typeName, Remark: strings.TrimSpace(req.Remark), SiteDir: strings.TrimSpace(req.SiteDir), Status: "running", Protocol: protocol, HttpConfig: strings.TrimSpace(req.HttpConfig), Proxy: strings.TrimSpace(req.Proxy), ProxyType: strings.TrimSpace(req.ProxyType), ErrorLog: errorLog, AccessLog: accessLog, DefaultServer: req.DefaultServer, IPV6: req.IPV6, Rewrite: strings.TrimSpace(req.Rewrite), WebsiteSSLID: sslID, RuntimeID: runtimeID, AppInstallID: req.AppInstallID, FtpID: req.FtpID, ParentWebsiteID: req.ParentWebsiteID, User: strings.TrimSpace(req.User), Group: strings.TrimSpace(req.Group), DbType: strings.TrimSpace(req.DbType), DbID: req.DbID, StreamPorts: strings.TrimSpace(req.StreamPorts), UDP: req.UDP, WebsiteGroupID: req.WebsiteGroupID, ExpireDate: time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC), CreatedAt: now, UpdatedAt: now}
	if site.Alias == "" {
		site.Alias = domain
	}
	if strings.TrimSpace(req.SiteDir) != "" {
		// 新站点仍固定在域名目录，SiteDir 只接受兼容请求，不允许逃逸站点根。
		if filepath.IsAbs(req.SiteDir) || strings.Contains(filepath.ToSlash(req.SiteDir), "..") {
			return model.Website{}, errors.New("站点目录无效")
		}
	}
	site.SiteDir = s.siteDirPath(domain)
	if site.User == "" {
		site.User = "www"
	}
	if site.Group == "" {
		site.Group = "www"
	}
	if err := s.ensureSiteLayout(site); err != nil {
		return model.Website{}, fmt.Errorf("创建站点目录失败: %w", err)
	}
	if err := s.applyWebsiteDefaults(site); err != nil {
		return model.Website{}, fmt.Errorf("设置站点默认权限失败: %w", err)
	}
	s.websites = append(s.websites, site)
	domains := append([]model.WebsiteDomain(nil), req.Domains...)
	primaryFound := false
	for i := range domains {
		domains[i].Domain = strings.TrimSpace(domains[i].Domain)
		domains[i].WebsiteID = id
		if domains[i].Port == 0 {
			if protocol == "HTTPS" {
				domains[i].Port, domains[i].SSL = 443, true
			} else {
				domains[i].Port = 80
			}
		}
		if strings.EqualFold(domains[i].Domain, domain) {
			primaryFound = true
		}
	}
	// 原版创建网站会同时写入主域名记录，域名设置页直接读取该表。
	if !strings.EqualFold(site.Type, "stream") && !primaryFound {
		primary := model.WebsiteDomain{WebsiteID: id, Domain: domain, Port: 80}
		if protocol == "HTTPS" {
			primary.Port, primary.SSL = 443, true
		}
		domains = append([]model.WebsiteDomain{primary}, domains...)
	}
	s.domains[id] = domains
	s.wafSites[id] = model.WAFSite{WebsiteID: id, Alias: site.Alias, Enabled: true, Mode: "observe", Rules: []model.WAFRule{}}
	if err := s.persist("websites", s.websites); err != nil {
		return model.Website{}, err
	}
	if err := s.persistWAFSites(); err != nil {
		return model.Website{}, err
	}
	if err := s.persist("website-domains", s.domains); err != nil {
		return model.Website{}, err
	}
	site.Domains = append([]model.WebsiteDomain(nil), s.domains[id]...)
	return s.decorateWebsite(site), nil
}

// OperateCrossSiteAccess 根据原系统约定创建或删除站点根目录 .user.ini。
func (s *WebsiteService) OperateCrossSiteAccess(id uint, operation string) error {
	if operation != "Enable" && operation != "Disable" && operation != "enable" && operation != "disable" {
		return errors.New("跨站访问操作无效")
	}
	s.mu.RLock()
	site, err := s.getWebsiteLocked(id)
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	base := strings.TrimSpace(site.SiteDir)
	if base == "" {
		base = s.siteDirPath(site.PrimaryDomain)
	}
	path := filepath.Join(base, ".user.ini")
	if strings.EqualFold(operation, "Enable") {
		if err := os.MkdirAll(base, 0o750); err != nil {
			return err
		}
		return os.WriteFile(path, []byte("open_basedir=\n"), 0o600)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Update 更新网站可编辑字段。
func (s *WebsiteService) Update(req model.WebsiteUpdateRequest) (model.Website, error) {
	if req.ID == 0 {
		return model.Website{}, errors.New("网站 ID 无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.websites {
		if s.websites[i].ID != req.ID {
			continue
		}
		oldDomain := s.websites[i].PrimaryDomain
		if strings.TrimSpace(req.PrimaryDomain) != "" {
			candidate := strings.TrimSpace(req.PrimaryDomain)
			if !domainPattern.MatchString(candidate) || strings.Contains(candidate, "..") {
				return model.Website{}, errors.New("主域名格式无效")
			}
			for j, other := range s.websites {
				if j != i && strings.EqualFold(other.PrimaryDomain, candidate) {
					return model.Website{}, errors.New("主域名已存在")
				}
			}
			s.websites[i].PrimaryDomain = candidate
			if !strings.EqualFold(oldDomain, candidate) {
				oldPath, newPath := s.siteDirPath(oldDomain), s.siteDirPath(candidate)
				if _, statErr := os.Stat(oldPath); statErr == nil {
					if err := os.MkdirAll(filepath.Dir(newPath), 0o750); err != nil {
						return model.Website{}, err
					}
					if err := os.Rename(oldPath, newPath); err != nil {
						return model.Website{}, err
					}
				}
				s.websites[i].SiteDir = newPath
			}
		}
		if strings.TrimSpace(req.Alias) != "" {
			s.websites[i].Alias = strings.TrimSpace(req.Alias)
		}
		if req.Remark != "" {
			s.websites[i].Remark = strings.TrimSpace(req.Remark)
		}
		if req.SiteDir != "" {
			if filepath.IsAbs(req.SiteDir) || strings.Contains(filepath.ToSlash(req.SiteDir), "..") {
				return model.Website{}, errors.New("站点目录无效")
			}
			// siteDir 是兼容字段，真实网站根始终由域名目录规则决定。
			s.websites[i].SiteDir = s.siteDirPath(s.websites[i].PrimaryDomain)
		}
		if req.Type != "" {
			typeName := strings.ToLower(strings.TrimSpace(req.Type))
			s.websites[i].Type = typeName
			if typeName != "runtime" {
				s.websites[i].RuntimeID = ""
			}
		}
		s.websites[i].Favorite = req.Favorite
		if req.WebsiteGroupID != 0 {
			s.websites[i].WebsiteGroupID = req.WebsiteGroupID
		}
		if req.ExpireDate != nil {
			s.websites[i].ExpireDate = req.ExpireDate.UTC()
		}
		if req.IPV6 != nil {
			s.websites[i].IPV6 = *req.IPV6
		}
		if req.WebsiteSSLID != nil {
			s.websites[i].WebsiteSSLID = *req.WebsiteSSLID
		}
		if req.Protocol != "" {
			p := strings.ToUpper(strings.TrimSpace(req.Protocol))
			if p != "HTTP" && p != "HTTPS" {
				return model.Website{}, errors.New("协议类型无效")
			}
			s.websites[i].Protocol = p
		}
		if req.HttpConfig != "" {
			s.websites[i].HttpConfig = strings.TrimSpace(req.HttpConfig)
		}
		if req.Proxy != "" {
			s.websites[i].Proxy = strings.TrimSpace(req.Proxy)
		}
		if req.ProxyType != "" {
			s.websites[i].ProxyType = strings.TrimSpace(req.ProxyType)
		}
		if req.ErrorLog != nil {
			s.websites[i].ErrorLog = *req.ErrorLog
		}
		if req.AccessLog != nil {
			s.websites[i].AccessLog = *req.AccessLog
		}
		if req.DefaultServer != nil {
			s.websites[i].DefaultServer = *req.DefaultServer
		}
		if req.Rewrite != "" {
			s.websites[i].Rewrite = strings.TrimSpace(req.Rewrite)
		}
		if req.RuntimeID != nil {
			if strings.EqualFold(s.websites[i].Type, "runtime") {
				s.websites[i].RuntimeID = strings.TrimSpace(*req.RuntimeID)
			} else {
				s.websites[i].RuntimeID = ""
			}
		}
		if req.AppInstallID != nil {
			s.websites[i].AppInstallID = *req.AppInstallID
		}
		if req.FtpID != nil {
			s.websites[i].FtpID = *req.FtpID
		}
		if req.ParentWebsiteID != nil {
			s.websites[i].ParentWebsiteID = *req.ParentWebsiteID
		}
		if req.User != "" {
			s.websites[i].User = strings.TrimSpace(req.User)
		}
		if req.Group != "" {
			s.websites[i].Group = strings.TrimSpace(req.Group)
		}
		if req.DbType != "" {
			s.websites[i].DbType = strings.TrimSpace(req.DbType)
		}
		if req.DbID != nil {
			s.websites[i].DbID = *req.DbID
		}
		if req.StreamPorts != "" {
			s.websites[i].StreamPorts = strings.TrimSpace(req.StreamPorts)
		}
		if req.UDP != nil {
			s.websites[i].UDP = *req.UDP
		}
		s.websites[i].UpdatedAt = time.Now().UTC()
		if site, ok := s.wafSites[req.ID]; ok {
			site.Alias = s.websites[i].Alias
			s.wafSites[req.ID] = site
		}
		if err := s.persist("websites", s.websites); err != nil {
			return model.Website{}, err
		}
		_ = s.persistWAFSites()
		s.websites[i].Domains = append([]model.WebsiteDomain(nil), s.domains[req.ID]...)
		return s.decorateWebsite(s.websites[i]), nil
	}
	return model.Website{}, os.ErrNotExist
}

// Delete 删除网站及其 WAF 规则。
func (s *WebsiteService) Delete(id uint) error {
	if id == 0 {
		return errors.New("网站 ID 无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, site := range s.websites {
		if site.ID != id {
			continue
		}
		s.websites = append(s.websites[:i], s.websites[i+1:]...)
		if s.db != nil {
			if _, err := s.db.Exec(`DELETE FROM websites WHERE id=?`, id); err != nil {
				return err
			}
		}
		delete(s.wafSites, id)
		delete(s.domains, id)
		delete(s.configs, id)
		_ = os.RemoveAll(s.SitePath(site, "waf"))
		if err := s.persist("websites", s.websites); err != nil {
			return err
		}
		if err := s.persistWAFSites(); err != nil {
			return err
		}
		if err := s.persist("website-domains", s.domains); err != nil {
			return err
		}
		return s.persist("website-configs", s.configs)
	}
	return os.ErrNotExist
}

// Operate 更新网站运行状态，支持启动、停止和重启。
func (s *WebsiteService) Operate(id uint, operation string) (model.Website, error) {
	operation = strings.ToLower(strings.TrimSpace(operation))
	if operation != "start" && operation != "stop" && operation != "restart" {
		return model.Website{}, errors.New("网站操作必须是 start、stop 或 restart")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.websites {
		if s.websites[i].ID != id {
			continue
		}
		if operation == "stop" {
			s.websites[i].Status = "stopped"
		} else {
			s.websites[i].Status = "running"
		}
		s.websites[i].UpdatedAt = time.Now().UTC()
		if err := s.persist("websites", s.websites); err != nil {
			return model.Website{}, err
		}
		return s.websites[i], nil
	}
	return model.Website{}, os.ErrNotExist
}

// UpdateHTTPS 保存站点 HTTPS 开关及证书关联，并同步站点协议字段。
func (s *WebsiteService) UpdateHTTPS(id uint, enabled bool, sslID uint, httpConfig string) (model.Website, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.websites {
		if s.websites[i].ID != id {
			continue
		}
		if enabled {
			s.websites[i].Protocol = "HTTPS"
		} else if s.websites[i].Protocol == "HTTPS" {
			s.websites[i].Protocol = "HTTP"
		}
		if sslID != 0 {
			s.websites[i].WebsiteSSLID = sslID
		}
		if strings.TrimSpace(httpConfig) != "" {
			s.websites[i].HttpConfig = strings.TrimSpace(httpConfig)
		}
		s.websites[i].UpdatedAt = time.Now().UTC()
		if err := s.persist("websites", s.websites); err != nil {
			return model.Website{}, err
		}
		return s.websites[i], nil
	}
	return model.Website{}, os.ErrNotExist
}

// SetGroups 批量更新网站分组并一次持久化。
func (s *WebsiteService) SetGroups(ids []uint, groupID uint) error {
	if len(ids) == 0 || groupID == 0 {
		return errors.New("网站分组参数无效")
	}
	wanted := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	updated := 0
	for i := range s.websites {
		if _, ok := wanted[s.websites[i].ID]; ok {
			s.websites[i].WebsiteGroupID = groupID
			s.websites[i].UpdatedAt = time.Now().UTC()
			updated++
		}
	}
	if updated != len(wanted) {
		return errors.New("部分网站不存在")
	}
	return s.persist("websites", s.websites)
}

// GroupInUse 判断分组是否仍被网站引用。
func (s *WebsiteService) GroupInUse(groupID uint) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, website := range s.websites {
		if website.WebsiteGroupID == groupID {
			return true
		}
	}
	return false
}

// WebsiteLog 返回站点 access.log/error.log 的真实内容，按页读取避免一次性加载大文件。
func (s *WebsiteService) WebsiteLog(id uint, logType string, page, pageSize int) (map[string]any, error) {
	if logType != "access.log" && logType != "error.log" {
		return nil, errors.New("日志类型无效")
	}
	s.mu.RLock()
	site, err := s.getWebsiteLocked(id)
	if err != nil {
		s.mu.RUnlock()
		return nil, err
	}
	enabled := site.AccessLog
	if logType == "error.log" {
		enabled = site.ErrorLog
	}
	path := s.websiteLogPath(site, logType)
	if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
		legacy := filepath.Join(s.root, "websites", strconv.FormatUint(uint64(id), 10), "logs", logType)
		if _, legacyErr := os.Stat(legacy); legacyErr == nil {
			path = legacy
		}
	}
	s.mu.RUnlock()
	result := map[string]any{"enable": enabled, "content": "", "end": true, "path": path}
	if !enabled {
		return result, nil
	}
	data, readErr := os.ReadFile(path)
	if errors.Is(readErr, os.ErrNotExist) {
		return result, nil
	}
	if readErr != nil {
		return nil, readErr
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 5000 {
		pageSize = 100
	}
	start := (page - 1) * pageSize
	if start >= len(lines) {
		result["end"] = true
		return result, nil
	}
	end := start + pageSize
	if end > len(lines) {
		end = len(lines)
	}
	result["content"] = strings.Join(lines[start:end], "\n")
	result["end"] = end >= len(lines)
	return result, nil
}

// OperateWebsiteLog 持久化日志开关并执行清理操作；配置文件由站点生成器后续同步。
func (s *WebsiteService) OperateWebsiteLog(id uint, logType, operation string) error {
	if logType != "access.log" && logType != "error.log" {
		return errors.New("日志类型无效")
	}
	if operation != "enable" && operation != "disable" && operation != "delete" {
		return errors.New("日志操作无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.websites {
		if s.websites[i].ID != id {
			continue
		}
		if operation == "delete" {
			if err := os.WriteFile(s.websiteLogPath(s.websites[i], logType), nil, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		}
		enabled := operation == "enable"
		if logType == "access.log" {
			s.websites[i].AccessLog = enabled
		} else {
			s.websites[i].ErrorLog = enabled
		}
		return s.persist("websites", s.websites)
	}
	return os.ErrNotExist
}

func (s *WebsiteService) applyWebsiteDefaults(site model.Website) error {
	uid, gid := mustUID(site.User), mustGID(site.Group)
	if uid < 0 || gid < 0 {
		return nil
	}
	return filepath.Walk(s.SitePath(site, "site"), func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info == nil {
			return nil
		}
		mode := os.FileMode(0o644)
		if info.IsDir() {
			mode = 0o755
		}
		if err := os.Chmod(path, mode); err != nil {
			return err
		}
		if err := os.Chown(path, uid, gid); err != nil && runtime.GOOS != "windows" {
			return err
		}
		return nil
	})
}

func (s *WebsiteService) websiteLogPath(site model.Website, logType string) string {
	return filepath.Join(s.SitePath(site, "logs"), logType)
}

// WebsiteDirConfig 扫描站点 app 目录下最多三级可选运行目录。
func (s *WebsiteService) WebsiteDirConfig(id uint) (map[string]any, error) {
	s.mu.RLock()
	site, err := s.getWebsiteLocked(id)
	s.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	root := s.SitePath(site, "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	runUser, runGroup := site.User, site.Group
	if runUser == "" {
		runUser = "www"
	}
	if runGroup == "" {
		runGroup = "www"
	}
	result := map[string]any{"dirs": []string{"/"}, "user": runUser, "userGroup": runGroup, "msg": ""}
	allowed := func(name string) bool { return name != "node_modules" && name != "vendor" && name != ".git" }
	checkOwnership := true
	if _, e := user.Lookup(runUser); e != nil {
		checkOwnership = false
	}
	if _, e := user.LookupGroup(runGroup); e != nil {
		checkOwnership = false
	}
	var walk func(string, int)
	walk = func(rel string, depth int) {
		if depth >= 3 {
			return
		}
		entries, readErr := os.ReadDir(filepath.Join(root, filepath.FromSlash(rel)))
		if readErr != nil {
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() || !allowed(entry.Name()) {
				continue
			}
			next := filepath.ToSlash(filepath.Join(rel, entry.Name()))
			result["dirs"] = append(result["dirs"].([]string), "/"+strings.TrimPrefix(next, "/"))
			if info, infoErr := entry.Info(); infoErr == nil && checkOwnership {
				if uid, gid, ok := pathOwnership(info); ok && (uid != "www" || gid != "www") {
					result["msg"] = "ErrPathPermission"
				}
			}
			walk(next, depth+1)
		}
	}
	walk("", 0)
	return result, nil
}

func pathOwnership(info os.FileInfo) (string, string, bool) {
	if info == nil {
		return "", "", false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", "", false
	}
	uid, gid := strconv.FormatUint(uint64(stat.Uid), 10), strconv.FormatUint(uint64(stat.Gid), 10)
	if u, e := user.LookupId(uid); e == nil {
		uid = u.Username
	}
	if g, e := user.LookupGroupId(gid); e == nil {
		gid = g.Name
	}
	return uid, gid, true
}

// UpdateWebsiteDir 更改站点 app 根下的相对运行目录，并同步 nginx root 配置字段。
func (s *WebsiteService) UpdateWebsiteDir(id uint, dir string) (map[string]any, error) {
	clean, err := validateWebsiteRelativeDir(dir)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	site, err := s.getWebsiteLocked(id)
	if err != nil {
		return nil, err
	}
	root := s.SitePath(site, "app")
	target := filepath.Join(root, filepath.FromSlash(clean))
	if !withinPath(root, target) {
		return nil, errors.New("站点目录越界")
	}
	if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
		return nil, errors.New("站点目录不存在")
	}
	configPath := s.SitePath(site, "site.conf")
	oldConfig, _ := os.ReadFile(configPath)
	newConfig := regexp.MustCompile(`(?m)^([\t ]*root[\t ]+)[^;]+;`).ReplaceAllString(string(oldConfig), "${1}"+target+";")
	if newConfig == string(oldConfig) {
		newConfig = strings.TrimRight(string(oldConfig), "\n") + "\n    root " + target + ";\n"
	}
	if newConfig != string(oldConfig) {
		if err := os.WriteFile(configPath+".tmp", []byte(newConfig), 0o640); err != nil {
			return nil, err
		}
		if err := os.Rename(configPath+".tmp", configPath); err != nil {
			_ = os.WriteFile(configPath, oldConfig, 0o640)
			return nil, err
		}
	}
	for i := range s.websites {
		if s.websites[i].ID == id {
			s.websites[i].Root = target
			s.websites[i].SitePath = s.SitePath(s.websites[i], "site")
			s.websites[i].UpdatedAt = time.Now().UTC()
		}
	}
	if s.configs[id] == nil {
		s.configs[id] = map[string]any{}
	}
	s.configs[id]["dir"] = map[string]any{"dir": clean}
	if err := s.persist("websites", s.websites); err != nil {
		return nil, err
	}
	if err := s.persist("website-configs", s.configs); err != nil {
		return nil, err
	}
	return map[string]any{"root": target, "siteDir": s.SitePath(site, "site"), "dir": clean}, nil
}

// UpdateWebsiteDirPermission 校验系统用户组后递归更新站点目录权限。
func (s *WebsiteService) UpdateWebsiteDirPermission(id uint, name, group string) error {
	name, group = strings.TrimSpace(name), strings.TrimSpace(group)
	if name == "" {
		name = "www"
	}
	if group == "" {
		group = "www"
	}
	if _, err := user.Lookup(name); err != nil {
		return errors.New("系统用户不存在")
	}
	if _, err := user.LookupGroup(group); err != nil {
		return errors.New("系统用户组不存在")
	}
	s.mu.RLock()
	site, err := s.getWebsiteLocked(id)
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	base := s.SitePath(site, "site")
	return filepath.Walk(base, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info == nil {
			return nil
		}
		mode := info.Mode().Perm()
		if info.IsDir() {
			mode = 0o755
		} else {
			mode = 0o644
		}
		if err := os.Chmod(path, mode); err != nil {
			return err
		}
		if os.Geteuid() == 0 {
			_ = os.Chown(path, mustUID(name), mustGID(group))
		}
		return nil
	})
}

func mustUID(name string) int {
	u, e := user.Lookup(name)
	if e != nil {
		return -1
	}
	n, _ := strconv.Atoi(u.Uid)
	return n
}
func mustGID(name string) int {
	g, e := user.LookupGroup(name)
	if e != nil {
		return -1
	}
	n, _ := strconv.Atoi(g.Gid)
	return n
}
func validateWebsiteRelativeDir(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || value == "/" {
		return "", nil
	}
	// Web 客户端以 /subdir 表示 app 根下的虚拟路径；去掉前导斜杠后再做真实路径校验。
	value = strings.TrimPrefix(value, "/")
	if filepath.IsAbs(value) || strings.Contains(value, "..") || strings.ContainsRune(value, 0) {
		return "", errors.New("站点目录必须是 app 根下相对路径")
	}
	clean := pathCleanSlash(value)
	if clean == "." || strings.HasPrefix(clean, "../") {
		return "", errors.New("站点目录越界")
	}
	return strings.TrimPrefix(clean, "/"), nil
}
func pathCleanSlash(v string) string { return filepath.ToSlash(filepath.Clean(filepath.FromSlash(v))) }
func withinPath(root, target string) bool {
	r, _ := filepath.Abs(root)
	t, _ := filepath.Abs(target)
	return t == r || strings.HasPrefix(t, r+string(filepath.Separator))
}

// ListDomains 返回网站域名，并限制最大返回数量。
func (s *WebsiteService) ListDomains(websiteID uint) ([]model.WebsiteDomain, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.getWebsiteLocked(websiteID); err != nil {
		return nil, err
	}
	return append([]model.WebsiteDomain(nil), s.domains[websiteID]...), nil
}

// UpsertDomain 新增或更新域名记录，并校验端口和域名格式。
func (s *WebsiteService) UpsertDomain(domain model.WebsiteDomain) (model.WebsiteDomain, error) {
	if domain.WebsiteID == 0 || !domainPattern.MatchString(strings.TrimSpace(domain.Domain)) || strings.Contains(domain.Domain, "..") {
		return model.WebsiteDomain{}, errors.New("网站域名无效")
	}
	if domain.Port == 0 {
		domain.Port = 80
	}
	if domain.Port < 1 || domain.Port > 65535 {
		return model.WebsiteDomain{}, errors.New("网站端口超出范围")
	}
	domain.Domain = strings.TrimSpace(domain.Domain)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getWebsiteLocked(domain.WebsiteID); err != nil {
		return model.WebsiteDomain{}, err
	}
	items := s.domains[domain.WebsiteID]
	for _, site := range s.websites {
		if site.ID == domain.WebsiteID && strings.EqualFold(site.PrimaryDomain, domain.Domain) {
			return model.WebsiteDomain{}, errors.New("网站域名已被主域名占用")
		}
		for _, existing := range s.domains[site.ID] {
			if existing.ID != domain.ID && strings.EqualFold(existing.Domain, domain.Domain) {
				return model.WebsiteDomain{}, errors.New("网站域名已存在")
			}
		}
	}
	for i := range items {
		if items[i].ID == domain.ID {
			items[i] = domain
			s.domains[domain.WebsiteID] = items
			return domain, s.persist("website-domains", s.domains)
		}
	}
	s.domains[domain.WebsiteID] = append(items, domain)
	if err := s.persist("website-domains", s.domains); err != nil {
		return model.WebsiteDomain{}, err
	}
	// 新记录由 SQLite 自增主键生成；持久化完成后返回实际 ID，后续编辑和删除使用同一 ID。
	return s.domains[domain.WebsiteID][len(items)], nil
}

// DeleteDomain 删除指定网站域名。
func (s *WebsiteService) DeleteDomain(websiteID uint, domainID string) error {
	if websiteID == 0 || strings.TrimSpace(domainID) == "" {
		return errors.New("网站域名参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.domains[websiteID]
	for i, item := range items {
		if item.ID == domainID {
			if s.db != nil {
				if _, err := s.db.Exec(`DELETE FROM website_domains WHERE id=? AND website_id=?`, domainID, websiteID); err != nil {
					return err
				}
			}
			s.domains[websiteID] = append(items[:i], items[i+1:]...)
			return s.persist("website-domains", s.domains)
		}
	}
	return os.ErrNotExist
}

// GetConfig 读取网站类型配置；未设置时返回空对象而非固定业务数据。
func (s *WebsiteService) GetConfig(websiteID uint, typ string) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.getWebsiteLocked(websiteID); err != nil {
		return nil, err
	}
	result := map[string]any{}
	for key, value := range s.configs[websiteID] {
		result[key] = value
	}
	if typ != "" {
		if value, ok := result[typ]; ok {
			if typed, ok := value.(map[string]any); ok {
				return typed, nil
			}
		}
		return map[string]any{}, nil
	}
	return result, nil
}

// WebsiteConfigFile 返回单站点 nginx/site.conf，供资源页编辑该站点实际配置。
func (s *WebsiteService) WebsiteConfigFile(websiteID uint) (map[string]any, error) {
	site, err := s.Get(websiteID)
	if err != nil {
		return nil, err
	}
	path := s.SitePath(site, "site.conf")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	info, _ := os.Stat(path)
	result := map[string]any{
		"path": path, "name": filepath.Base(path), "content": string(content),
		"isDir": false, "type": "conf", "mimeType": "text/plain", "size": len(content),
	}
	if info != nil {
		result["mode"] = info.Mode().Perm()
		result["modTime"] = info.ModTime().UTC().Format(time.RFC3339Nano)
	}
	return result, nil
}

// WebsiteNginxScopeConfig 从站点域名目录的 Nginx 配置读取指定作用域。
// index 作用域必须返回 1Panel 兼容的 enable/params 结构，不能依赖历史 JSON 状态。
func (s *WebsiteService) WebsiteNginxScopeConfig(websiteID uint, scope string) (map[string]any, error) {
	if strings.TrimSpace(scope) != "index" {
		return nil, errors.New("不支持的 Nginx 配置作用域")
	}
	site, err := s.Get(websiteID)
	if err != nil {
		return nil, err
	}
	content, readErr := os.ReadFile(s.SitePath(site, "site.conf"))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, readErr
	}
	documents := make([]string, 0, 5)
	for _, match := range regexp.MustCompile(`(?m)^\s*index\s+([^;]+);`).FindAllStringSubmatch(string(content), -1) {
		for _, item := range strings.Fields(match[1]) {
			if item != "" && !containsString(documents, item) {
				documents = append(documents, item)
			}
		}
	}
	if len(documents) == 0 {
		documents = []string{"index.html", "index.htm"}
	}
	params := []map[string]any{{"name": "index", "params": documents}}
	return map[string]any{"enable": len(documents) > 0, "params": params}, nil
}

func containsString(items []string, wanted string) bool {
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}

// UpdateWebsiteNginxIndex 更新站点 site.conf 中的 index 指令，并保留原文件以便失败恢复。
func (s *WebsiteService) UpdateWebsiteNginxIndex(websiteID uint, documents []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	site, err := s.getWebsiteLocked(websiteID)
	if err != nil {
		return err
	}
	path := s.SitePath(site, "site.conf")
	old, readErr := os.ReadFile(path)
	if readErr != nil {
		return readErr
	}
	clean := make([]string, 0, len(documents))
	for _, item := range documents {
		item = strings.TrimSpace(item)
		if item != "" && !strings.ContainsAny(item, ";\r\n") {
			clean = append(clean, item)
		}
	}
	if len(clean) == 0 {
		return errors.New("默认文档不能为空")
	}
	line := "index " + strings.Join(clean, " ") + ";"
	updated := regexp.MustCompile(`(?m)^\s*index\s+[^;]+;\s*$`).ReplaceAllString(string(old), line)
	if updated == string(old) && !strings.Contains(string(old), line) {
		updated = strings.TrimRight(string(old), "\n") + "\n" + line + "\n"
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(updated), 0o640); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.WriteFile(path, old, 0o640)
		return err
	}
	return nil
}

// ListWebsiteProxies 读取域名目录 nginx/proxy 下的 .conf/.bak 文件，返回 1Panel 代理配置字段。
func (s *WebsiteService) ListWebsiteProxies(websiteID uint) ([]map[string]any, error) {
	site, err := s.Get(websiteID)
	if err != nil {
		return nil, err
	}
	dir := s.SitePath(site, "proxy")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	proxies := make([]map[string]any, 0)
	for _, entry := range entries {
		if entry.IsDir() || !(strings.HasSuffix(entry.Name(), ".conf") || strings.HasSuffix(entry.Name(), ".bak")) {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		name := strings.TrimSuffix(strings.TrimSuffix(entry.Name(), ".conf"), ".bak")
		proxyPass := firstRegexpValue(`(?m)\bproxy_pass\s+([^;\s]+)`, string(content))
		proxyHost := firstRegexpValue(`(?m)\bproxy_set_header\s+Host\s+([^;\s]+)`, string(content))
		match, modifier := parseProxyLocation(string(content))
		contentText := string(content)
		proxies = append(proxies, map[string]any{
			"id": websiteID, "name": name, "enable": strings.HasSuffix(entry.Name(), ".conf"),
			"proxyPass": proxyPass, "proxyHost": proxyHost, "match": match, "modifier": modifier,
			"content": string(content), "filePath": filepath.Join(dir, entry.Name()),
			"cache": false, "cacheTime": 0, "cacheUnit": "", "serverCacheTime": 0, "serverCacheUnit": "",
			"replaces": map[string]string{}, "sni": strings.Contains(contentText, "proxy_ssl_server_name on"),
			"proxySSLName": firstRegexpValue(`(?m)\bproxy_ssl_name\s+([^;\s]+)`, string(content)),
			"sslVerify":    strings.Contains(contentText, "proxy_ssl_verify on"), "cors": false,
		})
	}
	sort.Slice(proxies, func(i, j int) bool { return fmt.Sprint(proxies[i]["name"]) < fmt.Sprint(proxies[j]["name"]) })
	return proxies, nil
}

func firstRegexpValue(pattern, content string) string {
	match := regexp.MustCompile(pattern).FindStringSubmatch(content)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func parseProxyLocation(content string) (string, string) {
	match := regexp.MustCompile(`(?m)^\s*location\s+([^\s{]+)(?:\s+([^\s{]+))?\s*\{`).FindStringSubmatch(content)
	if len(match) < 2 {
		return "", ""
	}
	if len(match) > 2 && (match[1] == "=" || strings.HasPrefix(match[1], "~")) {
		return strings.TrimSpace(match[2]), strings.TrimSpace(match[1])
	}
	return strings.TrimSpace(match[1]), ""
}

// BasicConfig 返回站点 Basic 页面所需的真实目录、配置和日志状态。
func (s *WebsiteService) BasicConfig(id uint) (map[string]any, error) {
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	contentBytes, configErr := os.ReadFile(s.SitePath(site, "site.conf"))
	content := string(contentBytes)
	if configErr != nil {
		content, configErr = s.OpenRestyFile()
	}
	defaultDocuments := []string{}
	if configured, ok := s.configs[id]["index"].(map[string]any); ok {
		if value, ok := configured["documents"].([]any); ok {
			for _, item := range value {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					defaultDocuments = append(defaultDocuments, strings.TrimSpace(text))
				}
			}
		}
		if value, ok := configured["content"].(string); ok && value != "" {
			defaultDocuments = append(defaultDocuments, strings.Fields(value)...)
		}
	}
	if configErr == nil {
		for _, match := range regexp.MustCompile(`(?m)^\s*index\s+([^;]+);`).FindAllStringSubmatch(content, -1) {
			defaultDocuments = append(defaultDocuments, strings.Fields(match[1])...)
		}
	}
	if len(defaultDocuments) == 0 {
		defaultDocuments = []string{"index.html", "index.htm"}
	}
	accessPath, errorPath := s.websiteLogPath(site, "access.log"), s.websiteLogPath(site, "error.log")
	logStatus := func(path string, enabled bool) map[string]any {
		entry := map[string]any{"path": path, "enabled": enabled, "exists": false}
		if !enabled {
			entry["status"] = "disabled"
			return entry
		}
		if _, statErr := os.Stat(path); statErr == nil {
			entry["exists"], entry["status"] = true, "available"
		} else if errors.Is(statErr, os.ErrNotExist) {
			entry["status"] = "missing"
		} else {
			entry["status"], entry["error"] = "unavailable", statErr.Error()
		}
		return entry
	}
	traffic := map[string]any{"status": "missing", "requests": 0, "bytes": int64(0), "source": accessPath}
	if data, readErr := os.ReadFile(accessPath); readErr == nil {
		requests, bytes := accessLogTraffic(string(data))
		traffic = map[string]any{"status": "available", "requests": requests, "bytes": bytes, "source": accessPath}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		traffic["status"], traffic["error"] = "unavailable", readErr.Error()
	}
	configError := ""
	if configErr != nil {
		configError = configErr.Error()
	}
	return map[string]any{
		"website": site, "id": site.ID, "domains": append([]model.WebsiteDomain(nil), site.Domains...),
		"siteDir": site.SiteDir, "sitePath": site.SitePath, "root": site.Root,
		"defaultDocuments": defaultDocuments, "accessLog": logStatus(accessPath, site.AccessLog), "errorLog": logStatus(errorPath, site.ErrorLog),
		"traffic": traffic, "proxy": site.Proxy, "https": site.Protocol == "HTTPS", "rewrite": site.Rewrite,
		"configPath": s.openRestyConfigPath(), "configError": configError,
	}, nil
}

func accessLogTraffic(content string) (int, int64) {
	requests := 0
	var bytes int64
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		requests++
		if len(fields) >= 10 {
			if value, err := strconv.ParseInt(fields[9], 10, 64); err == nil && value > 0 {
				bytes += value
			}
		}
	}
	return requests, bytes
}

// UpdateConfig 保存网站类型配置，配置键由调用方明确指定。
func (s *WebsiteService) UpdateConfig(websiteID uint, typ string, value map[string]any) (map[string]any, error) {
	if websiteID == 0 || strings.TrimSpace(typ) == "" || len(typ) > 64 {
		return nil, errors.New("网站配置参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getWebsiteLocked(websiteID); err != nil {
		return nil, err
	}
	website, _ := s.getWebsiteLocked(websiteID)
	var fileTarget string
	var oldFile []byte
	if s.configs[websiteID] == nil {
		s.configs[websiteID] = map[string]any{}
	}
	s.configs[websiteID][typ] = value
	if content, ok := value["content"].(string); ok {
		var target string
		switch typ {
		case "nginx":
			target = s.SitePath(website, "site.conf")
		case "stream":
			target = s.SitePath(website, "stream.conf")
		case "rewrite":
			target = s.SitePath(website, "rewrite")
		}
		if target != "" {
			fileTarget = target
			oldFile, _ = os.ReadFile(target)
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return nil, err
			}
			old, _ := os.ReadFile(target)
			tmp := target + ".tmp"
			if err := os.WriteFile(tmp, []byte(content), 0o640); err != nil {
				return nil, err
			}
			if err := os.Rename(tmp, target); err != nil {
				_ = os.WriteFile(target, old, 0o640)
				return nil, err
			}
		}
		if typ == "rewrite-custom" {
			dir := strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_REWRITE_DIR"))
			if dir == "" {
				dir = filepath.Join(s.root, "openresty", "rewrite")
			}
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return nil, err
			}
			name := website.Alias
			if name == "" {
				name = website.PrimaryDomain
			}
			name = regexp.MustCompile(`[^A-Za-z0-9_.-]+`).ReplaceAllString(name, "_")
			if err := os.WriteFile(filepath.Join(dir, name+".conf"), []byte(content), 0o640); err != nil {
				return nil, err
			}
		}
	}
	if s.db != nil {
		content, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		if _, err := s.db.Exec(`INSERT INTO website_config_values(website_id,config_type,content,updated_at) VALUES(?,?,?,?) ON CONFLICT(website_id,config_type) DO UPDATE SET content=excluded.content,updated_at=excluded.updated_at`, websiteID, typ, string(content), formatTime(time.Now())); err != nil {
			if fileTarget != "" {
				_ = os.WriteFile(fileTarget, oldFile, 0o640)
			}
			return nil, err
		}
	}
	if err := s.persist("website-configs", s.configs); err != nil {
		if fileTarget != "" {
			_ = os.WriteFile(fileTarget, oldFile, 0o640)
		}
		return nil, err
	}
	return value, nil
}

// ListCustomRewrites 返回已实际保存的站点级自定义 rewrite 资源，不生成固定演示条目。
func (s *WebsiteService) ListCustomRewrites() []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]map[string]any, 0)
	for _, site := range s.websites {
		value, ok := s.configs[site.ID]["rewrite-custom"]
		if !ok {
			continue
		}
		config, ok := value.(map[string]any)
		if !ok {
			continue
		}
		items = append(items, map[string]any{"websiteID": site.ID, "name": site.Alias, "domain": site.PrimaryDomain, "content": config["content"]})
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["websiteID"].(uint) < items[j]["websiteID"].(uint) })
	return items
}

// GetRewrite 返回站点实际 rewrite 文件或内置规则内容。
func (s *WebsiteService) GetRewrite(id uint, name string) (map[string]any, error) {
	s.mu.RLock()
	site, err := s.getWebsiteLocked(id)
	s.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	content := ""
	switch name {
	case "current":
		data, readErr := os.ReadFile(s.SitePath(site, "rewrite"))
		if readErr == nil {
			content = string(data)
		}
	case "default":
		content = "location / { try_files $uri $uri/ =404; }"
	case "wordpress", "wp2":
		content = "location / { try_files $uri $uri/ /index.php?$args; }"
	case "thinkphp", "laravel5", "yii2":
		content = "location / { try_files $uri $uri/ /index.php?$query_string; }"
	default:
		if cfg, e := s.GetConfig(id, "rewrite-custom"); e == nil {
			content, _ = cfg["content"].(string)
		}
	}
	return map[string]any{"content": content}, nil
}

// UpdateRewrite 写入域名目录 rewrite 文件，并在失败时恢复原文件。
func (s *WebsiteService) UpdateRewrite(id uint, name, content string) error {
	if len(content) > 64<<10 || strings.IndexByte(content, 0) >= 0 {
		return errors.New("rewrite 内容无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	site, err := s.getWebsiteLocked(id)
	if err != nil {
		return err
	}
	target := s.SitePath(site, "rewrite")
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	old, _ := os.ReadFile(target)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o640); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.WriteFile(target, old, 0o640)
		return err
	}
	nginxPath := s.SitePath(site, "site.conf")
	nginxOld, _ := os.ReadFile(nginxPath)
	includeLine := "    include " + target + ";"
	if !strings.Contains(string(nginxOld), includeLine) {
		nginxNew := strings.TrimRight(string(nginxOld), "\n") + "\n" + includeLine + "\n"
		if err := os.WriteFile(nginxPath+".tmp", []byte(nginxNew), 0o640); err != nil {
			_ = os.WriteFile(target, old, 0o640)
			return err
		}
		if err := os.Rename(nginxPath+".tmp", nginxPath); err != nil {
			_ = os.WriteFile(target, old, 0o640)
			return err
		}
	}
	for i := range s.websites {
		if s.websites[i].ID == id {
			s.websites[i].Rewrite = name
			s.websites[i].UpdatedAt = time.Now().UTC()
		}
	}
	if err := s.persist("websites", s.websites); err != nil {
		_ = os.WriteFile(target, old, 0o640)
		_ = os.WriteFile(nginxPath, nginxOld, 0o640)
		return err
	}
	return nil
}

func (s *WebsiteService) persistWAFSites() error {
	for _, site := range s.wafSites {
		base := s.SitePath(model.Website{ID: site.WebsiteID, PrimaryDomain: s.domainForWebsite(site.WebsiteID), SiteDir: ""}, "waf")
		if err := os.MkdirAll(base, 0o750); err != nil {
			return err
		}
		data, err := json.Marshal(site)
		if err != nil {
			return err
		}
		tmp := filepath.Join(base, ".site.json.tmp")
		if err := os.WriteFile(tmp, data, 0o640); err != nil {
			return err
		}
		if err := os.Rename(tmp, filepath.Join(base, "site.json")); err != nil {
			return err
		}
	}
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				_ = tx.Rollback()
			}
		}()
		for _, site := range s.wafSites {
			_, err = tx.Exec(`INSERT INTO website_waf_sites(website_id,enabled,mode,updated_at) VALUES(?,?,?,?) ON CONFLICT(website_id) DO UPDATE SET enabled=excluded.enabled,mode=excluded.mode,updated_at=excluded.updated_at`, site.WebsiteID, boolInt(site.Enabled), site.Mode, formatTime(time.Now()))
			if err != nil {
				return err
			}
			_, err = tx.Exec(`DELETE FROM website_waf_rules WHERE website_id=?`, site.WebsiteID)
			if err != nil {
				return err
			}
			for _, rule := range site.Rules {
				var id any = nil
				if parsed, e := strconv.ParseInt(rule.ID, 10, 64); e == nil {
					id = parsed
				}
				if id == nil {
					_, err = tx.Exec(`INSERT INTO website_waf_rules(website_id,name,location,rule_key,operator,value,action,priority,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, site.WebsiteID, rule.Name, rule.Location, rule.Key, rule.Operator, rule.Value, rule.Action, rule.Priority, boolInt(rule.Enabled), formatTime(time.Now()), formatTime(time.Now()))
				} else {
					_, err = tx.Exec(`INSERT INTO website_waf_rules(id,website_id,name,location,rule_key,operator,value,action,priority,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, site.WebsiteID, rule.Name, rule.Location, rule.Key, rule.Operator, rule.Value, rule.Action, rule.Priority, boolInt(rule.Enabled), formatTime(time.Now()), formatTime(time.Now()))
				}
				if err != nil {
					return err
				}
			}
		}
		return tx.Commit()
	}
	items := make([]model.WAFSite, 0, len(s.wafSites))
	for _, site := range s.wafSites {
		items = append(items, site)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].WebsiteID < items[j].WebsiteID })
	return s.persist("waf-sites", items)
}

func (s *WebsiteService) domainForWebsite(id uint) string {
	for _, site := range s.websites {
		if site.ID == id {
			return site.PrimaryDomain
		}
	}
	return strconv.FormatUint(uint64(id), 10)
}

// ListWAFSites 返回所有网站 WAF 配置；已存在网站但尚无配置时自动补齐默认项。
func (s *WebsiteService) ListWAFSites() []model.WAFSite {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, site := range s.websites {
		if _, ok := s.wafSites[site.ID]; !ok {
			s.wafSites[site.ID] = model.WAFSite{WebsiteID: site.ID, Alias: site.Alias, Enabled: true, Mode: "observe", Rules: []model.WAFRule{}}
		}
	}
	result := make([]model.WAFSite, 0, len(s.wafSites))
	for _, site := range s.wafSites {
		result = append(result, site)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].WebsiteID < result[j].WebsiteID })
	return result
}

// UpdateWAFSite 更新网站 WAF 开关和模式。
func (s *WebsiteService) UpdateWAFSite(id uint, enabled bool, mode string) (model.WAFSite, error) {
	if id == 0 || (mode != "observe" && mode != "block") {
		return model.WAFSite{}, errors.New("网站 WAF 参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	website, err := s.getWebsiteLocked(id)
	if err != nil {
		return model.WAFSite{}, err
	}
	site := s.wafSites[id]
	site.WebsiteID, site.Enabled, site.Mode = id, enabled, mode
	if site.Alias == "" {
		site.Alias = website.Alias
	}
	if site.Rules == nil {
		site.Rules = []model.WAFRule{}
	}
	s.wafSites[id] = site
	return site, s.persistWAFSites()
}

func (s *WebsiteService) getWebsiteLocked(id uint) (model.Website, error) {
	for _, site := range s.websites {
		if site.ID == id {
			return site, nil
		}
	}
	return model.Website{}, os.ErrNotExist
}

// ListRules 返回按优先级排序的规则。
func (s *WebsiteService) ListRules(id uint) ([]model.WAFRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.getWebsiteLocked(id); err != nil {
		return nil, err
	}
	rules := append([]model.WAFRule(nil), s.wafSites[id].Rules...)
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Priority < rules[j].Priority })
	return rules, nil
}

// UpsertRule 新增或更新结构化规则。
func (s *WebsiteService) UpsertRule(id uint, rule model.WAFRule) (model.WAFRule, error) {
	if id == 0 || strings.TrimSpace(rule.Name) == "" || len(rule.Name) > 120 || strings.TrimSpace(rule.Value) == "" || len(rule.Value) > 2048 {
		return model.WAFRule{}, errors.New("WAF 规则参数无效")
	}
	validLocation, validOperator, validAction := map[string]bool{"ip": true, "uri": true, "args": true, "header": true, "cookie": true, "method": true, "body": true}, map[string]bool{"contains": true, "regex": true, "equals": true, "ip-cidr": true}, map[string]bool{"allow": true, "log": true, "block": true}
	if !validLocation[rule.Location] || !validOperator[rule.Operator] || !validAction[rule.Action] {
		return model.WAFRule{}, errors.New("WAF 规则字段无效")
	}
	if rule.Priority <= 0 {
		rule.Priority = 100
	}
	if rule.Priority > 10000 {
		return model.WAFRule{}, errors.New("WAF 规则优先级无效")
	}
	if rule.Operator == "regex" {
		if len(rule.Value) > 256 {
			return model.WAFRule{}, errors.New("正则表达式长度不能超过 256")
		}
		if _, err := regexp.Compile(rule.Value); err != nil {
			return model.WAFRule{}, fmt.Errorf("正则表达式无效: %w", err)
		}
	}
	if rule.Operator == "ip-cidr" {
		if _, _, err := net.ParseCIDR(rule.Value); err != nil {
			return model.WAFRule{}, errors.New("IP 网段无效")
		}
	}
	if strings.TrimSpace(rule.ID) == "" {
		rule.ID = fmt.Sprintf("rule-%d", time.Now().UnixNano())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getWebsiteLocked(id); err != nil {
		return model.WAFRule{}, err
	}
	site := s.wafSites[id]
	site.WebsiteID, site.Rules = id, append([]model.WAFRule(nil), site.Rules...)
	found := false
	for i := range site.Rules {
		if site.Rules[i].ID == rule.ID {
			site.Rules[i], found = rule, true
			break
		}
	}
	if !found {
		site.Rules = append(site.Rules, rule)
	}
	s.wafSites[id] = site
	return rule, s.persistWAFSites()
}

// DeleteRule 删除指定网站规则。
func (s *WebsiteService) DeleteRule(id uint, ruleID string) error {
	if id == 0 || strings.TrimSpace(ruleID) == "" {
		return errors.New("WAF 规则参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getWebsiteLocked(id); err != nil {
		return err
	}
	site := s.wafSites[id]
	filtered := site.Rules[:0]
	for _, rule := range site.Rules {
		if rule.ID != ruleID {
			filtered = append(filtered, rule)
		}
	}
	site.Rules = filtered
	s.wafSites[id] = site
	return s.persistWAFSites()
}

// GetGlobal、UpdateGlobal 读取和更新节点级配置。
func (s *WebsiteService) GetGlobal() model.WAFGlobalConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.global
}
func (s *WebsiteService) UpdateGlobal(cfg model.WAFGlobalConfig) (model.WAFGlobalConfig, error) {
	if cfg.Mode != "observe" && cfg.Mode != "block" {
		return model.WAFGlobalConfig{}, errors.New("WAF 模式无效")
	}
	if cfg.ParanoiaLevel == 0 {
		cfg.ParanoiaLevel = 1
	}
	if cfg.ParanoiaLevel < 1 || cfg.ParanoiaLevel > 4 || cfg.InboundThreshold < 1 || cfg.InboundThreshold > 99 || cfg.RequestBodyLimit < 0 || cfg.RequestBodyLimit > 10485760 {
		return model.WAFGlobalConfig{}, errors.New("WAF 全局参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.global = cfg
	dir := strings.TrimSpace(os.Getenv("WORKMESH_WAF_GLOBAL_DIR"))
	if dir == "" {
		dir = filepath.Join(s.root, "waf")
	}
	if dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return model.WAFGlobalConfig{}, err
		}
		data, _ := json.Marshal(cfg)
		if err := os.WriteFile(filepath.Join(dir, "global.json"), data, 0o640); err != nil {
			return model.WAFGlobalConfig{}, err
		}
	}
	return cfg, s.persist("waf-global", cfg)
}
func (s *WebsiteService) GetLists() model.WAFAccessLists {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lists
}

// UpdateLists 校验 IP/CIDR、去重并持久化黑白名单。
func (s *WebsiteService) UpdateLists(lists model.WAFAccessLists) (model.WAFAccessLists, error) {
	clean := func(items []string) ([]string, error) {
		seen := map[string]bool{}
		out := []string{}
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if net.ParseIP(item) == nil {
				if _, _, err := net.ParseCIDR(item); err != nil {
					return nil, fmt.Errorf("IP 或网段无效: %s", item)
				}
			}
			if !seen[item] {
				seen[item] = true
				out = append(out, item)
			}
		}
		return out, nil
	}
	whitelist, err := clean(lists.Whitelist)
	if err != nil {
		return model.WAFAccessLists{}, err
	}
	blacklist, err := clean(lists.Blacklist)
	if err != nil {
		return model.WAFAccessLists{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lists = model.WAFAccessLists{Whitelist: whitelist, Blacklist: blacklist}
	return s.lists, s.persist("waf-access-lists", s.lists)
}

// StandardRules 返回离线可用的基础检测清单。
func (s *WebsiteService) StandardRules() []model.WAFStandardRule {
	return []model.WAFStandardRule{{ID: "CRS-942100", Category: "SQL 注入", Description: "检测联合查询和布尔盲注特征", Locations: []string{"uri", "args", "body", "header", "cookie"}}, {ID: "CRS-941100", Category: "跨站脚本", Description: "检测脚本标签和 javascript 协议", Locations: []string{"uri", "args", "body", "header", "cookie"}}, {ID: "CRS-930110", Category: "本地文件包含", Description: "检测目录穿越和敏感文件读取", Locations: []string{"uri", "args", "body"}}}
}

// GetOpenResty、UpdateOpenResty 管理 OpenResty 状态和配置摘要。
func (s *WebsiteService) GetOpenResty() model.OpenRestyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.openresty
}
func (s *WebsiteService) UpdateOpenResty(cfg model.OpenRestyConfig) (model.OpenRestyConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(cfg.Version) == "" {
		cfg.Version = s.openresty.Version
	}
	if cfg.Modules == nil {
		cfg.Modules = s.openresty.Modules
	}
	cfg.UpdatedAt = time.Now().UTC()
	s.openresty = cfg
	return cfg, s.persist("openresty", cfg)
}

// OpenRestyFile 返回持久化的 nginx.conf 内容；首次使用时从受控配置文件读取。
func (s *WebsiteService) OpenRestyFile() (string, error) {
	s.mu.RLock()
	content := s.openresty.ConfigContent
	s.mu.RUnlock()
	if content != "" {
		return content, nil
	}
	path := s.openRestyConfigPath()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// 自定义 WAF 镜像通常沿用官方路径，配置页应读取实际 nginx.conf。
		for _, candidate := range []string{"/etc/nginx/nginx.conf", "/usr/local/openresty/nginx/conf/nginx.conf", "/usr/local/openresty/nginx/conf/nginx.conf.default"} {
			if candidate == path {
				continue
			}
			if data, readErr := os.ReadFile(candidate); readErr == nil {
				path, b, err = candidate, data, nil
				break
			}
		}
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("读取 OpenResty 配置失败: %w", err)
	}
	if len(b) > 4<<20 {
		return "", errors.New("OpenResty 配置超过 4 MiB 限制")
	}
	return string(b), nil
}

// UpdateOpenRestyFile 原子写入 nginx.conf，并在需要时保留可回滚备份。
func (s *WebsiteService) UpdateOpenRestyFile(content string, backup bool) error {
	if strings.TrimSpace(content) == "" || strings.IndexByte(content, 0) >= 0 || len(content) > 4<<20 {
		return errors.New("OpenResty 配置内容无效")
	}
	if !balancedConfig(content) {
		return errors.New("OpenResty 配置括号不匹配")
	}
	path := s.openRestyConfigPath()
	old, readErr := os.ReadFile(path)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("读取 OpenResty 原配置失败: %w", readErr)
	}
	if backup && len(old) > 0 {
		if err := os.WriteFile(path+".bak", old, 0o600); err != nil {
			return fmt.Errorf("保存 OpenResty 配置备份失败: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("创建 OpenResty 配置目录失败: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return fmt.Errorf("写入 OpenResty 临时配置失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("替换 OpenResty 配置失败: %w", err)
	}
	s.mu.Lock()
	s.openresty.ConfigContent = content
	s.openresty.UpdatedAt = time.Now().UTC()
	err := s.persist("openresty", s.openresty)
	s.mu.Unlock()
	if err != nil {
		// 数据库状态写入失败时恢复原配置，避免文件与控制面状态分叉。
		if len(old) == 0 {
			_ = os.Remove(path)
		} else {
			_ = os.WriteFile(path, old, 0o600)
		}
		return fmt.Errorf("保存 OpenResty 配置状态失败: %w", err)
	}
	return nil
}

// OpenRestyScope 读取指定作用域下的白名单指令，防止任意字段写入配置。
func (s *WebsiteService) OpenRestyScope(scope string) (map[string]string, error) {
	keys, ok := openRestyScopeKeys(strings.TrimSpace(scope))
	if !ok {
		return nil, errors.New("OpenResty 配置作用域无效")
	}
	content, err := s.OpenRestyFile()
	if err != nil {
		return nil, err
	}
	values := parseOpenRestyDirectives(content)
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, exists := values[key]; exists {
			result[key] = value
		}
	}
	return result, nil
}

// UpdateOpenRestyScope 更新作用域白名单指令并复用原子配置写入流程。
func (s *WebsiteService) UpdateOpenRestyScope(scope string, params map[string]string, backup bool) error {
	keys, ok := openRestyScopeKeys(strings.TrimSpace(scope))
	if !ok || len(params) == 0 || len(params) > 32 {
		return errors.New("OpenResty 作用域参数无效")
	}
	allowed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		allowed[key] = struct{}{}
	}
	for key, value := range params {
		if _, exists := allowed[key]; !exists || strings.TrimSpace(value) == "" || len(value) > 256 || strings.ContainsAny(value, "{};\x00") {
			return fmt.Errorf("OpenResty 指令 %q 不允许或值无效", key)
		}
	}
	content, err := s.OpenRestyFile()
	if err != nil {
		return err
	}
	lines := strings.Split(content, "\n")
	seen := make(map[string]bool, len(params))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		for key, value := range params {
			if strings.HasPrefix(trimmed, key+" ") || strings.HasPrefix(trimmed, key+"\t") {
				indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				lines[i] = indent + key + " " + value + ";"
				seen[key] = true
			}
		}
	}
	for key, value := range params {
		if !seen[key] {
			lines = append(lines, key+" "+value+";")
		}
	}
	return s.UpdateOpenRestyFile(strings.Join(lines, "\n"), backup)
}

// BuildOpenResty 执行只读配置检查并返回真实探测结果，避免伪造构建成功。
func (s *WebsiteService) BuildOpenResty(ctx context.Context, modules []string) (OpenRestyStatus, error) {
	status := s.ProbeOpenResty(ctx)
	if !status.Available {
		if status.Error == "" {
			status.Error = "OpenResty 不可用"
		}
		return status, fmt.Errorf("OpenResty 构建前检查失败: %s", status.Error)
	}
	if !status.ConfigValid {
		return status, errors.New("OpenResty 配置检查未通过")
	}
	if len(modules) > 100 {
		return status, errors.New("OpenResty 模块数量超出限制")
	}
	selected := make(map[string]struct{}, len(modules))
	for _, name := range modules {
		name = strings.TrimSpace(name)
		if name == "" || len(name) > 120 || strings.ContainsAny(name, " /\\") {
			return status, errors.New("OpenResty 模块名称无效")
		}
		selected[name] = struct{}{}
	}
	return status, nil
}

// OperateOpenResty 对宿主机或容器中的 OpenResty 执行受控信号操作。
func (s *WebsiteService) OperateOpenResty(ctx context.Context, operation string) (OpenRestyStatus, error) {
	operation = strings.ToLower(strings.TrimSpace(operation))
	if operation != "reload" && operation != "restart" && operation != "stop" && operation != "start" {
		return OpenRestyStatus{}, errors.New("OpenResty 操作无效")
	}
	status := s.ProbeOpenResty(ctx)
	if status.Binary == "" {
		return status, errors.New("OpenResty 未安装或不可用")
	}
	if strings.HasPrefix(status.Binary, "proc://") {
		return status, errors.New("OpenResty 位于独立命名空间，当前节点无法执行 reload；请配置 WORKMESH_OPENRESTY_BIN 或等效 reload 命令")
	}
	var cmd *exec.Cmd
	if strings.HasPrefix(status.Binary, "docker://") {
		name := strings.TrimPrefix(status.Binary, "docker://")
		if name == "" {
			return status, errors.New("OpenResty 容器标识无效")
		}
		docker := dockerBinaryOrName()
		switch operation {
		case "start":
			cmd = exec.CommandContext(ctx, docker, "start", name)
		case "restart":
			cmd = exec.CommandContext(ctx, docker, "restart", name)
		case "stop":
			cmd = exec.CommandContext(ctx, docker, "stop", name)
		default:
			cmd = exec.CommandContext(ctx, docker, "exec", name, "nginx", "-s", "reload")
		}
	} else {
		switch operation {
		case "start":
			cmd = exec.CommandContext(ctx, status.Binary)
		case "restart":
			// nginx 没有 restart signal，使用 stop 后重新启动，确保返回真实错误。
			stop := exec.CommandContext(ctx, status.Binary, "-s", "stop")
			if output, err := stop.CombinedOutput(); err != nil {
				return status, fmt.Errorf("OpenResty restart 停止阶段失败: %s: %w", strings.TrimSpace(string(output)), err)
			}
			cmd = exec.CommandContext(ctx, status.Binary)
		default:
			cmd = exec.CommandContext(ctx, status.Binary, "-s", operation)
		}
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return status, fmt.Errorf("OpenResty %s 失败: %s: %w", operation, strings.TrimSpace(string(output)), err)
	}
	return s.ProbeOpenResty(ctx), nil
}

// ClearOpenRestyCache 清理受控的反向代理缓存目录，不接受请求直接传入的任意路径。
func (s *WebsiteService) ClearOpenRestyCache() error {
	cacheRoot := strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_CACHE_DIR"))
	if cacheRoot == "" {
		cacheRoot = filepath.Join(s.root, "openresty-cache")
	}
	cacheRoot = filepath.Clean(cacheRoot)
	if cacheRoot == "." || cacheRoot == string(filepath.Separator) || strings.Contains(cacheRoot, ".."+string(filepath.Separator)) {
		return errors.New("OpenResty 缓存目录无效")
	}
	if err := os.MkdirAll(cacheRoot, 0o750); err != nil {
		return err
	}
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(cacheRoot, entry.Name())); err != nil {
			return fmt.Errorf("清理 OpenResty 缓存失败: %w", err)
		}
	}
	return nil
}

func (s *WebsiteService) openRestyConfigPath() string {
	if configured := strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_CONFIG")); configured != "" {
		return filepath.Clean(configured)
	}
	return filepath.Join(s.root, "openresty.conf")
}

func openRestyScopeKeys(scope string) ([]string, bool) {
	switch scope {
	case "index":
		return []string{"index"}, true
	case "limit-conn":
		return []string{"limit_conn", "limit_rate", "limit_conn_zone"}, true
	case "ssl":
		return []string{"ssl_certificate", "ssl_certificate_key"}, true
	case "http-per":
		return []string{"server_names_hash_bucket_size", "client_header_buffer_size", "client_max_body_size", "keepalive_timeout", "gzip", "gzip_min_length", "gzip_comp_level"}, true
	default:
		return nil, false
	}
}

func parseOpenRestyDirectives(content string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(strings.TrimSuffix(line, ";"))
		if len(fields) >= 2 {
			result[fields[0]] = strings.Join(fields[1:], " ")
		}
	}
	return result
}

func balancedConfig(content string) bool {
	depth := 0
	for _, r := range content {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

// ProbeOpenResty 使用受限外部命令探测 OpenResty 安装及配置状态；命令均设置超时且不接受用户参数。
func (s *WebsiteService) ProbeOpenResty(ctx context.Context) OpenRestyStatus {
	cfg := s.GetOpenResty()
	status := OpenRestyStatus{Enabled: cfg.Enabled, DefaultHTTPS: cfg.DefaultHTTPS, Modules: append([]model.OpenRestyModule{}, cfg.Modules...)}
	bin := strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_BIN"))
	if bin == "" {
		for _, candidate := range []string{"openresty", "nginx"} {
			if found, err := lookupApplicationBinary(candidate, "openresty", false); err == nil {
				bin = found
				break
			}
		}
	}
	if bin == "" {
		if container, ok := probeOpenRestyContainer(ctx); ok {
			containerStatus := OpenRestyStatus{
				Available: true, ConfigValid: true, Enabled: container.IsActive,
				Version: container.Version, Binary: container.Binary,
				DefaultHTTPS: cfg.DefaultHTTPS, Modules: append([]model.OpenRestyModule{}, cfg.Modules...),
			}
			return s.probeStubStatus(ctx, containerStatus)
		}
		if process, ok := s.probeOpenRestyProcessNamespace(); ok {
			process.Enabled = cfg.Enabled
			process.DefaultHTTPS = cfg.DefaultHTTPS
			process.Modules = append([]model.OpenRestyModule{}, cfg.Modules...)
			return process
		}
		status.Error = "未找到 OpenResty 可执行文件或运行进程"
		return status
	}
	status.Binary = bin
	status.ConfigPath = s.openRestyConfigPath()
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	versionOut, err := exec.CommandContext(probeCtx, bin, "-v").CombinedOutput()
	if err != nil {
		status.Error = strings.TrimSpace(string(versionOut))
		if status.Error == "" {
			status.Error = err.Error()
		}
		return status
	}
	status.Available = true
	status.Version = parseOpenRestyVersion(string(versionOut))
	if status.Version == "" {
		status.Version = cfg.Version
	}
	configCtx, cancelConfig := context.WithTimeout(ctx, 2*time.Second)
	defer cancelConfig()
	configOut, configErr := exec.CommandContext(configCtx, bin, "-t").CombinedOutput()
	status.ConfigValid = configErr == nil
	if configErr != nil && status.Error == "" {
		status.Error = strings.TrimSpace(string(configOut))
	}
	status = s.probeStubStatus(ctx, status)
	return status
}

// probeStubStatus 读取 nginx_stub_status 的实时计数；接口不可用时保留零值并返回探测状态。
func (s *WebsiteService) probeStubStatus(ctx context.Context, status OpenRestyStatus) OpenRestyStatus {
	endpoint := strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_STATUS_URL"))
	if endpoint == "" {
		endpoint = "http://127.0.0.1/status"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return status
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	response, err := client.Do(request)
	if err != nil {
		return status
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return status
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return status
	}
	metrics := parseStubStatus(string(body))
	status.Active, status.Accepts, status.Handled, status.Requests, status.Reading, status.Writing, status.Waiting = metrics.active, metrics.accepts, metrics.handled, metrics.requests, metrics.reading, metrics.writing, metrics.waiting
	return status
}

type stubMetrics struct {
	active, reading, writing, waiting int
	accepts, handled, requests        int64
}

func parseStubStatus(content string) stubMetrics {
	var m stubMetrics
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		f := strings.Fields(line)
		if len(f) >= 3 && strings.EqualFold(f[0], "Active") {
			m.active, _ = strconv.Atoi(f[len(f)-1])
		}
		if len(f) >= 4 && f[0] == "server" && i+1 < len(lines) {
			v := strings.Fields(lines[i+1])
			if len(v) >= 3 {
				m.accepts, _ = strconv.ParseInt(v[0], 10, 64)
				m.handled, _ = strconv.ParseInt(v[1], 10, 64)
				m.requests, _ = strconv.ParseInt(v[2], 10, 64)
			}
		}
		if len(f) >= 6 && f[0] == "Reading:" {
			m.reading, _ = strconv.Atoi(strings.TrimSuffix(f[1], ","))
			m.writing, _ = strconv.Atoi(strings.TrimSuffix(f[3], ","))
			m.waiting, _ = strconv.Atoi(strings.TrimSuffix(f[5], ","))
		}
	}
	return m
}

// probeOpenRestyProcessNamespace 从 procfs 识别独立挂载命名空间中的 Nginx/OpenResty 主进程。
// 无法进入命名空间时只报告真实进程、cgroup、配置和监听信息，不虚构 -t 校验结果。
func (s *WebsiteService) probeOpenRestyProcessNamespace() (OpenRestyStatus, bool) {
	if runtime.GOOS == "windows" {
		return OpenRestyStatus{}, false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return OpenRestyStatus{}, false
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		base := filepath.Join("/proc", entry.Name())
		exe, _ := os.Readlink(filepath.Join(base, "exe"))
		cmdline, _ := os.ReadFile(filepath.Join(base, "cmdline"))
		identity := strings.ToLower(filepath.Base(exe) + " " + strings.ReplaceAll(string(cmdline), "\x00", " "))
		if !strings.Contains(identity, "openresty") && !strings.Contains(identity, "nginx") {
			continue
		}
		cgroup, _ := os.ReadFile(filepath.Join(base, "cgroup"))
		status := OpenRestyStatus{Available: true, Binary: "proc://" + entry.Name() + "/exe", ProcessID: pid, Cgroup: strings.TrimSpace(string(cgroup)), Listening: processListeningPorts(base)}
		for _, configured := range []string{s.openRestyConfigPath(), "/etc/nginx/nginx.conf", "/usr/local/openresty/nginx/conf/nginx.conf"} {
			path := filepath.Join(base, "root", strings.TrimPrefix(filepath.Clean(configured), string(filepath.Separator)))
			if content, readErr := os.ReadFile(path); readErr == nil {
				status.ConfigPath = configured
				if balancedConfig(string(content)) {
					status.Error = "检测到独立命名空间中的 OpenResty，当前服务无法进入该命名空间执行配置语法检查"
				} else {
					status.Error = "检测到独立命名空间中的 OpenResty，但读取到的配置括号不匹配"
				}
				return status, true
			}
		}
		status.Error = "检测到独立命名空间中的 OpenResty，但无权限读取其配置文件"
		return status, true
	}
	return OpenRestyStatus{}, false
}

func processListeningPorts(procRoot string) []int {
	ports := make([]int, 0, 2)
	seen := map[int]bool{}
	for _, name := range []string{"tcp", "tcp6"} {
		content, err := os.ReadFile(filepath.Join(procRoot, "net", name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(content), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 || fields[3] != "0A" {
				continue
			}
			parts := strings.Split(fields[1], ":")
			if len(parts) != 2 {
				continue
			}
			port, err := strconv.ParseInt(parts[1], 16, 32)
			if err == nil && port > 0 && !seen[int(port)] {
				seen[int(port)] = true
				ports = append(ports, int(port))
			}
		}
	}
	sort.Ints(ports)
	return ports
}

func parseOpenRestyVersion(output string) string {
	for _, token := range strings.Fields(output) {
		if strings.HasPrefix(token, "openresty/") {
			return strings.TrimPrefix(token, "openresty/")
		}
		if strings.HasPrefix(token, "nginx/") {
			return strings.TrimPrefix(token, "nginx/")
		}
	}
	return ""
}
