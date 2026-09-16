// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

// SSLService 管理网站证书元数据、证书校验和站点证书文件同步。
type SSLService struct {
	mu         sync.RWMutex
	root       string
	serial     uint
	items      map[uint]model.WebsiteSSL
	db         *sql.DB
	repository storage.Transactional
	owner      *storage.Store
}

// NewSSLService 创建证书服务并从进程公共 SQLite 数据库恢复证书记录。
func NewSSLService() *SSLService {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	s := &SSLService{root: root, items: make(map[uint]model.WebsiteSSL), db: currentWebsiteDB()}
	if s.db == nil {
		if opened, err := storage.Open(filepath.Join(root, "workmesh.db")); err == nil {
			s.owner, s.db = opened, opened.DB()
		}
	}
	if s.db != nil {
		_ = ensureWebsiteTables(s.db)
		_, _ = s.sqliteRepository()
	}
	s.load()
	return s
}

func (s *SSLService) sqliteRepository() (storage.Transactional, error) {
	if s == nil {
		return nil, errors.New("证书服务未初始化")
	}
	if s.repository != nil {
		return s.repository, nil
	}
	if s.db == nil {
		return nil, errors.New("网站公共数据库未初始化")
	}
	repository, err := storage.NewSQLiteRepository(s.db)
	if err != nil {
		return nil, err
	}
	s.repository = repository
	return repository, nil
}

// List 按域名过滤并返回脱敏证书列表。
func (s *SSLService) List(_ context.Context, domain string) []model.WebsiteSSL {
	s.mu.RLock()
	defer s.mu.RUnlock()
	domain = strings.ToLower(strings.TrimSpace(domain))
	result := make([]model.WebsiteSSL, 0, len(s.items))
	for _, item := range s.items {
		if domain == "" || strings.Contains(strings.ToLower(item.PrimaryDomain), domain) || strings.Contains(strings.ToLower(item.Domains), domain) {
			result = append(result, publicSSL(item))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID > result[j].ID })
	s.enrichPublicSSL(result)
	return result
}

// Get 返回指定证书详情，响应中不包含私钥。
func (s *SSLService) Get(_ context.Context, id uint) (model.WebsiteSSL, error) {
	s.mu.RLock()
	item, ok := s.items[id]
	s.mu.RUnlock()
	if !ok {
		return model.WebsiteSSL{}, errors.New("证书不存在")
	}
	result := publicSSL(item)
	items := []model.WebsiteSSL{result}
	s.enrichPublicSSL(items)
	return items[0], nil
}

// LogPath 返回指定证书申请日志的受控路径，不接受请求传入任意文件路径。
func (s *SSLService) LogPath(_ context.Context, id uint) (string, error) {
	s.mu.RLock()
	item, ok := s.items[id]
	s.mu.RUnlock()
	if !ok {
		return "", errors.New("证书不存在")
	}
	domain := strings.TrimSpace(item.PrimaryDomain)
	if domain == "" || filepath.Base(domain) != domain || strings.ContainsAny(domain, `/\`) {
		return "", errors.New("证书主域名无效")
	}
	return filepath.Join(s.root, "logs", "ssl", fmt.Sprintf("%s-ssl-%d.log", domain, item.ID)), nil
}

// publicSSL 生成不含私钥的证书响应副本。
func publicSSL(item model.WebsiteSSL) model.WebsiteSSL {
	item.PrivateKey = ""
	return item
}

// enrichPublicSSL 批量补齐原系统证书 DTO 的账户、网站和日志字段。
func (s *SSLService) enrichPublicSSL(items []model.WebsiteSSL) {
	repository, err := s.sqliteRepository()
	if err != nil || len(items) == 0 {
		return
	}
	acme := map[uint]model.WebsiteSSLACMEAccount{}
	if rows, err := repository.Query(`SELECT id,email,url,type,key_type,use_proxy FROM website_acme_accounts`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var item model.WebsiteSSLACMEAccount
			var proxy int
			if rows.Scan(&item.ID, &item.Email, &item.URL, &item.Type, &item.KeyType, &proxy) == nil {
				item.UseProxy = proxy != 0
				acme[item.ID] = item
			}
		}
	}
	dnsAccounts := map[uint]model.WebsiteSSLDNSAccount{}
	if rows, err := repository.Query(`SELECT id,name,provider FROM website_dns_accounts`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var item model.WebsiteSSLDNSAccount
			if rows.Scan(&item.ID, &item.Name, &item.Type) == nil {
				dnsAccounts[item.ID] = item
			}
		}
	}
	websites := map[uint][]model.Website{}
	if rows, err := repository.Query(`SELECT id,primary_domain,website_ssl_id FROM websites WHERE website_ssl_id>0 ORDER BY id`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var website model.Website
			var sslID uint
			if rows.Scan(&website.ID, &website.PrimaryDomain, &sslID) == nil {
				websites[sslID] = append(websites[sslID], website)
			}
		}
	}
	for i := range items {
		if account, ok := acme[items[i].AcmeAccountID]; ok {
			accountCopy := account
			items[i].AcmeAccount = &accountCopy
		}
		if account, ok := dnsAccounts[items[i].DnsAccountID]; ok {
			accountCopy := account
			items[i].DnsAccount = &accountCopy
		}
		items[i].Websites = websites[items[i].ID]
		if items[i].Websites == nil {
			items[i].Websites = []model.Website{}
		}
		if path, err := s.LogPath(context.Background(), items[i].ID); err == nil {
			items[i].LogPath = path
		}
	}
}

// firstDomain 从证书 SAN 或主题中选取主域名。
func firstDomain(certificate *x509.Certificate) string {
	if len(certificate.DNSNames) > 0 {
		return certificate.DNSNames[0]
	}
	return certificate.Subject.CommonName
}
