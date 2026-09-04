// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreparePHPExtensionRemovalCommit(t *testing.T) {
	root, extensionDir := phpExtensionFixture(t)
	tx, err := PreparePHPExtensionRemoval(PHPExtensionRemoval{
		RuntimeDir: root, ExtensionDir: extensionDir, Name: "ionCube", ModuleFile: "ioncube_loader.so", Extensions: []string{"redis"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	for _, path := range []string{
		filepath.Join(extensionDir, "ioncube_loader.so"),
		filepath.Join(root, "conf", "conf.d", "docker-php-ext-ionCube.ini"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("卸载后文件仍存在: %s err=%v", path, err)
		}
	}
	phpINI, _ := os.ReadFile(filepath.Join(root, "conf", "php.ini"))
	if strings.Contains(string(phpINI), "ioncube_loader.so") || !strings.Contains(string(phpINI), "redis.so") {
		t.Fatalf("php.ini 更新错误: %s", phpINI)
	}
	env, _ := os.ReadFile(filepath.Join(root, ".env"))
	if !strings.Contains(string(env), "PHP_EXTENSIONS=redis\n") || strings.Count(string(env), "PHP_EXTENSIONS=") != 1 {
		t.Fatalf(".env 更新错误: %s", env)
	}
}

func TestPreparePHPExtensionRemovalRollback(t *testing.T) {
	root, extensionDir := phpExtensionFixture(t)
	tx, err := PreparePHPExtensionRemoval(PHPExtensionRemoval{
		RuntimeDir: root, ExtensionDir: extensionDir, Name: "ionCube", ModuleFile: "ioncube_loader.so", Extensions: []string{"redis"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(extensionDir, "ioncube_loader.so"),
		filepath.Join(root, "conf", "conf.d", "docker-php-ext-ionCube.ini"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("回滚未恢复文件: %s err=%v", path, err)
		}
	}
	phpINI, _ := os.ReadFile(filepath.Join(root, "conf", "php.ini"))
	if !strings.Contains(string(phpINI), `zend_extension="ioncube_loader.so"`) {
		t.Fatalf("回滚未恢复 php.ini: %s", phpINI)
	}
	env, _ := os.ReadFile(filepath.Join(root, ".env"))
	if !strings.Contains(string(env), "PHP_EXTENSIONS=ionCube,redis") {
		t.Fatalf("回滚未恢复 .env: %s", env)
	}
}

func TestPreparePHPExtensionRemovalRejectsEscapedDirectory(t *testing.T) {
	root, _ := phpExtensionFixture(t)
	outside := t.TempDir()
	if _, err := PreparePHPExtensionRemoval(PHPExtensionRemoval{
		RuntimeDir: root, ExtensionDir: outside, Name: "redis", ModuleFile: "redis.so",
	}); err == nil || !strings.Contains(err.Error(), "越界") {
		t.Fatalf("应拒绝越界扩展目录: %v", err)
	}
}

func phpExtensionFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	extensionDir := filepath.Join(root, "extensions", "no-debug-non-zts-20220829")
	if err := os.MkdirAll(extensionDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "conf", "conf.d"), 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(extensionDir, "ioncube_loader.so"):                    "module",
		filepath.Join(root, "conf", "conf.d", "docker-php-ext-ionCube.ini"): `zend_extension="ioncube_loader.so"`,
		filepath.Join(root, "conf", "php.ini"):                              "extension=redis.so\nzend_extension=\"ioncube_loader.so\"\n",
		filepath.Join(root, ".env"):                                         "OTHER=value\nPHP_EXTENSIONS=ionCube,redis\nPHP_EXTENSIONS=duplicate\n",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root, extensionDir
}
