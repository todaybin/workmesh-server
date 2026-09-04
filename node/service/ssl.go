// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

// SSLService 管理网站证书元数据和证书内容校验。
type SSLService struct {
	mu     sync.RWMutex
	root   string
	serial uint
	items  map[uint]model.WebsiteSSL
	db     *sql.DB
	owner  *storage.Store
}

// NewSSLService 创建证书服务。
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
	}
	s.load()
	return s
}

func (s *SSLService) load() {
	if s.db == nil {
		return
	}
	rows, err := s.db.Query(`SELECT id,primary_domain,private_key,pem,domains,provider,acme_account_id,dns_account_id,auto_renew,expire_date,start_date,status,message,key_type,push_dir,dir,description,push_node,nodes,created_at,updated_at FROM website_ssls ORDER BY id`)
	if err == nil {
		for rows.Next() {
			var item model.WebsiteSSL
			var id, acmeID, dnsID int64
			var autoRenew, pushDir, pushNode int
			var expire, start, created, updated string
			if rows.Scan(&id, &item.PrimaryDomain, &item.PrivateKey, &item.Certificate, &item.Domains, &item.Provider, &acmeID, &dnsID, &autoRenew, &expire, &start, &item.Status, &item.Message, &item.KeyType, &pushDir, &item.Dir, &item.Description, &pushNode, &item.Nodes, &created, &updated) == nil {
				item.ID = uint(id)
				item.PEM = item.Certificate
				item.AcmeAccountID = uint(acmeID)
				item.DnsAccountID = uint(dnsID)
				item.AutoRenew = autoRenew != 0
				item.PushDir = pushDir != 0
				item.PushNode = pushNode != 0
				item.ExpireDate, _ = time.Parse(time.RFC3339Nano, expire)
				item.StartDate, _ = time.Parse(time.RFC3339Nano, start)
				item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
				item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
				s.items[item.ID] = item
				if item.ID > s.serial {
					s.serial = item.ID
				}
			}
		}
		rows.Close()
	}
}

