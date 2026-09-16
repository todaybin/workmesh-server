#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ROOT_DIR="${WORKMESH_SERVER_ROOT:-/opt/workmesh-server}"
DATA_DIR="${WORKMESH_DATA_DIR:-$ROOT_DIR/data}"
DB_ROOT="${WORKMESH_DB_ROOT:-$DATA_DIR/database}"
DB_SECRETS_DIR="${WORKMESH_DB_SECRETS_DIR:-$DB_ROOT/secrets}"
OPENRESTY_DIR="${WORKMESH_OPENRESTY_DIR:-$ROOT_DIR/openresty-waf}"
WEBSITE_ROOT="${WORKMESH_WEBSITE_ROOT:-/www/wwwroot}"
DOMAIN="${WORKMESH_PUBLIC_DOMAIN:-${WORKMESH_ACCEPTANCE_DOMAIN:-workmesh.cs.sopvip.com}}"

DB_TEMPLATE="$REPO_ROOT/deploy/database/docker-compose.yml.tmpl"
DB_COMPOSE="$DB_ROOT/docker-compose.yml"
WAF_COMPOSE_TEMPLATE="$REPO_ROOT/deploy/openresty-waf/docker-compose.yml"
WAF_COMPOSE="$OPENRESTY_DIR/docker-compose.yml"
SERVER_ENV="$ROOT_DIR/config/server.env"
APPLY=0
FORCE=0
SELF_TEST=0

log() {
  printf '[prepare-runtime] %s\n' "$*"
}

fail() {
  printf '[prepare-runtime] FAIL: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'USAGE'
Prepare WorkMesh runtime directories for a production node.

Usage:
  deploy/install/prepare-runtime.sh [--apply] [--force] [--domain NAME]

Options:
  --apply   Write runtime directories and files. Without this flag the script
            only prints the actions it would take.
  --force   Replace generated runtime Compose files. Secrets and WAF data are
            still never overwritten.
  --domain NAME  Set WORKMESH_PUBLIC_URL to https://NAME in server.env.
                 NAME must match <prefix>.cs.sopvip.com.
  --self-test  Validate this script without touching production paths.
  --help    Show this help.

Environment:
  WORKMESH_SERVER_ROOT      Default: /opt/workmesh-server
  WORKMESH_DATA_DIR         Default: $WORKMESH_SERVER_ROOT/data
  WORKMESH_DB_ROOT          Default: $WORKMESH_DATA_DIR/database
  WORKMESH_DB_SECRETS_DIR   Default: $WORKMESH_DB_ROOT/secrets
  WORKMESH_OPENRESTY_DIR    Default: $WORKMESH_SERVER_ROOT/openresty-waf
  WORKMESH_WEBSITE_ROOT     Default: /www/wwwroot
  WORKMESH_PUBLIC_DOMAIN or WORKMESH_ACCEPTANCE_DOMAIN
                            Optional domain written to server.env

Safety:
  The script does not start, stop, delete, or recreate containers, images,
  networks, volumes, certificates, DNS records, or website files. Existing
  secret and WAF data files are preserved.
USAGE
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --apply)
        APPLY=1
        shift
        ;;
      --force)
        FORCE=1
        shift
        ;;
      --domain)
        DOMAIN="${2:?missing value for --domain}"
        shift 2
        ;;
      --self-test)
        SELF_TEST=1
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

valid_domain() {
  [[ "$1" =~ ^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.cs\.sopvip\.com$ ]]
}

run_self_test() {
  local scan
  bash -n "${BASH_SOURCE[0]}"
  scan="$(sed '/^run_self_test()/,/^}/d' "${BASH_SOURCE[0]}")"
  if printf '%s\n' "$scan" | grep -Eiq \
    'docker[[:space:]]+(compose[[:space:]]+)?(up|down|restart|rm|rmi|stop|kill|volume[[:space:]]+rm|network[[:space:]]+rm|system[[:space:]]+prune)|systemctl[[:space:]]|nginx[[:space:]]+-s|openresty[[:space:]]+-s|certbot[[:space:]]+(certonly|renew|delete)|rm[[:space:]]+-rf'; then
    fail "self-test found a forbidden mutating command"
  fi
  printf 'prepare-runtime self-test passed\n'
}

as_root_or_dry_run() {
  if [[ "$APPLY" == "1" && "$(id -u)" -ne 0 ]]; then
    fail "writing production runtime paths requires root; rerun with sudo or set WORKMESH_* paths under a writable directory"
  fi
}

