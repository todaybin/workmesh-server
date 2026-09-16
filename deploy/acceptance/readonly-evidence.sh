#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

OUT_DIR="${EVIDENCE_DIR:-/tmp/workmesh-evidence-$(date +%Y%m%d-%H%M%S)}"
OPENRESTY_DIR="${WORKMESH_OPENRESTY_DIR:-/opt/workmesh-server/openresty-waf}"
OPENRESTY_CONTAINER="${WORKMESH_OPENRESTY_CONTAINER:-workmesh-openresty-waf}"
CONTAINER_OPENRESTY_BIN="${WORKMESH_CONTAINER_OPENRESTY_BIN:-/usr/local/openresty/nginx/sbin/nginx}"
CHECK_CONTAINER="${CHECK_CONTAINER:-0}"
WEBSITE_CONF="${WEBSITE_CONF:-}"
DATABASE_COMPOSE="${WORKMESH_DATABASE_COMPOSE:-$REPO_ROOT/deploy/database/docker-compose.yml.tmpl}"
BASE="${BASE:-}"
API_BASE="${API_BASE:-${WORKMESH_API_BASE:-}}"
CID="${CID:-}"
WEBSITE_ID="${WEBSITE_ID:-}"
SQLITE_DB="${WORKMESH_SQLITE_DB:-${WORKMESH_DATA_DIR:-/opt/workmesh-server/data}/workmesh.db}"
DOMAIN="${DOMAIN:-${WORKMESH_ACCEPTANCE_DOMAIN:-}}"
RESOLVE_IP="${RESOLVE_IP:-${WORKMESH_PUBLIC_IP:-${ADDRESS:-}}}"
ATTACK_PATH="${ATTACK_PATH:-/?workmesh_waf_probe=%3Cscript%3Ealert(1)%3C/script%3E}"
CURL_TIMEOUT="${CURL_TIMEOUT:-10}"

usage() {
  cat <<'USAGE'
WorkMesh read-only evidence collector.

Usage:
  deploy/acceptance/readonly-evidence.sh [options]
  deploy/acceptance/readonly-evidence.sh --self-test

Options:
  --out DIR              Evidence output directory. Default: EVIDENCE_DIR or /tmp/workmesh-evidence-<timestamp>
  --openresty-dir DIR    OpenResty/WAF deployment directory. Default: WORKMESH_OPENRESTY_DIR or /opt/workmesh-server/openresty-waf
  --container NAME       OpenResty container name for optional nginx -t. Default: WORKMESH_OPENRESTY_CONTAINER or workmesh-openresty-waf
  --container-nginx PATH OpenResty nginx path inside the container. Default: /usr/local/openresty/nginx/sbin/nginx
  --check-container      Run read-only docker exec <container> nginx -t.
  --website-conf PATH    Copy a redacted site.conf snapshot from this path.
  --database-compose PATH
                         Compose file/template used for docker compose config --quiet.
  --base URL             Optional HTTP base URL to probe.
  --api-base URL         Optional WorkMesh API base URL for read-only API probes.
  --cid HEADER           Optional single request header sent to curl, for example "Cookie: ...".
                         The value is never written to evidence.
  --website-id ID        Optional numeric website ID for API and SQLite summaries.
  --sqlite-db PATH       Optional WorkMesh SQLite database path.
  --domain NAME          Optional site domain for http:// and https:// probes.
  --resolve-ip IP        Optional IP used with curl --resolve for DOMAIN probes.
  --attack-path PATH     Optional WAF probe path. Default is an encoded script payload query.
  --curl-timeout SECONDS Curl max time. Default: 10.
  --help                 Show this help.

Safety:
  The script writes only into the evidence directory. It does not alter OpenResty,
  containers, certificates, DNS, database state, or website files. Curl stores
  status code and redacted response header summaries only, never response bodies.
USAGE
}

log() {
  printf '[readonly-evidence] %s\n' "$*"
}

has_command() {
  command -v "$1" >/dev/null 2>&1
}

safe_name() {
  printf '%s' "$1" | tr -c '[:alnum:]_.-' '_'
}

