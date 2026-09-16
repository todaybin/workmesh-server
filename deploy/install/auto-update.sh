#!/usr/bin/env bash
set -Eeuo pipefail

# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

# This script is intentionally disabled until WORKMESH_AUTO_UPDATE_ENABLED=1
# is set in config/update.env. It only accepts a signed release and delegates
# the final switch and health checks to activate-release.sh.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="${WORKMESH_SERVER_ROOT:-/opt/workmesh-server}"
UNIT_NAME="${WORKMESH_SERVER_UNIT:-workmesh-server.service}"
DATA_DIR="${WORKMESH_DATA_DIR:-$ROOT_DIR/data}"
CONFIG_FILE="${WORKMESH_SERVER_CONFIG:-$ROOT_DIR/config/server.json}"
CURRENT_BINARY="${WORKMESH_SERVER_BINARY:-$ROOT_DIR/bin/workmesh-server}"
ACTIVATOR="${WORKMESH_RELEASE_ACTIVATOR:-$ROOT_DIR/bin/activate-release.sh}"
UPDATE_ENABLED="${WORKMESH_AUTO_UPDATE_ENABLED:-0}"
UPDATE_ENV_FILE="${WORKMESH_UPDATE_ENV_FILE:-$ROOT_DIR/config/update.env}"
ARTIFACT_PATH="${WORKMESH_UPDATE_ARTIFACT_PATH:-}"
ARTIFACT_URL="${WORKMESH_UPDATE_ARTIFACT_URL:-}"
SIGNATURE_PATH="${WORKMESH_UPDATE_SIGNATURE_PATH:-}"
SIGNATURE_URL="${WORKMESH_UPDATE_SIGNATURE_URL:-}"
MANIFEST_PATH="${WORKMESH_UPDATE_MANIFEST_PATH:-}"
MANIFEST_URL="${WORKMESH_UPDATE_MANIFEST_URL:-}"
MANIFEST_SIGNATURE_PATH="${WORKMESH_UPDATE_MANIFEST_SIGNATURE_PATH:-}"
MANIFEST_SIGNATURE_URL="${WORKMESH_UPDATE_MANIFEST_SIGNATURE_URL:-}"
PUBLIC_KEY="${WORKMESH_UPDATE_PUBLIC_KEY:-${WORKMESH_ARTIFACT_PUBLIC_KEY:-}}"
EXPECTED_SHA="${WORKMESH_UPDATE_SHA256:-}"
VERSION="${WORKMESH_UPDATE_VERSION:-}"
CURL_TIMEOUT="${WORKMESH_UPDATE_CURL_TIMEOUT:-60}"
MAX_SIZE="${WORKMESH_UPDATE_MAX_SIZE:-536870912}"
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)-$$"
LOCK_DIR="$ROOT_DIR/.auto-update.lock"
BACKUP_DIR="$ROOT_DIR/backups/auto-update-$RUN_ID"
TEMP_DIR="$DATA_DIR/releases/.auto-update-$RUN_ID"
STATE_FILE="$DATA_DIR/auto-update-state.env"
STOPPED=0
artifact=""
signature=""
candidate=""
manifest=""
manifest_signature=""

log() {
  local message
  message="[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*"
  printf '[workmesh-auto-update] %s\n' "$*"
  if [[ -d "$ROOT_DIR/logs" ]]; then
    printf '%s\n' "$message" >>"$ROOT_DIR/logs/auto-update.log" || true
  fi
}

fail() {
  log "failed: $*"
  return 1
}

