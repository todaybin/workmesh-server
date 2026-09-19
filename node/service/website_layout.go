// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/model"
	"os"
	"path/filepath"
	"strings"
)

// siteRootPath 返回网站目录根；新站点配置与入口文件均按域名隔离。
func (s *WebsiteService) siteRootPath() string {
	if configured := strings.TrimSpace(s.websiteRoot); configured != "" {
		return filepath.Clean(configured)
	}
	return "/www/wwwroot"
}

func (s *WebsiteService) siteDirPath(domain string) string {
	return filepath.Join(s.siteRootPath(), domain)
}

// SitePath 返回站点域名目录下的标准路径，供 API 和文件操作统一使用。
func (s *WebsiteService) SitePath(site model.Website, kind string) string {
	base := strings.TrimSpace(site.SiteDir)
	if base == "" {
		base = s.siteDirPath(site.PrimaryDomain)
	}
	switch kind {
	case "root", "site":
		return base
	case "app":
		return filepath.Join(base, "app")
	case "nginx":
		return filepath.Join(base, "nginx")
	case "site.conf":
		return filepath.Join(base, "nginx", "site.conf")
	case "stream.conf":
		return filepath.Join(base, "nginx", "stream.conf")
	case "rewrite":
		return filepath.Join(base, "nginx", "rewrite", site.PrimaryDomain+".conf")
	case "waf":
		return filepath.Join(base, "waf")
	case "runtime":
		return filepath.Join(base, ".workmesh", "runtime")
	case "cache":
		return filepath.Join(base, ".workmesh", "cache")
	case "ssl":
		return filepath.Join(base, "ssl")
	case "logs":
		return filepath.Join(base, "logs")
	case "proxy":
		return filepath.Join(base, "nginx", "proxy")
	case "redirect":
		return filepath.Join(base, "nginx", "redirect")
	case "auth_basic":
		return filepath.Join(base, "nginx", "auth_basic")
	case "path_auth":
		return filepath.Join(base, "nginx", "path_auth")
	case "upstream":
		return filepath.Join(base, "nginx", "upstream")
	case "cors":
		return filepath.Join(base, "nginx", "cors.conf")
	case "realip":
		return filepath.Join(base, "nginx", "realip.conf")
	case "php":
		return filepath.Join(base, "nginx", "php.conf")
	case "leech":
		return filepath.Join(base, "nginx", "leech")
	}
	return base
}

