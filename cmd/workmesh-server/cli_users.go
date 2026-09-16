// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// persistedCLIUser 是 CLI 兼容用户文件中的最小用户表示。
type persistedCLIUser struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Role     string   `json:"role"`
	Password string   `json:"password"`
	Groups   []string `json:"groups,omitempty"`
	MFA      bool     `json:"mfa"`
}

// loadCLIUsers 从数据目录读取 CLI 用户兼容文件。
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

// printCLIUsers 按稳定列宽输出本地 CLI 用户列表。
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

// joinCLIColumns 按中文字符宽度拼接一行表格内容。
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

// updateCLIUserName 校验并持久化默认 CLI 用户名变更。
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

// resetCLIMFA 清除指定 CLI 用户的多因素认证状态。
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

// updateCLIUserPassword 更新指定 CLI 用户的密码摘要。
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

// hashCLIValue 计算 CLI 敏感值的 SHA-256 摘要。
func hashCLIValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
