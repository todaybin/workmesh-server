// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	controlservice "github.com/todaybin/workmesh-server/control/service"
)

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
		return true, fmt.Errorf("%s 需要通过受控部署接口执行，命令行不接受未验证的路径", args[0])
	default:
		return false, nil
	}
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
