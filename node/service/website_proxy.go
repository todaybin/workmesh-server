// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
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

// initializeReverseProxySite 为反向站点创建与代理菜单一致的默认 root.conf。
func (s *WebsiteService) initializeReverseProxySite(site model.Website) error {
	if !strings.EqualFold(strings.TrimSpace(site.Type), "proxy") || strings.TrimSpace(site.Proxy) == "" {
		return nil
	}
	files, err := s.renderNamedProxyFiles(site, "root", map[string]any{
		"enabled": true, "proxyPass": site.Proxy, "match": "/", "proxyHost": "$host",
	})
	if err == nil {
		for path, content := range files {
			if err = writeWebsiteAtomic(path, content, 0o640); err != nil {
				break
			}
		}
	}
	return err
}

// renderNamedProxyFiles 将结构化代理设置渲染到指定名称的代理文件。
func (s *WebsiteService) renderNamedProxyFiles(site model.Website, name string, value map[string]any) (map[string][]byte, error) {
	name = proxyFileStem(name)
	if name == "" {
		return nil, errors.New("代理名称无效")
	}
	files := map[string][]byte{}
	write := func(_ string, content string) {
		files[filepath.Join(s.SitePath(site, "proxy"), name+".conf")] = []byte(content)
	}
	_, err := s.renderWebsiteProxySetting(site, value, files, write)
	if err != nil {
		return nil, err
	}
	return files, nil
}

