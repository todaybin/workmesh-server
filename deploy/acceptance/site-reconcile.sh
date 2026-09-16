#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DATA_DIR="${WORKMESH_DATA_DIR:-/opt/workmesh-server/data}"
SQLITE_DB="${WORKMESH_SQLITE_DB:-$DATA_DIR/workmesh.db}"
WEBSITE_ROOT="${WORKMESH_WEBSITE_ROOT:-/www/wwwroot}"
OUT_DIR="${RECONCILE_DIR:-/tmp/workmesh-site-reconcile-$(date +%Y%m%d-%H%M%S)}"
GO_SQLITE_EXPORTER="${WORKMESH_SITE_RECONCILE_SQLITE_EXPORTER:-}"
STRICT="${STRICT:-0}"

MISMATCHES=0
WARNINGS=0
SQLITE_AVAILABLE=0

usage() {
  cat <<'USAGE'
WorkMesh read-only website/domain reconciliation.

Usage:
  deploy/acceptance/site-reconcile.sh [options]
  deploy/acceptance/site-reconcile.sh --self-test

Options:
  --out DIR          Report output directory. Default: RECONCILE_DIR or /tmp/workmesh-site-reconcile-<timestamp>
  --sqlite-db PATH   WorkMesh SQLite database path. Default: WORKMESH_DATA_DIR/workmesh.db.
  --website-root DIR Website root directory. Default: WORKMESH_WEBSITE_ROOT or /www/wwwroot.
  --strict           Return non-zero when warnings or mismatches exist.
  --help             Show this help.

Safety:
  The script only reads SQLite and website files, and writes reports under the
  output directory. It does not reload OpenResty, change DNS, modify websites,
  delete files, or execute write requests.
USAGE
}

log() {
  printf '[site-reconcile] %s\n' "$*"
}

warn() {
  WARNINGS=$((WARNINGS + 1))
  printf '[warn] %s\n' "$*" | tee -a "$OUT_DIR/mismatches.txt" >/dev/null
}

mismatch() {
  MISMATCHES=$((MISMATCHES + 1))
  printf '[mismatch] %s\n' "$*" | tee -a "$OUT_DIR/mismatches.txt" >/dev/null
}

pass() {
  printf '[pass] %s\n' "$*" >>"$OUT_DIR/mismatches.txt"
}

has_command() {
  command -v "$1" >/dev/null 2>&1
}

sqlite_query() {
  local output=$1
  local sql=$2
  sqlite3 -readonly -noheader -separator "$(printf '\t')" "$SQLITE_DB" "$sql" >"$output"
}

go_sqlite_export() {
  local output=$1
  local kind=$2
  if [[ -n "$GO_SQLITE_EXPORTER" ]]; then
    "$GO_SQLITE_EXPORTER" --db "$SQLITE_DB" --website-root "$WEBSITE_ROOT" --kind "$kind" >"$output"
    return
  fi
  if ! has_command go; then
    return 127
  fi
  GOCACHE="${GOCACHE:-/tmp/workmesh-go-cache}" go run "$REPO_ROOT/deploy/acceptance/cmd/site-reconcile-sqlite" --db "$SQLITE_DB" --website-root "$WEBSITE_ROOT" --kind "$kind" >"$output"
}