redact_stream() {
  sed -E \
    -e 's#([Aa]uthorization:[[:space:]]*)[^[:space:]].*#\1<redacted>#g' \
    -e 's#([Cc]ookie:[[:space:]]*)[^[:space:]].*#\1<redacted>#g' \
    -e 's#([Ss]et-[Cc]ookie:[[:space:]]*)[^[:space:]].*#\1<redacted>#g' \
    -e 's#([Xx]-[Cc][Ss][Rr][Ff]-[Tt]oken:[[:space:]]*)[^[:space:]].*#\1<redacted>#g' \
    -e 's#([Xx]-[Aa][Pp][Ii]-[Kk]ey:[[:space:]]*)[^[:space:]].*#\1<redacted>#g' \
    -e 's#([Xx]-[Ww]ork[Mm]esh-[Tt]oken:[[:space:]]*)[^[:space:]].*#\1<redacted>#g' \
    -e 's#((password|passwd|secret|token|api[_-]?key|session|csrf)[^=:"[:space:]]*[=:][[:space:]]*)[^,;[:space:]}]+#\1<redacted>#Ig' \
    -e 's#(\$?(password|passwd|secret|token|api[_-]?key|session|csrf)[^[:space:];]*[[:space:]]+)[^;[:space:]]+#\1<redacted>#Ig' \
    -e 's#(-----BEGIN )[A-Z ]*(PRIVATE KEY-----)#\1<redacted> \2#g'
}

redact_url() {
  printf '%s' "$1" | sed -E \
    -e 's#([?&][^=]*(token|key|secret|password|auth|session|csrf)[^=]*=)[^&[:space:]]+#\1<redacted>#Ig'
}

write_kv() {
  local path=$1
  local key=$2
  local value=$3
  printf '%s=%s\n' "$key" "$value" >>"$path"
}

run_capture() {
  local output=$1
  shift
  {
    printf 'command:'
    printf ' %q' "$@"
    printf '\n'
  } >"$output"
  set +e
  local command_output
  command_output="$("$@" 2>&1)"
  local status=$?
  set -e
  printf '%s\n' "$command_output" | redact_stream >>"$output"
  printf '\nexit_code=%s\n' "$status" >>"$output"
}

capture_metadata() {
  local output="$OUT_DIR/metadata.txt"
  log "capturing time/version metadata"
  {
    write_kv /dev/stdout local_time "$(date -Is)"
    write_kv /dev/stdout utc_time "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    write_kv /dev/stdout repo_root "$REPO_ROOT"
    write_kv /dev/stdout openresty_dir "$OPENRESTY_DIR"
    write_kv /dev/stdout website_conf "${WEBSITE_CONF:-<unset>}"
    write_kv /dev/stdout database_compose "$DATABASE_COMPOSE"
    write_kv /dev/stdout base "$(redact_url "${BASE:-<unset>}")"
    write_kv /dev/stdout api_base "$(redact_url "${API_BASE:-<unset>}")"
    write_kv /dev/stdout website_id "${WEBSITE_ID:-<unset>}"
    write_kv /dev/stdout sqlite_db "$SQLITE_DB"
    write_kv /dev/stdout domain "${DOMAIN:-<unset>}"
    write_kv /dev/stdout resolve_ip "${RESOLVE_IP:-<unset>}"
    write_kv /dev/stdout attack_path "$(redact_url "$ATTACK_PATH")"
    write_kv /dev/stdout cid_provided "$([[ -n "$CID" ]] && printf yes || printf no)"
    uname -a 2>/dev/null | sed 's/^/uname=/'
    if has_command git && git -C "$REPO_ROOT" rev-parse --show-toplevel >/dev/null 2>&1; then
      git -C "$REPO_ROOT" rev-parse HEAD | sed 's/^/git_head=/'
      git -C "$REPO_ROOT" status --short | sed 's/^/git_status=/'
    else
      printf 'git_head=<unavailable>\n'
    fi
    if [[ -n "${WORKMESH_SERVER_BIN:-}" && -x "${WORKMESH_SERVER_BIN:-}" ]]; then
      set +e
      local server_version
      server_version="$("$WORKMESH_SERVER_BIN" --version 2>&1)"
      local server_status=$?
      set -e
      printf '%s\n' "$server_version" | redact_stream | sed 's/^/workmesh_server_version=/'
      printf 'workmesh_server_version_exit_code=%s\n' "$server_status"
    elif has_command workmesh-server; then
      set +e
      local server_version
      server_version="$(workmesh-server --version 2>&1)"
      local server_status=$?
      set -e
      printf '%s\n' "$server_version" | redact_stream | sed 's/^/workmesh_server_version=/'
      printf 'workmesh_server_version_exit_code=%s\n' "$server_status"
    else
      printf 'workmesh_server_version=<unavailable>\n'
    fi
    if has_command docker; then
      set +e
      local docker_version
      docker_version="$(docker version 2>&1)"
      local docker_status=$?
      set -e
      printf '%s\n' "$docker_version" | redact_stream | sed 's/^/docker_version: /'
      printf 'docker_version_exit_code=%s\n' "$docker_status"
    else
      printf 'docker_version=<cli-missing>\n'
    fi
  } >"$output"
}

