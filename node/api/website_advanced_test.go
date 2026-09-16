// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// TestWebsiteAdvancedRoutes 验证网站域名、HTTPS、配置和重写接口的组合行为。
func TestWebsiteAdvancedRoutes(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	RegisterSSLRoutes(mux)
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
	certificate, privateKey := testCertificateForDomains(t, "advanced.example", "www.advanced.example")
	certificateBody, err := json.Marshal(map[string]string{"type": "pem", "certificate": certificate, "privateKey": privateKey})
	if err != nil {
		t.Fatal(err)
	}
	sslUpload := call(http.MethodPost, "/api/v2/websites/ssl/upload", string(certificateBody))
	if sslUpload.Code != http.StatusOK {
		t.Fatalf("测试证书导入失败: %d %s", sslUpload.Code, sslUpload.Body.String())
	}
	var sslEnvelope struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(sslUpload.Body.Bytes(), &sslEnvelope); err != nil || sslEnvelope.Data.ID == 0 {
		t.Fatalf("测试证书导入响应无 ID: %s", sslUpload.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v2/websites/1/https", fmt.Sprintf(`{"enabled":true,"websiteSSLId":%d}`, sslEnvelope.Data.ID)); rec.Code != http.StatusOK {
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

// TestWebsiteCheckUsesOpenRestyInstallContainerStatus 验证网站预检读取真实 OpenResty 容器状态。
func TestWebsiteCheckUsesOpenRestyInstallContainerStatus(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	dbStore, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(dbStore); err != nil {
		_ = dbStore.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		resetSharedStoreForTest()
		_ = dbStore.Close()
	})
	resetAppStoreForTest()
	defer resetAppStoreForTest()
	appMux := http.NewServeMux()
	RegisterAppRoutes(appMux)
	install := httptest.NewRecorder()
	appMux.ServeHTTP(install, httptest.NewRequest(http.MethodPost, "/api/v2/apps/install", bytes.NewBufferString(`{"id":"resty","key":"openresty","name":"resty-prod","version":"1.27.1","containerName":"workmesh-openresty"}`)))
	if install.Code != http.StatusOK {
		t.Fatalf("安装 OpenResty 失败: %d %s", install.Code, install.Body.String())
	}
	appStore := getAppStore()
	appStore.containerStates = func(_ context.Context, _ []string) (map[string]string, error) {
		return map[string]string{"workmesh-openresty": "running"}, nil
	}
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	ok := httptest.NewRecorder()
	mux.ServeHTTP(ok, httptest.NewRequest(http.MethodPost, "/api/v2/websites/check", bytes.NewBufferString(`{}`)))
	if ok.Code != http.StatusOK || !bytes.Contains(ok.Body.Bytes(), []byte(`"data":null`)) {
		t.Fatalf("运行中的 OpenResty 不应阻止创建: %d %s", ok.Code, ok.Body.String())
	}
	appStore.containerStates = func(_ context.Context, _ []string) (map[string]string, error) {
		return map[string]string{"workmesh-openresty": "exited"}, nil
	}
	stopped := httptest.NewRecorder()
	mux.ServeHTTP(stopped, httptest.NewRequest(http.MethodPost, "/api/v2/websites/check", bytes.NewBufferString(`{}`)))
	if stopped.Code != http.StatusOK || !bytes.Contains(stopped.Body.Bytes(), []byte(`"status":"Stopped"`)) {
		t.Fatalf("停止的 OpenResty 应返回预检异常: %d %s", stopped.Code, stopped.Body.String())
	}
}

// TestWebsiteLBSAndResourceRoutes 验证负载均衡和资源详情路由返回真实站点数据。
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

func TestWebsiteLoadBalanceFrontendShapeAndDelete(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_OPENRESTY_BIN", filepath.Join(t.TempDir(), "missing-openresty"))
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}
	if res := call(http.MethodPost, "/api/v2/websites", `{"primaryDomain":"lb-frontend.example"}`); res.Code != http.StatusOK {
		t.Fatalf("create website: %d %s", res.Code, res.Body.String())
	}
	create := call(http.MethodPost, "/api/v2/websites/lbs/create", `{"websiteID":1,"name":"primary","algorithm":"roundRobin","servers":[{"server":"127.0.0.1:18080","weight":2,"failTimeout":5,"failTimeoutUnit":"s","maxFails":3,"maxConns":10,"flag":""}]}`)
	if create.Code != http.StatusOK {
		t.Fatalf("create load balance: %d %s", create.Code, create.Body.String())
	}
	list := call(http.MethodGet, "/api/v2/websites/1/lbs", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"name":"primary"`) || !strings.Contains(list.Body.String(), `"server":"127.0.0.1:18080"`) {
		t.Fatalf("load balance readback: %d %s", list.Code, list.Body.String())
	}
	deleted := call(http.MethodPost, "/api/v2/websites/lbs/del", `{"websiteID":1,"name":"primary"}`)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete load balance: %d %s", deleted.Code, deleted.Body.String())
	}
	list = call(http.MethodGet, "/api/v2/websites/1/lbs", "")
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), "127.0.0.1:18080") {
		t.Fatalf("deleted load balance remains: %d %s; delete=%s", list.Code, list.Body.String(), deleted.Body.String())
	}
}

// TestWebsiteAdvancedRouteValidation 验证不存在的网站 ID 被正确拒绝。
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

// TestWebsiteLimitConnArrayProtocol 验证 limit-conn 数组参数和删除契约。
func TestWebsiteLimitConnArrayProtocol(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	t.Setenv("WORKMESH_WEBSITE_ROOT", filepath.Join(t.TempDir(), "wwwroot"))
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := call(http.MethodPost, "/api/v2/websites", `{"primaryDomain":"limit.example"}`); rec.Code != 200 {
		t.Fatalf("create: %s", rec.Body.String())
	}
	body := `{"websiteId":1,"scope":"limit-conn","operate":"update","params":[{"limit_conn":"perserver 300"},{"limit_conn":"perip 25"},{"limit_rate":"512k"}]}`
	if rec := call(http.MethodPost, "/api/v2/websites/config", body); rec.Code != 200 {
		t.Fatalf("update: %s", rec.Body.String())
	}
	get := call(http.MethodPost, "/api/v2/websites/config", `{"websiteId":1,"scope":"limit-conn","operate":"get"}`)
	if get.Code != 200 || !bytes.Contains(get.Body.Bytes(), []byte(`"enable":true`)) || !bytes.Contains(get.Body.Bytes(), []byte(`perserver`)) {
		t.Fatalf("get: %s", get.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v2/websites/config", `{"websiteId":1,"scope":"limit-conn","operate":"delete","params":[]}`); rec.Code != 200 {
		t.Fatalf("delete: %s", rec.Body.String())
	}
	bad := call(http.MethodPost, "/api/v2/websites/config", `{"websiteId":1,"scope":"limit-conn","operate":"update","params":[{"limit_conn":"bad 1"}]}`)
	if bad.Code != 400 {
		t.Fatalf("invalid directive status=%d body=%s", bad.Code, bad.Body.String())
	}
}

// TestWebsiteManagedSettingFragmentsAndRollback 验证网站托管配置片段和失败回滚。
func TestWebsiteManagedSettingFragmentsAndRollback(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_WEBSITE_ROOT", filepath.Join(root, "wwwroot"))
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}
	if res := call(http.MethodPost, "/api/v2/websites", `{"primaryDomain":"settings.example"}`); res.Code != http.StatusOK {
		t.Fatalf("create: %d %s", res.Code, res.Body.String())
	}
	settings := []struct {
		path string
		body string
		file string
		want string
	}{
		{"/api/v2/websites/proxy/config", `{"websiteID":1,"config":{"enabled":true,"proxyPass":"127.0.0.1:28080","match":"/api"}}`, "nginx/proxy/managed.conf", "proxy_pass http://127.0.0.1:28080;"},
		{"/api/v2/websites/lbs/create", `{"websiteID":1,"config":{"upstreams":[{"address":"127.0.0.1:28080","weight":2}]}}`, "nginx/upstream/managed.conf", "weight=2"},
		{"/api/v2/websites/cors/update", `{"websiteID":1,"config":{"enabled":true,"origin":"https://client.example"}}`, "nginx/cors.conf", "Access-Control-Allow-Origin"},
		{"/api/v2/websites/realip/config", `{"websiteID":1,"config":{"enabled":true,"trusted":["10.0.0.0/8"]}}`, "nginx/realip.conf", "set_real_ip_from 10.0.0.0/8;"},
		{"/api/v2/websites/leech/update", `{"websiteID":1,"config":{"enabled":true,"domains":"settings.example"}}`, "nginx/leech/managed.conf", "valid_referers"},
		{"/api/v2/websites/redirect/update", `{"websiteID":1,"config":{"enabled":true,"target":"https://target.example","code":"302"}}`, "nginx/redirect/managed.conf", "return 302 https://target.example;"},
		{"/api/v2/websites/auths", `{"websiteID":1,"username":"tester","password":"not-recorded"}`, "nginx/auth_basic/managed.conf", "auth_basic_user_file"},
		{"/api/v2/websites/auths/path/update", `{"websiteID":1,"path":"/private","username":"tester","password":"not-recorded"}`, "nginx/path_auth/managed.conf", "location /private"},
	}
	for _, item := range settings {
		if res := call(http.MethodPost, item.path, item.body); res.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", item.path, res.Code, res.Body.String())
		}
		data, err := os.ReadFile(filepath.Join(root, "wwwroot", "settings.example", item.file))
		if err != nil || !strings.Contains(string(data), item.want) {
			t.Fatalf("%s fragment missing %s: err=%v content=%s", item.path, item.want, err, data)
		}
	}
	siteConf, err := os.ReadFile(filepath.Join(root, "wwwroot", "settings.example", "nginx", "site.conf"))
	if err != nil || !strings.Contains(string(siteConf), "# workmesh-managed") || !strings.Contains(string(siteConf), "nginx/redirect/*.conf") {
		t.Fatalf("managed includes missing: %v %s", err, siteConf)
	}
	if res := call(http.MethodPost, "/api/v2/websites/redirect/update", `{"websiteID":1,"config":{"enabled":false}}`); res.Code != http.StatusOK {
		t.Fatalf("disable redirect: %d %s", res.Code, res.Body.String())
	}
	siteConf, err = os.ReadFile(filepath.Join(root, "wwwroot", "settings.example", "nginx", "site.conf"))
	if err != nil || strings.Contains(string(siteConf), "nginx/redirect/*.conf") {
		t.Fatalf("disabled redirect include remains: %v %s", err, siteConf)
	}
	passwordFile := filepath.Join(root, "wwwroot", "settings.example", "nginx", "auth_basic", "users.htpasswd")
	if info, statErr := os.Stat(passwordFile); statErr != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("auth password file must be runtime-readable: info=%v err=%v", info, statErr)
	}
	db, dbErr := storage.Open(filepath.Join(root, "workmesh.db"))
	if dbErr != nil {
		t.Fatal(dbErr)
	}
	defer db.Close()
	var authConfig string
	if err := db.DB().QueryRow(`SELECT content FROM website_settings WHERE website_id=1 AND config_type='auths'`).Scan(&authConfig); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(authConfig, "not-recorded") || strings.Contains(strings.ToLower(authConfig), `"password"`) || !strings.Contains(authConfig, `"hasPassword":true`) {
		t.Fatalf("SQLite auth config contains plaintext or misses password marker: %s", authConfig)
	}
	bad := call(http.MethodPost, "/api/v2/websites/proxy/config", `{"websiteID":1,"config":{"enabled":true,"proxyPass":"http://bad; return 200"}}`)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid proxy should fail: %d %s", bad.Code, bad.Body.String())
	}
	good := call(http.MethodGet, "/api/v2/websites/1/config/proxy", "")
	if good.Code != http.StatusOK || !strings.Contains(good.Body.String(), "127.0.0.1:28080") {
		t.Fatalf("failed config must not replace SQLite value: %d %s", good.Code, good.Body.String())
	}
}

// TestWebsiteDeleteCleansRuntimeState 验证删除后 SQLite、配置和站点目录均无残留。
func TestWebsiteDeleteCleansRuntimeState(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_WEBSITE_ROOT", filepath.Join(root, "wwwroot"))
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	call := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}
	if res := call("/api/v2/websites", `{"primaryDomain":"delete-cleanup.example"}`); res.Code != http.StatusOK {
		t.Fatalf("create: %d %s", res.Code, res.Body.String())
	}
	if res := call("/api/v2/websites/cors/update", `{"websiteID":1,"config":{"enabled":true,"origin":"https://client.example"}}`); res.Code != http.StatusOK {
		t.Fatalf("config: %d %s", res.Code, res.Body.String())
	}
	siteRoot := filepath.Join(root, "wwwroot", "delete-cleanup.example")
	if _, err := os.Stat(filepath.Join(siteRoot, "nginx", "cors.conf")); err != nil {
		t.Fatalf("managed config missing before delete: %v", err)
	}
	if res := call("/api/v2/websites/del", `{"id":1,"deleteApp":false,"deleteBackup":false,"forceDelete":false}`); res.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(siteRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("site directory remains after delete: %v", err)
	}
	store, err := storage.Open(filepath.Join(root, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, query := range []string{
		`SELECT COUNT(*) FROM websites WHERE id=1`,
		`SELECT COUNT(*) FROM website_domains WHERE website_id=1`,
		`SELECT COUNT(*) FROM website_settings WHERE website_id=1`,
	} {
		var count int
		if err := store.DB().QueryRow(query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("runtime row remains after delete: query=%s count=%d err=%v", query, count, err)
		}
	}
}

// TestWebsiteExtendedFieldsAndLogs 验证扩展字段持久化并从真实日志文件读取记录。
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

// TestWebsiteConfigAliasesPersist 验证配置接口兼容字段写入 SQLite 后仍可读取。
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

// TestWebsiteConfigAndProxyReadRealSiteFiles 验证配置和代理查询读取真实站点文件。
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

// TestOpenRestyFileAndScopeArePersistent 验证 OpenResty 文件和作用域配置使用真实持久化。
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
	if rec := call(http.MethodPost, "/api/v2/openresty/scope", `{"scope":"http-per"}`); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"name":"gzip"`)) || !bytes.Contains(rec.Body.Bytes(), []byte(`"params":["off"]`)) {
		t.Fatalf("OpenResty 作用域读取失败: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodGet, "/api/v2/openresty", ""); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("events {}")) {
		t.Fatalf("OpenResty 完整配置读取失败: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodPost, "/api/v2/openresty/file", `{"content":"events {"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("括号不匹配应返回 400，实际 %d", rec.Code)
	}
}

// TestOpenRestyBuildRequiresRealBinary 验证缺少真实 OpenResty 二进制时返回明确不可用状态。
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
