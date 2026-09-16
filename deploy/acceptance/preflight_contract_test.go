// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package acceptance

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreflightScriptKeepsRepositoryRelativeContracts(t *testing.T) {
	raw, err := os.ReadFile("preflight.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)

	for _, fragment := range []string{
		`SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"`,
		`REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"`,
		`"$REPO_ROOT/deploy/database/docker-compose.yml.tmpl"`,
		`"$REPO_ROOT/web/node_modules/@playwright"`,
		`"$REPO_ROOT/deploy/acceptance/domain-acceptance.sh"`,
		`source "$SCRIPT_DIR/host-ip.sh"`,
		`"$REPO_ROOT/deploy/acceptance/host-ip.sh"`,
		`"$REPO_ROOT/deploy/install/domain-plan.sh"`,
		`"$REPO_ROOT/deploy/openresty-waf/tests/image-release-contract.sh"`,
		`"$REPO_ROOT/docs/operations/waf-domain-acceptance.md"`,
		`CONTAINER_OPENRESTY_BIN="${WORKMESH_CONTAINER_OPENRESTY_BIN:-/usr/local/openresty/nginx/sbin/nginx}"`,
		`check_path "$OPENRESTY_DIR/waf/generated/custom-rules.conf"`,
		`check_path "$OPENRESTY_DIR/waf/generated/global-rules.conf"`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("preflight script is missing repository-relative contract %q", fragment)
		}
	}

	for _, forbidden := range []string{
		"docker compose down",
		"docker rm",
		"docker volume rm",
		"docker system prune",
		"certbot certonly",
		"nginx -s reload",
		"openresty -s reload",
		"rm -rf",
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("preflight script contains forbidden mutating operation %q", forbidden)
		}
	}
}

func TestPreflightScriptSyntax(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available")
	}
	for _, script := range []string{
		"preflight.sh",
		"readonly-evidence.sh",
		"site-reconcile.sh",
		"domain-acceptance.sh",
		"host-ip.sh",
		"../install/activate-release.sh",
		"../install/uninstall-production.sh",
	} {
		cmd := exec.Command(bash, "-n", script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bash -n %s failed: %v\n%s", script, err, out)
		}
	}
}

func TestPreflightHelpDoesNotRunReadinessChecks(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available")
	}
	cmd := exec.Command(bash, "preflight.sh", "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("preflight --help failed: %v\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "Usage:") || !strings.Contains(text, "--domain NAME") {
		t.Fatalf("preflight --help output missing usage text:\n%s", text)
	}
	if strings.Contains(text, "WorkMesh production preflight started") {
		t.Fatalf("preflight --help unexpectedly ran readiness checks:\n%s", text)
	}
}

func TestReadonlyEvidenceKeepsReadOnlyEvidenceContract(t *testing.T) {
	raw, err := os.ReadFile("readonly-evidence.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, fragment := range []string{
		`REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"`,
		`CONTAINER_OPENRESTY_BIN="${WORKMESH_CONTAINER_OPENRESTY_BIN:-/usr/local/openresty/nginx/sbin/nginx}"`,
		`docker exec "$OPENRESTY_CONTAINER" "$CONTAINER_OPENRESTY_BIN" -t`,
		`docker compose -f "$DATABASE_COMPOSE" config --quiet`,
		`API_BASE="${API_BASE:-${WORKMESH_API_BASE:-}}"`,
		`SQLITE_DB="${WORKMESH_SQLITE_DB:-${WORKMESH_DATA_DIR:-/opt/workmesh-server/data}/workmesh.db}"`,
		`sqlite3 -readonly -header -column "$SQLITE_DB"`,
		`capture_sqlite_summary`,
		`capture_api_probes`,
		`waf-status "$api_root/api/v2/websites/waf/status"`,
		`self-test failed: SQLite summary branch was not captured`,
		`self-test failed: API probe branch was not captured`,
		`--dump-header -`,
		`--output /dev/null`,
		`cid_provided`,
		`redact_stream`,
		`redact_url`,
		`capture_manifest`,
		`capture_waf_runtime_summary`,
		`$OUT_DIR/waf/runtime-summary.txt`,
		`runtime_json_sha256=`,
		`runtime_json.effective=`,
		`generated_conf=`,
		`standard_rules_present=`,
		`standard-rules.conf`,
		`jq -e 'type == "object"`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("readonly evidence script is missing contract fragment %q", fragment)
		}
	}
	scan := stripShellFunction(script, "self_test")
	for _, forbidden := range []string{
		"docker compose up",
		"docker compose down",
		"docker compose restart",
		"docker rm",
		"docker volume rm",
		"docker system prune",
		"nginx -s reload",
		"openresty -s reload",
		"certbot certonly",
		"certbot renew",
		"curl -X POST",
		"curl -X PUT",
		"curl -X PATCH",
		"curl -X DELETE",
		"rm -rf",
	} {
		if strings.Contains(scan, forbidden) {
			t.Fatalf("readonly evidence script contains forbidden mutating operation %q", forbidden)
		}
	}
}

