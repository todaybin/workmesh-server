#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=host-ip.sh
source "$SCRIPT_DIR/host-ip.sh"

DATA_DIR="${WORKMESH_DATA_DIR:-/opt/workmesh-server/data}"
WEBSITE_ROOT="${WORKMESH_WEBSITE_ROOT:-/www/wwwroot}"
OPENRESTY_DIR="${WORKMESH_OPENRESTY_DIR:-/opt/workmesh-server/openresty-waf}"
OPENRESTY_CONTAINER="${WORKMESH_OPENRESTY_CONTAINER:-workmesh-openresty-waf}"
CONTAINER_OPENRESTY_BIN="${WORKMESH_CONTAINER_OPENRESTY_BIN:-/usr/local/openresty/nginx/sbin/nginx}"
DEFAULT_DOMAIN="workmesh.cs.sopvip.com"
DEFAULT_PUBLIC_IP="61.184.12.165"
DOMAIN="${WORKMESH_ACCEPTANCE_DOMAIN:-${DOMAIN:-$DEFAULT_DOMAIN}}"
PUBLIC_IP="${WORKMESH_PUBLIC_IP:-${ADDRESS:-$DEFAULT_PUBLIC_IP}}"
STRICT="${STRICT:-0}"
CHECK_CONTAINER="${CHECK_CONTAINER:-0}"
TMP_ROOT="${WORKMESH_PREFLIGHT_TMP:-/tmp/workmesh-preflight-$$}"

WARNINGS=0
FAILURES=0

usage() {
  cat <<'USAGE'
Run the WorkMesh production readiness checks.

Usage:
  deploy/acceptance/preflight.sh [options]

Options:
  --domain NAME       Domain used for DNS readiness checks. Default: workmesh.cs.sopvip.com.
  --public-ip IP      Expected IPv4 address for the domain. Default: 61.184.12.165.
  --strict            Treat warnings as failures.
  --check-container   Run nginx -t inside the configured WAF container when Docker is ready.
  --help              Show this help.

Environment variables remain supported:
  WORKMESH_ACCEPTANCE_DOMAIN, WORKMESH_PUBLIC_IP, STRICT, CHECK_CONTAINER
USAGE
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
      --strict)
        STRICT=1
        shift
        ;;
      --check-container)
        CHECK_CONTAINER=1
        shift
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

note() {
  printf '[preflight] %s\n' "$*"
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

check_command() {
  if has_command "$1"; then
    pass "command available: $1 ($(command -v "$1"))"
  else
    warn "command missing: $1"
  fi
}

check_path() {
  if [[ -e "$1" ]]; then
    pass "path exists: $1"
  else
    warn "path missing: $1"
  fi
}

docker_ready() {
  has_command docker && docker version --format '{{.Server.Version}}' >/dev/null 2>&1
}

valid_domain() {
  [[ "$1" =~ ^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.cs\.sopvip\.com$ ]]
}

check_docker() {
  if ! has_command docker; then
    warn "Docker CLI missing"
    return
  fi
  pass "Docker CLI available: $(command -v docker)"
  if docker version --format '{{.Server.Version}}' >/dev/null 2>&1; then
    pass "Docker daemon accessible"
  else
    warn "Docker daemon/socket not accessible"
  fi
  if docker compose version >/dev/null 2>&1; then
    pass "docker compose available"
  else
    warn "docker compose unavailable"
  fi
}

check_openresty_layout() {
  note "checking OpenResty/WAF layout"
  check_path "$OPENRESTY_DIR/docker-compose.yml"
  check_path "$OPENRESTY_DIR/waf"
  check_path "$OPENRESTY_DIR/waf/data"
  check_path "$OPENRESTY_DIR/waf/generated"
  check_path "$OPENRESTY_DIR/waf/logs"
  check_path "$OPENRESTY_DIR/waf/data/global.json"
  check_path "$OPENRESTY_DIR/waf/data/access-lists.json"
  check_path "$OPENRESTY_DIR/waf/generated/modsecurity-mode.conf"
  check_path "$OPENRESTY_DIR/waf/generated/custom-rules.conf"
  check_path "$OPENRESTY_DIR/waf/generated/standard-rules.conf"
  check_path "$OPENRESTY_DIR/waf/generated/global-rules.conf"

  if [[ -f "$OPENRESTY_DIR/docker-compose.yml" ]] && has_command docker && docker compose version >/dev/null 2>&1; then
    check_waf_image_reference "$OPENRESTY_DIR/docker-compose.yml"
    if docker compose -f "$OPENRESTY_DIR/docker-compose.yml" config --quiet; then
      pass "OpenResty compose static config is valid"
    else
      fail "OpenResty compose static config is invalid"
    fi
  fi

  case "$CHECK_CONTAINER" in
    0) ;;
    1)
      if docker_ready; then
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
          warn "OpenResty container health is ${health:-unknown}; expected healthy"
        fi
      else
        warn "CHECK_CONTAINER=1 requested but Docker daemon is unavailable"
      fi
      ;;
    *) fail "CHECK_CONTAINER must be 0 or 1" ;;
  esac
}

