// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// saveDomainSettings 合并保存兼容的域名设置文件。
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

// updateServerPort 更新服务端口并同步 systemd 环境覆盖值。
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

// restartRunningService 仅在目标服务处于活动状态时请求 systemd 重启。
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

// saveSecurityEntrance 校验并保存安全入口到统一存储或兼容文件。
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

// loadSecurityEntrance 从兼容域名设置文件读取安全入口。
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

// loadCLISettings 读取本地 CLI 设置，不存在时返回空设置。
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

// saveCLISettings 以安全替换方式保存本地 CLI 设置。
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