func TestSiteReconcileKeepsReadOnlyContract(t *testing.T) {
	raw, err := os.ReadFile("site-reconcile.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, fragment := range []string{
		`SQLITE_DB="${WORKMESH_SQLITE_DB:-$DATA_DIR/workmesh.db}"`,
		`WEBSITE_ROOT="${WORKMESH_WEBSITE_ROOT:-/www/wwwroot}"`,
		`GO_SQLITE_EXPORTER="${WORKMESH_SITE_RECONCILE_SQLITE_EXPORTER:-}"`,
		`sqlite3 -readonly -noheader`,
		`go run "$REPO_ROOT/deploy/acceptance/cmd/site-reconcile-sqlite"`,
		`server_name`,
		`waf/config.json`,
		`waf/rules.json`,
		`orphan-site-conf`,
		`orphan-site-classification.tsv`,
		`orphan_site_confs=`,
		`classifying orphan site.conf files`,
		`possible-production-or-external`,
		`SQLITE_AVAILABLE=0`,
		`# skipped: SQLite rows are unavailable`,
		`self-test failed: orphan site.conf mismatch was not detected`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("site reconcile script is missing contract fragment %q", fragment)
		}
	}
	scan := stripShellFunction(script, "self_test")
	for _, forbidden := range []string{
		"docker ",
		"curl -X",
		"nginx -s",
		"openresty -s",
		"certbot ",
		"rm -rf",
	} {
		if strings.Contains(scan, forbidden) {
			t.Fatalf("site reconcile script contains forbidden mutating operation %q", forbidden)
		}
	}
}

func TestDomainAcceptanceKeepsReadOnlyContract(t *testing.T) {
	raw, err := os.ReadFile("domain-acceptance.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, fragment := range []string{
		`DEFAULT_DOMAIN="workmesh.cs.sopvip.com"`,
		`DEFAULT_PUBLIC_IP="61.184.12.165"`,
		`DOMAIN="${WORKMESH_ACCEPTANCE_DOMAIN:-${DOMAIN:-$DEFAULT_DOMAIN}}"`,
		`PUBLIC_IP="${WORKMESH_PUBLIC_IP:-${ADDRESS:-$DEFAULT_PUBLIC_IP}}"`,
		`source "$SCRIPT_DIR/host-ip.sh"`,
		`ATTACK_PATH="${ATTACK_PATH:-/%3Cscript%3Eworkmesh-waf-probe%3C%2Fscript%3E}"`,
		`EXPECTED_ATTACK_STATUS="${EXPECTED_ATTACK_STATUS:-403}"`,
		`curl_probe domain-http "http://$DOMAIN/" "$EXPECTED_HTTP_STATUS"`,
		`curl_probe domain-https "https://$DOMAIN/" "$EXPECTED_HTTPS_STATUS"`,
		`--request GET`,
		`expected one of`,
		`docker inspect --format '{{.State.Health.Status}}' "$OPENRESTY_CONTAINER"`,
		`docker exec "$OPENRESTY_CONTAINER" "$CONTAINER_OPENRESTY_BIN" -t`,
		`certificates, reload OpenResty, restart containers, or change website files`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("domain acceptance script is missing contract fragment %q", fragment)
		}
	}
	scan := stripShellFunction(script, "self_test")
	for _, forbidden := range []string{
		"docker compose up",
		"docker compose down",
		"docker compose restart",
		"docker rm",
		"docker volume rm",
		"docker system prune",
		"nginx -s reload",
		"openresty -s reload",
		"certbot certonly",
		"certbot renew",
		"curl -X POST",
		"curl -X PUT",
		"curl -X PATCH",
		"curl -X DELETE",
		"rm -rf",
	} {
		if strings.Contains(scan, forbidden) {
			t.Fatalf("domain acceptance script contains forbidden mutating operation %q", forbidden)
		}
	}
}

