// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestUpdateOpenRestyRollsBackMemoryWhenPersistenceFails(t *testing.T) {
	root := t.TempDir()
	svc := NewWebsiteService(root)
	original := svc.GetOpenResty()
	svc.db.Close()

	updated := original
	updated.Version = "synthetic-new-version"
	if _, err := svc.UpdateOpenResty(updated); err == nil {
		t.Fatal("SQLite 关闭后 OpenResty 状态更新应失败")
	}
	if got := svc.GetOpenResty(); got.Version != original.Version || got.ConfigContent != original.ConfigContent {
		t.Fatalf("OpenResty 内存状态未回滚: got=%#v want=%#v", got, original)
	}
}

func TestUpdateOpenRestyAppliesRuntimeAndRollsBackOnReloadFailure(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(root, "fake-openresty.sh"))
	logPath := filepath.Join(root, "openresty.log")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> '" + logPath + "'\n" +
		"if [ \"$1\" = \"-v\" ]; then echo 'nginx version: openresty/1.27.1'; exit 0; fi\n" +
		"if [ \"$1\" = \"-t\" ]; then exit 0; fi\n" +
		"if [ \"$1\" = \"-s\" ] && [ \"$2\" = \"reload\" ]; then echo reload failed >&2; exit 1; fi\n" +
		"exit 2\n"
	if err := os.WriteFile(filepath.Join(root, "fake-openresty.sh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	svc := NewWebsiteService(root)
	original := svc.GetOpenResty()
	updated := original
	updated.DefaultHTTPS = !original.DefaultHTTPS

	if _, err := svc.UpdateOpenResty(updated); err == nil || !strings.Contains(err.Error(), "reload") {
		t.Fatalf("reload failure must reject OpenResty update: %v", err)
	}
	if got := svc.GetOpenResty(); got.DefaultHTTPS != original.DefaultHTTPS {
		t.Fatalf("OpenResty state was not rolled back: got=%#v want=%#v", got, original)
	}
	var defaultHTTPS int
	if err := svc.db.QueryRow(`SELECT default_https FROM website_openresty_config WHERE id=1`).Scan(&defaultHTTPS); err != nil {
		t.Fatal(err)
	}
	if (defaultHTTPS != 0) != original.DefaultHTTPS {
		t.Fatalf("OpenResty SQLite state was not rolled back: %d", defaultHTTPS)
	}
	logs, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logs), "-t") || !strings.Contains(string(logs), "-s reload") {
		t.Fatalf("OpenResty update did not execute runtime validation: %s", logs)
	}
}

func TestUpdateOpenRestyFileRollsBackDiskAndMemoryWhenPersistenceFails(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "openresty.conf")
	t.Setenv("WORKMESH_OPENRESTY_CONFIG", configPath)
	svc := NewWebsiteService(root)
	original := "events {}\nhttp {}\n"
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	svc.openresty.ConfigContent = original
	svc.db.Close()

	if err := svc.UpdateOpenRestyFile("events {}\nhttp { server {} }\n", true); err == nil {
		t.Fatal("SQLite 关闭后 OpenResty 文件更新应失败")
	}
	content, err := os.ReadFile(configPath)
	if err != nil || string(content) != original {
		t.Fatalf("OpenResty 配置文件未回滚: err=%v content=%s", err, content)
	}
	if got := svc.GetOpenResty(); got.ConfigContent != original {
		t.Fatalf("OpenResty 内存配置未回滚: %#v", got)
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
	stopped, ok := parseOpenRestyContainerList("workmesh-openresty-waf\tworkmesh/openresty-waf:1.25.3.1\tExited (0) 2 minutes ago\n")
	if !ok || !stopped.IsExist || stopped.IsActive || stopped.Status != "Stopped" || stopped.Binary != "docker://workmesh-openresty-waf" {
		t.Fatalf("已停止的 WorkMesh OpenResty 容器必须可被识别用于启动: %#v", stopped)
	}
}

