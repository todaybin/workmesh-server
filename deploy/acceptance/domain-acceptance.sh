#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=host-ip.sh
source "$SCRIPT_DIR/host-ip.sh"

DEFAULT_DOMAIN="workmesh.cs.sopvip.com"
DEFAULT_PUBLIC_IP="61.184.12.165"
DOMAIN="${WORKMESH_ACCEPTANCE_DOMAIN:-${DOMAIN:-$DEFAULT_DOMAIN}}"
PUBLIC_IP="${WORKMESH_PUBLIC_IP:-${ADDRESS:-$DEFAULT_PUBLIC_IP}}"
if [[ -z "$PUBLIC_IP" || "$PUBLIC_IP" == "auto" ]]; then
  PUBLIC_IP="$(detect_host_ipv4 || true)"
fi
BASE="${BASE:-${WORKMESH_BASE_URL:-}}"
OPENRESTY_CONTAINER="${WORKMESH_OPENRESTY_CONTAINER:-workmesh-openresty-waf}"
CONTAINER_OPENRESTY_BIN="${WORKMESH_CONTAINER_OPENRESTY_BIN:-/usr/local/openresty/nginx/sbin/nginx}"
ATTACK_PATH="${ATTACK_PATH:-/%3Cscript%3Eworkmesh-waf-probe%3C%2Fscript%3E}"
EXPECTED_HTTP_STATUS="${EXPECTED_HTTP_STATUS:-200,301,302,307,308}"
EXPECTED_HTTPS_STATUS="${EXPECTED_HTTPS_STATUS:-200}"
EXPECTED_ATTACK_STATUS="${EXPECTED_ATTACK_STATUS:-403}"
CURL_TIMEOUT="${CURL_TIMEOUT:-10}"
CHECK_CONTAINER="${CHECK_CONTAINER:-0}"
STRICT="${STRICT:-0}"

WARNINGS=0
FAILURES=0

usage() {
  cat <<'USAGE'
WorkMesh read-only domain/WAF acceptance probe.

Usage:
  deploy/acceptance/domain-acceptance.sh [options]
  deploy/acceptance/domain-acceptance.sh --self-test

Options:
  --domain NAME        Domain to probe. Default: workmesh.cs.sopvip.com.
  --public-ip IP      Optional IP used with curl --resolve. Default: 61.184.12.165.
  --base URL          Optional base URL for direct root and attack probes.
  --attack-path PATH  WAF probe path. Default: encoded script payload.
  --expected-http-status LIST   Accepted HTTP status codes, comma separated.
  --expected-https-status LIST  Accepted HTTPS status codes, comma separated.
  --expected-attack-status LIST Accepted WAF attack status codes, comma separated.
  --container NAME    OpenResty container for optional nginx -t.
  --check-container   Run container nginx -t and require healthy status when Docker daemon is accessible.
  --strict            Return non-zero when warnings or failures exist.
  --help              Show this help.

Safety:
  The script only performs DNS lookups, GET requests, optional container
  nginx -t, and summary logging. It does not write DNS, request ACME
  certificates, reload OpenResty, restart containers, or change website files.
USAGE
}

log() {
  printf '[domain-acceptance] %s\n' "$*"
}

pass() {
  printf '[pass] %s\n' "$*"
}

warn() {
  WARNINGS=$((WARNINGS + 1))
  printf '[warn] %s\n' "$*"
}

fail() {
  FAILURES=$((FAILURES + 1))
  printf '[fail] %s\n' "$*"
}

has_command() {
  command -v "$1" >/dev/null 2>&1
}

valid_domain() {
  [[ "$1" =~ ^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.cs\.sopvip\.com$ ]]
}

validate_targets() {
  local valid=0
  if ! valid_domain "$DOMAIN"; then
    fail "invalid domain: $DOMAIN"
    valid=1
  fi
  if [[ -n "$PUBLIC_IP" && "$PUBLIC_IP" != "auto" ]] && ! is_ipv4 "$PUBLIC_IP"; then
    fail "invalid public IPv4: $PUBLIC_IP"
    valid=1
  fi
  return "$valid"
}

docker_ready() {
  has_command docker && docker version --format '{{.Server.Version}}' >/dev/null 2>&1
}

curl_probe() {
  local name=$1
  local url=$2
  local expected=$3
  shift 3
  local args=(
    --silent
    --show-error
    --location
    --request GET
    --max-time "$CURL_TIMEOUT"
    --output /dev/null
    --write-out '%{http_code} %{ssl_verify_result} %{remote_ip} %{time_total}'
  )
  local output
  if output="$(curl "${args[@]}" "$@" "$url" 2>/dev/null)"; then
    local status="${output%% *}"
    if contains_status "$expected" "$status"; then
      pass "$name $url -> $output"
    else
      fail "$name $url -> $output; expected one of $expected"
    fi
  else
    fail "$name $url failed"
  fi
}

contains_status() {
  local expected=$1
  local actual=$2
  local item
  IFS=',' read -r -a items <<<"$expected"
  for item in "${items[@]}"; do
    if [[ "$item" == "$actual" ]]; then
      return 0
    fi
  done
  return 1
}

check_tools() {
  for command in curl openssl; do
    if has_command "$command"; then
      pass "command available: $command ($(command -v "$command"))"
    else
      warn "command missing: $command"
    fi
  done
}