do_run() {
  if [[ "$APPLY" == "1" ]]; then
    "$@"
  else
    printf '[dry-run] '
    printf '%q ' "$@"
    printf '\n'
  fi
}

ensure_dir() {
  local mode=$1
  local path=$2
  if [[ "$APPLY" == "1" ]]; then
    install -d -m "$mode" "$path"
  else
    log "would ensure directory $path mode=$mode"
  fi
}

copy_file_once() {
  local src=$1
  local dst=$2
  local mode=$3
  [[ -f "$src" ]] || fail "missing source file: $src"
  if [[ -e "$dst" && "$FORCE" != "1" ]]; then
    log "preserve existing file: $dst"
    return
  fi
  ensure_dir 0750 "$(dirname "$dst")"
  do_run install -m "$mode" "$src" "$dst"
}

random_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
    return
  fi
  command -v od >/dev/null 2>&1 || fail "missing openssl or od for secret generation"
  od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
  printf '\n'
}

ensure_secret() {
  local name=$1
  local path="$DB_SECRETS_DIR/$name"
  if [[ -s "$path" ]]; then
    log "preserve existing secret: $path"
    if [[ "$APPLY" == "1" ]]; then
      chmod 0600 "$path"
    fi
    return
  fi
  ensure_dir 0700 "$DB_SECRETS_DIR"
  if [[ "$APPLY" == "1" ]]; then
    umask 077
    random_secret >"$path"
    chmod 0600 "$path"
    log "created secret: $path"
  else
    log "would create secret: $path"
  fi
}

write_file_once() {
  local path=$1
  local mode=$2
  local content=$3
  if [[ -e "$path" ]]; then
    log "preserve existing file: $path"
    return
  fi
  ensure_dir 0750 "$(dirname "$path")"
  if [[ "$APPLY" == "1" ]]; then
    umask 027
    printf '%s' "$content" >"$path"
    chmod "$mode" "$path"
    log "created file: $path"
  else
    log "would create file: $path"
  fi
}

write_server_env() {
  local tmp line found_addr=0 found_data=0 found_url=0
  if [[ -n "$DOMAIN" ]]; then
    valid_domain "$DOMAIN" || fail "domain must match <prefix>.cs.sopvip.com: $DOMAIN"
  fi
  if [[ "$APPLY" != "1" ]]; then
    log "would ensure server environment: $SERVER_ENV"
    [[ -z "$DOMAIN" ]] || log "would set WORKMESH_PUBLIC_URL=https://$DOMAIN"
    return
  fi
  ensure_dir 0750 "$(dirname "$SERVER_ENV")"
  tmp="$(mktemp "${SERVER_ENV}.tmp.XXXXXX")"
  if [[ -f "$SERVER_ENV" ]]; then
    while IFS= read -r line || [[ -n "$line" ]]; do
      case "$line" in
        WORKMESH_SERVER_ADDR=*)
          found_addr=1
          printf '%s\n' "$line" >>"$tmp"
          ;;
        WORKMESH_DATA_DIR=*)
          found_data=1
          printf '%s\n' "WORKMESH_DATA_DIR=$DATA_DIR" >>"$tmp"
          ;;
        WORKMESH_PUBLIC_URL=*)
          found_url=1
          if [[ -n "$DOMAIN" ]]; then
            printf '%s\n' "WORKMESH_PUBLIC_URL=https://$DOMAIN" >>"$tmp"
          else
            printf '%s\n' "$line" >>"$tmp"
          fi
          ;;
        *)
          printf '%s\n' "$line" >>"$tmp"
          ;;
      esac
    done <"$SERVER_ENV"
  fi
  if [[ "$found_addr" == "0" ]]; then
    printf '%s\n' "WORKMESH_SERVER_ADDR=0.0.0.0:9999" >>"$tmp"
  fi
  if [[ "$found_data" == "0" ]]; then
    printf '%s\n' "WORKMESH_DATA_DIR=$DATA_DIR" >>"$tmp"
  fi
  if [[ "$found_url" == "0" && -n "$DOMAIN" ]]; then
    printf '%s\n' "WORKMESH_PUBLIC_URL=https://$DOMAIN" >>"$tmp"
  fi
  chmod 0600 "$tmp"
  mv -f "$tmp" "$SERVER_ENV"
  log "updated server environment: $SERVER_ENV"
}

