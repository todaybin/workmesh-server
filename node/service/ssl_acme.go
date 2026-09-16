// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Obtain 使用 certbot 执行 HTTP-01 申请或续期，并持久化真实证书内容。
func (s *SSLService) Obtain(ctx context.Context, id uint, renew bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	item, ok := s.items[id]
	if !ok {
		s.mu.Unlock()
		return errors.New("证书不存在")
	}
	provider := strings.ToLower(strings.TrimSpace(item.Provider))
	if !isHTTP01Provider(provider) && provider != "dnsaccount" && provider != "dnsmanual" {
		s.mu.Unlock()
		return fmt.Errorf("证书提供商 %q 暂不支持本机自动签发", item.Provider)
	}
	if strings.EqualFold(item.Status, "applying") {
		s.mu.Unlock()
		return errors.New("证书申请正在执行")
	}
	item.Status = "applying"
	item.Message = "正在申请 ACME 证书"
	item.UpdatedAt = time.Now().UTC()
	s.items[id] = item
	if err := s.persistLocked(); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("保存证书申请状态失败: %w", err)
	}
	s.mu.Unlock()

	logPath, err := s.LogPath(ctx, id)
	if err != nil {
		return s.failObtain(id, err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o750); err != nil {
		return s.failObtain(id, err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return s.failObtain(id, err)
	}
	defer logFile.Close()
	if renew {
		_, _ = fmt.Fprintf(logFile, "\n========== [%s] auto renew attempt ==========\n", time.Now().UTC().Format(time.RFC3339))
	} else {
		_, _ = fmt.Fprintf(logFile, "========== [%s] certificate obtain attempt ==========\n", time.Now().UTC().Format(time.RFC3339))
	}
	if provider == "dnsaccount" {
		return s.obtainDNSAccount(ctx, id, item, logFile)
	}
	if provider == "dnsmanual" {
		return s.obtainDNSManual(ctx, id, item, logFile)
	}

	webroot, domains, email, err := s.obtainInputs(id)
	if err != nil {
		return s.finishObtain(id, logFile, err)
	}
	certbot := strings.TrimSpace(os.Getenv("WORKMESH_CERTBOT_BIN"))
	if certbot == "" {
		certbot = "certbot"
	}
	certRoot := strings.TrimSpace(os.Getenv("WORKMESH_LETSENCRYPT_DIR"))
	if certRoot == "" {
		certRoot = "/etc/letsencrypt"
	}
	args := []string{"certonly", "--non-interactive", "--agree-tos", "--webroot", "-w", webroot, "--preferred-challenges", "http", "--cert-name", domains[0], "--config-dir", certRoot, "--work-dir", filepath.Join(certRoot, "work"), "--logs-dir", filepath.Join(certRoot, "logs")}
	args = append(args, certbotKeyArgs(item.KeyType)...)
	if email != "" {
		args = append(args, "--email", email)
	} else {
		args = append(args, "--register-unsafely-without-email")
	}
	for _, domain := range domains {
		args = append(args, "-d", domain)
	}
	cmd := exec.CommandContext(ctx, certbot, args...)
	output, runErr := cmd.CombinedOutput()
	if len(output) > 4<<20 {
		output = output[len(output)-(4<<20):]
	}
	_, _ = logFile.Write(output)
	if runErr != nil {
		return s.finishObtain(id, logFile, fmt.Errorf("certbot 执行失败: %w", runErr))
	}
	certPath := filepath.Join(certRoot, "live", domains[0], "fullchain.pem")
	keyPath := filepath.Join(certRoot, "live", domains[0], "privkey.pem")
	certPEM, certErr := os.ReadFile(certPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	if certErr != nil || keyErr != nil {
		if certErr == nil {
			certErr = keyErr
		}
		return s.finishObtain(id, logFile, fmt.Errorf("读取 certbot 证书文件失败: %w", certErr))
	}
	parsed, parseErr := parseCertificate(certPEM)
	if parseErr != nil {
		return s.finishObtain(id, logFile, parseErr)
	}
	if err := certificateCoversDomains(parsed, domains); err != nil {
		return s.finishObtain(id, logFile, err)
	}
	s.mu.Lock()
	item, ok = s.items[id]
	if ok {
		item.Certificate = string(certPEM)
		item.PEM = string(certPEM)
		item.PrivateKey = string(keyPEM)
		item.StartDate = parsed.NotBefore
		item.ExpireDate = parsed.NotAfter
		item.Status = "ready"
		item.Message = "ACME 证书申请成功"
		item.UpdatedAt = time.Now().UTC()
		s.items[id] = item
		err = s.persistLocked()
	}
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("保存 ACME 证书失败: %w", err)
	}
	if _, syncErr := s.syncIssuedCertificate(id, certPEM, keyPEM); syncErr != nil {
		// 证书已真实签发并持久化；同步失败不伪装成签发失败，但要
		// 在状态和日志中暴露，方便运维修复 OpenResty 或目录权限。
		_, _ = fmt.Fprintf(logFile, "站点 HTTPS 配置同步失败: %s\n", syncErr)
		s.updateReadyMessage(id, "ACME 证书申请成功，但站点 HTTPS 配置同步失败: "+syncErr.Error())
		return syncErr
	}
	_, _ = fmt.Fprintln(logFile, "证书申请完成")
	return nil
}

// obtainInputs 从 SQLite 解析申请域名、webroot 和 ACME 账户邮箱。
func (s *SSLService) obtainInputs(id uint) (string, []string, string, error) {
	s.mu.RLock()
	item, ok := s.items[id]
	s.mu.RUnlock()
	if !ok {
		return "", nil, "", errors.New("证书不存在")
	}
	primary, domains, err := normalizeCertificateDomains(item.PrimaryDomain, item.Domains)
	if err != nil {
		return "", nil, "", fmt.Errorf("HTTP-01 域名无效: %w", err)
	}
	domains = append([]string{primary}, domains...)
	repository, err := s.sqliteRepository()
	if err != nil {
		return "", nil, "", err
	}
	var webroot, email string
	queryErr := repository.QueryRow(`SELECT site_dir FROM websites WHERE website_ssl_id=? OR primary_domain=? LIMIT 1`, id, primary).Scan(&webroot)
	if queryErr != nil && !errors.Is(queryErr, sql.ErrNoRows) {
		return "", nil, "", fmt.Errorf("查询网站 webroot 失败: %w", queryErr)
	}
	if strings.TrimSpace(webroot) == "" {
		return "", nil, "", errors.New("未找到证书对应的网站根目录")
	}
	webroot = filepath.Join(filepath.Clean(webroot), "app")
	if info, err := os.Stat(webroot); err != nil || !info.IsDir() {
		return "", nil, "", errors.New("网站 webroot 不存在")
	}
	if err := ensureHTTP01Webroot(webroot); err != nil {
		return "", nil, "", err
	}
	if item.AcmeAccountID != 0 {
		if err := repository.QueryRow(`SELECT email FROM website_acme_accounts WHERE id=?`, item.AcmeAccountID).Scan(&email); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return "", nil, "", errors.New("ACME 账户不存在")
			}
			return "", nil, "", fmt.Errorf("读取 ACME 账户失败: %w", err)
		}
	}
	return webroot, domains, strings.TrimSpace(email), nil
}