capture_nginx_tests() {
  install -d -m 0755 "$OUT_DIR/openresty"
  log "capturing nginx -t results"
  if [[ -n "${OPENRESTY_BIN:-}" && -x "$OPENRESTY_BIN" ]]; then
    run_capture "$OUT_DIR/openresty/nginx-test-host.txt" "$OPENRESTY_BIN" -t
  elif has_command openresty; then
    run_capture "$OUT_DIR/openresty/nginx-test-host.txt" openresty -t
  elif has_command nginx; then
    run_capture "$OUT_DIR/openresty/nginx-test-host.txt" nginx -t
  else
    printf 'skipped: no openresty/nginx binary found on host\n' >"$OUT_DIR/openresty/nginx-test-host.txt"
  fi

  case "$CHECK_CONTAINER" in
    0)
      printf 'skipped: set CHECK_CONTAINER=1 or --check-container to run container nginx -t\n' >"$OUT_DIR/openresty/nginx-test-container.txt"
      ;;
    1)
      if has_command docker; then
        run_capture "$OUT_DIR/openresty/nginx-test-container.txt" docker exec "$OPENRESTY_CONTAINER" "$CONTAINER_OPENRESTY_BIN" -t
      else
        printf 'skipped: docker CLI missing\n' >"$OUT_DIR/openresty/nginx-test-container.txt"
      fi
      ;;
    *)
      printf 'invalid CHECK_CONTAINER=%s\n' "$CHECK_CONTAINER" >"$OUT_DIR/openresty/nginx-test-container.txt"
      ;;
  esac
}

capture_waf_files() {
  local waf_dir="$OPENRESTY_DIR/waf"
  install -d -m 0755 "$OUT_DIR/waf"
  log "capturing WAF file list and sha256"
  if [[ ! -d "$waf_dir" ]]; then
    printf 'missing WAF directory: %s\n' "$waf_dir" >"$OUT_DIR/waf/files.txt"
    printf 'missing WAF directory: %s\n' "$waf_dir" >"$OUT_DIR/waf/sha256.txt"
    return
  fi
  (
    cd "$waf_dir"
    find . -xdev \( -type f -o -type l -o -type d \) -printf '%y %M %s %TY-%Tm-%TdT%TH:%TM:%TS %p\n' | LC_ALL=C sort
  ) >"$OUT_DIR/waf/files.txt"
  if has_command sha256sum; then
    (
      cd "$waf_dir"
      find . -xdev -type f -print0 |
        LC_ALL=C sort -z |
        while IFS= read -r -d '' file; do
          sha256sum "$file"
        done
    ) >"$OUT_DIR/waf/sha256.txt"
  else
    printf 'skipped: sha256sum missing\n' >"$OUT_DIR/waf/sha256.txt"
  fi
}

