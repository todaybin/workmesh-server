// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestExternalWebsiteLifecycle 使用临时 Nginx 和上游容器验证网站配置的真实加载与访问。
func TestExternalWebsiteLifecycle(t *testing.T) {
	if os.Getenv("WORKMESH_WEBSITE_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_WEBSITE_EXTERNAL_TEST=1 to run Docker website acceptance")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI unavailable")
	}
	root := t.TempDir()
	websiteRoot := filepath.Join(root, "sites")
	upstreamRoot := filepath.Join(root, "upstream")
	if err := os.MkdirAll(upstreamRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(upstreamRoot, "index.html"), []byte("workmesh-upstream-ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nginxConfig := filepath.Join(root, "nginx.conf")
	config := fmt.Sprintf("pid /tmp/nginx.pid;\nevents {}\nhttp {\n include /usr/local/openresty/nginx/conf/mime.types;\n limit_conn_zone $server_name zone=perserver:10m;\n limit_conn_zone $binary_remote_addr zone=perip:10m;\n server { listen 80 default_server; return 404; }\n include %s/*/nginx/site.conf;\n}\n", websiteRoot)
	if err := os.WriteFile(nginxConfig, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	networkName := "workmesh-acceptance-website-" + suffix
	upstreamName := "workmesh-acceptance-upstream-" + suffix
	phpName := "workmesh-acceptance-php-" + suffix
	nginxName := "workmesh-acceptance-openresty-" + suffix
	dockerExternal(t, "network", "create", networkName)
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", nginxName, upstreamName, phpName).Run()
		_ = exec.Command("docker", "network", "rm", networkName).Run()
	})
	dockerExternal(t, "run", "-d", "--rm", "--name", upstreamName, "--network", networkName, "-v", upstreamRoot+":/srv:ro", "python:3.12-alpine", "python", "-m", "http.server", "80", "--directory", "/srv")
	dockerExternal(t, "run", "-d", "--rm", "--name", phpName, "--network", networkName, "-v", websiteRoot+":"+websiteRoot, "1panel-php-fpm:7.4.33")
	time.Sleep(800 * time.Millisecond)
	httpPort := externalFreePort(t)
	httpsPort := externalFreePort(t)
	dockerExternal(t, "run", "-d", "--rm", "--name", nginxName, "--network", networkName,
		"-p", fmt.Sprintf("127.0.0.1:%d:80", httpPort), "-p", fmt.Sprintf("127.0.0.1:%d:443", httpsPort),
		"-v", nginxConfig+":/usr/local/openresty/nginx/conf/nginx.conf:ro", "-v", websiteRoot+":"+websiteRoot, externalWAFImage(t))

	t.Setenv("WORKMESH_DATA_DIR", filepath.Join(root, "data"))
	t.Setenv("WORKMESH_WEBSITE_ROOT", websiteRoot)
	t.Setenv("WORKMESH_OPENRESTY_CONTAINER", nginxName)
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	RegisterSSLRoutes(mux)
	call := func(method, path string, payload any) *httptest.ResponseRecorder {
		var body io.Reader
		if payload != nil {
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			body = bytes.NewReader(encoded)
		}
		req := httptest.NewRequest(method, path, body)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}
	expectOK := func(method, path string, payload any) *httptest.ResponseRecorder {
		res := call(method, path, payload)
		if res.Code != http.StatusOK {
			t.Fatalf("%s %s: %d %s", method, path, res.Code, res.Body.String())
		}
		return res
	}

	domain := "website-" + suffix + ".cs.sopvip.com"
	expectOK(http.MethodPost, "/api/v2/websites", map[string]any{"primaryDomain": domain, "type": "static"})
	indexConfig := expectOK(http.MethodPost, "/api/v2/websites/config", map[string]any{"websiteID": 1, "scope": "index", "operate": "get"}).Body.String()
	for _, document := range []string{"index.php", "index.html", "index.htm", "default.php", "default.htm", "default.html"} {
		if !strings.Contains(indexConfig, document) {
			t.Fatalf("default document list misses %s: %s", document, indexConfig)
		}
	}
	dockerExternal(t, "exec", nginxName, "openresty", "-t")
	dockerExternal(t, "exec", nginxName, "openresty", "-s", "reload")
	time.Sleep(500 * time.Millisecond)
	status, body, _ := externalWebsiteRequest(t, false, httpPort, domain, "", "")
	if status != http.StatusOK || len(body) == 0 {
		siteConfig, _ := os.ReadFile(filepath.Join(websiteRoot, domain, "nginx", "site.conf"))
		loadedConfig, _ := exec.Command("docker", "exec", nginxName, "openresty", "-T").CombinedOutput()
		t.Fatalf("static website response: status=%d body=%q site.conf=%q openresty-T=%s", status, body, siteConfig, loadedConfig)
	}
	// A subsite owns its server/config directory but serves from a selected
	// directory below the parent site's app root, as in the original panel.
	childDomain := "child-" + suffix + ".cs.sopvip.com"
	childRoot := filepath.Join(websiteRoot, domain, "app", "child")
	if err := os.MkdirAll(childRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childRoot, "index.html"), []byte("workmesh-subsite-ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expectOK(http.MethodPost, "/api/v2/websites", map[string]any{"primaryDomain": childDomain, "type": "subsite", "parentWebsiteID": 1, "siteDir": "/child"})
	dockerExternal(t, "exec", nginxName, "openresty", "-t")
	dockerExternal(t, "exec", nginxName, "openresty", "-s", "reload")
	time.Sleep(400 * time.Millisecond)
	status, body, _ = externalWebsiteRequest(t, false, httpPort, childDomain, "", "")
	if status != http.StatusOK || !strings.Contains(body, "workmesh-subsite-ok") {
		t.Fatalf("subsite response: status=%d body=%q", status, body)
	}
	phpDomain := "php-" + suffix + ".cs.sopvip.com"
	phpSiteRoot := filepath.Join(websiteRoot, phpDomain, "app")
	if err := os.MkdirAll(phpSiteRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(phpSiteRoot, "index.php"), []byte("<?php echo 'workmesh-php-ok';"), 0o644); err != nil {
		t.Fatal(err)
	}
	expectOK(http.MethodPost, "/api/v2/websites", map[string]any{"primaryDomain": phpDomain, "type": "runtime", "runtimeID": "php-acceptance", "proxyType": "fpm", "proxy": phpName + ":9000"})
	dockerExternal(t, "exec", nginxName, "openresty", "-t")
	dockerExternal(t, "exec", nginxName, "openresty", "-s", "reload")
	time.Sleep(500 * time.Millisecond)
	status, body, _ = externalWebsiteRequest(t, false, httpPort, phpDomain, "", "")
	if status != http.StatusOK || !strings.Contains(body, "workmesh-php-ok") {
		t.Fatalf("PHP-FPM website response: status=%d body=%q", status, body)
	}
	// Anti-leech is tested while the site is still static, then disabled before
	// proxy assertions so the two settings cannot mask each other.
	expectOK(http.MethodPost, "/api/v2/websites/leech/update", map[string]any{"websiteID": 1, "enabled": true, "domains": "*.cs.sopvip.com", "extends": "html", "return": "403"})
	dockerExternal(t, "exec", nginxName, "openresty", "-t")
	dockerExternal(t, "exec", nginxName, "openresty", "-s", "reload")
	time.Sleep(300 * time.Millisecond)
	status, _, _ = externalWebsiteRequestWithReferer(t, false, httpPort, domain, "", "", "https://evil.example/")
	if status != http.StatusForbidden {
		t.Fatalf("anti-leech anonymous response: %d", status)
	}
	expectOK(http.MethodPost, "/api/v2/websites/leech/update", map[string]any{"websiteID": 1, "enabled": false})
	dockerExternal(t, "exec", nginxName, "openresty", "-t")
	dockerExternal(t, "exec", nginxName, "openresty", "-s", "reload")
	time.Sleep(300 * time.Millisecond)

	proxy := map[string]any{"websiteID": 1, "config": map[string]any{"enabled": true, "proxyPass": "http://" + upstreamName, "match": "/"}}
	expectOK(http.MethodPost, "/api/v2/websites/proxy/config", proxy)
	expectOK(http.MethodPost, "/api/v2/websites/cors/update", map[string]any{"websiteID": 1, "config": map[string]any{"enabled": true, "origin": "https://client.example"}})
	expectOK(http.MethodPost, "/api/v2/websites/realip/config", map[string]any{"websiteID": 1, "config": map[string]any{"enabled": true, "trusted": []string{"172.16.0.0/12"}}})
	dockerExternal(t, "exec", nginxName, "openresty", "-t")
	dockerExternal(t, "exec", nginxName, "openresty", "-s", "reload")
	time.Sleep(500 * time.Millisecond)
	status, body, headers := externalWebsiteRequest(t, false, httpPort, domain, "", "")
	if status != http.StatusOK || !strings.Contains(body, "workmesh-upstream-ok") || headers.Get("Access-Control-Allow-Origin") != "https://client.example" {
		t.Fatalf("proxy/CORS response: status=%d body=%q headers=%v", status, body, headers)
	}
	limitUpdate := expectOK(http.MethodPost, "/api/v2/websites/config", map[string]any{"websiteID": 1, "scope": "limit-conn", "operate": "update", "params": []map[string]string{{"limit_conn": "perserver 300"}, {"limit_conn": "perip 25"}, {"limit_rate": "512k"}}}).Body.String()
	if !strings.Contains(limitUpdate, `"enable":true`) {
		t.Fatalf("limit update did not enable: %s", limitUpdate)
	}
	limitRead := expectOK(http.MethodPost, "/api/v2/websites/config", map[string]any{"websiteID": 1, "scope": "limit-conn", "operate": "get"}).Body.String()
	if !strings.Contains(limitRead, "perserver") || !strings.Contains(limitRead, "512k") {
		t.Fatalf("limit readback mismatch: %s", limitRead)
	}
	dockerExternal(t, "exec", nginxName, "openresty", "-t")
	expectOK(http.MethodPost, "/api/v2/websites/config", map[string]any{"websiteID": 1, "scope": "limit-conn", "operate": "delete", "params": []map[string]string{}})
	dockerExternal(t, "exec", nginxName, "openresty", "-t")

	// Exercise the frontend LoadBalanceReq shape against the real OpenResty
	// container, then remove it and restore the regular proxy for later checks.
	expectOK(http.MethodPost, "/api/v2/websites/proxy/clear", map[string]any{"websiteID": 1})
	expectOK(http.MethodPost, "/api/v2/websites/lbs/create", map[string]any{"websiteID": 1, "name": "primary", "algorithm": "roundRobin", "servers": []map[string]any{{"server": upstreamName, "weight": 1}}})
	dockerExternal(t, "exec", nginxName, "openresty", "-t")
	dockerExternal(t, "exec", nginxName, "openresty", "-s", "reload")
	time.Sleep(500 * time.Millisecond)
	status, body, _ = externalWebsiteRequest(t, false, httpPort, domain, "", "")
	if status != http.StatusOK || !strings.Contains(body, "workmesh-upstream-ok") {
		t.Fatalf("load balance response: status=%d body=%q", status, body)
	}
	lbRead := expectOK(http.MethodGet, "/api/v2/websites/1/lbs", nil).Body.String()
	if !strings.Contains(lbRead, `"name":"primary"`) || !strings.Contains(lbRead, upstreamName) {
		t.Fatalf("load balance readback: %s", lbRead)
	}
	expectOK(http.MethodPost, "/api/v2/websites/lbs/file", map[string]any{"websiteID": 1, "name": "custom.conf", "content": "upstream custom_external { server " + upstreamName + "; }\n"})
	dockerExternal(t, "exec", nginxName, "openresty", "-t")
	expectOK(http.MethodPost, "/api/v2/websites/lbs/del", map[string]any{"websiteID": 1, "name": "primary"})
	dockerExternal(t, "exec", nginxName, "openresty", "-t")
	expectOK(http.MethodPost, "/api/v2/websites/proxy/config", proxy)
	dockerExternal(t, "exec", nginxName, "openresty", "-t")

	expectOK(http.MethodPost, "/api/v2/websites/redirect/update", map[string]any{"websiteID": 1, "config": map[string]any{"enabled": true, "target": "https://target.example", "code": "302"}})
	dockerExternal(t, "exec", nginxName, "openresty", "-s", "reload")
	time.Sleep(500 * time.Millisecond)
	status, _, headers = externalWebsiteRequest(t, false, httpPort, domain, "", "")
	if status != http.StatusFound || headers.Get("Location") != "https://target.example" {
		loadedConfig, _ := exec.Command("docker", "exec", nginxName, "openresty", "-T").CombinedOutput()
		t.Fatalf("redirect response: status=%d location=%q openresty-T=%s", status, headers.Get("Location"), loadedConfig)
	}
	expectOK(http.MethodPost, "/api/v2/websites/redirect/update", map[string]any{"websiteID": 1, "config": map[string]any{"enabled": false}})
	expectOK(http.MethodPost, "/api/v2/websites/auths", map[string]any{"websiteID": 1, "username": "acceptance", "password": "temporary-password"})
	dockerExternal(t, "exec", nginxName, "openresty", "-t")
	dockerExternal(t, "exec", nginxName, "openresty", "-s", "reload")
	time.Sleep(500 * time.Millisecond)
	status, _, _ = externalWebsiteRequest(t, false, httpPort, domain, "", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("basic auth anonymous response: %d", status)
	}
	status, body, _ = externalWebsiteRequest(t, false, httpPort, domain, "acceptance", "temporary-password")
	if status != http.StatusOK || !strings.Contains(body, "workmesh-upstream-ok") {
		errorLog, _ := os.ReadFile(filepath.Join(websiteRoot, domain, "logs", "error.log"))
		passwordInfo, _ := os.Stat(filepath.Join(websiteRoot, domain, "nginx", "auth_basic", "users.htpasswd"))
		t.Fatalf("basic auth response: status=%d body=%q error.log=%s passwordMode=%v", status, body, errorLog, passwordInfo)
	}
	expectOK(http.MethodPost, "/api/v2/websites/auths/update", map[string]any{"websiteID": 1, "enabled": false})

	certificate, privateKey := testCertificateForDomains(t, domain)
	ssl := expectOK(http.MethodPost, "/api/v2/websites/ssl/upload", map[string]any{"type": "pem", "certificate": certificate, "privateKey": privateKey})
	var sslEnvelope struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(ssl.Body.Bytes(), &sslEnvelope); err != nil || sslEnvelope.Data.ID == 0 {
		t.Fatalf("invalid SSL response: %v %s", err, ssl.Body.String())
	}
	expectOK(http.MethodPost, "/api/v2/websites/1/https", map[string]any{"enable": true, "websiteSSLId": sslEnvelope.Data.ID, "httpConfig": "HTTPSOnly"})
	dockerExternal(t, "exec", nginxName, "openresty", "-t")
	dockerExternal(t, "exec", nginxName, "openresty", "-s", "reload")
	time.Sleep(500 * time.Millisecond)
	status, body, _ = externalWebsiteRequest(t, true, httpsPort, domain, "", "")
	if status != http.StatusOK || !strings.Contains(body, "workmesh-upstream-ok") {
		t.Fatalf("HTTPS response: status=%d body=%q", status, body)
	}

	bad := expectOK(http.MethodGet, "/api/v2/websites/1/config/proxy", nil).Body.String()
	failed := call(http.MethodPost, "/api/v2/websites/proxy/config", map[string]any{"websiteID": 1, "config": map[string]any{"enabled": true, "proxyPass": "http://bad; return 200"}})
	if failed.Code != http.StatusBadRequest {
		t.Fatalf("invalid config must fail: %d %s", failed.Code, failed.Body.String())
	}
	after := expectOK(http.MethodGet, "/api/v2/websites/1/config/proxy", nil).Body.String()
	if bad != after {
		t.Fatalf("failed config changed persisted proxy: before=%s after=%s", bad, after)
	}

	if blocked := call(http.MethodPost, "/api/v2/websites/del", map[string]any{"id": 1, "forceDelete": false}); blocked.Code == http.StatusOK {
		t.Fatal("parent website deletion must be blocked while subsite exists")
	}
	expectOK(http.MethodPost, "/api/v2/websites/del", map[string]any{"id": 2, "forceDelete": false})
	expectOK(http.MethodPost, "/api/v2/websites/del", map[string]any{"id": 3, "forceDelete": false})
	expectOK(http.MethodPost, "/api/v2/websites/del", map[string]any{"id": 1, "forceDelete": false})
	if _, err := os.Stat(filepath.Join(websiteRoot, childDomain)); !os.IsNotExist(err) {
		t.Fatalf("subsite directory remains after delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(websiteRoot, domain)); !os.IsNotExist(err) {
		t.Fatalf("site directory remains after delete: %v", err)
	}
	dockerExternal(t, "exec", nginxName, "openresty", "-t")
}

// TestExternalWebsiteStreamLifecycle verifies the stream website path with
// real TCP and UDP upstreams. It is intentionally isolated from the HTTP
// lifecycle test because stream listeners require a separate OpenResty
// stream{} context and independent ports.
func TestExternalWebsiteStreamLifecycle(t *testing.T) {
	if os.Getenv("WORKMESH_WEBSITE_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_WEBSITE_EXTERNAL_TEST=1 to run Docker website acceptance")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI unavailable")
	}
	root := t.TempDir()
	websiteRoot := filepath.Join(root, "sites")
	if err := os.MkdirAll(websiteRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	// The upstream scripts use only Python's standard library, making the
	// behavior deterministic and observable without introducing another image.
	upstreamRoot := filepath.Join(root, "upstreams")
	if err := os.MkdirAll(upstreamRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	tcpScript := "" +
		"import socket\n" +
		"s=socket.socket(); s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1); s.bind(('0.0.0.0',19001)); s.listen(8)\n" +
		"while True:\n" +
		" c,a=s.accept(); c.sendall(b'tcp-stream-ok\\n'); c.close()\n"
	udpScript := "" +
		"import socket\n" +
		"s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind(('0.0.0.0',19002))\n" +
		"while True:\n" +
		" d,a=s.recvfrom(65535); s.sendto(b'udp-stream-ok\\n',a)\n"
	if err := os.WriteFile(filepath.Join(upstreamRoot, "tcp.py"), []byte(tcpScript), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(upstreamRoot, "udp.py"), []byte(udpScript), 0o644); err != nil {
		t.Fatal(err)
	}
	nginxConfig := filepath.Join(root, "nginx.conf")
	config := fmt.Sprintf("pid /tmp/nginx-stream.pid;\nevents {}\nhttp {\n include /usr/local/openresty/nginx/conf/mime.types;\n server { listen 80 default_server; return 404; }\n}\nstream {\n include %s/*/nginx/stream.conf;\n}\n", websiteRoot)
	if err := os.WriteFile(nginxConfig, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	networkName := "workmesh-acceptance-stream-net-" + suffix
	tcpName := "workmesh-acceptance-stream-tcp-" + suffix
	udpName := "workmesh-acceptance-stream-udp-" + suffix
	nginxName := "workmesh-acceptance-stream-openresty-" + suffix
	tcpPort := externalFreePort(t)
	udpPort := externalFreePort(t)
	dockerExternal(t, "network", "create", networkName)
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", nginxName, tcpName, udpName).Run()
		_ = exec.Command("docker", "network", "rm", networkName).Run()
	})
	dockerExternal(t, "run", "-d", "--name", tcpName, "--network", networkName, "-v", upstreamRoot+":/srv:ro", "python:3.12-alpine", "python", "/srv/tcp.py")
	dockerExternal(t, "run", "-d", "--name", udpName, "--network", networkName, "-v", upstreamRoot+":/srv:ro", "python:3.12-alpine", "python", "/srv/udp.py")
	dockerExternal(t, "run", "-d", "--rm", "--name", nginxName, "--network", networkName,
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", tcpPort, tcpPort),
		"-p", fmt.Sprintf("127.0.0.1:%d:%d/udp", udpPort, udpPort),
		"-v", nginxConfig+":/usr/local/openresty/nginx/conf/nginx.conf:ro",
		"-v", websiteRoot+":"+websiteRoot, externalWAFImage(t))

	t.Setenv("WORKMESH_DATA_DIR", filepath.Join(root, "data"))
	t.Setenv("WORKMESH_WEBSITE_ROOT", websiteRoot)
	t.Setenv("WORKMESH_OPENRESTY_CONTAINER", nginxName)
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	call := func(method, path string, payload any) *httptest.ResponseRecorder {
		var body io.Reader
		if payload != nil {
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			body = bytes.NewReader(encoded)
		}
		req := httptest.NewRequest(method, path, body)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}
	expectOK := func(method, path string, payload any) *httptest.ResponseRecorder {
		res := call(method, path, payload)
		if res.Code != http.StatusOK {
			t.Fatalf("%s %s: %d %s", method, path, res.Code, res.Body.String())
		}
		return res
	}

	// Wait for both upstream processes to bind before OpenResty reload.
	time.Sleep(700 * time.Millisecond)
	tcpDomain := "tcp-" + suffix + ".cs.sopvip.com"
	udpDomain := "udp-" + suffix + ".cs.sopvip.com"
	expectOK(http.MethodPost, "/api/v2/websites", map[string]any{
		"primaryDomain": tcpDomain, "type": "stream", "streamPorts": strconv.Itoa(tcpPort),
		"servers": []map[string]any{{"server": tcpName + ":19001", "weight": 1}},
	})
	expectOK(http.MethodPost, "/api/v2/websites", map[string]any{
		"primaryDomain": udpDomain, "type": "stream", "streamPorts": strconv.Itoa(udpPort), "udp": true,
		"servers": []map[string]any{{"server": udpName + ":19002", "weight": 1}},
	})
	dockerExternal(t, "exec", nginxName, "openresty", "-t")
	dockerExternal(t, "exec", nginxName, "openresty", "-s", "reload")

	dialTCP := func() string {
		address := fmt.Sprintf("127.0.0.1:%d", tcpPort)
		var last string
		for attempt := 0; attempt < 20; attempt++ {
			conn, err := net.DialTimeout("tcp", address, 2*time.Second)
			if err == nil {
				_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
				data := make([]byte, 1024)
				n, readErr := conn.Read(data)
				_ = conn.Close()
				if n > 0 {
					return string(data[:n])
				}
				last = fmt.Sprintf("read=%v", readErr)
			} else {
				last = err.Error()
			}
			time.Sleep(150 * time.Millisecond)
		}
		return last
	}
	if got := dialTCP(); !strings.Contains(got, "tcp-stream-ok") {
		streamConfig, _ := os.ReadFile(filepath.Join(websiteRoot, tcpDomain, "nginx", "stream.conf"))
		loadedConfig, _ := exec.Command("docker", "exec", nginxName, "openresty", "-T").CombinedOutput()
		nginxLogs, _ := exec.Command("docker", "logs", nginxName).CombinedOutput()
		tcpLogs, _ := exec.Command("docker", "logs", tcpName).CombinedOutput()
		t.Fatalf("TCP stream response: %q stream.conf=%s openresty-T=%s nginx-logs=%s upstream-logs=%s", got, streamConfig, loadedConfig, nginxLogs, tcpLogs)
	}
	udpConn, err := net.DialTimeout("udp", fmt.Sprintf("127.0.0.1:%d", udpPort), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer udpConn.Close()
	_ = udpConn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := udpConn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	udpReply := make([]byte, 1024)
	n, err := udpConn.Read(udpReply)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(udpReply[:n]), "udp-stream-ok") {
		t.Fatalf("UDP stream response: %q", udpReply[:n])
	}

	streamRead := expectOK(http.MethodGet, "/api/v2/websites/1/config/stream", nil).Body.String()
	if !strings.Contains(streamRead, tcpName+":19001") {
		t.Fatalf("TCP stream config readback missing upstream: %s", streamRead)
	}
	if !strings.Contains(expectOK(http.MethodGet, "/api/v2/websites/2", nil).Body.String(), `"udp":true`) {
		t.Fatal("UDP stream state did not persist")
	}
	expectOK(http.MethodPost, "/api/v2/websites/del", map[string]any{"id": 1, "forceDelete": false})
	expectOK(http.MethodPost, "/api/v2/websites/del", map[string]any{"id": 2, "forceDelete": false})
	if _, err := os.Stat(filepath.Join(websiteRoot, tcpDomain)); !os.IsNotExist(err) {
		t.Fatalf("TCP stream directory remains after delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(websiteRoot, udpDomain)); !os.IsNotExist(err) {
		t.Fatalf("UDP stream directory remains after delete: %v", err)
	}
}

func dockerExternal(t *testing.T, args ...string) string {
	t.Helper()
	command := exec.Command("docker", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func externalFreePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func externalWebsiteRequest(t *testing.T, https bool, port int, host, username, password string) (int, string, http.Header) {
	return externalWebsiteRequestWithReferer(t, https, port, host, username, password, "")
}

func externalWebsiteRequestWithReferer(t *testing.T, https bool, port int, host, username, password, referer string) (int, string, http.Header) {
	t.Helper()
	scheme := "http"
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if https {
		scheme = "https"
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // 测试只验证刚生成的隔离自签证书。
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s://127.0.0.1:%d/", scheme, port), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = host
	if referer != "" {
		request.Header.Set("Referer", referer)
	}
	if username != "" {
		request.SetBasicAuth(username, password)
	}
	var response *http.Response
	for attempt := 0; attempt < 30; attempt++ {
		response, err = client.Do(request)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, string(body), response.Header.Clone()
}
