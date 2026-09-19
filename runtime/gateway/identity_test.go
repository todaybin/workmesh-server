// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package gateway

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadOrCreateIdentityPersistsPublicKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway-identity.ed25519")
	first, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatalf("首次创建身份失败: %v", err)
	}
	second, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatalf("重启加载身份失败: %v", err)
	}
	if !ed25519.PublicKey(first.PublicKey).Equal(second.PublicKey) {
		t.Fatal("重启后节点公钥发生变化")
	}
	if runtime.GOOS != "windows" {
		if mode := func() os.FileMode { info, _ := os.Stat(path); return info.Mode().Perm() }(); mode&0o077 != 0 {
			t.Fatalf("身份文件权限过宽: %o", mode)
		}
	}
}

func TestLoadOrCreateIdentityRejectsInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	if err := os.WriteFile(path, []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateIdentity(path); err == nil {
		t.Fatal("无效身份文件应被拒绝")
	}
}

func TestRotateIdentityStopsUsingCopiedKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	first, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RotateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if ed25519.PublicKey(first.PublicKey).Equal(second.PublicKey) {
		t.Fatal("机器变化后仍复用了旧 Ed25519 身份")
	}
	matches, err := filepath.Glob(path + ".revoked-*")
	if err != nil || len(matches) != 1 {
		t.Fatalf("旧身份审计副本数量=%d err=%v", len(matches), err)
	}
}
