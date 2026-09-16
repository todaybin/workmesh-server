package service

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/todaybin/workmesh-server/node/model"
)

// writeWAFGeneratedConfig keeps the ModSecurity engine mode in sync with the
// global WAF switch before OpenResty is validated and reloaded.
func (s *WebsiteService) writeWAFGeneratedConfig() error {
	engine := "DetectionOnly"
	if !s.global.Enabled {
		engine = "Off"
	} else if s.global.Mode == "block" {
		engine = "On"
	}
	content := fmt.Sprintf("# WorkMesh WAF mode\nSecRuleEngine %s\n", engine)
	root := s.wafGeneratedRoot()
	if err := atomicWriteFile(filepath.Join(root, "modsecurity-mode.conf"), []byte(content), 0o640); err != nil {
		return err
	}
	// These directives are consumed by ModSecurity itself. Keep control-plane
	// settings in a generated include instead of leaving them as JSON-only UI
	// state.
	if err := atomicWriteFile(filepath.Join(root, "custom-rules.conf"), []byte(s.wafModSecurityRuntimeConfig()), 0o640); err != nil {
		return err
	}
	// Keep the ModSecurity engine mode independent from the CRS switch. The
	// panel's "standard rules" setting must not disable custom ModSecurity
	// rules or WorkMesh Lua rules.
	crs := []byte("# WorkMesh standard CRS rules enabled\n" +
		"Include /etc/workmesh-waf/crs/crs-setup.conf\n" +
		s.wafModSecurityThresholdConfig() +
		"Include /etc/workmesh-waf/crs/rules/*.conf\n")
	if !s.global.StandardRules {
		crs = []byte("# WorkMesh standard CRS rules disabled\n")
	}
	if err := atomicWriteFile(filepath.Join(root, "standard-rules.conf"), crs, 0o640); err != nil {
		return err
	}
	// Retain this file as a stable generated include for older deployments.
	return atomicWriteFile(filepath.Join(root, "global-rules.conf"), []byte("# WorkMesh global rules are handled by Lua and the image baseline\n"), 0o640)
}

func (s *WebsiteService) wafModSecurityRuntimeConfig() string {
	limit := s.global.RequestBodyLimit
	if limit <= 0 {
		limit = 1 << 20
	}

	var content strings.Builder
	content.WriteString("# WorkMesh global ModSecurity runtime settings\n")
	fmt.Fprintf(&content, "SecRequestBodyLimit %d\n", limit)

	// ModSecurity runs before access_by_lua. Add a phase-1 per-host engine
	// control so a site-level WAF switch also applies to CRS and image baseline
	// rules, not only to WorkMesh Lua rules.
	type hostMode struct {
		domain string
		mode   string
	}
	modes := make([]hostMode, 0)
	seen := map[string]struct{}{}
	for _, site := range s.websites {
		wafSite, ok := s.wafSites[site.ID]
		if !ok {
			wafSite = defaultWAFSite(site.ID, site.Alias)
		}
		if wafSite.Mode != "block" && wafSite.Mode != "observe" {
			wafSite.Mode = "observe"
		}
		mode := "DetectionOnly"
		if !s.global.Enabled || !wafSite.Enabled {
			mode = "Off"
		} else if s.global.Mode == "block" || wafSite.Mode == "block" {
			mode = "On"
		}
		domains := []string{site.PrimaryDomain}
		for _, attached := range s.domains[site.ID] {
			domains = append(domains, attached.Domain)
		}
		for _, domain := range domains {
			domain = strings.ToLower(strings.TrimSpace(domain))
			if domain == "" {
				continue
			}
			key := domain + "\x00" + mode
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			modes = append(modes, hostMode{domain: domain, mode: mode})
		}
	}
	sort.Slice(modes, func(i, j int) bool {
		if modes[i].domain == modes[j].domain {
			return modes[i].mode < modes[j].mode
		}
		return modes[i].domain < modes[j].domain
	})
	for index, item := range modes {
		fmt.Fprintf(&content, "SecRule SERVER_NAME \"@streq %s\" \"id:%d,phase:1,pass,nolog,ctl:ruleEngine=%s\"\n", item.domain, 101000+index, item.mode)
	}
	return content.String()
}

func (s *WebsiteService) wafModSecurityThresholdConfig() string {
	paranoia := s.global.ParanoiaLevel
	if paranoia < 1 {
		paranoia = 1
	}
	threshold := s.global.InboundThreshold
	if threshold < 1 {
		threshold = 5
	}
	return fmt.Sprintf("SecAction \"id:100900,phase:1,nolog,pass,t:none,setvar:tx.paranoia_level=%d,setvar:tx.blocking_paranoia_level=%d,setvar:tx.detection_paranoia_level=%d,setvar:tx.inbound_anomaly_score_threshold=%d\"\n", paranoia, paranoia, paranoia, threshold)
}

func defaultWAFSite(id uint, alias string) model.WAFSite {
	return model.WAFSite{
		WebsiteID: id, Alias: alias, Enabled: true, Mode: "observe",
		DetectionLevel: 1, FrequencyEnabled: true, Rules: []model.WAFRule{},
	}
}
