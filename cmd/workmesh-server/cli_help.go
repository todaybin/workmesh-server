// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"fmt"
	"strings"
)

// printCLIHelp 打印根命令及其可用子命令。
func printCLIHelp() {
	fmt.Println("Usage:")
	fmt.Println("  wh [flags]")
	fmt.Println("  wh [command]")
	fmt.Println()
	fmt.Println("Available Commands:")
	fmt.Println("  app         ")
	fmt.Println("  completion  Generate the autocompletion script for the specified shell")
	fmt.Println("  help        Help about any command")
	fmt.Println("  listen-ip   ")
	fmt.Println("  reset       ")
	fmt.Println("  restore     ")
	fmt.Println("  update      ")
	fmt.Println("  user-info   ")
	fmt.Println("  version     ")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -h, --help              help for wh")
	fmt.Println("  -l, --language string   Set the language (default \"en\")")
	fmt.Println()
	fmt.Println("Use \"wh [command] --help\" for more information about a command.")
}

// printResetHelp 打印重置类命令的用法和可选项。
func printResetHelp() {
	fmt.Println("重置系统信息")
	fmt.Println("\nUsage:\n  wh reset [command]\n\nAvailable Commands:\n\n  domain      取消访问域名绑定\n  entrance    取消安全入口\n  https       取消 https 登录\n  ips         取消授权 IP 限制\n  mfa         取消两步验证\n  passkey     清空通行密钥\n\nFlags:\n  -h, --help   help for reset\n\nUse \"wh reset [command] --help\" for more information about a command.")
}

// printListenIPHelp 打印监听地址切换命令的用法。
func printListenIPHelp() {
	fmt.Println("切换监听 IP")
	fmt.Println("\nUsage:\n  wh listen-ip [command]\n\nAvailable Commands:\n\n  ipv4        监听 IPv4\n  ipv6        监听 IPv6\n\nFlags:\n  -h, --help   help for listen-ip\n\nUse \"wh listen-ip [command] --help\" for more information about a command.")
}

// printUpdateHelp 打印面板更新命令的用法和子命令。
func printUpdateHelp() {
	fmt.Println("修改面板信息")
	fmt.Println("\nUsage:\n  wh update [command]\n\nAvailable Commands:\n\n  entrance    修改安全入口\n  password    修改用户密码\n  port        修改面板端口\n  username    修改用户信息\n  version     更新服务版本\n\nFlags:\n  -h, --help   help for update\n\nUse \"wh update [command] --help\" for more information about a command.")
}

// printCommandHelp 根据命令路径选择对应的帮助文本。
func printCommandHelp(path []string) {
	if len(path) == 0 {
		printCLIHelp()
		return
	}
	switch path[0] {
	case "update":
		if len(path) == 1 {
			printUpdateHelp()
		} else {
			printLeafHelp(strings.Join(path, " "), "修改面板信息")
		}
	case "reset":
		if len(path) == 1 {
			printResetHelp()
		} else {
			printLeafHelp(strings.Join(path, " "), "重置系统信息")
		}
	case "listen-ip":
		if len(path) == 1 {
			printListenIPHelp()
		} else {
			printLeafHelp(strings.Join(path, " "), "切换监听 IP")
		}
	case "app":
		if len(path) == 1 {
			printAppHelp()
		} else {
			printLeafHelp(strings.Join(path, " "), "初始化应用")
		}
	default:
		printCLIHelp()
	}
}

// printAppHelp 打印应用初始化命令的用法。
func printAppHelp() {
	fmt.Println("应用相关命令\n\nUsage:\n  wh app [command]\n\nAvailable Commands:\n\n  init        初始化应用\n\nFlags:\n  -h, --help             help for app\n  -k, --key string       应用标识\n  -v, --version string   应用版本\n\nUse \"wh app [command] --help\" for more information about a command.")
}

// printLeafHelp 打印没有专用说明的叶子命令通用帮助。
func printLeafHelp(command, title string) {
	fmt.Printf("%s\n\nUsage:\n  wh %s [flags]\n\nFlags:\n  -h, --help   help for %s\n", title, command, command)
}