capture_waf_runtime_summary() {
  local waf_dir="$OPENRESTY_DIR/waf"
  local generated_dir="$waf_dir/generated"
  local runtime_json="$waf_dir/runtime.json"
  local output="$OUT_DIR/waf/runtime-summary.txt"
  log "capturing WAF runtime summary"
  {
    printf 'waf_root=%s\n' "$waf_dir"
    printf 'runtime_json=%s\n' "$runtime_json"
    if [[ -f "$runtime_json" ]]; then
      printf 'runtime_json_status=present\n'
      if has_command sha256sum; then
        printf 'runtime_json_sha256=%s\n' "$(sha256sum "$runtime_json" | awk '{print $1}')"
      else
        printf 'runtime_json_sha256=<sha256sum-missing>\n'
      fi
      if ! has_command jq; then
        printf 'runtime_json_parse_status=jq-missing\n'
      elif ! jq -e 'type == "object"' "$runtime_json" >/dev/null 2>&1; then
        printf 'runtime_json_parse_status=invalid-json-or-non-object\n'
      else
        printf 'runtime_json_parse_status=valid-object\n'
        printf 'runtime_json.effective='
        jq -r 'if (.effective | type) == "boolean" then .effective else "<missing-or-invalid>" end' "$runtime_json"
        printf 'runtime_json.configHash='
        jq -r 'if (.configHash | type) == "string" and (.configHash | test("^[0-9a-fA-F]{64}$")) then .configHash else "<missing-or-invalid>" end' "$runtime_json"
        for field in appliedAt configRoot siteRoot; do
          printf 'runtime_json.%s=' "$field"
          jq -r --arg field "$field" 'if (.[$field] | type) == "string" and (.[$field] != "") then "<present>" else "<missing-or-invalid>" end' "$runtime_json"
        done
      fi
    else
      printf 'runtime_json_status=missing\n'
      printf 'runtime_json_sha256=<unavailable>\n'
      printf 'runtime_json_parse_status=not-applicable\n'
    fi

    printf 'generated_dir=%s\n' "$generated_dir"
    if [[ ! -d "$generated_dir" ]]; then
      printf 'generated_dir_status=missing\n'
      printf 'generated_conf_count=0\n'
      printf 'standard_rules_present=no\n'
    else
      printf 'generated_dir_status=present\n'
      local count=0
      while IFS= read -r -d '' file; do
        count=$((count + 1))
        if has_command sha256sum; then
          printf 'generated_conf=%s sha256=%s\n' \
            "${file#"$generated_dir"/}" "$(sha256sum "$file" | awk '{print $1}')"
        else
          printf 'generated_conf=%s sha256=<sha256sum-missing>\n' "${file#"$generated_dir"/}"
        fi
      done < <(find "$generated_dir" -maxdepth 1 -type f -name '*.conf' -print0 | LC_ALL=C sort -z)
      printf 'generated_conf_count=%s\n' "$count"
      if [[ -f "$generated_dir/standard-rules.conf" ]]; then
        printf 'standard_rules_present=yes\n'
        if has_command sha256sum; then
          printf 'standard_rules_sha256=%s\n' "$(sha256sum "$generated_dir/standard-rules.conf" | awk '{print $1}')"
        else
          printf 'standard_rules_sha256=<sha256sum-missing>\n'
        fi
      else
        printf 'standard_rules_present=no\n'
      fi
    fi
  } >"$output"
}

capture_site_conf() {
  install -d -m 0755 "$OUT_DIR/site"
  log "capturing site.conf snapshot"
  if [[ -z "$WEBSITE_CONF" ]]; then
    printf 'skipped: WEBSITE_CONF or --website-conf is not set\n' >"$OUT_DIR/site/site-conf.txt"
    return
  fi
  if [[ ! -f "$WEBSITE_CONF" ]]; then
    printf 'missing WEBSITE_CONF: %s\n' "$WEBSITE_CONF" >"$OUT_DIR/site/site-conf.txt"
    return
  fi
  {
    printf '# source=%s\n' "$WEBSITE_CONF"
    printf '# sha256='
    if has_command sha256sum; then
      sha256sum "$WEBSITE_CONF" | awk '{print $1}'
    else
      printf '<sha256sum-missing>\n'
    fi
    redact_stream <"$WEBSITE_CONF"
  } >"$OUT_DIR/site/site-conf.txt"
}

sqlite_query() {
  local output=$1
  local title=$2
  local sql=$3
  {
    printf '\n[%s]\n' "$title"
    printf 'sql=%s\n' "$sql"
  } >>"$output"
  set +e
  local query_output
  query_output="$(sqlite3 -readonly -header -column "$SQLITE_DB" "$sql" 2>&1)"
  local status=$?
  set -e
  printf '%s\n' "$query_output" | redact_stream >>"$output"
  printf 'exit_code=%s\n' "$status" >>"$output"
}

