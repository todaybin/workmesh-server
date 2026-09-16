// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/config"
)

// handleCLIUserPassword 处理兼容的 user-password 顶层命令。
func handleCLIUserPassword(args []string, dataDir string) error {
	if len(args) != 3 || strings.TrimSpace(args[1]) == "" || len(args[2]) < 8 {
		return errors.New("用法: user-password <用户名> <新密码(至少8位)>")
	}
	if err := updateCLIUserPassword(dataDir, args[1], args[2]); err != nil {
		return err
	}
	_ = restartRunningService()
	fmt.Println("用户密码已更新")
	return nil
}

// handleCLISecurityEntrance 保存顶层安全入口设置。
func handleCLISecurityEntrance(args []string, dataDir string) error {
	if len(args) != 2 {
		return errors.New("用法: security-entrance <入口后缀>")
	}
	if err := saveSecurityEntrance(dataDir, args[1]); err != nil {
		return err
	}
	fmt.Println("安全入口已更新")
	return nil
}

// handleCLIUserInfo 输出登录地址、默认用户名和安全入口信息。
func handleCLIUserInfo(dataDir string) error {
	address, loadErr := config.Load()
	users, usersErr := loadCLIUsers(dataDir)
	username := defaultCLIUsername(users, usersErr)
	entrance, _ := loadSecurityEntrance(dataDir)
	if loadErr == nil {
		base := address.PublicURL
		if base == "" {
			base = "http://" + address.ListenAddr
		}
		address.PublicURL = strings.TrimRight(base, "/") + "/" + entrance
		address.ListenAddr = strings.TrimRight(address.ListenAddr, "/") + "/" + entrance
	}
	printCLIUserInfo(address, loadErr, username, entrance)
	return nil
}

// defaultCLIUsername 从已加载用户中选出稳定的默认用户名。
func defaultCLIUsername(users map[string]persistedCLIUser, usersErr error) string {
	if usersErr != nil || len(users) == 0 {
		return "admin"
	}
	names := make([]string, 0, len(users))
	for name := range users {
		names = append(names, name)
	}
	sortCLIUserNames(names)
	return users[names[0]].Name
}

// sortCLIUserNames 对 CLI 用户名排序，保证输出和选择结果稳定。
func sortCLIUserNames(names []string) {
	sort.Strings(names)
}

// printCLIUserInfo 输出用户信息，避免在终端显示敏感密码内容。
func printCLIUserInfo(address config.Config, loadErr error, username, entrance string) {
	if entrance == "" {
		entrance = "(未设置)"
	}
	if loadErr == nil && address.PublicURL != "" {
		fmt.Println("登录地址:", address.PublicURL)
	} else if loadErr == nil {
		fmt.Println("登录地址: http://" + address.ListenAddr)
	} else {
		fmt.Println("登录地址: 未配置 (请检查 server.json)")
	}
	fmt.Println("用户名:", username)
	fmt.Println("密码: ********")
	fmt.Println("安全入口:", entrance)
}

// handleCLIListenIP 更新 IPv4/IPv6 监听设置并同步域名设置。
func handleCLIListenIP(args []string, dataDir string) error {
	if len(args) < 2 || (args[1] != "ipv4" && args[1] != "ipv6") {
		if len(args) < 2 {
			printListenIPHelp()
			return nil
		}
		return errors.New("用法: listen-ip ipv4|ipv6")
	}
	settings, err := loadCLISettings(dataDir)
	if err != nil {
		return err
	}
	settings["bindAddress"], settings["ipv6"] = listenIPValues(args[1])
	if err := saveCLISettings(dataDir, settings); err != nil {
		return err
	}
	ipv6 := "disable"
	if enabled, ok := settings["ipv6"].(bool); ok && enabled {
		ipv6 = "enable"
	}
	if err := saveDomainSettings(dataDir, map[string]any{"bindAddress": settings["bindAddress"], "ipv6": ipv6}); err != nil {
		return err
	}
	fmt.Printf("监听地址已更新为 %s\n", settings["bindAddress"])
	return nil
}

// listenIPValues 将用户选择转换为配置值。
func listenIPValues(protocol string) (string, bool) {
	if protocol == "ipv6" {
		return "::", true
	}
	return "0.0.0.0", false
}

// handleCLIReset 执行入口、HTTPS、IP、域名和通行密钥重置。
func handleCLIReset(args []string, dataDir string) error {
	if len(args) < 2 {
		printResetHelp()
		return nil
	}
	settings, err := loadCLISettings(dataDir)
	if err != nil {
		return err
	}
	key := strings.TrimSpace(args[1])
	if err := applyCLIReset(key, args, dataDir, settings); err != nil {
		return err
	}
	if err := saveCLISettings(dataDir, settings); err != nil {
		return err
	}
	if err := saveResetDomainSettings(dataDir, settings, key); err != nil {
		return err
	}
	fmt.Println("设置已重置")
	return nil
}

// applyCLIReset 修改内存中的设置并处理需要单独持久化的 MFA 与入口。
func applyCLIReset(key string, args []string, dataDir string, settings map[string]any) error {
	switch key {
	case "mfa":
		username := ""
		if len(args) > 2 {
			username = strings.TrimSpace(args[2])
		}
		return resetCLIMFA(dataDir, username)
	case "entrance":
		if err := saveSecurityEntrance(dataDir, ""); err != nil {
			return err
		}
		settings["securityEntrance"] = ""
	case "https":
		settings["ssl"] = false
	case "ips":
		settings["bindAddress"], settings["ipv6"] = "0.0.0.0", false
	case "domain":
		settings["bindDomain"] = ""
	case "passkey":
		settings["passkeyResetAt"] = time.Now().UTC().Format(time.RFC3339)
	default:
		return fmt.Errorf("不支持的重置项: %s", key)
	}
	return nil
}

