// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/todaybin/workmesh-server/node/model"
)

// reconcileLoadedWebsiteFiles restores runtime configuration files from the
// SQLite settings snapshot after restart. It deliberately ignores stopped
// sites so a restart cannot unexpectedly publish a disabled website.
func (s *WebsiteService) reconcileLoadedWebsiteFiles() {
	for index := range s.websites {
		site := s.websites[index]
		if strings.EqualFold(strings.TrimSpace(site.Status), "stopped") {
			continue
		}
		if strings.TrimSpace(site.SiteDir) == "" {
			site.SiteDir = s.siteDirPath(site.PrimaryDomain)
			s.websites[index].SiteDir = site.SiteDir
		}
		restoreSiteConfig := shouldRestoreWebsiteFile(s.SitePath(site, "site.conf"))
		restoreStreamConfig := strings.EqualFold(site.Type, "stream") && shouldRestoreStreamConfig(s.SitePath(site, "stream.conf"))
		if err := s.ensureSiteLayout(site); err != nil {
			continue
		}
		if restoreSiteConfig {
			if strings.EqualFold(site.Type, "stream") {
				_ = writeWebsiteAtomic(s.SitePath(site, "site.conf"), []byte("# workmesh stream site; HTTP listener disabled\n"), 0o640)
			} else if strings.EqualFold(site.Type, "subsite") {
				// Subsites retain their own management directory but their HTTP
				// root belongs to the parent website. Rebuild that relationship
				// from the persisted relative run directory after restart.
				runDir := "/"
				if cfg, ok := s.configs[site.ID]["dir"].(map[string]any); ok {
					if rel, ok := cfg["dir"].(string); ok && strings.TrimSpace(rel) != "" {
						runDir = "/" + strings.TrimPrefix(strings.TrimSpace(rel), "/")
					}
				}
				if err := s.writeInitialSiteConfigWithRoot(site, runDir); err != nil {
					// Keep the existing file if the parent run directory was
					// removed; startup must not invent a path outside the parent.
					continue
				}
			} else if cfg, ok := s.configs[site.ID]["nginx"].(map[string]any); ok && strings.TrimSpace(websiteLoadedString(cfg["content"])) != "" {
				content := websiteLoadedString(cfg["content"])
				content = s.syncManagedWebsiteIncludes(site, content)
				_ = writeWebsiteAtomic(s.SitePath(site, "site.conf"), []byte(content), 0o640)
			} else {
				_ = s.writeInitialSiteConfig(site)
			}
		}
		if restoreStreamConfig {
			if !s.restorePersistedStreamFile(site) {
				// No persisted upstream is not a reason to invent one. Leave a
				// comment-only file so the stream{} glob remains valid.
				_ = writeWebsiteAtomic(s.SitePath(site, "stream.conf"), []byte("# workmesh stream site; upstream not configured\n"), 0o640)
			}
		}
		// SQLite remains authoritative after restart. For an HTTPS site with a
		// real certificate, restore the certificate files and managed 443 block
		// as well; upgrades may remove runtime files while leaving the DB intact.
		if strings.EqualFold(strings.TrimSpace(site.Protocol), "HTTPS") && site.WebsiteSSLID != 0 {
			if err := s.validateHTTPSCertificateLocked(site, site.WebsiteSSLID); err == nil {
				_ = s.updateHTTPSRuntimeFilesLocked(site, true, site.WebsiteSSLID)
			}
		}
	}
}

func shouldRestoreWebsiteFile(path string) bool {
	info, err := os.Stat(path)
	return errors.Is(err, os.ErrNotExist) || (err == nil && info.Size() == 0)
}

// shouldRestoreStreamConfig also catches the old placeholder upstream. A
// persisted site must never keep proxy_pass 127.0.0.1:9 after an upgrade.
func shouldRestoreStreamConfig(path string) bool {
	if shouldRestoreWebsiteFile(path) {
		return true
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(content)
	return strings.Contains(text, "127.0.0.1:9") || strings.Contains(text, "proxy_pass http://;")
}

func (s *WebsiteService) restorePersistedStreamFile(site model.Website) bool {
	cfg, ok := s.configs[site.ID]["stream"].(map[string]any)
	if !ok {
		if strings.TrimSpace(site.Proxy) == "" {
			return false
		}
		return s.writeInitialStreamConfig(site) == nil
	}
	ports := websiteLoadedString(cfg["streamPorts"])
	if ports == "" {
		ports = site.StreamPorts
	}
	udp, _ := cfg["udp"].(bool)
	if raw, ok := cfg["servers"].([]map[string]any); ok {
		if normalizedPorts, portErr := normalizeStreamPorts(ports); portErr == nil {
			if servers, serverErr := normalizeStreamServers(raw); serverErr == nil {
				content := renderStreamConfig(site.ID, normalizedPorts, udp, websiteLoadedString(cfg["algorithm"]), servers)
				return writeWebsiteAtomic(s.SitePath(site, "stream.conf"), []byte(content), 0o640) == nil
			}
		}
	} else if encoded, err := json.Marshal(cfg["servers"]); err == nil {
		var raw []map[string]any
		if json.Unmarshal(encoded, &raw) == nil {
			if normalizedPorts, portErr := normalizeStreamPorts(ports); portErr == nil {
				if servers, serverErr := normalizeStreamServers(raw); serverErr == nil {
					content := renderStreamConfig(site.ID, normalizedPorts, udp, websiteLoadedString(cfg["algorithm"]), servers)
					return writeWebsiteAtomic(s.SitePath(site, "stream.conf"), []byte(content), 0o640) == nil
				}
			}
		}
	}
	return false
}

func websiteLoadedString(value any) string {
	if value == nil {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
