package service

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	legoacme "github.com/go-acme/lego/v5/acme"
	"github.com/go-acme/lego/v5/certcrypto"
	"github.com/go-acme/lego/v5/certificate"
	"github.com/go-acme/lego/v5/challenge/dns01"
	"github.com/go-acme/lego/v5/lego"
	mdns "github.com/miekg/dns"
	"github.com/todaybin/workmesh-server/node/model"
	"golang.org/x/crypto/acme"
)

type storedDNSOrder struct {
	client *acme.Client
	order  *acme.Order
}

var dnsOrders sync.Map
var dnsChallengeMu sync.Mutex
var manualOrderMu sync.Mutex

func parseSignerPEM(raw string) (crypto.Signer, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, errors.New("ACME 账户私钥无效")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if signer, ok := key.(crypto.Signer); ok {
			return signer, nil
		}
	}
	return nil, errors.New("ACME 账户私钥类型不支持")
}

func (s *SSLService) loadAcmeForSSL(item model.WebsiteSSL) (WebsiteACMEAccount, error) {
	if item.AcmeAccountID == 0 {
		return WebsiteACMEAccount{}, errors.New("ACME 账户不存在")
	}
	repository, err := s.sqliteRepository()
	if err != nil {
		return WebsiteACMEAccount{}, err
	}
	var a WebsiteACMEAccount
	var proxy, eab int
	var created, updated string
	err = repository.QueryRow(`SELECT id,email,url,private_key,type,eab_kid,eab_hmac_key,key_type,use_proxy,ca_dir_url,use_eab,created_at,updated_at FROM website_acme_accounts WHERE id=?`, item.AcmeAccountID).Scan(&a.ID, &a.Email, &a.URL, &a.PrivateKey, &a.Type, &a.EabKid, &a.EabHmacKey, &a.KeyType, &proxy, &a.CaDirURL, &eab, &created, &updated)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return a, errors.New("ACME 账户不存在")
		}
		return a, err
	}
	a.UseProxy, a.UseEAB = proxy != 0, eab != 0
	return a, nil
}

// LoadACMEForSSL returns the non-secret ACME account metadata needed by the
// manual DNS endpoints. The private key is used internally and never encoded
// in an HTTP response.
func (s *SSLService) LoadACMEForSSL(item model.WebsiteSSL) (WebsiteACMEAccount, error) {
	return s.loadAcmeForSSL(item)
}

func (s *SSLService) obtainDNSAccount(ctx context.Context, id uint, item model.WebsiteSSL, logFile *os.File) error {
	account, err := s.loadAcmeForSSL(item)
	if err != nil {
		return s.finishObtain(id, logFile, err)
	}
	var provider string
	var credentials []byte
	repository, err := s.sqliteRepository()
	if err != nil {
		return s.finishObtain(id, logFile, err)
	}
	if err = repository.QueryRow(`SELECT provider,credentials FROM website_dns_accounts WHERE id=?`, item.DnsAccountID).Scan(&provider, &credentials); err != nil {
		return s.finishObtain(id, logFile, fmt.Errorf("读取 DNS 账户失败: %w", err))
	}
	key, err := parseSignerPEM(account.PrivateKey)
	if err != nil {
		return s.finishObtain(id, logFile, err)
	}
	user := &acmeUser{email: account.Email, key: key, registration: &legoacme.ExtendedAccount{Location: account.URL}}
	cfg := lego.NewConfig(user)
	cfg.CADirURL, cfg.HTTPClient, cfg.UserAgent = acmeDirectoryURL(account.Type, account.CaDirURL), acmeHTTPClient(account.UseProxy), "WorkMesh"
	client, err := lego.NewClient(cfg)
	if err != nil {
		return s.finishObtain(id, logFile, err)
	}
	dnsProvider, err := newDNSProvider(provider, credentials, cfg.HTTPClient)
	if err != nil {
		return s.finishObtain(id, logFile, err)
	}
	unlockDNS := configureDNSChallenge(item)
	defer unlockDNS()
	var challengeOptions []dns01.ChallengeOption
	if item.SkipDNS {
		challengeOptions = append(challengeOptions, dns01.DisableAuthoritativeNssPropagationRequirement())
	}
	if err = client.Challenge.SetDNS01Provider(dnsProvider, challengeOptions...); err != nil {
		return s.finishObtain(id, logFile, err)
	}
	domains, err := sslDomains(item)
	if err != nil {
		return s.finishObtain(id, logFile, err)
	}
	certKey, err := generateCertificateKey(item.KeyType)
	if err != nil {
		return s.finishObtain(id, logFile, err)
	}
	resource, err := client.Certificate.Obtain(ctx, certificate.ObtainRequest{Domains: domains, Bundle: true, PrivateKey: certKey, EnableCommonName: true})
	if err != nil {
		return s.finishObtain(id, logFile, err)
	}
	parsed, parseErr := parseCertificate(resource.Certificate)
	if parseErr != nil {
		return s.finishObtain(id, logFile, parseErr)
	}
	if domainErr := certificateCoversDomains(parsed, domains); domainErr != nil {
		return s.finishObtain(id, logFile, domainErr)
	}
	return s.saveIssuedResource(id, *resource, logFile)
}

