// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

// testCertificateForDomains 生成仅供接口测试使用的真实 PEM 证书，
// 用于验证 HTTPS 启用流程不会接受缺失或不匹配的证书。
func testCertificateForDomains(t *testing.T, domains ...string) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	serial := big.NewInt(now.UnixNano())
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: domains[0]}, DNSNames: domains, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return string(certPEM), string(keyPEM)
}

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
	for _, rel := range []string{"app", "nginx", "nginx/rewrite", "nginx/proxy", "nginx/redirect", "nginx/auth_basic", "nginx/path_auth", "nginx/upstream", "waf", "logs", "ssl", ".workmesh/runtime", ".workmesh/cache"} {
		if info, err := os.Stat(filepath.Join(base, rel)); err != nil || !info.IsDir() {
			t.Fatalf("缺少站点目录 %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "config", "basic")); !os.IsNotExist(err) {
		t.Fatalf("不应创建 config/basic 兼容磁盘目录: %v", err)
	}
	for _, rel := range []string{"nginx/site.conf", "nginx/stream.conf"} {
		if info, err := os.Stat(filepath.Join(base, rel)); err != nil || info.IsDir() {
			t.Fatalf("缺少站点配置文件 %s: %v", rel, err)
		}
	}
	for _, rel := range []string{"waf/config.json", "waf/rules.json"} {
		if info, err := os.Stat(filepath.Join(base, rel)); err != nil || info.IsDir() {
			t.Fatalf("缺少文件型 WAF 配置 %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "waf", "site.json")); !os.IsNotExist(err) {
		t.Fatalf("不应生成旧 WAF JSON sidecar: %v", err)
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
	options := expectOK(http.MethodPost, "/api/v2/websites/options", `{}`)
	var websiteOptions struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(options.Body.Bytes(), &websiteOptions); err != nil || len(websiteOptions.Data) == 0 {
		t.Fatalf("网站 options 必须返回数组: %s", options.Body.String())
	}
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
	certificate, privateKey := testCertificateForDomains(t, "znmp.sopvip.com", "www.znmp.sopvip.com")
	certificateBody, err := json.Marshal(map[string]string{"type": "pem", "certificate": certificate, "privateKey": privateKey})
	if err != nil {
		t.Fatal(err)
	}
	sslUpload := expectOK(http.MethodPost, "/api/v2/websites/ssl/upload", string(certificateBody))
	var importedSSLEnvelope struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(sslUpload.Body.Bytes(), &importedSSLEnvelope); err != nil || importedSSLEnvelope.Data.ID == 0 {
		t.Fatalf("ZNMP 测试证书导入响应无 ID: %s", sslUpload.Body.String())
	}
	expectOK(http.MethodPost, "/api/v2/websites/1/https", fmt.Sprintf(`{"enabled":true,"websiteSSLId":%d}`, importedSSLEnvelope.Data.ID))
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
	expectOK(http.MethodPost, "/api/v2/websites/proxies", `{"id":1}`)
	expectOK(http.MethodPost, "/api/v2/websites/proxies/update", `{"id":1,"name":"znmp-proxy","proxyPass":"127.0.0.1:8080"}`)
	expectOK(http.MethodPost, "/api/v2/websites/proxies/status", `{"id":1,"name":"znmp-proxy","status":"enable"}`)
	expectOK(http.MethodPost, "/api/v2/websites/proxies/file", `{"websiteID":1,"name":"znmp-proxy.conf","content":"location /proxy { return 200; }"}`)
	expectOK(http.MethodPost, "/api/v2/websites/proxies/delete", `{"id":1,"name":"znmp-proxy.conf"}`)
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
	var templateArchive bytes.Buffer
	zipWriter := zip.NewWriter(&templateArchive)
	part, err := zipWriter.Create("index.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("<h1>{{siteName}}</h1>")); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	var uploadBody bytes.Buffer
	uploadForm := multipart.NewWriter(&uploadBody)
	uploadFile, err := uploadForm.CreateFormFile("file", "znmp.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uploadFile.Write(templateArchive.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := uploadForm.Close(); err != nil {
		t.Fatal(err)
	}
	uploadReq := httptest.NewRequest(http.MethodPost, "/api/v2/websites/templates/upload", &uploadBody)
	uploadReq.Header.Set("Content-Type", uploadForm.FormDataContentType())
	uploadRes := httptest.NewRecorder()
	mux.ServeHTTP(uploadRes, uploadReq)
	if uploadRes.Code != http.StatusOK {
		t.Fatalf("POST /api/v2/websites/templates/upload 返回 %d: %s", uploadRes.Code, uploadRes.Body.String())
	}
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

	// SSL 资源接口：验证证书元数据 CRUD、查询、解析及推送设置持久化契约。
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
	if _, err := os.Stat(filepath.Join(base, "waf")); !os.IsNotExist(err) {
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
