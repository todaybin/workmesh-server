// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/node/model"
)

// TestReadCertbotFileRejectsEscapingSymlink 确认证书 live 链接只能解析到
// LETSENCRYPT 根目录内，防止续期扫描读取任意外部文件。
func TestReadCertbotFileRejectsEscapingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 测试环境可能没有创建符号链接权限")
	}
	root := t.TempDir()
	archive := filepath.Join(root, "archive", "example.com")
	live := filepath.Join(root, "live", "example.com")
	outside := filepath.Join(t.TempDir(), "outside.pem")
	if err := os.MkdirAll(archive, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(live, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archive, "fullchain.pem"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	insideLink := filepath.Join(live, "fullchain.pem")
	if err := os.Symlink(filepath.Join("..", "..", "archive", "example.com", "fullchain.pem"), insideLink); err != nil {
		t.Fatal(err)
	}
	data, err := readCertbotFile(root, insideLink)
	if err != nil || string(data) != "inside" {
		t.Fatalf("合法 certbot live 链接读取失败: %q %v", data, err)
	}
	escapeLink := filepath.Join(live, "escape.pem")
	if err := os.Symlink(outside, escapeLink); err != nil {
		t.Fatal(err)
	}
	if _, err := readCertbotFile(root, escapeLink); err == nil || !strings.Contains(err.Error(), "越过证书根目录") {
		t.Fatalf("越界 certbot 链接未被拒绝: %v", err)
	}
}

// TestRollbackSSLFilesRestoresPreviousState 验证部分写入失败后的回滚同时
// 恢复旧内容和删除原本不存在的新文件。
func TestRollbackSSLFilesRestoresPreviousState(t *testing.T) {
	root := t.TempDir()
	certPath := filepath.Join(root, "fullchain.pem")
	keyPath := filepath.Join(root, "privkey.pem")
	missingPath := filepath.Join(root, "new.pem")
	if err := os.WriteFile(certPath, []byte("old-cert"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("old-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	backups := []sslFileBackup{{path: certPath, data: []byte("old-cert"), mode: 0o644, exists: true}, {path: keyPath, data: []byte("old-key"), mode: 0o600, exists: true}, {path: missingPath, mode: 0o600}}
	if err := writeWebsiteAtomic(certPath, []byte("new-cert"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeWebsiteAtomic(keyPath, []byte("new-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeWebsiteAtomic(missingPath, []byte("new-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rollbackSSLFiles(backups); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{certPath: "old-cert", keyPath: "old-key"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("文件未恢复 %s: %q %v", path, got, err)
		}
	}
	if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
		t.Fatalf("原本不存在的文件未删除: %v", err)
	}
}

// TestRetryPendingSyncPersistsReadyState 验证同步失败状态会写入 SQLite，
// 重试成功后消息更新，第二次扫描不再重复处理同一证书。
func TestRetryPendingSyncPersistsReadyState(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	service := NewSSLService()
	item, err := service.Create(context.Background(), model.WebsiteSSLCreateRequest{PrimaryDomain: "retry.example.com", Provider: "letsencrypt", AutoRenew: true})
	if err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	stored := service.items[item.ID]
	stored.Status = "ready"
	stored.Certificate = "certificate"
	stored.PrivateKey = "private-key"
	stored.Message = "ACME 证书申请成功，但站点 HTTPS 配置同步失败: temporary"
	service.items[item.ID] = stored
	if err := service.persistLocked(); err != nil {
		service.mu.Unlock()
		t.Fatal(err)
	}
	service.mu.Unlock()
	attempted, failed := service.RetryPendingSync(context.Background())
	if attempted != 1 || failed != 0 {
		t.Fatalf("首次重试统计异常: attempted=%d failed=%d", attempted, failed)
	}
	reloaded := NewSSLService()
	got, err := reloaded.Get(context.Background(), item.ID)
	if err != nil || got.Status != "ready" || !strings.Contains(got.Message, "配置已同步") {
		t.Fatalf("重试后的 SQLite 状态异常: %+v %v", got, err)
	}
	attempted, failed = reloaded.RetryPendingSync(context.Background())
	if attempted != 0 || failed != 0 {
		t.Fatalf("幂等重试异常: attempted=%d failed=%d", attempted, failed)
	}
}
