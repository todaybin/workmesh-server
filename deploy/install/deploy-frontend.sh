#!/usr/bin/env bash
set -Eeuo pipefail

# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

# Frontend deployment is deliberately separate from install.sh so a static
# asset rollout can be backed up and rolled back without touching the service.
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

SOURCE_DIR="${WORKMESH_FRONTEND_DIST:-$REPO_ROOT/web/dist}"
TARGET_DIR="${WORKMESH_FRONTEND_TARGET:-/opt/workmesh-server/web/dist}"
BACKUP_DIR="${WORKMESH_FRONTEND_BACKUP_DIR:-$(dirname "$TARGET_DIR")/dist-backups}"
APPLY=0
SELF_TEST=0

log() {
  printf '[deploy-frontend] %s\n' "$*"
}

fail() {
  printf '[deploy-frontend] FAIL: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'USAGE'
Deploy the WorkMesh frontend dist directory with an atomic backup/switch.

Usage:
  deploy/install/deploy-frontend.sh
  deploy/install/deploy-frontend.sh --apply
  deploy/install/deploy-frontend.sh --self-test

Options:
  --apply       Back up the current target and atomically install the source.
  --self-test   Validate this script without touching production paths.
  --help        Show this help.

Environment:
  WORKMESH_FRONTEND_DIST
      Source dist directory. Default: <repository>/web/dist.
  WORKMESH_FRONTEND_TARGET
      Production dist directory. Default: /opt/workmesh-server/web/dist.
  WORKMESH_FRONTEND_BACKUP_DIR
      Backup parent directory. Default: <target-parent>/dist-backups.

Safety:
  Without --apply the script only validates and prints the planned switch.
  The apply path requires root, never deletes the existing frontend, and does
  not restart systemd, change DNS, or modify databases, containers, or WAF data.
USAGE
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --apply)
        APPLY=1
        shift
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

validate_absolute_path() {
  local name=$1
  local value=$2
  [[ "$value" == /* ]] || fail "$name must be an absolute path: $value"
  [[ "$value" != "/" ]] || fail "$name must not be /"
  [[ "$value" != *[[:space:]]* ]] || fail "$name must not contain whitespace"
}

validate_source() {
  [[ -d "$SOURCE_DIR" ]] || fail "frontend dist directory is missing: $SOURCE_DIR"
  [[ -f "$SOURCE_DIR/index.html" ]] || fail "frontend index.html is missing: $SOURCE_DIR/index.html"
  [[ -d "$SOURCE_DIR/assets" ]] || fail "frontend assets directory is missing: $SOURCE_DIR/assets"

  local entry
  entry="$(sed -nE 's#^[[:space:]]*<script[^>]+src="([^"]+)"[^>]*></script>[[:space:]]*$#\1#p' \
    "$SOURCE_DIR/index.html" | head -n1)"
  [[ -n "$entry" ]] || fail "index.html has no module entry script"
  [[ "$entry" == /* ]] || fail "index.html entry script must use an absolute asset path: $entry"
  [[ -f "$SOURCE_DIR${entry}" ]] || fail "index.html entry script is missing: $SOURCE_DIR${entry}"

  local waf_chunks
  waf_chunks="$(find "$SOURCE_DIR/assets/js" -maxdepth 1 -type f \
    \( -name 'waf-*.js' -o -name 'overview-*.js' -o -name 'attack-*.js' \
       -o -name 'intercept-*.js' -o -name 'block-*.js' -o -name 'blackwhite-*.js' \
       -o -name 'websites-*.js' -o -name 'global-*.js' \) -print)"
  [[ "$(printf '%s\n' "$waf_chunks" | sed '/^$/d' | wc -l)" -ge 8 ]] ||
    fail "frontend dist does not contain the WAF shell and seven page chunks"
}

hash_file() {
  sha256sum "$1" | awk '{print $1}'
}

run_self_test() {
  bash -n "${BASH_SOURCE[0]}"
  grep -Fq 'mv "$TARGET_DIR" "$backup_path"' "${BASH_SOURCE[0]}" ||
    fail "self-test could not find the target backup switch"
  grep -Fq 'mv "$staging_dir" "$TARGET_DIR"' "${BASH_SOURCE[0]}" ||
    fail "self-test could not find the atomic install switch"
  printf 'deploy-frontend self-test passed\n'
}

deploy() {
  local target_parent staging_dir backup_path source_hash
  target_parent="$(dirname "$TARGET_DIR")"
  source_hash="$(hash_file "$SOURCE_DIR/index.html")"

  if [[ "$APPLY" != "1" ]]; then
    log "dry-run: source=$SOURCE_DIR"
    log "dry-run: target=$TARGET_DIR"
    log "dry-run: backup-parent=$BACKUP_DIR"
    log "candidate index.html sha256=$source_hash"
    return
  fi

  [[ "$(id -u)" -eq 0 ]] || fail 'applying a production frontend deployment requires root'
  validate_absolute_path WORKMESH_FRONTEND_DIST "$SOURCE_DIR"
  validate_absolute_path WORKMESH_FRONTEND_TARGET "$TARGET_DIR"
  validate_absolute_path WORKMESH_FRONTEND_BACKUP_DIR "$BACKUP_DIR"
  install -d -m 0750 "$target_parent" "$BACKUP_DIR"

  staging_dir="$(mktemp -d "$target_parent/.dist.new.XXXXXX")"
  backup_path="$BACKUP_DIR/dist-$(date +%Y%m%d-%H%M%S)-${source_hash:0:12}"
  cleanup() {
    rm -rf "$staging_dir"
  }
  trap cleanup EXIT INT TERM

  cp -a "$SOURCE_DIR"/. "$staging_dir"/
  validate_source "$staging_dir"

  if [[ -e "$TARGET_DIR" ]]; then
    [[ -d "$TARGET_DIR" ]] || fail "frontend target is not a directory: $TARGET_DIR"
    [[ ! -e "$backup_path" ]] || fail "backup path already exists: $backup_path"
    mv "$TARGET_DIR" "$backup_path"
  fi

  if ! mv "$staging_dir" "$TARGET_DIR"; then
    if [[ -e "$backup_path" && ! -e "$TARGET_DIR" ]]; then
      mv "$backup_path" "$TARGET_DIR"
    fi
    fail "atomic frontend switch failed"
  fi
  trap - EXIT INT TERM

  log "frontend deployed: $TARGET_DIR"
  log "backup: ${backup_path:-none}"
  log "index.html sha256=$(hash_file "$TARGET_DIR/index.html")"
}

parse_args "$@"
if [[ "$SELF_TEST" == "1" ]]; then
  run_self_test
  exit 0
fi

validate_absolute_path WORKMESH_FRONTEND_DIST "$SOURCE_DIR"
validate_absolute_path WORKMESH_FRONTEND_TARGET "$TARGET_DIR"
validate_absolute_path WORKMESH_FRONTEND_BACKUP_DIR "$BACKUP_DIR"
validate_source
deploy
