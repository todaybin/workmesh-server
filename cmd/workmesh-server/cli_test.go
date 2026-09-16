// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLIListenIPPersistsSettings 验证 listen-ip 命令会把 IPv6 监听地址写入设置。
func TestCLIListenIPPersistsSettings(t *testing.T) {
	dir := t.TempDir()
	handled, err := runCLI([]string{"listen-ip", "ipv6"}, dir)
	if !handled || err != nil {
		t.Fatalf("listen-ip 执行失败: %v", err)
	}
	settings, err := loadCLISettings(dir)
	if err != nil || settings["bindAddress"] != "::" {
		t.Fatalf("监听地址未持久化: %#v, %v", settings, err)
	}
}

// TestCLIHelpFlagsNeverStartServer 验证帮助、语言和未知命令参数不会启动服务进程。
func TestCLIHelpFlagsNeverStartServer(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"-l"}, {"--language"}, {"-l", "zh"}, {"unknown-command"}} {
		handled, err := runCLI(args, t.TempDir())
		if !handled || err != nil {
			t.Fatalf("参数 %v 应由 CLI 消费，handled=%v err=%v", args, handled, err)
		}
	}
}

// TestCLIUserAndSecuritySettingsPersist 验证用户凭据、安全入口和域名解绑命令均能持久化。
func TestCLIUserAndSecuritySettingsPersist(t *testing.T) {
	dir := t.TempDir()
	if handled, err := runCLI([]string{"update", "username", "operator"}, dir); !handled || err != nil {
		t.Fatalf("修改用户名失败: handled=%v err=%v", handled, err)
	}
	users, err := loadCLIUsers(dir)
	if err != nil || users["operator"].Name != "operator" {
		t.Fatalf("用户名未写入: %#v err=%v", users, err)
	}
	if handled, err := runCLI([]string{"update", "password", "new-password"}, dir); !handled || err != nil {
		t.Fatalf("修改密码失败: handled=%v err=%v", handled, err)
	}
	if handled, err := runCLI([]string{"update", "entrance", "secureentry"}, dir); !handled || err != nil {
		t.Fatalf("修改安全入口失败: handled=%v err=%v", handled, err)
	}
	if entrance, err := loadSecurityEntrance(dir); err != nil || entrance != "secureentry" {
		t.Fatalf("安全入口未写入: %q err=%v", entrance, err)
	}
	if handled, err := runCLI([]string{"reset", "domain"}, dir); !handled || err != nil {
		t.Fatalf("取消域名绑定失败: handled=%v err=%v", handled, err)
	}
}

// TestCLIUpdatePortPersistsEnvironmentOverride 验证更新端口会同步覆盖 server.env 中的地址。
func TestCLIUpdatePortPersistsEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "server.json")
	if err := os.WriteFile(configPath, []byte(`{"listenAddress":"127.0.0.1","listenPort":9999}`), 0o600); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(dir, "server.env")
	if err := os.WriteFile(envPath, []byte("WORKMESH_SERVER_ADDR=0.0.0.0:9999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := os.Getenv("WORKMESH_SERVER_CONFIG")
	defer os.Setenv("WORKMESH_SERVER_CONFIG", old)
	_ = os.Setenv("WORKMESH_SERVER_CONFIG", configPath)
	if err := updateServerPort(18080); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(envPath)
	if err != nil || !strings.Contains(string(raw), "0.0.0.0:18080") {
		t.Fatalf("环境端口未更新: %s err=%v", raw, err)
	}
}

// TestCLIAppInitCreatesDataDirectory 验证 app init 仅创建应用数据目录并正确处理 CLI 调用。
func TestCLIAppInitCreatesDataDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	handled, err := runCLI([]string{"app", "init"}, dir)
	if !handled || err != nil {
		t.Fatalf("app init 执行失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "apps")); err != nil {
		t.Fatalf("应用目录未创建: %v", err)
	}
}

// TestInitializeDataDirCreatesRuntimeLayout 验证初始化只创建启动必需目录，按需目录保持延迟创建。
func TestInitializeDataDirCreatesRuntimeLayout(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "data")
	if err := initializeDataDir(dir); err != nil {
		t.Fatalf("初始化数据目录失败: %v", err)
	}
	for _, name := range []string{"logs", "releases"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.IsDir() {
			t.Fatalf("缺少运行目录 %s: %v", name, err)
		}
	}
	for _, name := range []string{"apps", "backups", "runtime", "uploads"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("按需目录 %s 不应在启动时创建", name)
		}
	}
}

// TestInitializeDataDirRejectsFileAndFilesystemRoot 验证数据目录不能指向普通文件或文件系统根目录。
func TestInitializeDataDirRejectsFileAndFilesystemRoot(t *testing.T) {
	file := filepath.Join(t.TempDir(), "data")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := initializeDataDir(file); err == nil {
		t.Fatal("数据目录为普通文件时应拒绝")
	}
	root := filepath.VolumeName(file) + string(filepath.Separator)
	if err := initializeDataDir(root); err == nil {
		t.Fatal("文件系统根目录不应作为数据目录")
	}
}

