// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/config"
	"github.com/todaybin/workmesh-server/internal/storage"
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
	if args[0] == "-l" || args[0] == "--language" {
		// 语言参数必须始终作为 CLI 参数处理，不能因缺少命令而落入服务启动分支。
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			fmt.Printf("Error: flag needs an argument: '%s'\n\n", strings.TrimLeft(args[0], "-"))
			printCLIHelp()
			return true, nil
		}
		if len(args) == 2 {
			printCLIHelp()
			return true, nil
		}
		args = args[2:]
	}
	if args[len(args)-1] == "-h" || args[len(args)-1] == "--help" {
		printCommandHelp(args[:len(args)-1])
		return true, nil
	}
	switch args[0] {
	case "help":
		printCLIHelp()
		return true, nil
	case "version":
		version := os.Getenv("WORKMESH_VERSION")
		if version == "" {
			version = "dev"
		}
		fmt.Println(version)
		return true, nil
	case "user-list":
		if err := printCLIUsers(cfgDataDir); err != nil {
			return true, err
		}
		return true, nil
	case "user-password":
		if len(args) != 3 || strings.TrimSpace(args[1]) == "" || len(args[2]) < 8 {
			return true, errors.New("用法: user-password <用户名> <新密码(至少8位)>")
		}
		if err := updateCLIUserPassword(cfgDataDir, args[1], args[2]); err != nil {
			return true, err
		}
		_ = restartRunningService()
		fmt.Println("用户密码已更新")
		return true, nil
	case "security-entrance":
		if len(args) != 2 {
			return true, errors.New("用法: security-entrance <入口后缀>")
		}
		if err := saveSecurityEntrance(cfgDataDir, args[1]); err != nil {
			return true, err
		}
		fmt.Println("安全入口已更新")
		return true, nil
	case "user-info":
		address, loadErr := config.Load()
		users, usersErr := loadCLIUsers(cfgDataDir)
		username := "admin"
		if usersErr == nil && len(users) > 0 {
			names := make([]string, 0, len(users))
			for name := range users {
				names = append(names, name)
			}
			sort.Strings(names)
			username = users[names[0]].Name
		}
		entrance, _ := loadSecurityEntrance(cfgDataDir)
		if loadErr == nil {
			base := address.PublicURL
			if base == "" {
				base = "http://" + address.ListenAddr
			}
			address.PublicURL = strings.TrimRight(base, "/") + "/" + entrance
			address.ListenAddr = strings.TrimRight(address.ListenAddr, "/") + "/" + entrance
		}
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
		return true, nil
	case "listen-ip":
		if len(args) < 2 || (args[1] != "ipv4" && args[1] != "ipv6") {
			if len(args) < 2 {
				printListenIPHelp()
				return true, nil
			}
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
		ipv6 := "disable"
		if enabled, ok := settings["ipv6"].(bool); ok && enabled {
			ipv6 = "enable"
		}
		if err := saveDomainSettings(cfgDataDir, map[string]any{"bindAddress": settings["bindAddress"], "ipv6": ipv6}); err != nil {
			return true, err
		}
		fmt.Printf("监听地址已更新为 %s\n", settings["bindAddress"])
		return true, nil
	case "reset":
		if len(args) < 2 {
			printResetHelp()
			return true, nil
		}
		settings, err := loadCLISettings(cfgDataDir)
		if err != nil {
			return true, err
		}
		key := strings.TrimSpace(args[1])
		if key == "mfa" {
			username := ""
			if len(args) > 2 {
				username = strings.TrimSpace(args[2])
			}
			if err := resetCLIMFA(cfgDataDir, username); err != nil {
				return true, err
			}
		} else if key == "entrance" {
			if err := saveSecurityEntrance(cfgDataDir, ""); err != nil {
				return true, err
			}
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
		domainValues := map[string]any{}
		for _, key := range []string{"ssl", "bindAddress", "ipv6", "bindDomain", "allowIPs", "securityEntrance"} {
			if value, ok := settings[key]; ok {
				domainValues[key] = value
			}
		}
		if key == "entrance" {
			domainValues["securityEntrance"] = ""
		}
		if err := saveDomainSettings(cfgDataDir, domainValues); err != nil {
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
	case "update":
		if len(args) == 1 {
			printUpdateHelp()
			return true, nil
		}
		if len(args) >= 2 {
			switch args[1] {
			case "username":
				if len(args) < 3 {
					return true, errors.New("用法: update username <新用户名>")
				}
				if err := updateCLIUserName(cfgDataDir, args[2]); err != nil {
					return true, err
				}
				_ = restartRunningService()
				fmt.Println("用户名已更新为:", args[2])
				return true, nil
			case "password":
				if len(args) < 3 {
					return true, errors.New("用法: update password <新密码> [--username <用户名>]")
				}
				name := ""
				for i := 3; i+1 < len(args); i++ {
					if args[i] == "--username" {
						name = args[i+1]
					}
				}
				if name == "" {
					users, _ := loadCLIUsers(cfgDataDir)
					for key := range users {
						name = key
						break
					}
				}
				if name == "" {
					name = "admin"
				}
				if err := updateCLIUserPassword(cfgDataDir, name, args[2]); err != nil {
					return true, err
				}
				_ = restartRunningService()
				fmt.Println("密码已更新")
				return true, nil
			case "port":
				if len(args) < 3 {
					return true, errors.New("用法: update port <端口>")
				}
				port, err := strconv.Atoi(args[2])
				if err != nil || port < 1 || port > 65535 {
					return true, errors.New("端口必须是 1-65535")
				}
				if err := updateServerPort(port); err != nil {
					return true, err
				}
				_ = restartRunningService()
				fmt.Printf("端口已更新为 %d\n", port)
				return true, nil
			case "version":
				if len(args) < 3 {
					return true, errors.New("用法: update version <版本>")
				}
				if !strings.HasPrefix(strings.TrimSpace(args[2]), "v2.") {
					return true, errors.New("版本必须以 v2. 开头")
				}
				if err := saveDomainSettings(cfgDataDir, map[string]any{"version": args[2]}); err != nil {
					return true, err
				}
				fmt.Println("版本更新任务已记录:", args[2])
				return true, nil
			case "entrance":
				if len(args) < 3 {
					return true, errors.New("用法: update entrance <入口后缀>")
				}
				if err := saveSecurityEntrance(cfgDataDir, args[2]); err != nil {
					return true, err
				}
				fmt.Println("安全入口已更新:", strings.Trim(args[2], "/"))
				return true, nil
			}
		}
		// 兼容旧版：update <制品路径> ...
		fallthrough
	case "restore":
		result, err := installSignedArtifact(args[0], args[1:], cfgDataDir)
		if err != nil {
			return true, err
		}
		fmt.Printf("%s 制品已原子安装: %s (sha256=%s)\n", result.Mode, result.Target, result.SHA256)
		return true, nil
	default:
		fmt.Printf("未知命令: %s\n\n", args[0])
		printCLIHelp()
		return true, nil
	}
}

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
func printResetHelp() {
	fmt.Println("重置系统信息")
	fmt.Println("\nUsage:\n  wh reset [command]\n\nAvailable Commands:\n\n  domain      取消访问域名绑定\n  entrance    取消安全入口\n  https       取消 https 登录\n  ips         取消授权 IP 限制\n  mfa         取消两步验证\n  passkey     清空通行密钥\n\nFlags:\n  -h, --help   help for reset\n\nUse \"wh reset [command] --help\" for more information about a command.")
}
func printListenIPHelp() {
	fmt.Println("切换监听 IP")
	fmt.Println("\nUsage:\n  wh listen-ip [command]\n\nAvailable Commands:\n\n  ipv4        监听 IPv4\n  ipv6        监听 IPv6\n\nFlags:\n  -h, --help   help for listen-ip\n\nUse \"wh listen-ip [command] --help\" for more information about a command.")
}
func printUpdateHelp() {
	fmt.Println("修改面板信息")
	fmt.Println("\nUsage:\n  wh update [command]\n\nAvailable Commands:\n\n  entrance    修改安全入口\n  password    修改用户密码\n  port        修改面板端口\n  username    修改用户信息\n  version     更新服务版本\n\nFlags:\n  -h, --help   help for update\n\nUse \"wh update [command] --help\" for more information about a command.")
}

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

func printAppHelp() {
	fmt.Println("应用相关命令\n\nUsage:\n  wh app [command]\n\nAvailable Commands:\n\n  init        初始化应用\n\nFlags:\n  -h, --help             help for app\n  -k, --key string       应用标识\n  -v, --version string   应用版本\n\nUse \"wh app [command] --help\" for more information about a command.")
}

func printLeafHelp(command, title string) {
	fmt.Printf("%s\n\nUsage:\n  wh %s [flags]\n\nFlags:\n  -h, --help   help for %s\n", title, command, command)
}

func loadCLIUsers(dataDir string) (map[string]persistedCLIUser, error) {
	users := map[string]persistedCLIUser{}
	raw, err := os.ReadFile(filepath.Join(dataDir, "users.json"))
	if errors.Is(err, os.ErrNotExist) {
		return users, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &users); err != nil {
		return nil, fmt.Errorf("读取用户数据失败: %w", err)
	}
	return users, nil
}

func printCLIUsers(dataDir string) error {
	users, err := loadCLIUsers(dataDir)
	if err != nil {
		return err
	}
	if len(users) == 0 {
		name := strings.TrimSpace(os.Getenv("WORKMESH_ADMIN_USERNAME"))
		if name == "" {
			name = "admin"
		}
		users[name] = persistedCLIUser{ID: name, Name: name, Role: "ADMIN", Groups: []string{"administrators"}}
	}
	names := make([]string, 0, len(users))
	for name := range users {
		names = append(names, name)
	}
	sort.Strings(names)
	headers := []string{"用户名", "角色", "MFA", "分组"}
	widths := []int{len([]rune(headers[0])), len([]rune(headers[1])), len([]rune(headers[2])), len([]rune(headers[3]))}
	rows := make([][]string, 0, len(names))
	for _, name := range names {
		u := users[name]
		mfa := "禁用"
		if u.MFA {
			mfa = "启用"
		}
		groups := strings.Join(u.Groups, ",")
		row := []string{u.Name, u.Role, mfa, groups}
		rows = append(rows, row)
		for i, v := range row {
			if len([]rune(v)) > widths[i] {
				widths[i] = len([]rune(v))
			}
		}
	}
	fmt.Println(joinCLIColumns(headers, widths))
	for _, row := range rows {
		fmt.Println(joinCLIColumns(row, widths))
	}
	return nil
}
func joinCLIColumns(values []string, widths []int) string {
	var b strings.Builder
	for i, value := range values {
		b.WriteString(value)
		if n := widths[i] - len([]rune(value)); n > 0 {
			b.WriteString(strings.Repeat(" ", n))
		}
		if i < len(values)-1 {
			b.WriteString("  ")
		}
	}
	return b.String()
}

func updateCLIUserName(dataDir, newName string) error {
	newName = strings.TrimSpace(newName)
	if !regexp.MustCompile(`^[a-zA-Z0-9_\x{4e00}-\x{9fa5}]{3,30}$`).MatchString(newName) {
		return errors.New("用户名格式无效（3-30位字母、数字、下划线或中文）")
	}
	users, err := loadCLIUsers(dataDir)
	if err != nil {
		return err
	}
	if len(users) == 0 {
		name := strings.TrimSpace(os.Getenv("WORKMESH_ADMIN_USERNAME"))
		if name == "" {
			name = "admin"
		}
		users[name] = persistedCLIUser{ID: name, Name: name, Role: "ADMIN", Groups: []string{"administrators"}}
	}
	old := ""
	for key := range users {
		old = key
		break
	}
	if item, ok := users[old]; ok {
		delete(users, old)
		item.ID, item.Name = newName, newName
		users[newName] = item
	}
	return saveJSONFile(filepath.Join(dataDir, "users.json"), users)
}
func resetCLIMFA(dataDir, username string) error {
	users, err := loadCLIUsers(dataDir)
	if err != nil {
		return err
	}
	if len(users) == 0 {
		name := strings.TrimSpace(os.Getenv("WORKMESH_ADMIN_USERNAME"))
		if name == "" {
			name = "admin"
		}
		users[name] = persistedCLIUser{ID: name, Name: name, Role: "ADMIN", Groups: []string{"administrators"}}
	}
	if username == "" {
		for key := range users {
			username = key
			break
		}
	}
	user, ok := users[username]
	if !ok {
		return fmt.Errorf("用户不存在: %s", username)
	}
	user.MFA = false
	users[username] = user
	if err := saveJSONFile(filepath.Join(dataDir, "users.json"), users); err != nil {
		return err
	}
	fmt.Println("MFA 已重置:", username)
	return nil
}

func saveDomainSettings(dataDir string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	path := filepath.Join(dataDir, "domains.json")
	doc := map[string]any{"settings": map[string]any{}}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &doc)
	}
	settings, ok := doc["settings"].(map[string]any)
	if !ok {
		settings = map[string]any{}
		doc["settings"] = settings
	}
	for key, value := range values {
		settings[key] = value
	}
	return saveJSONFile(path, doc)
}

