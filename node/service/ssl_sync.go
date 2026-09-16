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

// sslFileBackup 保存同步前的证书文件内容，用于 reload 失败时恢复工作版本。
type sslFileBackup struct {
	path   string
	data   []byte
	mode   os.FileMode
	exists bool
}

// syncIssuedCertificate 将已签发证书同步到所有引用该证书的网站，并
// 在 OpenResty 可探测时执行受控 reload。目标路径必须位于网站根目录内。
func (s *SSLService) syncIssuedCertificate(id uint, certificate, privateKey []byte) (int, error) {
	repository, err := s.sqliteRepository()
	if err != nil {
		return 0, err
	}
	s.mu.RLock()
	_, exists := s.items[id]
	s.mu.RUnlock()
	if !exists {
		return 0, errors.New("证书不存在")
	}
	websiteRoot := strings.TrimSpace(os.Getenv("WORKMESH_WEBSITE_ROOT"))
	if websiteRoot == "" {
		websiteRoot = "/www/wwwroot"
	}
	websiteRoot = filepath.Clean(websiteRoot)
	type target struct{ certPath, keyPath string }
	targets := make([]target, 0, 2)
	rows, err := repository.Query(`SELECT site_dir,primary_domain FROM websites WHERE website_ssl_id=?`, id)
	if err != nil {
		return 0, fmt.Errorf("查询 HTTPS 站点失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var siteDir, domain string
		if err := rows.Scan(&siteDir, &domain); err != nil {
			return 0, fmt.Errorf("读取 HTTPS 站点失败: %w", err)
		}
		siteDir = strings.TrimSpace(siteDir)
		if siteDir == "" {
			domain = strings.TrimSpace(domain)
			if domain == "" || filepath.Base(domain) != domain || strings.ContainsAny(domain, `/\`) {
				return 0, errors.New("HTTPS 站点目录无效")
			}
			siteDir = filepath.Join(websiteRoot, domain)
		}
		siteDir = filepath.Clean(siteDir)
		if !withinPath(websiteRoot, siteDir) || siteDir == websiteRoot {
			return 0, fmt.Errorf("HTTPS 站点目录越界: %s", siteDir)
		}
		info, statErr := os.Lstat(siteDir)
		if statErr != nil {
			return 0, fmt.Errorf("HTTPS 站点目录不可用: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return 0, errors.New("HTTPS 站点目录不能是符号链接")
		}
		sslDir := filepath.Join(siteDir, "ssl")
		sslInfo, sslErr := os.Lstat(sslDir)
		if errors.Is(sslErr, os.ErrNotExist) {
			if err := os.Mkdir(sslDir, 0o750); err != nil {
				return 0, fmt.Errorf("创建 HTTPS 证书目录失败: %w", err)
			}
		} else if sslErr != nil {
			return 0, fmt.Errorf("检查 HTTPS 证书目录失败: %w", sslErr)
		} else if sslInfo.Mode()&os.ModeSymlink != 0 || !sslInfo.IsDir() {
			return 0, errors.New("HTTPS 证书目录不能是符号链接")
		}
		targets = append(targets, target{certPath: filepath.Join(sslDir, "fullchain.pem"), keyPath: filepath.Join(sslDir, "privkey.pem")})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	backups := make([]sslFileBackup, 0, len(targets)*2)
	for _, target := range targets {
		certBackup, err := readSSLFileBackup(target.certPath)
		if err != nil {
			return 0, fmt.Errorf("备份 HTTPS 证书失败: %w", err)
		}
		keyBackup, err := readSSLFileBackup(target.keyPath)
		if err != nil {
			return 0, fmt.Errorf("备份 HTTPS 私钥失败: %w", err)
		}
		backups = append(backups, certBackup, keyBackup)
	}
	for index, target := range targets {
		if err := writeWebsiteAtomic(target.certPath, certificate, 0o644); err != nil {
			return 0, combineSSLSyncErrors(fmt.Errorf("写入 HTTPS 证书失败: %w", err), rollbackSSLFiles(backups))
		}
		if err := writeWebsiteAtomic(target.keyPath, privateKey, 0o600); err != nil {
			return 0, combineSSLSyncErrors(fmt.Errorf("写入 HTTPS 私钥失败: %w", err), rollbackSSLFiles(backups[:(index+1)*2]))
		}
	}
	if len(targets) == 0 {
		return 0, nil
	}
	runtime := &WebsiteService{root: s.root, websiteRoot: websiteRoot}
	reloadCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := runtime.OperateOpenResty(reloadCtx, "reload"); err != nil {
		rollbackErr := rollbackSSLFiles(backups)
		// 文件已恢复后再次 reload，尽可能让 worker 回到旧证书；若
		// reload 本身因配置错误失败，错误会被保留供 SQLite 记录。
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 15*time.Second)
		_, restoreReloadErr := runtime.OperateOpenResty(restoreCtx, "reload")
		restoreCancel()
		return len(targets), combineSSLSyncErrors(fmt.Errorf("OpenResty reload 失败: %w", err), rollbackErr, restoreReloadErr)
	}
	return len(targets), nil
}

// RetryPendingSync 重试此前签发成功但站点文件或 OpenResty reload 失败的证书。
// 它由节点级后台扫描器调用，避免一次短暂 reload 故障让证书长期无法生效。
func (s *SSLService) RetryPendingSync(ctx context.Context) (attempted, failed int) {
	s.mu.RLock()
	pending := make([]model.WebsiteSSL, 0)
	for _, item := range s.items {
		if strings.EqualFold(item.Status, "ready") && strings.Contains(item.Message, "配置同步失败") && strings.TrimSpace(item.Certificate) != "" && strings.TrimSpace(item.PrivateKey) != "" {
			pending = append(pending, item)
		}
	}
	s.mu.RUnlock()
	for _, item := range pending {
		attempted++
		if _, err := s.syncIssuedCertificate(item.ID, []byte(item.Certificate), []byte(item.PrivateKey)); err != nil {
			failed++
			s.updateReadyMessage(item.ID, "ACME 证书申请成功，但站点 HTTPS 配置同步失败: "+err.Error())
			continue
		}
		s.updateReadyMessage(item.ID, "ACME 证书申请成功，站点 HTTPS 配置已同步")
	}
	return attempted, failed
}

// SyncCertbotLiveCertificates 接收 certbot.timer 或外部 certbot 已更新的
// live 证书，并将新证书回写 SQLite 后同步到站点。它不执行签发命令，
// 只接受位于 WORKMESH_LETSENCRYPT_DIR 下且通过完整校验的真实文件。
func (s *SSLService) SyncCertbotLiveCertificates(ctx context.Context) (updated, failed int) {
	if _, err := s.sqliteRepository(); err != nil {
		return 0, 0
	}
	certRoot := strings.TrimSpace(os.Getenv("WORKMESH_LETSENCRYPT_DIR"))
	if certRoot == "" {
		certRoot = "/etc/letsencrypt"
	}
	certRoot = filepath.Clean(certRoot)
	s.mu.RLock()
	items := make([]model.WebsiteSSL, 0, len(s.items))
	for _, item := range s.items {
		if isHTTP01Provider(item.Provider) {
			items = append(items, item)
		}
	}
	s.mu.RUnlock()
	for _, item := range items {
		primary, domains, err := normalizeCertificateDomains(item.PrimaryDomain, item.Domains)
		if err != nil {
			continue
		}
		domains = append([]string{primary}, domains...)
		certPath := filepath.Join(certRoot, "live", primary, "fullchain.pem")
		keyPath := filepath.Join(certRoot, "live", primary, "privkey.pem")
		if !withinPath(certRoot, certPath) || !withinPath(certRoot, keyPath) {
			continue
		}
		certPEM, certErr := readCertbotFile(certRoot, certPath)
		keyPEM, keyErr := readCertbotFile(certRoot, keyPath)
		if certErr != nil || keyErr != nil {
			// certbot.timer 在替换 live 链接的短窗口中可能读到一半；
			// 下一个后台周期会再次发现，不污染当前有效 SQLite 状态。
			continue
		}
		parsed, parseErr := parseCertificate(certPEM)
		if parseErr != nil || !parsed.NotAfter.After(time.Now()) || certificateCoversDomains(parsed, domains) != nil || validateImportedPrivateKey(parsed, string(keyPEM), "pem") != nil {
			continue
		}
		if !parsed.NotAfter.After(item.ExpireDate) && string(certPEM) == item.Certificate && string(keyPEM) == item.PrivateKey {
			continue
		}
		s.mu.Lock()
		current, ok := s.items[item.ID]
		stale := ok && !current.ExpireDate.IsZero() && parsed.NotAfter.Before(current.ExpireDate)
		if stale {
			s.mu.Unlock()
			continue
		}
		updated++
		if ok && !stale && (current.ExpireDate.Before(parsed.NotAfter) || current.Certificate != string(certPEM) || current.PrivateKey != string(keyPEM)) {
			current.Certificate = string(certPEM)
			current.PEM = string(certPEM)
			current.PrivateKey = string(keyPEM)
			current.StartDate = parsed.NotBefore
			current.ExpireDate = parsed.NotAfter
			current.Status = "ready"
			current.Message = "certbot 证书已同步到 SQLite，正在同步站点"
			current.UpdatedAt = time.Now().UTC()
			s.items[item.ID] = current
			err = s.persistLocked()
		}
		s.mu.Unlock()
		if err != nil {
			failed++
			s.updateReadyMessage(item.ID, "certbot 证书已更新，但 SQLite 保存失败: "+err.Error())
			continue
		}
		if _, syncErr := s.syncIssuedCertificate(item.ID, certPEM, keyPEM); syncErr != nil {
			failed++
			s.updateReadyMessage(item.ID, "certbot 证书已更新，但站点 HTTPS 配置同步失败: "+syncErr.Error())
			continue
		}
		s.updateReadyMessage(item.ID, "certbot 证书已同步，站点 HTTPS 配置已 reload")
	}
	return updated, failed
}

// readCertbotFile 读取 certbot live 文件并校验最终解析路径仍位于证书根目录。
// certbot 默认使用 live -> archive 符号链接，因此只拒绝越界链接而非所有链接。
func readCertbotFile(certRoot, path string) ([]byte, error) {
	resolvedRoot, err := filepath.EvalSymlinks(certRoot)
	if err != nil {
		return nil, err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	if !withinPath(resolvedRoot, resolvedPath) {
		return nil, fmt.Errorf("certbot 文件符号链接越过证书根目录: %s", path)
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("certbot 文件不是常规文件: %s", path)
	}
	return os.ReadFile(resolvedPath)
}

// readSSLFileBackup 读取常规证书文件并拒绝符号链接或目录目标。
func readSSLFileBackup(path string) (sslFileBackup, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return sslFileBackup{path: path, mode: 0o600}, nil
	}
	if err != nil {
		return sslFileBackup{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return sslFileBackup{}, fmt.Errorf("证书目标不是常规文件: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return sslFileBackup{}, err
	}
	return sslFileBackup{path: path, data: data, mode: info.Mode().Perm(), exists: true}, nil
}

// rollbackSSLFiles 恢复证书同步前的全部文件状态。
func rollbackSSLFiles(backups []sslFileBackup) error {
	var first error
	for _, backup := range backups {
		var err error
		if backup.exists {
			mode := backup.mode
			if mode == 0 {
				mode = 0o600
			}
			err = writeWebsiteAtomic(backup.path, backup.data, mode)
		} else {
			err = os.Remove(backup.path)
			if errors.Is(err, os.ErrNotExist) {
				err = nil
			}
		}
		if err != nil && first == nil {
			first = fmt.Errorf("恢复 HTTPS 文件 %s 失败: %w", backup.path, err)
		}
	}
	return first
}

// combineSSLSyncErrors 合并同步、回滚和恢复 reload 错误，确保日志保留完整根因。
func combineSSLSyncErrors(errs ...error) error {
	valid := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			valid = append(valid, err)
		}
	}
	if len(valid) == 0 {
		return nil
	}
	return errors.Join(valid...)
}

// updateReadyMessage 记录证书签发后同步结果，不改变 ready 状态。
func (s *SSLService) updateReadyMessage(id uint, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item, ok := s.items[id]; ok {
		item.Message = message
		item.UpdatedAt = time.Now().UTC()
		s.items[id] = item
		_ = s.persistLocked()
	}
}
