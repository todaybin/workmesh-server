// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

func TestSSLServiceCreateAndList(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", filepath.Join(".tmp", "ssl-create-test"))
	_ = os.RemoveAll(os.Getenv("WORKMESH_DATA_DIR"))
	defer os.RemoveAll(os.Getenv("WORKMESH_DATA_DIR"))
	service := NewSSLService()
	item, err := service.Create(context.Background(), model.WebsiteSSLCreateRequest{PrimaryDomain: "example.com", Provider: "letsencrypt", AutoRenew: true})
	if err != nil || item.ID == 0 || item.Status != "pending" {
		t.Fatalf("创建证书配置失败: %+v, %v", item, err)
	}
	list := service.List(context.Background(), "example")
	if len(list) != 1 || list[0].PrivateKey != "" {
		t.Fatalf("证书列表异常或泄漏私钥: %+v", list)
	}
}

func TestSSLServiceEnrichesAccountRelationsWithoutSecrets(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	store, err := storage.Open(filepath.Join(root, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetWebsiteDB(store.DB()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { websiteDBMu.Lock(); websiteDB = nil; websiteDBMu.Unlock(); _ = store.Close() })
	svc := NewSSLService()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := svc.db.Exec(`INSERT INTO website_acme_accounts(email,url,private_key,type,key_type,use_proxy,ca_dir_url,use_eab,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, "ssl@example.com", "https://acme.test/acct/1", "secret", "letsencrypt", "RSA2048", 0, "", 0, now, now)
	if err != nil {
		t.Fatalf("create ACME account: %v", err)
	}
	accountID, _ := result.LastInsertId()
	if accountID == 0 {
		t.Fatal("ACME account id is zero")
	}
	var check int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM website_acme_accounts WHERE id=?`, accountID).Scan(&check); err != nil || check != 1 {
		t.Fatalf("ACME row missing: %v count=%d", err, check)
	}
	item, err := svc.Create(context.Background(), model.WebsiteSSLCreateRequest{PrimaryDomain: "relation.example.com", Provider: "http", AcmeAccountID: uint(accountID)})
	if err != nil {
		t.Fatalf("create SSL: %v", err)
	}
	got, err := svc.Get(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("get SSL: %v", err)
	}
	if got.AcmeAccount == nil || got.AcmeAccount.Email != "ssl@example.com" {
		t.Fatalf("ACME relation missing: %+v", got.AcmeAccount)
	}
	if got.PrivateKey != "" || got.AcmeAccount.URL == "" {
		t.Fatalf("SSL response leaked or omitted public fields: %+v", got)
	}
}

func TestSSLServicePersistsPrivateKeyAcrossRestart(t *testing.T) {
	root := filepath.Join(".tmp", "ssl-persistence-test")
	_ = os.RemoveAll(root)
	defer os.RemoveAll(root)
	t.Setenv("WORKMESH_DATA_DIR", root)
	service := NewSSLService()
	item, err := service.Upload(context.Background(), model.WebsiteSSLUploadRequest{Certificate: testCertificatePEM(t), PrivateKey: "PRIVATE-KEY", Description: "persist"})
	if err != nil {
		t.Fatalf("上传证书失败: %v", err)
	}
	if item.PrivateKey != "" {
		t.Fatal("上传响应不得返回私钥")
	}
	reloaded := NewSSLService()
	loaded, err := reloaded.Get(context.Background(), item.ID)
	if err != nil || loaded.Status != "active" || loaded.Description != "persist" {
		t.Fatalf("重启后证书状态未恢复: %+v, %v", loaded, err)
	}
	if loaded.PrivateKey != "" {
		t.Fatal("重启查询不得泄漏私钥")
	}
	if reloaded.items[item.ID].PrivateKey != "PRIVATE-KEY" {
		t.Fatal("私钥未从关系表恢复")
	}
	if _, err := os.Stat(filepath.Join(root, "ssl.json")); !os.IsNotExist(err) {
		t.Fatalf("不应再创建旧 JSON 证书状态文件: %v", err)
	}
}

func testCertificatePEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "persist.example"}, DNSNames: []string{"persist.example"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestSSLServiceRejectsInvalidCertificate(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", filepath.Join(".tmp", "ssl-invalid-test"))
	_ = os.RemoveAll(os.Getenv("WORKMESH_DATA_DIR"))
	defer os.RemoveAll(os.Getenv("WORKMESH_DATA_DIR"))
	service := NewSSLService()
	if _, err := service.Upload(context.Background(), model.WebsiteSSLUploadRequest{Certificate: "not-pem"}); err == nil {
		t.Fatal("无效证书应被拒绝")
	}
}

func TestWebsiteSecurityRenewsDueSelfSignedCertificate(t *testing.T) {
	root := filepath.Join(".tmp", "website-security-renew-test")
	_ = os.RemoveAll(root)
	defer os.RemoveAll(root)
	security := NewWebsiteSecurityService(root)
	ca, err := security.CreateCA("renew-ca", "renew-ca", "CN", "WorkMesh", "", "", "", "RSA2048")
	if err != nil {
		t.Fatalf("创建 CA 失败: %v", err)
	}
	ssl, err := security.ObtainCA(ca.ID, 0, "renew.example.com", "RSA2048", "month", 1, true, "auto")
	if err != nil {
		t.Fatalf("签发证书失败: %v", err)
	}
	report := security.RenewDueCertificates(context.Background(), 90*24*time.Hour)
	if report.Checked != 1 || report.Renewed != 1 || len(report.Failed) != 0 {
		t.Fatalf("续期扫描结果异常: %+v", report)
	}
	renewed, err := security.GetSignedSSL(ssl.ID)
	if err != nil || !renewed.ExpireDate.After(ssl.ExpireDate) {
		t.Fatalf("证书未更新: %+v, %v", renewed, err)
	}
}

func TestWebsiteSecurityRenewRetriesAndReportsFailure(t *testing.T) {
	security := NewWebsiteSecurityService(t.TempDir())
	security.ssls = []WebsiteCASignedSSL{{ID: 99, CAID: 404, Type: "self-signed", AutoRenew: true, ExpireDate: time.Now().Add(24 * time.Hour)}}
	report := security.RenewDueCertificates(context.Background(), 30*24*time.Hour)
	if report.Checked != 1 || report.Renewed != 0 || report.Retries != 2 || len(report.Failed) != 1 {
		t.Fatalf("续期失败重试统计异常: %+v", report)
	}
}
