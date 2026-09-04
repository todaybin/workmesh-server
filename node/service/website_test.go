// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
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

func TestContainerImageVersion(t *testing.T) {
	if got := containerImageVersion("registry.example/openresty:1.21.4.3"); got != "1.21.4.3" {
		t.Fatalf("unexpected image version %q", got)
	}
	if got := containerImageVersion("registry.example/openresty@sha256:abc"); got != "sha256:abc" {
		t.Fatalf("unexpected digest version %q", got)
	}
}

func TestParseOpenRestyContainerList(t *testing.T) {
	status, ok := parseOpenRestyContainerList("database\tpostgres:18\tUp 1 hour\nworkmesh-openresty\tworkmesh/openresty-waf:1.25.3.1\tUp 2 hours\n")
	if !ok || !status.IsExist || !status.IsActive || status.Status != "Running" {
		t.Fatalf("container status not detected: %#v", status)
	}
	if status.Version != "1.25.3.1" || status.Binary != "docker://workmesh-openresty" {
		t.Fatalf("unexpected container metadata: %#v", status)
	}
	if _, ok := parseOpenRestyContainerList("database\tpostgres:18\tUp 1 hour\n"); ok {
		t.Fatal("unrelated container must not be detected as OpenResty")
	}
	if _, ok := parseOpenRestyContainerList("1Panel-openresty-ydp8\t1panel/openresty:1.31.1.1-2-4-noble\tUp 1 hour\n"); ok {
		t.Fatal("旧 1Panel OpenResty 不得被 WorkMesh 识别")
	}
}

func TestWebsiteServicePersistsWebsiteAndWAF(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WEBSITE_ROOT", root)
	svc := NewWebsiteService(root)
	website, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "example.com", Alias: "示例站点", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.html", "404.html"} {
		content, readErr := os.ReadFile(filepath.Join(svc.SitePath(website, "app"), name))
		if readErr != nil || len(content) == 0 {
			t.Fatalf("默认站点文件未创建 %s: %v", name, readErr)
		}
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
	if _, err := reloaded.Get(website.ID); err != nil {
		t.Fatalf("关系表重新加载网站失败: %v", err)
	}
}

func TestWebsiteRuntimeIDUsesStringAndStaticClearsReference(t *testing.T) {
	root := t.TempDir()
	svc := NewWebsiteService(root)
	if _, err := svc.db.Exec(`CREATE TABLE IF NOT EXISTS runtime_records (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`INSERT INTO runtime_records(id) VALUES('php74')`); err != nil {
		t.Fatal(err)
	}
	runtimeSite, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "runtime.example", Type: "runtime", RuntimeID: "php74"})
	if err != nil || runtimeSite.RuntimeID != "php74" {
		t.Fatalf("字符串运行时引用创建失败: %#v %v", runtimeSite, err)
	}
	legacy := "999"
	staticSite, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "static.example", Type: "static", RuntimeID: legacy})
	if err != nil || staticSite.RuntimeID != "" {
		t.Fatalf("静态站点未清空运行时引用: %#v %v", staticSite, err)
	}
	updated, err := svc.Update(model.WebsiteUpdateRequest{ID: runtimeSite.ID, Type: "static", RuntimeID: &legacy})
	if err != nil || updated.RuntimeID != "" {
		t.Fatalf("切换静态类型未清空运行时引用: %#v %v", updated, err)
	}
}

// TestWebsiteServiceWritesOldPhysicalSchema verifies an in-place upgrade keeps old
// physical table constraints satisfiable without restoring any legacy blob content.
func TestWebsiteServiceWritesOldPhysicalSchema(t *testing.T) {
	root := t.TempDir()
	store, err := storage.Open(filepath.Join(root, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.DB()
	for _, statement := range []string{
		`CREATE TABLE websites (id INTEGER PRIMARY KEY, primary_domain TEXT NOT NULL UNIQUE, payload BLOB NOT NULL, status TEXT NOT NULL, group_id INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE website_domains (id TEXT PRIMARY KEY, website_id INTEGER NOT NULL REFERENCES websites(id) ON DELETE CASCADE, domain TEXT NOT NULL, port INTEGER NOT NULL DEFAULT 80, ssl INTEGER NOT NULL DEFAULT 0, payload BLOB NOT NULL, UNIQUE(website_id, domain))`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := SetWebsiteDB(db); err != nil {
		t.Fatal(err)
	}
	defer func() {
		websiteDBMu.Lock()
		websiteDB = nil
		websiteDBMu.Unlock()
	}()
	svc := NewWebsiteService(root)
	website, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "upgrade.example"})
	if err != nil {
		t.Fatal(err)
	}
	domain, err := svc.UpsertDomain(model.WebsiteDomain{WebsiteID: website.ID, Domain: "www.upgrade.example"})
	if err != nil {
		t.Fatal(err)
	}
	if domain.ID == "" || domain.ID == "domain-0" {
		t.Fatalf("未返回数据库生成的域名 ID: %#v", domain)
	}
	var websitePayload, domainPayload int
	if err := db.QueryRow(`SELECT length(payload) FROM websites WHERE id=?`, website.ID).Scan(&websitePayload); err != nil || websitePayload != 0 {
		t.Fatalf("网站旧约束列必须为空 blob: length=%d err=%v", websitePayload, err)
	}
	if err := db.QueryRow(`SELECT length(payload) FROM website_domains WHERE id=?`, domain.ID).Scan(&domainPayload); err != nil || domainPayload != 0 {
		t.Fatalf("域名旧约束列必须为空 blob: length=%d err=%v", domainPayload, err)
	}
	if err := svc.DeleteDomain(website.ID, domain.ID); err != nil {
		t.Fatalf("实际域名 ID 删除失败: %v", err)
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
	if err != nil || len(items) != 2 || items[0].Domain != "advanced.example" || items[1].ID != domain.ID {
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

func TestWebsiteServiceBackfillsLegacyPrimaryDomain(t *testing.T) {
	root := t.TempDir()
	svc := NewWebsiteService(root)
	website, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "legacy-domain.example", Protocol: "HTTP"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`DELETE FROM website_domains WHERE website_id=?`, website.ID); err != nil {
		t.Fatal(err)
	}
	reloaded := NewWebsiteService(root)
	items, err := reloaded.ListDomains(website.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Domain != "legacy-domain.example" || items[0].Port != 80 || items[0].ID == "" {
		t.Fatalf("历史网站主域名回填失败: %#v", items)
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
