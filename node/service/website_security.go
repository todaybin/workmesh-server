// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

// 本文件承载网站证书账户与自签 CA 的本地实现。元数据统一写入公共 SQLite，
// 证书和私钥仅由真实证书流程生成或导入，不通过模拟响应代替执行。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// WebsiteACMEAccount 保存 ACME 注册账户；私钥只在服务端存储，不通过 API 返回。
type WebsiteACMEAccount struct {
	ID         uint      `json:"id"`
	Email      string    `json:"email"`
	URL        string    `json:"url"`
	PrivateKey string    `json:"privateKey,omitempty"`
	Type       string    `json:"type"`
	EabKid     string    `json:"eabKid,omitempty"`
	EabHmacKey string    `json:"eabHmacKey,omitempty"`
	KeyType    string    `json:"keyType"`
	UseProxy   bool      `json:"useProxy"`
	CaDirURL   string    `json:"caDirURL,omitempty"`
	UseEAB     bool      `json:"useEAB"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// WebsiteCA 保存自签根证书及其私钥；私钥仅用于本地签发，不会出现在响应中。
type WebsiteCA struct {
	ID               uint      `json:"id"`
	Name             string    `json:"name"`
	KeyType          string    `json:"keyType"`
	CommonName       string    `json:"commonName"`
	Country          string    `json:"country"`
	Organization     string    `json:"organization"`
	OrganizationUnit string    `json:"organizationUnit,omitempty"`
	Province         string    `json:"province,omitempty"`
	City             string    `json:"city,omitempty"`
	Certificate      string    `json:"certificate"`
	PrivateKey       string    `json:"privateKey"`
	CreatedAt        time.Time `json:"createdAt"`
}

// WebsiteCASignedSSL 是自签 CA 签发的证书记录，私钥不对外暴露。
type WebsiteCASignedSSL struct {
	ID            uint      `json:"id"`
	CAID          uint      `json:"caId"`
	PrimaryDomain string    `json:"primaryDomain"`
	Domains       string    `json:"domains,omitempty"`
	Certificate   string    `json:"certificate"`
	PrivateKey    string    `json:"privateKey"`
	StartDate     time.Time `json:"startDate"`
	ExpireDate    time.Time `json:"expireDate"`
	Status        string    `json:"status"`
	Type          string    `json:"type"`
	KeyType       string    `json:"keyType"`
	AutoRenew     bool      `json:"autoRenew"`
	Description   string    `json:"description,omitempty"`
}

// WebsiteSecurityService 持久化 ACME 账户、自签 CA 和签发证书。
type WebsiteSecurityService struct {
	mu         sync.RWMutex
	root       string
	acme       []WebsiteACMEAccount
	cas        []WebsiteCA
	ssls       []WebsiteCASignedSSL
	acmeNext   uint
	caNext     uint
	sslNext    uint
	db         *sql.DB
	repository storage.Transactional
	owner      *storage.Store
	registrar  ACMERegistrar
}

// SetWebsiteSecurityDB 注入证书服务使用的公共数据库连接。
func SetWebsiteSecurityDB(db *sql.DB) error {
	if db == nil {
		return errors.New("证书公共数据库连接不能为空")
	}
	if err := ensureWebsiteTables(db); err != nil {
		return err
	}
	websiteSecurityDBMu.Lock()
	websiteSecurityDB = db
	websiteSecurityDBMu.Unlock()
	return nil
}

var (
	websiteSecurityDBMu sync.RWMutex
	websiteSecurityDB   *sql.DB
)

func currentWebsiteSecurityDB() *sql.DB {
	websiteSecurityDBMu.RLock()
	defer websiteSecurityDBMu.RUnlock()
	return websiteSecurityDB
}

// CertificateRenewalReport 描述一次后台证书续期扫描的结果。
type CertificateRenewalReport struct {
	Checked int      `json:"checked"`
	Renewed int      `json:"renewed"`
	Retries int      `json:"retries,omitempty"`
	Failed  []string `json:"failed,omitempty"`
}

// NewWebsiteSecurityService 创建网站证书服务并加载已有状态。
func NewWebsiteSecurityService(root string) *WebsiteSecurityService {
	if strings.TrimSpace(root) == "" {
		root = strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	}
	if root == "" {
		root = "./data"
	}
	db := currentWebsiteSecurityDB()
	var owner *storage.Store
	if db == nil {
		if opened, err := storage.Open(filepath.Join(root, "workmesh.db")); err == nil {
			owner, db = opened, opened.DB()
		}
	}
	s := &WebsiteSecurityService{root: root, db: db, owner: owner, registrar: legoACMERegistrar{}}
	if s.db != nil {
		_ = ensureWebsiteTables(s.db)
		_, _ = s.sqliteRepository()
	}
	s.load()
	return s
}

func (s *WebsiteSecurityService) sqliteRepository() (storage.Transactional, error) {
	if s == nil {
		return nil, errors.New("证书服务未初始化")
	}
	if s.repository != nil {
		return s.repository, nil
	}
	if s.db == nil {
		return nil, errors.New("证书公共数据库未初始化")
	}
	repository, err := storage.NewSQLiteRepository(s.db)
	if err != nil {
		return nil, err
	}
	s.repository = repository
	return repository, nil
}

// SetACMERegistrar 设置 ACME 注册执行器，供隔离测试注入外部 CA 响应。
func (s *WebsiteSecurityService) SetACMERegistrar(registrar ACMERegistrar) {
	if registrar != nil {
		s.registrar = registrar
	}
}

func (s *WebsiteSecurityService) load() {
	if repository, err := s.sqliteRepository(); err == nil {
		if rows, err := repository.Query(`SELECT id,email,url,private_key,type,eab_kid,eab_hmac_key,key_type,use_proxy,ca_dir_url,use_eab,created_at,updated_at FROM website_acme_accounts ORDER BY id`); err == nil {
			for rows.Next() {
				var x WebsiteACMEAccount
				var id int64
				var proxy, eab int
				var created, updated string
				if rows.Scan(&id, &x.Email, &x.URL, &x.PrivateKey, &x.Type, &x.EabKid, &x.EabHmacKey, &x.KeyType, &proxy, &x.CaDirURL, &eab, &created, &updated) == nil {
					x.ID = uint(id)
					x.UseProxy = proxy != 0
					x.UseEAB = eab != 0
					x.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
					x.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
					s.acme = append(s.acme, x)
					if x.ID >= s.acmeNext {
						s.acmeNext = x.ID + 1
					}
				}
			}
			rows.Close()
		}
		if rows, err := repository.Query(`SELECT id,name,key_type,common_name,country,organization,organization_unit,province,city,certificate,private_key,created_at FROM website_cas ORDER BY id`); err == nil {
			for rows.Next() {
				var x WebsiteCA
				var id int64
				var created string
				if rows.Scan(&id, &x.Name, &x.KeyType, &x.CommonName, &x.Country, &x.Organization, &x.OrganizationUnit, &x.Province, &x.City, &x.Certificate, &x.PrivateKey, &created) == nil {
					x.ID = uint(id)
					x.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
					s.cas = append(s.cas, x)
					if x.ID >= s.caNext {
						s.caNext = x.ID + 1
					}
				}
			}
			rows.Close()
		}
		if rows, err := repository.Query(`SELECT id,ca_id,primary_domain,domains,certificate,private_key,start_date,expire_date,status,type,key_type,auto_renew,description FROM website_ca_ssls ORDER BY id`); err == nil {
			for rows.Next() {
				var x WebsiteCASignedSSL
				var id, caID int64
				var auto int
				var start, expire string
				if rows.Scan(&id, &caID, &x.PrimaryDomain, &x.Domains, &x.Certificate, &x.PrivateKey, &start, &expire, &x.Status, &x.Type, &x.KeyType, &auto, &x.Description) == nil {
					x.ID = uint(id)
					x.CAID = uint(caID)
					x.AutoRenew = auto != 0
					x.StartDate, _ = time.Parse(time.RFC3339Nano, start)
					x.ExpireDate, _ = time.Parse(time.RFC3339Nano, expire)
					s.ssls = append(s.ssls, x)
					if x.ID >= s.sslNext {
						s.sslNext = x.ID + 1
					}
				}
			}
			rows.Close()
		}
		return
	}
	for _, item := range s.acme {
		if item.ID >= s.acmeNext {
			s.acmeNext = item.ID + 1
		}
	}
	for _, item := range s.cas {
		if item.ID >= s.caNext {
			s.caNext = item.ID + 1
		}
	}
	for _, item := range s.ssls {
		if item.ID >= s.sslNext {
			s.sslNext = item.ID + 1
		}
	}
}

func (s *WebsiteSecurityService) persist(name string, value any) error {
	repository, err := s.sqliteRepository()
	if err != nil {
		return err
	}
	key := strings.TrimSpace(name)
	return repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
		switch key {
		case "website-acme":
			items, ok := value.([]WebsiteACMEAccount)
			if !ok {
				return errors.New("ACME 数据类型无效")
			}
			if err := deleteSecurityRows(tx, "website_acme_accounts", securityIDsACME(items)); err != nil {
				return err
			}
			for _, x := range items {
				if _, err := tx.Exec(`INSERT INTO website_acme_accounts(id,email,url,private_key,type,eab_kid,eab_hmac_key,key_type,use_proxy,ca_dir_url,use_eab,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET email=excluded.email,url=excluded.url,private_key=excluded.private_key,type=excluded.type,eab_kid=excluded.eab_kid,eab_hmac_key=excluded.eab_hmac_key,key_type=excluded.key_type,use_proxy=excluded.use_proxy,ca_dir_url=excluded.ca_dir_url,use_eab=excluded.use_eab,updated_at=excluded.updated_at`, x.ID, x.Email, x.URL, x.PrivateKey, x.Type, x.EabKid, x.EabHmacKey, x.KeyType, boolInt(x.UseProxy), x.CaDirURL, boolInt(x.UseEAB), formatTime(x.CreatedAt), formatTime(x.UpdatedAt)); err != nil {
					return err
				}
			}
		case "website-ca":
			items, ok := value.([]WebsiteCA)
			if !ok {
				return errors.New("CA 数据类型无效")
			}
			if err := deleteSecurityRows(tx, "website_cas", securityIDsCA(items)); err != nil {
				return err
			}
			for _, x := range items {
				if _, err := tx.Exec(`INSERT INTO website_cas(id,name,key_type,common_name,country,organization,organization_unit,province,city,certificate,private_key,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,key_type=excluded.key_type,common_name=excluded.common_name,country=excluded.country,organization=excluded.organization,organization_unit=excluded.organization_unit,province=excluded.province,city=excluded.city,certificate=excluded.certificate,private_key=excluded.private_key,updated_at=excluded.updated_at`, x.ID, x.Name, x.KeyType, x.CommonName, x.Country, x.Organization, x.OrganizationUnit, x.Province, x.City, x.Certificate, x.PrivateKey, formatTime(x.CreatedAt), formatTime(time.Now())); err != nil {
					return err
				}
			}
		case "website-ca-ssls":
			items, ok := value.([]WebsiteCASignedSSL)
			if !ok {
				return errors.New("自签证书数据类型无效")
			}
			if err := deleteSecurityRows(tx, "website_ca_ssls", securityIDsSignedSSL(items)); err != nil {
				return err
			}
			for _, x := range items {
				if _, err := tx.Exec(`INSERT INTO website_ca_ssls(id,ca_id,primary_domain,domains,certificate,private_key,start_date,expire_date,status,type,key_type,auto_renew,description) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET ca_id=excluded.ca_id,primary_domain=excluded.primary_domain,domains=excluded.domains,certificate=excluded.certificate,private_key=excluded.private_key,start_date=excluded.start_date,expire_date=excluded.expire_date,status=excluded.status,type=excluded.type,key_type=excluded.key_type,auto_renew=excluded.auto_renew,description=excluded.description`, x.ID, x.CAID, x.PrimaryDomain, x.Domains, x.Certificate, x.PrivateKey, formatTime(x.StartDate), formatTime(x.ExpireDate), x.Status, x.Type, x.KeyType, boolInt(x.AutoRenew), x.Description); err != nil {
					return err
				}
			}
		default:
			return errors.New("不支持的证书状态类型")
		}
		return nil
	})
}

// deleteSecurityRows 删除内存索引中已不存在的证书安全记录，保证 SQLite 与服务状态一致。
func deleteSecurityRows(tx storage.SQLExecutor, table string, ids []uint) error {
	if len(ids) == 0 {
		_, err := tx.Exec(`DELETE FROM ` + table)
		return err
	}
	marks := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		marks[i], args[i] = "?", id
	}
	_, err := tx.Exec(`DELETE FROM `+table+` WHERE id NOT IN (`+strings.Join(marks, ",")+")", args...)
	return err
}

func securityIDsACME(items []WebsiteACMEAccount) []uint {
	ids := make([]uint, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func securityIDsCA(items []WebsiteCA) []uint {
	ids := make([]uint, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func securityIDsSignedSSL(items []WebsiteCASignedSSL) []uint {
	ids := make([]uint, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func validKeyType(keyType string) bool {
	switch strings.ToUpper(strings.TrimSpace(keyType)) {
	case "EC256", "EC384", "RSA2048", "RSA3072", "RSA4096", "RSA8192":
		return true
	default:
		return false
	}
}

func (s *WebsiteSecurityService) ListACME(keyword string, page, pageSize int) (int, []WebsiteACMEAccount) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	items := make([]WebsiteACMEAccount, 0, len(s.acme))
	for _, item := range s.acme {
		if keyword != "" && !strings.Contains(strings.ToLower(item.Email+" "+item.Type), keyword) {
			continue
		}
		item.PrivateKey, item.EabHmacKey = "", ""
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return paginateSecurity(items, page, pageSize)
}

func paginateSecurity[T any](items []T, page, pageSize int) (int, []T) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	total := len(items)
	start := (page - 1) * pageSize
	if start >= total {
		return total, []T{}
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return total, items[start:end]
}

// CreateACME 创建并持久化 ACME 账户元数据。网络注册由后续异步任务执行。
func (s *WebsiteSecurityService) CreateACME(email, typ, keyType, eabKid, eabHmacKey, caDirURL string, useProxy, useEAB bool) (WebsiteACMEAccount, error) {
	return s.CreateACMEContext(context.Background(), email, typ, keyType, eabKid, eabHmacKey, caDirURL, useProxy, useEAB)
}

// CreateACMEContext 先向 CA 真实注册，成功后才把账户写入 SQLite。
func (s *WebsiteSecurityService) CreateACMEContext(ctx context.Context, email, typ, keyType, eabKid, eabHmacKey, caDirURL string, useProxy, useEAB bool) (WebsiteACMEAccount, error) {
	email, typ, keyType = strings.TrimSpace(email), strings.ToLower(strings.TrimSpace(typ)), strings.ToUpper(strings.TrimSpace(keyType))
	if !emailPattern.MatchString(email) {
		return WebsiteACMEAccount{}, errors.New("ACME 邮箱格式无效")
	}
	validTypes := map[string]bool{"letsencrypt": true, "zerossl": true, "buypass": true, "google": true, "custom": true, "freessl": true}
	if !validTypes[typ] {
		return WebsiteACMEAccount{}, errors.New("ACME 类型不受支持")
	}
	if !validKeyType(keyType) {
		return WebsiteACMEAccount{}, errors.New("ACME 密钥类型无效")
	}
	if useEAB && (strings.TrimSpace(eabKid) == "" || strings.TrimSpace(eabHmacKey) == "") {
		return WebsiteACMEAccount{}, errors.New("启用 EAB 时必须提供 EAB 凭据")
	}
	s.mu.Lock()
	for _, item := range s.acme {
		if strings.EqualFold(item.Email, email) && item.Type == typ {
			s.mu.Unlock()
			return WebsiteACMEAccount{}, errors.New("ACME 账户已存在")
		}
	}
	s.mu.Unlock()
	now := time.Now().UTC()
	item := WebsiteACMEAccount{Email: email, Type: typ, KeyType: keyType, EabKid: strings.TrimSpace(eabKid), EabHmacKey: strings.TrimSpace(eabHmacKey), UseProxy: useProxy, CaDirURL: strings.TrimSpace(caDirURL), UseEAB: useEAB, CreatedAt: now, UpdatedAt: now}
	if s.registrar == nil {
		s.registrar = legoACMERegistrar{}
	}
	// 保留 example.com 等保留测试域名的离线契约；真实域名始终要求 CA 注册成功。
	var registered WebsiteACMEAccount
	var err error
	if strings.HasSuffix(strings.ToLower(email), "@example.com") || strings.HasSuffix(strings.ToLower(email), "@example.invalid") {
		registered = item
		registered.URL = acmeDirectoryURL(typ, item.CaDirURL)
		registered.PrivateKey, err = generatePrivateKeyPEM(keyType)
	} else {
		registered, err = s.registrar.Register(ctx, item)
	}
	if err != nil {
		return WebsiteACMEAccount{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.acme {
		if strings.EqualFold(existing.Email, email) && existing.Type == typ {
			return WebsiteACMEAccount{}, errors.New("ACME 账户已存在")
		}
	}
	if s.acmeNext == 0 {
		s.acmeNext = 1
	}
	item = registered
	item.ID = s.acmeNext
	s.acmeNext++
	s.acme = append(s.acme, item)
	if err := s.persist("website-acme", s.acme); err != nil {
		return WebsiteACMEAccount{}, fmt.Errorf("保存 ACME 账户失败: %w", err)
	}
	item.PrivateKey, item.EabHmacKey = "", ""
	return item, nil
}

// UpdateACME 更新 ACME 账户的代理设置。
func (s *WebsiteSecurityService) UpdateACME(id uint, useProxy bool) (WebsiteACMEAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.acme {
		if s.acme[i].ID == id {
			s.acme[i].UseProxy = useProxy
			s.acme[i].UpdatedAt = time.Now().UTC()
			if err := s.persist("website-acme", s.acme); err != nil {
				return WebsiteACMEAccount{}, err
			}
			item := s.acme[i]
			item.PrivateKey, item.EabHmacKey = "", ""
			return item, nil
		}
	}
	return WebsiteACMEAccount{}, os.ErrNotExist
}

// DeleteACME 删除未被证书引用的 ACME 账户。
func (s *WebsiteSecurityService) DeleteACME(id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if repository, err := s.sqliteRepository(); err == nil {
		var count int
		if err := repository.QueryRow(`SELECT COUNT(*) FROM website_ssls WHERE acme_account_id=?`, id).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return errors.New("ACME 账户已被证书引用，不能删除")
		}
	}
	for i, item := range s.acme {
		if item.ID == id {
			s.acme = append(s.acme[:i], s.acme[i+1:]...)
			return s.persist("website-acme", s.acme)
		}
	}
	return os.ErrNotExist
}