check_dns() {
  if [[ -z "$DOMAIN" ]]; then
    warn "domain is not set; domain probes skipped"
    return
  fi
  local resolved=""
  if has_command getent; then
    resolved="$(getent ahosts "$DOMAIN" | awk '{print $1}' | sort -u | tr '\n' ' ' || true)"
  fi
  if [[ -z "$resolved" ]] && has_command dig; then
    resolved="$(dig +short A "$DOMAIN" | tr '\n' ' ' || true)"
  fi
  if [[ -n "$resolved" ]]; then
    pass "domain resolves: $DOMAIN -> $resolved"
  else
    warn "domain does not resolve from this host: $DOMAIN"
  fi
  if [[ -n "$PUBLIC_IP" ]] && [[ -n "$resolved" ]]; then
    case " $resolved " in
      *" $PUBLIC_IP "*) pass "domain includes expected address: $PUBLIC_IP" ;;
      *) warn "domain does not include expected address $PUBLIC_IP; resolved: $resolved" ;;
    esac
  fi
}

check_container() {
  if [[ "$CHECK_CONTAINER" != "1" ]]; then
    return
  fi
  if ! docker_ready; then
    warn "CHECK_CONTAINER=1 requested but Docker daemon is unavailable"
    return
  fi
  if docker exec "$OPENRESTY_CONTAINER" "$CONTAINER_OPENRESTY_BIN" -t >/dev/null 2>&1; then
    pass "OpenResty container nginx -t passed"
  else
    fail "OpenResty container nginx -t failed"
  fi
  local health
  health="$(docker inspect --format '{{.State.Health.Status}}' "$OPENRESTY_CONTAINER" 2>/dev/null || true)"
  if [[ "$health" == "healthy" ]]; then
    pass "OpenResty container health: healthy"
  else
    fail "OpenResty container health is ${health:-unknown}; expected healthy"
  fi
}

check_http() {
  if ! has_command curl; then
    warn "curl missing; HTTP probes skipped"
    return
  fi
  if [[ -n "$BASE" ]]; then
    curl_probe base-root "$BASE/" "$EXPECTED_HTTPS_STATUS"
    curl_probe base-attack "${BASE%/}${ATTACK_PATH}" "$EXPECTED_ATTACK_STATUS"
  fi
  if [[ -z "$DOMAIN" ]]; then
    return
  fi
  local http_resolve=()
  local https_resolve=()
  if [[ -n "$PUBLIC_IP" ]]; then
    http_resolve=(--resolve "$DOMAIN:80:$PUBLIC_IP")
    https_resolve=(--resolve "$DOMAIN:443:$PUBLIC_IP")
  fi
  curl_probe domain-http "http://$DOMAIN/" "$EXPECTED_HTTP_STATUS" "${http_resolve[@]}"
  curl_probe domain-http-attack "http://$DOMAIN${ATTACK_PATH}" "$EXPECTED_ATTACK_STATUS" "${http_resolve[@]}"
  curl_probe domain-https "https://$DOMAIN/" "$EXPECTED_HTTPS_STATUS" "${https_resolve[@]}"
  curl_probe domain-https-attack "https://$DOMAIN${ATTACK_PATH}" "$EXPECTED_ATTACK_STATUS" "${https_resolve[@]}"
}

self_test() {
  local target="${BASH_SOURCE[0]}"
  bash -n "$target"
  local scan
  local destructive_regex
  scan="$(sed '/^self_test()/,/^}/d' "$target")"
  destructive_regex=$'docker[[:space:]]+(compose[[:space:]]+)?(up|down|restart|rm|rmi|stop|kill|volume[[:space:]]+rm|network[[:space:]]+rm|system[[:space:]]+prune)|nginx[[:space:]]+-s|openresty[[:space:]]+-s|certbot[[:space:]]+(certonly|renew|delete)|curl[^\\n]*[[:space:]]+-X[[:space:]]*(POST|PUT|PATCH|DELETE)|rm[[:space:]]+-rf'
  if printf '%s\n' "$scan" | grep -Eiq "$destructive_regex"; then
    echo "self-test failed: script contains a forbidden mutating command" >&2
    exit 1
  fi
  echo "self-test passed"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --domain)
        DOMAIN="${2:?missing value for --domain}"
        shift 2
        ;;
      --public-ip)
        PUBLIC_IP="${2:?missing value for --public-ip}"
        shift 2
        ;;
      --base)
        BASE="${2:?missing value for --base}"
        shift 2
        ;;
      --attack-path)
        ATTACK_PATH="${2:?missing value for --attack-path}"
        shift 2
        ;;
      --expected-http-status)
        EXPECTED_HTTP_STATUS="${2:?missing value for --expected-http-status}"
        shift 2
        ;;
      --expected-https-status)
        EXPECTED_HTTPS_STATUS="${2:?missing value for --expected-https-status}"
        shift 2
        ;;
      --expected-attack-status)
        EXPECTED_ATTACK_STATUS="${2:?missing value for --expected-attack-status}"
        shift 2
        ;;
      --container)
        OPENRESTY_CONTAINER="${2:?missing value for --container}"
        shift 2
        ;;
      --check-container)
        CHECK_CONTAINER=1
        shift
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

run_acceptance() {
  log "read-only domain/WAF acceptance started"
  if ! validate_targets; then
    log "summary: warnings=$WARNINGS failures=$FAILURES"
    exit 1
  fi
  check_tools
  check_dns
  check_container
  check_http
  log "summary: warnings=$WARNINGS failures=$FAILURES"
  if [[ "$FAILURES" -gt 0 ]] || [[ "$STRICT" == "1" && "$WARNINGS" -gt 0 ]]; then
    exit 1
  fi
}

parse_args "$@"
run_acceptance