server_names() {
  local conf=$1
  awk '
    {
      line = $0
      sub(/#.*/, "", line)
      if (line ~ /(^|[ \t])server_name[ \t]+/) {
        sub(/^.*server_name[ \t]+/, "", line)
        sub(/;.*/, "", line)
        print line
      }
    }
  ' "$conf" | tr ' \t' '\n' | sed '/^$/d' | LC_ALL=C sort -u
}

has_server_name() {
  local conf=$1
  local name=$2
  server_names "$conf" | grep -Fxq "$name"
}

capture_filesystem_confs() {
  local output="$OUT_DIR/filesystem-site-confs.tsv"
  log "capturing filesystem site.conf list"
  if [[ ! -d "$WEBSITE_ROOT" ]]; then
    warn "website root missing: $WEBSITE_ROOT"
    : >"$output"
    return
  fi
  find "$WEBSITE_ROOT" -mindepth 2 -maxdepth 4 -path '*/nginx/site.conf' -type f -print |
    LC_ALL=C sort >"$output"
}

capture_sqlite() {
  log "capturing SQLite website/domain rows"
  if [[ ! -f "$SQLITE_DB" ]]; then
    warn "SQLite database missing: $SQLITE_DB"
    : >"$OUT_DIR/sqlite-websites.tsv"
    : >"$OUT_DIR/sqlite-domains.tsv"
    return
  fi
  if has_command sqlite3; then
    if ! sqlite_query "$OUT_DIR/sqlite-websites.tsv" "SELECT id,primary_domain,COALESCE(NULLIF(site_dir,''),'$WEBSITE_ROOT'||'/'||primary_domain),status,type FROM websites ORDER BY id;"; then
      warn "failed to query websites from $SQLITE_DB"
      : >"$OUT_DIR/sqlite-websites.tsv"
      return
    fi
    if ! sqlite_query "$OUT_DIR/sqlite-domains.tsv" "SELECT website_id,domain,port,ssl FROM website_domains ORDER BY website_id,domain;"; then
      warn "failed to query website_domains from $SQLITE_DB"
      : >"$OUT_DIR/sqlite-domains.tsv"
      return
    fi
  else
    if ! go_sqlite_export "$OUT_DIR/sqlite-websites.tsv" websites; then
      warn "sqlite3 command missing and Go SQLite exporter failed; SQLite reconciliation skipped"
      : >"$OUT_DIR/sqlite-websites.tsv"
      : >"$OUT_DIR/sqlite-domains.tsv"
      return
    fi
    if ! go_sqlite_export "$OUT_DIR/sqlite-domains.tsv" domains; then
      warn "sqlite3 command missing and Go SQLite exporter failed for domains; SQLite reconciliation skipped"
      : >"$OUT_DIR/sqlite-websites.tsv"
      : >"$OUT_DIR/sqlite-domains.tsv"
      return
    fi
  fi
  SQLITE_AVAILABLE=1
}

domain_rows_for_site() {
  local website_id=$1
  awk -F '\t' -v id="$website_id" '$1 == id { print }' "$OUT_DIR/sqlite-domains.tsv"
}

path_known_by_sqlite() {
  local path=$1
  awk -F '\t' -v path="$path" '{
    expected = $3 "/nginx/site.conf"
    if (expected == path) {
      found = 1
    }
  } END { exit found ? 0 : 1 }' "$OUT_DIR/sqlite-websites.tsv"
}

classify_orphan_site_conf() {
  local conf=$1
  local names=$2
  local rel="$conf"
  rel="${rel#"$WEBSITE_ROOT"/}"
  local site="${rel%%/*}"
  local match_text="$site $names"
  local classification="possible-production-or-external"
  local next_action="confirm DNS, certificate, document root, upstream and owner before import or cleanup"

  if [[ -z "$names" ]]; then
    classification="unknown-no-server-name"
    next_action="inspect site.conf manually before any reload, import or cleanup"
  elif [[ "$match_text" =~ (^|[[:space:]/,])types-[^[:space:]/,]+\.example($|[[:space:],]) ]]; then
    classification="website-type-acceptance-sample"
    next_action="rebuild with isolated real acceptance domain or authorize cleanup"
  elif [[ "$match_text" =~ (^|[[:space:]/,])waf[^[:space:]/,]*\.example($|[[:space:],]) ]]; then
    classification="waf-acceptance-sample"
    next_action="rebuild WAF request matrix with real acceptance domain or authorize cleanup"
  elif [[ "$match_text" =~ (^|[[:space:]/,])(upgrade|legacy)[^[:space:]/,]*\.example($|[[:space:],]) ]]; then
    classification="migration-or-legacy-sample"
    next_action="compare with migration evidence, then import, archive or authorize cleanup"
  elif [[ "$match_text" =~ (^|[[:space:]/,])[^[:space:]/,]+\.example($|[[:space:],]) || "$match_text" =~ (^|[[:space:]/,])example\.(com|org|net)($|[[:space:],]) ]]; then
    classification="test-or-sample"
    next_action="keep only if still needed for acceptance; otherwise authorize cleanup"
  fi

  printf '%s\t%s\n' "$classification" "$next_action"
}

