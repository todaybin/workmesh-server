// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/todaybin/workmesh-server/node/model"
)

// containsString 判断字符串切片中是否已经存在目标值，避免重复生成 index 文档。
func containsString(items []string, wanted string) bool {
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}

// UpdateWebsiteNginxIndex 更新站点 site.conf 中的 index 指令，并保留原文件以便失败恢复。
func (s *WebsiteService) UpdateWebsiteNginxIndex(websiteID uint, documents []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	site, err := s.getWebsiteLocked(websiteID)
	if err != nil {
		return err
	}
	path := s.SitePath(site, "site.conf")
	old, readErr := os.ReadFile(path)
	if readErr != nil {
		return readErr
	}
	clean := make([]string, 0, len(documents))
	for _, item := range documents {
		item = strings.TrimSpace(item)
		if item != "" && !strings.ContainsAny(item, ";\r\n") {
			clean = append(clean, item)
		}
	}
	if len(clean) == 0 {
		return errors.New("默认文档不能为空")
	}
	line := "index " + strings.Join(clean, " ") + ";"
	updated := regexp.MustCompile(`(?m)^\s*index\s+[^;]+;\s*$`).ReplaceAllString(string(old), line)
	if updated == string(old) && !strings.Contains(string(old), line) {
		updated = strings.TrimRight(string(old), "\n") + "\n" + line + "\n"
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(updated), 0o640); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return errors.Join(err, writeWebsiteAtomic(path, old, 0o640))
	}
	return nil
}

