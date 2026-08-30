// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package gateway

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	identityFileMode = 0o600
	identityDirMode  = 0o700
	maxIdentityBytes = 256
)

// Identity 是节点用于 Gateway 请求签名的本地 Ed25519 身份。
// 私钥只保存在节点数据目录，绝不通过 HTTP 或日志输出。
type Identity struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
}

// NewIdentity 生成新的节点签名身份。
func NewIdentity() (*Identity, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("生成节点 Gateway 身份失败: %w", err)
	}
	return &Identity{PrivateKey: privateKey, PublicKey: publicKey}, nil
}

// LoadOrCreateIdentity 从受保护文件加载身份，不存在时以独占方式创建。
// 使用独占创建避免多个并发进程生成不同公钥并导致云端登记漂移。
func LoadOrCreateIdentity(filename string) (*Identity, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" || strings.ContainsRune(filename, 0) || !filepath.IsAbs(filename) {
		return nil, errors.New("节点 Gateway 身份路径必须是绝对路径")
	}
	if info, err := os.Lstat(filename); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("节点 Gateway 身份文件必须是普通文件")
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			return nil, errors.New("节点 Gateway 身份文件权限不能包含组或其他用户权限")
		}
		return loadIdentity(filename)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("检查节点 Gateway 身份文件失败: %w", err)
	}
	identity, err := NewIdentity()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(filename), identityDirMode); err != nil {
		return nil, fmt.Errorf("创建节点 Gateway 身份目录失败: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Dir(filename), identityDirMode); err != nil {
			return nil, fmt.Errorf("设置节点 Gateway 身份目录权限失败: %w", err)
		}
	}
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, identityFileMode)
	if errors.Is(err, os.ErrExist) {
		return loadIdentity(filename)
	}
	if err != nil {
		return nil, fmt.Errorf("创建节点 Gateway 身份文件失败: %w", err)
	}
	removeOnFailure := true
	defer func() {
		_ = file.Close()
		if removeOnFailure {
			_ = os.Remove(filename)
		}
	}()
	encoded := base64.RawStdEncoding.EncodeToString(identity.PrivateKey)
	if _, err := file.WriteString(encoded + "\n"); err != nil {
		return nil, fmt.Errorf("写入节点 Gateway 身份失败: %w", err)
	}
	if err := file.Sync(); err != nil {
		return nil, fmt.Errorf("同步节点 Gateway 身份失败: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("关闭节点 Gateway 身份失败: %w", err)
	}
	removeOnFailure = false
	return identity, nil
}

func loadIdentity(filename string) (*Identity, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取节点 Gateway 身份失败: %w", err)
	}
	if len(data) > maxIdentityBytes {
		return nil, errors.New("节点 Gateway 身份文件过大")
	}
	value := strings.TrimSpace(string(data))
	key, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		key, err = base64.StdEncoding.DecodeString(value)
	}
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("节点 Gateway 身份文件不是有效的 Ed25519 私钥")
	}
	privateKey := ed25519.PrivateKey(append([]byte(nil), key...))
	publicKey := append(ed25519.PublicKey(nil), privateKey.Public().(ed25519.PublicKey)...)
	return &Identity{PrivateKey: privateKey, PublicKey: publicKey}, nil
}
