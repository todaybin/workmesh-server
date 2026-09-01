// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/json"
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
	path   string
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
	s := &SSLService{root: root, path: filepath.Join(root, "ssl.json"), items: make(map[uint]model.WebsiteSSL), db: currentWebsiteDB()}
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

// sslPersisted 保存证书元数据及私钥。私钥只存在本地状态文件，不通过 API 返回。
type sslPersisted struct {
	Item       model.WebsiteSSL `json:"item"`
	PrivateKey string           `json:"privateKey,omitempty"`
}

type sslState struct {
	Serial uint           `json:"serial"`
	Items  []sslPersisted `json:"items"`
}

func (s *SSLService) load() {
	var b []byte
	if s.db != nil {
		_ = s.db.QueryRow("SELECT payload FROM website_state WHERE state_key='ssl-certificates'").Scan(&b)
	} else {
		return
	}
	if len(b) == 0 {
		return
	}
	var state sslState
	if json.Unmarshal(b, &state) != nil {
		return
	}
	s.serial = state.Serial
	for _, value := range state.Items {
		value.Item.PrivateKey = value.PrivateKey
		s.items[value.Item.ID] = value.Item
		if value.Item.ID > s.serial {
			s.serial = value.Item.ID
		}
	}
}

func (s *SSLService) persistLocked() error {
	if s.db == nil {
		return errors.New("网站公共数据库未初始化")
	}
	state := sslState{Serial: s.serial, Items: make([]sslPersisted, 0, len(s.items))}
	for _, item := range s.items {
		privateKey := item.PrivateKey
		item.PrivateKey = ""
		state.Items = append(state.Items, sslPersisted{Item: item, PrivateKey: privateKey})
	}
	sort.Slice(state.Items, func(i, j int) bool { return state.Items[i].Item.ID < state.Items[j].Item.ID })
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO website_state(state_key,payload,updated_at) VALUES('ssl-certificates',?,?) ON CONFLICT(state_key) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`, b, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// Create 保存证书申请配置，证书申请由 ACME 适配器异步完成。
func (s *SSLService) Create(_ context.Context, req model.WebsiteSSLCreateRequest) (model.WebsiteSSL, error) {
	if strings.TrimSpace(req.PrimaryDomain) == "" || strings.TrimSpace(req.Provider) == "" {
		return model.WebsiteSSL{}, errors.New("主域名和证书提供商不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serial++
	item := model.WebsiteSSL{ID: s.serial, PrimaryDomain: req.PrimaryDomain, Domains: req.OtherDomains, Provider: req.Provider, AcmeAccountID: req.AcmeAccountID, DnsAccountID: req.DnsAccountID, AutoRenew: req.AutoRenew, KeyType: req.KeyType, Description: req.Description, Status: "pending"}
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
	item.PrivateKey = req.PrivateKey
	item.StartDate = certificate.NotBefore
	item.ExpireDate = certificate.NotAfter
	item.Status = "active"
	item.Message = "证书已导入"
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
	item.PrimaryDomain, item.Domains, item.Provider = req.PrimaryDomain, req.OtherDomains, req.Provider
	item.AutoRenew, item.Description = req.AutoRenew, req.Description
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
