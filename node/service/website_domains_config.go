// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

// ListDomains 返回网站域名，并限制最大返回数量。
func (s *WebsiteService) ListDomains(websiteID uint) ([]model.WebsiteDomain, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.getWebsiteLocked(websiteID); err != nil {
		return nil, err
	}
	return append([]model.WebsiteDomain(nil), s.domains[websiteID]...), nil
}

// UpsertDomain 新增或更新域名记录，并校验端口和域名格式。
func (s *WebsiteService) UpsertDomain(domain model.WebsiteDomain) (model.WebsiteDomain, error) {
	if domain.WebsiteID == 0 {
		return model.WebsiteDomain{}, errors.New("网站域名无效")
	}
	normalizedDomain, normalizeErr := normalizeWebsiteDomain(domain.Domain)
	if normalizeErr != nil {
		return model.WebsiteDomain{}, normalizeErr
	}
	domain.Domain = normalizedDomain
	if domain.Port == 0 {
		if domain.SSL {
			domain.Port = 443
		} else {
			domain.Port = 80
		}
	}
	if domain.Port < 1 || domain.Port > 65535 {
		return model.WebsiteDomain{}, errors.New("网站端口超出范围")
	}
	domain.Domain = strings.TrimSpace(domain.Domain)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getWebsiteLocked(domain.WebsiteID); err != nil {
		return model.WebsiteDomain{}, err
	}
	items := s.domains[domain.WebsiteID]
	for _, site := range s.websites {
		for _, existing := range s.domains[site.ID] {
			if existing.ID != domain.ID && strings.EqualFold(existing.Domain, domain.Domain) {
				return model.WebsiteDomain{}, errors.New("网站域名已存在")
			}
		}
	}
	for i := range items {
		if items[i].ID == domain.ID {
			if site, err := s.getWebsiteLocked(domain.WebsiteID); err == nil && strings.EqualFold(site.PrimaryDomain, domain.Domain) {
				// The primary-domain row may update its port/SSL flags, but may
				// not be moved to another hostname through this endpoint.
				domain.Domain = site.PrimaryDomain
			}
			oldItems := append([]model.WebsiteDomain(nil), items...)
			items[i] = domain
			s.domains[domain.WebsiteID] = items
			site, _ := s.getWebsiteLocked(domain.WebsiteID)
			oldConfig, configErr := s.syncWebsiteServerNamesLocked(site)
			if configErr != nil {
				s.domains[domain.WebsiteID] = oldItems
				return model.WebsiteDomain{}, configErr
			}
			if err := s.persist("website-domains", s.domains); err != nil {
				s.domains[domain.WebsiteID] = oldItems
				return model.WebsiteDomain{}, errors.Join(err, s.restoreWebsiteConfigLocked(domain.WebsiteID, oldConfig))
			}
			return domain, nil
		}
	}
	oldItems := append([]model.WebsiteDomain(nil), items...)
	s.domains[domain.WebsiteID] = append(items, domain)
	site, _ := s.getWebsiteLocked(domain.WebsiteID)
	oldConfig, configErr := s.syncWebsiteServerNamesLocked(site)
	if configErr != nil {
		s.domains[domain.WebsiteID] = oldItems
		return model.WebsiteDomain{}, configErr
	}
	if err := s.persist("website-domains", s.domains); err != nil {
		s.domains[domain.WebsiteID] = oldItems
		return model.WebsiteDomain{}, errors.Join(err, s.restoreWebsiteConfigLocked(domain.WebsiteID, oldConfig))
	}
	// 新记录由 SQLite 自增主键生成；持久化完成后返回实际 ID，后续编辑和删除使用同一 ID。
	return s.domains[domain.WebsiteID][len(items)], nil
}