// ListWebsiteProxies 读取域名目录 nginx/proxy 下的 .conf/.bak 文件，返回 1Panel 代理配置字段。
func (s *WebsiteService) ListWebsiteProxies(websiteID uint) ([]map[string]any, error) {
	site, err := s.Get(websiteID)
	if err != nil {
		return nil, err
	}
	// 兼容历史反向站点：旧版本只把 proxy 写进 site.conf，首次进入菜单时补齐 root.conf。
	if err := s.ensureReverseProxyConfig(websiteID); err != nil {
		return nil, err
	}
	site, err = s.Get(websiteID)
	if err != nil {
		return nil, err
	}
	dir := s.SitePath(site, "proxy")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	proxies := make([]map[string]any, 0)
	for _, entry := range entries {
		if entry.IsDir() || !(strings.HasSuffix(entry.Name(), ".conf") || strings.HasSuffix(entry.Name(), ".bak")) {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		name := strings.TrimSuffix(strings.TrimSuffix(entry.Name(), ".conf"), ".bak")
		proxyPass := firstRegexpValue(`(?m)\bproxy_pass\s+([^;\s]+)`, string(content))
		proxyHost := firstRegexpValue(`(?m)\bproxy_set_header\s+Host\s+([^;\s]+)`, string(content))
		match, modifier := parseProxyLocation(string(content))
		contentText := string(content)
		cacheTime, cacheUnit := parseDurationDirective(contentText, `(?m)\bexpires\s+([^;\s]+)`)
		serverCacheTime, serverCacheUnit := parseDurationDirective(contentText, `(?m)\bproxy_cache_valid\s+200\s+304\s+301\s+302\s+([^;\s]+)`)
		replaces := map[string]string{}
		for _, m := range regexp.MustCompile(`(?m)\bsub_filter\s+"((?:\\.|[^"])*)"\s+"((?:\\.|[^"])*)"`).FindAllStringSubmatch(contentText, -1) {
			if len(m) > 2 {
				replaces[m[1]] = m[2]
			}
		}
		cache := strings.TrimSpace(firstRegexpValue(`(?m)\bproxy_cache\s+([^;\s]+)`, contentText)) != ""
		cors := strings.Contains(contentText, "Access-Control-Allow-Origin")
		proxies = append(proxies, map[string]any{
			"id": websiteID, "name": name, "enable": strings.HasSuffix(entry.Name(), ".conf"),
			"proxyPass": proxyPass, "proxyHost": proxyHost, "match": match, "modifier": modifier,
			"content": string(content), "filePath": filepath.Join(dir, entry.Name()),
			"cache": cache, "cacheTime": cacheTime, "cacheUnit": cacheUnit, "serverCacheTime": serverCacheTime, "serverCacheUnit": serverCacheUnit,
			"replaces": replaces, "sni": strings.Contains(contentText, "proxy_ssl_server_name on"),
			"proxySSLName": firstRegexpValue(`(?m)\bproxy_ssl_name\s+([^;\s]+)`, string(content)),
			"sslVerify":    strings.Contains(contentText, "proxy_ssl_verify on"), "cors": cors,
			"allowOrigins":     firstRegexpValue(`(?m)\bAccess-Control-Allow-Origin\s+([^;\s]+)`, contentText),
			"allowMethods":     firstRegexpValue(`(?m)\bAccess-Control-Allow-Methods\s+([^;\s]+)`, contentText),
			"allowHeaders":     firstRegexpValue(`(?m)\bAccess-Control-Allow-Headers\s+([^;\s]+)`, contentText),
			"allowCredentials": strings.Contains(contentText, "Access-Control-Allow-Credentials true"),
			"preflight":        strings.Contains(contentText, "$request_method = 'OPTIONS'"),
		})
	}
	sort.Slice(proxies, func(i, j int) bool { return fmt.Sprint(proxies[i]["name"]) < fmt.Sprint(proxies[j]["name"]) })
	return proxies, nil
}

func parseDurationDirective(content, pattern string) (int, string) {
	raw := firstRegexpValue(pattern, content)
	if raw == "" {
		return 0, ""
	}
	unit := raw[len(raw)-1:]
	number := raw
	if unit < "0" || unit > "9" {
		number = raw[:len(raw)-1]
	} else {
		unit = "s"
	}
	n, err := strconv.Atoi(number)
	if err != nil {
		return 0, ""
	}
	return n, unit
}

// firstRegexpValue 返回正则表达式第一个捕获组的去空白结果。
func firstRegexpValue(pattern, content string) string {
	match := regexp.MustCompile(pattern).FindStringSubmatch(content)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

// parseProxyLocation 解析代理配置中的 location 匹配路径和可选修饰符。
func parseProxyLocation(content string) (string, string) {
	match := regexp.MustCompile(`(?m)^\s*location\s+([^\s{]+)(?:\s+([^\s{]+))?\s*\{`).FindStringSubmatch(content)
	if len(match) < 2 {
		return "", ""
	}
	if len(match) > 2 && (match[1] == "=" || strings.HasPrefix(match[1], "~")) {
		return strings.TrimSpace(match[2]), strings.TrimSpace(match[1])
	}
	return strings.TrimSpace(match[1]), ""
}

// BasicConfig 返回站点 Basic 页面所需的真实目录、配置和日志状态。
func (s *WebsiteService) BasicConfig(id uint) (map[string]any, error) {
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	contentBytes, configErr := os.ReadFile(s.SitePath(site, "site.conf"))
	content := string(contentBytes)
	if configErr != nil {
		content, configErr = s.OpenRestyFile()
	}
	defaultDocuments := []string{}
	if configured, ok := s.configs[id]["index"].(map[string]any); ok {
		if value, ok := configured["documents"].([]any); ok {
			for _, item := range value {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					defaultDocuments = append(defaultDocuments, strings.TrimSpace(text))
				}
			}
		}
		if value, ok := configured["content"].(string); ok && value != "" {
			defaultDocuments = append(defaultDocuments, strings.Fields(value)...)
		}
	}
	if configErr == nil {
		for _, match := range regexp.MustCompile(`(?m)^\s*index\s+([^;]+);`).FindAllStringSubmatch(content, -1) {
			defaultDocuments = append(defaultDocuments, strings.Fields(match[1])...)
		}
	}
	if len(defaultDocuments) == 0 {
		defaultDocuments = []string{"index.php", "index.html", "index.htm", "default.php", "default.htm", "default.html"}
	}
	accessPath, errorPath := s.websiteLogPath(site, "access.log"), s.websiteLogPath(site, "error.log")
	logStatus := func(path string, enabled bool) map[string]any {
		entry := map[string]any{"path": path, "enabled": enabled, "exists": false}
		if !enabled {
			entry["status"] = "disabled"
			return entry
		}
		if _, statErr := os.Stat(path); statErr == nil {
			entry["exists"], entry["status"] = true, "available"
		} else if errors.Is(statErr, os.ErrNotExist) {
			entry["status"] = "missing"
		} else {
			entry["status"], entry["error"] = "unavailable", statErr.Error()
		}
		return entry
	}
	traffic := map[string]any{"status": "missing", "requests": 0, "bytes": int64(0), "source": accessPath}
	if data, readErr := os.ReadFile(accessPath); readErr == nil {
		requests, bytes := accessLogTraffic(string(data))
		traffic = map[string]any{"status": "available", "requests": requests, "bytes": bytes, "source": accessPath}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		traffic["status"], traffic["error"] = "unavailable", readErr.Error()
	}
	configError := ""
	if configErr != nil {
		configError = configErr.Error()
	}
	return map[string]any{
		"website": site, "id": site.ID, "domains": append([]model.WebsiteDomain(nil), site.Domains...),
		"siteDir": site.SiteDir, "sitePath": site.SitePath, "root": site.Root,
		"defaultDocuments": defaultDocuments, "accessLog": logStatus(accessPath, site.AccessLog), "errorLog": logStatus(errorPath, site.ErrorLog),
		"traffic": traffic, "proxy": site.Proxy, "https": site.Protocol == "HTTPS", "rewrite": site.Rewrite,
		"configPath": s.openRestyConfigPath(), "configError": configError,
	}, nil
}

// accessLogTraffic 统计访问日志行数和标准 combined log 第 10 列的响应字节数。
func accessLogTraffic(content string) (int, int64) {
	requests := 0
	var bytes int64
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		requests++
		if len(fields) >= 10 {
			if value, err := strconv.ParseInt(fields[9], 10, 64); err == nil && value > 0 {
				bytes += value
			}
		}
	}
	return requests, bytes
}
