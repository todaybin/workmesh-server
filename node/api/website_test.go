// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/node/model"
)

func TestWebsiteWAFRoutesCRUD(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	RegisterSSLRoutes(mux)
	create := httptest.NewRequest(http.MethodPost, "/api/v2/websites", bytes.NewBufferString(`{"primaryDomain":"example.com","alias":"站点"}`))
	create.Header.Set("Content-Type", "application/json")
	record := httptest.NewRecorder()
	mux.ServeHTTP(record, create)
	if record.Code != http.StatusOK {
		t.Fatalf("创建网站状态码: %d %s", record.Code, record.Body.String())
	}
	var envelope struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(record.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.ID == 0 {
		t.Fatal("创建响应缺少 ID")
	}
	rulesBody := bytes.NewBufferString(`{"websiteID":1,"name":"阻止管理路径","location":"uri","operator":"contains","value":"/admin","action":"block","enabled":true}`)
	rules := httptest.NewRequest(http.MethodPost, "/api/v2/websites/waf/rules", rulesBody)
	rules.Header.Set("Content-Type", "application/json")
	rulesRecord := httptest.NewRecorder()
	mux.ServeHTTP(rulesRecord, rules)
	if rulesRecord.Code != http.StatusOK {
		t.Fatalf("新增规则状态码: %d %s", rulesRecord.Code, rulesRecord.Body.String())
	}
	get := httptest.NewRequest(http.MethodGet, "/api/v2/websites/waf/sites/1/rules", nil)
	getRecord := httptest.NewRecorder()
	mux.ServeHTTP(getRecord, get)
	if getRecord.Code != http.StatusOK || !bytes.Contains(getRecord.Body.Bytes(), []byte("阻止管理路径")) {
		t.Fatalf("规则查询失败: %d %s", getRecord.Code, getRecord.Body.String())
	}
}

// TestZNMPStaticWebsiteAllInterfaces 验证目标静态站点的完整本地生命周期和主要网站接口。
func TestZNMPStaticWebsiteAllInterfaces(t *testing.T) {
	dataRoot := t.TempDir()
	siteRoot := filepath.Join(dataRoot, "wwwroot")
	t.Setenv("WORKMESH_DATA_DIR", dataRoot)
	t.Setenv("WORKMESH_WEBSITE_ROOT", siteRoot)
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(dataRoot, "missing-openresty"))
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	RegisterSSLRoutes(mux)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}
	expectOK := func(method, path, body string) *httptest.ResponseRecorder {
		res := call(method, path, body)
		if res.Code != http.StatusOK {
			t.Fatalf("%s %s 返回 %d: %s", method, path, res.Code, res.Body.String())
		}
		return res
	}

	created := expectOK(http.MethodPost, "/api/v2/websites", `{"primaryDomain":"znmp.sopvip.com","type":"static","alias":"ZNMP 静态站点"}`)
	var createdEnvelope struct {
		Data struct {
			ID      uint   `json:"id"`
			SiteDir string `json:"siteDir"`
			Root    string `json:"root"`
			User    string `json:"user"`
			Group   string `json:"group"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdEnvelope); err != nil {
		t.Fatal(err)
	}
	if createdEnvelope.Data.ID != 1 || createdEnvelope.Data.SiteDir != filepath.Join(siteRoot, "znmp.sopvip.com") || createdEnvelope.Data.Root != filepath.Join(siteRoot, "znmp.sopvip.com", "app") {
		t.Fatalf("ZNMP 站点目录不符合约定: %#v", createdEnvelope.Data)
	}
	if createdEnvelope.Data.User != "www" || createdEnvelope.Data.Group != "www" {
		t.Fatalf("默认运行用户/组错误: %#v", createdEnvelope.Data)
	}

	base := filepath.Join(siteRoot, "znmp.sopvip.com")
	for _, rel := range []string{"app", "nginx", "nginx/rewrite", "nginx/proxy", "nginx/redirect", "nginx/auth_basic", "nginx/path_auth", "nginx/upstream", "waf", "logs", "ssl", ".workmesh/runtime", ".workmesh/cache", "config/basic"} {
		if info, err := os.Stat(filepath.Join(base, rel)); err != nil || !info.IsDir() {
			t.Fatalf("缺少站点目录 %s: %v", rel, err)
		}
	}
	for _, rel := range []string{"nginx/site.conf", "nginx/stream.conf"} {
		if info, err := os.Stat(filepath.Join(base, rel)); err != nil || info.IsDir() {
			t.Fatalf("缺少站点配置文件 %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "waf", "site.json")); err != nil {
		t.Fatalf("创建时未生成独立 WAF 配置: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(base, "app", "subdir", "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	dirs := expectOK(http.MethodPost, "/api/v2/websites/dir", `{"id":1}`)
	for _, item := range []string{`"/"`, `"/subdir"`, `"/subdir/nested"`} {
		if !bytes.Contains(dirs.Body.Bytes(), []byte(item)) {
			t.Fatalf("目录扫描缺少 %s: %s", item, dirs.Body.String())
		}
	}
	if !bytes.Contains(dirs.Body.Bytes(), []byte(`"user":"www"`)) || !bytes.Contains(dirs.Body.Bytes(), []byte(`"userGroup":"www"`)) {
		t.Fatalf("目录默认用户/组错误: %s", dirs.Body.String())
	}
	expectOK(http.MethodPost, "/api/v2/websites/dir/update", `{"id":1,"siteDir":"/subdir"}`)
	nginxContent, err := os.ReadFile(filepath.Join(base, "nginx/site.conf"))
	if err != nil || !bytes.Contains(nginxContent, []byte(filepath.Join(base, "app", "subdir"))) {
		t.Fatalf("目录更新未同步 nginx root: %v %s", err, nginxContent)
	}
	expectOK(http.MethodPost, "/api/v2/websites/dir/permission", `{"id":1,"user":"root","group":"root"}`)
	if res := call(http.MethodPost, "/api/v2/websites/dir/update", `{"id":1,"siteDir":"../../escape"}`); res.Code != http.StatusBadRequest {
		t.Fatalf("越界目录应拒绝，实际 %d: %s", res.Code, res.Body.String())
	}

	// CRUD、配置、默认文档和 rewrite 接口。
	expectOK(http.MethodGet, "/api/v2/websites", "")
	expectOK(http.MethodGet, "/api/v2/websites/list", "")
	expectOK(http.MethodGet, "/api/v2/websites/1", "")
	expectOK(http.MethodPost, "/api/v2/websites/search", `{"domain":"znmp.sopvip.com","page":1,"pageSize":10}`)
	expectOK(http.MethodPost, "/api/v2/websites/options", `{}`)
	expectOK(http.MethodPost, "/api/v2/websites/check", `{}`)
	expectOK(http.MethodGet, "/api/v2/websites/monitor/config/global", "")
	expectOK(http.MethodPost, "/api/v2/websites/monitor/config/global", `{"enabled":true}`)
	expectOK(http.MethodPost, "/api/v2/websites/monitor/config/site", `{"websiteID":1}`)
	expectOK(http.MethodPost, "/api/v2/websites/monitor/config/site/update", `{"websiteID":1,"enabled":true}`)
	expectOK(http.MethodPost, "/api/v2/websites/config", `{"websiteID":1,"scope":"index"}`)
	expectOK(http.MethodPost, "/api/v2/websites/config", `{"websiteID":1,"scope":"index","operate":"disable","params":{"index":"index.php"}}`)
	expectOK(http.MethodPost, "/api/v2/websites/nginx/update", `{"websiteID":1,"content":"server { root /tmp/znmp; }"}`)
	expectOK(http.MethodGet, "/api/v2/websites/1/config/basic", "")
	expectOK(http.MethodGet, "/api/v2/websites/1/config/nginx", "")
	expectOK(http.MethodPost, "/api/v2/websites/rewrite/update", `{"websiteID":1,"name":"wordpress","content":"location / { try_files $uri /index.php?$args; }"}`)
	for _, name := range []string{"current", "default", "wordpress", "thinkphp"} {
		expectOK(http.MethodPost, "/api/v2/websites/rewrite", `{"websiteID":1,"name":"`+name+`"}`)
	}
	expectOK(http.MethodPost, "/api/v2/websites/rewrite/custom", `{"websiteID":1,"content":"location /custom { return 200; }"}`)
	expectOK(http.MethodGet, "/api/v2/websites/rewrite/custom?websiteID=1", "")
	expectOK(http.MethodPost, "/api/v2/websites/cors/update", `{"websiteID":1,"config":{"enabled":true}}`)
	expectOK(http.MethodGet, "/api/v2/websites/cors/1", "")
	expectOK(http.MethodPost, "/api/v2/websites/lbs/create", `{"websiteID":1,"config":{"upstreams":[]}}`)
	expectOK(http.MethodGet, "/api/v2/websites/1/lbs", "")
	expectOK(http.MethodPost, "/api/v2/websites/redirect/update", `{"websiteID":1,"config":{"enabled":false}}`)
	expectOK(http.MethodPost, "/api/v2/websites/leech/update", `{"websiteID":1,"config":{"enabled":false}}`)
	expectOK(http.MethodPost, "/api/v2/websites/stream/update", `{"websiteID":1,"config":{"ports":[]}}`)
	expectOK(http.MethodPost, "/api/v2/websites/default/server", `{"websiteID":1,"enabled":true}`)
	expectOK(http.MethodPost, "/api/v2/websites/default/html/update", `{"websiteID":1,"content":"<html>znmp</html>"}`)
	expectOK(http.MethodGet, "/api/v2/websites/default/html/index", "")
	expectOK(http.MethodPost, "/api/v2/websites/proxy/config", `{"websiteID":1,"config":{"proxy":"http://127.0.0.1:8080"}}`)
	expectOK(http.MethodGet, "/api/v2/websites/proxy/config/1", "")
	expectOK(http.MethodPost, "/api/v2/websites/proxy/clear", `{"websiteID":1}`)
	expectOK(http.MethodPost, "/api/v2/websites/realip/config", `{"websiteID":1,"config":{"enabled":false}}`)
	expectOK(http.MethodGet, "/api/v2/websites/realip/config/1", "")

	// 域名、HTTPS、日志和跨站访问。
	domainRes := expectOK(http.MethodPost, "/api/v2/websites/domains", `{"websiteID":1,"domain":"www.znmp.sopvip.com","port":80}`)
	var domainEnvelope struct {
		Data model.WebsiteDomain `json:"data"`
	}
	if err := json.Unmarshal(domainRes.Body.Bytes(), &domainEnvelope); err != nil || domainEnvelope.Data.ID == "" {
		t.Fatalf("附加域名响应无 ID: %s", domainRes.Body.String())
	}
	expectOK(http.MethodGet, "/api/v2/websites/domains/1", "")
	expectOK(http.MethodPost, "/api/v2/websites/domains/update", `{"websiteID":1,"id":"`+domainEnvelope.Data.ID+`","domain":"www.znmp.sopvip.com","port":8080}`)
	expectOK(http.MethodPost, "/api/v2/websites/1/https", `{"enabled":true}`)
	expectOK(http.MethodGet, "/api/v2/websites/1/https", "")
	if err := os.WriteFile(filepath.Join(base, "logs/access.log"), []byte("GET /znmp 200\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	expectOK(http.MethodPost, "/api/v2/websites/log/search", `{"id":1,"logType":"access.log","page":1,"pageSize":10}`)
	expectOK(http.MethodPost, "/api/v2/websites/log/operate", `{"id":1,"logType":"access.log","operate":"enable"}`)
	expectOK(http.MethodPost, "/api/v2/websites/crosssite", `{"id":1,"operate":"enable"}`)
	expectOK(http.MethodPost, "/api/v2/websites/crosssite", `{"id":1,"operate":"disable"}`)
	expectOK(http.MethodPost, "/api/v2/websites/domains/del", `{"websiteID":1,"id":"`+domainEnvelope.Data.ID+`"}`)

	// 旧网站扩展接口也在同一条 ZNMP 站点生命周期中逐一冒烟。
	expectOK(http.MethodGet, "/api/v2/websites/databases", "")
	expectOK(http.MethodPost, "/api/v2/websites/auths", `{"id":"znmp-auth","websiteID":1}`)
	expectOK(http.MethodPost, "/api/v2/websites/auths/path", `{"id":"znmp-path-auth","websiteID":1}`)
	expectOK(http.MethodPost, "/api/v2/websites/auths/path/update", `{"id":"znmp-path-auth","websiteID":1}`)
	expectOK(http.MethodPost, "/api/v2/websites/auths/update", `{"id":"znmp-auth","websiteID":1}`)
	expectOK(http.MethodPost, "/api/v2/websites/batch/group", `{"ids":[1],"groupID":1}`)
	expectOK(http.MethodPost, "/api/v2/websites/group/change", `{"ids":[1],"groupID":2}`)
	expectOK(http.MethodPost, "/api/v2/websites/batch/operate", `{"ids":[1],"operate":"stop"}`)
	expectOK(http.MethodPost, "/api/v2/websites/batch/operate", `{"ids":[1],"operate":"start"}`)
	expectOK(http.MethodPost, "/api/v2/websites/batch/ssl", `{"ids":[1],"enabled":false}`)
	expectOK(http.MethodPost, "/api/v2/websites/php/version", `{"id":1,"version":"8.3"}`)
	expectOK(http.MethodPost, "/api/v2/websites/proxies", `{"id":"znmp-proxy","websiteID":1}`)
	expectOK(http.MethodPost, "/api/v2/websites/proxies/update", `{"id":"znmp-proxy","websiteID":1}`)
	expectOK(http.MethodPost, "/api/v2/websites/proxies/status", `{"id":"znmp-proxy","websiteID":1}`)
	expectOK(http.MethodPost, "/api/v2/websites/proxies/file", `{"id":"znmp-proxy","websiteID":1}`)
	expectOK(http.MethodPost, "/api/v2/websites/proxies/delete", `{"id":"znmp-proxy","websiteID":1}`)
	composerPath := filepath.Join(base, "app")
	if err := os.WriteFile(filepath.Join(composerPath, "composer.json"), []byte(`{"name":"znmp/test"}`), 0o640); err != nil {
		t.Fatal(err)
	}
	expectOK(http.MethodPost, "/api/v2/websites/exec/composer", `{"path":"`+composerPath+`"}`)
	for _, path := range []string{"/api/v2/websites/monitor/logs/clear", "/api/v2/websites/monitor/logs/detail", "/api/v2/websites/monitor/logs/search", "/api/v2/websites/monitor/logs/stat", "/api/v2/websites/monitor/qps", "/api/v2/websites/monitor/rank", "/api/v2/websites/monitor/stat", "/api/v2/websites/monitor/trend", "/api/v2/websites/monitor/visitors", "/api/v2/websites/monitor/visitors/loc", "/api/v2/websites/monitor/websites"} {
		expectOK(http.MethodPost, path, `{"websiteID":1,"page":1,"pageSize":10}`)
	}
	template := expectOK(http.MethodPost, "/api/v2/websites/templates", `{"name":"znmp-template","content":"index"}`)
	var templateEnvelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(template.Body.Bytes(), &templateEnvelope); err != nil {
		t.Fatal(err)
	}
	templateID := templateEnvelope.Data["id"]
	expectOK(http.MethodPost, "/api/v2/websites/templates/search", `{"page":1,"pageSize":10}`)
	expectOK(http.MethodPost, "/api/v2/websites/templates/get", `{"id":"`+strings.TrimSpace(fmt.Sprint(templateID))+`"}`)
	expectOK(http.MethodPost, "/api/v2/websites/templates/update", `{"id":"`+strings.TrimSpace(fmt.Sprint(templateID))+`","name":"znmp-template-updated"}`)
	expectOK(http.MethodPost, "/api/v2/websites/templates/preview", `{"content":"znmp"}`)
	expectOK(http.MethodPost, "/api/v2/websites/templates/upload", `{"name":"znmp.zip"}`)
	output := expectOK(http.MethodPost, "/api/v2/websites/templates/outputs", `{"templateID":"`+strings.TrimSpace(fmt.Sprint(templateID))+`"}`)
	var outputEnvelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(output.Body.Bytes(), &outputEnvelope); err != nil {
		t.Fatal(err)
	}
	outputID := strings.TrimSpace(fmt.Sprint(outputEnvelope.Data["id"]))
	expectOK(http.MethodPost, "/api/v2/websites/templates/outputs/search", `{"page":1,"pageSize":10}`)
	expectOK(http.MethodPost, "/api/v2/websites/templates/outputs/get", `{"id":"`+outputID+`"}`)
	expectOK(http.MethodPost, "/api/v2/websites/templates/outputs/del", `{"id":"`+outputID+`"}`)
	expectOK(http.MethodPost, "/api/v2/websites/templates/del", `{"id":"`+strings.TrimSpace(fmt.Sprint(templateID))+`"}`)
	for _, path := range []string{"/api/v2/websites/waf/attack/stat", "/api/v2/websites/waf/block/search", "/api/v2/websites/waf/log/search", "/api/v2/websites/waf/relation/stat"} {
		expectOK(http.MethodPost, path, `{"websiteID":1,"page":1,"pageSize":10}`)
	}

	// 网站 WAF 接口和规则必须指向同一域名站点。
	expectOK(http.MethodGet, "/api/v2/websites/waf/status", "")
	expectOK(http.MethodGet, "/api/v2/websites/waf/standard-rules", "")
	expectOK(http.MethodGet, "/api/v2/websites/waf/sites", "")
	expectOK(http.MethodPost, "/api/v2/websites/waf/sites", `{"websiteID":1,"enabled":true,"mode":"block"}`)
	expectOK(http.MethodPost, "/api/v2/websites/waf/rules", `{"websiteID":1,"id":"znmp-rule","name":"ZNMP 管理路径","location":"uri","operator":"contains","value":"/admin","action":"block","enabled":true}`)
	if rules := expectOK(http.MethodGet, "/api/v2/websites/waf/sites/1/rules", ""); !bytes.Contains(rules.Body.Bytes(), []byte("ZNMP 管理路径")) {
		t.Fatalf("ZNMP WAF 规则未返回: %s", rules.Body.String())
	}
	expectOK(http.MethodPost, "/api/v2/websites/waf/test", `{"websiteID":1,"uri":"/admin"}`)
	expectOK(http.MethodPost, "/api/v2/websites/waf/rules/delete", `{"websiteID":1,"id":"znmp-rule"}`)
	expectOK(http.MethodPost, "/api/v2/websites/waf/global", `{"enabled":true,"standardRules":true,"mode":"observe","paranoiaLevel":1,"inboundThreshold":5,"requestBodyLimit":1048576}`)
	expectOK(http.MethodGet, "/api/v2/websites/waf/global", "")
	expectOK(http.MethodPost, "/api/v2/websites/waf/access-lists", `{"whitelist":["127.0.0.1"],"blacklist":[]}`)
	expectOK(http.MethodGet, "/api/v2/websites/waf/access-lists", "")

	// SSL 资源接口：验证证书元数据 CRUD、查询、解析及未配置执行器的明确失败契约。
	sslCreate := expectOK(http.MethodPost, "/api/v2/websites/ssl", `{"primaryDomain":"znmp.sopvip.com","otherDomains":"www.znmp.sopvip.com","provider":"self","keyType":"RSA2048"}`)
	var sslEnvelope struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(sslCreate.Body.Bytes(), &sslEnvelope); err != nil || sslEnvelope.Data.ID == 0 {
		t.Fatalf("SSL 创建响应无 ID: %s", sslCreate.Body.String())
	}
	sslID := fmt.Sprint(sslEnvelope.Data.ID)
	expectOK(http.MethodPost, "/api/v2/websites/ssl/list", `{}`)
	expectOK(http.MethodGet, "/api/v2/websites/ssl/list?domain=znmp.sopvip.com", "")
	expectOK(http.MethodPost, "/api/v2/websites/ssl/search", `{"domain":"znmp.sopvip.com"}`)
	expectOK(http.MethodGet, "/api/v2/websites/ssl/"+sslID, "")
	expectOK(http.MethodGet, "/api/v2/websites/ssl/website/1", "")
	expectOK(http.MethodPost, "/api/v2/websites/ssl/update", `{"id":`+sslID+`,"primaryDomain":"znmp.sopvip.com","description":"ZNMP TLS"}`)
	expectOK(http.MethodPost, "/api/v2/websites/ssl/resolve", `{"websiteSSLId":`+sslID+`}`)
	if res := call(http.MethodPost, "/api/v2/websites/ssl/obtain", `{"id":`+sslID+`}`); res.Code != http.StatusServiceUnavailable {
		t.Fatalf("未配置 SSL 执行器应返回 503，实际 %d: %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/websites/ssl/push", `{"id":`+sslID+`}`); res.Code != http.StatusServiceUnavailable {
		t.Fatalf("未配置 SSL 推送执行器应返回 503，实际 %d: %s", res.Code, res.Body.String())
	}
	expectOK(http.MethodPost, "/api/v2/websites/ssl/download", `{"id":`+sslID+`}`)
	expectOK(http.MethodPost, "/api/v2/websites/ssl/del", `{"ids":[`+sslID+`]}`)
	acme := expectOK(http.MethodPost, "/api/v2/websites/acme", `{"email":"znmp@example.com","type":"letsencrypt","keyType":"RSA2048"}`)
	var acmeEnvelope struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(acme.Body.Bytes(), &acmeEnvelope); err != nil || acmeEnvelope.Data.ID == 0 {
		t.Fatalf("ACME 创建响应无 ID: %s", acme.Body.String())
	}
	expectOK(http.MethodPost, "/api/v2/websites/acme/search", `{"keyword":"znmp"}`)
	expectOK(http.MethodPost, "/api/v2/websites/acme/update", `{"id":`+fmt.Sprint(acmeEnvelope.Data.ID)+`,"useProxy":false}`)
	expectOK(http.MethodPost, "/api/v2/websites/acme/del", `{"id":`+fmt.Sprint(acmeEnvelope.Data.ID)+`}`)
	ca := expectOK(http.MethodPost, "/api/v2/websites/ca", `{"name":"znmp-ca","commonName":"ZNMP Root","country":"CN","organization":"WorkMesh","keyType":"RSA2048"}`)
	var caEnvelope struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(ca.Body.Bytes(), &caEnvelope); err != nil || caEnvelope.Data.ID == 0 {
		t.Fatalf("CA 创建响应无 ID: %s", ca.Body.String())
	}
	expectOK(http.MethodPost, "/api/v2/websites/ca/search", `{"keyword":"znmp"}`)
	expectOK(http.MethodGet, "/api/v2/websites/ca/"+fmt.Sprint(caEnvelope.Data.ID), "")
	obtained := expectOK(http.MethodPost, "/api/v2/websites/ca/obtain", `{"id":`+fmt.Sprint(caEnvelope.Data.ID)+`,"domains":"znmp.sopvip.com","keyType":"RSA2048","unit":"year","time":1}`)
	var obtainedEnvelope struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(obtained.Body.Bytes(), &obtainedEnvelope); err != nil || obtainedEnvelope.Data.ID == 0 {
		t.Fatalf("CA 证书响应无 ID: %s", obtained.Body.String())
	}
	expectOK(http.MethodPost, "/api/v2/websites/ca/renew", `{"SSLID":`+fmt.Sprint(obtainedEnvelope.Data.ID)+`}`)
	if res := call(http.MethodPost, "/api/v2/websites/ca/download", `{"id":`+fmt.Sprint(caEnvelope.Data.ID)+`}`); res.Code != http.StatusOK {
		t.Fatalf("CA 下载失败: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/websites/ca/del", `{"id":`+fmt.Sprint(caEnvelope.Data.ID)+`}`); res.Code != http.StatusBadRequest {
		t.Fatalf("被证书引用的 CA 应拒绝删除，实际 %d: %s", res.Code, res.Body.String())
	}

	expectOK(http.MethodPost, "/api/v2/websites/operate", `{"id":1,"operate":"stop"}`)
	expectOK(http.MethodPost, "/api/v2/websites/operate", `{"id":1,"operate":"start"}`)
	expectOK(http.MethodPost, "/api/v2/websites/del", `{"id":1}`)
	if _, err := os.Stat(filepath.Join(base, "waf", "site.json")); !os.IsNotExist(err) {
		t.Fatalf("删除站点后独立 WAF 配置仍存在: %v", err)
	}
}

func TestWebsiteWAFTestDetectsSample(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	create := httptest.NewRequest(http.MethodPost, "/api/v2/websites", bytes.NewBufferString(`{"primaryDomain":"waf-test.example"}`))
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	mux.ServeHTTP(created, create)
	if created.Code != http.StatusOK {
		t.Fatalf("创建网站失败: %d %s", created.Code, created.Body.String())
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/websites/waf/test", bytes.NewBufferString(`{"websiteID":1,"uri":"/search?q=1 union select 1","method":"GET"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"matched":true`)) {
		t.Fatalf("WAF 样本检测失败: %d %s", rec.Code, rec.Body.String())
	}
}

func TestWebsiteAdvancedRoutes(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	created := call(http.MethodPost, "/api/v2/websites", `{"primaryDomain":"advanced.example"}`)
	if created.Code != http.StatusOK {
		t.Fatalf("创建网站失败: %d %s", created.Code, created.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v2/websites/operate", `{"id":1,"operate":"stop"}`); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"stopped"`)) {
		t.Fatalf("网站停止失败: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v2/websites/domains", `{"websiteID":1,"domain":"www.advanced.example","port":443,"ssl":true}`); rec.Code != http.StatusOK {
		t.Fatalf("域名新增失败: %d %s", rec.Code, rec.Body.String())
	}
	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v2/websites/domains/1", nil))
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte("www.advanced.example")) {
		t.Fatalf("域名列表失败: %d %s", list.Code, list.Body.String())
	}
	var domainEnvelope struct {
		Data []struct {
			Domain string `json:"domain"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &domainEnvelope); err != nil {
		t.Fatalf("域名列表响应不是数组契约: %v body=%s", err, list.Body.String())
	}
	if len(domainEnvelope.Data) != 2 || domainEnvelope.Data[0].Domain != "advanced.example" {
		t.Fatalf("创建时主域名未写入域名列表: %#v", domainEnvelope.Data)
	}
	if rec := call(http.MethodPost, "/api/v2/websites/config/update", `{"websiteID":1,"type":"nginx","config":{"content":"server {}"}}`); rec.Code != http.StatusOK {
		t.Fatalf("配置更新失败: %d %s", rec.Code, rec.Body.String())
	}
	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v2/websites/1/config/nginx", nil))
	if get.Code != http.StatusOK || !bytes.Contains(get.Body.Bytes(), []byte("server {}")) {
		t.Fatalf("配置读取失败: %d %s", get.Code, get.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v2/websites/1/https", `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("HTTPS 更新失败: %d %s", rec.Code, rec.Body.String())
	}
	getHTTPS := httptest.NewRecorder()
	mux.ServeHTTP(getHTTPS, httptest.NewRequest(http.MethodGet, "/api/v2/websites/1/https", nil))
	if getHTTPS.Code != http.StatusOK || !bytes.Contains(getHTTPS.Body.Bytes(), []byte(`"enabled":true`)) {
		t.Fatalf("HTTPS 读取失败: %d %s", getHTTPS.Code, getHTTPS.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v2/websites/rewrite/custom", `{"websiteID":1,"config":{"rules":"/old /new"}}`); rec.Code != http.StatusOK {
		t.Fatalf("自定义重写更新失败: %d %s", rec.Code, rec.Body.String())
	}
	rewriteGet := httptest.NewRecorder()
	mux.ServeHTTP(rewriteGet, httptest.NewRequest(http.MethodGet, "/api/v2/websites/rewrite/custom?websiteID=1", nil))
	if rewriteGet.Code != http.StatusOK || !bytes.Contains(rewriteGet.Body.Bytes(), []byte("/old /new")) {
		t.Fatalf("自定义重写读取失败: %d %s", rewriteGet.Code, rewriteGet.Body.String())
	}
}

func TestWebsiteCheckUsesOpenRestyInstallContainerStatus(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	resetAppStoreForTest()
	defer resetAppStoreForTest()
	appMux := http.NewServeMux()
	RegisterAppRoutes(appMux)
	install := httptest.NewRecorder()
	appMux.ServeHTTP(install, httptest.NewRequest(http.MethodPost, "/api/v2/apps/install", bytes.NewBufferString(`{"id":"resty","key":"openresty","name":"resty-prod","version":"1.27.1","containerName":"workmesh-openresty"}`)))
	if install.Code != http.StatusOK {
		t.Fatalf("安装 OpenResty 失败: %d %s", install.Code, install.Body.String())
	}
	store := getAppStore()
	store.containerStates = func(_ context.Context, _ []string) (map[string]string, error) {
		return map[string]string{"workmesh-openresty": "running"}, nil
	}
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	ok := httptest.NewRecorder()
	mux.ServeHTTP(ok, httptest.NewRequest(http.MethodPost, "/api/v2/websites/check", bytes.NewBufferString(`{}`)))
	if ok.Code != http.StatusOK || !bytes.Contains(ok.Body.Bytes(), []byte(`"data":null`)) {
		t.Fatalf("运行中的 OpenResty 不应阻止创建: %d %s", ok.Code, ok.Body.String())
	}
	store.containerStates = func(_ context.Context, _ []string) (map[string]string, error) {
		return map[string]string{"workmesh-openresty": "exited"}, nil
	}
	stopped := httptest.NewRecorder()
	mux.ServeHTTP(stopped, httptest.NewRequest(http.MethodPost, "/api/v2/websites/check", bytes.NewBufferString(`{}`)))
	if stopped.Code != http.StatusOK || !bytes.Contains(stopped.Body.Bytes(), []byte(`"status":"Stopped"`)) {
		t.Fatalf("停止的 OpenResty 应返回预检异常: %d %s", stopped.Code, stopped.Body.String())
	}
}

func TestWebsiteLBSAndResourceRoutes(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	create := httptest.NewRequest(http.MethodPost, "/api/v2/websites", bytes.NewBufferString(`{"primaryDomain":"resource.example"}`))
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	mux.ServeHTTP(created, create)
	if created.Code != http.StatusOK {
		t.Fatalf("网站创建失败: %d %s", created.Code, created.Body.String())
	}
	for _, path := range []string{"/api/v2/websites/1/lbs", "/api/v2/websites/resource/1"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusOK {
			t.Fatalf("%s 返回 %d: %s", path, res.Code, res.Body.String())
		}
	}
}

func TestWebsiteAdvancedRouteValidation(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/websites/domains", bytes.NewBufferString(`{"websiteID":999,"domain":"x.example"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("不存在网站应返回 404，实际 %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWebsiteExtendedFieldsAndLogs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/websites", bytes.NewBufferString(`{"primaryDomain":"extended.example","protocol":"HTTPS","websiteSSLId":7,"errorLog":false,"domains":[{"domain":"www.extended.example","port":443,"ssl":true}]}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte(`"websiteSSLId":7`)) {
		t.Fatalf("扩展字段未保存: %d %s", res.Code, res.Body.String())
	}
	var envelope struct {
		Data struct {
			SiteDir string `json:"siteDir"`
		} `json:"data"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &envelope)
	logPath := filepath.Join(root, "websites", "1", "logs", "access.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("GET /\nPOST /login\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	logReq := httptest.NewRequest(http.MethodPost, "/api/v2/websites/log/search", bytes.NewBufferString(`{"id":1,"logType":"access.log","page":1,"pageSize":1}`))
	logReq.Header.Set("Content-Type", "application/json")
	logRes := httptest.NewRecorder()
	mux.ServeHTTP(logRes, logReq)
	if logRes.Code != http.StatusOK || !bytes.Contains(logRes.Body.Bytes(), []byte("GET /")) {
		t.Fatalf("真实日志读取失败: %d %s", logRes.Code, logRes.Body.String())
	}
}

func TestWebsiteConfigAliasesPersist(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := call(http.MethodPost, "/api/v2/websites", `{"primaryDomain":"aliases.example"}`); rec.Code != http.StatusOK {
		t.Fatalf("创建网站失败: %d %s", rec.Code, rec.Body.String())
	}
	for _, item := range []struct{ path, cfg string }{
		{"/api/v2/websites/config/update", `{"content":"server { listen 80; }"}`},
		{"/api/v2/websites/cors/update", `{"enabled":true,"origins":["https://example.com"]}`},
		{"/api/v2/websites/lbs/create", `{"upstreams":[{"address":"127.0.0.1:8080"}]}`},
		{"/api/v2/websites/proxy/clear", `{"enabled":false}`},
	} {
		body := `{"websiteID":1,"config":` + item.cfg + `}`
		if rec := call(http.MethodPost, item.path, body); rec.Code != http.StatusOK {
			t.Fatalf("配置写入 %s 失败: %d %s", item.path, rec.Code, rec.Body.String())
		}
	}
	if rec := call(http.MethodGet, "/api/v2/websites/1/config/nginx", ""); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("listen 80")) {
		t.Fatalf("网站配置查询失败: %d %s", rec.Code, rec.Body.String())
	}
	monitor := call(http.MethodPost, "/api/v2/websites/monitor/config/site/update", `{"websiteID":1,"enabled":false}`)
	if monitor.Code != http.StatusOK {
		t.Fatalf("监控配置更新失败: %d %s", monitor.Code, monitor.Body.String())
	}
}

func TestWebsiteConfigAndProxyReadRealSiteFiles(t *testing.T) {
	root := t.TempDir()
	siteRoot := filepath.Join(root, "wwwroot")
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_WEBSITE_ROOT", siteRoot)
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	call := func(method, route, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, route, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	created := call(http.MethodPost, "/api/v2/websites", `{"primaryDomain":"proxy.example","type":"static"}`)
	if created.Code != http.StatusOK {
		t.Fatalf("创建站点失败: %d %s", created.Code, created.Body.String())
	}
	siteDir := filepath.Join(siteRoot, "proxy.example")
	if err := os.WriteFile(filepath.Join(siteDir, "nginx", "site.conf"), []byte("server {\n    root /www/wwwroot/proxy.example/app;\n    index index.php index.html default.htm;\n}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(siteDir, "nginx", "proxy", "api.conf"), []byte("location /api {\n    proxy_pass http://127.0.0.1:8080;\n    proxy_set_header Host $host;\n}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	index := call(http.MethodPost, "/api/v2/websites/config", `{"operate":"update","scope":"index","websiteId":1,"params":{}}`)
	if index.Code != http.StatusOK || !bytes.Contains(index.Body.Bytes(), []byte(`"enable":true`)) || !bytes.Contains(index.Body.Bytes(), []byte("index.php")) {
		t.Fatalf("默认文档未读取 site.conf: %d %s", index.Code, index.Body.String())
	}
	proxies := call(http.MethodPost, "/api/v2/websites/proxies", `{"id":1}`)
	if proxies.Code != http.StatusOK || !bytes.Contains(proxies.Body.Bytes(), []byte("http://127.0.0.1:8080")) || !bytes.Contains(proxies.Body.Bytes(), []byte(`"enable":true`)) {
		t.Fatalf("代理未读取域名目录配置: %d %s", proxies.Code, proxies.Body.String())
	}
}

func TestOpenRestyFileAndScopeArePersistent(t *testing.T) {
	root := filepath.Join(".tmp", "openresty-api-test")
	_ = os.RemoveAll(root)
	defer os.RemoveAll(root)
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(root, "missing-openresty"))
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := call(http.MethodPost, "/api/v2/openresty/file", `{"content":"events {}\nhttp {\n  gzip on;\n}"}`); rec.Code != http.StatusOK {
		t.Fatalf("OpenResty 配置写入失败: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v2/openresty/scope", `{"scope":"http-per","params":{"gzip":"off","keepalive_timeout":"30"}}`); rec.Code != http.StatusOK {
		t.Fatalf("OpenResty 作用域更新失败: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v2/openresty/scope", `{"scope":"http-per"}`); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"gzip":"off"`)) {
		t.Fatalf("OpenResty 作用域读取失败: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodGet, "/api/v2/openresty", ""); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("events {}")) {
		t.Fatalf("OpenResty 完整配置读取失败: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v2/openresty/file", `{"content":"events {"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("括号不匹配应返回 400，实际 %d", rec.Code)
	}
}

func TestOpenRestyBuildRequiresRealBinary(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", filepath.Join(".tmp", "openresty-build-test"))
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(os.Getenv("WORKMESH_DATA_DIR"), "missing-openresty"))
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/openresty/build", bytes.NewBufferString(`{"modules":["headers-more"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !bytes.Contains(rec.Body.Bytes(), []byte("OpenResty 构建前检查失败")) {
		t.Fatalf("缺少 OpenResty 二进制时应明确返回不可用: %d %s", rec.Code, rec.Body.String())
	}
}

// TestXPackAliasesUseServeMuxMethodPatterns 验证 xpack 旧路径别名进入真实处理器。
func TestXPackAliasesUseServeMuxMethodPatterns(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v2/xpack/monitor/status"},
		{http.MethodPost, "/api/v2/xpack/monitor/stat"},
		{http.MethodGet, "/api/v2/xpack/waf/status"},
		{http.MethodGet, "/api/v2/xpack/waf/standard-rules"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(`{}`))
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code == http.StatusNotFound {
			t.Fatalf("alias %s %s returned 404", tc.method, tc.path)
		}
	}
}
