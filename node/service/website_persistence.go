// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/model"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

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
		repository, err := s.sqliteRepository()
		if err != nil {
			return err
		}
		legacyPayload, err := hasColumn(repository, "websites", "payload")
		if err != nil {
			return fmt.Errorf("读取网站表结构失败: %w", err)
		}
		return repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
			for _, item := range items {
				args := []any{item.ID, item.Protocol, item.PrimaryDomain, item.Type, item.Alias, item.Remark, item.Status, item.HttpConfig, formatTime(item.ExpireDate), item.Proxy, item.ProxyType, item.SiteDir, boolInt(item.ErrorLog), boolInt(item.AccessLog), boolInt(item.DefaultServer), boolInt(item.IPV6), item.Rewrite, item.WebsiteGroupID, item.WebsiteSSLID, item.RuntimeID, item.AppInstallID, item.AppInstallRef, item.FtpID, item.ParentWebsiteID, item.User, item.Group, item.DbType, item.DbID, boolInt(item.Favorite), item.StreamPorts, boolInt(item.UDP), formatTime(item.CreatedAt), formatTime(item.UpdatedAt)}
				statement := `INSERT INTO websites(id,protocol,primary_domain,type,alias,remark,status,http_config,expire_date,proxy,proxy_type,site_dir,error_log,access_log,default_server,ipv6,rewrite,website_group_id,website_ssl_id,runtime_id,app_install_id,app_install_ref,ftp_id,parent_website_id,user,"group",db_type,db_id,favorite,stream_ports,udp,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET protocol=excluded.protocol,primary_domain=excluded.primary_domain,type=excluded.type,alias=excluded.alias,remark=excluded.remark,status=excluded.status,http_config=excluded.http_config,expire_date=excluded.expire_date,proxy=excluded.proxy,proxy_type=excluded.proxy_type,site_dir=excluded.site_dir,error_log=excluded.error_log,access_log=excluded.access_log,default_server=excluded.default_server,ipv6=excluded.ipv6,rewrite=excluded.rewrite,website_group_id=excluded.website_group_id,website_ssl_id=excluded.website_ssl_id,runtime_id=excluded.runtime_id,app_install_id=excluded.app_install_id,app_install_ref=excluded.app_install_ref,ftp_id=excluded.ftp_id,parent_website_id=excluded.parent_website_id,user=excluded.user,"group"=excluded."group",db_type=excluded.db_type,db_id=excluded.db_id,favorite=excluded.favorite,stream_ports=excluded.stream_ports,udp=excluded.udp,updated_at=excluded.updated_at`
				if legacyPayload {
					statement = `INSERT INTO websites(id,protocol,primary_domain,type,alias,remark,status,http_config,expire_date,proxy,proxy_type,site_dir,error_log,access_log,default_server,ipv6,rewrite,website_group_id,website_ssl_id,runtime_id,app_install_id,app_install_ref,ftp_id,parent_website_id,user,"group",db_type,db_id,favorite,stream_ports,udp,created_at,updated_at,payload) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET protocol=excluded.protocol,primary_domain=excluded.primary_domain,type=excluded.type,alias=excluded.alias,remark=excluded.remark,status=excluded.status,http_config=excluded.http_config,expire_date=excluded.expire_date,proxy=excluded.proxy,proxy_type=excluded.proxy_type,site_dir=excluded.site_dir,error_log=excluded.error_log,access_log=excluded.access_log,default_server=excluded.default_server,ipv6=excluded.ipv6,rewrite=excluded.rewrite,website_group_id=excluded.website_group_id,website_ssl_id=excluded.website_ssl_id,runtime_id=excluded.runtime_id,app_install_id=excluded.app_install_id,app_install_ref=excluded.app_install_ref,ftp_id=excluded.ftp_id,parent_website_id=excluded.parent_website_id,user=excluded.user,"group"=excluded."group",db_type=excluded.db_type,db_id=excluded.db_id,favorite=excluded.favorite,stream_ports=excluded.stream_ports,udp=excluded.udp,updated_at=excluded.updated_at`
					args = append(args, []byte{})
				}
				// Keep the legacy SQL literal in one place while matching the typed argument list.
				if !legacyPayload {
					statement = strings.Replace(statement, "?,?) ON CONFLICT", "?) ON CONFLICT", 1)
				}
				if _, err := tx.Exec(statement, args...); err != nil {
					return err
				}
			}
			return nil
		})
	}
	if key == "website-domains" {
		domains, ok := value.(map[uint][]model.WebsiteDomain)
		if !ok {
			return errors.New("网站域名数据类型无效")
		}
		repository, err := s.sqliteRepository()
		if err != nil {
			return err
		}
		legacyPayload, err := hasColumn(repository, "website_domains", "payload")
		if err != nil {
			return fmt.Errorf("读取网站域名表结构失败: %w", err)
		}
		return repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
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
						if _, err := tx.Exec(statement, args...); err != nil {
							return err
						}
					}
				}
			}
			return nil
		})
	}
	if key == "website-configs" {
		configs, ok := value.(map[uint]map[string]any)
		if !ok {
			return errors.New("网站配置数据类型无效")
		}
		repository, err := s.sqliteRepository()
		if err != nil {
			return err
		}
		return repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
			for websiteID, entries := range configs {
				for typ, raw := range entries {
					if strings.TrimSpace(typ) == "" {
						continue
					}
					encoded, marshalErr := json.Marshal(raw)
					if marshalErr != nil {
						return marshalErr
					}
					if _, err := tx.Exec(`INSERT INTO website_settings(website_id,config_type,content,updated_at) VALUES(?,?,?,?) ON CONFLICT(website_id,config_type) DO UPDATE SET content=excluded.content,updated_at=excluded.updated_at`, websiteID, typ, string(encoded), formatTime(time.Now())); err != nil {
						return err
					}
				}
			}
			return nil
		})
	}
	if key == "openresty" {
		cfg, ok := value.(model.OpenRestyConfig)
		if !ok {
			return errors.New("OpenResty 数据类型无效")
		}
		repository, err := s.sqliteRepository()
		if err != nil {
			return err
		}
		return repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
			if _, err := tx.Exec(`INSERT INTO website_openresty_config(id,version,enabled,default_https,ssl_reject_handshake,config_content,updated_at) VALUES(1,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET version=excluded.version,enabled=excluded.enabled,default_https=excluded.default_https,ssl_reject_handshake=excluded.ssl_reject_handshake,config_content=excluded.config_content,updated_at=excluded.updated_at`, cfg.Version, boolInt(cfg.Enabled), boolInt(cfg.DefaultHTTPS), boolInt(cfg.SSLRejectHandshake), cfg.ConfigContent, formatTime(cfg.UpdatedAt)); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM website_openresty_modules`); err != nil {
				return err
			}
			for _, m := range cfg.Modules {
				if _, err := tx.Exec(`INSERT INTO website_openresty_modules(name,enabled,build_mode,load_order,custom,script,packages,params,provider,build_status,load_status,last_error) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, m.Name, boolInt(m.Enabled), m.BuildMode, m.LoadOrder, boolInt(m.Custom), m.Script, m.Packages, m.Params, m.Provider, m.BuildStatus, m.LoadStatus, m.LastError); err != nil {
					return err
				}
			}
			return nil
		})
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