// DeleteDomain 删除指定网站域名。
func (s *WebsiteService) DeleteDomain(websiteID uint, domainID string) error {
	if websiteID == 0 || strings.TrimSpace(domainID) == "" {
		return errors.New("网站域名参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.domains[websiteID]
	for i, item := range items {
		if item.ID == domainID {
			site, _ := s.getWebsiteLocked(websiteID)
			if strings.EqualFold(item.Domain, site.PrimaryDomain) {
				return errors.New("主域名不能删除")
			}
			oldItems := append([]model.WebsiteDomain(nil), items...)
			s.domains[websiteID] = append(append([]model.WebsiteDomain(nil), items[:i]...), items[i+1:]...)
			oldConfig, configErr := s.syncWebsiteServerNamesLocked(site)
			if configErr != nil {
				s.domains[websiteID] = oldItems
				return configErr
			}
			if err := s.persist("website-domains", s.domains); err != nil {
				s.domains[websiteID] = oldItems
				return errors.Join(err, s.restoreWebsiteConfigLocked(websiteID, oldConfig))
			}
			if repository, repositoryErr := s.sqliteRepository(); repositoryErr == nil {
				if _, err := repository.Exec(`DELETE FROM website_domains WHERE id=? AND website_id=?`, domainID, websiteID); err != nil {
					s.domains[websiteID] = oldItems
					return errors.Join(err, s.restoreWebsiteConfigLocked(websiteID, oldConfig), s.persist("website-domains", s.domains))
				}
			} else if s.db != nil {
				s.domains[websiteID] = oldItems
				return errors.Join(repositoryErr, s.restoreWebsiteConfigLocked(websiteID, oldConfig), s.persist("website-domains", s.domains))
			}
			return nil
		}
	}
	return os.ErrNotExist
}

// GetConfig 读取网站类型配置；未设置时返回空对象而非固定业务数据。
func (s *WebsiteService) GetConfig(websiteID uint, typ string) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	site, err := s.getWebsiteLocked(websiteID)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(strings.TrimSpace(typ), "https") {
		return s.websiteHTTPSConfigLocked(site), nil
	}
	result := map[string]any{}
	for key, value := range s.configs[websiteID] {
		result[key] = value
	}
	if typ != "" {
		if value, ok := result[typ]; ok {
			if typed, ok := value.(map[string]any); ok {
				return normalizeWebsiteSetting(typ, typed), nil
			}
		}
		return normalizeWebsiteSetting(typ, map[string]any{}), nil
	}
	return result, nil
}

// websiteHTTPSConfigLocked 在已持有服务读锁时组装 HTTPS 配置，并以关系表状态覆盖旧配置快照。
func (s *WebsiteService) websiteHTTPSConfigLocked(site model.Website) map[string]any {
	enabled := strings.EqualFold(site.Protocol, "HTTPS")
	httpConfig := site.HttpConfig
	if enabled && strings.TrimSpace(httpConfig) == "" {
		httpConfig = "HTTPToHTTPS"
	}
	result := map[string]any{
		// enable is the 1Panel field; enabled/port/protocols are retained for
		// older WorkMesh clients that consumed the earlier generic response.
		"enable":                enabled,
		"enabled":               enabled,
		"websiteSSLId":          site.WebsiteSSLID,
		"port":                  443,
		"httpsPort":             "443",
		"httpsPorts":            []int{443},
		"httpRedirect":          false,
		"hsts":                  false,
		"hstsIncludeSubDomains": false,
		"http3":                 false,
		"SSLProtocol":           []string{"TLSv1.3", "TLSv1.2"},
		"protocols":             []string{"TLSv1.2", "TLSv1.3"},
		"algorithm":             "",
		"certificate":           "",
		"certificateOk":         false,
		"httpConfig":            httpConfig,
		"SSL":                   map[string]any{"id": site.WebsiteSSLID, "primaryDomain": site.PrimaryDomain, "pem": "", "certificate": ""},
	}
	if configured, ok := s.configs[site.ID]["https"].(map[string]any); ok {
		for key, value := range configured {
			if key == "privateKey" || key == "key" {
				continue
			}
			result[key] = value
		}
	}
	// The relationship table is canonical for enablement and certificate
	// association; an old settings blob must not resurrect stale HTTPS state.
	result["enable"], result["enabled"] = enabled, enabled
	result["websiteSSLId"] = site.WebsiteSSLID
	result["httpConfig"] = httpConfig
	if site.WebsiteSSLID == 0 {
		return result
	}
	var certificate, keyType, domains, status, expire string
	executor, executorErr := s.sqliteRepository()
	if executorErr != nil {
		return result
	}
	if err := executor.QueryRowContext(context.Background(), `SELECT pem,key_type,domains,status,expire_date FROM website_ssls WHERE id=?`, site.WebsiteSSLID).Scan(&certificate, &keyType, &domains, &status, &expire); err == nil {
		result["certificate"] = certificate
		result["certificateOk"] = strings.TrimSpace(certificate) != ""
		result["algorithm"] = keyType
		result["domains"] = domains
		result["status"] = status
		result["expireDate"] = expire
		result["SSL"] = map[string]any{
			"id": site.WebsiteSSLID, "primaryDomain": site.PrimaryDomain,
			"domains": domains, "pem": certificate, "certificate": certificate,
			"status": status, "keyType": keyType, "expireDate": expire,
		}
	}
	return result
}

// WebsiteConfigFile 返回单站点 nginx/site.conf，供资源页编辑该站点实际配置。
func (s *WebsiteService) WebsiteConfigFile(websiteID uint) (map[string]any, error) {
	site, err := s.Get(websiteID)
	if err != nil {
		return nil, err
	}
	path := s.SitePath(site, "site.conf")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	info, _ := os.Stat(path)
	result := map[string]any{
		"path": path, "name": filepath.Base(path), "content": string(content),
		"isDir": false, "type": "conf", "mimeType": "text/plain", "size": len(content),
	}
	if info != nil {
		result["mode"] = info.Mode().Perm()
		result["modTime"] = info.ModTime().UTC().Format(time.RFC3339Nano)
	}
	return result, nil
}

// WebsiteNginxScopeConfig 从站点域名目录的 Nginx 配置读取指定作用域。
// index 作用域必须返回 1Panel 兼容的 enable/params 结构，不能依赖历史 JSON 状态。
func (s *WebsiteService) WebsiteNginxScopeConfig(websiteID uint, scope string) (map[string]any, error) {
	if strings.TrimSpace(scope) == "limit-conn" {
		return s.GetLimitConnConfig(websiteID)
	}
	if strings.TrimSpace(scope) != "index" {
		return nil, errors.New("不支持的 Nginx 配置作用域")
	}
	site, err := s.Get(websiteID)
	if err != nil {
		return nil, err
	}
	content, readErr := os.ReadFile(s.SitePath(site, "site.conf"))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, readErr
	}
	documents := make([]string, 0, 5)
	for _, match := range regexp.MustCompile(`(?m)^\s*index\s+([^;]+);`).FindAllStringSubmatch(string(content), -1) {
		for _, item := range strings.Fields(match[1]) {
			if item != "" && !containsString(documents, item) {
				documents = append(documents, item)
			}
		}
	}
	if len(documents) == 0 {
		documents = []string{"index.php", "index.html", "index.htm", "default.php", "default.htm", "default.html"}
	}
	params := []map[string]any{{"name": "index", "params": documents}}
	return map[string]any{"enable": len(documents) > 0, "params": params}, nil
}

