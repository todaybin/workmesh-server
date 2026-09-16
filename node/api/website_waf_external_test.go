package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestExternalWAFLifecycle exercises the WAF Lua runtime in an isolated
// container. It is opt-in because it creates Docker resources and binds a
// random loopback port; no existing WorkMesh container or site is touched.
func TestExternalWAFLifecycle(t *testing.T) {
	if os.Getenv("WORKMESH_WAF_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_WAF_EXTERNAL_TEST=1 to run isolated Docker WAF acceptance")
	}
	root := t.TempDir()
	sitesRoot := filepath.Join(root, "sites")
	wafRoot := filepath.Join(root, "waf")
	if err := os.MkdirAll(filepath.Join(wafRoot, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	suffix := time.Now().UnixNano()
	network := "workmesh-waf-acceptance-" + strconvBase36(suffix)
	container := network + "-nginx"
	domainA := "waf-a-" + strconvBase36(suffix) + ".cs.sopvip.com"
	domainB := "waf-b-" + strconvBase36(suffix) + ".cs.sopvip.com"
	for _, domain := range []string{domainA, domainB} {
		app := filepath.Join(sitesRoot, domain, "app")
		if err := os.MkdirAll(app, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(app, "index.html"), []byte(domain+"-ok\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	global := map[string]any{"enabled": true, "mode": "observe", "requestBodyLimit": 1048576}
	writeJSON := func(path string, value any) {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON(filepath.Join(wafRoot, "data", "global.json"), global)
	writeJSON(filepath.Join(wafRoot, "data", "access-lists.json"), map[string]any{"whitelist": []string{}, "blacklist": []string{}})
	rule := map[string]any{"id": "A-ADMIN", "location": "uri", "operator": "contains", "value": "/admin", "action": "block", "enabled": true}
	for _, domain := range []string{domainA, domainB} {
		if err := os.MkdirAll(filepath.Join(sitesRoot, domain, "waf"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON(filepath.Join(sitesRoot, domainA, "waf", "config.json"), map[string]any{"enabled": true, "mode": "observe", "rules": []any{rule}})
	writeJSON(filepath.Join(sitesRoot, domainB, "waf", "config.json"), map[string]any{"enabled": true, "mode": "observe", "rules": []any{}})
	if err := os.MkdirAll(filepath.Join(wafRoot, "logs"), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(wafRoot, "logs"), 0o777); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "nginx.conf")
	// nginx runs inside the temporary container.  The site tree is mounted at
	// /www/wwwroot, so all paths embedded in the container configuration must
	// use that mount point rather than the host's temporary directory.
	config := "pid /tmp/waf-nginx.pid;\nerror_log stderr info;\nevents {}\nhttp {\n lua_shared_dict workmesh_waf 16m;\n lua_package_path \"/usr/local/openresty/lualib/?.lua;;\";\n include /usr/local/openresty/nginx/conf/mime.types;\n server { listen 80 default_server; server_name _; return 404; }\n include /www/wwwroot/*/nginx/site.conf;\n}\n"
	for _, domain := range []string{domainA, domainB} {
		nginxDir := filepath.Join(sitesRoot, domain, "nginx")
		if err := os.MkdirAll(nginxDir, 0o755); err != nil {
			t.Fatal(err)
		}
		siteConf := "server { listen 80; server_name " + domain + "; root /www/wwwroot/" + domain + "/app; index index.html; location / { access_by_lua_file /usr/local/openresty/lualib/workmesh_waf/access.lua; try_files $uri /index.html; } }\n"
		if err := os.WriteFile(filepath.Join(nginxDir, "site.conf"), []byte(siteConf), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(configPath, []byte(config), 0o640); err != nil {
		t.Fatal(err)
	}
	port := externalFreePort(t)
	dockerExternal(t, "network", "create", network)
	t.Cleanup(func() {
		_ = os.RemoveAll(root)
		_ = dockerQuiet("rm", "-f", container)
		_ = dockerQuiet("network", "rm", network)
	})
	dockerExternal(t, "run", "-d", "--name", container, "--network", network, "-p", "127.0.0.1:"+itoa(port)+":80", "-v", sitesRoot+":/www/wwwroot:rw", "-v", wafRoot+":/opt/workmesh/waf:rw", "-v", configPath+":/usr/local/openresty/nginx/conf/nginx.conf:ro", externalWAFImage(t))
	dockerExternal(t, "exec", container, "nginx", "-t")
	dockerExternal(t, "exec", container, "nginx", "-s", "reload")
	status, _, _ := externalWebsiteRequest(t, false, port, domainA, "", "")
	if status != 200 {
		t.Fatalf("observe normal request status=%d", status)
	}
	status, _, _ = externalWebsiteRequestPath(t, port, domainA, "/admin")
	if status != 200 {
		t.Fatalf("observe attack should pass status=%d", status)
	}
	// Switch only site A to block mode; site B must remain unaffected.
	writeJSON(filepath.Join(sitesRoot, domainA, "waf", "config.json"), map[string]any{"enabled": true, "mode": "block", "rules": []any{rule}})
	time.Sleep(3500 * time.Millisecond)
	status, _, _ = externalWebsiteRequestPath(t, port, domainA, "/admin")
	if status != 403 {
		t.Fatalf("block attack should be forbidden status=%d", status)
	}
	status, _, _ = externalWebsiteRequestPath(t, port, domainB, "/admin")
	if status != 200 {
		t.Fatalf("site B rule isolation failed status=%d", status)
	}
	data, err := os.ReadFile(filepath.Join(wafRoot, "logs", "workmesh-custom-audit.jsonl"))
	if err != nil || !strings.Contains(string(data), "A-ADMIN") {
		t.Fatalf("missing WAF audit log err=%v data=%s", err, data)
	}
	// Requests originate from the Docker bridge, so match its private subnet.
	writeJSON(filepath.Join(wafRoot, "data", "access-lists.json"), map[string]any{"whitelist": []string{}, "blacklist": []string{"172.16.0.0/12"}})
	time.Sleep(3500 * time.Millisecond)
	status, _, _ = externalWebsiteRequest(t, false, port, domainB, "", "")
	if status != 403 {
		t.Fatalf("blacklist should be forbidden status=%d", status)
	}
	writeJSON(filepath.Join(wafRoot, "data", "access-lists.json"), map[string]any{"whitelist": []string{"172.16.0.0/12"}, "blacklist": []string{"172.16.0.0/12"}})
	time.Sleep(3500 * time.Millisecond)
	status, _, _ = externalWebsiteRequest(t, false, port, domainB, "", "")
	if status != 200 {
		t.Fatalf("whitelist precedence failed status=%d", status)
	}
}

func externalWebsiteRequestPath(t *testing.T, port int, host, path string) (int, string, http.Header) {
	// Reuse the existing helper's transport behavior while targeting a path.
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+itoa(port)+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), resp.Header.Clone()
}

func strconvBase36(value int64) string {
	const chars = "0123456789abcdefghijklmnopqrstuvwxyz"
	if value == 0 {
		return "0"
	}
	out := ""
	for value > 0 {
		out = string(chars[value%36]) + out
		value /= 36
	}
	return out
}
func itoa(value int) string            { return strconv.Itoa(value) }
func dockerQuiet(args ...string) error { return exec.Command("docker", args...).Run() }
