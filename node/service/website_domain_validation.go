// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"errors"
	"net"
	"regexp"
	"strings"
)

var websiteDomainLabelPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// normalizeWebsiteDomain 校验并规范化站点域名，允许 DNS 通配符和 IPv4，
// 但拒绝路径、控制字符、空标签以及会破坏 Nginx 配置的字符。
func normalizeWebsiteDomain(raw string) (string, error) {
	domain := strings.TrimSpace(raw)
	if domain == "" || len(domain) > 253 || strings.ContainsAny(domain, " \t\r\n/\\;{}\x00") {
		return "", errors.New("网站域名无效")
	}
	if strings.HasPrefix(domain, "*.") {
		domain = strings.TrimPrefix(domain, "*.")
		if domain == "" {
			return "", errors.New("网站域名无效")
		}
	}
	if strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return "", errors.New("网站域名无效")
	}
	if net.ParseIP(domain) != nil {
		return domain, nil
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) > 63 || !websiteDomainLabelPattern.MatchString(label) {
			return "", errors.New("网站域名无效")
		}
	}
	if strings.HasPrefix(strings.TrimSpace(raw), "*.") {
		return "*." + strings.ToLower(domain), nil
	}
	return strings.ToLower(domain), nil
}
