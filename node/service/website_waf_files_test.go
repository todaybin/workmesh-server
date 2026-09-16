package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

func TestWAFFilesAreAuthoritative(t *testing.T) {
	root := t.TempDir()
	sites := filepath.Join(root, "sites")
	t.Setenv("WORKMESH_WEBSITE_ROOT", sites)
	t.Setenv("WORKMESH_WAF_ROOT", filepath.Join(root, "waf"))
	svc := &WebsiteService{root: root, websiteRoot: sites, websites: []model.Website{{ID: 1, PrimaryDomain: "file-waf.example", Alias: "file"}}, wafSites: map[uint]model.WAFSite{}, domains: map[uint][]model.WebsiteDomain{}, configs: map[uint]map[string]any{}}
	website := svc.websites[0]
	if _, err := svc.UpdateWAFSite(website.ID, false, "block"); err != nil {
		t.Fatal(err)
	}
	rule, err := svc.UpsertRule(website.ID, model.WAFRule{Name: "admin", Location: "uri", Operator: "contains", Value: "/admin", Action: "block", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.UpdateGlobal(model.WAFGlobalConfig{Enabled: false, StandardRules: true, Mode: "block", ParanoiaLevel: 2, InboundThreshold: 7, RequestBodyLimit: 2048}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.UpdateLists(model.WAFAccessLists{Whitelist: []string{"192.0.2.1"}, Blacklist: []string{"198.51.100.0/24"}}); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(sites, website.PrimaryDomain, "waf")
	for _, name := range []string{"config.json", "rules.json"} {
		if _, err := os.Stat(filepath.Join(base, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	var diskRules []model.WAFRule
	data, _ := os.ReadFile(filepath.Join(base, "rules.json"))
	if err := json.Unmarshal(data, &diskRules); err != nil {
		t.Fatal(err)
	}
	if len(diskRules) != 1 || diskRules[0].ID != rule.ID {
		t.Fatalf("rules not persisted: %#v", diskRules)
	}
	reloaded := &WebsiteService{root: root, websiteRoot: sites, websites: []model.Website{website}, wafSites: map[uint]model.WAFSite{}, domains: map[uint][]model.WebsiteDomain{}, configs: map[uint]map[string]any{}}
	reloaded.load()
	rules, err := reloaded.ListRules(website.ID)
	if err != nil || len(rules) != 1 {
		t.Fatalf("reload rules: %v %#v", err, rules)
	}
	if cfg := reloaded.GetGlobal(); cfg.Enabled || cfg.Mode != "block" || cfg.InboundThreshold != 7 {
		t.Fatalf("reload global: %#v", cfg)
	}
	if lists := reloaded.GetLists(); len(lists.Whitelist) != 1 || len(lists.Blacklist) != 1 {
		t.Fatalf("reload lists: %#v", lists)
	}
	if _, err := os.Stat(filepath.Join(root, "waf", "global.json")); err != nil {
		t.Fatal(err)
	}
}

func TestWAFRuntimeApplyRequiresOpenRestyReload(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WAF_ROOT", filepath.Join(root, "waf"))
	logPath := filepath.Join(root, "openresty.log")
	binPath := filepath.Join(root, "fake-openresty.sh")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> '" + logPath + "'\n" +
		"if [ \"$1\" = \"-v\" ]; then echo 'nginx version: openresty/1.27.1'; exit 0; fi\n" +
		"if [ \"$1\" = \"-t\" ]; then exit 0; fi\n" +
		"if [ \"$1\" = \"-T\" ]; then echo 'modsecurity_rules_file /etc/workmesh-waf/modsecurity.conf'; echo 'access_by_lua_file /usr/local/openresty/lualib/workmesh_waf/access.lua'; exit 0; fi\n" +
		"if [ \"$1\" = \"-s\" ] && [ \"$2\" = \"reload\" ]; then exit 0; fi\n" +
		"exit 2\n"
	if err := os.WriteFile(binPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_WAF_ENFORCE", "1")
	t.Setenv("WORKMESH_OPENRESTY_BIN", binPath)
	svc := &WebsiteService{root: root, wafSites: map[uint]model.WAFSite{}}
	if _, err := svc.UpdateGlobal(model.WAFGlobalConfig{Enabled: true, StandardRules: true, Mode: "block", ParanoiaLevel: 1, InboundThreshold: 5, RequestBodyLimit: 1048576, Frequency: map[string]model.WAFFrequencyLimit{"access": {Enabled: true, Mode: "uri", Period: 10, Count: 100, BlockTime: 600}}}); err != nil {
		t.Fatal(err)
	}
	logs, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logs), "-t") || !strings.Contains(string(logs), "-s reload") {
		t.Fatalf("WAF apply did not check and reload OpenResty: %s", logs)
	}
	if !strings.Contains(string(logs), "-T") {
		t.Fatalf("WAF apply did not verify the expanded OpenResty configuration: %s", logs)
	}
	var disk model.WAFGlobalConfig
	if err := readJSON(filepath.Join(root, "waf", "global.json"), &disk); err != nil {
		t.Fatal(err)
	}
	if len(disk.FrequencyLimit) == 0 || disk.FrequencyLimit["access"].BlockTime != 600 {
		t.Fatalf("frequencyLimit not written for runtime: %#v", disk)
	}
	status := svc.WAFRuntimeStatus()
	if status["effective"] != true || status["configHash"] == "" {
		t.Fatalf("runtime not marked effective after reload: %#v", status)
	}
}

func TestWAFSiteFrequencyTogglePreservesRateLimits(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WAF_ROOT", filepath.Join(root, "waf"))
	svc := &WebsiteService{
		root: root, websiteRoot: root,
		websites: []model.Website{{ID: 1, PrimaryDomain: "frequency.example", Alias: "frequency"}},
		wafSites: map[uint]model.WAFSite{1: {
			WebsiteID: 1, Alias: "frequency", Enabled: true, Mode: "observe",
			FrequencyEnabled: true,
			RateLimits: map[string]model.WAFFrequencyLimit{
				"access": {Enabled: true, Mode: "uri", Period: 10, Count: 20, BlockTime: 60},
			},
			Rules: []model.WAFRule{},
		}},
	}
	enabled := true
	got, err := svc.UpdateWAFSiteSettings(1, true, "observe", &enabled)
	if err != nil {
		t.Fatal(err)
	}
	if got.RateLimits["access"].Count != 20 {
		t.Fatalf("enabling frequency must preserve existing limits: %#v", got.RateLimits)
	}
	disabled := false
	got, err = svc.UpdateWAFSiteSettings(1, true, "observe", &disabled)
	if err != nil {
		t.Fatal(err)
	}
	if got.FrequencyEnabled || len(got.RateLimits) != 0 {
		t.Fatalf("disabling frequency must clear limits: %#v", got)
	}
}

func TestWAFSiteSaveGeneratesRuntimeConfigAndReloads(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WAF_ROOT", filepath.Join(root, "waf"))
	t.Setenv("WORKMESH_WAF_ENFORCE", "1")
	logPath := filepath.Join(root, "openresty.log")
	binPath := filepath.Join(root, "fake-openresty.sh")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> '" + logPath + "'\n" +
		"if [ \"$1\" = \"-v\" ]; then echo 'nginx version: openresty/1.27.1'; exit 0; fi\n" +
		"if [ \"$1\" = \"-t\" ]; then exit 0; fi\n" +
		"if [ \"$1\" = \"-T\" ]; then echo 'modsecurity_rules_file /etc/workmesh-waf/modsecurity.conf'; echo 'access_by_lua_file /usr/local/openresty/lualib/workmesh_waf/access.lua'; exit 0; fi\n" +
		"if [ \"$1\" = \"-s\" ] && [ \"$2\" = \"reload\" ]; then exit 0; fi\n" +
		"exit 2\n"
	if err := os.WriteFile(binPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_OPENRESTY_BIN", binPath)
	svc := &WebsiteService{
		root: root, websiteRoot: root,
		websites: []model.Website{{ID: 7, PrimaryDomain: "site-runtime.example", Alias: "site-runtime"}},
		wafSites: map[uint]model.WAFSite{},
		global: model.WAFGlobalConfig{
			Enabled: true, Mode: "observe", StandardRules: true,
			ParanoiaLevel: 2, InboundThreshold: 7, RequestBodyLimit: 4096,
		},
	}
	site, err := svc.UpdateWAFSite(7, true, "block")
	if err != nil {
		t.Fatal(err)
	}
	if site.Mode != "block" {
		t.Fatalf("unexpected saved site mode: %#v", site)
	}
	data, err := os.ReadFile(filepath.Join(root, "waf", "generated", "custom-rules.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `@streq site-runtime.example`) ||
		!strings.Contains(string(data), `ctl:ruleEngine=On`) ||
		!strings.Contains(string(data), "SecRequestBodyLimit 4096") {
		t.Fatalf("site save did not generate effective ModSecurity configuration: %s", data)
	}
	logs, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, argument := range []string{"-t", "-s reload", "-T"} {
		if !strings.Contains(string(logs), argument) {
			t.Fatalf("site save did not execute %s: %s", argument, logs)
		}
	}
}

func TestWAFRuntimeApplyRejectsUnloadedWAFHook(t *testing.T) {
	root := t.TempDir()
	wafRoot := filepath.Join(root, "waf")
	t.Setenv("WORKMESH_WAF_ROOT", wafRoot)
	t.Setenv("WORKMESH_WAF_ENFORCE", "1")
	binPath := filepath.Join(root, "fake-openresty-no-waf.sh")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"-v\" ]; then echo 'nginx version: openresty/1.27.1'; exit 0; fi\n" +
		"if [ \"$1\" = \"-t\" ]; then exit 0; fi\n" +
		"if [ \"$1\" = \"-T\" ]; then echo 'http { include /tmp/unrelated.conf; }'; exit 0; fi\n" +
		"if [ \"$1\" = \"-s\" ] && [ \"$2\" = \"reload\" ]; then exit 0; fi\n" +
		"exit 2\n"
	if err := os.WriteFile(binPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_OPENRESTY_BIN", binPath)
	svc := &WebsiteService{root: root, wafSites: map[uint]model.WAFSite{}}
	t.Setenv("WORKMESH_WAF_ENFORCE", "0")
	if _, err := svc.UpdateGlobal(model.WAFGlobalConfig{Enabled: true, StandardRules: true, Mode: "observe", ParanoiaLevel: 1, InboundThreshold: 5}); err != nil {
		t.Fatal(err)
	}
	previous, err := os.ReadFile(filepath.Join(wafRoot, "global.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_WAF_ENFORCE", "1")
	if _, err := svc.UpdateGlobal(model.WAFGlobalConfig{Enabled: true, StandardRules: true, Mode: "block", ParanoiaLevel: 1, InboundThreshold: 5}); err == nil || !strings.Contains(err.Error(), "未加载 WorkMesh WAF") {
		t.Fatalf("unloaded WAF hook must reject the write, got %v", err)
	}
	current, err := os.ReadFile(filepath.Join(wafRoot, "global.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(previous) {
		t.Fatalf("failed runtime verification left global.json changed:\nprevious=%s\ncurrent=%s", previous, current)
	}
	if svc.GetGlobal().Mode != "observe" {
		t.Fatalf("failed runtime verification left in-memory global mode changed: %#v", svc.GetGlobal())
	}
}

func TestApplyDefaultWAFIncludesWebsitesWithoutLoadedSiteConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WAF_ROOT", filepath.Join(root, "waf"))
	t.Setenv("WORKMESH_WEBSITE_ROOT", root)
	svc := &WebsiteService{
		root: root, websiteRoot: root,
		websites: []model.Website{
			{ID: 1, PrimaryDomain: "loaded.example", Alias: "loaded"},
			{ID: 2, PrimaryDomain: "missing.example", Alias: "missing"},
		},
		wafSites: map[uint]model.WAFSite{
			1: {WebsiteID: 1, Alias: "loaded", Enabled: true, Mode: "observe"},
		},
	}
	rules := []model.WAFRule{{ID: "default-apply", Name: "block-admin", Location: "uri", Operator: "contains", Value: "/admin", Action: "block", Enabled: true}}
	if _, err := svc.UpdateDefaultWAFRules(rules); err != nil {
		t.Fatal(err)
	}
	if err := svc.ApplyDefaultWAFToWebsites(); err != nil {
		t.Fatal(err)
	}
	site, ok := svc.wafSites[2]
	if !ok || len(site.Rules) != 1 || site.Rules[0].ID != "default-apply" {
		t.Fatalf("default rules were not applied to an uninitialized website: %#v", svc.wafSites)
	}
	data, err := os.ReadFile(filepath.Join(root, "missing.example", "waf", "rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "default-apply") {
		t.Fatalf("missing website rules file was not written: %s", data)
	}
}

func TestWAFRuntimeConfigurationTreatsExplicitFalseAsDisabled(t *testing.T) {
	for _, value := range []string{"0", "false", "no", "off", " FALSE "} {
		t.Setenv("WORKMESH_WAF_ENFORCE", "")
		t.Setenv("WORKMESH_WAF_RELOAD", value)
		t.Setenv("WORKMESH_OPENRESTY_BIN", "")
		t.Setenv("WORKMESH_OPENRESTY_CONTAINER", "")
		if wafRuntimeConfigured() {
			t.Fatalf("WORKMESH_WAF_RELOAD=%q should not force runtime validation", value)
		}
	}
}

func TestProductionWAFRootRequiresRuntimeValidation(t *testing.T) {
	t.Setenv("WORKMESH_WAF_ENFORCE", "")
	t.Setenv("WORKMESH_WAF_RELOAD", "")
	t.Setenv("WORKMESH_OPENRESTY_BIN", "")
	t.Setenv("WORKMESH_OPENRESTY_CONTAINER", "")
	svc := &WebsiteService{root: "/opt/workmesh-server/data"}
	if !svc.wafRuntimeRequired() {
		t.Fatal("production data root must require OpenResty runtime validation")
	}
	t.Setenv("WORKMESH_WAF_ENFORCE", "0")
	if svc.wafRuntimeRequired() {
		t.Fatal("explicit WORKMESH_WAF_ENFORCE=0 should disable runtime validation")
	}
}

func TestProductionWAFWriteRejectsWithoutOpenResty(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WAF_ROOT", filepath.Join(root, "waf"))
	t.Setenv("WORKMESH_WAF_ENFORCE", "")
	t.Setenv("WORKMESH_WAF_RELOAD", "")
	t.Setenv("WORKMESH_OPENRESTY_CONTAINER", "")
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(root, "missing-openresty"))
	svc := &WebsiteService{root: "/opt/workmesh-server/data", wafSites: map[uint]model.WAFSite{}}

	if _, err := svc.UpdateGlobal(model.WAFGlobalConfig{Enabled: true, Mode: "block", StandardRules: true}); err == nil {
		t.Fatal("production WAF write must fail when OpenResty cannot be validated")
	}
	if _, err := os.Stat(filepath.Join(root, "waf", "global.json")); !os.IsNotExist(err) {
		t.Fatalf("failed production WAF write left global.json behind: %v", err)
	}
}

func TestWAFListMetadataPersistsAndFiltersToKnownEntries(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WAF_ROOT", filepath.Join(root, "waf"))
	svc := &WebsiteService{root: root, wafSites: map[uint]model.WAFSite{}}
	lists, err := svc.UpdateLists(model.WAFAccessLists{
		Blacklist: []string{"198.51.100.1", "198.51.100.2"},
		ListMeta: map[string]model.WAFListMeta{
			"blacklist:198.51.100.1": {Enabled: false, Remark: "temporary"},
			"blacklist:unknown":      {Enabled: false, Remark: "must drop"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(lists.ListMeta) != 1 || lists.ListMeta["blacklist:198.51.100.1"].Enabled {
		t.Fatalf("unexpected list metadata: %#v", lists.ListMeta)
	}
	reloaded := &WebsiteService{root: root, wafSites: map[uint]model.WAFSite{}}
	reloaded.load()
	got := reloaded.GetLists()
	if got.ListMeta["blacklist:198.51.100.1"].Remark != "temporary" || len(got.ListMeta) != 1 {
		t.Fatalf("list metadata was not persisted: %#v", got.ListMeta)
	}
}

func TestWAFRuntimeApplyRollsBackOnReloadFailure(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WAF_ROOT", filepath.Join(root, "waf"))
	svc := &WebsiteService{root: root, wafSites: map[uint]model.WAFSite{}}
	t.Setenv("WORKMESH_WAF_ENFORCE", "0")
	if _, err := svc.UpdateLists(model.WAFAccessLists{Whitelist: []string{"192.0.2.1"}, Blacklist: []string{"198.51.100.1"}}); err != nil {
		t.Fatal(err)
	}
	oldData, err := os.ReadFile(filepath.Join(root, "waf", "access-lists.json"))
	if err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(root, "fake-openresty-fail.sh")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"-v\" ]; then echo 'nginx version: openresty/1.27.1'; exit 0; fi\n" +
		"if [ \"$1\" = \"-t\" ]; then exit 0; fi\n" +
		"if [ \"$1\" = \"-T\" ]; then echo 'modsecurity_rules_file /etc/workmesh-waf/modsecurity.conf'; echo 'access_by_lua_file /usr/local/openresty/lualib/workmesh_waf/access.lua'; exit 0; fi\n" +
		"if [ \"$1\" = \"-s\" ] && [ \"$2\" = \"reload\" ]; then echo reload failed >&2; exit 1; fi\n" +
		"exit 2\n"
	if err := os.WriteFile(binPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_WAF_ENFORCE", "1")
	t.Setenv("WORKMESH_OPENRESTY_BIN", binPath)
	if _, err := svc.UpdateLists(model.WAFAccessLists{Whitelist: []string{"203.0.113.1"}, Blacklist: []string{"203.0.113.2"}}); err == nil || !strings.Contains(err.Error(), "reload") {
		t.Fatalf("reload failure should reject WAF write, got %v", err)
	}
	if got := svc.GetLists(); len(got.Whitelist) != 1 || got.Whitelist[0] != "192.0.2.1" {
		t.Fatalf("in-memory WAF lists not rolled back: %#v", got)
	}
	newData, err := os.ReadFile(filepath.Join(root, "waf", "access-lists.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(newData) != string(oldData) {
		t.Fatalf("disk WAF lists not rolled back:\nold=%s\nnew=%s", oldData, newData)
	}
}

func TestWAFRuntimeRejectsOpenRestyWithoutWAFWiring(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WAF_ROOT", filepath.Join(root, "waf"))
	binPath := filepath.Join(root, "fake-openresty-unwired.sh")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"-v\" ]; then echo 'nginx version: openresty/1.27.1'; exit 0; fi\n" +
		"if [ \"$1\" = \"-t\" ]; then exit 0; fi\n" +
		"if [ \"$1\" = \"-T\" ]; then echo 'events {}'; exit 0; fi\n" +
		"exit 2\n"
	if err := os.WriteFile(binPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_WAF_ENFORCE", "1")
	t.Setenv("WORKMESH_OPENRESTY_BIN", binPath)
	svc := &WebsiteService{root: root, wafSites: map[uint]model.WAFSite{}}

	if _, err := svc.UpdateGlobal(model.WAFGlobalConfig{Enabled: true, Mode: "block", StandardRules: true, ParanoiaLevel: 1, InboundThreshold: 5}); err == nil ||
		!strings.Contains(err.Error(), "未加载 WorkMesh WAF 钩子") {
		t.Fatalf("unwired OpenResty must reject WAF write, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "waf", "global.json")); !os.IsNotExist(err) {
		t.Fatalf("unwired WAF write left global.json behind: %v", err)
	}
}

func TestWAFGlobalRulesUseRuntimeValidation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WAF_ROOT", filepath.Join(root, "waf"))
	svc := &WebsiteService{root: root, wafSites: map[uint]model.WAFSite{}}
	if _, err := svc.UpdateDefaultWAFRules([]model.WAFRule{{Name: "invalid", Location: "uri", Operator: "unknown", Value: "x", Action: "block"}}); err == nil {
		t.Fatal("default WAF rules must reject unsupported operators")
	}
	if _, err := svc.UpdateCustomWAFRules([]model.WAFRule{{Name: "invalid", Location: "uri", Operator: "regex", Value: "[", Action: "block"}}); err == nil {
		t.Fatal("custom WAF rules must reject invalid regular expressions")
	}
}

func TestWAFSiteSettingsPreserveRateLimitsAndUseSiteDir(t *testing.T) {
	root := t.TempDir()
	siteDir := filepath.Join(root, "custom-site")
	t.Setenv("WORKMESH_WAF_ROOT", filepath.Join(root, "waf"))
	svc := &WebsiteService{
		root: root, websiteRoot: filepath.Join(root, "sites"),
		websites: []model.Website{{ID: 1, PrimaryDomain: "site-dir.example", Alias: "site", SiteDir: siteDir}},
		wafSites: map[uint]model.WAFSite{1: {
			WebsiteID: 1, Alias: "site", Enabled: true, Mode: "observe",
			DetectionLevel: 1, FrequencyEnabled: true,
			RateLimits: map[string]model.WAFFrequencyLimit{
				"access": {Enabled: true, Mode: "uri", Period: 10, Count: 100, BlockTime: 5},
			},
		}},
	}
	disabled := false
	if _, err := svc.UpdateWAFSiteSettingsWithDetection(1, true, "block", &disabled, nil); err != nil {
		t.Fatal(err)
	}
	if len(svc.wafSites[1].RateLimits) != 0 {
		t.Fatalf("disabled frequency must clear site rate limits: %#v", svc.wafSites[1].RateLimits)
	}
	configPath := filepath.Join(siteDir, "waf", "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("site WAF config was not written to SiteDir: %v", err)
	}
	defaultPath := filepath.Join(svc.websiteRoot, "site-dir.example", "waf", "config.json")
	if _, err := os.Stat(defaultPath); !os.IsNotExist(err) {
		t.Fatalf("site WAF config was incorrectly written to domain fallback: %v", err)
	}
}

func TestWebsiteUpdateReturnsWAFReloadFailure(t *testing.T) {
	root := t.TempDir()
	siteRoot := filepath.Join(root, "wwwroot")
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_WEBSITE_ROOT", siteRoot)
	svc := NewWebsiteService(root)
	site, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "waf-update.example", Alias: "old-alias", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	oldConfig, err := os.ReadFile(filepath.Join(svc.SitePath(site, "waf"), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(root, "fake-openresty-update-fail.sh")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"-v\" ]; then echo 'nginx version: openresty/1.27.1'; exit 0; fi\n" +
		"if [ \"$1\" = \"-t\" ]; then exit 0; fi\n" +
		"if [ \"$1\" = \"-T\" ]; then echo 'modsecurity_rules_file /etc/workmesh-waf/modsecurity.conf'; echo 'access_by_lua_file /usr/local/openresty/lualib/workmesh_waf/access.lua'; exit 0; fi\n" +
		"if [ \"$1\" = \"-s\" ] && [ \"$2\" = \"reload\" ]; then echo reload failed >&2; exit 1; fi\n" +
		"exit 2\n"
	if err := os.WriteFile(binPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_WAF_ENFORCE", "1")
	t.Setenv("WORKMESH_OPENRESTY_BIN", binPath)
	if _, err := svc.Update(model.WebsiteUpdateRequest{ID: site.ID, Alias: "new-alias"}); err == nil || !strings.Contains(err.Error(), "reload") {
		t.Fatalf("WAF reload failure should reject website update, got %v", err)
	}
	reloaded, err := svc.Get(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Alias != "old-alias" {
		t.Fatalf("website alias was not rolled back in memory: %#v", reloaded)
	}
	var storedAlias string
	if err := svc.db.QueryRow(`SELECT alias FROM websites WHERE id=?`, site.ID).Scan(&storedAlias); err != nil {
		t.Fatal(err)
	}
	if storedAlias != "old-alias" {
		t.Fatalf("website alias was not rolled back in SQLite: %q", storedAlias)
	}
	currentConfig, err := os.ReadFile(filepath.Join(svc.SitePath(site, "waf"), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(currentConfig) != string(oldConfig) {
		t.Fatalf("WAF site config was not rolled back:\nold=%s\nnew=%s", oldConfig, currentConfig)
	}
}

func TestWAFGlobalRuleFiles(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WAF_ROOT", filepath.Join(root, "waf"))
	svc := &WebsiteService{root: root, wafSites: map[uint]model.WAFSite{}}
	want := []model.WAFRule{{ID: "default-1", Name: "default", Location: "uri", Operator: "contains", Value: "/blocked", Action: "block", Enabled: true}}
	if _, err := svc.UpdateDefaultWAFRules(want); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ListDefaultWAFRules()
	if err != nil || len(got) != 1 || got[0].ID != "default-1" {
		t.Fatalf("default rules: %v %#v", err, got)
	}
	if _, err := svc.UpdateCustomWAFRules(want); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "waf", "custom-rules.json")); err != nil {
		t.Fatal(err)
	}
}

func TestQueryWAFLogsReadsJSONLFiles(t *testing.T) {
	root := t.TempDir()
	sites := filepath.Join(root, "sites")
	t.Setenv("WORKMESH_WEBSITE_ROOT", sites)
	svc := &WebsiteService{root: root, websiteRoot: sites, websites: []model.Website{{ID: 1, PrimaryDomain: "logs.example"}}, wafSites: map[uint]model.WAFSite{}, domains: map[uint][]model.WebsiteDomain{}, configs: map[uint]map[string]any{}}
	path := filepath.Join(sites, "logs.example", "waf", "logs", "attack.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	content := `{"timestamp":"2026-09-09T10:00:00Z","ruleID":"SQL-1","action":"block","uri":"/admin"}` + "\n" +
		`{"timestamp":"2026-09-09T09:00:00Z","ruleID":"XSS-1","action":"log","uri":"/"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	result, err := svc.QueryWAFLogs("attack", map[string]string{"ruleID": "SQL-1"}, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if result["total"] != 1 {
		t.Fatalf("unexpected total: %#v", result)
	}
	items, ok := result["items"].([]map[string]any)
	if !ok || len(items) != 1 || items[0]["ruleID"] != "SQL-1" {
		t.Fatalf("unexpected items: %#v", result["items"])
	}
}

func TestQueryWAFLogsReadsModSecurityAuditRecords(t *testing.T) {
	root := t.TempDir()
	logRoot := filepath.Join(root, "waf")
	t.Setenv("WORKMESH_WAF_ROOT", logRoot)
	svc := &WebsiteService{
		root: root, wafSites: map[uint]model.WAFSite{},
	}
	path := filepath.Join(logRoot, "logs", "modsecurity-audit.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	content := `[
		{"transaction":{"client_ip":"198.51.100.21","time_stamp":"2026-09-12T10:00:00Z","request":{"uri":"/admin","headers":{"host":"waf.example","Host":"ignored.example"}},"response":{"http_code":403}},"messages":[{"message":"blocked"}]},
		{"transaction":{"client_ip":"198.51.100.22","time_stamp":"2026-09-12T10:01:00Z","request":{"uri":"/","headers":{"host":"waf.example"}},"response":{"http_code":200}}}
	]`
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}

	result, err := svc.QueryWAFLogs("intercept", map[string]string{
		"ip": "198.51.100.21", "status": "403", "host": "waf.example", "keyword": "/admin",
	}, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := result["items"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("unexpected ModSecurity items: %#v", result["items"])
	}
	item := items[0]
	if item["client_ip"] != "198.51.100.21" || item["timestamp"] != "2026-09-12T10:00:00Z" || item["time"] != "2026-09-12T10:00:00Z" {
		t.Fatalf("ModSecurity transaction fields were not normalized: %#v", item)
	}
	if item["status"] != float64(403) || item["uri"] != "/admin" || item["host"] != "waf.example" {
		t.Fatalf("ModSecurity request/response fields were not normalized: %#v", item)
	}
}

func TestParseWAFJSONRecordsSupportsObjectAndJSONL(t *testing.T) {
	object := parseWAFJSONRecords(`{"transaction":{"client_ip":"192.0.2.10","time_stamp":"2026-09-12T10:00:00Z"}}`)
	if len(object) != 1 || object[0]["client_ip"] != "192.0.2.10" {
		t.Fatalf("single ModSecurity object was not parsed: %#v", object)
	}
	jsonl := parseWAFJSONRecords(
		`{"transaction":{"client_ip":"192.0.2.11","time_stamp":"2026-09-12T10:01:00Z"}}` + "\n" +
			`{"transaction":{"client_ip":"192.0.2.12","time_stamp":"2026-09-12T10:02:00Z"}}` + "\n",
	)
	if len(jsonl) != 2 || jsonl[1]["client_ip"] != "192.0.2.12" {
		t.Fatalf("ModSecurity JSONL was not parsed: %#v", jsonl)
	}
}

func TestQueryWAFLogsReadsNginxCombinedAccessLog(t *testing.T) {
	root := t.TempDir()
	sites := filepath.Join(root, "sites")
	t.Setenv("WORKMESH_WEBSITE_ROOT", sites)
	svc := &WebsiteService{
		root: root, websiteRoot: sites,
		websites: []model.Website{{ID: 9, PrimaryDomain: "access.example"}},
		wafSites: map[uint]model.WAFSite{}, domains: map[uint][]model.WebsiteDomain{}, configs: map[uint]map[string]any{},
	}
	path := filepath.Join(sites, "access.example", "waf", "logs", "access.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	content := `203.0.113.8 - - [12/Sep/2026:10:00:00 +0000] "GET /admin HTTP/1.1" 403 123 "-" "Mozilla/5.0"
not a combined access line
not-an-ip - - [12/Sep/2026:10:01:00 +0000] "GET /ignored HTTP/1.1" 200 12 "-" "agent"
203.0.113.9 - - [12/Sep/2026:10:02:00 +0000] "POST /login HTTP/1.1" 200 - "https://ref.example/" "curl/8.0"`
	if err := os.WriteFile(path, []byte(content+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	result, err := svc.QueryWAFLogs("log", map[string]string{
		"ip": "203.0.113.8", "status": "403", "keyword": "/admin",
	}, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := result["items"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("unexpected combined access items: %#v", result["items"])
	}
	item := items[0]
	if item["client_ip"] != "203.0.113.8" || item["method"] != "GET" || item["uri"] != "/admin" {
		t.Fatalf("combined request fields were not parsed: %#v", item)
	}
	if item["status"] != 403 || item["host"] != "access.example" || item["source"] != "access-log" {
		t.Fatalf("combined access metadata was not normalized: %#v", item)
	}
	if item["websiteID"] != uint(9) {
		t.Fatalf("combined access website metadata was not applied: %#v", item)
	}
}

func TestQueryWAFLogsFiltersTimeStatusWebsiteAndKeyword(t *testing.T) {
	root := t.TempDir()
	sites := filepath.Join(root, "sites")
	t.Setenv("WORKMESH_WEBSITE_ROOT", sites)
	svc := &WebsiteService{
		root: root, websiteRoot: sites,
		websites: []model.Website{{ID: 7, PrimaryDomain: "filter.example"}},
		wafSites: map[uint]model.WAFSite{}, domains: map[uint][]model.WebsiteDomain{}, configs: map[uint]map[string]any{},
	}
	path := filepath.Join(sites, "filter.example", "waf", "logs", "audit.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	content := `{"timestamp":"2026-09-10T10:00:00Z","websiteID":7,"status":403,"client_ip":"198.51.100.8","ipRegion":"中国","rule":"SQL-1","uri":"/admin"}` + "\n" +
		`{"timestamp":"2026-09-08T10:00:00Z","websiteID":7,"status":200,"client_ip":"198.51.100.9","rule":"allow","uri":"/"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	result, err := svc.QueryWAFLogs("intercept", map[string]string{
		"websiteID": "7", "status": "403", "ipRegion": "中国", "keyword": "admin",
		"startTime": "2026-09-09T00:00:00Z", "endTime": "2026-09-11T00:00:00Z",
	}, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if result["total"] != 1 {
		t.Fatalf("filters did not narrow records: %#v", result)
	}
}

func TestWAFOverviewAggregatesAllRecords(t *testing.T) {
	root := t.TempDir()
	sites := filepath.Join(root, "sites")
	t.Setenv("WORKMESH_WEBSITE_ROOT", sites)
	svc := &WebsiteService{
		root: root, websiteRoot: sites,
		websites: []model.Website{{ID: 1, PrimaryDomain: "overview.example"}},
		wafSites: map[uint]model.WAFSite{}, domains: map[uint][]model.WebsiteDomain{}, configs: map[uint]map[string]any{},
	}
	logDir := filepath.Join(sites, "overview.example", "waf", "logs")
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	requests := `{"timestamp":"` + now.Format(time.RFC3339) + `","status":200}` + "\n" +
		`{"timestamp":"` + now.Add(-24*time.Hour).Format(time.RFC3339) + `","status":404}` + "\n"
	intercepts := `{"timestamp":"` + now.Format(time.RFC3339) + `","status":403,"ipRegion":"中国"}`
	if err := os.WriteFile(filepath.Join(logDir, "audit.jsonl"), []byte(requests), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "intercept.jsonl"), []byte(intercepts+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	result, err := svc.WAFOverview()
	if err != nil {
		t.Fatal(err)
	}
	today := result["today"].(map[string]any)
	if today["requests"] != 1 || today["intercepts"] != 1 {
		t.Fatalf("overview counts incorrect: %#v", today)
	}
	trend := result["requestTrend"].([]map[string]any)
	total := 0
	for _, item := range trend {
		total += item["count"].(int)
	}
	if total != 2 {
		t.Fatalf("overview trend did not aggregate all request records: %#v", trend)
	}
}

func TestProductionWAFRootAndInlineRulesCompatibility(t *testing.T) {
	t.Setenv("WORKMESH_WAF_ROOT", "")
	t.Setenv("WORKMESH_WAF_GLOBAL_DIR", "")
	oldRoot := "/opt/workmesh-server/data"
	svc := &WebsiteService{root: oldRoot, wafSites: map[uint]model.WAFSite{}}
	if got := svc.wafRoot(); got != "/opt/workmesh-server/openresty-waf/waf/data" {
		t.Fatalf("unexpected production WAF root: %s", got)
	}
	root := t.TempDir()
	svc.root, svc.websiteRoot = root, filepath.Join(root, "sites")
	svc.websites = []model.Website{{ID: 1, PrimaryDomain: "compat.example", Alias: "compat"}}
	svc.wafSites = map[uint]model.WAFSite{1: {WebsiteID: 1, Alias: "compat", Enabled: true, Mode: "block", Rules: []model.WAFRule{{ID: "r1", Name: "admin", Location: "uri", Operator: "contains", Value: "/admin", Action: "block", Enabled: true}}}}
	if err := svc.persistWAFSites(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "sites", "compat.example", "waf", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Rules []model.WAFRule `json:"rules"`
	}
	if err := json.Unmarshal(data, &config); err != nil || len(config.Rules) != 1 || config.Rules[0].ID != "r1" {
		t.Fatalf("inline rules missing: %s", data)
	}
}
