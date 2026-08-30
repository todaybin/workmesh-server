// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

// 本文件承载网站证书账户与自签 CA 的轻量本地实现。所有状态写入工作目录，
// 使用临时文件加原子 rename，确保单进程部署重启后仍能恢复，不依赖外部数据库。

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
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
	mu       sync.RWMutex
	root     string
	acme     []WebsiteACMEAccount
	cas      []WebsiteCA
	ssls     []WebsiteCASignedSSL
	acmeNext uint
	caNext   uint
	sslNext  uint
}

// CertificateRenewalReport 描述一次后台证书续期扫描的结果。
type CertificateRenewalReport struct {
	Checked int      `json:"checked"`
	Renewed int      `json:"renewed"`
	Failed  []string `json:"failed,omitempty"`
}

// NewWebsiteSecurityService 创建网站证书服务并加载已有状态。
func NewWebsiteSecurityService(root string) *WebsiteSecurityService {
	if strings.TrimSpace(root) == "" {
		root = strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	}
	if root == "" {
		root = ".workmesh-data"
	}
	s := &WebsiteSecurityService{root: root}
	s.load()
	return s
}

func (s *WebsiteSecurityService) load() {
	_ = os.MkdirAll(s.root, 0o750)
	read := func(name string, target any) {
		data, err := os.ReadFile(filepath.Join(s.root, name))
		if err == nil && len(data) > 0 {
			_ = json.Unmarshal(data, target)
		}
	}
	read("website-acme.json", &s.acme)
	read("website-ca.json", &s.cas)
	read("website-ca-ssls.json", &s.ssls)
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
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(s.root, 0o750); err != nil {
		return err
	}
	tmp := filepath.Join(s.root, name+".tmp")
	if err = os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	if err = os.Rename(tmp, filepath.Join(s.root, name)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
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
	key, err := generatePrivateKeyPEM(keyType)
	if err != nil {
		return WebsiteACMEAccount{}, fmt.Errorf("生成 ACME 私钥失败: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.acme {
		if strings.EqualFold(item.Email, email) && item.Type == typ {
			return WebsiteACMEAccount{}, errors.New("ACME 账户已存在")
		}
	}
	if s.acmeNext == 0 {
		s.acmeNext = 1
	}
	now := time.Now().UTC()
	item := WebsiteACMEAccount{ID: s.acmeNext, Email: email, Type: typ, KeyType: keyType, PrivateKey: key, EabKid: strings.TrimSpace(eabKid), EabHmacKey: strings.TrimSpace(eabHmacKey), UseProxy: useProxy, CaDirURL: strings.TrimSpace(caDirURL), UseEAB: useEAB, CreatedAt: now, UpdatedAt: now}
	item.URL = acmeDirectoryURL(typ, item.CaDirURL)
	s.acmeNext++
	s.acme = append(s.acme, item)
	if err := s.persist("website-acme.json", s.acme); err != nil {
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
			if err := s.persist("website-acme.json", s.acme); err != nil {
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
	for _, ssl := range s.ssls {
		_ = ssl /* 自签证书不引用 ACME 账户，保留此边界便于后续扩展。 */
	}
	for i, item := range s.acme {
		if item.ID == id {
			s.acme = append(s.acme[:i], s.acme[i+1:]...)
			return s.persist("website-acme.json", s.acme)
		}
	}
	return os.ErrNotExist
}

// CreateCA 生成 X.509 自签根 CA 并持久化证书和私钥。
func (s *WebsiteSecurityService) CreateCA(name, commonName, country, organization, unit, province, city, keyType string) (WebsiteCA, error) {
	name, commonName, country, organization, keyType = strings.TrimSpace(name), strings.TrimSpace(commonName), strings.TrimSpace(country), strings.TrimSpace(organization), strings.ToUpper(strings.TrimSpace(keyType))
	if name == "" || commonName == "" || country == "" || organization == "" {
		return WebsiteCA{}, errors.New("CA 名称、通用名、国家和组织不能为空")
	}
	if !validKeyType(keyType) {
		return WebsiteCA{}, errors.New("CA 密钥类型无效")
	}
	privateKey, publicKey, privatePEM, err := generateKey(keyType)
	if err != nil {
		return WebsiteCA{}, fmt.Errorf("生成 CA 密钥失败: %w", err)
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(now.UnixNano()), Subject: pkix.Name{CommonName: commonName, Country: []string{country}, Organization: []string{organization}, OrganizationalUnit: nonEmpty(unit), Province: nonEmpty(province), Locality: nonEmpty(city)}, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(10, 0, 0), IsCA: true, BasicConstraintsValid: true, MaxPathLen: 1, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, publicKey, privateKey)
	if err != nil {
		return WebsiteCA{}, fmt.Errorf("签发 CA 证书失败: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.cas {
		if strings.EqualFold(item.Name, name) {
			return WebsiteCA{}, errors.New("CA 名称已存在")
		}
	}
	if s.caNext == 0 {
		s.caNext = 1
	}
	item := WebsiteCA{ID: s.caNext, Name: name, KeyType: keyType, CommonName: commonName, Country: country, Organization: organization, OrganizationUnit: unit, Province: province, City: city, Certificate: string(certPEM), PrivateKey: string(privatePEM), CreatedAt: now}
	s.caNext++
	s.cas = append(s.cas, item)
	if err := s.persist("website-ca.json", s.cas); err != nil {
		return WebsiteCA{}, fmt.Errorf("保存 CA 失败: %w", err)
	}
	return publicCA(item), nil
}

func nonEmpty(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{strings.TrimSpace(value)}
}

func publicCA(item WebsiteCA) WebsiteCA { item.PrivateKey = ""; return item }

// ListCA 分页返回自签 CA，响应中不含私钥。
func (s *WebsiteSecurityService) ListCA(keyword string, page, pageSize int) (int, []WebsiteCA) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	items := make([]WebsiteCA, 0, len(s.cas))
	for _, item := range s.cas {
		if keyword != "" && !strings.Contains(strings.ToLower(item.Name+" "+item.CommonName), keyword) {
			continue
		}
		items = append(items, publicCA(item))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return paginateSecurity(items, page, pageSize)
}

// GetCA 获取单个 CA 详情并从证书主体补充字段。
func (s *WebsiteSecurityService) GetCA(id uint) (WebsiteCA, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.cas {
		if item.ID == id {
			return publicCA(item), nil
		}
	}
	return WebsiteCA{}, os.ErrNotExist
}

// DeleteCA 删除未签发证书引用的 CA。
func (s *WebsiteSecurityService) DeleteCA(id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ssl := range s.ssls {
		if ssl.CAID == id {
			return errors.New("CA 已被证书引用，不能删除")
		}
	}
	for i, item := range s.cas {
		if item.ID == id {
			s.cas = append(s.cas[:i], s.cas[i+1:]...)
			return s.persist("website-ca.json", s.cas)
		}
	}
	return os.ErrNotExist
}

// ObtainCA 按请求生成由根 CA 签名的站点证书；renewID 非零时覆盖原证书记录。
func (s *WebsiteSecurityService) ObtainCA(caID, renewID uint, domains, keyType, unit string, years int, autoRenew bool, description string) (WebsiteCASignedSSL, error) {
	if years < 1 || years > 10 {
		return WebsiteCASignedSSL{}, errors.New("证书有效期必须为 1-10")
	}
	keyType = strings.ToUpper(strings.TrimSpace(keyType))
	if !validKeyType(keyType) {
		return WebsiteCASignedSSL{}, errors.New("证书密钥类型无效")
	}
	parts := splitDomains(domains)
	if len(parts) == 0 {
		return WebsiteCASignedSSL{}, errors.New("证书域名不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var ca *WebsiteCA
	for i := range s.cas {
		if s.cas[i].ID == caID {
			ca = &s.cas[i]
			break
		}
	}
	if ca == nil && renewID != 0 {
		for i := range s.ssls {
			if s.ssls[i].ID == renewID {
				caID = s.ssls[i].CAID
				for j := range s.cas {
					if s.cas[j].ID == caID {
						ca = &s.cas[j]
						break
					}
				}
				break
			}
		}
	}
	if ca == nil {
		return WebsiteCASignedSSL{}, os.ErrNotExist
	}
	rootBlock, _ := pem.Decode([]byte(ca.Certificate))
	keyBlock, _ := pem.Decode([]byte(ca.PrivateKey))
	if rootBlock == nil || keyBlock == nil {
		return WebsiteCASignedSSL{}, errors.New("CA 证书或私钥格式无效")
	}
	rootCert, err := x509.ParseCertificate(rootBlock.Bytes)
	if err != nil {
		return WebsiteCASignedSSL{}, err
	}
	rootKey, err := parsePrivateKey(keyBlock.Bytes)
	if err != nil {
		return WebsiteCASignedSSL{}, err
	}
	_, public, privatePEM, err := generateKey(keyType)
	if err != nil {
		return WebsiteCASignedSSL{}, err
	}
	now := time.Now().UTC()
	notAfter := now.AddDate(years, 0, 0)
	if strings.EqualFold(strings.TrimSpace(unit), "day") {
		notAfter = now.AddDate(0, 0, years)
	} else if strings.EqualFold(strings.TrimSpace(unit), "month") {
		notAfter = now.AddDate(0, years, 0)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(now.UnixNano()), Subject: pkix.Name{CommonName: parts[0]}, DNSNames: filterDNS(parts), NotBefore: now.Add(-time.Minute), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, leaf, rootCert, public, rootKey)
	if err != nil {
		return WebsiteCASignedSSL{}, fmt.Errorf("签发站点证书失败: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	record := WebsiteCASignedSSL{CAID: caID, PrimaryDomain: parts[0], Domains: strings.Join(parts[1:], ","), Certificate: string(certPEM), PrivateKey: string(privatePEM), StartDate: now, ExpireDate: notAfter, Status: "ready", Type: "self-signed", KeyType: keyType, AutoRenew: autoRenew, Description: description}
	if renewID != 0 {
		for i := range s.ssls {
			if s.ssls[i].ID == renewID {
				record.ID = renewID
				s.ssls[i] = record
				if err := s.persist("website-ca-ssls.json", s.ssls); err != nil {
					return WebsiteCASignedSSL{}, err
				}
				record.PrivateKey = ""
				return record, nil
			}
		}
		return WebsiteCASignedSSL{}, os.ErrNotExist
	}
	if s.sslNext == 0 {
		s.sslNext = 1
	}
	record.ID = s.sslNext
	s.sslNext++
	s.ssls = append(s.ssls, record)
	if err := s.persist("website-ca-ssls.json", s.ssls); err != nil {
		return WebsiteCASignedSSL{}, err
	}
	record.PrivateKey = ""
	return record, nil
}

// RenewCA 按已有证书元数据重新签发一年期证书，保持域名和密钥算法不变。
func (s *WebsiteSecurityService) RenewCA(id uint) (WebsiteCASignedSSL, error) {
	s.mu.RLock()
	var existing WebsiteCASignedSSL
	for _, item := range s.ssls {
		if item.ID == id {
			existing = item
			break
		}
	}
	s.mu.RUnlock()
	if existing.ID == 0 {
		return WebsiteCASignedSSL{}, os.ErrNotExist
	}
	return s.ObtainCA(existing.CAID, existing.ID, existing.PrimaryDomain+","+existing.Domains, existing.KeyType, "year", 1, existing.AutoRenew, existing.Description)
}

// RenewDueCertificates 续期即将到期的本地自签证书。
//
// ACME 证书的云端签发需要账户授权和挑战环境，由显式 API 异步处理；后台扫描
// 只处理本服务可安全完成的 self-signed 证书，避免在无凭据时伪造续期成功。
func (s *WebsiteSecurityService) RenewDueCertificates(ctx context.Context, horizon time.Duration) CertificateRenewalReport {
	if horizon <= 0 || horizon > 90*24*time.Hour {
		horizon = 30 * 24 * time.Hour
	}
	deadline := time.Now().UTC().Add(horizon)
	s.mu.RLock()
	ids := make([]uint, 0)
	for _, item := range s.ssls {
		if item.AutoRenew && item.Type == "self-signed" && !item.ExpireDate.IsZero() && item.ExpireDate.Before(deadline) {
			ids = append(ids, item.ID)
		}
	}
	s.mu.RUnlock()
	report := CertificateRenewalReport{Checked: len(ids)}
	for _, id := range ids {
		if ctx != nil {
			select {
			case <-ctx.Done():
				report.Failed = append(report.Failed, fmt.Sprintf("%d: %v", id, ctx.Err()))
				continue
			default:
			}
		}
		if _, err := s.RenewCA(id); err != nil {
			report.Failed = append(report.Failed, fmt.Sprintf("%d: %v", id, err))
			continue
		}
		report.Renewed++
	}
	return report
}

// DownloadCA 返回包含 ca.crt 和 ca.key 的 ZIP 字节，调用方必须限制下载权限。
func (s *WebsiteSecurityService) DownloadCA(id uint) ([]byte, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.cas {
		if item.ID != id {
			continue
		}
		var b bytes.Buffer
		zw := zip.NewWriter(&b)
		f, _ := zw.Create("ca.crt")
		_, _ = f.Write([]byte(item.Certificate))
		f, _ = zw.Create("ca.key")
		_, _ = f.Write([]byte(item.PrivateKey))
		if err := zw.Close(); err != nil {
			return nil, "", err
		}
		return b.Bytes(), item.Name + ".zip", nil
	}
	return nil, "", os.ErrNotExist
}

// GetSignedSSL 获取证书公开详情，不返回私钥。
func (s *WebsiteSecurityService) GetSignedSSL(id uint) (WebsiteCASignedSSL, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.ssls {
		if item.ID == id {
			item.PrivateKey = ""
			return item, nil
		}
	}
	return WebsiteCASignedSSL{}, os.ErrNotExist
}

func splitDomains(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t' })
	out := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, v := range fields {
		v = strings.TrimSpace(v)
		if v != "" && !seen[strings.ToLower(v)] {
			seen[strings.ToLower(v)] = true
			out = append(out, v)
		}
	}
	return out
}
func filterDNS(domains []string) []string {
	out := make([]string, 0, len(domains))
	for _, d := range domains {
		if strings.Contains(d, ":") {
			continue
		}
		out = append(out, d)
	}
	return out
}
func acmeDirectoryURL(typ, custom string) string {
	if typ == "custom" && custom != "" {
		return custom
	}
	switch typ {
	case "zerossl":
		return "https://acme.zerossl.com/v2/DV90"
	case "buypass":
		return "https://api.buypass.com/acme/directory"
	case "google":
		return "https://dv.acme-v02.api.pki.goog/directory"
	default:
		return "https://acme-v02.api.letsencrypt.org/directory"
	}
}

func generatePrivateKeyPEM(keyType string) (string, error) {
	_, _, b, err := generateKey(keyType)
	return string(b), err
}
func generateKey(keyType string) (any, any, []byte, error) {
	var private any
	var err error
	switch strings.ToUpper(keyType) {
	case "EC256":
		private, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "EC384":
		private, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	default:
		bits := 2048
		if strings.HasPrefix(strings.ToUpper(keyType), "RSA") {
			fmt.Sscanf(strings.TrimPrefix(strings.ToUpper(keyType), "RSA"), "%d", &bits)
		}
		if bits > 4096 {
			bits = 4096
		}
		private, err = rsa.GenerateKey(rand.Reader, bits)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	var public any
	var data []byte
	switch k := private.(type) {
	case *ecdsa.PrivateKey:
		public = &k.PublicKey
		data, err = x509.MarshalECPrivateKey(k)
		if err == nil {
			data = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: data})
		}
	case *rsa.PrivateKey:
		public = &k.PublicKey
		data = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
	}
	return private, public, data, err
}
func parsePrivateKey(data []byte) (any, error) {
	if b, _ := pem.Decode(data); b != nil {
		data = b.Bytes
	}
	if key, err := x509.ParsePKCS1PrivateKey(data); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(data); err == nil {
		return key, nil
	}
	return nil, errors.New("私钥格式无效")
}
