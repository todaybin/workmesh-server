// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/todaybin/workmesh-server/node/model"
)

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