func updateServerPort(port int) error {
	path := strings.TrimSpace(os.Getenv("WORKMESH_SERVER_CONFIG"))
	if path == "" {
		path = "config/server.json"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取服务配置失败: %w", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	doc["listenPort"] = port
	if err := saveJSONFile(path, doc); err != nil {
		return err
	}
	// systemd 部署通常通过 server.env 注入 WORKMESH_SERVER_ADDR，必须同步更新，
	// 否则重启时环境变量会覆盖 server.json 的新端口。
	envPath := filepath.Join(filepath.Dir(path), "server.env")
	if raw, err := os.ReadFile(envPath); err == nil {
		lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
		address := "0.0.0.0"
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "WORKMESH_SERVER_ADDR=") {
				value := strings.TrimSpace(strings.SplitN(line, "=", 2)[1])
				if host, _, splitErr := net.SplitHostPort(value); splitErr == nil && host != "" {
					address = host
				}
				break
			}
		}
		updated := false
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "WORKMESH_SERVER_ADDR=") {
				lines[i] = "WORKMESH_SERVER_ADDR=" + net.JoinHostPort(address, strconv.Itoa(port))
				updated = true
				break
			}
		}
		if !updated {
			lines = append(lines, "WORKMESH_SERVER_ADDR="+net.JoinHostPort(address, strconv.Itoa(port)))
		}
		if err := os.WriteFile(envPath+".tmp", []byte(strings.Join(lines, "\n")), 0o600); err != nil {
			return fmt.Errorf("写入服务环境配置失败: %w", err)
		}
		if err := os.Rename(envPath+".tmp", envPath); err != nil {
			return fmt.Errorf("替换服务环境配置失败: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("读取服务环境配置失败: %w", err)
	}
	return nil
}