capture_sqlite_summary() {
  install -d -m 0755 "$OUT_DIR/sqlite"
  local output="$OUT_DIR/sqlite/summary.txt"
  log "capturing SQLite read-only summary"
  if [[ ! -f "$SQLITE_DB" ]]; then
    printf 'missing sqlite database: %s\n' "$SQLITE_DB" >"$output"
    return
  fi
  if ! has_command sqlite3; then
    printf 'skipped: sqlite3 missing\n' >"$output"
    return
  fi
  {
    printf 'database=%s\n' "$SQLITE_DB"
    printf 'website_id=%s\n' "${WEBSITE_ID:-<unset>}"
  } >"$output"
  sqlite_query "$output" tables "SELECT name FROM sqlite_master WHERE type='table' AND name IN ('websites','website_domains','website_openresty_config','database_runtime_states','database_backups') ORDER BY name;"
  sqlite_query "$output" websites "SELECT id,primary_domain,type,alias,status,protocol,website_ssl_id,runtime_id,updated_at FROM websites ORDER BY id LIMIT 50;"
  sqlite_query "$output" website_domains "SELECT id,website_id,domain,port,ssl,updated_at FROM website_domains ORDER BY website_id,domain LIMIT 100;"
  sqlite_query "$output" openresty_config "SELECT id,version,enabled,default_https,ssl_reject_handshake,length(config_content) AS config_bytes,updated_at FROM website_openresty_config ORDER BY id;"
  sqlite_query "$output" database_runtime_states "SELECT container_name,status,health,image,exit_code,observed_at FROM database_runtime_states ORDER BY container_name LIMIT 20;"
  if [[ -n "$WEBSITE_ID" ]]; then
    if [[ "$WEBSITE_ID" =~ ^[0-9]+$ ]]; then
      sqlite_query "$output" "website_${WEBSITE_ID}" "SELECT id,primary_domain,type,alias,status,protocol,website_ssl_id,runtime_id,updated_at FROM websites WHERE id=$WEBSITE_ID;"
      sqlite_query "$output" "website_${WEBSITE_ID}_domains" "SELECT id,website_id,domain,port,ssl,updated_at FROM website_domains WHERE website_id=$WEBSITE_ID ORDER BY domain;"
    else
      printf '\n[website_id]\ninvalid numeric WEBSITE_ID: %s\n' "$WEBSITE_ID" >>"$output"
    fi
  fi
}

capture_database_compose() {
  install -d -m 0755 "$OUT_DIR/database"
  local output="$OUT_DIR/database/compose-config-quiet.txt"
  log "capturing database compose config --quiet"
  if [[ ! -f "$DATABASE_COMPOSE" ]]; then
    printf 'missing database compose file: %s\n' "$DATABASE_COMPOSE" >"$output"
    return
  fi
  if ! has_command docker || ! docker compose version >/dev/null 2>&1; then
    printf 'skipped: docker compose unavailable\n' >"$output"
    return
  fi
  {
    printf 'command: WORKMESH_DB_SECRETS_DIR=<redacted> docker compose -f %q config --quiet\n' "$DATABASE_COMPOSE"
  } >"$output"
  set +e
  local compose_output
  if [[ -n "${WORKMESH_DB_SECRETS_DIR:-}" ]]; then
    compose_output="$(WORKMESH_DB_SECRETS_DIR="$WORKMESH_DB_SECRETS_DIR" docker compose -f "$DATABASE_COMPOSE" config --quiet 2>&1)"
  else
    compose_output="$(docker compose -f "$DATABASE_COMPOSE" config --quiet 2>&1)"
  fi
  local status=$?
  set -e
  printf '%s\n' "$compose_output" | redact_stream >>"$output"
  printf '\nexit_code=%s\n' "$status" >>"$output"
}

summarize_headers() {
  awk '
    BEGIN { IGNORECASE = 1 }
    /^[[:space:]]*$/ { next }
    /^HTTP\// { print; next }
    {
      name = $0
      sub(/:.*/, "", name)
      lower = tolower(name)
      if (lower == "set-cookie" || lower == "cookie" || lower == "authorization" ||
          lower == "x-csrf-token" || lower == "x-api-key" || lower == "x-workmesh-token") {
        print name ": <redacted>"
      } else if (lower == "date" || lower == "server" || lower == "content-type" ||
          lower == "content-length" || lower == "location" || lower == "cache-control" ||
          lower == "strict-transport-security" || lower == "content-security-policy" ||
          lower == "x-frame-options" || lower == "x-content-type-options" ||
          lower == "www-authenticate" || lower ~ /^x-/) {
        print
      }
    }
  ' | redact_stream | head -80
}

