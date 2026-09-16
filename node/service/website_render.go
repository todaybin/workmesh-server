// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/model"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func websiteSettingString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := value[key]; ok {
			switch item := raw.(type) {
			case string:
				if text := strings.TrimSpace(item); text != "" {
					return text
				}
			case fmt.Stringer:
				if text := strings.TrimSpace(item.String()); text != "" {
					return text
				}
			default:
				if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
					return text
				}
			}
		}
	}
	return ""
}

func websiteSettingBool(value map[string]any, key string, fallback bool) bool {
	raw, ok := value[key]
	if !ok {
		return fallback
	}
	switch item := raw.(type) {
	case bool:
		return item
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(item))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func validateNginxSettingValue(value string, allowDollar bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 4096 || strings.ContainsAny(value, "\x00\r\n;{}") {
		return "", errors.New("Nginx 设置包含非法字符")
	}
	if !allowDollar && strings.Contains(value, "$") {
		return "", errors.New("Nginx 设置不允许变量")
	}
	return value, nil
}

func normalizeWebsiteProxyTarget(raw string) (string, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return "", errors.New("代理目标不能为空")
	}
	if strings.ContainsAny(target, "\x00\r\n;{}") {
		return "", errors.New("代理目标包含非法字符")
	}
	if strings.HasPrefix(target, "unix:") {
		if len(target) > 4096 {
			return "", errors.New("代理目标过长")
		}
		return target, nil
	}
	if !strings.Contains(target, "://") {
		target = "http://" + target
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		return "", errors.New("代理目标只允许 http/https 或 unix")
	}
	return target, nil
}

func websiteSettingList(value any) []map[string]any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var items []map[string]any
	if json.Unmarshal(encoded, &items) != nil {
		return nil
	}
	return items
}

// normalizeWebsiteSetting accepts both the original 1Panel names and newer
// WorkMesh aliases, then returns both spellings for lossless form round trips.
func normalizeWebsiteSetting(typ string, input map[string]any) map[string]any {
	out := make(map[string]any, len(input)+8)
	for key, value := range input {
		out[key] = value
	}
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "realip":
		enabled := websiteSettingBool(input, "enabled", websiteSettingBool(input, "open", false))
		trusted := websiteSettingString(input, "trusted", "ipFrom")
		if raw, ok := input["trusted"].([]any); ok {
			parts := make([]string, 0, len(raw))
			for _, item := range raw {
				if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
					parts = append(parts, text)
				}
			}
			trusted = strings.Join(parts, "\n")
		}
		header := websiteSettingString(input, "header", "ipHeader")
		other := websiteSettingString(input, "other", "ipOther")
		if header == "" {
			header = "X-Real-IP"
		}
		out["enabled"], out["open"] = enabled, enabled
		out["trusted"], out["ipFrom"] = trusted, trusted
		out["header"], out["ipHeader"] = header, header
		out["other"], out["ipOther"] = other, other
	case "leech", "hotlink":
		enabled := websiteSettingBool(input, "enabled", websiteSettingBool(input, "enable", false))
		domains := websiteSettingString(input, "domains", "allowDomains", "referers")
		serverNames := []string{}
		if raw, ok := input["serverNames"].([]any); ok {
			for _, item := range raw {
				if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
					serverNames = append(serverNames, text)
				}
			}
		}
		if len(serverNames) == 0 && domains != "" {
			serverNames = strings.Fields(domains)
		}
		if domains == "" {
			domains = strings.Join(serverNames, "\n")
		}
		out["enabled"], out["enable"] = enabled, enabled
		out["domains"], out["serverNames"] = domains, serverNames
		if _, ok := out["cacheUint"]; !ok {
			out["cacheUint"] = out["cacheUnit"]
		}
		if _, ok := out["cacheUnit"]; !ok {
			out["cacheUnit"] = out["cacheUint"]
		}
	}
	return out
}

// renderWebsiteSetting 将站点设置转换为受控运行文件。SQLite 中保存请求的
// 结构化值，运行文件只包含白名单 Nginx 指令，禁止把请求 JSON 原样拼入配置。
func (s *WebsiteService) renderWebsiteSetting(site model.Website, typ string, value map[string]any) (map[string][]byte, error) {
	files := map[string][]byte{}
	write := func(kind, content string) {
		path := s.SitePath(site, kind)
		if kind == "proxy" || kind == "redirect" || kind == "auth_basic" || kind == "path_auth" || kind == "leech" {
			path = filepath.Join(path, "managed.conf")
		}
		files[path] = []byte(content)
	}
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "proxy":
		return s.renderWebsiteProxySetting(site, value, files, write)
	case "lbs":
		return s.renderWebsiteLoadBalanceSetting(site, value, files)
	case "cors":
		return s.renderWebsiteCORSSetting(site, value, files, write)
	case "realip":
		return s.renderWebsiteRealIPSetting(site, value, files, write)
	case "leech", "hotlink":
		return s.renderWebsiteLeechSetting(site, value, files, write)
	case "redirect":
		return s.renderWebsiteRedirectSetting(site, value, files, write)
	case "php":
		return s.renderWebsitePHPSetting(site, value, files, write)
	case "auths", "auth", "path-auth":
		return s.renderWebsiteAuthSetting(site, typ, value, files, write)
	}
	return files, nil
}

