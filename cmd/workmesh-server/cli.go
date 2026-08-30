// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	controlservice "github.com/todaybin/workmesh-server/control/service"
)

const (
	// artifactMaxSize 防止命令行更新意外读取无界文件。
	artifactMaxSize          = 512 << 20
	artifactSignatureMaxSize = 8 << 10
)

// artifactInstallResult 描述一次已完成的制品安装及其回滚备份。
type artifactInstallResult struct {
	Mode     string    `json:"mode"`
	Artifact string    `json:"artifact"`
	Target   string    `json:"target"`
	Previous string    `json:"previous,omitempty"`
	SHA256   string    `json:"sha256"`
	Version  string    `json:"version,omitempty"`
	At       time.Time `json:"at"`
}

// artifactOptions 是 restore/update 的受控参数。签名和公钥不得从仓库配置猜测。
type artifactOptions struct {
	Artifact  string
	Signature string
	PublicKey string
	Target    string
	SHA256    string
	Version   string
}

// runCLI 处理服务进程之外的本地管理命令；返回 true 表示已消费命令行。
func runCLI(args []string, cfgDataDir string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case "version":
		version := os.Getenv("WORKMESH_VERSION")
		if version == "" {
			version = "dev"
		}
		fmt.Println(version)
		return true, nil
	case "user-list":
		users := controlservice.NewCoreService().ListUsers()
		for _, user := range users {
			fmt.Printf("%s\t%s\n", user.Name, user.Role)
		}
		return true, nil
	case "user-info":
		fmt.Println("登录地址: http://127.0.0.1:9999")
		fmt.Println("用户名: admin")
		fmt.Println("密码: ********")
		return true, nil
	case "listen-ip":
		if len(args) < 2 || (args[1] != "ipv4" && args[1] != "ipv6") {
			return true, errors.New("用法: listen-ip ipv4|ipv6")
		}
		settings, err := loadCLISettings(cfgDataDir)
		if err != nil {
			return true, err
		}
		if args[1] == "ipv6" {
			settings["bindAddress"], settings["ipv6"] = "::", true
		} else {
			settings["bindAddress"], settings["ipv6"] = "0.0.0.0", false
		}
		if err := saveCLISettings(cfgDataDir, settings); err != nil {
			return true, err
		}
		fmt.Printf("监听地址已更新为 %s\n", settings["bindAddress"])
		return true, nil
	case "reset":
		if len(args) < 2 {
			return true, errors.New("用法: reset entrance|https|ips|domain|passkey")
		}
		settings, err := loadCLISettings(cfgDataDir)
		if err != nil {
			return true, err
		}
		key := strings.TrimSpace(args[1])
		if key == "entrance" {
			settings["securityEntrance"] = ""
		} else if key == "https" {
			settings["ssl"] = false
		} else if key == "ips" {
			settings["bindAddress"], settings["ipv6"] = "0.0.0.0", false
		} else if key == "domain" {
			settings["bindDomain"] = ""
		} else if key == "passkey" {
			settings["passkeyResetAt"] = time.Now().UTC().Format(time.RFC3339)
		} else {
			return true, fmt.Errorf("不支持的重置项: %s", key)
		}
		if err := saveCLISettings(cfgDataDir, settings); err != nil {
			return true, err
		}
		fmt.Println("设置已重置")
		return true, nil
	case "app":
		if len(args) < 2 || args[1] != "init" {
			return true, errors.New("用法: app init")
		}
		if err := os.MkdirAll(filepath.Join(cfgDataDir, "apps"), 0o750); err != nil {
			return true, err
		}
		fmt.Println("应用目录已初始化")
		return true, nil
	case "restore", "update":
		result, err := installSignedArtifact(args[0], args[1:], cfgDataDir)
		if err != nil {
			return true, err
		}
		fmt.Printf("%s 制品已原子安装: %s (sha256=%s)\n", result.Mode, result.Target, result.SHA256)
		return true, nil
	default:
		return false, nil
	}
}

// initializeDataDir 创建单进程服务所需的数据目录，避免运行时首次写入失败。
// 所有目录均位于用户配置的 DataDir 下，不使用系统临时目录保存项目状态。
func initializeDataDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		dir = "./data"
	}
	abs, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return fmt.Errorf("解析数据目录失败: %w", err)
	}
	if filepath.Dir(abs) == abs {
		return errors.New("数据目录不能使用文件系统根目录")
	}
	if info, statErr := os.Stat(abs); statErr == nil && !info.IsDir() {
		return fmt.Errorf("数据目录不是目录: %s", abs)
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("检查数据目录失败: %w", statErr)
	}
	dirs := []string{"apps", "backups", "logs", "releases", "runtime", "uploads"}
	for _, name := range append([]string{""}, dirs...) {
		path := filepath.Join(abs, name)
		if err := os.MkdirAll(path, 0o750); err != nil {
			return fmt.Errorf("创建数据目录 %s 失败: %w", name, err)
		}
	}
	return nil
}