curl_probe_to() {
  local directory=$1
  shift
  local label=$1
  local url=$2
  shift 2
  local -a extra_args=("$@")
  local safe_label
  safe_label="$(safe_name "$label")"
  install -d -m 0755 "$directory"
  local output="$directory/${safe_label}.txt"
  local -a args=(--globoff --silent --show-error --insecure --max-time "$CURL_TIMEOUT" --output /dev/null --dump-header - --write-out $'\n__WORKMESH_CURL_METRICS__%{http_code} %{remote_ip} %{time_total} %{ssl_verify_result} %{size_download}')
  if [[ -n "$CID" ]]; then
    args+=(-H "$CID")
  fi
  args+=("${extra_args[@]}" "$url")

  set +e
  local response
  response="$(curl "${args[@]}" 2>/dev/null)"
  local status=$?
  set -e
  local metrics
  metrics="$(printf '%s\n' "$response" | sed -n 's/^__WORKMESH_CURL_METRICS__//p' | tail -1)"
  local headers
  headers="$(printf '%s\n' "$response" | sed '/^__WORKMESH_CURL_METRICS__/,$d')"

  {
    printf 'label=%s\n' "$label"
    printf 'url=%s\n' "$(redact_url "$url")"
    printf 'cid_provided=%s\n' "$([[ -n "$CID" ]] && printf yes || printf no)"
    printf 'curl_exit_code=%s\n' "$status"
    printf 'curl_metrics="%s"\n' "$metrics"
    printf 'curl_stderr_discarded=%s\n' "$([[ "$status" -ne 0 ]] && printf yes || printf no)"
    printf '\n[headers]\n'
    if [[ -n "$headers" ]]; then
      summarize_headers <<<"$headers"
    else
      printf '<no response headers>\n'
    fi
  } >"$output"
}

curl_probe() {
  curl_probe_to "$OUT_DIR/http" "$@"
}

capture_api_probes() {
  install -d -m 0755 "$OUT_DIR/api"
  log "capturing WorkMesh API status probes"
  if [[ -z "$API_BASE" ]]; then
    printf 'skipped: API_BASE or --api-base is not set\n' >"$OUT_DIR/api/README.txt"
    return
  fi
  if ! has_command curl; then
    printf 'skipped: curl missing\n' >"$OUT_DIR/api/README.txt"
    return
  fi
  local api_root="${API_BASE%/}"
  curl_probe_to "$OUT_DIR/api" waf-status "$api_root/api/v2/websites/waf/status"
  curl_probe_to "$OUT_DIR/api" waf-global "$api_root/api/v2/websites/waf/global"
  curl_probe_to "$OUT_DIR/api" waf-sites "$api_root/api/v2/websites/waf/sites"
  curl_probe_to "$OUT_DIR/api" waf-overview "$api_root/api/v2/websites/waf/overview"
  curl_probe_to "$OUT_DIR/api" waf-log-sample "$api_root/api/v2/websites/waf/logs/access?page=1&pageSize=1"
  if [[ -n "$WEBSITE_ID" ]]; then
    if [[ "$WEBSITE_ID" =~ ^[0-9]+$ ]]; then
      curl_probe_to "$OUT_DIR/api" "website-${WEBSITE_ID}-domains" "$api_root/api/v2/websites/domains/$WEBSITE_ID"
      curl_probe_to "$OUT_DIR/api" "website-${WEBSITE_ID}-waf" "$api_root/api/v2/websites/waf/sites/$WEBSITE_ID"
      curl_probe_to "$OUT_DIR/api" "website-${WEBSITE_ID}-waf-rules" "$api_root/api/v2/websites/waf/sites/$WEBSITE_ID/rules"
    else
      printf 'invalid numeric WEBSITE_ID: %s\n' "$WEBSITE_ID" >"$OUT_DIR/api/website-id.txt"
    fi
  fi
}