// configureDNSChallenge mirrors 1Panel's process-wide lego DNS settings. The
// lego DNS client and CNAME switch are global, so every DNS issuance must hold
// the lock for its complete challenge lifecycle.
func configureDNSChallenge(item model.WebsiteSSL) func() {
	dnsChallengeMu.Lock()
	oldCNAME, hadCNAME := os.LookupEnv("LEGO_DISABLE_CNAME_SUPPORT")
	if item.DisableCNAME {
		_ = os.Setenv("LEGO_DISABLE_CNAME_SUPPORT", "true")
	} else {
		_ = os.Setenv("LEGO_DISABLE_CNAME_SUPPORT", "false")
	}
	previous := dns01.DefaultClient()
	nameservers := make([]string, 0, 2)
	if strings.TrimSpace(item.Nameserver1) != "" {
		nameservers = append(nameservers, strings.TrimSpace(item.Nameserver1))
	}
	if strings.TrimSpace(item.Nameserver2) != "" {
		nameservers = append(nameservers, strings.TrimSpace(item.Nameserver2))
	}
	dns01.SetDefaultClient(dns01.NewClient(&dns01.Options{RecursiveNameservers: nameservers}))
	return func() {
		dns01.SetDefaultClient(previous)
		if hadCNAME {
			_ = os.Setenv("LEGO_DISABLE_CNAME_SUPPORT", oldCNAME)
		} else {
			_ = os.Unsetenv("LEGO_DISABLE_CNAME_SUPPORT")
		}
		dnsChallengeMu.Unlock()
	}
}

func generateCertificateKey(keyType string) (crypto.Signer, error) {
	value, err := certcrypto.GeneratePrivateKey(certcrypto.KeyType(strings.ToUpper(strings.TrimSpace(keyType))))
	if err != nil {
		return nil, fmt.Errorf("生成证书私钥失败: %w", err)
	}
	signer, ok := value.(crypto.Signer)
	if !ok {
		return nil, errors.New("证书私钥类型无效")
	}
	return signer, nil
}

func sslDomains(item model.WebsiteSSL) ([]string, error) {
	primary := strings.TrimSpace(item.PrimaryDomain)
	if primary == "" {
		return nil, errors.New("主域名不能为空")
	}
	result := []string{primary}
	for _, domain := range strings.Split(item.Domains, ",") {
		domain = strings.TrimSpace(domain)
		if domain != "" && !sslContainsString(result, domain) {
			result = append(result, domain)
		}
	}
	return result, nil
}

func sslContainsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (s *SSLService) saveIssuedResource(id uint, resource certificate.Resource, logFile *os.File) error {
	parsed, err := parseCertificate(resource.Certificate)
	if err != nil {
		return s.finishObtain(id, logFile, err)
	}
	keyPEM := string(resource.PrivateKey)
	if keyPEM == "" {
		return s.finishObtain(id, logFile, errors.New("签发结果未包含私钥"))
	}
	s.mu.Lock()
	item, ok := s.items[id]
	if !ok {
		s.mu.Unlock()
		return errors.New("证书不存在")
	}
	item.Certificate, item.PEM, item.PrivateKey = string(resource.Certificate), string(resource.Certificate), keyPEM
	item.StartDate, item.ExpireDate, item.Status, item.Message, item.UpdatedAt = parsed.NotBefore, parsed.NotAfter, "ready", "ACME 证书申请成功", time.Now().UTC()
	s.items[id] = item
	err = s.persistLocked()
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("保存 ACME 证书失败: %w", err)
	}
	if _, syncErr := s.syncIssuedCertificate(id, resource.Certificate, []byte(keyPEM)); syncErr != nil {
		return syncErr
	}
	_, _ = fmt.Fprintln(logFile, "证书申请完成")
	return nil
}