prepare_database() {
  log "preparing database runtime"
  ensure_dir 0750 "$DB_ROOT"
  ensure_dir 0700 "$DB_SECRETS_DIR"
  ensure_secret postgres_password
  ensure_secret redis_password
  ensure_secret mariadb_password
  ensure_secret mariadb_root_password
  copy_file_once "$DB_TEMPLATE" "$DB_COMPOSE" 0644
}

prepare_waf() {
  log "preparing OpenResty/WAF runtime"
  ensure_dir 0750 "$OPENRESTY_DIR"
  ensure_dir 0750 "$OPENRESTY_DIR/conf/conf.d"
  ensure_dir 0750 "$OPENRESTY_DIR/conf/stream.d"
  ensure_dir 0750 "$OPENRESTY_DIR/conf/default"
  ensure_dir 0750 "$OPENRESTY_DIR/conf/ssl"
  ensure_dir 0750 "$OPENRESTY_DIR/waf/data/ip-groups"
  ensure_dir 0750 "$OPENRESTY_DIR/waf/generated"
  ensure_dir 0750 "$OPENRESTY_DIR/waf/logs"
  ensure_dir 0750 "$OPENRESTY_DIR/waf/cache"
  ensure_dir 0750 "$WEBSITE_ROOT"
  copy_file_once "$WAF_COMPOSE_TEMPLATE" "$WAF_COMPOSE" 0644
  write_file_once "$OPENRESTY_DIR/waf/data/global.json" 0640 '{
  "enabled": true,
  "standardRules": true,
  "mode": "observe",
  "requestBodyLimit": 1048576
}
'
  write_file_once "$OPENRESTY_DIR/waf/data/access-lists.json" 0640 '{
  "whitelist": [],
  "blacklist": []
}
'
  write_file_once "$OPENRESTY_DIR/waf/generated/modsecurity-mode.conf" 0640 '# WorkMesh default mode
SecRuleEngine DetectionOnly
'
  write_file_once "$OPENRESTY_DIR/waf/generated/custom-rules.conf" 0640 '# WorkMesh custom rules are generated by the control plane.
'
  write_file_once "$OPENRESTY_DIR/waf/generated/standard-rules.conf" 0640 '# WorkMesh standard CRS rules enabled
Include /etc/workmesh-waf/crs/crs-setup.conf
Include /etc/workmesh-waf/crs/rules/*.conf
'
  write_file_once "$OPENRESTY_DIR/waf/generated/global-rules.conf" 0640 '# WorkMesh global rules are generated by the control plane.
'
}

print_next_steps() {
  log "runtime paths"
  log "ROOT_DIR=$ROOT_DIR"
  log "DATA_DIR=$DATA_DIR"
  log "DB_COMPOSE=$DB_COMPOSE"
  log "OPENRESTY_DIR=$OPENRESTY_DIR"
  log "WEBSITE_ROOT=$WEBSITE_ROOT"
  log "SERVER_ENV=$SERVER_ENV"
  [[ -z "$DOMAIN" ]] || log "DOMAIN=$DOMAIN"
  if [[ "$APPLY" != "1" ]]; then
    log "dry-run complete; rerun with --apply to write files"
    return
  fi
  log "next commands on the deployment host:"
  log "  STRICT=1 WORKMESH_DATA_DIR='$DATA_DIR' WORKMESH_OPENRESTY_DIR='$OPENRESTY_DIR' bash deploy/database/preflight.sh"
  log "  WORKMESH_DB_SECRETS_DIR='$DB_SECRETS_DIR' docker compose -f '$DB_COMPOSE' config --quiet"
  log "  WORKMESH_DB_SECRETS_DIR='$DB_SECRETS_DIR' docker compose -f '$DB_COMPOSE' up -d"
  log "  WORKMESH_OPENRESTY_WAF_IMAGE='<immutable-image-ref>' docker compose -f '$WAF_COMPOSE' config --quiet"
  log "  WORKMESH_OPENRESTY_WAF_IMAGE='<immutable-image-ref>' docker compose -f '$WAF_COMPOSE' up -d"
}

main() {
  parse_args "$@"
  if [[ "$SELF_TEST" == "1" ]]; then
    run_self_test
    return
  fi
  as_root_or_dry_run
  prepare_database
  prepare_waf
  write_server_env
  print_next_steps
}

main "$@"
