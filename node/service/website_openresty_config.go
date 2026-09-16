// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

// StandardRules 返回离线可用的基础检测清单。
func (s *WebsiteService) StandardRules() []model.WAFStandardRule {
	return []model.WAFStandardRule{{ID: "CRS-942100", Category: "SQL 注入", Description: "检测联合查询和布尔盲注特征", Locations: []string{"uri", "args", "body", "header", "cookie"}}, {ID: "CRS-941100", Category: "跨站脚本", Description: "检测脚本标签和 javascript 协议", Locations: []string{"uri", "args", "body", "header", "cookie"}}, {ID: "CRS-930110", Category: "本地文件包含", Description: "检测目录穿越和敏感文件读取", Locations: []string{"uri", "args", "body"}}}
}

// GetOpenResty、UpdateOpenResty 管理 OpenResty 状态和配置摘要。
func (s *WebsiteService) GetOpenResty() model.OpenRestyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.openresty
}

// OpenRestyDirective 是 OpenResty 作用域接口使用的原始指令格式。
// 同名指令允许重复出现，例如 limit_conn，因此不能用 map 丢失顺序和重复项。
type OpenRestyDirective struct {
	Name   string   `json:"name"`
	Params []string `json:"params"`
}

func (s *WebsiteService) UpdateOpenResty(cfg model.OpenRestyConfig) (model.OpenRestyConfig, error) {
	cfg.Modules = cloneOpenRestyModules(cfg.Modules)
	s.mu.Lock()
	old := s.openresty
	if strings.TrimSpace(cfg.Version) == "" {
		cfg.Version = s.openresty.Version
	}
	if cfg.Modules == nil {
		cfg.Modules = cloneOpenRestyModules(s.openresty.Modules)
	}
	cfg.UpdatedAt = time.Now().UTC()
	s.openresty = cfg
	if err := s.persist("openresty", cfg); err != nil {
		s.openresty = old
		s.mu.Unlock()
		return model.OpenRestyConfig{}, err
	}
	s.mu.Unlock()

	// 模块、HTTPS 和摘要更新也必须经过同一条真实运行时校验链。
	// 没有可探测的 OpenResty 时保留文件型开发模式；一旦探测到目标，
	// -t、reload 和 reload 后复检任一步失败都回滚控制面状态。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.applyOpenRestyRuntime(ctx); err != nil {
		s.mu.Lock()
		s.openresty = old
		persistErr := s.persist("openresty", old)
		s.mu.Unlock()
		return model.OpenRestyConfig{}, errors.Join(fmt.Errorf("OpenResty 配置校验或 reload 失败: %w", err), persistErr)
	}
	return cfg, nil
}

func cloneOpenRestyModules(modules []model.OpenRestyModule) []model.OpenRestyModule {
	if modules == nil {
		return nil
	}
	cloned := make([]model.OpenRestyModule, len(modules))
	copy(cloned, modules)
	return cloned
}

// OpenRestyFile 返回持久化的 nginx.conf 内容；首次使用时从受控配置文件读取。
func (s *WebsiteService) OpenRestyFile() (string, error) {
	s.mu.RLock()
	content := s.openresty.ConfigContent
	s.mu.RUnlock()
	if content != "" {
		return content, nil
	}
	path := s.openRestyConfigPath()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// 自定义 WAF 镜像通常沿用官方路径，配置页应读取实际 nginx.conf。
		for _, candidate := range []string{"/etc/nginx/nginx.conf", "/usr/local/openresty/nginx/conf/nginx.conf", "/usr/local/openresty/nginx/conf/nginx.conf.default"} {
			if candidate == path {
				continue
			}
			if data, readErr := os.ReadFile(candidate); readErr == nil {
				path, b, err = candidate, data, nil
				break
			}
		}
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("读取 OpenResty 配置失败: %w", err)
	}
	if len(b) > 4<<20 {
		return "", errors.New("OpenResty 配置超过 4 MiB 限制")
	}
	return string(b), nil
}