func installSignedArtifact(mode string, args []string, dataDir string) (artifactInstallResult, error) {
	options, err := parseArtifactOptions(args)
	if err != nil {
		return artifactInstallResult{}, err
	}
	if err := initializeDataDir(dataDir); err != nil {
		return artifactInstallResult{}, err
	}
	if options.Signature == "" {
		options.Signature = options.Artifact + ".sig"
	}
	if options.PublicKey == "" {
		options.PublicKey = firstNonEmpty(os.Getenv("WORKMESH_ARTIFACT_PUBLIC_KEY"), os.Getenv("WORKMESH_UPDATE_PUBLIC_KEY"))
	}
	if options.PublicKey == "" {
		return artifactInstallResult{}, errors.New("制品公钥未配置，请设置 WORKMESH_ARTIFACT_PUBLIC_KEY 或 --public-key")
	}
	digest, size, err := hashArtifact(options.Artifact)
	if err != nil {
		return artifactInstallResult{}, err
	}
	if options.SHA256 != "" && !strings.EqualFold(options.SHA256, digest) {
		return artifactInstallResult{}, errors.New("制品 SHA256 校验失败")
	}
	publicKey, err := decodePublicKey(options.PublicKey)
	if err != nil {
		return artifactInstallResult{}, fmt.Errorf("解析制品公钥失败: %w", err)
	}
	signature, err := readEncodedValue(options.Signature, artifactSignatureMaxSize)
	if err != nil {
		return artifactInstallResult{}, fmt.Errorf("读取制品签名失败: %w", err)
	}
	if !verifyDigestSignature(publicKey, digest, signature) {
		return artifactInstallResult{}, errors.New("制品签名校验失败")
	}
	target := options.Target
	if target == "" {
		target = filepath.Join(dataDir, "releases", "current.artifact")
	}
	if err := ensureTargetWithinDataDir(dataDir, target); err != nil {
		return artifactInstallResult{}, err
	}
	// 元数据必须保存绝对路径，服务重启后工作目录变化时仍能恢复状态。
	if target, err = filepath.Abs(filepath.Clean(target)); err != nil {
		return artifactInstallResult{}, fmt.Errorf("解析制品目标失败: %w", err)
	}
	previous, err := atomicInstall(options.Artifact, target, size)
	if err != nil {
		return artifactInstallResult{}, err
	}
	result := artifactInstallResult{Mode: mode, Artifact: options.Artifact, Target: target, Previous: previous, SHA256: digest, Version: options.Version, At: time.Now().UTC()}
	if err := saveArtifactResult(dataDir, result); err != nil {
		return artifactInstallResult{}, err
	}
	return result, nil
}

func parseArtifactOptions(args []string) (artifactOptions, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return artifactOptions{}, errors.New("用法: restore|update <制品路径> [--signature <签名文件>] [--public-key <公钥>] [--target <目标>] [--sha256 <摘要>] [--version <版本>]")
	}
	result := artifactOptions{Artifact: args[0]}
	for i := 1; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "--") || i+1 >= len(args) {
			return artifactOptions{}, fmt.Errorf("无效的制品参数: %s", args[i])
		}
		value := strings.TrimSpace(args[i+1])
		if value == "" {
			return artifactOptions{}, fmt.Errorf("制品参数不能为空: %s", args[i])
		}
		switch args[i] {
		case "--signature":
			result.Signature = value
		case "--public-key":
			result.PublicKey = value
		case "--target":
			result.Target = value
		case "--sha256":
			result.SHA256 = value
		case "--version":
			result.Version = value
		default:
			return artifactOptions{}, fmt.Errorf("不支持的制品参数: %s", args[i])
		}
		i++
	}
	return result, nil
}