func pemEncodeSigner(key crypto.Signer) string {
	switch value := key.(type) {
	case *rsa.PrivateKey:
		return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(value)}))
	case *ecdsa.PrivateKey:
		der, _ := x509.MarshalECPrivateKey(value)
		return string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}))
	default:
		return ""
	}
}

func (s *SSLService) obtainDNSManual(ctx context.Context, id uint, item model.WebsiteSSL, logFile *os.File) error {
	// Manual DNS is finalized by /ssl/obtain after the UI confirms TXT records.
	return s.finishObtain(id, logFile, errors.New("DNS 手动验证需先调用 /websites/ssl/resolve"))
}

func acmeClientForAccount(account WebsiteACMEAccount) (*acme.Client, error) {
	key, err := parseSignerPEM(account.PrivateKey)
	if err != nil {
		return nil, err
	}
	return &acme.Client{Key: key, DirectoryURL: acmeDirectoryURL(account.Type, account.CaDirURL), HTTPClient: acmeHTTPClient(account.UseProxy), UserAgent: "WorkMesh"}, nil
}

func manualOrder(ctx context.Context, item model.WebsiteSSL, account WebsiteACMEAccount) ([]map[string]string, error) {
	manualOrderMu.Lock()
	defer manualOrderMu.Unlock()
	client, err := acmeClientForAccount(account)
	if err != nil {
		return nil, err
	}
	domains, err := sslDomains(item)
	if err != nil {
		return nil, err
	}
	var order *acme.Order
	if value, ok := dnsOrders.Load(item.ID); ok {
		if stored, valid := value.(storedDNSOrder); valid && stored.order != nil && (stored.order.Expires.IsZero() || stored.order.Expires.After(time.Now())) {
			client, order = stored.client, stored.order
		} else {
			dnsOrders.Delete(item.ID)
		}
	}
	if order == nil {
		order, err = client.AuthorizeOrder(ctx, acme.DomainIDs(domains...))
		if err != nil {
			return nil, err
		}
	}
	values := make([]map[string]string, 0, len(order.AuthzURLs))
	for _, authzURL := range order.AuthzURLs {
		authz, err := client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return nil, err
		}
		var challenge *acme.Challenge
		for i := range authz.Challenges {
			if authz.Challenges[i].Type == "dns-01" {
				challenge = authz.Challenges[i]
				break
			}
		}
		if challenge == nil {
			return nil, fmt.Errorf("域名 %s 没有 DNS-01 challenge", authz.Identifier.Value)
		}
		value, err := client.DNS01ChallengeRecord(challenge.Token)
		if err != nil {
			return nil, err
		}
		values = append(values, map[string]string{"domain": authz.Identifier.Value, "resolve": "_acme-challenge." + authz.Identifier.Value, "value": value, "err": ""})
	}
	dnsOrders.Store(item.ID, storedDNSOrder{client: client, order: order})
	return values, nil
}

// GetDNSManualResolve creates (or reuses) a pending ACME order and exposes the
// exact TXT record values that the frontend must publish.
func (s *SSLService) GetDNSManualResolve(ctx context.Context, item model.WebsiteSSL, account WebsiteACMEAccount) ([]map[string]string, error) {
	return manualOrder(ctx, item, account)
}