func stripShellFunction(script, name string) string {
	start := strings.Index(script, name+"() {")
	if start < 0 {
		return script
	}
	depth := 0
	end := start
	for end < len(script) {
		switch script[end] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return script[:start] + script[end+1:]
			}
		}
		end++
	}
	return script
}

func TestPreflightCanRunOutsideRepository(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available")
	}

	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()
	openrestyDir := filepath.Join(tmp, "openresty-waf")
	for _, dir := range []string{
		filepath.Join(openrestyDir, "waf", "data"),
		filepath.Join(openrestyDir, "waf", "generated"),
		filepath.Join(openrestyDir, "waf", "logs"),
		filepath.Join(tmp, "site-root"),
		filepath.Join(tmp, "data-root"),
		filepath.Join(tmp, "letsencrypt"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{
		filepath.Join(openrestyDir, "docker-compose.yml"),
		filepath.Join(openrestyDir, "waf", "data", "global.json"),
		filepath.Join(openrestyDir, "waf", "data", "access-lists.json"),
		filepath.Join(openrestyDir, "waf", "generated", "modsecurity-mode.conf"),
		filepath.Join(openrestyDir, "waf", "generated", "standard-rules.conf"),
	} {
		content := []byte("{}\n")
		if file == filepath.Join(openrestyDir, "docker-compose.yml") {
			content = []byte("services:\n  preflight-test:\n    image: alpine:3.20\n")
		}
		if err := os.WriteFile(file, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cmd := exec.Command(bash, filepath.Join(repoRoot, "deploy", "acceptance", "preflight.sh"))
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(),
		"CHECK_CONTAINER=0",
		"STRICT=0",
		"WORKMESH_ACCEPTANCE_DOMAIN=",
		"DOMAIN=",
		"WORKMESH_OPENRESTY_DIR="+openrestyDir,
		"WORKMESH_WEBSITE_ROOT="+filepath.Join(tmp, "site-root"),
		"WORKMESH_DATA_DIR="+filepath.Join(tmp, "data-root"),
		"WORKMESH_LETSENCRYPT_DIR="+filepath.Join(tmp, "letsencrypt"),
		"WORKMESH_PREFLIGHT_TMP="+filepath.Join(tmp, "preflight"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("preflight should run from outside the repository: %v\n%s", err, out)
	}
	output := string(out)
	for _, fragment := range []string{
		"REPO_ROOT=" + repoRoot,
		"path exists: " + filepath.Join(repoRoot, "deploy", "database", "acceptance.sh"),
		"path exists: " + filepath.Join(repoRoot, "deploy", "acceptance", "domain-acceptance.sh"),
		"path exists: " + filepath.Join(repoRoot, "deploy", "openresty-waf", "tests", "image-release-contract.sh"),
		"path exists: " + filepath.Join(repoRoot, "docs", "operations", "waf-domain-acceptance.md"),
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("preflight output is missing %q\n%s", fragment, output)
		}
	}
}
