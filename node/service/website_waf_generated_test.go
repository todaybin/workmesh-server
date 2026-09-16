package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/node/model"
)

func TestWAFGeneratedModSecurityModeFollowsGlobalConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WAF_ROOT", filepath.Join(root, "waf"))
	svc := &WebsiteService{root: root, global: model.WAFGlobalConfig{Enabled: true, Mode: "block"}}
	if err := svc.writeWAFGeneratedConfig(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "waf", "generated", "modsecurity-mode.conf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "SecRuleEngine On") {
		t.Fatalf("block mode was not generated: %s", data)
	}
	runtimeConfig, err := os.ReadFile(filepath.Join(root, "waf", "generated", "custom-rules.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runtimeConfig), "SecRequestBodyLimit 1048576") {
		t.Fatalf("request body limit was not generated: %s", runtimeConfig)
	}

	svc.global = model.WAFGlobalConfig{Enabled: true, Mode: "observe"}
	if err := svc.writeWAFGeneratedConfig(); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "SecRuleEngine DetectionOnly") {
		t.Fatalf("observe mode was not generated: %s", data)
	}

	svc.global.Enabled = false
	if err := svc.writeWAFGeneratedConfig(); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "SecRuleEngine Off") {
		t.Fatalf("disabled mode was not generated: %s", data)
	}

	svc.global = model.WAFGlobalConfig{Enabled: true, Mode: "block", StandardRules: false}
	if err := svc.writeWAFGeneratedConfig(); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(root, "waf", "generated", "standard-rules.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "SecRuleEngine Off") || strings.Contains(string(data), "Include /etc/workmesh-waf/crs") {
		t.Fatalf("standard rules disablement must not disable the ModSecurity engine or custom rules: %s", data)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "SecRuleEngine On") {
		t.Fatalf("standard rule disablement must keep block mode active: %s", data)
	}
	svc.global.StandardRules = true
	if err := svc.writeWAFGeneratedConfig(); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(root, "waf", "generated", "standard-rules.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Include /etc/workmesh-waf/crs/rules/*.conf") {
		t.Fatalf("standard rules include was not generated: %s", data)
	}
	if !strings.Contains(string(data), "tx.inbound_anomaly_score_threshold=5") {
		t.Fatalf("inbound threshold was not generated after CRS settings: %s", data)
	}
}

func TestWAFGeneratedSiteEngineControlsFollowSiteSettings(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WAF_ROOT", filepath.Join(root, "waf"))
	svc := &WebsiteService{
		root: root,
		websites: []model.Website{
			{ID: 1, PrimaryDomain: "block.example"},
			{ID: 2, PrimaryDomain: "observe.example"},
			{ID: 3, PrimaryDomain: "disabled.example"},
		},
		wafSites: map[uint]model.WAFSite{
			1: {WebsiteID: 1, Enabled: true, Mode: "block"},
			2: {WebsiteID: 2, Enabled: true, Mode: "observe"},
			3: {WebsiteID: 3, Enabled: false, Mode: "block"},
		},
		global: model.WAFGlobalConfig{Enabled: true, Mode: "observe", RequestBodyLimit: 2048},
	}
	if err := svc.writeWAFGeneratedConfig(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "waf", "generated", "custom-rules.conf"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		`SecRule SERVER_NAME "@streq block.example" "id:101000,phase:1,pass,nolog,ctl:ruleEngine=On"`,
		`SecRule SERVER_NAME "@streq disabled.example" "id:101001,phase:1,pass,nolog,ctl:ruleEngine=Off"`,
		`SecRule SERVER_NAME "@streq observe.example" "id:101002,phase:1,pass,nolog,ctl:ruleEngine=DetectionOnly"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing generated site engine control %q in %s", want, content)
		}
	}
}

func TestWAFGeneratedRootSupportsProductionDataMount(t *testing.T) {
	t.Setenv("WORKMESH_WAF_ROOT", "")
	t.Setenv("WORKMESH_WAF_GLOBAL_DIR", "")
	svc := &WebsiteService{root: "/opt/workmesh-server/data"}
	if got := svc.wafGeneratedRoot(); got != "/opt/workmesh-server/openresty-waf/waf/generated" {
		t.Fatalf("generated root = %q", got)
	}
	if got := svc.wafLogRoot(); got != "/opt/workmesh-server/openresty-waf/waf/logs" {
		t.Fatalf("log root = %q", got)
	}
	if got := svc.wafRoot(); got != "/opt/workmesh-server/openresty-waf/waf/data" {
		t.Fatalf("data root = %q", got)
	}
}

func TestWAFStartupRecreatesStandardRulesForBindMountedWAFRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WAF_ROOT", filepath.Join(root, "waf", "data"))
	svc := &WebsiteService{
		root:   root,
		global: model.WAFGlobalConfig{Enabled: true, StandardRules: true, Mode: "observe"},
	}
	if err := svc.writeWAFGeneratedConfig(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "waf", "generated", "standard-rules.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Include /etc/workmesh-waf/crs/rules/*.conf") {
		t.Fatalf("startup generated file does not enable CRS: %s", data)
	}
}
