// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/todaybin/workmesh-server/node/model"
)

func TestProbeOpenRestyMissingBinary(t *testing.T) {
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(t.TempDir(), "missing-openresty"))
	svc := NewWebsiteService(t.TempDir())
	status := svc.ProbeOpenResty(context.Background())
	if status.Available {
		t.Fatalf("missing binary should not be available: %#v", status)
	}
}

func TestParseOpenRestyVersion(t *testing.T) {
	if got := parseOpenRestyVersion("nginx version: openresty/1.25.3.1"); got != "1.25.3.1" {
		t.Fatalf("unexpected version %q", got)
	}
}

func TestWebsiteServicePersistsWebsiteAndWAF(t *testing.T) {
	root := t.TempDir()
	svc := NewWebsiteService(root)
	website, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "example.com", Alias: "示例站点", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertRule(website.ID, model.WAFRule{Name: "阻止测试", Location: "uri", Operator: "contains", Value: "/admin", Action: "block", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateLists(model.WAFAccessLists{Whitelist: []string{"10.0.0.1", "10.0.0.1"}, Blacklist: []string{"192.0.2.0/24"}}); err != nil {
		t.Fatal(err)
	}
	reloaded := NewWebsiteService(root)
	items := reloaded.List("example", 0, 10)
	if len(items) != 1 || items[0].ID != website.ID {
		t.Fatalf("网站未持久化: %#v", items)
	}
	rules, err := reloaded.ListRules(website.ID)
	if err != nil || len(rules) != 1 {
		t.Fatalf("规则未持久化: %v %#v", err, rules)
	}
	if got := reloaded.GetLists(); len(got.Whitelist) != 1 || got.Blacklist[0] != "192.0.2.0/24" {
		t.Fatalf("黑白名单错误: %#v", got)
	}
	for _, name := range []string{"websites.json", "waf-sites.json", "waf-access-lists.json"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("缺少持久化文件 %s: %v", name, err)
		}
	}
}

func TestWebsiteServiceRejectsInvalidWAFRule(t *testing.T) {
	root := t.TempDir()
	svc := NewWebsiteService(root)
	website, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "example.org"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertRule(website.ID, model.WAFRule{Name: "x", Location: "uri", Operator: "regex", Value: "[", Action: "block"}); err == nil {
		t.Fatal("无效正则应被拒绝")
	}
	if _, err := svc.UpdateLists(model.WAFAccessLists{Whitelist: []string{"bad-ip"}}); err == nil {
		t.Fatal("无效 IP 应被拒绝")
	}
}

func TestWebsiteServiceAdvancedStatePersists(t *testing.T) {
	root := t.TempDir()
	svc := NewWebsiteService(root)
	website, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "advanced.example"})
	if err != nil {
		t.Fatal(err)
	}
	domain, err := svc.UpsertDomain(model.WebsiteDomain{WebsiteID: website.ID, Domain: "www.advanced.example", Port: 443, SSL: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateConfig(website.ID, "nginx", map[string]any{"content": "server {}"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Operate(website.ID, "stop"); err != nil {
		t.Fatal(err)
	}
	reloaded := NewWebsiteService(root)
	items, err := reloaded.ListDomains(website.ID)
	if err != nil || len(items) != 1 || items[0].ID != domain.ID {
		t.Fatalf("域名未持久化: %v %#v", err, items)
	}
	cfg, err := reloaded.GetConfig(website.ID, "nginx")
	if err != nil || cfg["content"] != "server {}" {
		t.Fatalf("配置未持久化: %v %#v", err, cfg)
	}
	loaded, err := reloaded.Get(website.ID)
	if err != nil || loaded.Status != "stopped" {
		t.Fatalf("状态未持久化: %v %#v", err, loaded)
	}
}

func TestWebsiteServiceDomainValidation(t *testing.T) {
	svc := NewWebsiteService(t.TempDir())
	website, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "valid.example"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertDomain(model.WebsiteDomain{WebsiteID: website.ID, Domain: "bad..example"}); err == nil {
		t.Fatal("应拒绝连续点域名")
	}
	if _, err := svc.UpsertDomain(model.WebsiteDomain{WebsiteID: website.ID, Domain: "other.example", Port: 70000}); err == nil {
		t.Fatal("应拒绝越界端口")
	}
}