check_waf_image_reference() {
  local compose_file=$1
  local image_ref
  image_ref="$(awk '
    /^[[:space:]]*image:[[:space:]]*/ {
      sub(/^[[:space:]]*image:[[:space:]]*/, "", $0)
      print
      exit
    }
  ' "$compose_file" | tr -d '"' | xargs)"
  if [[ "$image_ref" == *'${'* ]]; then
    image_ref="${WORKMESH_OPENRESTY_WAF_IMAGE:-}"
  fi
  if [[ -z "$image_ref" ]]; then
    warn "WAF image reference is unresolved; set WORKMESH_OPENRESTY_WAF_IMAGE"
    return
  fi
  case "$image_ref" in
    *:latest|*/latest|*:20260904|*/20260904)
      fail "WAF image reference is legacy or mutable: $image_ref"
      return
      ;;
  esac
  if [[ "$image_ref" =~ @sha256:[0-9a-fA-F]{64}$ ]]; then
    pass "WAF image uses immutable digest: $image_ref"
  else
    warn "WAF image is tag-pinned but not digest-pinned: $image_ref"
  fi
}

check_database_compose_template() {
  local template="$REPO_ROOT/deploy/database/docker-compose.yml.tmpl"
  note "checking database compose template"
  check_path "$template"
  check_path "$REPO_ROOT/deploy/database/acceptance.sh"
  if [[ ! -f "$template" ]] || ! has_command docker || ! docker compose version >/dev/null 2>&1; then
    return
  fi
  install -d -m 0700 "$TMP_ROOT/db-secrets"
  for secret in postgres_password redis_password mariadb_password mariadb_root_password; do
    if [[ ! -f "$TMP_ROOT/db-secrets/$secret" ]]; then
      printf 'workmesh-preflight-secret\n' >"$TMP_ROOT/db-secrets/$secret"
      chmod 0600 "$TMP_ROOT/db-secrets/$secret"
    fi
  done
  if WORKMESH_DB_SECRETS_DIR="$TMP_ROOT/db-secrets" docker compose -f "$template" config --quiet; then
    pass "database compose template static config is valid"
  else
    fail "database compose template static config is invalid"
  fi
}

check_dns() {
  note "checking domain readiness"
  if [[ -z "$DOMAIN" ]]; then
    warn "WORKMESH_ACCEPTANCE_DOMAIN/DOMAIN is not set; DNS and Host routing checks skipped"
    return
  fi
  if ! valid_domain "$DOMAIN"; then
    fail "invalid acceptance domain: $DOMAIN"
    return
  fi
  if [[ -n "$PUBLIC_IP" && "$PUBLIC_IP" != "auto" ]] && ! is_ipv4 "$PUBLIC_IP"; then
    fail "invalid expected public IPv4: $PUBLIC_IP"
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

check_acme_tools() {
  note "checking ACME/HTTPS tools"
  check_command curl
  check_command openssl
  check_command certbot
  check_path "${WORKMESH_LETSENCRYPT_DIR:-/etc/letsencrypt}"
}

check_frontend_render_tools() {
  note "checking browser render tools"
  local found_browser=0
  for bin in chromium chromium-browser google-chrome firefox; do
    if has_command "$bin"; then
      pass "browser available: $bin ($(command -v "$bin"))"
      found_browser=1
    fi
  done
  if [[ "$found_browser" == "0" ]]; then
    warn "no Chromium/Chrome/Firefox executable found"
  fi
  if [[ -d "$REPO_ROOT/web/node_modules/@playwright" ]] || [[ -d "$REPO_ROOT/web/node_modules/puppeteer" ]]; then
    pass "browser automation dependency present"
  else
    warn "Playwright/Puppeteer dependency missing"
  fi
}

check_acceptance_artifacts() {
  note "checking acceptance artifacts"
  check_path "$REPO_ROOT/docs/operations/waf-domain-acceptance.md"
  check_path "$REPO_ROOT/deploy/acceptance/domain-acceptance.sh"
  check_path "$REPO_ROOT/deploy/acceptance/host-ip.sh"
  check_path "$REPO_ROOT/deploy/install/domain-plan.sh"
  check_path "$REPO_ROOT/deploy/install/prepare-runtime.sh"
  check_path "$REPO_ROOT/deploy/openresty-waf/Dockerfile"
  check_path "$REPO_ROOT/deploy/openresty-waf/tests/runtime-contract.sh"
  check_path "$REPO_ROOT/deploy/openresty-waf/tests/image-release-contract.sh"
  check_path "$REPO_ROOT/deploy/openresty-waf/tests/domain-acceptance.sh"
  check_path "$REPO_ROOT/deploy/database/acceptance.sh"
  check_path "$WEBSITE_ROOT"
  check_path "$DATA_DIR"
}

main() {
  parse_args "$@"
  if [[ -z "$PUBLIC_IP" || "$PUBLIC_IP" == "auto" ]]; then
    PUBLIC_IP="$(detect_host_ipv4 || true)"
  fi
  note "WorkMesh production preflight started"
  note "REPO_ROOT=$REPO_ROOT"
  note "DATA_DIR=$DATA_DIR"
  note "WEBSITE_ROOT=$WEBSITE_ROOT"
  note "OPENRESTY_DIR=$OPENRESTY_DIR"
  check_docker
  check_openresty_layout
  check_database_compose_template
  check_dns
  check_acme_tools
  check_frontend_render_tools
  check_acceptance_artifacts
  note "temporary files, if created, are under $TMP_ROOT"
  note "summary: warnings=$WARNINGS failures=$FAILURES"
  if [[ "$FAILURES" -gt 0 ]] || [[ "$STRICT" == "1" && "$WARNINGS" -gt 0 ]]; then
    exit 1
  fi
}

main "$@"