// UpdateOpenRestyFile 原子写入 nginx.conf，并在需要时保留可回滚备份。
func (s *WebsiteService) UpdateOpenRestyFile(content string, backup bool) error {
	if strings.TrimSpace(content) == "" || strings.IndexByte(content, 0) >= 0 || len(content) > 4<<20 {
		return errors.New("OpenResty 配置内容无效")
	}
	if !balancedConfig(content) {
		return errors.New("OpenResty 配置括号不匹配")
	}
	path := s.openRestyConfigPath()
	old, readErr := os.ReadFile(path)
	oldExists := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("读取 OpenResty 原配置失败: %w", readErr)
	}
	backupPath := path + ".bak"
	oldBackup, backupReadErr := os.ReadFile(backupPath)
	oldBackupExists := backupReadErr == nil
	if backupReadErr != nil && !errors.Is(backupReadErr, os.ErrNotExist) {
		return fmt.Errorf("读取 OpenResty 配置备份失败: %w", backupReadErr)
	}
	if backup && oldExists {
		if err := os.WriteFile(backupPath, old, 0o600); err != nil {
			return fmt.Errorf("保存 OpenResty 配置备份失败: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("创建 OpenResty 配置目录失败: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return fmt.Errorf("写入 OpenResty 临时配置失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("替换 OpenResty 配置失败: %w", err)
	}
	s.mu.Lock()
	oldState := s.openresty
	s.openresty.ConfigContent = content
	s.openresty.UpdatedAt = time.Now().UTC()
	err := s.persist("openresty", s.openresty)
	s.mu.Unlock()
	if err != nil {
		// 数据库状态写入失败时恢复原配置，避免文件与控制面状态分叉。
		s.mu.Lock()
		s.openresty = oldState
		s.mu.Unlock()
		var restoreErr error
		if !oldExists {
			restoreErr = os.Remove(path)
			if errors.Is(restoreErr, os.ErrNotExist) {
				restoreErr = nil
			}
		} else {
			restoreErr = atomicWriteFile(path, old, 0o600)
		}
		var backupRestoreErr error
		if backup && oldExists {
			if oldBackupExists {
				backupRestoreErr = atomicWriteFile(backupPath, oldBackup, 0o600)
			} else {
				backupRestoreErr = os.Remove(backupPath)
				if errors.Is(backupRestoreErr, os.ErrNotExist) {
					backupRestoreErr = nil
				}
			}
		}
		return errors.Join(fmt.Errorf("保存 OpenResty 配置状态失败: %w", err), restoreErr, backupRestoreErr)
	}
	runtimeCtx, cancelRuntime := context.WithTimeout(context.Background(), 30*time.Second)
	runtimeErr := s.applyOpenRestyRuntime(runtimeCtx)
	cancelRuntime()
	if runtimeErr != nil {
		s.mu.Lock()
		s.openresty = oldState
		persistErr := s.persist("openresty", oldState)
		s.mu.Unlock()
		restoreErr := restoreOpenRestyFile(path, old, oldExists)
		var backupRestoreErr error
		if backup && oldExists {
			if oldBackupExists {
				backupRestoreErr = atomicWriteFile(backupPath, oldBackup, 0o600)
			} else {
				backupRestoreErr = os.Remove(backupPath)
				if errors.Is(backupRestoreErr, os.ErrNotExist) {
					backupRestoreErr = nil
				}
			}
		}
		return errors.Join(fmt.Errorf("OpenResty 配置校验或 reload 失败: %w", runtimeErr), persistErr, restoreErr, backupRestoreErr)
	}
	return nil
}

func restoreOpenRestyFile(path string, old []byte, existed bool) error {
	if existed {
		return atomicWriteFile(path, old, 0o600)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// applyOpenRestyRuntime validates and reloads a real OpenResty instance when
// one is available. File-only development/test services remain persistence-only.
func (s *WebsiteService) applyOpenRestyRuntime(ctx context.Context) error {
	status := s.ProbeOpenResty(ctx)
	if !status.Available {
		return nil
	}
	if !status.ConfigValid {
		return errors.New(status.Error)
	}
	var output []byte
	var err error
	if strings.HasPrefix(status.Binary, "docker://") {
		name := strings.TrimPrefix(status.Binary, "docker://")
		output, err = runContainerOpenRestyCommand(ctx, dockerBinaryOrName(), name, "-t")
		if err == nil {
			output, err = runContainerOpenRestyCommand(ctx, dockerBinaryOrName(), name, "-s", "reload")
		}
	} else if strings.HasPrefix(status.Binary, "proc://") {
		return errors.New("OpenResty 位于无法控制的独立命名空间，不能确认 reload 已生效")
	} else {
		output, err = exec.CommandContext(ctx, status.Binary, "-t").CombinedOutput()
		if err == nil {
			output, err = exec.CommandContext(ctx, status.Binary, "-s", "reload").CombinedOutput()
		}
	}
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return errors.New(message)
	}
	confirmed := s.ProbeOpenResty(ctx)
	if !confirmed.Available || !confirmed.ConfigValid {
		if confirmed.Error != "" {
			return errors.New(confirmed.Error)
		}
		return errors.New("OpenResty reload 后配置确认失败")
	}
	return nil
}

// OpenRestyScope 读取指定作用域下的白名单指令，防止任意字段写入配置。
func (s *WebsiteService) OpenRestyScope(scope string) ([]OpenRestyDirective, error) {
	keys, ok := openRestyScopeKeys(strings.TrimSpace(scope))
	if !ok {
		return nil, errors.New("OpenResty 配置作用域无效")
	}
	content, err := s.OpenRestyFile()
	if err != nil {
		return nil, err
	}
	result := make([]OpenRestyDirective, 0, len(keys))
	for _, key := range keys {
		found := false
		for _, line := range strings.Split(content, "\n") {
			fields := strings.Fields(strings.TrimSuffix(strings.TrimSpace(line), ";"))
			if len(fields) < 2 || fields[0] != key {
				continue
			}
			result = append(result, OpenRestyDirective{Name: key, Params: append([]string(nil), fields[1:]...)})
			found = true
		}
		if !found {
			result = append(result, OpenRestyDirective{Name: key, Params: []string{}})
		}
	}
	return result, nil
}

// UpdateOpenRestyScope 更新作用域白名单指令并复用原子配置写入流程。
func (s *WebsiteService) UpdateOpenRestyScope(scope string, params map[string]string, backup bool) error {
	keys, ok := openRestyScopeKeys(strings.TrimSpace(scope))
	if !ok || len(params) == 0 || len(params) > 32 {
		return errors.New("OpenResty 作用域参数无效")
	}
	allowed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		allowed[key] = struct{}{}
	}
	for key, value := range params {
		if _, exists := allowed[key]; !exists || strings.TrimSpace(value) == "" || len(value) > 256 || strings.ContainsAny(value, "{};\x00") {
			return fmt.Errorf("OpenResty 指令 %q 不允许或值无效", key)
		}
	}
	content, err := s.OpenRestyFile()
	if err != nil {
		return err
	}
	updated := updateOpenRestyHTTPDirectives(content, params)
	return s.UpdateOpenRestyFile(updated, backup)
}

var openRestyHTTPBlockPattern = regexp.MustCompile(`^\s*http\s*\{`)

func updateOpenRestyHTTPDirectives(content string, params map[string]string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	seen := make(map[string]bool, len(params))
	result := make([]string, 0, len(lines)+len(params)+3)
	depth := 0
	httpStart, httpEnd := -1, -1
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if depth == 0 && openRestyHTTPBlockPattern.MatchString(line) {
			httpStart = len(result)
		}
		if depth == 0 && isOpenRestyDirectiveLine(trimmed, params) {
			continue
		}
		if httpStart >= 0 && httpEnd < 0 && depth == 1 {
			for key, value := range params {
				if openRestyLineHasDirective(trimmed, key) {
					indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
					line = indent + key + " " + value + ";"
					seen[key] = true
					break
				}
			}
		}
		result = append(result, line)
		depth += openRestyBraceDelta(line)
		if httpStart >= 0 && httpEnd < 0 && depth == 0 {
			httpEnd = len(result) - 1
		}
	}
	missing := make([]string, 0, len(params))
	missingKeys := make([]string, 0, len(params))
	for key := range params {
		if !seen[key] {
			missingKeys = append(missingKeys, key)
		}
	}
	sort.Strings(missingKeys)
	for _, key := range missingKeys {
		missing = append(missing, key+" "+params[key]+";")
	}
	if len(missing) == 0 {
		return strings.Join(result, "\n")
	}
	if httpStart < 0 {
		if len(result) > 0 && strings.TrimSpace(result[len(result)-1]) != "" {
			result = append(result, "")
		}
		result = append(result, "http {")
		for _, line := range missing {
			result = append(result, "    "+line)
		}
		result = append(result, "}")
		return strings.Join(result, "\n")
	}
	startLine := result[httpStart]
	startIndent := startLine[:len(startLine)-len(strings.TrimLeft(startLine, " \t"))]
	childIndent := startIndent + "    "
	insert := make([]string, 0, len(missing))
	for _, line := range missing {
		insert = append(insert, childIndent+line)
	}
	if httpEnd == httpStart {
		replacement := append([]string{startIndent + "http {"}, insert...)
		replacement = append(replacement, startIndent+"}")
		updated := make([]string, 0, len(result)+len(replacement)-1)
		updated = append(updated, result[:httpStart]...)
		updated = append(updated, replacement...)
		updated = append(updated, result[httpStart+1:]...)
		return strings.Join(updated, "\n")
	}
	updated := make([]string, 0, len(result)+len(insert))
	updated = append(updated, result[:httpEnd]...)
	updated = append(updated, insert...)
	updated = append(updated, result[httpEnd:]...)
	return strings.Join(updated, "\n")
}

func isOpenRestyDirectiveLine(trimmed string, params map[string]string) bool {
	for key := range params {
		if openRestyLineHasDirective(trimmed, key) {
			return true
		}
	}
	return false
}

func openRestyLineHasDirective(trimmed, key string) bool {
	if strings.HasPrefix(trimmed, "#") {
		return false
	}
	return strings.HasPrefix(trimmed, key+" ") || strings.HasPrefix(trimmed, key+"\t") || strings.HasPrefix(trimmed, key+";")
}

func openRestyBraceDelta(line string) int {
	if index := strings.IndexByte(line, '#'); index >= 0 {
		line = line[:index]
	}
	delta := 0
	for _, r := range line {
		switch r {
		case '{':
			delta++
		case '}':
			delta--
		}
	}
	return delta
}