// renderWebsiteProxySetting 渲染反向代理配置并限制可写入的请求头。
func (s *WebsiteService) renderWebsiteProxySetting(site model.Website, value map[string]any, files map[string][]byte, write func(string, string)) (map[string][]byte, error) {
	if !websiteSettingBool(value, "enabled", true) {
		write("proxy", "")
		return files, nil
	}
	target, err := normalizeWebsiteProxyTarget(websiteSettingString(value, "proxyPass", "proxy", "target", "url", "address"))
	if err != nil {
		return nil, err
	}
	match := websiteSettingString(value, "match", "path", "location")
	if match == "" {
		match = "/"
	}
	if !strings.HasPrefix(match, "/") || strings.ContainsAny(match, "\x00\r\n;{} \t") {
		return nil, errors.New("代理匹配路径无效")
	}
	modifier := websiteSettingString(value, "modifier")
	if modifier != "" && modifier != "=" && modifier != "~" && modifier != "~*" {
		return nil, errors.New("代理匹配修饰符无效")
	}
	proxyHost := websiteSettingString(value, "proxyHost", "host")
	if proxyHost == "" {
		proxyHost = "$host"
	}
	if _, err := validateNginxSettingValue(proxyHost, true); err != nil {
		return nil, err
	}
	headers := fmt.Sprintf("        proxy_set_header Host %s;\n        proxy_set_header X-Real-IP $remote_addr;\n        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n        proxy_set_header X-Forwarded-Proto $scheme;\n        proxy_set_header Upgrade $http_upgrade;\n        proxy_set_header Connection \"upgrade\";\n", proxyHost)
	if raw, ok := value["headers"].(map[string]any); ok {
		for key, val := range raw {
			if key != "Host" && key != "X-Real-IP" && key != "X-Forwarded-For" && key != "X-Forwarded-Proto" {
				continue
			}
			text, validErr := validateNginxSettingValue(fmt.Sprint(val), true)
			if validErr != nil {
				return nil, validErr
			}
			headers += fmt.Sprintf("        proxy_set_header %s %s;\n", key, text)
		}
	}
	lines := []string{fmt.Sprintf("location %s%s {", modifier, match), "        proxy_http_version 1.1;", "        proxy_connect_timeout 10s;", "        proxy_send_timeout 60s;", "        proxy_read_timeout 60s;"}
	lines = append(lines, strings.TrimRight(headers, "\n"))
	if strings.HasPrefix(target, "https://") {
		sni := websiteSettingBool(value, "sni", websiteSettingBool(value, "SNI", true))
		if sni {
			lines = append(lines, "        proxy_ssl_server_name on;")
		} else {
			lines = append(lines, "        proxy_ssl_server_name off;")
		}
		sslName := websiteSettingString(value, "proxySSLName", "sslName")
		if sslName == "" {
			sslName = "$proxy_host"
		}
		if _, err := validateNginxSettingValue(sslName, true); err != nil {
			return nil, err
		}
		lines = append(lines, "        proxy_ssl_name "+sslName+";")
		if websiteSettingBool(value, "sslVerify", websiteSettingBool(value, "proxySSLVerify", false)) {
			lines = append(lines, "        proxy_ssl_verify on;", "        proxy_ssl_trusted_certificate /etc/ssl/certs/ca-certificates.crt;")
		}
	}
	cache := websiteSettingBool(value, "cache", false)
	serverCacheTime := websiteSettingInt(value, "serverCacheTime", 0)
	serverCacheUnit := normalizeCacheUnit(websiteSettingString(value, "serverCacheUnit", "serverCacheUint"))
	if cache {
		if serverCacheTime <= 0 {
			serverCacheTime = 10
		}
		if serverCacheUnit == "" {
			serverCacheUnit = "m"
		}
		zone := "proxy_cache_zone_of_" + sanitizeCacheName(site.Alias)
		lines = append(lines, "        proxy_ignore_headers Set-Cookie Cache-Control expires;", "        proxy_cache "+zone+";", "        proxy_cache_key $host$uri$is_args$args;", fmt.Sprintf("        proxy_cache_valid 200 304 301 302 %d%s;", serverCacheTime, serverCacheUnit))
		cachePath := filepath.Join(s.SitePath(site, "root"), ".workmesh", "cache")
		cacheConf := fmt.Sprintf("proxy_cache_path %s levels=1:2 keys_zone=%s:10m inactive=60m max_size=1g use_temp_path=off;\n", cachePath, zone)
		files[filepath.Join(s.SitePath(site, "root"), "nginx", "proxy-cache.conf")] = []byte(cacheConf)
	} else {
		files[filepath.Join(s.SitePath(site, "root"), "nginx", "proxy-cache.conf")] = nil
	}
	cacheTime := websiteSettingInt(value, "cacheTime", 0)
	cacheUnit := normalizeCacheUnit(websiteSettingString(value, "cacheUnit", "cacheUint"))
	if cacheTime > 0 {
		if cacheUnit == "" {
			cacheUnit = "d"
		}
		lines = append(lines, fmt.Sprintf("        expires %d%s;", cacheTime, cacheUnit))
	} else if cacheTime < 0 {
		lines = append(lines, "        add_header Cache-Control no-cache;")
	}
	if replaces := websiteSettingStringMap(value["replaces"]); len(replaces) > 0 {
		lines = append(lines, "        proxy_set_header Accept-Encoding \"\";")
		for from, to := range replaces {
			lines = append(lines, fmt.Sprintf("        sub_filter \"%s\" \"%s\";", escapeNginxQuoted(from), escapeNginxQuoted(to)))
		}
		lines = append(lines, "        sub_filter_once off;", "        sub_filter_types *;")
	}
	if websiteSettingBool(value, "cors", false) {
		origin := websiteSettingString(value, "allowOrigins", "allowOrigin")
		if origin == "" {
			origin = "*"
		}
		lines = append(lines, "        add_header Access-Control-Allow-Origin "+origin+" always;")
		if methods := websiteSettingString(value, "allowMethods"); methods != "" {
			lines = append(lines, "        add_header Access-Control-Allow-Methods "+methods+" always;")
		}
		if hdrs := websiteSettingString(value, "allowHeaders"); hdrs != "" {
			lines = append(lines, "        add_header Access-Control-Allow-Headers "+hdrs+" always;")
		}
		if websiteSettingBool(value, "allowCredentials", false) {
			lines = append(lines, "        add_header Access-Control-Allow-Credentials true always;")
		}
		if websiteSettingBool(value, "preflight", true) {
			lines = append(lines, "        if ($request_method = 'OPTIONS') {", "            add_header Access-Control-Max-Age 1728000;", "            add_header Content-Type 'text/plain;charset=UTF-8';", "            add_header Content-Length 0;", "            return 204;", "        }")
		}
	}
	lines = append(lines, "        proxy_pass "+target+";", "}")
	write("proxy", strings.Join(lines, "\n")+"\n")
	return files, nil
}