func (s *SSLService) persistLocked() error {
	if s.db == nil {
		return errors.New("网站公共数据库未初始化")
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
	for _, item := range s.items {
		_, err = tx.Exec(`INSERT INTO website_ssls(id,primary_domain,private_key,pem,domains,provider,acme_account_id,dns_account_id,auto_renew,expire_date,start_date,status,message,key_type,push_dir,dir,description,push_node,nodes,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET primary_domain=excluded.primary_domain,private_key=excluded.private_key,pem=excluded.pem,domains=excluded.domains,provider=excluded.provider,acme_account_id=excluded.acme_account_id,dns_account_id=excluded.dns_account_id,auto_renew=excluded.auto_renew,expire_date=excluded.expire_date,start_date=excluded.start_date,status=excluded.status,message=excluded.message,key_type=excluded.key_type,push_dir=excluded.push_dir,dir=excluded.dir,description=excluded.description,push_node=excluded.push_node,nodes=excluded.nodes,updated_at=excluded.updated_at`, item.ID, item.PrimaryDomain, item.PrivateKey, item.Certificate, item.Domains, item.Provider, item.AcmeAccountID, item.DnsAccountID, boolInt(item.AutoRenew), formatTime(item.ExpireDate), formatTime(item.StartDate), item.Status, item.Message, item.KeyType, boolInt(item.PushDir), item.Dir, item.Description, boolInt(item.PushNode), item.Nodes, formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Create 保存证书申请配置，证书申请由 ACME 适配器异步完成。
func (s *SSLService) Create(_ context.Context, req model.WebsiteSSLCreateRequest) (model.WebsiteSSL, error) {
	if strings.TrimSpace(req.PrimaryDomain) == "" || strings.TrimSpace(req.Provider) == "" {
		return model.WebsiteSSL{}, errors.New("主域名和证书提供商不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serial++
	now := time.Now().UTC()
	item := model.WebsiteSSL{ID: s.serial, PrimaryDomain: req.PrimaryDomain, Domains: req.OtherDomains, Provider: req.Provider, AcmeAccountID: req.AcmeAccountID, DnsAccountID: req.DnsAccountID, AutoRenew: req.AutoRenew, KeyType: req.KeyType, Description: req.Description, Status: "pending", Type: "acme", CreatedAt: now, UpdatedAt: now}
	s.items[item.ID] = item
	if err := s.persistLocked(); err != nil {
		delete(s.items, item.ID)
		return model.WebsiteSSL{}, fmt.Errorf("保存证书配置失败: %w", err)
	}
	return publicSSL(item), nil
}

// List 按域名过滤并返回脱敏证书列表。
func (s *SSLService) List(_ context.Context, domain string) []model.WebsiteSSL {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]model.WebsiteSSL, 0, len(s.items))
	for _, item := range s.items {
		if domain == "" || strings.Contains(item.PrimaryDomain, domain) || strings.Contains(item.Domains, domain) {
			result = append(result, publicSSL(item))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID > result[j].ID })
	return result
}

// Get 返回指定证书，不泄漏私钥。
func (s *SSLService) Get(_ context.Context, id uint) (model.WebsiteSSL, error) {
	s.mu.RLock()
	item, ok := s.items[id]
	s.mu.RUnlock()
	if !ok {
		return model.WebsiteSSL{}, errors.New("证书不存在")
	}
	return publicSSL(item), nil
}

// Upload 导入 PEM 证书并检查证书有效期。
func (s *SSLService) Upload(_ context.Context, req model.WebsiteSSLUploadRequest) (model.WebsiteSSL, error) {
	if strings.TrimSpace(req.Certificate) == "" {
		return model.WebsiteSSL{}, errors.New("证书内容不能为空")
	}
	block, _ := pem.Decode([]byte(req.Certificate))
	if block == nil || block.Type != "CERTIFICATE" {
		return model.WebsiteSSL{}, errors.New("证书必须是 PEM CERTIFICATE")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return model.WebsiteSSL{}, fmt.Errorf("解析证书失败: %w", err)
	}
	if !certificate.NotAfter.After(time.Now()) {
		return model.WebsiteSSL{}, errors.New("证书已经过期")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.ID == 0 {
		s.serial++
		req.ID = s.serial
	}
	item := s.items[req.ID]
	item.ID = req.ID
	item.PrimaryDomain = firstDomain(certificate)
	item.Certificate = req.Certificate
	item.PEM = req.Certificate
	item.PrivateKey = req.PrivateKey
	item.StartDate = certificate.NotBefore
	item.ExpireDate = certificate.NotAfter
	item.Status = "active"
	item.Message = "证书已导入"
	item.Type = req.Type
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	item.UpdatedAt = time.Now().UTC()
	if req.Description != "" {
		item.Description = req.Description
	}
	s.items[item.ID] = item
	if err := s.persistLocked(); err != nil {
		return model.WebsiteSSL{}, fmt.Errorf("保存证书失败: %w", err)
	}
	return publicSSL(item), nil
}

// Delete 删除证书。
func (s *SSLService) Delete(_ context.Context, ids []uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		if _, ok := s.items[id]; !ok {
			return fmt.Errorf("证书 %d 不存在", id)
		}
	}
	for _, id := range ids {
		delete(s.items, id)
	}
	if err := s.persistLocked(); err != nil {
		return fmt.Errorf("保存证书删除状态失败: %w", err)
	}
	return nil
}

// Update 更新可变配置字段。
func (s *SSLService) Update(_ context.Context, req model.WebsiteSSLUpdateRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[req.ID]
	if !ok {
		return errors.New("证书不存在")
	}
	if strings.TrimSpace(req.PrimaryDomain) == "" {
		return errors.New("主域名不能为空")
	}
	item.PrimaryDomain, item.Domains, item.Provider = strings.TrimSpace(req.PrimaryDomain), req.OtherDomains, req.Provider
	item.AutoRenew, item.Description = req.AutoRenew, req.Description
	item.UpdatedAt = time.Now().UTC()
	s.items[req.ID] = item
	if err := s.persistLocked(); err != nil {
		return fmt.Errorf("保存证书设置失败: %w", err)
	}
	return nil
}

// RenewCheck 返回需要续期的证书列表，不主动调用 ACME，避免阻塞 HTTP 请求。
func (s *SSLService) RenewCheck(_ context.Context, before time.Duration) []model.WebsiteSSL {
	deadline := time.Now().Add(before)
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]model.WebsiteSSL, 0)
	for _, item := range s.items {
		if item.AutoRenew && !item.ExpireDate.IsZero() && item.ExpireDate.Before(deadline) {
			result = append(result, publicSSL(item))
		}
	}
	return result
}

func publicSSL(item model.WebsiteSSL) model.WebsiteSSL { item.PrivateKey = ""; return item }

func firstDomain(certificate *x509.Certificate) string {
	if len(certificate.DNSNames) > 0 {
		return certificate.DNSNames[0]
	}
	return certificate.Subject.CommonName
}
