// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strings"
)

// certbotKeyArgs 将面板选择的密钥算法转换为 certbot 参数。
func certbotKeyArgs(keyType string) []string {
	switch strings.ToUpper(strings.TrimSpace(keyType)) {
	case "EC256":
		return []string{"--key-type", "ecdsa", "--elliptic-curve", "secp256r1"}
	case "EC384":
		return []string{"--key-type", "ecdsa", "--elliptic-curve", "secp384r1"}
	case "RSA2048", "RSA3072", "RSA4096", "RSA8192":
		return []string{"--key-type", "rsa", "--rsa-key-size", strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(keyType)), "RSA")}
	default:
		return nil
	}
}

// parseCertificate 解析 certbot 或上传内容中的第一张 PEM 证书。
func parseCertificate(pemData []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemData)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("certbot 返回的证书格式无效")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("解析 certbot 证书失败: %w", err)
	}
	return cert, nil
}

// normalizeCertificateDomains 规范化 HTTP-01 域名并拒绝通配符、IP 和路径。
func normalizeCertificateDomains(primary, others string) (string, []string, error) {
	parts := make([]string, 0, 1)
	parts = append(parts, primary)
	parts = append(parts, strings.FieldsFunc(others, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' ' })...)
	seen := make(map[string]struct{}, len(parts))
	clean := make([]string, 0, len(parts))
	for _, raw := range parts {
		domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
		if err := validateCertificateDomain(domain); err != nil {
			return "", nil, err
		}
		if _, exists := seen[domain]; exists {
			continue
		}
		seen[domain] = struct{}{}
		clean = append(clean, domain)
	}
	if len(clean) == 0 {
		return "", nil, errors.New("主域名不能为空")
	}
	return clean[0], clean[1:], nil
}

// normalizeStoredDomains 规范化非 HTTP-01 证书配置，允许单级通配符。
func normalizeStoredDomains(primary, others string) (string, []string, error) {
	parts := append([]string{primary}, strings.FieldsFunc(others, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' ' })...)
	seen := make(map[string]struct{}, len(parts))
	clean := make([]string, 0, len(parts))
	for _, raw := range parts {
		domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
		if err := validateImportedCertificateDomain(domain); err != nil {
			return "", nil, err
		}
		if _, exists := seen[domain]; exists {
			continue
		}
		seen[domain] = struct{}{}
		clean = append(clean, domain)
	}
	if len(clean) == 0 {
		return "", nil, errors.New("主域名不能为空")
	}
	return clean[0], clean[1:], nil
}

// validateCertificateDomain 校验证书申请域名的安全字符集。
func validateCertificateDomain(domain string) error {
	if domain == "" || len(domain) > 253 || strings.ContainsAny(domain, "*/\\/:@") || net.ParseIP(domain) != nil {
		return errors.New("HTTP-01 不支持通配符、IP 地址或无效域名")
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("域名 %q 格式无效", domain)
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return fmt.Errorf("域名 %q 包含不支持的字符", domain)
			}
		}
	}
	return nil
}

// validateImportedCertificateDomain 允许导入证书中的单级通配符。
func validateImportedCertificateDomain(domain string) error {
	if strings.HasPrefix(domain, "*.") {
		return validateCertificateDomain(strings.TrimPrefix(domain, "*."))
	}
	return validateCertificateDomain(domain)
}

// validateImportedPrivateKey 校验私钥 PEM 类型并确认其公钥与证书匹配。
func validateImportedPrivateKey(cert *x509.Certificate, keyPEM, typ string) error {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		if strings.TrimSpace(typ) == "" {
			// 兼容历史内部调用方传入的 opaque key；HTTP API 传入 type 时必须是 PEM。
			return nil
		}
		return errors.New("私钥必须是 PEM 格式")
	}
	var parsed any
	var err error
	switch block.Type {
	case "RSA PRIVATE KEY":
		parsed, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		parsed, err = x509.ParseECPrivateKey(block.Bytes)
	case "PRIVATE KEY":
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	default:
		return fmt.Errorf("不支持的私钥 PEM 类型 %q", block.Type)
	}
	if err != nil {
		return fmt.Errorf("解析私钥失败: %w", err)
	}
	var public any
	switch key := parsed.(type) {
	case *rsa.PrivateKey:
		public = &key.PublicKey
	case *ecdsa.PrivateKey:
		public = &key.PublicKey
	case ed25519.PrivateKey:
		public = key.Public()
	default:
		return errors.New("私钥算法不受支持")
	}
	want, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return fmt.Errorf("读取证书公钥失败: %w", err)
	}
	got, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return fmt.Errorf("读取私钥公钥失败: %w", err)
	}
	if !bytes.Equal(want, got) {
		return errors.New("证书与私钥不匹配")
	}
	return nil
}

// certificateCoversDomains 确认签发证书覆盖所有请求域名。
func certificateCoversDomains(cert *x509.Certificate, domains []string) error {
	for _, domain := range domains {
		if err := cert.VerifyHostname(domain); err != nil {
			return fmt.Errorf("签发证书不包含域名 %s: %w", domain, err)
		}
	}
	return nil
}

// isHTTP01Provider 判断证书是否使用本机 webroot 的 HTTP-01 流程。
func isHTTP01Provider(provider string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	return provider == "http" || provider == "letsencrypt"
}