func TestUpdateOpenRestyScopeInsertsMissingDirectivesInsideHTTPBlock(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "openresty.conf")
	t.Setenv("WORKMESH_OPENRESTY_CONFIG", configPath)
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(root, "missing-openresty"))
	if err := os.WriteFile(configPath, []byte("events {}\nhttp {}\ngzip off;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := NewWebsiteService(root)
	if err := svc.UpdateOpenRestyScope("http-per", map[string]string{"gzip": "on", "client_max_body_size": "64m"}, false); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(content)
	if !strings.Contains(got, "http {\n    client_max_body_size 64m;\n    gzip on;\n}") {
		t.Fatalf("missing directives were not inserted inside http block:\n%s", got)
	}
	if strings.Count(got, "gzip on;") != 1 || strings.Contains(got, "\ngzip off;") {
		t.Fatalf("top-level legacy directive was not removed:\n%s", got)
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

func TestOperateWebsiteLogSynchronizesSiteConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WEBSITE_ROOT", root)
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(root, "missing-openresty"))
	svc := NewWebsiteService(root)
	site, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "logs.example.com", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	configPath := svc.SitePath(site, "site.conf")
	if err := svc.OperateWebsiteLog(site.ID, "access.log", "disable"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(configPath)
	if err != nil || !strings.Contains(string(content), "access_log off;") {
		t.Fatalf("禁用访问日志未同步 site.conf: err=%v content=%s", err, content)
	}
	if err := svc.OperateWebsiteLog(site.ID, "access.log", "enable"); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(configPath)
	if err != nil || !strings.Contains(string(content), "access_log "+svc.websiteLogPath(site, "access.log")+";") {
		t.Fatalf("启用访问日志未恢复 site.conf: err=%v content=%s", err, content)
	}
	reloaded := NewWebsiteService(root)
	loaded, err := reloaded.Get(site.ID)
	if err != nil || !loaded.AccessLog {
		t.Fatalf("访问日志开关未持久化: err=%v site=%#v", err, loaded)
	}
}

func TestBasicConfigUsesCompleteDefaultDocuments(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WEBSITE_ROOT", root)
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(root, "missing-openresty"))
	svc := NewWebsiteService(root)
	site, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "defaults.example.com", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(svc.SitePath(site, "site.conf"), []byte("server {\n    server_name defaults.example.com;\n}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	cfg, err := svc.BasicConfig(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"index.php", "index.html", "index.htm", "default.php", "default.htm", "default.html"}
	got, ok := cfg["defaultDocuments"].([]string)
	if !ok || strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("默认文档不完整: %#v", cfg["defaultDocuments"])
	}
}

func TestWebsiteDirectoryUsesNumericOwnershipAndSupportsNumericUpdates(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("需要 root 才能验证 1000:1000 属主")
	}
	root := t.TempDir()
	t.Setenv("WORKMESH_WEBSITE_ROOT", root)
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(root, "missing-openresty"))
	svc := NewWebsiteService(root)
	site, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "ownership.example.com", Type: "static", User: "1000", Group: "1000"})
	if err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(svc.SitePath(site, "app"), "public", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "index.php"), []byte("<?php echo 'ok';"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateWebsiteDirPermission(site.ID, "1000", "1000"); err != nil {
		t.Fatal(err)
	}
	cfg, err := svc.WebsiteDirConfig(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cfg["msg"] == "ErrPathPermission" {
		info, statErr := os.Stat(svc.SitePath(site, "app"))
		uid, gid, ok := pathOwnership(info)
		if statErr != nil || !ok || uid != "1000" || gid != "1000" {
			t.Skip("当前临时文件系统不支持合成 1000:1000 属主")
		}
	}
	if cfg["user"] != "1000" || cfg["userGroup"] != "1000" || cfg["msg"] != "" {
		t.Fatalf("1000:1000 目录被错误判定: %#v", cfg)
	}
	loaded, err := svc.Get(site.ID)
	if err != nil || loaded.User != "1000" || loaded.Group != "1000" {
		t.Fatalf("用户组保存未回填: %#v err=%v", loaded, err)
	}
	if err := os.Chown(filepath.Join(nested, "index.php"), 0, 0); err != nil {
		t.Fatal(err)
	}
	cfg, err = svc.WebsiteDirConfig(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cfg["msg"] != "ErrPathPermission" {
		t.Fatalf("错误属主未返回 ErrPathPermission: %#v", cfg)
	}
}

func TestPHPFPMOwnerResolutionDistinguishesLocalAndContainer(t *testing.T) {
	root := t.TempDir()
	svc := NewWebsiteService(root)
	if _, err := svc.db.Exec(`CREATE TABLE IF NOT EXISTS runtime_records (id TEXT PRIMARY KEY, type TEXT NOT NULL, payload BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`INSERT INTO runtime_records(id,type,payload) VALUES('php-local','php','{"params":{}}')`); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(root, "www.conf")
	if err := os.WriteFile(conf, []byte("[www]\nuser = root\ngroup = root\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_PHP_FPM_CONFIG", conf)
	if runtimeType, container, fpmUser, fpmGroup := svc.runtimeOwnerMetadata("php-local"); runtimeType != "php" || container || fpmUser != "root" || fpmGroup != "root" {
		t.Fatalf("本地 PHP-FPM 识别错误: type=%q container=%v user=%q group=%q", runtimeType, container, fpmUser, fpmGroup)
	}
	if _, err := svc.db.Exec(`UPDATE runtime_records SET payload=? WHERE id='php-local'`, []byte(`{"container":"php-local","image":"php:8.3"}`)); err != nil {
		t.Fatal(err)
	}
	if _, container, fpmUser, fpmGroup := svc.runtimeOwnerMetadata("php-local"); !container || fpmUser != "" || fpmGroup != "" {
		t.Fatalf("PHP 容器识别错误: container=%v user=%q group=%q", container, fpmUser, fpmGroup)
	}
}

// TestManagedWebsiteIncludesOnlyReferenceGeneratedFiles 防止缺失 include 破坏 nginx -t。
func TestManagedWebsiteIncludesOnlyReferenceGeneratedFiles(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WEBSITE_ROOT", root)
	svc := NewWebsiteService(root)
	site, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "include.example.com", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	content := "server {\n    server_name include.example.com;\n}\n"
	managed := svc.syncManagedWebsiteIncludes(site, content)
	if strings.Contains(managed, "leech;") || strings.Contains(managed, "cors.conf;") || strings.Contains(managed, "realip.conf;") {
		t.Fatalf("不得引用缺失普通文件或目录: %s", managed)
	}
	if strings.Contains(managed, "upstream/*.conf") {
		t.Fatalf("空 upstream 目录不得生成 include: %s", managed)
	}
	if !strings.Contains(managed, "server_name include.example.com") {
		t.Fatalf("应保留用户配置: %s", managed)
	}
	if err := os.WriteFile(filepath.Join(svc.SitePath(site, "upstream"), "managed.conf"), []byte("proxy_pass http://127.0.0.1:8080;\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	managed = svc.syncManagedWebsiteIncludes(site, content)
	if !strings.Contains(managed, "upstream/*.conf;") {
		t.Fatalf("生成 upstream 文件后应使用 glob include: %s", managed)
	}
	if again := svc.syncManagedWebsiteIncludes(site, managed); again != managed {
		t.Fatalf("重复同步必须幂等: %s", again)
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

func TestWebsiteServiceCreatesAllWebsiteTypesWithRuntimeConfig(t *testing.T) {
	root := t.TempDir()
	svc := NewWebsiteService(root)
	static, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "types-static.example", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "types-proxy.example", Type: "proxy", Proxy: "http://127.0.0.1:28080"})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "types-deployment.example", Type: "deployment", AppInstallID: 7, Proxy: "http://127.0.0.1:28081"})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "types-runtime.example", Type: "runtime", RuntimeID: "node-test", Proxy: "http://127.0.0.1:28082", ProxyType: "tcp"})
	if err != nil {
		t.Fatal(err)
	}
	php, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "types-php.example", Type: "runtime", RuntimeID: "php-test", Proxy: "127.0.0.1:28085", ProxyType: "fpm"})
	if err != nil {
		t.Fatal(err)
	}
	subsite, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "types-subsite.example", Type: "subsite", ParentWebsiteID: static.ID})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "types-stream.example", Type: "stream", StreamPorts: "29001", UDP: false})
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name string
		site model.Website
		want string
	}{
		{"proxy", proxy, "proxy_pass http://127.0.0.1:28080;"},
		{"deployment", deployment, "proxy_pass http://127.0.0.1:28081;"},
		{"runtime", runtime, "proxy_pass http://127.0.0.1:28082;"},
		{"php", php, "fastcgi_pass 127.0.0.1:28085;"},
		{"subsite", subsite, "root " + svc.SitePath(subsite, "app") + ";"},
		{"stream", stream, "listen 29001;"},
	}
	for _, check := range checks {
		path := svc.SitePath(check.site, "site.conf")
		if check.name == "stream" {
			path = svc.SitePath(check.site, "stream.conf")
		}
		if check.name == "proxy" {
			path = filepath.Join(svc.SitePath(check.site, "proxy"), "root.conf")
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil || !strings.Contains(string(content), check.want) {
			t.Fatalf("%s 配置不符合预期: err=%v want=%q content=%s", check.name, readErr, check.want, content)
		}
	}
	proxySiteConfig, err := os.ReadFile(svc.SitePath(proxy, "site.conf"))
	if err != nil || !strings.Contains(string(proxySiteConfig), "nginx/proxy/*.conf") {
		t.Fatalf("反向站点 site.conf 未引用代理托管目录: err=%v content=%s", err, proxySiteConfig)
	}
}

func TestNamedWebsiteProxyLifecycleAndLegacyRepair(t *testing.T) {
	root := t.TempDir()
	websiteRoot := filepath.Join(root, "wwwroot")
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_WEBSITE_ROOT", websiteRoot)
	svc := NewWebsiteService(root)
	site, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "named-proxy.example", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateNamedWebsiteProxy(site.ID, "api", "create", map[string]any{"proxyPass": "127.0.0.1:18080", "match": "/api"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateNamedWebsiteProxy(site.ID, "web", "create", map[string]any{"proxyPass": "https://upstream.example"}); err != nil {
		t.Fatal(err)
	}
	items, err := svc.ListWebsiteProxies(site.ID)
	if err != nil || len(items) != 2 {
		t.Fatalf("命名代理列表错误: err=%v items=%#v", err, items)
	}
	if err := svc.UpdateNamedWebsiteProxyStatus(site.ID, "api", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(svc.SitePath(site, "proxy"), "api.bak")); err != nil {
		t.Fatalf("禁用代理未生成 bak: %v", err)
	}
	if err := svc.UpdateNamedWebsiteProxyStatus(site.ID, "api", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(svc.SitePath(site, "proxy"), "api.conf")); err != nil {
		t.Fatalf("启用代理未恢复 conf: %v", err)
	}
	if err := svc.DeleteWebsiteProxy(site.ID, "api"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(svc.SitePath(site, "proxy"), "api.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("删除代理后 conf 仍存在: %v", err)
	}

	legacy, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "legacy-proxy.example", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	for i := range svc.websites {
		if svc.websites[i].ID == legacy.ID {
			svc.websites[i].Type = "proxy"
			svc.websites[i].Proxy = "127.0.0.1:19090"
		}
	}
	svc.mu.Unlock()
	if err := os.RemoveAll(svc.SitePath(legacy, "proxy")); err != nil {
		t.Fatal(err)
	}
	items, err = svc.ListWebsiteProxies(legacy.ID)
	if err != nil || len(items) != 1 || items[0]["name"] != "root" || !strings.Contains(fmt.Sprint(items[0]["proxyPass"]), "127.0.0.1:19090") {
		t.Fatalf("历史反向站点未自动补齐: err=%v items=%#v", err, items)
	}
}

func TestSubsiteUsesParentRunDirectoryAndPersistsSelection(t *testing.T) {
	root := t.TempDir()
	svc := NewWebsiteService(root)
	parent, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "parent-subsite.example", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	parentDir := filepath.Join(svc.SitePath(parent, "app"), "public")
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parentDir, "index.html"), []byte("parent"), 0o644); err != nil {
		t.Fatal(err)
	}
	child, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "child-subsite.example", Type: "subsite", ParentWebsiteID: parent.ID, SiteDir: "/public"})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(svc.SitePath(child, "site.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "root "+parentDir+";") {
		t.Fatalf("子网站 root 未指向父站运行目录: %s", content)
	}
	loaded, err := svc.Get(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Root != parentDir {
		t.Fatalf("子网站 root 回填错误: got=%q want=%q", loaded.Root, parentDir)
	}
	if _, err := svc.UpdateWebsiteDir(child.ID, "/"); err != nil {
		t.Fatal(err)
	}
	updated, err := svc.Get(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Root != svc.SitePath(parent, "app") {
		t.Fatalf("子网站切换根目录失败: got=%q", updated.Root)
	}
	if err := svc.Delete(parent.ID); err == nil || !strings.Contains(err.Error(), "父网站仍有子网站") {
		t.Fatalf("存在子网站时删除父站应被拒绝: %v", err)
	}
}

func TestPHPRuntimeSubsiteUsesParentFastCGI(t *testing.T) {
	root := t.TempDir()
	svc := NewWebsiteService(root)
	if _, err := svc.db.Exec(`CREATE TABLE IF NOT EXISTS runtime_records (id TEXT PRIMARY KEY, type TEXT NOT NULL, payload BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`INSERT INTO runtime_records(id,type,payload) VALUES('php-parent','php','{}')`); err != nil {
		t.Fatal(err)
	}
	parent, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "php-parent.example", Type: "runtime", RuntimeID: "php-parent", Proxy: "127.0.0.1:19000", ProxyType: "tcp"})
	if err != nil {
		t.Fatal(err)
	}
	childDir := filepath.Join(svc.SitePath(parent, "app"), "child")
	if err := os.MkdirAll(childDir, 0o755); err != nil {
		t.Fatal(err)
	}
	child, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "php-child.example", Type: "subsite", ParentWebsiteID: parent.ID, SiteDir: "/child"})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(svc.SitePath(child, "site.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "fastcgi_pass 127.0.0.1:19000;") {
		t.Fatalf("PHP 父运行时子网站未生成 FastCGI 配置: %s", content)
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

func TestWebsiteStreamUpdateRollsBackFileAndMemoryWhenPersistenceFails(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WEBSITE_ROOT", filepath.Join(root, "wwwroot"))
	svc := NewWebsiteService(root)
	site, err := svc.Create(model.WebsiteCreateRequest{
		PrimaryDomain: "stream-rollback.example",
		Type:          "stream",
		StreamPorts:   "29001",
		Servers: []map[string]any{{
			"server": "127.0.0.1:30001",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := svc.SitePath(site, "stream.conf")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	svc.db.Close()

	if _, err := svc.UpdateStream(site.ID, "29002", false, "least_conn", []map[string]any{{"server": "127.0.0.1:30002"}}); err == nil {
		t.Fatal("SQLite 关闭后更新 stream 应失败")
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != string(original) {
		t.Fatalf("持久化失败后 stream.conf 未恢复: err=%v content=%s", err, current)
	}
	restored, err := svc.Get(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.StreamPorts != site.StreamPorts || restored.Servers[0].Server != site.Servers[0].Server {
		t.Fatalf("持久化失败后内存 stream 状态未恢复: %#v", restored)
	}
}

func TestWebsiteConfigUpdateRollsBackFileAndMemoryWhenPersistenceFails(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WEBSITE_ROOT", filepath.Join(root, "wwwroot"))
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(root, "missing-openresty"))
	svc := NewWebsiteService(root)
	site, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "config-rollback.example", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	path := svc.SitePath(site, "site.conf")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	svc.db.Close()

	if _, err := svc.UpdateConfig(site.ID, "nginx", map[string]any{
		"content": "server {\n    listen 80;\n    server_name changed.example;\n}\n",
	}); err == nil {
		t.Fatal("SQLite 关闭后网站配置更新应失败")
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != string(original) {
		t.Fatalf("配置持久化失败后 site.conf 未恢复: err=%v content=%s", err, current)
	}
	config, err := svc.GetConfig(site.ID, "nginx")
	if err != nil {
		t.Fatal(err)
	}
	if len(config) != 0 {
		t.Fatalf("配置持久化失败后不应保留内存配置: %#v", config)
	}
}

func TestWebsiteUpdateRollsBackRenameWhenPersistenceFails(t *testing.T) {
	root := t.TempDir()
	siteRoot := filepath.Join(root, "wwwroot")
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_WEBSITE_ROOT", siteRoot)
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(root, "missing-openresty"))
	svc := NewWebsiteService(root)
	site, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "rename-before.example", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	oldPath := svc.SitePath(site, "root")
	oldConfig, err := os.ReadFile(svc.SitePath(site, "site.conf"))
	if err != nil {
		t.Fatal(err)
	}
	svc.db.Close()

	updated, err := svc.Update(model.WebsiteUpdateRequest{ID: site.ID, PrimaryDomain: "rename-after.example"})
	if err == nil {
		t.Fatal("SQLite 关闭后域名更新应失败")
	}
	if updated.ID != 0 {
		t.Fatalf("失败更新不应返回网站: %#v", updated)
	}
	current, err := svc.Get(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.PrimaryDomain != site.PrimaryDomain || current.SiteDir != site.SiteDir {
		t.Fatalf("失败更新后网站内存未恢复: %#v", current)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("失败更新后旧站点目录未恢复: %v", err)
	}
	if _, err := os.Stat(filepath.Join(siteRoot, "rename-after.example")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("失败更新后新站点目录不应残留: %v", err)
	}
	restoredConfig, err := os.ReadFile(filepath.Join(oldPath, "nginx", "site.conf"))
	if err != nil || string(restoredConfig) != string(oldConfig) {
		t.Fatalf("失败更新后 site.conf 未恢复: err=%v content=%s", err, restoredConfig)
	}
}

func TestWebsiteUpdateRollsBackWAFAndWebsiteWhenRuntimePersistenceFails(t *testing.T) {
	root := t.TempDir()
	siteRoot := filepath.Join(root, "wwwroot")
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_WEBSITE_ROOT", siteRoot)
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(root, "missing-openresty"))
	svc := NewWebsiteService(root)
	site, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "waf-update-rollback.example", Alias: "old-alias", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	originalWAF, err := os.ReadFile(svc.SitePath(site, "waf") + "/config.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_WAF_ENFORCE", "1")

	if _, err := svc.Update(model.WebsiteUpdateRequest{ID: site.ID, Alias: "new-alias"}); err == nil {
		t.Fatal("OpenResty/WAF 生效失败时网站更新应失败")
	}
	current, err := svc.Get(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Alias != "old-alias" {
		t.Fatalf("WAF 生效失败后网站别名未恢复: %#v", current)
	}
	wafSite := svc.ListWAFSites()
	if len(wafSite) != 1 || wafSite[0].Alias != "old-alias" {
		t.Fatalf("WAF 内存状态未恢复: %#v", wafSite)
	}
	restoredWAF, err := os.ReadFile(svc.SitePath(site, "waf") + "/config.json")
	if err != nil || string(restoredWAF) != string(originalWAF) {
		t.Fatalf("WAF 文件未恢复: err=%v content=%s", err, restoredWAF)
	}
	reloaded := NewWebsiteService(root)
	loaded, err := reloaded.Get(site.ID)
	if err != nil || loaded.Alias != "old-alias" {
		t.Fatalf("SQLite/重载状态未恢复: err=%v site=%#v", err, loaded)
	}
}

// TestWebsiteServiceRejectsPrimaryDomainMatchingAttachedDomain 防止不同站点生成重复 server_name。
func TestWebsiteServiceRejectsPrimaryDomainMatchingAttachedDomain(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WEBSITE_ROOT", filepath.Join(root, "wwwroot"))
	svc := NewWebsiteService(root)
	first, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "first.example"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertDomain(model.WebsiteDomain{WebsiteID: first.ID, Domain: "shared.example"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "shared.example"}); err == nil {
		t.Fatal("主域名与其他站点附加域名冲突时应拒绝创建")
	}
	var count int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM websites`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("冲突创建不应写入 SQLite websites: count=%d", count)
	}
	if _, err := svc.Get(2); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("冲突创建不应留下网站记录: %v", err)
	}
}

// TestWebsiteCreateRollsBackAfterOpenRestyValidationFailure 确认外部配置检查失败时不残留文件或 SQLite 行。
func TestWebsiteCreateRollsBackAfterOpenRestyValidationFailure(t *testing.T) {
	root := t.TempDir()
	siteRoot := filepath.Join(root, "wwwroot")
	countPath := filepath.Join(root, "openresty-check-count")
	binPath := filepath.Join(root, "fake-openresty.sh")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"-v\" ]; then echo 'nginx version: openresty/1.25.3.1'; exit 0; fi\n" +
		"if [ \"$1\" = \"-t\" ]; then\n" +
		"  if [ ! -f '" + countPath + "' ]; then touch '" + countPath + "'; exit 0; fi\n" +
		"  echo 'synthetic nginx config failure' >&2; exit 1\n" +
		"fi\nexit 2\n"
	if err := os.WriteFile(binPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_WEBSITE_ROOT", siteRoot)
	t.Setenv("WORKMESH_OPENRESTY_BIN", binPath)
	svc := NewWebsiteService(root)
	if _, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "rollback.example", Type: "static"}); err == nil || !strings.Contains(err.Error(), "OpenResty 配置检查失败") {
		t.Fatalf("应返回 OpenResty 配置检查错误，实际: %v", err)
	}
	if _, err := svc.Get(1); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("失败创建不应保留内存记录: %v", err)
	}
	var count int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM websites`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("失败创建不应写入 SQLite websites: count=%d", count)
	}
	if _, err := os.Stat(filepath.Join(siteRoot, "rollback.example")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("失败创建不应残留站点目录: %v", err)
	}
}

func TestWebsiteCreateRollsBackAfterWAFPersistenceFailure(t *testing.T) {
	root := t.TempDir()
	siteRoot := filepath.Join(root, "wwwroot")
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_WEBSITE_ROOT", siteRoot)
	t.Setenv("WORKMESH_WAF_ENFORCE", "1")
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(root, "missing-openresty"))
	svc := NewWebsiteService(root)

	if _, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "waf-create-rollback.example", Type: "static"}); err == nil {
		t.Fatal("WAF 持久化依赖不可用时创建应失败")
	}
	if _, err := svc.Get(1); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("WAF 持久化失败后不应保留内存网站: %v", err)
	}
	var count int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM websites`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("WAF 持久化失败后不应保留 SQLite 网站: %d", count)
	}
	if _, err := os.Stat(filepath.Join(siteRoot, "waf-create-rollback.example")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("WAF 持久化失败后不应残留站点目录: %v", err)
	}
}

// TestWebsiteManagedConfigRollsBackAfterOpenRestyValidationFailure 验证反代和重定向配置检查失败时恢复文件与 SQLite。
func TestWebsiteManagedConfigRollsBackAfterOpenRestyValidationFailure(t *testing.T) {
	root := t.TempDir()
	siteRoot := filepath.Join(root, "wwwroot")
	countPath := filepath.Join(root, "openresty-check-count")
	binPath := filepath.Join(root, "fake-openresty.sh")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"-v\" ]; then echo 'nginx version: openresty/1.25.3.1'; exit 0; fi\n" +
		"if [ \"$1\" = \"-t\" ]; then\n" +
		"  if [ ! -f '" + countPath + "' ]; then touch '" + countPath + "'; exit 0; fi\n" +
		"  echo 'synthetic nginx config failure' >&2; exit 1\n" +
		"fi\nexit 2\n"
	if err := os.WriteFile(binPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_WEBSITE_ROOT", siteRoot)
	// 创建站点时不启用假 OpenResty，确保测试只覆盖配置修改阶段。
	svc := NewWebsiteService(root)
	site, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "managed-rollback.example", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(svc.SitePath(site, "site.conf"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_OPENRESTY_BIN", binPath)
	for _, item := range []struct {
		name  string
		value map[string]any
		dir   string
	}{
		{name: "proxy", value: map[string]any{"enabled": true, "proxyPass": "https://upstream.example"}, dir: "proxy"},
		{name: "redirect", value: map[string]any{"enabled": true, "target": "https://target.example", "code": "302"}, dir: "redirect"},
	} {
		_ = os.Remove(countPath)
		if _, err := svc.UpdateConfig(site.ID, item.name, item.value); err == nil || !strings.Contains(err.Error(), "OpenResty 配置检查失败") {
			t.Fatalf("%s 应因 OpenResty 检查失败，实际: %v", item.name, err)
		}
		current, readErr := os.ReadFile(svc.SitePath(site, "site.conf"))
		if readErr != nil || string(current) != string(original) {
			t.Fatalf("%s 失败后 site.conf 未恢复: err=%v", item.name, readErr)
		}
		matches, globErr := filepath.Glob(filepath.Join(svc.SitePath(site, item.dir), "*.conf"))
		if globErr != nil || len(matches) != 0 {
			t.Fatalf("%s 失败后托管配置不应残留: %v", item.name, matches)
		}
		var settings int
		if err := svc.db.QueryRow(`SELECT COUNT(*) FROM website_settings WHERE website_id=? AND config_type=?`, site.ID, item.name).Scan(&settings); err != nil {
			t.Fatal(err)
		}
		if settings != 0 {
			t.Fatalf("%s 失败后不应写入 SQLite website_settings: %d", item.name, settings)
		}
	}
}

// TestWebsiteDeleteRejectsInvalidOpenRestyConfig 保证删除前配置检查失败不会改动 SQLite 或站点文件。
func TestWebsiteDeleteRejectsInvalidOpenRestyConfig(t *testing.T) {
	root := t.TempDir()
	siteRoot := filepath.Join(root, "wwwroot")
	binPath := filepath.Join(root, "fake-openresty-delete.sh")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"-v\" ]; then echo 'nginx version: openresty/1.25.3.1'; exit 0; fi\n" +
		"echo 'synthetic nginx config failure' >&2; exit 1\n"
	if err := os.WriteFile(binPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_WEBSITE_ROOT", siteRoot)
	svc := NewWebsiteService(root)
	site, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "delete-rollback.example", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_OPENRESTY_BIN", binPath)
	if err := svc.Delete(site.ID); err == nil || !strings.Contains(err.Error(), "OpenResty 配置检查失败") {
		t.Fatalf("配置检查失败时删除应被拒绝，实际: %v", err)
	}
	if _, err := svc.Get(site.ID); err != nil {
		t.Fatalf("删除失败后网站内存记录丢失: %v", err)
	}
	var count int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM websites WHERE id=?`, site.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("删除失败后 SQLite websites 记录丢失: %d", count)
	}
	if _, err := os.Stat(svc.SitePath(site, "site.conf")); err != nil {
		t.Fatalf("删除失败后站点配置文件丢失: %v", err)
	}
}

func TestWebsiteDeleteRollsBackDirectoryWhenPersistenceFails(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_WEBSITE_ROOT", filepath.Join(root, "wwwroot"))
	svc := NewWebsiteService(root)
	site, err := svc.Create(model.WebsiteCreateRequest{PrimaryDomain: "delete-persist-rollback.example", Type: "static"})
	if err != nil {
		t.Fatal(err)
	}
	siteRoot := svc.SitePath(site, "root")
	svc.db.Close()

	if err := svc.Delete(site.ID); err == nil {
		t.Fatal("SQLite 关闭后删除网站应失败")
	}
	if _, err := svc.Get(site.ID); err != nil {
		t.Fatalf("删除持久化失败后网站内存记录丢失: %v", err)
	}
	if _, err := os.Stat(siteRoot); err != nil {
		t.Fatalf("删除持久化失败后站点目录未恢复: %v", err)
	}
}