// saveResetDomainSettings 仅同步域名相关字段，保持旧 CLI 的配置边界。
func saveResetDomainSettings(dataDir string, settings map[string]any, key string) error {
	values := map[string]any{}
	for _, name := range []string{"ssl", "bindAddress", "ipv6", "bindDomain", "allowIPs", "securityEntrance"} {
		if value, ok := settings[name]; ok {
			values[name] = value
		}
	}
	if key == "entrance" {
		values["securityEntrance"] = ""
	}
	return saveDomainSettings(dataDir, values)
}

// handleCLIApp 处理应用目录初始化命令。
func handleCLIApp(args []string, dataDir string) error {
	if len(args) < 2 || args[1] != "init" {
		return errors.New("用法: app init")
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "apps"), 0o750); err != nil {
		return err
	}
	fmt.Println("应用目录已初始化")
	return nil
}

// handleCLIUpdate 处理面板更新子命令及旧版制品更新语法。
func handleCLIUpdate(args []string, dataDir string) error {
	if len(args) == 1 {
		printUpdateHelp()
		return nil
	}
	if handled, err := handleCLIUpdateSubcommand(args, dataDir); handled {
		return err
	}
	return handleCLIArtifact("update", args[1:], dataDir)
}

// handleCLIUpdateSubcommand 分派用户名、密码、端口、版本和入口更新。
func handleCLIUpdateSubcommand(args []string, dataDir string) (bool, error) {
	switch args[1] {
	case "username":
		return true, updateCLIUsernameCommand(args, dataDir)
	case "password":
		return true, updateCLIPasswordCommand(args, dataDir)
	case "port":
		return true, updateCLIPortCommand(args, dataDir)
	case "version":
		return true, updateCLIVersionCommand(args, dataDir)
	case "entrance":
		return true, updateCLIEntranceCommand(args, dataDir)
	default:
		return false, nil
	}
}

// updateCLIUsernameCommand 修改用户名并请求运行中的服务重启。
func updateCLIUsernameCommand(args []string, dataDir string) error {
	if len(args) < 3 {
		return errors.New("用法: update username <新用户名>")
	}
	if err := updateCLIUserName(dataDir, args[2]); err != nil {
		return err
	}
	_ = restartRunningService()
	fmt.Println("用户名已更新为:", args[2])
	return nil
}

// updateCLIPasswordCommand 修改密码并解析可选的用户名参数。
func updateCLIPasswordCommand(args []string, dataDir string) error {
	if len(args) < 3 {
		return errors.New("用法: update password <新密码> [--username <用户名>]")
	}
	name := cliPasswordUsername(args, dataDir)
	if err := updateCLIUserPassword(dataDir, name, args[2]); err != nil {
		return err
	}
	_ = restartRunningService()
	fmt.Println("密码已更新")
	return nil
}

// cliPasswordUsername 取得显式或用户文件中的密码更新目标。
func cliPasswordUsername(args []string, dataDir string) string {
	name := ""
	for i := 3; i+1 < len(args); i++ {
		if args[i] == "--username" {
			name = args[i+1]
		}
	}
	if name != "" {
		return name
	}
	users, _ := loadCLIUsers(dataDir)
	for key := range users {
		return key
	}
	return "admin"
}

// updateCLIPortCommand 校验端口并同步运行服务配置。
func updateCLIPortCommand(args []string, dataDir string) error {
	if len(args) < 3 {
		return errors.New("用法: update port <端口>")
	}
	port, err := strconv.Atoi(args[2])
	if err != nil || port < 1 || port > 65535 {
		return errors.New("端口必须是 1-65535")
	}
	if err := updateServerPort(port); err != nil {
		return err
	}
	_ = restartRunningService()
	fmt.Printf("端口已更新为 %d\n", port)
	return nil
}

// updateCLIVersionCommand 记录符合 v2 约定的版本更新任务。
func updateCLIVersionCommand(args []string, dataDir string) error {
	if len(args) < 3 {
		return errors.New("用法: update version <版本>")
	}
	if !strings.HasPrefix(strings.TrimSpace(args[2]), "v2.") {
		return errors.New("版本必须以 v2. 开头")
	}
	if err := saveDomainSettings(dataDir, map[string]any{"version": args[2]}); err != nil {
		return err
	}
	fmt.Println("版本更新任务已记录:", args[2])
	return nil
}

// updateCLIEntranceCommand 保存 update entrance 子命令的安全入口。
func updateCLIEntranceCommand(args []string, dataDir string) error {
	if len(args) < 3 {
		return errors.New("用法: update entrance <入口后缀>")
	}
	if err := saveSecurityEntrance(dataDir, args[2]); err != nil {
		return err
	}
	fmt.Println("安全入口已更新:", strings.Trim(args[2], "/"))
	return nil
}

// handleCLIArtifact 安装签名制品并输出安装结果摘要。
func handleCLIArtifact(mode string, args []string, dataDir string) error {
	result, err := installSignedArtifact(mode, args, dataDir)
	if err != nil {
		return err
	}
	fmt.Printf("%s 制品已原子安装: %s (sha256=%s)\n", result.Mode, result.Target, result.SHA256)
	return nil
}
