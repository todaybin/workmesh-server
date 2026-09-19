// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"fmt"
	"os"
	"strings"
)

// runCLI 解析本地管理命令；返回 true 表示命令已被 CLI 消费。
func runCLI(args []string, cfgDataDir string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	normalized, consumed := normalizeCLILanguageArgs(args)
	if consumed {
		return true, nil
	}
	if normalized[len(normalized)-1] == "-h" || normalized[len(normalized)-1] == "--help" {
		printCommandHelp(normalized[:len(normalized)-1])
		return true, nil
	}
	switch normalized[0] {
	case "help":
		printCLIHelp()
		return true, nil
	case "version":
		return true, printCLIVersion()
	case "user-list":
		return true, printCLIUsers(cfgDataDir)
	case "user-password":
		return true, handleCLIUserPassword(normalized, cfgDataDir)
	case "security-entrance":
		return true, handleCLISecurityEntrance(normalized, cfgDataDir)
	case "user-info":
		return true, handleCLIUserInfo(cfgDataDir)
	case "listen-ip":
		return true, handleCLIListenIP(normalized, cfgDataDir)
	case "reset":
		return true, handleCLIReset(normalized, cfgDataDir)
	case "app":
		return true, handleCLIApp(normalized, cfgDataDir)
	case "update":
		return true, handleCLIUpdate(normalized, cfgDataDir)
	case "restore":
		return true, handleCLIArtifact(normalized[0], normalized[1:], cfgDataDir)
	case "maintenance":
		return true, handleCLIMaintenance(normalized[1:], cfgDataDir)
	default:
		fmt.Printf("未知命令: %s\n\n", normalized[0])
		printCLIHelp()
		return true, nil
	}
}

// normalizeCLILanguageArgs 消费全局语言参数并处理其缺少值的情况。
func normalizeCLILanguageArgs(args []string) ([]string, bool) {
	if args[0] != "-l" && args[0] != "--language" {
		return args, false
	}
	if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
		fmt.Printf("Error: flag needs an argument: '%s'\n\n", strings.TrimLeft(args[0], "-"))
		printCLIHelp()
		return nil, true
	}
	if len(args) == 2 {
		printCLIHelp()
		return nil, true
	}
	return args[2:], false
}

// printCLIVersion 输出当前构建注入的版本号。
func printCLIVersion() error {
	version := os.Getenv("WORKMESH_VERSION")
	if version == "" {
		version = "dev"
	}
	fmt.Println(version)
	return nil
}