capture_http_probes() {
  install -d -m 0755 "$OUT_DIR/http"
  log "capturing HTTP/HTTPS status probes"
  if [[ "$ATTACK_PATH" != /* ]]; then
    ATTACK_PATH="/$ATTACK_PATH"
  fi
  if ! has_command curl; then
    printf 'skipped: curl missing\n' >"$OUT_DIR/http/README.txt"
    return
  fi
  if [[ -n "$BASE" ]]; then
    curl_probe base-root "$BASE/"
    curl_probe base-attack "${BASE%/}${ATTACK_PATH}"
  fi
  if [[ -n "$DOMAIN" ]]; then
    local -a http_resolve=()
    local -a https_resolve=()
    if [[ -n "$RESOLVE_IP" ]]; then
      http_resolve=(--resolve "$DOMAIN:80:$RESOLVE_IP")
      https_resolve=(--resolve "$DOMAIN:443:$RESOLVE_IP")
    fi
    curl_probe domain-http "http://$DOMAIN/" "${http_resolve[@]}"
    curl_probe domain-https "https://$DOMAIN/" "${https_resolve[@]}"
    curl_probe domain-http-attack "http://$DOMAIN${ATTACK_PATH}" "${http_resolve[@]}"
    curl_probe domain-https-attack "https://$DOMAIN${ATTACK_PATH}" "${https_resolve[@]}"
  fi
  if [[ -z "$BASE" && -z "$DOMAIN" ]]; then
    printf 'skipped: BASE and DOMAIN are not set\n' >"$OUT_DIR/http/README.txt"
  fi
}

capture_manifest() {
  log "writing manifest"
  (
    cd "$OUT_DIR"
    find . -type f -printf '%s %p\n' | LC_ALL=C sort
  ) >"$OUT_DIR/manifest.txt"
}

self_test() {
  local target="$SCRIPT_DIR/readonly-evidence.sh"
  bash -n "$target"
  local scan
  scan="$(sed '/^self_test()/,/^}/d' "$target")"
  local destructive_regex
  destructive_regex=$'docker[[:space:]]+compose[[:space:]]+(up|down|restart|pull|push|rm)|docker[[:space:]]+(rm|rmi|restart|stop|kill|volume[[:space:]]+rm|network[[:space:]]+rm|system[[:space:]]+prune)|nginx[[:space:]]+-s|openresty[[:space:]]+-s|certbot[[:space:]]+(certonly|renew|delete)|rm[[:space:]]+-rf|curl[^\\n]*[[:space:]]+-X[[:space:]]*(POST|PUT|PATCH|DELETE)'
  if printf '%s\n' "$scan" | grep -Eiq "$destructive_regex"; then
    printf 'self-test failed: script contains a forbidden mutating command\n' >&2
    exit 1
  fi
  local tmp
  tmp="$(mktemp -d "${TMPDIR:-/tmp}/workmesh-readonly-evidence-selftest.XXXXXX")"
  mkdir -p "$tmp/bin"
  cat >"$tmp/bin/curl" <<'CURL'
#!/usr/bin/env bash
set -Eeuo pipefail
header_file=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dump-header)
      header_file="$2"
      shift 2
      ;;
    --write-out)
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [[ "$header_file" == "-" ]]; then
  printf 'HTTP/1.1 200 OK\nSet-Cookie: workmesh_session=secret-response-cookie\nAuthorization: Bearer secret-response-token\nX-CSRF-Token: secret-response-csrf\nContent-Type: text/plain\n\n'
else
  printf 'HTTP/1.1 200 OK\nSet-Cookie: workmesh_session=secret-response-cookie\nAuthorization: Bearer secret-response-token\nX-CSRF-Token: secret-response-csrf\nContent-Type: text/plain\n\n' >"$header_file"
fi
printf '\n__WORKMESH_CURL_METRICS__200 127.0.0.1 0.001 0'
CURL
  cat >"$tmp/bin/sqlite3" <<'SQLITE'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'id  primary_domain      status\n'
printf '1   selftest.example    running\n'
SQLITE
  chmod 0755 "$tmp/bin/curl"
  chmod 0755 "$tmp/bin/sqlite3"
  printf 'sqlite placeholder\n' >"$tmp/workmesh.db"
  mkdir -p "$tmp/openresty-waf/waf/generated"
  runtime_hash="$(printf 'a%.0s' {1..64})"
  printf '{"effective":true,"configHash":"%s","appliedAt":"2026-09-12T00:00:00Z","configRoot":"/secret/config","siteRoot":"/secret/sites","secret":"runtime-secret"}\n' \
    "$runtime_hash" >"$tmp/openresty-waf/waf/runtime.json"
  printf '# standard rules\n' >"$tmp/openresty-waf/waf/generated/standard-rules.conf"
  printf '# custom rules\n' >"$tmp/openresty-waf/waf/generated/custom-rules.conf"
  CID='Cookie: workmesh_session=super-secret-cookie; pcsrftoken=super-secret-csrf' \
    PATH="$tmp/bin:$PATH" \
    EVIDENCE_DIR="$tmp/evidence" WORKMESH_OPENRESTY_DIR="$tmp/openresty-waf" \
    BASE='http://selftest.invalid' API_BASE='http://selftest.invalid' DOMAIN='' WEBSITE_ID=1 \
    WORKMESH_SQLITE_DB="$tmp/workmesh.db" CHECK_CONTAINER=0 "$target" >"$tmp/run.log"
  if grep -R 'super-secret' "$tmp" >/dev/null 2>&1; then
    printf 'self-test failed: CID value leaked into evidence\n' >&2
    exit 1
  fi
  if grep -R 'secret-response' "$tmp/evidence" >/dev/null 2>&1; then
    printf 'self-test failed: response header secret leaked into evidence\n' >&2
    exit 1
  fi
  if grep -R 'response body' "$tmp/evidence" >/dev/null 2>&1; then
    printf 'self-test failed: response body leaked into evidence\n' >&2
    exit 1
  fi
  if ! grep -Fq 'selftest.example' "$tmp/evidence/sqlite/summary.txt"; then
    printf 'self-test failed: SQLite summary branch was not captured\n' >&2
    exit 1
  fi
  if ! grep -Fq 'waf-status' "$tmp/evidence/api/waf-status.txt"; then
    printf 'self-test failed: API probe branch was not captured\n' >&2
    exit 1
  fi
  runtime_summary="$tmp/evidence/waf/runtime-summary.txt"
  if ! grep -Fq 'runtime_json_status=present' "$runtime_summary" ||
    ! grep -Fq 'runtime_json_parse_status=valid-object' "$runtime_summary" ||
    ! grep -Fq 'runtime_json.effective=true' "$runtime_summary" ||
    ! grep -Fq 'runtime_json.appliedAt=<present>' "$runtime_summary" ||
    ! grep -Fq 'generated_conf=standard-rules.conf sha256=' "$runtime_summary" ||
    ! grep -Fq 'standard_rules_present=yes' "$runtime_summary"; then
    printf 'self-test failed: WAF runtime summary was not captured\n' >&2
    exit 1
  fi
  if grep -Fq '/secret/' "$runtime_summary" || grep -Fq 'runtime-secret' "$runtime_summary"; then
    printf 'self-test failed: WAF runtime sensitive values leaked into evidence\n' >&2
    exit 1
  fi
  printf 'self-test passed\n'
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --out)
        OUT_DIR="${2:?missing value for --out}"
        shift 2
        ;;
      --openresty-dir)
        OPENRESTY_DIR="${2:?missing value for --openresty-dir}"
        shift 2
        ;;
      --container)
        OPENRESTY_CONTAINER="${2:?missing value for --container}"
        shift 2
        ;;
      --container-nginx)
        CONTAINER_OPENRESTY_BIN="${2:?missing value for --container-nginx}"
        shift 2
        ;;
      --check-container)
        CHECK_CONTAINER=1
        shift
        ;;
      --website-conf)
        WEBSITE_CONF="${2:?missing value for --website-conf}"
        shift 2
        ;;
      --database-compose)
        DATABASE_COMPOSE="${2:?missing value for --database-compose}"
        shift 2
        ;;
      --base)
        BASE="${2:?missing value for --base}"
        shift 2
        ;;
      --api-base)
        API_BASE="${2:?missing value for --api-base}"
        shift 2
        ;;
      --cid)
        CID="${2:?missing value for --cid}"
        shift 2
        ;;
      --website-id)
        WEBSITE_ID="${2:?missing value for --website-id}"
        shift 2
        ;;
      --sqlite-db)
        SQLITE_DB="${2:?missing value for --sqlite-db}"
        shift 2
        ;;
      --domain)
        DOMAIN="${2:?missing value for --domain}"
        shift 2
        ;;
      --resolve-ip)
        RESOLVE_IP="${2:?missing value for --resolve-ip}"
        shift 2
        ;;
      --attack-path)
        ATTACK_PATH="${2:?missing value for --attack-path}"
        shift 2
        ;;
      --curl-timeout)
        CURL_TIMEOUT="${2:?missing value for --curl-timeout}"
        shift 2
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

main() {
  parse_args "$@"
  install -d -m 0755 "$OUT_DIR"
  log "evidence directory: $OUT_DIR"
  capture_metadata
  capture_nginx_tests
  capture_waf_files
  capture_waf_runtime_summary
  capture_site_conf
  capture_database_compose
  capture_sqlite_summary
  capture_api_probes
  capture_http_probes
  capture_manifest
  log "done"
}

main "$@"