reconcile_sqlite_to_files() {
  log "checking SQLite records against site.conf and WAF files"
  local output="$OUT_DIR/sqlite-to-files.tsv"
  printf 'website_id\tprimary_domain\tsite_dir\tstatus\ttype\tconf\tresult\n' >"$output"
  if [[ "$SQLITE_AVAILABLE" -ne 1 ]]; then
    printf '# skipped: SQLite rows are unavailable\n' >>"$output"
    return
  fi
  while IFS=$'\t' read -r website_id primary_domain site_dir status site_type; do
    [[ -n "${website_id:-}" ]] || continue
    local conf="$site_dir/nginx/site.conf"
    local result="ok"
    if [[ ! -f "$conf" ]]; then
      mismatch "website $website_id $primary_domain missing site.conf: $conf"
      result="missing-site-conf"
    else
      if has_server_name "$conf" "$primary_domain"; then
        pass "website $website_id primary domain present in site.conf: $primary_domain"
      else
        mismatch "website $website_id primary domain not in site.conf server_name: $primary_domain"
        result="primary-domain-missing"
      fi
      while IFS=$'\t' read -r _ domain port ssl; do
        [[ -n "${domain:-}" ]] || continue
        if has_server_name "$conf" "$domain"; then
          pass "website $website_id extra domain present in site.conf: $domain"
        else
          mismatch "website $website_id domain row not in site.conf server_name: $domain port=$port ssl=$ssl"
          result="domain-row-missing"
        fi
      done < <(domain_rows_for_site "$website_id")
    fi
    for waf_file in "$site_dir/waf/config.json" "$site_dir/waf/rules.json"; do
      if [[ -f "$waf_file" ]]; then
        pass "website $website_id WAF file exists: $waf_file"
      else
        warn "website $website_id WAF file missing: $waf_file"
      fi
    done
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$website_id" "$primary_domain" "$site_dir" "$status" "$site_type" "$conf" "$result" >>"$output"
  done <"$OUT_DIR/sqlite-websites.tsv"
}

reconcile_files_to_sqlite() {
  log "checking filesystem site.conf files against SQLite records"
  local output="$OUT_DIR/files-to-sqlite.tsv"
  printf 'conf\tserver_names\tresult\n' >"$output"
  if [[ "$SQLITE_AVAILABLE" -ne 1 ]]; then
    printf '# skipped: SQLite rows are unavailable\n' >>"$output"
    return
  fi
  while IFS= read -r conf; do
    [[ -n "$conf" ]] || continue
    local names
    names="$(server_names "$conf" | paste -sd ',' -)"
    local result="known"
    if path_known_by_sqlite "$conf"; then
      pass "site.conf known by SQLite: $conf"
    else
      mismatch "site.conf has no matching SQLite site_dir: $conf names=${names:-<none>}"
      result="orphan-site-conf"
    fi
    printf '%s\t%s\t%s\n' "$conf" "${names:-}" "$result" >>"$output"
  done <"$OUT_DIR/filesystem-site-confs.tsv"
}

write_orphan_classification() {
  log "classifying orphan site.conf files"
  local output="$OUT_DIR/orphan-site-classification.tsv"
  printf 'conf\tserver_names\tclassification\tnext_action\n' >"$output"
  if [[ "$SQLITE_AVAILABLE" -ne 1 ]]; then
    printf '# skipped: SQLite rows are unavailable\n' >>"$output"
    return
  fi
  while IFS=$'\t' read -r conf names result; do
    [[ "$result" == "orphan-site-conf" ]] || continue
    local classified classification next_action
    classified="$(classify_orphan_site_conf "$conf" "$names")"
    classification="${classified%%$'\t'*}"
    next_action="${classified#*$'\t'}"
    printf '%s\t%s\t%s\t%s\n' "$conf" "$names" "$classification" "$next_action" >>"$output"
  done < <(tail -n +2 "$OUT_DIR/files-to-sqlite.tsv")
}

write_summary() {
  {
    printf 'sqlite_db=%s\n' "$SQLITE_DB"
    printf 'sqlite_available=%s\n' "$SQLITE_AVAILABLE"
    printf 'website_root=%s\n' "$WEBSITE_ROOT"
    printf 'sqlite_websites=%s\n' "$(grep -c . "$OUT_DIR/sqlite-websites.tsv" || true)"
    printf 'sqlite_domains=%s\n' "$(grep -c . "$OUT_DIR/sqlite-domains.tsv" || true)"
    printf 'filesystem_site_confs=%s\n' "$(grep -c . "$OUT_DIR/filesystem-site-confs.tsv" || true)"
    printf 'orphan_site_confs=%s\n' "$(awk -F '\t' 'NR > 1 && $3 == "orphan-site-conf" { count++ } END { print count + 0 }' "$OUT_DIR/files-to-sqlite.tsv" 2>/dev/null || printf 0)"
    printf 'warnings=%s\n' "$WARNINGS"
    printf 'mismatches=%s\n' "$MISMATCHES"
  } >"$OUT_DIR/summary.txt"
}