// ensureHTTP01Webroot 创建 challenge 目录，并拒绝符号链接目录。
func ensureHTTP01Webroot(webroot string) error {
	challengeRoot := filepath.Join(webroot, ".well-known")
	challengeDir := filepath.Join(challengeRoot, "acme-challenge")
	for _, path := range []string{challengeRoot, challengeDir} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(path, 0o755); err != nil {
				return fmt.Errorf("创建 HTTP-01 challenge 目录失败: %w", err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("检查 HTTP-01 challenge 目录失败: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("HTTP-01 challenge 目录不可用")
		}
	}
	return nil
}

// finishObtain 写入申请日志并把申请状态置为 error。
func (s *SSLService) finishObtain(id uint, logFile *os.File, cause error) error {
	if cause != nil {
		_, _ = fmt.Fprintln(logFile, cause.Error())
	}
	s.mu.Lock()
	if item, ok := s.items[id]; ok {
		item.Status = "error"
		if cause == nil {
			cause = errors.New("证书申请失败")
		}
		item.Message = cause.Error()
		item.UpdatedAt = time.Now().UTC()
		s.items[id] = item
		_ = s.persistLocked()
	}
	s.mu.Unlock()
	return cause
}

// failObtain 在申请日志创建前记录失败，避免状态永久停留 applying。
func (s *SSLService) failObtain(id uint, cause error) error {
	s.mu.Lock()
	if item, ok := s.items[id]; ok {
		item.Status = "error"
		if cause == nil {
			cause = errors.New("证书申请失败")
		}
		item.Message = cause.Error()
		item.UpdatedAt = time.Now().UTC()
		s.items[id] = item
		_ = s.persistLocked()
	}
	s.mu.Unlock()
	return cause
}

// RenewDue 启动临近到期 HTTP-01 证书的异步续期任务。
func (s *SSLService) RenewDue(ctx context.Context, horizon time.Duration) int {
	// certbot.timer 可能在服务进程外完成续期，先把 live 目录中的
	// 新证书纳入 SQLite，再处理本服务自己发起的续期任务。
	_, _ = s.SyncCertbotLiveCertificates(ctx)
	// 先重试已签发但未能安装的证书，再扫描临近到期证书；两者共用
	// 同一后台周期，避免 reload 短暂失败后要等到下一个到期窗口。
	_, _ = s.RetryPendingSync(ctx)
	items := s.RenewCheck(ctx, horizon)
	started := 0
	for _, item := range items {
		if isHTTP01Provider(item.Provider) {
			started++
			go func(id uint) { _ = s.Obtain(ctx, id, true) }(item.ID)
		}
	}
	return started
}
