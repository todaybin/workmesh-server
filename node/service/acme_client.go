// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	legoacme "github.com/go-acme/lego/v5/acme"
	"github.com/go-acme/lego/v5/certcrypto"
	"github.com/go-acme/lego/v5/lego"
	"github.com/go-acme/lego/v5/registration"
)

const acmeNetworkTimeout = 2 * time.Minute

// ACMERegistrar 将 ACME 账户创建与外部 CA 注册隔离，便于验证失败不落库。
type ACMERegistrar interface {
	Register(context.Context, WebsiteACMEAccount) (WebsiteACMEAccount, error)
}

type legoACMERegistrar struct{}

type acmeUser struct {
	email        string
	registration *legoacme.ExtendedAccount
	key          crypto.Signer
}

func (u *acmeUser) GetEmail() string                           { return u.email }
func (u *acmeUser) GetRegistration() *legoacme.ExtendedAccount { return u.registration }
func (u *acmeUser) GetPrivateKey() crypto.Signer               { return u.key }

// Register 向所选 CA 创建真实账户，只有取得 registration URL 后才返回成功。
func (legoACMERegistrar) Register(ctx context.Context, account WebsiteACMEAccount) (WebsiteACMEAccount, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, acmeNetworkTimeout)
	defer cancel()
	key, err := generateACMEKey(account.KeyType)
	if err != nil {
		return account, err
	}
	user := &acmeUser{email: account.Email, key: key}
	config := lego.NewConfig(user)
	config.CADirURL = acmeDirectoryURL(account.Type, account.CaDirURL)
	config.UserAgent = "1Panel"
	config.HTTPClient = acmeHTTPClient(account.UseProxy)
	config.Certificate.Timeout = time.Minute
	client, err := lego.NewClient(config)
	if err != nil {
		return account, fmt.Errorf("创建 ACME 客户端失败: %w", err)
	}
	if account.Type == "zerossl" && (account.EabKid == "" || account.EabHmacKey == "") {
		account.EabKid, account.EabHmacKey, err = fetchZeroSSLEAB(ctx, account.Email, config.HTTPClient)
		if err != nil {
			return account, err
		}
	}
	var registered *legoacme.ExtendedAccount
	if account.UseEAB || account.Type == "zerossl" || account.Type == "google" || account.Type == "freessl" {
		if account.EabKid == "" || account.EabHmacKey == "" {
			return account, errors.New("ACME CA 要求 EAB 凭据")
		}
		registered, err = client.Registration.RegisterWithExternalAccountBinding(ctx, registration.RegisterEABOptions{TermsOfServiceAgreed: true, Kid: account.EabKid, HmacEncoded: account.EabHmacKey})
	} else {
		registered, err = client.Registration.Register(ctx, registration.RegisterOptions{TermsOfServiceAgreed: true})
	}
	if err != nil {
		return account, fmt.Errorf("注册 ACME 账户失败: %w", err)
	}
	if registered == nil || strings.TrimSpace(registered.Location) == "" {
		return account, errors.New("ACME CA 未返回账户地址")
	}
	privateKey, err := marshalACMEKey(key)
	if err != nil {
		return account, err
	}
	account.URL, account.PrivateKey = registered.Location, privateKey
	return account, nil
}

func generateACMEKey(keyType string) (crypto.Signer, error) {
	key, err := certcrypto.GeneratePrivateKey(certcrypto.KeyType(strings.ToUpper(strings.TrimSpace(keyType))))
	if err != nil {
		return nil, fmt.Errorf("生成 ACME 账户密钥失败: %w", err)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, errors.New("ACME 账户密钥类型无效")
	}
	return signer, nil
}

func marshalACMEKey(key crypto.Signer) (string, error) {
	var block *pem.Block
	switch value := key.(type) {
	case *ecdsa.PrivateKey:
		der, err := x509.MarshalECPrivateKey(value)
		if err != nil {
			return "", err
		}
		block = &pem.Block{Type: "EC PRIVATE KEY", Bytes: der}
	case *rsa.PrivateKey:
		block = &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(value)}
	default:
		return "", errors.New("不支持的 ACME 账户密钥")
	}
	return string(pem.EncodeToMemory(block)), nil
}

func acmeHTTPClient(useProxy bool) *http.Client {
	proxy := func(*http.Request) (*url.URL, error) { return nil, nil }
	if useProxy {
		proxy = http.ProxyFromEnvironment
	}
	return &http.Client{Timeout: acmeNetworkTimeout, Transport: &http.Transport{Proxy: proxy, DialContext: (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 30 * time.Second, ResponseHeaderTimeout: time.Minute, TLSClientConfig: &tls.Config{ServerName: os.Getenv("LEGO_CA_SERVER_NAME"), MinVersion: tls.VersionTLS12}}}
}

func fetchZeroSSLEAB(ctx context.Context, email string, client *http.Client) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.zerossl.com/acme/eab-credentials-email?email="+url.QueryEscape(email), nil)
	if err != nil {
		return "", "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("获取 ZeroSSL EAB 失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("获取 ZeroSSL EAB 失败: HTTP %d", resp.StatusCode)
	}
	var result struct {
		Success bool   `json:"success"`
		Kid     string `json:"eab_kid"`
		HMAC    string `json:"eab_hmac_key"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", err
	}
	if !result.Success || result.Kid == "" || result.HMAC == "" {
		return "", "", errors.New("ZeroSSL 未返回有效 EAB 凭据")
	}
	return result.Kid, result.HMAC, nil
}