// restartRunningService 仅在当前机器确实由 systemd 管理服务时重启，开发环境不产生副作用。
func restartRunningService() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	serviceName := strings.TrimSpace(os.Getenv("WORKMESH_SERVICE_NAME"))
	if serviceName == "" {
		serviceName = "workmesh-server.service"
	}
	if err := exec.Command("systemctl", "is-active", "--quiet", serviceName).Run(); err != nil {
		return nil
	}
	return exec.Command("systemctl", "restart", serviceName).Run()
}

// initializeDataDir 创建单进程服务所需的数据目录，避免运行时首次写入失败。
// 所有目录均位于用户配置的 DataDir 下，不使用系统临时目录保存项目状态。
func saveSecurityEntrance(dataDir, entrance string) error {
	entrance = strings.Trim(strings.TrimSpace(entrance), "/")
	if entrance != "" {
		if len(entrance) < 5 || len(entrance) > 116 {
			return errors.New("安全入口长度必须为 5-116 位")
		}
		for _, r := range entrance {
			if (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
				return errors.New("安全入口仅支持数字或字母")
			}
		}
	}
	// Production state lives in SQLite. Keep the JSON path only as a fallback
	// for isolated CLI tests or pre-database installations.
	dbPath := filepath.Join(dataDir, "workmesh.db")
	if _, statErr := os.Stat(dbPath); statErr == nil {
		store, err := storage.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()
		var payload []byte
		if err := store.DB().QueryRow(`SELECT payload FROM functional_domain_state WHERE id=1`).Scan(&payload); err != nil {
			return fmt.Errorf("读取 SQLite 安全设置失败: %w", err)
		}
		var doc map[string]any
		if err := json.Unmarshal(payload, &doc); err != nil {
			return fmt.Errorf("解析 SQLite 安全设置失败: %w", err)
		}
		settings, ok := doc["settings"].(map[string]any)
		if !ok {
			settings = map[string]any{}
			doc["settings"] = settings
		}
		settings["securityEntrance"] = entrance
		updated, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		_, err = store.DB().Exec(`UPDATE functional_domain_state SET payload=?, updated_at=? WHERE id=1`, updated, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}
	path := filepath.Join(dataDir, "domains.json")
	doc := map[string]any{"settings": map[string]any{}}
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &doc)
	}
	settings, ok := doc["settings"].(map[string]any)
	if !ok {
		settings = map[string]any{}
		doc["settings"] = settings
	}
	settings["securityEntrance"] = entrance
	return saveJSONFile(path, doc)
}