// UpdateNamedWebsiteProxy 创建或编辑菜单中的单个命名代理。
func (s *WebsiteService) UpdateNamedWebsiteProxy(id uint, name, operate string, value map[string]any) (map[string]any, error) {
	if id == 0 || value == nil {
		return nil, errors.New("代理配置参数无效")
	}
	operate = strings.ToLower(strings.TrimSpace(operate))
	if operate == "" {
		// 旧客户端省略 operate 时等价于新增代理配置。
		operate = "create"
	}
	if operate != "create" && operate != "edit" {
		return nil, errors.New("代理操作必须是 create 或 edit")
	}
	name = proxyFileStem(name)
	if name == "" {
		return nil, errors.New("代理名称无效")
	}
	status := s.ProbeOpenResty(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	site, err := s.getWebsiteLocked(id)
	if err != nil {
		return nil, err
	}
	dir := s.SitePath(site, "proxy")
	confPath := filepath.Join(dir, name+".conf")
	bakPath := filepath.Join(dir, name+".bak")
	if operate == "create" {
		if _, statErr := os.Stat(confPath); statErr == nil {
			return nil, errors.New("代理名称已存在")
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return nil, statErr
		}
		if _, statErr := os.Stat(bakPath); statErr == nil {
			return nil, errors.New("代理名称已存在")
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return nil, statErr
		}
	} else if _, statErr := os.Stat(confPath); statErr != nil {
		return nil, fmt.Errorf("代理配置不存在或已禁用: %w", statErr)
	}
	oldFiles, err := snapshotWebsiteFiles(confPath, bakPath, s.SitePath(site, "site.conf"))
	if err != nil {
		return nil, err
	}
	files, err := s.renderNamedProxyFiles(site, name, value)
	if err != nil {
		return nil, err
	}
	current, err := os.ReadFile(s.SitePath(site, "site.conf"))
	if err != nil {
		return nil, err
	}
	files[s.SitePath(site, "site.conf")] = []byte(s.syncManagedWebsiteIncludesPending(site, string(current), files))
	for path, content := range files {
		if err := writeWebsiteAtomic(path, content, 0o640); err != nil {
			return nil, errors.Join(err, restoreWebsiteFiles(oldFiles))
		}
	}
	if err := validateCreatedWebsiteConfig(context.Background(), status); err != nil {
		return nil, errors.Join(err, restoreWebsiteFiles(oldFiles))
	}
	if s.configs[id] == nil {
		s.configs[id] = map[string]any{}
	}
	oldConfig, hadOldConfig := s.configs[id]["proxy:"+name]
	persisted := cloneMap(value)
	persisted["enabled"], persisted["name"] = true, name
	s.configs[id]["proxy:"+name] = persisted
	if err := s.persist("website-configs", s.configs); err != nil {
		if hadOldConfig {
			s.configs[id]["proxy:"+name] = oldConfig
		} else {
			delete(s.configs[id], "proxy:"+name)
		}
		return nil, errors.Join(err, restoreWebsiteFiles(oldFiles))
	}
	persisted["id"], persisted["filePath"] = id, confPath
	return persisted, nil
}

// UpdateNamedWebsiteProxyStatus 原子切换单个代理的 .conf/.bak 状态。
func (s *WebsiteService) UpdateNamedWebsiteProxyStatus(id uint, name string, enabled bool) error {
	name = proxyFileStem(name)
	if id == 0 || name == "" {
		return errors.New("代理状态参数无效")
	}
	status := s.ProbeOpenResty(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	site, err := s.getWebsiteLocked(id)
	if err != nil {
		return err
	}
	if s.configs[id] == nil {
		s.configs[id] = map[string]any{}
	}
	oldConfig, hadOldConfig := s.configs[id]["proxy:"+name]
	dir := s.SitePath(site, "proxy")
	confPath, bakPath := filepath.Join(dir, name+".conf"), filepath.Join(dir, name+".bak")
	oldFiles, err := snapshotWebsiteFiles(confPath, bakPath, s.SitePath(site, "site.conf"))
	if err != nil {
		return err
	}
	if enabled {
		if _, err := os.Stat(confPath); errors.Is(err, os.ErrNotExist) {
			if err := os.Rename(bakPath, confPath); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if err := os.Remove(bakPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else {
		if _, err := os.Stat(bakPath); errors.Is(err, os.ErrNotExist) {
			if err := os.Rename(confPath, bakPath); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := os.Remove(confPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	current, err := os.ReadFile(s.SitePath(site, "site.conf"))
	if err != nil {
		return err
	}
	updated := s.syncManagedWebsiteIncludes(site, string(current))
	if err := writeWebsiteAtomic(s.SitePath(site, "site.conf"), []byte(updated), 0o640); err != nil {
		return errors.Join(err, restoreWebsiteFiles(oldFiles))
	}
	if err := validateCreatedWebsiteConfig(context.Background(), status); err != nil {
		return errors.Join(err, restoreWebsiteFiles(oldFiles))
	}
	if cfg, ok := s.configs[id]["proxy:"+name].(map[string]any); ok {
		cfg["enabled"] = enabled
	} else if enabled {
		content, readErr := os.ReadFile(confPath)
		if readErr == nil {
			if proxyPass := firstRegexpValue(`(?m)\bproxy_pass\s+([^;\s]+)`, string(content)); proxyPass != "" {
				s.configs[id]["proxy:"+name] = map[string]any{"enabled": true, "name": name, "proxyPass": proxyPass}
			}
		}
	}
	if err := s.persist("website-configs", s.configs); err != nil {
		if hadOldConfig {
			s.configs[id]["proxy:"+name] = oldConfig
		} else {
			delete(s.configs[id], "proxy:"+name)
		}
		return errors.Join(err, restoreWebsiteFiles(oldFiles))
	}
	return nil
}

// DeleteWebsiteProxy 禁用站点代理并清理指定代理文件，避免只删除内存记录。
func (s *WebsiteService) DeleteWebsiteProxy(id uint, name string) error {
	if id == 0 {
		return errors.New("网站 ID 无效")
	}
	stem := proxyFileStem(name)
	if stem == "" {
		return errors.New("代理名称无效")
	}
	status := s.ProbeOpenResty(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	site, err := s.getWebsiteLocked(id)
	if err != nil {
		return err
	}
	confPath, bakPath := filepath.Join(s.SitePath(site, "proxy"), stem+".conf"), filepath.Join(s.SitePath(site, "proxy"), stem+".bak")
	oldFiles, err := snapshotWebsiteFiles(confPath, bakPath, s.SitePath(site, "site.conf"))
	if err != nil {
		return err
	}
	if err := os.Remove(confPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(bakPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	current, err := os.ReadFile(s.SitePath(site, "site.conf"))
	if err != nil {
		return err
	}
	if err := writeWebsiteAtomic(s.SitePath(site, "site.conf"), []byte(s.syncManagedWebsiteIncludes(site, string(current))), 0o640); err != nil {
		return errors.Join(err, restoreWebsiteFiles(oldFiles))
	}
	if err := validateCreatedWebsiteConfig(context.Background(), status); err != nil {
		return errors.Join(err, restoreWebsiteFiles(oldFiles))
	}
	delete(s.configs[id], "proxy:"+stem)
	if err := s.persist("website-configs", s.configs); err != nil {
		return errors.Join(err, restoreWebsiteFiles(oldFiles))
	}
	return nil
}

// UpdateWebsiteProxyFile 写入用户明确指定的代理配置文件，文件名必须位于 nginx/proxy 目录内。
func (s *WebsiteService) UpdateWebsiteProxyFile(id uint, name, content string) (map[string]any, error) {
	if id == 0 {
		return nil, errors.New("网站 ID 无效")
	}
	name = proxyFileStem(name)
	if name == "" {
		return nil, errors.New("代理文件名无效")
	}
	status := s.ProbeOpenResty(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	site, err := s.getWebsiteLocked(id)
	if err != nil {
		return nil, err
	}
	if content == "" || len(content) > 1<<20 || strings.IndexByte(content, 0) >= 0 {
		return nil, errors.New("代理文件内容无效")
	}
	dir := s.SitePath(site, "proxy")
	path := filepath.Join(dir, name+".conf")
	if !withinPath(dir, path) {
		return nil, errors.New("代理文件路径越界")
	}
	oldFiles, err := snapshotWebsiteFiles(path, s.SitePath(site, "site.conf"))
	if err != nil {
		return nil, err
	}
	if err := writeWebsiteAtomic(path, []byte(content), 0o640); err != nil {
		return nil, fmt.Errorf("写入代理文件失败: %w", err)
	}
	current, err := os.ReadFile(s.SitePath(site, "site.conf"))
	if err != nil {
		return nil, errors.Join(err, restoreWebsiteFiles(oldFiles))
	}
	if err := writeWebsiteAtomic(s.SitePath(site, "site.conf"), []byte(s.syncManagedWebsiteIncludes(site, string(current))), 0o640); err != nil {
		return nil, errors.Join(err, restoreWebsiteFiles(oldFiles))
	}
	if err := validateCreatedWebsiteConfig(context.Background(), status); err != nil {
		return nil, errors.Join(err, restoreWebsiteFiles(oldFiles))
	}
	if s.configs[id] == nil {
		s.configs[id] = map[string]any{}
	}
	oldConfig, hadOldConfig := s.configs[id]["proxy:"+name]
	s.configs[id]["proxy:"+name] = map[string]any{"enabled": true, "name": name, "content": content}
	if err := s.persist("website-configs", s.configs); err != nil {
		if hadOldConfig {
			s.configs[id]["proxy:"+name] = oldConfig
		} else {
			delete(s.configs[id], "proxy:"+name)
		}
		return nil, errors.Join(err, restoreWebsiteFiles(oldFiles))
	}
	return map[string]any{"websiteID": id, "name": name, "filePath": path, "content": content, "updatedAt": time.Now().UTC().Format(time.RFC3339Nano)}, nil
}

func (s *WebsiteService) ensureReverseProxyConfig(id uint) error {
	site, err := s.Get(id)
	if err != nil || !strings.EqualFold(strings.TrimSpace(site.Type), "proxy") || strings.TrimSpace(site.Proxy) == "" {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := s.SitePath(site, "proxy")
	matches, _ := filepath.Glob(filepath.Join(dir, "*.conf"))
	if len(matches) > 0 {
		return nil
	}
	if err := s.initializeReverseProxySite(site); err != nil {
		return err
	}
	current, err := os.ReadFile(s.SitePath(site, "site.conf"))
	if err != nil {
		return err
	}
	if err := writeWebsiteAtomic(s.SitePath(site, "site.conf"), []byte(s.syncManagedWebsiteIncludes(site, string(current))), 0o640); err != nil {
		return err
	}
	if s.configs[id] == nil {
		s.configs[id] = map[string]any{}
	}
	s.configs[id]["proxy:root"] = map[string]any{"enabled": true, "name": "root", "proxyPass": site.Proxy, "match": "/", "proxyHost": "$host"}
	return s.persist("website-configs", s.configs)
}

// reconcileReverseProxyFiles 在启动时补齐历史反向站点的菜单配置。
func (s *WebsiteService) reconcileReverseProxyFiles() {
	changed := false
	for _, site := range s.websites {
		if !strings.EqualFold(strings.TrimSpace(site.Type), "proxy") || strings.TrimSpace(site.Proxy) == "" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(s.SitePath(site, "proxy"), "*.conf"))
		if len(matches) > 0 {
			continue
		}
		if err := s.initializeReverseProxySite(site); err != nil {
			continue
		}
		path := s.SitePath(site, "site.conf")
		content, err := os.ReadFile(path)
		if err != nil || writeWebsiteAtomic(path, []byte(s.syncManagedWebsiteIncludes(site, string(content))), 0o640) != nil {
			continue
		}
		if s.configs[site.ID] == nil {
			s.configs[site.ID] = map[string]any{}
		}
		s.configs[site.ID]["proxy:root"] = map[string]any{"enabled": true, "name": "root", "proxyPass": site.Proxy, "match": "/", "proxyHost": "$host"}
		changed = true
	}
	if changed {
		_ = s.persist("website-configs", s.configs)
	}
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

func proxyFileStem(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00\r\n") {
		return ""
	}
	name = strings.TrimSuffix(strings.TrimSuffix(name, ".conf"), ".bak")
	if name == "" || name == "." || name == ".." {
		return ""
	}
	return name
}

func cloneMap(value map[string]any) map[string]any {
	encoded, _ := json.Marshal(value)
	var cloned map[string]any
	if json.Unmarshal(encoded, &cloned) != nil || cloned == nil {
		cloned = map[string]any{}
	}
	return cloned
}
