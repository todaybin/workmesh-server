// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"github.com/todaybin/workmesh-server/node/model"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const managedIncludeSuffix = " " + managedWebsiteIncludeMarker + " include"

// syncManagedWebsiteIncludes 仅重建逐行标记的托管引用，保留用户 include 和 HTTPS 标记。
// upstream 属于 http 作用域，其余托管配置插入 server，禁止把 server 指令提升到 http。
func (s *WebsiteService) syncManagedWebsiteIncludes(site model.Website, content string) string {
	return s.syncManagedWebsiteIncludesPending(site, content, nil)
}

// syncManagedWebsiteIncludesPending 根据本次待写文件的最终内容构建 include，
// 保证首次启用和同次禁用设置时不依赖下一次请求才能更新引用。
func (s *WebsiteService) syncManagedWebsiteIncludesPending(site model.Website, content string, pending map[string][]byte) string {
	legacyManaged := false
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "include ") && strings.HasSuffix(trim, managedIncludeSuffix) {
			continue
		}
		if trim == managedWebsiteIncludeMarker+" managed" {
			legacyManaged = true
			continue
		}
		if legacyManaged && isLegacyManagedInclude(site, trim) {
			continue
		}
		legacyManaged = false
		kept = append(kept, line)
	}
	content = strings.Join(kept, "\n")
	server := regexp.MustCompile(`(?m)^\s*server\s*\{`).FindStringIndex(content)
	if server == nil {
		return content
	}
	header, inside := "", ""
	for _, kind := range []string{"upstream", "proxy", "redirect", "auth_basic", "path_auth", "cors", "realip", "leech", "php"} {
		path := s.managedIncludePathPending(site, kind, pending)
		if path == "" || hasWebsiteInclude(content, path) {
			continue
		}
		line := "include " + path + ";" + managedIncludeSuffix + "\n"
		if kind == "upstream" {
			header += line
		} else {
			inside += "    " + line
		}
	}
	position := server[1]
	if inside != "" {
		if position < len(content) && content[position] == '\n' {
			position++
			content = content[:position] + inside + content[position:]
		} else {
			content = content[:position] + "\n" + inside + content[position:]
		}
	}
	return header + content
}

// isLegacyManagedInclude 仅识别旧版本生成的托管路径，保护同一站点的用户自定义 include。
func isLegacyManagedInclude(site model.Website, line string) bool {
	if !strings.HasPrefix(line, "include ") || !strings.HasSuffix(line, ";") {
		return false
	}
	path := strings.TrimSuffix(strings.TrimPrefix(line, "include "), ";")
	for _, kind := range []string{"upstream", "proxy", "redirect", "auth_basic", "path_auth", "cors", "realip", "leech", "php"} {
		base := filepath.ToSlash(safeWebsitePath(site, kind))
		candidate := filepath.ToSlash(strings.Trim(path, `"`))
		if candidate == base || candidate == base+"/*.conf" {
			return true
		}
	}
	return false
}

// safeWebsitePath 返回站点托管路径，供旧 include 清理逻辑复用且不改变路径校验。
func safeWebsitePath(site model.Website, kind string) string {
	base := strings.TrimSpace(site.SiteDir)
	if base == "" {
		base = filepath.Join("/www/wwwroot", site.PrimaryDomain)
	}
	return filepath.Join(base, "nginx", map[string]string{
		"upstream": "upstream", "proxy": "proxy", "redirect": "redirect", "auth_basic": "auth_basic", "path_auth": "path_auth", "cors": "cors.conf", "realip": "realip.conf", "leech": "leech", "php": "php.conf",
	}[kind])
}

// managedIncludePath 区分普通文件和目录，仅返回存在的常规文件或包含常规配置的 glob。
func (s *WebsiteService) managedIncludePath(site model.Website, kind string) string {
	return s.managedIncludePathPending(site, kind, nil)
}

func (s *WebsiteService) managedIncludePathPending(site model.Website, kind string, pending map[string][]byte) string {
	path := s.SitePath(site, kind)
	if kind == "cors" || kind == "realip" || kind == "php" {
		if content, exists := pending[path]; exists {
			if len(content) > 0 {
				return path
			}
			return ""
		}
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
		return ""
	}
	pattern := filepath.Join(path, "*.conf")
	matches, _ := filepath.Glob(pattern)
	for _, match := range matches {
		if content, exists := pending[match]; exists && len(content) == 0 {
			continue
		}
		if info, err := os.Stat(match); err == nil && info.Mode().IsRegular() {
			return pattern
		}
	}
	for candidate, content := range pending {
		if len(content) > 0 && filepath.Dir(candidate) == path && strings.HasSuffix(candidate, ".conf") {
			return pattern
		}
	}
	return ""
}

// hasWebsiteInclude 避免复制用户已有的同目标引用，不按路径前缀删除自定义配置。
func hasWebsiteInclude(content, path string) bool {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line == "include "+path+";" || line == "include \""+path+"\";" {
			return true
		}
	}
	return false
}