run_reconcile() {
  install -d -m 0755 "$OUT_DIR"
  : >"$OUT_DIR/mismatches.txt"
  log "report directory: $OUT_DIR"
  capture_sqlite
  capture_filesystem_confs
  reconcile_sqlite_to_files
  reconcile_files_to_sqlite
  write_orphan_classification
  write_summary
  log "summary: warnings=$WARNINGS mismatches=$MISMATCHES"
  if [[ "$STRICT" == "1" ]] && { [[ "$WARNINGS" -gt 0 ]] || [[ "$MISMATCHES" -gt 0 ]]; }; then
    exit 1
  fi
}

self_test() {
  local target="$SCRIPT_DIR/site-reconcile.sh"
  bash -n "$target"
  local scan
  scan="$(sed '/^self_test()/,/^}/d' "$target")"
  if printf '%s\n' "$scan" | grep -Eiq '(^|[[:space:]])docker[[:space:]]|curl[[:space:]].*-X|nginx[[:space:]]+-s|openresty[[:space:]]+-s|(^|[[:space:]])certbot[[:space:]]|rm[[:space:]]+-rf|sqlite3[[:space:]].*(INSERT|UPDATE|DELETE|DROP|ALTER|VACUUM)'; then
    echo "self-test failed: script contains a forbidden mutating command" >&2
    exit 1
  fi
  local tmp
  tmp="$(mktemp -d "${TMPDIR:-/tmp}/workmesh-site-reconcile-selftest.XXXXXX")"
  mkdir -p "$tmp/bin" "$tmp/sites/example.com/nginx" "$tmp/sites/example.com/waf" "$tmp/sites/orphan.example/nginx"
  printf 'placeholder\n' >"$tmp/workmesh.db"
  cat >"$tmp/bin/sqlite3" <<'SQLITE'
#!/usr/bin/env bash
set -eu
sql="${@: -1}"
case "$sql" in
  *"FROM websites"*)
    printf '1\texample.com\t%s/sites/example.com\trunning\tstatic\n' "$WORKMESH_SITE_RECONCILE_TMP"
    ;;
  *"FROM website_domains"*)
    printf '1\twww.example.com\t80\t0\n'
    ;;
esac
SQLITE
  chmod 0755 "$tmp/bin/sqlite3"
  cat >"$tmp/sites/example.com/nginx/site.conf" <<'CONF'
server {
    listen 80;
    server_name example.com www.example.com;
}
CONF
  printf '{"enabled":true}\n' >"$tmp/sites/example.com/waf/config.json"
  printf '[]\n' >"$tmp/sites/example.com/waf/rules.json"
  printf 'server { server_name orphan.example; }\n' >"$tmp/sites/orphan.example/nginx/site.conf"
  WORKMESH_SITE_RECONCILE_TMP="$tmp" PATH="$tmp/bin:$PATH" RECONCILE_DIR="$tmp/report" WORKMESH_SQLITE_DB="$tmp/workmesh.db" WORKMESH_WEBSITE_ROOT="$tmp/sites" "$target" >/dev/null
  grep -Fq 'mismatches=1' "$tmp/report/summary.txt" || {
    echo "self-test failed: orphan site.conf mismatch was not detected" >&2
    exit 1
  }
  grep -Fq 'orphan_site_confs=1' "$tmp/report/summary.txt" || {
    echo "self-test failed: orphan site.conf count was not written" >&2
    exit 1
  }
  grep -Fq 'orphan.example' "$tmp/report/orphan-site-classification.tsv" || {
    echo "self-test failed: orphan site.conf classification was not written" >&2
    exit 1
  }
  grep -Fq 'example.com' "$tmp/report/sqlite-to-files.tsv" || {
    echo "self-test failed: SQLite website was not reconciled" >&2
    exit 1
  }
  echo "self-test passed"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --out)
        OUT_DIR="${2:?missing value for --out}"
        shift 2
        ;;
      --sqlite-db)
        SQLITE_DB="${2:?missing value for --sqlite-db}"
        shift 2
        ;;
      --website-root)
        WEBSITE_ROOT="${2:?missing value for --website-root}"
        shift 2
        ;;
      --strict)
        STRICT=1
        shift
        ;;
      --self-test)
        self_test
        exit 0
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      *)
        printf 'unknown argument: %s\n\n' "$1" >&2
        usage >&2
        exit 2
        ;;
    esac
  done
}

parse_args "$@"
run_reconcile