// TestCLIUpdateVerifiesSignatureAndAtomicallyInstalls 验证更新命令校验签名并写入制品元数据。
func TestCLIUpdateVerifiesSignatureAndAtomicallyInstalls(t *testing.T) {
	dir := t.TempDir()
	artifact := filepath.Join(dir, "release.bin")
	target := filepath.Join(dir, "releases", "current.artifact")
	content := []byte("signed release v1")
	if err := os.WriteFile(artifact, content, 0o700); err != nil {
		t.Fatal(err)
	}
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	signature := ed25519.Sign(private, []byte(hex.EncodeToString(digest[:])))
	public := base64.RawStdEncoding.EncodeToString(private.Public().(ed25519.PublicKey))
	sigPath := filepath.Join(dir, "release.sig")
	if err := os.WriteFile(sigPath, []byte(base64.RawStdEncoding.EncodeToString(signature)), 0o600); err != nil {
		t.Fatal(err)
	}
	handled, err := runCLI([]string{"update", artifact, "--signature", sigPath, "--public-key", public, "--target", target, "--version", "v1.0.0"}, dir)
	if !handled || err != nil {
		t.Fatalf("update 执行失败: handled=%v err=%v", handled, err)
	}
	installed, err := os.ReadFile(target)
	if err != nil || string(installed) != string(content) {
		t.Fatalf("制品未正确安装: %q %v", installed, err)
	}
	var result artifactInstallResult
	metadata, err := os.ReadFile(filepath.Join(dir, "deployment-artifact.json"))
	if err != nil || json.Unmarshal(metadata, &result) != nil {
		t.Fatalf("制品元数据无效: %s (%v)", metadata, err)
	}
	if result.Mode != "update" || result.Version != "v1.0.0" || !strings.EqualFold(result.SHA256, hex.EncodeToString(digest[:])) {
		t.Fatalf("制品元数据内容错误: %#v", result)
	}
}

// TestCLIRestoreRejectsTamperedArtifactAndUnsafeTarget 验证恢复会拒绝篡改制品和越界目标路径。
func TestCLIRestoreRejectsTamperedArtifactAndUnsafeTarget(t *testing.T) {
	dir := t.TempDir()
	artifact := filepath.Join(dir, "release.bin")
	content := []byte("original")
	if err := os.WriteFile(artifact, content, 0o600); err != nil {
		t.Fatal(err)
	}
	_, private, _ := ed25519.GenerateKey(nil)
	digest := sha256.Sum256(content)
	signature := ed25519.Sign(private, []byte(hex.EncodeToString(digest[:])))
	sigPath := filepath.Join(dir, "release.sig")
	_ = os.WriteFile(sigPath, []byte(base64.RawStdEncoding.EncodeToString(signature)), 0o600)
	// 修改制品后摘要与签名不再匹配，目标目录不应被创建或覆盖。
	_ = os.WriteFile(artifact, []byte("tampered"), 0o600)
	public := base64.RawStdEncoding.EncodeToString(private.Public().(ed25519.PublicKey))
	handled, err := runCLI([]string{"restore", artifact, "--signature", sigPath, "--public-key", public}, dir)
	if !handled || err == nil || !strings.Contains(err.Error(), "签名") {
		t.Fatalf("篡改制品应返回签名错误: handled=%v err=%v", handled, err)
	}
	// 使用与篡改后内容匹配的签名继续验证目标路径，越界仍必须先于写入被拒绝。
	tamperedDigest := sha256.Sum256([]byte("tampered"))
	tamperedSignature := ed25519.Sign(private, []byte(hex.EncodeToString(tamperedDigest[:])))
	_ = os.WriteFile(sigPath, []byte(base64.RawStdEncoding.EncodeToString(tamperedSignature)), 0o600)
	handled, err = runCLI([]string{"restore", artifact, "--signature", sigPath, "--public-key", public, "--target", filepath.Join(dir, "..", "outside.bin")}, dir)
	if !handled || err == nil || !strings.Contains(err.Error(), "数据目录") {
		t.Fatalf("越界目标应被拒绝: handled=%v err=%v", handled, err)
	}
}

// TestCLIRestoreRequiresSigningMaterial 验证恢复命令缺少公钥时返回明确错误。
func TestCLIRestoreRequiresSigningMaterial(t *testing.T) {
	dir := t.TempDir()
	artifact := filepath.Join(dir, "release.bin")
	if err := os.WriteFile(artifact, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	handled, err := runCLI([]string{"restore", artifact}, dir)
	if !handled || err == nil || !strings.Contains(err.Error(), "公钥") {
		t.Fatalf("缺失公钥应返回明确错误: handled=%v err=%v", handled, err)
	}
}