// FinalizeDNSManual validates the requested TXT records and finalizes a stored
// ACME order. It intentionally fails when no order was created by /resolve.
func (s *SSLService) FinalizeDNSManual(ctx context.Context, id uint, records map[string]string) (resultErr error) {
	s.mu.RLock()
	item, ok := s.items[id]
	s.mu.RUnlock()
	if !ok {
		return errors.New("证书不存在")
	}
	defer func() {
		if resultErr == nil {
			return
		}
		s.mu.Lock()
		if current, exists := s.items[id]; exists {
			current.Status, current.Message, current.UpdatedAt = "error", resultErr.Error(), time.Now().UTC()
			s.items[id] = current
			_ = s.persistLocked()
		}
		s.mu.Unlock()
	}()
	s.mu.Lock()
	if current, exists := s.items[id]; exists {
		if strings.EqualFold(current.Status, "applying") {
			s.mu.Unlock()
			return errors.New("证书申请正在执行")
		}
		current.Status, current.Message, current.UpdatedAt = "applying", "正在申请 ACME 证书", time.Now().UTC()
		s.items[id] = current
		if persistErr := s.persistLocked(); persistErr != nil {
			s.mu.Unlock()
			return persistErr
		}
	}
	s.mu.Unlock()
	value, ok := dnsOrders.Load(id)
	if !ok {
		return errors.New("DNS 验证订单不存在，请先获取解析记录")
	}
	stored, ok := value.(storedDNSOrder)
	if !ok || stored.client == nil || stored.order == nil {
		return errors.New("DNS 验证订单无效")
	}
	account, err := s.loadAcmeForSSL(item)
	if err != nil {
		return err
	}
	domains, err := sslDomains(item)
	if err != nil {
		return err
	}
	for _, authzURL := range stored.order.AuthzURLs {
		authz, err := stored.client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return err
		}
		var challenge *acme.Challenge
		for i := range authz.Challenges {
			if authz.Challenges[i].Type == "dns-01" {
				challenge = authz.Challenges[i]
				break
			}
		}
		if challenge == nil {
			return fmt.Errorf("域名 %s 没有 DNS-01 challenge", authz.Identifier.Value)
		}
		expected, err := stored.client.DNS01ChallengeRecord(challenge.Token)
		if err != nil {
			return err
		}
		key := authz.Identifier.Value
		provided := records[key]
		if provided == "" {
			provided = records["_acme-challenge."+strings.TrimPrefix(key, "*.")]
		}
		if provided != "" && provided != expected {
			return fmt.Errorf("域名 %s TXT 记录不匹配", key)
		}
		if provided == "" && !txtContains(ctx, key, expected, item.Nameserver1, item.Nameserver2) {
			return fmt.Errorf("域名 %s TXT 记录尚未生效", key)
		}
		if _, err = stored.client.Accept(ctx, challenge); err != nil {
			return err
		}
	}
	order, err := stored.client.WaitOrder(ctx, stored.order.URI)
	if err != nil {
		return err
	}
	certKey, err := generateCertificateKey(item.KeyType)
	if err != nil {
		return err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{DNSNames: domains}, certKey)
	if err != nil {
		return err
	}
	der, _, err := stored.client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return err
	}
	if len(der) == 0 {
		return errors.New("ACME 未返回证书")
	}
	certPEM := make([]byte, 0, len(der)*512)
	for _, block := range der {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: block})...)
	}
	resource := certificate.Resource{Certificate: certPEM, PrivateKey: []byte(pemEncodeSigner(certKey))}
	_ = account
	dnsOrders.Delete(id)
	logPath, logErr := s.LogPath(ctx, id)
	if logErr != nil {
		return logErr
	}
	logFile, logErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if logErr != nil {
		return logErr
	}
	defer logFile.Close()
	return s.saveIssuedResource(id, resource, logFile)
}

func txtContains(ctx context.Context, domain, expected string, nameservers ...string) bool {
	name := "_acme-challenge." + strings.TrimPrefix(domain, "*.")
	if len(nameservers) == 0 || (strings.TrimSpace(nameservers[0]) == "" && strings.TrimSpace(nameservers[1]) == "") {
		values, err := net.DefaultResolver.LookupTXT(ctx, name)
		if err != nil {
			return false
		}
		for _, value := range values {
			if value == expected {
				return true
			}
		}
		return false
	}
	servers := make([]string, 0, len(nameservers))
	for _, nameserver := range nameservers {
		nameserver = strings.TrimSpace(nameserver)
		if nameserver != "" {
			if !strings.Contains(nameserver, ":") {
				nameserver += ":53"
			}
			servers = append(servers, nameserver)
		}
	}
	query := new(mdns.Msg)
	query.SetQuestion(mdns.Fqdn(name), mdns.TypeTXT)
	client := &mdns.Client{Timeout: 10 * time.Second}
	for _, server := range servers {
		message, _, err := client.ExchangeContext(ctx, query, server)
		if err != nil || message == nil {
			continue
		}
		for _, answer := range message.Answer {
			txt, ok := answer.(*mdns.TXT)
			if ok && strings.Join(txt.Txt, "") == expected {
				return true
			}
		}
	}
	return false
}
