#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ACCEPTANCE_DIR="$(cd "$SCRIPT_DIR/../acceptance" && pwd)"
# shellcheck source=../acceptance/host-ip.sh
source "$ACCEPTANCE_DIR/host-ip.sh"

DEFAULT_DOMAIN="workmesh.cs.sopvip.com"
DEFAULT_PUBLIC_IP="61.184.12.165"
DOMAIN="${WORKMESH_ACCEPTANCE_DOMAIN:-${DOMAIN:-$DEFAULT_DOMAIN}}"
PUBLIC_IP="${WORKMESH_PUBLIC_IP:-${ADDRESS:-$DEFAULT_PUBLIC_IP}}"
SELF_TEST=0

log() {
  printf '[domain-plan] %s\n' "$*"
}

fail() {
  printf '[domain-plan] FAIL: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'USAGE'
Print a read-only deployment plan for one WorkMesh acceptance domain.

Usage:
  deploy/install/domain-plan.sh [--domain NAME] [--public-ip IP]
  deploy/install/domain-plan.sh --self-test

Defaults:
  Domain:    workmesh.cs.sopvip.com
  Public IP: WORKMESH_PUBLIC_IP/ADDRESS, otherwise 61.184.12.165

Safety:
  This command only validates local input and prints DNS/acceptance commands.
  It does not write DNS, website files, certificates, OpenResty config,
  containers, or system services.
USAGE
}

valid_domain() {
  [[ "$1" =~ ^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.cs\.sopvip\.com$ ]]
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

run_self_test() {
  bash -n "${BASH_SOURCE[0]}"
  bash -n "$ACCEPTANCE_DIR/host-ip.sh"
  valid_domain workmesh.cs.sopvip.com
  valid_domain edge-01.cs.sopvip.com
  ! valid_domain cs.sopvip.com
  ! valid_domain WorkMesh.cs.sopvip.com
  is_ipv4 61.184.12.165
  ! is_ipv4 127.0.0.1
  ! is_ipv4 999.1.1.1
  local scan
  scan="$(sed '/^run_self_test()/,/^}/d' "${BASH_SOURCE[0]}")"
  if printf '%s\n' "$scan" | grep -Eiq \
    'docker[[:space:]]+(compose[[:space:]]+)?(up|down|restart|rm|rmi|stop|kill|volume[[:space:]]+rm|network[[:space:]]+rm|system[[:space:]]+prune)|systemctl[[:space:]]|nginx[[:space:]]+-s|openresty[[:space:]]+-s|certbot[[:space:]]+(certonly|renew|delete)|rm[[:space:]]+-rf'; then
    fail "self-test found a forbidden mutating command"
  fi
  printf 'domain-plan self-test passed\n'
}

main() {
  parse_args "$@"
  if [[ "$SELF_TEST" == "1" ]]; then
    run_self_test
    return
  fi
  valid_domain "$DOMAIN" || fail "domain must match <prefix>.cs.sopvip.com: $DOMAIN"
  if [[ -z "$PUBLIC_IP" || "$PUBLIC_IP" == "auto" ]]; then
    PUBLIC_IP="$(detect_host_ipv4 || true)"
  fi
  is_ipv4 "$PUBLIC_IP" || fail "public IPv4 is unavailable or invalid; pass --public-ip or WORKMESH_PUBLIC_IP"

  log "domain=$DOMAIN"
  log "host_ipv4=$PUBLIC_IP"
  log "DNS A record: ${DOMAIN%.cs.sopvip.com} IN A $PUBLIC_IP"
  log "public URL: https://$DOMAIN"
  log "DNS verification: dig +short A '$DOMAIN'"
  log "preflight:"
  log "  STRICT=1 WORKMESH_ACCEPTANCE_DOMAIN='$DOMAIN' WORKMESH_PUBLIC_IP='$PUBLIC_IP' bash deploy/acceptance/preflight.sh"
  log "domain acceptance:"
  log "  bash deploy/acceptance/domain-acceptance.sh --domain '$DOMAIN' --public-ip '$PUBLIC_IP' --check-container --strict"
}

main "$@"