require_root_file() {
  local path="$1"
  [[ -f "$path" && ! -L "$path" ]] ||
    { fail "file must be a regular non-symlink file: $path"; return 1; }
  [[ "$(stat -c '%u' "$path")" == "0" ]] ||
    { fail "file must be owned by root: $path"; return 1; }
  local mode
  mode="$(stat -c '%a' "$path")"
  (( (8#$mode & 18) == 0 )) ||
    { fail "file must not be group/world writable: $path"; return 1; }
}

usage() {
  cat <<'USAGE'
Run one signed WorkMesh production update.

Configuration is read from config/update.env by systemd:
  WORKMESH_AUTO_UPDATE_ENABLED=1
  WORKMESH_UPDATE_ARTIFACT_URL=https://updates.example.invalid/workmesh-server
  WORKMESH_UPDATE_SIGNATURE_URL=https://updates.example.invalid/workmesh-server.sig
  WORKMESH_UPDATE_PUBLIC_KEY=<Ed25519 raw/base64/hex key or file>
  WORKMESH_UPDATE_SHA256=<optional expected SHA-256>
  WORKMESH_UPDATE_VERSION=<optional release version>

For a local release, use WORKMESH_UPDATE_ARTIFACT_PATH and
WORKMESH_UPDATE_SIGNATURE_PATH instead of URLs.

For a signed release manifest, configure:
  WORKMESH_UPDATE_MANIFEST_URL=https://updates.example.invalid/release.json
  WORKMESH_UPDATE_MANIFEST_SIGNATURE_URL=https://updates.example.invalid/release.json.sig
The manifest must contain version, os, arch, size, sha256, artifact_url and
signature_url. Explicit artifact/signature paths override manifest URLs.
USAGE
}

cleanup() {
  local status=$?
  if [[ "$status" != "0" && -d "$TEMP_DIR" ]]; then
    install -d -m 0750 "$BACKUP_DIR" 2>/dev/null || true
    for evidence in "$artifact" "$signature" "$candidate" "$candidate.sha256" "$manifest" "$manifest_signature"; do
      if [[ -f "$evidence" ]]; then
        cp -a "$evidence" "$BACKUP_DIR/" 2>/dev/null || true
      fi
    done
    printf 'status=%s\n' "$status" >"$BACKUP_DIR/failure.txt" 2>/dev/null || true
  fi
  if [[ "$status" != "0" && "$STOPPED" == "1" ]] &&
    command -v systemctl >/dev/null 2>&1; then
    log "update failed; restoring service state"
    if ! systemctl start "$UNIT_NAME"; then
      log "failed to start $UNIT_NAME after update failure"
    fi
  fi
  rm -rf "$TEMP_DIR" 2>/dev/null || true
  rmdir "$LOCK_DIR" 2>/dev/null || true
  exit "$status"
}
trap cleanup EXIT

while [[ $# -gt 0 ]]; do
  case "$1" in
    --help|-h)
      usage
      exit 0
      ;;
    *)
      printf 'unknown argument: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

[[ "$(id -u)" -eq 0 ]] || { fail 'run as root on the production host'; exit 1; }
[[ "$ROOT_DIR" == /* && "$DATA_DIR" == /* ]] || {
  fail 'WORKMESH_SERVER_ROOT and WORKMESH_DATA_DIR must be absolute paths'
  exit 1
}
[[ "$MAX_SIZE" =~ ^[1-9][0-9]*$ ]] ||
  { fail 'WORKMESH_UPDATE_MAX_SIZE must be a positive integer'; exit 1; }
[[ "$CURL_TIMEOUT" =~ ^[1-9][0-9]*$ ]] ||
  { fail 'WORKMESH_UPDATE_CURL_TIMEOUT must be a positive integer'; exit 1; }

if [[ "$UPDATE_ENABLED" != "1" ]]; then
  log 'automatic updates are disabled; set WORKMESH_AUTO_UPDATE_ENABLED=1 to enable'
  exit 0
fi

command -v systemctl >/dev/null 2>&1 || { fail 'systemctl is unavailable'; exit 1; }
command -v sha256sum >/dev/null 2>&1 || { fail 'sha256sum is unavailable'; exit 1; }
[[ -x "$CURRENT_BINARY" ]] || { fail "installed binary is missing: $CURRENT_BINARY"; exit 1; }
[[ -x "$ACTIVATOR" ]] || { fail "release activator is missing: $ACTIVATOR"; exit 1; }
[[ -d "$DATA_DIR" ]] || { fail "data directory is missing: $DATA_DIR"; exit 1; }
[[ -n "$PUBLIC_KEY" ]] || { fail 'WORKMESH_UPDATE_PUBLIC_KEY is required'; exit 1; }
if [[ -e "$PUBLIC_KEY" ]]; then
  require_root_file "$PUBLIC_KEY" || exit 1
fi
if [[ -e "$UPDATE_ENV_FILE" ]]; then
  require_root_file "$UPDATE_ENV_FILE" || exit 1
fi
[[ -n "$MANIFEST_PATH" || -n "$MANIFEST_URL" || -n "$ARTIFACT_PATH" || -n "$ARTIFACT_URL" ]] || {
  fail 'configure a signed manifest or artifact source'
  exit 1
}
if [[ -n "$MANIFEST_PATH" || -n "$MANIFEST_URL" ]]; then
  [[ -n "$MANIFEST_SIGNATURE_PATH" || -n "$MANIFEST_SIGNATURE_URL" ]] || {
    fail 'a manifest signature source is required'
    exit 1
  }
else
  [[ -n "$SIGNATURE_PATH" || -n "$SIGNATURE_URL" ]] || {
    fail 'configure WORKMESH_UPDATE_SIGNATURE_PATH or WORKMESH_UPDATE_SIGNATURE_URL'
    exit 1
  }
fi

if ! mkdir "$LOCK_DIR" 2>/dev/null; then
  fail "another automatic update is already running: $LOCK_DIR"
  exit 1
fi
install -d -m 0750 "$TEMP_DIR" "$BACKUP_DIR" "$ROOT_DIR/logs"

artifact="$TEMP_DIR/workmesh-server.candidate"
signature="$TEMP_DIR/workmesh-server.candidate.sig"
manifest="$TEMP_DIR/release-manifest.json"
manifest_signature="$TEMP_DIR/release-manifest.json.sig"

download_source() {
  local source="$1" destination="$2" max_size="$3"
  if [[ "$source" == /* ]]; then
    [[ -f "$source" && ! -L "$source" ]] ||
      { fail "source is not a regular file: $source"; return 1; }
    [[ "$(stat -c '%s' "$source")" -le "$max_size" ]] ||
      { fail "source exceeds maximum size: $source"; return 1; }
    cp -- "$source" "$destination"
    return
  fi
  [[ "$source" == https://* ]] ||
    { fail 'remote update sources must use HTTPS'; return 1; }
  command -v curl >/dev/null 2>&1 || { fail 'curl is unavailable'; return 1; }
  curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
    --connect-timeout 10 --max-time "$CURL_TIMEOUT" \
    --max-filesize "$max_size" "$source" -o "$destination"
}

if [[ -n "$MANIFEST_PATH" || -n "$MANIFEST_URL" ]]; then
  command -v jq >/dev/null 2>&1 || { fail 'jq is required for signed manifests'; exit 1; }
  download_source "${MANIFEST_PATH:-$MANIFEST_URL}" "$manifest" 1048576
  download_source "${MANIFEST_SIGNATURE_PATH:-$MANIFEST_SIGNATURE_URL}" \
    "$manifest_signature" 8192
  manifest_sha="$(sha256sum "$manifest" | awk '{print $1}')"
  verified_manifest="$DATA_DIR/releases/verified-manifest-$RUN_ID"
  manifest_args=(update "$manifest" --signature "$manifest_signature" \
    --public-key "$PUBLIC_KEY" --target "$verified_manifest" --sha256 "$manifest_sha")
  if ! env WORKMESH_DATA_DIR="$DATA_DIR" WORKMESH_SERVER_CONFIG="$CONFIG_FILE" \
    "$CURRENT_BINARY" "${manifest_args[@]}"; then
    fail 'release manifest signature verification failed'
    exit 1
  fi
  VERSION="$(jq -er '.version | strings | select(test("^v[0-9]+\\.[0-9]+([.][0-9]+)?$"))' "$verified_manifest")" ||
    { fail 'manifest version is invalid'; exit 1; }
  EXPECTED_SHA="$(jq -er '.sha256 | strings | ascii_downcase | select(test("^[0-9a-f]{64}$"))' "$verified_manifest")" ||
    { fail 'manifest sha256 is invalid'; exit 1; }
  manifest_size="$(jq -er '.size | numbers | tostring | select(test("^[0-9]+$"))' "$verified_manifest")" ||
    { fail 'manifest size is invalid'; exit 1; }
  (( manifest_size > 0 && manifest_size <= MAX_SIZE )) ||
    { fail 'manifest size is outside the allowed range'; exit 1; }
  manifest_os="$(jq -er '.os | strings' "$verified_manifest")" ||
    { fail 'manifest os is missing'; exit 1; }
  manifest_arch="$(jq -er '.arch | strings' "$verified_manifest")" ||
    { fail 'manifest arch is missing'; exit 1; }
  [[ "$manifest_os" == "linux" && "$manifest_arch" == "amd64" ]] ||
    { fail "unsupported manifest target: $manifest_os/$manifest_arch"; exit 1; }
  if [[ -z "$ARTIFACT_PATH" && -z "$ARTIFACT_URL" ]]; then
    ARTIFACT_URL="$(jq -er '.artifact_url | strings | select(startswith("https://"))' "$verified_manifest")" ||
      { fail 'manifest artifact_url must use HTTPS'; exit 1; }
  fi
  if [[ -z "$SIGNATURE_PATH" && -z "$SIGNATURE_URL" ]]; then
    SIGNATURE_URL="$(jq -er '.signature_url | strings | select(startswith("https://"))' "$verified_manifest")" ||
      { fail 'manifest signature_url must use HTTPS'; exit 1; }
  fi
fi

if [[ -n "$ARTIFACT_PATH" ]]; then
  download_source "$ARTIFACT_PATH" "$artifact" "$MAX_SIZE"
else
  download_source "$ARTIFACT_URL" "$artifact" "$MAX_SIZE"
fi

if [[ -n "$SIGNATURE_PATH" ]]; then
  download_source "$SIGNATURE_PATH" "$signature" 8192
else
  download_source "$SIGNATURE_URL" "$signature" 8192
fi

artifact_sha="$(sha256sum "$artifact" | awk '{print $1}')"
if [[ -n "${manifest_size:-}" && "$(stat -c '%s' "$artifact")" != "$manifest_size" ]]; then
  fail "artifact size differs from signed manifest: expected $manifest_size"
  exit 1
fi
if [[ -n "$EXPECTED_SHA" ]] &&
  ! printf '%s  %s\n' "$EXPECTED_SHA" "$artifact" | sha256sum --check --status -; then
  fail "artifact SHA-256 mismatch: expected $EXPECTED_SHA, got $artifact_sha"
  exit 1
fi

# The existing CLI owns Ed25519 decoding and verification. It writes a
# verified candidate inside DATA_DIR, while the activator alone may replace
# the systemd executable.
candidate="$DATA_DIR/releases/verified-$RUN_ID"
mkdir -p "$(dirname "$candidate")"
cli_args=(update "$artifact" --signature "$signature" --public-key "$PUBLIC_KEY" --target "$candidate")
[[ -n "$EXPECTED_SHA" ]] && cli_args+=(--sha256 "$EXPECTED_SHA")
[[ -n "$VERSION" ]] && cli_args+=(--version "$VERSION")
if ! env WORKMESH_DATA_DIR="$DATA_DIR" WORKMESH_SERVER_CONFIG="$CONFIG_FILE" \
  "$CURRENT_BINARY" "${cli_args[@]}"; then
  fail 'installed CLI rejected the candidate release'
  exit 1
fi
candidate_sha="$(sha256sum "$candidate" | awk '{print $1}')"
[[ "$candidate_sha" == "$artifact_sha" ]] || {
  fail "verified candidate changed during staging: $candidate_sha != $artifact_sha"
  exit 1
}
[[ -x "$candidate" ]] || chmod 0750 "$candidate"
printf '%s  %s\n' "$candidate_sha" "$candidate" >"$candidate.sha256"

current_sha="$(sha256sum "$CURRENT_BINARY" | awk '{print $1}')"
if [[ "$current_sha" == "$candidate_sha" ]]; then
  log "no update required; running SHA-256 is already $current_sha"
  rm -f "$candidate" "$candidate.sha256"
  exit 0
fi

if [[ -n "$VERSION" && -f "$STATE_FILE" ]]; then
  state_version="$(awk -F= '$1 == "version" {print $2; exit}' "$STATE_FILE")"
  state_sha="$(awk -F= '$1 == "sha256" {print $2; exit}' "$STATE_FILE")"
  if [[ -n "$state_version" && "$VERSION" != "$state_version" ]]; then
    oldest_version="$(printf '%s\n%s\n' "$VERSION" "$state_version" | sort -V | head -1)"
    [[ "$oldest_version" == "$state_version" ]] ||
      { fail "refusing version downgrade: installed=$state_version candidate=$VERSION"; exit 1; }
  elif [[ -n "$state_version" && "$VERSION" == "$state_version" && "$state_sha" != "$current_sha" ]]; then
    log "same release label has a different binary; accepting because the signed digest changed"
  fi
fi

if ! systemctl is-active --quiet "$UNIT_NAME"; then
  fail "$UNIT_NAME is not active; refusing an unattended update"
  exit 1
fi

printf 'run_id=%s\nversion=%s\ncandidate_sha256=%s\nprevious_sha256=%s\n' \
  "$RUN_ID" "$VERSION" "$candidate_sha" "$current_sha" >"$BACKUP_DIR/manifest.txt"
for path in \
  "$ROOT_DIR/config/server.json" \
  "$ROOT_DIR/config/server.env" \
  "$DATA_DIR/workmesh.db" \
  "$DATA_DIR/workmesh.db-wal" \
  "$DATA_DIR/workmesh.db-shm"; do
  if [[ -e "$path" ]]; then
    cp -a -- "$path" "$BACKUP_DIR/"
  fi
done
sync

log "stopping $UNIT_NAME for update"
systemctl stop "$UNIT_NAME"
STOPPED=1
for _ in {1..30}; do
  if ! systemctl is-active --quiet "$UNIT_NAME"; then
    break
  fi
  sleep 1
done
systemctl is-active --quiet "$UNIT_NAME" &&
  { fail "$UNIT_NAME did not stop"; exit 1; }

if ! env \
  WORKMESH_SERVER_ROOT="$ROOT_DIR" \
  WORKMESH_SERVER_BINARY="$candidate" \
  WORKMESH_SERVER_SHA_FILE="$candidate.sha256" \
  "$ACTIVATOR"; then
  fail 'release activation failed; previous binary was restored when possible'
  exit 1
fi

STOPPED=0
if ! cp -a -- "$candidate" "$BACKUP_DIR/verified-candidate"; then
  log 'warning: could not copy verified candidate into the audit backup'
else
  printf '%s  %s\n' "$candidate_sha" "$BACKUP_DIR/verified-candidate" \
    >"$BACKUP_DIR/verified-candidate.sha256" || true
fi
if [[ -n "$VERSION" ]]; then
  state_tmp="$STATE_FILE.$RUN_ID.tmp"
  if printf 'version=%s\nsha256=%s\nupdated_at=%s\n' \
    "$VERSION" "$candidate_sha" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$state_tmp" &&
    chmod 0600 "$state_tmp" &&
    mv -f "$state_tmp" "$STATE_FILE"; then
    :
  else
    log "warning: could not persist update state: $STATE_FILE"
    rm -f "$state_tmp" 2>/dev/null || true
  fi
fi
rm -f "$candidate" "$candidate.sha256" 2>/dev/null || true
log "automatic update completed: sha256=$candidate_sha version=${VERSION:-unknown}"
exit 0