func (s *WebsiteService) ensureSiteLayout(site model.Website) error {
	for _, kind := range []string{"app", "nginx", "proxy", "redirect", "auth_basic", "path_auth", "upstream", "rewrite", "waf", "logs", "ssl", "runtime", "cache", "leech", "cors"} {
		p := s.SitePath(site, kind)
		if kind == "rewrite" || kind == "cors" {
			p = filepath.Dir(p)
		}
		if err := os.MkdirAll(p, 0o755); err != nil {
			return err
		}
	}
	siteConfigPath := s.SitePath(site, "site.conf")
	configInfo, configErr := os.Stat(siteConfigPath)
	if errors.Is(configErr, os.ErrNotExist) || (configErr == nil && configInfo.Size() == 0) {
		appRoot := s.SitePath(site, "app")
		content := fmt.Sprintf("server {\n    listen 80;\n    server_name %s;\n    root %s;\n    access_log %s;\n    error_log %s;\n    index index.php index.html index.htm default.php default.htm default.html;\n    error_page 404 /404.html;\n    location ^~ /.well-known/acme-challenge/ {\n        alias %s/.well-known/acme-challenge/;\n        default_type text/plain;\n    }\n}\n", site.PrimaryDomain, appRoot, s.websiteLogPath(site, "access.log"), s.websiteLogPath(site, "error.log"), appRoot)
		if err := os.WriteFile(s.SitePath(site, "site.conf"), []byte(content), 0o644); err != nil {
			return err
		}
	} else if configErr != nil {
		return configErr
	}
	for name, content := range map[string]string{"index.html": defaultWebsiteIndexHTML, "404.html": defaultWebsite404HTML} {
		path := filepath.Join(s.SitePath(site, "app"), name)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	if _, err := os.Stat(s.SitePath(site, "stream.conf")); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(s.SitePath(site, "stream.conf"), nil, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// writeInitialSiteConfig 生成创建站点时的最小可运行配置。
// 业务状态写入 SQLite，site.conf/stream.conf 只是可由 OpenResty 加载的运行文件。
func (s *WebsiteService) writeInitialSiteConfig(site model.Website) error {
	return s.writeInitialSiteConfigWithRoot(site, "")
}

// writeInitialSiteConfigWithRoot writes the initial HTTP server block. A
// subsite keeps its own server/config directory, but serves content from the
// selected directory inside its parent website, matching 1Panel semantics.
func (s *WebsiteService) writeInitialSiteConfigWithRoot(site model.Website, runDir string) error {
	if strings.ContainsAny(site.Proxy, "\r\n;{}\x00") {
		return errors.New("代理目标包含非法字符")
	}
	// TCP/UDP sites do not belong to the HTTP server block. Never emit the
	// historical placeholder proxy 127.0.0.1:9: an unconfigured stream site
	// must stay inactive until the user supplies a real upstream server.
	if site.Type == "stream" {
		if err := writeWebsiteAtomic(s.SitePath(site, "site.conf"), []byte("# workmesh stream site; HTTP listener disabled\n"), 0o640); err != nil {
			return err
		}
		if strings.TrimSpace(site.Proxy) == "" {
			// Keep the requested listener visible for the legacy response/test
			// contract, but comment it out until a real upstream is configured.
			ports, _ := normalizeStreamPorts(site.StreamPorts)
			var disabled strings.Builder
			disabled.WriteString("# workmesh stream site; upstream not configured\n")
			for _, port := range ports {
				disabled.WriteString("# listen " + port)
				if site.UDP {
					disabled.WriteString(" udp")
				}
				disabled.WriteString(";\n")
			}
			return writeWebsiteAtomic(s.SitePath(site, "stream.conf"), []byte(disabled.String()), 0o640)
		}
		return s.writeInitialStreamConfig(site)
	}
	serverName := site.PrimaryDomain
	root := s.SitePath(site, "app")
	var parent model.Website
	if site.Type == "subsite" && strings.TrimSpace(runDir) != "" {
		var err error
		parent, err = s.getWebsiteLocked(site.ParentWebsiteID)
		if err != nil {
			return errors.New("子网站父站点不存在")
		}
		parentRoot := s.SitePath(parent, "app")
		clean, err := validateWebsiteRelativeDir(runDir)
		if err != nil {
			return err
		}
		root = filepath.Join(parentRoot, filepath.FromSlash(clean))
		if !withinPath(parentRoot, root) {
			return errors.New("子网站运行目录越界")
		}
		if info, statErr := os.Stat(root); statErr != nil || !info.IsDir() {
			return errors.New("子网站运行目录不存在")
		}
	}
	accessLog := s.websiteLogPath(site, "access.log")
	errorLog := s.websiteLogPath(site, "error.log")
	base := fmt.Sprintf("server {\n    listen 80;\n    server_name %s;\n    access_log %s;\n    error_log %s;\n", serverName, accessLog, errorLog)
	base += fmt.Sprintf("    location ^~ /.well-known/acme-challenge/ {\n        alias %s/.well-known/acme-challenge/;\n        default_type text/plain;\n    }\n", root)
	if site.Type == "runtime" && strings.EqualFold(site.ProxyType, "fpm") {
		target := strings.TrimSpace(site.Proxy)
		if target == "" {
			target = "127.0.0.1:9000"
		}
		base += "    root " + root + ";\n    index index.php index.html;\n    location ~ \\.php$ {\n        include fastcgi_params;\n        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;\n        fastcgi_pass " + target + ";\n    }\n"
	} else if (site.Type == "runtime" || site.Type == "deployment") && strings.TrimSpace(site.Proxy) != "" {
		target := strings.TrimSpace(site.Proxy)
		if !strings.Contains(target, "://") && !strings.HasPrefix(target, "unix:") {
			target = "http://" + target
		}
		base += "    location / {\n        proxy_http_version 1.1;\n        proxy_set_header Host $host;\n        proxy_set_header X-Real-IP $remote_addr;\n        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n        proxy_pass " + target + ";\n    }\n"
	} else if site.Type == "runtime" {
		// 没有运行时端口时保留可加载的 PHP 默认入口，实际端口由运行时设置页补齐。
		base += "    root " + root + ";\n    index index.php index.html;\n    location ~ \\.php$ {\n        include fastcgi_params;\n        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;\n        fastcgi_pass 127.0.0.1:9000;\n    }\n"
	} else if site.Type == "subsite" {
		base += "    root " + root + ";\n    index index.php index.html index.htm default.php default.htm default.html;\n    error_page 404 /404.html;\n"
		if strings.TrimSpace(runDir) != "" && strings.EqualFold(parent.Type, "runtime") {
			parentRuntimeType, _, _, _ := s.runtimeOwnerMetadata(parent.RuntimeID)
			isPHPParent := strings.EqualFold(parentRuntimeType, "php") || strings.EqualFold(parent.ProxyType, "fpm") || strings.EqualFold(parent.ProxyType, "unix")
			if isPHPParent {
				target := strings.TrimSpace(parent.Proxy)
				if target == "" {
					target = "127.0.0.1:9000"
				}
				base += "    location ~ \\.php$ {\n        include fastcgi_params;\n        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;\n        fastcgi_pass " + strings.TrimPrefix(target, "http://") + ";\n    }\n"
			} else if strings.TrimSpace(parent.Proxy) != "" {
				target := strings.TrimSpace(parent.Proxy)
				if !strings.Contains(target, "://") && !strings.HasPrefix(target, "unix:") {
					target = "http://" + target
				}
				base += "    location / {\n        proxy_http_version 1.1;\n        proxy_set_header Host $host;\n        proxy_set_header X-Real-IP $remote_addr;\n        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n        proxy_pass " + target + ";\n    }\n"
			}
		}
	} else if site.Type == "proxy" {
		// 反向代理由 nginx/proxy/*.conf 托管；主站点只保留可加载的 server 基础块。
		base += "    root " + root + ";\n    index index.html;\n"
	} else {
		base += "    root " + root + ";\n    index index.php index.html index.htm default.php default.htm default.html;\n    error_page 404 /404.html;\n"
	}
	base += "}\n"
	if err := writeWebsiteAtomic(s.SitePath(site, "site.conf"), []byte(base), 0o640); err != nil {
		return err
	}
	return nil
}

// writeInitialStreamConfig creates the initial real stream proxy when the
// create request already contains one upstream target.
func (s *WebsiteService) writeInitialStreamConfig(site model.Website) error {
	ports := make([]string, 0, len(strings.Split(site.StreamPorts, ",")))
	for _, raw := range strings.Split(site.StreamPorts, ",") {
		port := strings.TrimSpace(raw)
		if port == "" {
			continue
		}
		protocol := ""
		if site.UDP {
			protocol = " udp"
		}
		ports = append(ports, fmt.Sprintf("        listen %s%s;", port, protocol))
	}
	if len(ports) == 0 {
		return errors.New("TCP/UDP 网站必须设置端口")
	}
	// OpenResty 顶层配置已经把每个站点 stream.conf 放进 stream{}；文件只保留 server{}，避免嵌套上下文。
	target := strings.TrimSpace(site.Proxy)
	if !strings.Contains(target, "://") && !strings.HasPrefix(target, "unix:") && !strings.Contains(target, ":") {
		target = "127.0.0.1:" + target
	}
	target = strings.TrimPrefix(strings.TrimPrefix(target, "http://"), "https://")
	if _, err := normalizeStreamServers([]map[string]any{{"server": target}}); err != nil {
		return fmt.Errorf("TCP/UDP 上游服务器无效: %w", err)
	}
	stream := "server {\n" + strings.Join(ports, "\n") + "\n        proxy_pass " + target + ";\n    }\n"
	return writeWebsiteAtomic(s.SitePath(site, "stream.conf"), []byte(stream), 0o640)
}

func writeWebsiteAtomic(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