func hashArtifact(path string) (string, int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", 0, fmt.Errorf("制品不存在: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", 0, errors.New("制品必须是普通文件，拒绝符号链接或目录")
	}
	if info.Size() < 0 || info.Size() > artifactMaxSize {
		return "", 0, fmt.Errorf("制品大小超出限制（最大 %d 字节）", artifactMaxSize)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("打开制品失败: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.CopyN(h, f, info.Size())
	if err != nil {
		return "", 0, fmt.Errorf("读取制品失败: %w", err)
	}
	if n != info.Size() {
		return "", 0, errors.New("制品在读取期间发生变化")
	}
	return hex.EncodeToString(h.Sum(nil)), info.Size(), nil
}

func readEncodedValue(value string, max int64) ([]byte, error) {
	if info, err := os.Stat(value); err == nil {
		if !info.Mode().IsRegular() || info.Size() > max {
			return nil, errors.New("签名文件必须是受限大小的普通文件")
		}
		data, err := os.ReadFile(value)
		if err != nil {
			return nil, err
		}
		value = strings.TrimSpace(string(data))
	}
	if decoded, err := hex.DecodeString(value); err == nil {
		return decoded, nil
	}
	for _, encoding := range []*base64.Encoding{base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("签名必须是十六进制或 Base64 编码")
}

func decodePublicKey(value string) (ed25519.PublicKey, error) {
	if info, err := os.Stat(value); err == nil {
		if !info.Mode().IsRegular() || info.Size() > artifactSignatureMaxSize {
			return nil, errors.New("公钥文件必须是受限大小的普通文件")
		}
		data, err := os.ReadFile(value)
		if err != nil {
			return nil, err
		}
		value = strings.TrimSpace(string(data))
	}
	if block, _ := pem.Decode([]byte(value)); block != nil {
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		public, ok := key.(ed25519.PublicKey)
		if !ok {
			return nil, errors.New("公钥不是 Ed25519 类型")
		}
		return public, nil
	}
	decoded, err := readEncodedValue(value, artifactSignatureMaxSize)
	if err != nil {
		return nil, err
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("Ed25519 公钥长度无效: %d", len(decoded))
	}
	return ed25519.PublicKey(decoded), nil
}

func verifyDigestSignature(public ed25519.PublicKey, digest string, signature []byte) bool {
	if len(signature) != ed25519.SignatureSize {
		return false
	}
	// 网关签名协议通常签名摘要文本；同时接受摘要原始字节以便制品工具互操作。
	if ed25519.Verify(public, []byte(digest), signature) {
		return true
	}
	binaryDigest, err := hex.DecodeString(digest)
	return err == nil && ed25519.Verify(public, binaryDigest, signature)
}

func ensureTargetWithinDataDir(dataDir, target string) error {
	base, err := filepath.Abs(filepath.Clean(dataDir))
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(base, absTarget)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("目标路径必须位于数据目录内")
	}
	if info, statErr := os.Lstat(absTarget); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("目标路径不能是符号链接")
	}
	if err := os.MkdirAll(filepath.Dir(absTarget), 0o750); err != nil {
		return fmt.Errorf("创建制品目标目录失败: %w", err)
	}
	return nil
}

func atomicInstall(source, target string, size int64) (string, error) {
	target = filepath.Clean(target)
	tmp, err := os.CreateTemp(filepath.Dir(target), ".artifact-*.tmp")
	if err != nil {
		return "", fmt.Errorf("创建制品临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = tmp.Close(); _ = os.Remove(tmpName) }
	src, err := os.Open(source)
	if err != nil {
		cleanup()
		return "", fmt.Errorf("打开源制品失败: %w", err)
	}
	n, copyErr := io.CopyN(tmp, src, size)
	if copyErr != nil {
		_ = src.Close()
		cleanup()
		return "", fmt.Errorf("复制制品失败: %w", copyErr)
	}
	if n != size {
		_ = src.Close()
		cleanup()
		return "", errors.New("制品在复制期间发生变化")
	}
	_ = src.Close()
	if err := tmp.Sync(); err != nil {
		cleanup()
		return "", fmt.Errorf("刷新制品临时文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	if err := os.Chmod(tmpName, 0o750); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	previous := ""
	if _, statErr := os.Stat(target); statErr == nil {
		previous = fmt.Sprintf("%s.previous.%d", target, time.Now().UnixNano())
		if err := os.Rename(target, previous); err != nil {
			_ = os.Remove(tmpName)
			return "", fmt.Errorf("备份当前制品失败: %w", err)
		}
	}
	if err := os.Rename(tmpName, target); err != nil {
		if previous != "" {
			_ = os.Rename(previous, target)
		}
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("原子替换制品失败: %w", err)
	}
	return previous, nil
}

func saveArtifactResult(dataDir string, result artifactInstallResult) error {
	path := filepath.Join(dataDir, "deployment-artifact.json")
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".deployment-artifact-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func loadCLISettings(dir string) (map[string]any, error) {
	if strings.TrimSpace(dir) == "" {
		dir = "./data"
	}
	path := filepath.Join(dir, "cli-settings.json")
	data := map[string]any{}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return data, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("读取 CLI 设置失败: %w", err)
	}
	return data, nil
}

func saveCLISettings(dir string, data map[string]any) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	path := filepath.Join(dir, "cli-settings.json")
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
