// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

// load 从 SQLite 恢复证书记录，并同步当前序列号。
func (s *SSLService) load() {
	repository, err := s.sqliteRepository()
	if err != nil {
		return
	}
	rows, err := repository.Query(`SELECT id,primary_domain,private_key,pem,domains,provider,acme_account_id,dns_account_id,auto_renew,expire_date,start_date,status,message,key_type,push_dir,dir,description,push_node,nodes,created_at,updated_at FROM website_ssls ORDER BY id`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var item model.WebsiteSSL
		var id, acmeID, dnsID int64
		var autoRenew, pushDir, pushNode int
		var expire, start, created, updated string
		if rows.Scan(&id, &item.PrimaryDomain, &item.PrivateKey, &item.Certificate, &item.Domains, &item.Provider, &acmeID, &dnsID, &autoRenew, &expire, &start, &item.Status, &item.Message, &item.KeyType, &pushDir, &item.Dir, &item.Description, &pushNode, &item.Nodes, &created, &updated) != nil {
			continue
		}
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

// persistLocked 在调用方持有 s.mu 写锁时，将完整证书索引事务化写入 SQLite。
func (s *SSLService) persistLocked() error {
	repository, err := s.sqliteRepository()
	if err != nil {
		return err
	}
	return repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
		// 清理内存索引中已删除的行，避免重启后恢复已删除证书。
		if len(s.items) == 0 {
			if _, err := tx.Exec(`DELETE FROM website_ssls`); err != nil {
				return err
			}
		} else {
			ids := make([]any, 0, len(s.items))
			marks := make([]string, 0, len(s.items))
			for id := range s.items {
				ids = append(ids, id)
				marks = append(marks, "?")
			}
			if _, err := tx.Exec(`DELETE FROM website_ssls WHERE id NOT IN (`+strings.Join(marks, ",")+")", ids...); err != nil {
				return err
			}
		}
		for _, item := range s.items {
			if _, err := tx.Exec(`INSERT INTO website_ssls(id,primary_domain,private_key,pem,domains,provider,acme_account_id,dns_account_id,auto_renew,expire_date,start_date,status,message,key_type,push_dir,dir,description,push_node,nodes,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET primary_domain=excluded.primary_domain,private_key=excluded.private_key,pem=excluded.pem,domains=excluded.domains,provider=excluded.provider,acme_account_id=excluded.acme_account_id,dns_account_id=excluded.dns_account_id,auto_renew=excluded.auto_renew,expire_date=excluded.expire_date,start_date=excluded.start_date,status=excluded.status,message=excluded.message,key_type=excluded.key_type,push_dir=excluded.push_dir,dir=excluded.dir,description=excluded.description,push_node=excluded.push_node,nodes=excluded.nodes,updated_at=excluded.updated_at`, item.ID, item.PrimaryDomain, item.PrivateKey, item.Certificate, item.Domains, item.Provider, item.AcmeAccountID, item.DnsAccountID, boolInt(item.AutoRenew), formatTime(item.ExpireDate), formatTime(item.StartDate), item.Status, item.Message, item.KeyType, boolInt(item.PushDir), item.Dir, item.Description, boolInt(item.PushNode), item.Nodes, formatTime(item.CreatedAt), formatTime(item.UpdatedAt)); err != nil {
				return err
			}
		}
		return nil
	})
}

