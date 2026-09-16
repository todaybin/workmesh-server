// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UpdateWebsiteLoadBalanceFile writes a user-managed upstream fragment and
// validates the complete OpenResty configuration before committing it.
func (s *WebsiteService) UpdateWebsiteLoadBalanceFile(id uint, name, content string) (map[string]any, error) {
	if id == 0 {
		return nil, errors.New("网站 ID 无效")
	}
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	name = safeProxyFileName(name)
	if name == "" {
		return nil, errors.New("负载均衡文件名无效")
	}
	if content == "" || len(content) > 1<<20 || strings.IndexByte(content, 0) >= 0 {
		return nil, errors.New("负载均衡文件内容无效")
	}
	dir := s.SitePath(site, "upstream")
	path := filepath.Join(dir, name)
	old, readErr := os.ReadFile(path)
	existed := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, readErr
	}
	if err := writeWebsiteAtomic(path, []byte(content), 0o640); err != nil {
		return nil, err
	}
	status := s.ProbeOpenResty(context.Background())
	if err := validateCreatedWebsiteConfig(context.Background(), status); err != nil {
		var restoreErr error
		if existed {
			restoreErr = writeWebsiteAtomic(path, old, 0o640)
		} else {
			restoreErr = os.Remove(path)
			if errors.Is(restoreErr, os.ErrNotExist) {
				restoreErr = nil
			}
		}
		return nil, errors.Join(err, restoreErr)
	}
	return map[string]any{"websiteID": id, "name": name, "filePath": path, "content": content}, nil
}

// UpdateWebsiteProxy 将代理表单写入网站设置和 nginx/proxy/managed.conf。
func (s *WebsiteService) UpdateWebsiteProxy(id uint, value map[string]any) (map[string]any, error) {
	if id == 0 {
		return nil, errors.New("网站 ID 无效")
	}
	if value == nil {
		return nil, errors.New("代理配置不能为空")
	}
	return s.UpdateConfig(id, "proxy", value)
}

// DeleteWebsiteProxy 禁用站点代理并清理指定代理文件，避免只删除内存记录。
func (s *WebsiteService) DeleteWebsiteProxy(id uint, name string) error {
	site, err := s.Get(id)
	if err != nil {
		return err
	}
	name = safeProxyFileName(name)
	if name == "" {
		name = "managed.conf"
	}
	path := filepath.Join(s.SitePath(site, "proxy"), name)
	if !withinPath(s.SitePath(site, "proxy"), path) {
		return errors.New("代理文件路径越界")
	}
	if _, err = s.UpdateConfig(id, "proxy", map[string]any{"enabled": false, "name": name, "updatedAt": time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// UpdateWebsiteProxyFile 写入用户明确指定的代理配置文件，文件名必须位于 nginx/proxy 目录内。
func (s *WebsiteService) UpdateWebsiteProxyFile(id uint, name, content string) (map[string]any, error) {
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	name = safeProxyFileName(name)
	if name == "" {
		return nil, errors.New("代理文件名无效")
	}
	if content == "" || len(content) > 1<<20 || strings.IndexByte(content, 0) >= 0 {
		return nil, errors.New("代理文件内容无效")
	}
	dir := s.SitePath(site, "proxy")
	path := filepath.Join(dir, name)
	if !withinPath(dir, path) {
		return nil, errors.New("代理文件路径越界")
	}
	if err := writeWebsiteAtomic(path, []byte(content), 0o640); err != nil {
		return nil, fmt.Errorf("写入代理文件失败: %w", err)
	}
	return map[string]any{"websiteID": id, "name": name, "filePath": path, "content": content, "updatedAt": time.Now().UTC().Format(time.RFC3339Nano)}, nil
}

// safeProxyFileName 只接受当前目录下的 .conf/.bak 文件名，阻断路径穿越和任意文件覆盖。
func safeProxyFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00\r\n") {
		return ""
	}
	if !strings.HasSuffix(name, ".conf") && !strings.HasSuffix(name, ".bak") {
		name += ".conf"
	}
	return name
}