func loadSecurityEntrance(dataDir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dataDir, "domains.json"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var doc struct {
		Settings map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", err
	}
	value, _ := doc.Settings["securityEntrance"].(string)
	return strings.Trim(strings.TrimSpace(value), "/"), nil
}

func updateCLIUserPassword(dataDir, username, password string) error {
	path := filepath.Join(dataDir, "users.json")
	users := map[string]persistedCLIUser{}
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &users); err != nil {
			return fmt.Errorf("读取用户数据失败: %w", err)
		}
	}
	user := users[username]
	if user.ID == "" {
		user = persistedCLIUser{ID: username, Name: username, Role: "ADMIN", Groups: []string{"administrators"}}
	}
	user.Password = hashCLIValue(password)
	users[username] = user
	return saveJSONFile(path, users)
}

type persistedCLIUser struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Role     string   `json:"role"`
	Password string   `json:"password"`
	Groups   []string `json:"groups,omitempty"`
	MFA      bool     `json:"mfa"`
}

func hashCLIValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func saveJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func randomSecurityEntrance() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	raw := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "workmesh-entry"
	}
	for i := range raw {
		raw[i] = alphabet[int(raw[i])%len(alphabet)]
	}
	return string(raw)
}

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
	// 仅创建启动必需目录；应用、备份、运行时和上传目录由首次使用的功能按需创建。
	dirs := []string{"logs", "releases"}
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