// GetLimitConnConfig 从 SQLite 读取站点限流状态，并返回前端所需的 enable/params 结构。
func (s *WebsiteService) GetLimitConnConfig(websiteID uint) (map[string]any, error) {
	if websiteID == 0 {
		return nil, errors.New("网站 ID 无效")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.getWebsiteLocked(websiteID); err != nil {
		return nil, err
	}
	enabled, perserver, perip, rate := false, 300, 25, 512
	if executor, executorErr := s.sqliteRepository(); executorErr == nil {
		var e int
		err := executor.QueryRowContext(context.Background(), `SELECT enabled,perserver,perip,rate_k FROM website_limit_conn WHERE website_id=?`, websiteID).Scan(&e, &perserver, &perip, &rate)
		if err == nil {
			enabled = e != 0
		}
	}
	params := []map[string]any{}
	if enabled {
		params = append(params, map[string]any{"name": "limit_conn", "params": []string{"perserver", strconv.Itoa(perserver)}})
		params = append(params, map[string]any{"name": "limit_conn", "params": []string{"perip", strconv.Itoa(perip)}})
		params = append(params, map[string]any{"name": "limit_rate", "params": []string{fmt.Sprintf("%dk", rate)}})
	}
	return map[string]any{"enable": enabled, "params": params}, nil
}

// UpdateLimitConn 校验限流范围后原子更新 site.conf 和 SQLite，失败时恢复旧配置文件。
func (s *WebsiteService) UpdateLimitConn(websiteID uint, enabled bool, perserver, perip, rate int) (map[string]any, error) {
	if websiteID == 0 || perserver < 1 || perserver > 65535 || perip < 1 || perip > 65535 || rate < 1 || rate > 99999999 {
		return nil, errors.New("限流参数范围无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	site, err := s.getWebsiteLocked(websiteID)
	if err != nil {
		return nil, err
	}
	path := s.SitePath(site, "site.conf")
	old, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, fmt.Errorf("读取网站配置失败: %w", readErr)
	}
	lines := make([]string, 0)
	for _, line := range strings.Split(string(old), "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "limit_conn ") || strings.HasPrefix(trim, "limit_rate ") {
			continue
		}
		lines = append(lines, line)
	}
	if enabled {
		insert := []string{fmt.Sprintf("    limit_conn perserver %d;", perserver), fmt.Sprintf("    limit_conn perip %d;", perip), fmt.Sprintf("    limit_rate %dk;", rate)}
		at := len(lines)
		for i := len(lines) - 1; i >= 0; i-- {
			if strings.TrimSpace(lines[i]) == "}" {
				at = i
				break
			}
		}
		lines = append(lines, "")
		lines = append(lines[:at], append(insert, lines[at:]...)...)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")), 0o640); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return nil, errors.Join(err, writeWebsiteAtomic(path, old, 0o640))
	}
	if repository, repositoryErr := s.sqliteRepository(); repositoryErr == nil {
		err = repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
			_, execErr := tx.Exec(`INSERT INTO website_limit_conn(website_id,enabled,perserver,perip,rate_k,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(website_id) DO UPDATE SET enabled=excluded.enabled,perserver=excluded.perserver,perip=excluded.perip,rate_k=excluded.rate_k,updated_at=excluded.updated_at`, websiteID, boolInt(enabled), perserver, perip, rate, formatTime(time.Now()))
			return execErr
		})
		if err != nil {
			return nil, errors.Join(err, writeWebsiteAtomic(path, old, 0o640))
		}
	} else if s.db != nil {
		return nil, errors.Join(repositoryErr, writeWebsiteAtomic(path, old, 0o640))
	}
	params := []map[string]any{}
	if enabled {
		params = append(params, map[string]any{"name": "limit_conn", "params": []string{"perserver", strconv.Itoa(perserver)}}, map[string]any{"name": "limit_conn", "params": []string{"perip", strconv.Itoa(perip)}}, map[string]any{"name": "limit_rate", "params": []string{fmt.Sprintf("%dk", rate)}})
	}
	return map[string]any{"enable": enabled, "params": params}, nil
}

const managedWebsiteIncludeMarker = "# workmesh-managed"