// Create 保存证书申请配置，实际 ACME 签发由 Obtain 异步执行。
func (s *SSLService) Create(_ context.Context, req model.WebsiteSSLCreateRequest) (model.WebsiteSSL, error) {
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		return model.WebsiteSSL{}, errors.New("主域名和证书提供商不能为空")
	}
	if provider != "http" && provider != "letsencrypt" && provider != "manual" && provider != "self" && provider != "selfsigned" && provider != "self-signed" && provider != "dnsaccount" && provider != "dnsmanual" && provider != "frommaster" {
		return model.WebsiteSSL{}, fmt.Errorf("不支持的证书提供商 %q", req.Provider)
	}
	var primary string
	var domains []string
	var err error
	if isHTTP01Provider(provider) {
		primary, domains, err = normalizeCertificateDomains(req.PrimaryDomain, req.OtherDomains)
	} else {
		primary, domains, err = normalizeStoredDomains(req.PrimaryDomain, req.OtherDomains)
	}
	if err != nil {
		return model.WebsiteSSL{}, err
	}
	var accountCount int
	repository, repositoryErr := s.sqliteRepository()
	if req.AcmeAccountID != 0 && (repositoryErr != nil || repository.QueryRow(`SELECT COUNT(*) FROM website_acme_accounts WHERE id=?`, req.AcmeAccountID).Scan(&accountCount) != nil || accountCount == 0) {
		return model.WebsiteSSL{}, errors.New("ACME 账户不存在")
	}
	if provider == "dnsaccount" {
		var dnsCount int
		if req.DnsAccountID == 0 || repositoryErr != nil || repository.QueryRow(`SELECT COUNT(*) FROM website_dns_accounts WHERE id=?`, req.DnsAccountID).Scan(&dnsCount) != nil || dnsCount == 0 {
			return model.WebsiteSSL{}, errors.New("DNS 账户不存在")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serial++
	now := time.Now().UTC()
	status := "init"
	if req.AcmeAccountID == 0 {
		status = "pending"
	}
	item := model.WebsiteSSL{ID: s.serial, PrimaryDomain: primary, Domains: strings.Join(domains, ","), Provider: provider, AcmeAccountID: req.AcmeAccountID, DnsAccountID: req.DnsAccountID, AutoRenew: req.AutoRenew, KeyType: req.KeyType, Description: strings.TrimSpace(req.Description), Status: status, Type: "acme", PushDir: req.PushDir, Dir: strings.TrimSpace(req.Dir), SkipDNS: req.SkipDNS, Nameserver1: strings.TrimSpace(req.Nameserver1), Nameserver2: strings.TrimSpace(req.Nameserver2), DisableCNAME: req.DisableCNAME, ExecShell: req.ExecShell, Shell: req.Shell, PushNode: req.PushNode, Nodes: req.Nodes, IsIP: req.IsIP, CreatedAt: now, UpdatedAt: now}
	s.items[item.ID] = item
	if err := s.persistLocked(); err != nil {
		delete(s.items, item.ID)
		return model.WebsiteSSL{}, fmt.Errorf("保存证书配置失败: %w", err)
	}
	logPath := filepath.Join(s.root, "logs", "ssl", fmt.Sprintf("%s-ssl-%d.log", item.PrimaryDomain, item.ID))
	if err := os.MkdirAll(filepath.Dir(logPath), 0o750); err == nil {
		if file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err == nil {
			_ = file.Close()
		}
	}
	return publicSSL(item), nil
}

// Upload 导入 PEM 证书，校验有效期及可选私钥后持久化。
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
	if strings.TrimSpace(req.PrivateKey) != "" {
		if err := validateImportedPrivateKey(certificate, req.PrivateKey, req.Type); err != nil {
			return model.WebsiteSSL{}, err
		}
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
	if err := validateImportedCertificateDomain(strings.ToLower(strings.TrimSuffix(item.PrimaryDomain, "."))); err != nil {
		return model.WebsiteSSL{}, fmt.Errorf("证书主域名无效: %w", err)
	}
	item.PrimaryDomain = strings.ToLower(strings.TrimSuffix(item.PrimaryDomain, "."))
	otherDomains := make([]string, 0, len(certificate.DNSNames))
	for _, domain := range certificate.DNSNames {
		domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
		if domain != "" && domain != item.PrimaryDomain {
			otherDomains = append(otherDomains, domain)
		}
	}
	item.Domains = strings.Join(otherDomains, ",")
	item.Certificate, item.PEM, item.PrivateKey = req.Certificate, req.Certificate, req.PrivateKey
	item.StartDate, item.ExpireDate = certificate.NotBefore, certificate.NotAfter
	item.Status, item.Message, item.Type = "active", "证书已导入", req.Type
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

// Delete 删除指定证书记录。
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

// Update 更新证书申请的域名、提供商、自动续期和描述配置。
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
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = strings.ToLower(strings.TrimSpace(item.Provider))
		if provider == "" {
			return errors.New("证书提供商不能为空")
		}
	}
	if provider == "dnsaccount" && req.DnsAccountID == 0 {
		return errors.New("DNS 账户不能为空")
	}
	repository, repositoryErr := s.sqliteRepository()
	if repositoryErr != nil && (req.AcmeAccountID != 0 || req.DnsAccountID != 0) {
		return errors.New("网站公共数据库未初始化")
	}
	if req.AcmeAccountID != 0 {
		var count int
		if err := repository.QueryRow(`SELECT COUNT(*) FROM website_acme_accounts WHERE id=?`, req.AcmeAccountID).Scan(&count); err != nil || count == 0 {
			return errors.New("ACME 账户不存在")
		}
	}
	if req.DnsAccountID != 0 {
		var count int
		if err := repository.QueryRow(`SELECT COUNT(*) FROM website_dns_accounts WHERE id=?`, req.DnsAccountID).Scan(&count); err != nil || count == 0 {
			return errors.New("DNS 账户不存在")
		}
	}
	if isHTTP01Provider(provider) {
		primary, domains, err := normalizeCertificateDomains(req.PrimaryDomain, req.OtherDomains)
		if err != nil {
			return err
		}
		req.PrimaryDomain, req.OtherDomains = primary, strings.Join(domains, ",")
	}
	item.PrimaryDomain, item.Domains, item.Provider = strings.TrimSpace(req.PrimaryDomain), req.OtherDomains, provider
	item.AutoRenew, item.Description = req.AutoRenew, req.Description
	item.AcmeAccountID, item.DnsAccountID, item.KeyType = req.AcmeAccountID, req.DnsAccountID, req.KeyType
	item.PushDir, item.Dir, item.SkipDNS = req.PushDir, strings.TrimSpace(req.Dir), req.SkipDNS
	item.Nameserver1, item.Nameserver2, item.DisableCNAME = strings.TrimSpace(req.Nameserver1), strings.TrimSpace(req.Nameserver2), req.DisableCNAME
	item.ExecShell, item.Shell, item.PushNode, item.Nodes, item.IsIP = req.ExecShell, req.Shell, req.PushNode, req.Nodes, req.IsIP
	item.UpdatedAt = time.Now().UTC()
	s.items[req.ID] = item
	if err := s.persistLocked(); err != nil {
		return fmt.Errorf("保存证书设置失败: %w", err)
	}
	return nil
}

// RenewCheck 返回临近到期且启用自动续期的证书，不执行外部命令。
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