func websiteSettingInt(value map[string]any, key string, fallback int) int {
	raw, ok := value[key]
	if !ok {
		return fallback
	}
	switch x := raw.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	}
	return fallback
}
func normalizeCacheUnit(unit string) string {
	unit = strings.ToLower(strings.TrimSpace(unit))
	switch unit {
	case "s", "m", "h", "d", "w", "y":
		return unit
	}
	return ""
}
func sanitizeCacheName(name string) string {
	name = regexp.MustCompile(`[^A-Za-z0-9_]+`).ReplaceAllString(name, "_")
	if name == "" {
		return "site"
	}
	return name
}
func escapeNginxQuoted(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`)
}
func websiteSettingStringMap(value any) map[string]string {
	out := map[string]string{}
	raw, ok := value.(map[string]any)
	if !ok {
		if typed, ok2 := value.(map[string]string); ok2 {
			return typed
		}
		return out
	}
	for k, v := range raw {
		ks, ke := validateNginxSettingValue(fmt.Sprint(k), false)
		vs, ve := validateNginxSettingValue(fmt.Sprint(v), true)
		if ke == nil && ve == nil {
			out[ks] = vs
		}
	}
	return out
}

// renderWebsiteLoadBalanceSetting 渲染 upstream 和对应的代理入口。
func (s *WebsiteService) renderWebsiteLoadBalanceSetting(site model.Website, value map[string]any, files map[string][]byte) (map[string][]byte, error) {
	upstreams := websiteSettingList(value["upstreams"])
	if len(upstreams) == 0 {
		// An empty upstream block is invalid OpenResty syntax. Treat deleting
		// the last backend as removal of both managed fragments instead.
		files[s.SitePath(site, "upstream")+"/managed.conf"] = nil
		files[s.SitePath(site, "proxy")+"/lbs.conf"] = nil
		return files, nil
	}
	name := fmt.Sprintf("workmesh_upstream_%d", site.ID)
	upstream := "upstream " + name + " {\n"
	proxyTarget := ""
	for _, item := range upstreams {
		address, err := validateNginxSettingValue(websiteSettingString(item, "address", "server", "url"), false)
		if err != nil {
			return nil, err
		}
		line := "    server " + address
		if weight := websiteSettingString(item, "weight"); weight != "" {
			if _, err := strconv.Atoi(weight); err != nil {
				return nil, errors.New("负载均衡权重无效")
			}
			line += " weight=" + weight
		}
		upstream += line + ";\n"
		if proxyTarget == "" {
			proxyTarget = "http://" + name
		}
	}
	upstream += "}\n"
	files[s.SitePath(site, "upstream")+"/managed.conf"] = []byte(upstream)
	if proxyTarget != "" && websiteSettingBool(value, "enabled", true) {
		files[s.SitePath(site, "proxy")+"/lbs.conf"] = []byte("location / {\n        proxy_http_version 1.1;\n        proxy_set_header Host $host;\n        proxy_pass " + proxyTarget + ";\n}\n")
	} else {
		files[s.SitePath(site, "proxy")+"/lbs.conf"] = nil
	}
	return files, nil
}

// renderWebsiteCORSSetting 渲染跨域响应头。
func (s *WebsiteService) renderWebsiteCORSSetting(site model.Website, value map[string]any, files map[string][]byte, write func(string, string)) (map[string][]byte, error) {
	if !websiteSettingBool(value, "enabled", false) {
		write("cors", "")
		return files, nil
	}
	origin := websiteSettingString(value, "origin", "allowOrigin")
	if origin == "" {
		if items, ok := value["origins"].([]any); ok && len(items) > 0 {
			origin = fmt.Sprint(items[0])
		}
	}
	if origin == "" {
		origin = "$http_origin"
	}
	origin, err := validateNginxSettingValue(origin, true)
	if err != nil {
		return nil, err
	}
	methods := websiteSettingString(value, "methods", "allowMethods")
	if methods == "" {
		methods = "GET,POST,PUT,DELETE,OPTIONS"
	}
	methods, err = validateNginxSettingValue(methods, false)
	if err != nil {
		return nil, err
	}
	write("cors", fmt.Sprintf("add_header Access-Control-Allow-Origin %q always;\nadd_header Access-Control-Allow-Methods %q always;\nadd_header Access-Control-Allow-Headers %q always;\n", origin, methods, "Content-Type,Authorization"))
	return files, nil
}

// renderWebsiteRealIPSetting 渲染可信代理地址。
func (s *WebsiteService) renderWebsiteRealIPSetting(site model.Website, value map[string]any, files map[string][]byte, write func(string, string)) (map[string][]byte, error) {
	if !websiteSettingBool(value, "enabled", false) {
		write("realip", "")
		return files, nil
	}
	raw, _ := value["trusted"].(string)
	trusted := strings.Fields(raw)
	if len(trusted) == 0 {
		if items, ok := value["trusted"].([]any); ok {
			for _, item := range items {
				trusted = append(trusted, fmt.Sprint(item))
			}
		}
	}
	if len(trusted) == 0 {
		trusted = []string{"127.0.0.1"}
	}
	header := websiteSettingString(value, "header", "ipHeader")
	if header == "" {
		header = "X-Real-IP"
	}
	if header == "other" {
		header = websiteSettingString(value, "other", "ipOther")
	}
	if header == "" || strings.ContainsAny(header, "\x00\r\n;{} \t") {
		return nil, errors.New("真实 IP Header 无效")
	}
	content := "real_ip_header " + header + ";\nreal_ip_recursive on;\n"
	for _, item := range trusted {
		item, err := validateNginxSettingValue(item, false)
		if err != nil {
			return nil, err
		}
		content += "set_real_ip_from " + item + ";\n"
	}
	write("realip", content)
	return files, nil
}

// renderWebsiteLeechSetting 渲染防盗链规则。
func (s *WebsiteService) renderWebsiteLeechSetting(site model.Website, value map[string]any, files map[string][]byte, write func(string, string)) (map[string][]byte, error) {
	if !websiteSettingBool(value, "enabled", false) {
		write("leech", "")
		return files, nil
	}
	domains := websiteSettingString(value, "domains", "allowDomains", "referers")
	if domains == "" {
		domains = site.PrimaryDomain
	}
	domains = strings.Join(strings.Fields(domains), " ")
	domains, err := validateNginxSettingValue(domains, false)
	if err != nil {
		return nil, err
	}
	status := websiteSettingString(value, "return", "status")
	if status == "" {
		status = "403"
	}
	if status != "400" && status != "403" && status != "404" {
		return nil, errors.New("防盗链返回码无效")
	}
	extensions := websiteSettingString(value, "extends", "extensions")
	location := ""
	if extensions != "" {
		parts := strings.FieldsFunc(extensions, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\r' })
		safe := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimPrefix(strings.TrimSpace(part), ".")
			if part != "" && regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(part) {
				safe = append(safe, part)
			}
		}
		if len(safe) > 0 {
			location = "location ~* \\.(" + strings.Join(safe, "|") + ")$ {\n"
		}
	}
	content := fmt.Sprintf("%svalid_referers none blocked server_names %s;\nif ($invalid_referer) { return %s; }\n", location, domains, status)
	if location != "" {
		content += "}\n"
	}
	write("leech", content)
	return files, nil
}

// renderWebsiteRedirectSetting 渲染站点重定向规则。
func (s *WebsiteService) renderWebsiteRedirectSetting(site model.Website, value map[string]any, files map[string][]byte, write func(string, string)) (map[string][]byte, error) {
	if !websiteSettingBool(value, "enabled", false) {
		write("redirect", "")
		return files, nil
	}
	target := websiteSettingString(value, "target", "redirect", "url", "to")
	if target == "" {
		return nil, errors.New("重定向目标不能为空")
	}
	var err error
	target, err = validateNginxSettingValue(target, true)
	if err != nil {
		return nil, err
	}
	code := websiteSettingString(value, "code", "status")
	if code == "" {
		code = "301"
	}
	if code != "301" && code != "302" && code != "307" && code != "308" {
		return nil, errors.New("重定向状态码无效")
	}
	write("redirect", fmt.Sprintf("return %s %s;\n", code, target))
	return files, nil
}

// renderWebsitePHPSetting 渲染 PHP FastCGI 入口。
func (s *WebsiteService) renderWebsitePHPSetting(site model.Website, value map[string]any, files map[string][]byte, write func(string, string)) (map[string][]byte, error) {
	rawTarget := websiteSettingString(value, "fastcgiPass", "proxy", "target")
	if rawTarget == "" {
		write("php", "")
		return files, nil
	}
	target, err := normalizeWebsiteProxyTarget(rawTarget)
	if err != nil {
		return nil, err
	}
	target = strings.TrimPrefix(target, "http://")
	write("php", "location ~ \\.php$ {\n    include fastcgi_params;\n    fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;\n    fastcgi_pass "+target+";\n}\n")
	return files, nil
}

// renderWebsiteAuthSetting 渲染全站或路径级基础认证。
func (s *WebsiteService) renderWebsiteAuthSetting(site model.Website, typ string, value map[string]any, files map[string][]byte, write func(string, string)) (map[string][]byte, error) {
	if !websiteSettingBool(value, "enabled", true) {
		if typ == "path-auth" {
			files[filepath.Join(s.SitePath(site, "path_auth"), "managed.conf")] = nil
		} else {
			write("auth_basic", "")
		}
		return files, nil
	}
	username := websiteSettingString(value, "username", "user")
	password := websiteSettingString(value, "password")
	if username == "" || strings.ContainsAny(username, "\r\n:\x00") {
		return nil, errors.New("认证用户名无效")
	}
	if password == "" {
		return nil, errors.New("认证密码不能为空")
	}
	sum := sha1.Sum([]byte(password))
	usersPath := filepath.Join(s.SitePath(site, "auth_basic"), "users.htpasswd")
	files[usersPath] = []byte(username + ":{SHA}" + base64.StdEncoding.EncodeToString(sum[:]) + "\n")
	if typ == "path-auth" {
		match := websiteSettingString(value, "path", "location")
		if match == "" {
			match = "/"
		}
		if !strings.HasPrefix(match, "/") || strings.ContainsAny(match, "\r\n;{} \t") {
			return nil, errors.New("路径认证路径无效")
		}
		files[s.SitePath(site, "path_auth")+"/managed.conf"] = []byte(fmt.Sprintf("location %s {\n    auth_basic \"Restricted\";\n    auth_basic_user_file %s;\n}\n", match, usersPath))
	} else {
		write("auth_basic", "auth_basic \"Restricted\";\nauth_basic_user_file "+usersPath+";\n")
	}
	return files, nil
}
