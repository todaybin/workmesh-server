// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestWebsiteWAFRoutesCRUD(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
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
		{"/api/v2/websites/dns/update", `{"records":[{"type":"A","value":"127.0.0.1"}]}`},
		{"/api/v2/websites/cors/update", `{"enabled":true,"origins":["https://example.com"]}`},
		{"/api/v2/websites/lbs/create", `{"upstreams":[{"address":"127.0.0.1:8080"}]}`},
		{"/api/v2/websites/proxy/clear", `{"enabled":false}`},
	} {
		body := `{"websiteID":1,"config":` + item.cfg + `}`
		if rec := call(http.MethodPost, item.path, body); rec.Code != http.StatusOK {
			t.Fatalf("配置写入 %s 失败: %d %s", item.path, rec.Code, rec.Body.String())
		}
	}
	if rec := call(http.MethodPost, "/api/v2/websites/dns/search", `{"websiteID":1}`); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("127.0.0.1")) {
		t.Fatalf("DNS 查询失败: %d %s", rec.Code, rec.Body.String())
	}
	monitor := call(http.MethodPost, "/api/v2/websites/monitor/config/site/update", `{"websiteID":1,"enabled":false}`)
	if monitor.Code != http.StatusOK {
		t.Fatalf("监控配置更新失败: %d %s", monitor.Code, monitor.Body.String())
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
