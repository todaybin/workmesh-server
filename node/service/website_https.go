// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

// UpdateHTTPS 保存站点 HTTPS 开关及证书关联，并同步站点协议字段。
func (s *WebsiteService) UpdateHTTPS(id uint, enabled bool, sslID uint, httpConfig string) (model.Website, error) {
	openRestyStatus := s.ProbeOpenResty(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.websites {
		if s.websites[i].ID != id {
			continue
		}
		original := s.websites[i]
		candidate := original
		if enabled {
			if sslID == 0 {
				return model.Website{}, errors.New("启用 HTTPS 必须选择有效证书")
			}
			if err := s.validateHTTPSCertificateLocked(candidate, sslID); err != nil {
				return model.Website{}, err
			}
			candidate.Protocol = "HTTPS"
			candidate.WebsiteSSLID = sslID
		} else {
			candidate.Protocol = "HTTP"
			// 与 1Panel 一致：停用 HTTPS 后解除证书关联，避免重启时再次
			// 把已停用的证书恢复到站点配置。
			candidate.WebsiteSSLID = 0
		}
		if strings.TrimSpace(httpConfig) != "" {
			candidate.HttpConfig = strings.TrimSpace(httpConfig)
		}
		if err := s.ensureSiteLayout(candidate); err != nil {
			return model.Website{}, fmt.Errorf("准备 HTTPS 站点目录失败: %w", err)
		}
		configPath := s.SitePath(candidate, "site.conf")
		certPath := filepath.Join(s.SitePath(candidate, "ssl"), "fullchain.pem")
		keyPath := filepath.Join(s.SitePath(candidate, "ssl"), "privkey.pem")
		oldFiles, snapshotErr := snapshotWebsiteFiles(configPath, certPath, keyPath)
		if snapshotErr != nil {
			return model.Website{}, snapshotErr
		}
		if err := s.updateHTTPSRuntimeFilesLocked(candidate, enabled, sslID); err != nil {
			return model.Website{}, errors.Join(err, restoreWebsiteFiles(oldFiles))
		}
		if err := validateCreatedWebsiteConfig(context.Background(), openRestyStatus); err != nil {
			return model.Website{}, errors.Join(err, restoreWebsiteFiles(oldFiles))
		}
		candidate.UpdatedAt = time.Now().UTC()
		s.websites[i] = candidate
		if err := s.persist("websites", s.websites); err != nil {
			s.websites[i] = original
			return model.Website{}, errors.Join(err, restoreWebsiteFiles(oldFiles))
		}
		return candidate, nil
	}
	return model.Website{}, os.ErrNotExist
}

// validateHTTPSCertificateLocked 确认证书、私钥和站点域名均真实匹配。
// 只有可解析、未过期且覆盖站点 HTTPS 域名的证书才允许写入 443 配置。
func (s *WebsiteService) validateHTTPSCertificateLocked(site model.Website, sslID uint) error {
	repository, repositoryErr := s.sqliteRepository()
	if repositoryErr != nil {
		return errors.New("网站公共数据库未初始化，无法启用 HTTPS")
	}
	var certificate, privateKey string
	if err := repository.QueryRow(`SELECT pem,private_key FROM website_ssls WHERE id=?`, sslID).Scan(&certificate, &privateKey); err != nil {
		return fmt.Errorf("HTTPS 证书不存在: %w", err)
	}
	certificate = strings.TrimSpace(certificate)
	privateKey = strings.TrimSpace(privateKey)
	if certificate == "" || privateKey == "" {
		return errors.New("HTTPS 证书或私钥尚未就绪")
	}
	parsed, err := parseCertificate([]byte(certificate))
	if err != nil {
		return fmt.Errorf("HTTPS 证书无效: %w", err)
	}
	now := time.Now()
	if !parsed.NotBefore.Before(now) || !parsed.NotAfter.After(now) {
		return errors.New("HTTPS 证书尚未生效或已经过期")
	}
	if err := validateImportedPrivateKey(parsed, privateKey, "pem"); err != nil {
		return fmt.Errorf("HTTPS 私钥无效: %w", err)
	}
	domains := []string{strings.TrimSpace(site.PrimaryDomain)}
	for _, domain := range s.domains[site.ID] {
		if domain.SSL && strings.TrimSpace(domain.Domain) != "" {
			domains = append(domains, strings.TrimSpace(domain.Domain))
		}
	}
	if err := certificateCoversDomains(parsed, domains); err != nil {
		return fmt.Errorf("HTTPS 证书域名不匹配: %w", err)
	}
	return nil
}

func (s *WebsiteService) updateHTTPSRuntimeFilesLocked(site model.Website, enabled bool, sslID uint) error {
	path := s.SitePath(site, "site.conf")
	old, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := removeManagedHTTPSBlock(string(old))
	if enabled && sslID != 0 {
		repository, repositoryErr := s.sqliteRepository()
		if repositoryErr != nil {
			return repositoryErr
		}
		var certificate, privateKey string
		if err := repository.QueryRow(`SELECT pem,private_key FROM website_ssls WHERE id=?`, sslID).Scan(&certificate, &privateKey); err != nil {
			return err
		}
		certPath, keyPath := filepath.Join(s.SitePath(site, "ssl"), "fullchain.pem"), filepath.Join(s.SitePath(site, "ssl"), "privkey.pem")
		if err := writeWebsiteAtomic(certPath, []byte(certificate), 0o644); err != nil {
			return fmt.Errorf("写入 HTTPS 证书失败: %w", err)
		}
		if err := writeWebsiteAtomic(keyPath, []byte(privateKey), 0o600); err != nil {
			return fmt.Errorf("写入 HTTPS 私钥失败: %w", err)
		}
		if hasHTTPSListener(content) {
			// 站点配置可能由旧版本或管理员维护了独立的 443 server；
			// 只同步证书文件，不能重复插入 listen 造成 nginx 配置冲突。
		} else {
			open := strings.Index(content, "{")
			if open < 0 {
				return errors.New("网站配置缺少 server 块，无法启用 HTTPS")
			}
			block := managedWebsiteIncludeMarker + " https\n    listen 443 ssl;\n    ssl_certificate " + certPath + ";\n    ssl_certificate_key " + keyPath + ";\n" + managedWebsiteIncludeMarker + " https end\n"
			content = content[:open+1] + "\n" + block + content[open+1:]
		}
	}
	if err := writeWebsiteAtomic(path, []byte(content), 0o640); err != nil {
		_ = writeWebsiteAtomic(path, old, 0o640)
		return err
	}
	return nil
}

// hasHTTPSListener 判断站点配置是否已有 HTTPS 监听，兼容 IPv4/IPv6 写法。
func hasHTTPSListener(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "listen 443 ssl") || strings.HasPrefix(trim, "listen [::]:443 ssl") {
			return true
		}
	}
	return false
}

// removeManagedHTTPSBlock 只移除本服务生成的 HTTPS 片段，不破坏用户手写的
// listen/证书指令，避免反复保存 HTTPS 时覆盖站点自定义配置。
func removeManagedHTTPSBlock(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	inManaged := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == managedWebsiteIncludeMarker+" https" {
			inManaged = true
			continue
		}
		if inManaged {
			if trim == managedWebsiteIncludeMarker+" https end" {
				inManaged = false
				continue
			}
			if strings.HasPrefix(trim, "listen 443 ssl") || strings.HasPrefix(trim, "listen 443;") || strings.HasPrefix(trim, "ssl_certificate ") || strings.HasPrefix(trim, "ssl_certificate_key ") {
				continue
			}
			inManaged = false
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
