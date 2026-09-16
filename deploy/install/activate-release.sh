#!/usr/bin/env bash
set -Eeuo pipefail

# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

ROOT_DIR="${WORKMESH_SERVER_ROOT:-/opt/workmesh-server}"
UNIT_NAME="${WORKMESH_SERVER_UNIT:-workmesh-server.service}"
if [[ -n "${WORKMESH_SERVER_BINARY:-}" ]]; then
  BINARY="$WORKMESH_SERVER_BINARY"
elif [[ -x "$REPO_ROOT/release/workmesh-server-linux-amd64" ]]; then
  BINARY="$REPO_ROOT/release/workmesh-server-linux-amd64"
elif [[ -x "/www/apps/workmesh-server/release/workmesh-server-linux-amd64" ]]; then
  BINARY="/www/apps/workmesh-server/release/workmesh-server-linux-amd64"
else
  BINARY="$REPO_ROOT/release/workmesh-server-linux-amd64"
fi
SHA_FILE="${WORKMESH_SERVER_SHA_FILE:-$BINARY.sha256}"
ADDR="${WORKMESH_SERVER_LOCAL_ADDR:-http://127.0.0.1:9999}"
DATA_DIR="${WORKMESH_DATA_DIR:-$ROOT_DIR/data}"
CURL_TIMEOUT="${WORKMESH_DEPLOY_CURL_TIMEOUT:-10}"
PORT_WAIT_SECONDS="${WORKMESH_DEPLOY_PORT_WAIT_SECONDS:-15}"
SKIP_RESTART="${WORKMESH_DEPLOY_SKIP_RESTART:-0}"
SKIP_HTTP_CHECK="${WORKMESH_DEPLOY_SKIP_HTTP_CHECK:-0}"
SKIP_PROCESS_CHECK="${WORKMESH_DEPLOY_SKIP_PROCESS_CHECK:-0}"
SKIP_PORT_CHECK="${WORKMESH_DEPLOY_SKIP_PORT_CHECK:-0}"
LOCK_DIR=""
REPLACED=0
ROLLBACK_DONE=0
SERVICE_TOUCHED=0
SERVICE_WAS_ACTIVE=0
COOKIE_JAR=""

log() { printf '[activate-release] %s\n' "$*"; }
fail() { printf '[activate-release] failed: %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'USAGE'
Activate a built WorkMesh single-binary release on the production host.

Usage:
  deploy/install/activate-release.sh [options]

Options:
  --root DIR         Install root. Default: WORKMESH_SERVER_ROOT or /opt/workmesh-server.
  --binary PATH      Release binary. Default: release/workmesh-server-linux-amd64.
  --sha-file PATH    SHA-256 file. Default: <binary>.sha256.
  --addr URL         Local base URL for health checks. Default: http://127.0.0.1:9999.
  --skip-restart     Replace the binary but do not restart systemd.
  --skip-http-check  Do not run curl checks after restart.
  --skip-process-check
                     Do not verify systemd MainPID executable after restart.
  --skip-port-check  Do not verify that MainPID owns port 9999.
  --help             Show this help.

Environment variables mirror the option names:
  WORKMESH_SERVER_ROOT, WORKMESH_SERVER_BINARY, WORKMESH_SERVER_SHA_FILE,
  WORKMESH_SERVER_LOCAL_ADDR, WORKMESH_SERVER_UNIT,
  WORKMESH_DEPLOY_SKIP_RESTART, WORKMESH_DEPLOY_SKIP_HTTP_CHECK,
  WORKMESH_DEPLOY_SKIP_PROCESS_CHECK, WORKMESH_DEPLOY_SKIP_PORT_CHECK,
  WORKMESH_DEPLOY_PORT_WAIT_SECONDS.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --root)
      ROOT_DIR="${2:?missing value for --root}"
      shift 2
      ;;
    --binary)
      BINARY="${2:?missing value for --binary}"
      shift 2
      ;;
    --sha-file)
      SHA_FILE="${2:?missing value for --sha-file}"
      shift 2
      ;;
    --addr)
      ADDR="${2:?missing value for --addr}"
      shift 2
      ;;
    --skip-restart)
      SKIP_RESTART=1
      shift
      ;;
    --skip-http-check)
      SKIP_HTTP_CHECK=1
      shift
      ;;
    --skip-process-check)
      SKIP_PROCESS_CHECK=1
      shift
      ;;
    --skip-port-check)
      SKIP_PORT_CHECK=1
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

[[ "$(id -u)" -eq 0 ]] || fail 'run as root on the production host'
[[ "$ROOT_DIR" == /* ]] || fail "root must be absolute: $ROOT_DIR"
[[ "$PORT_WAIT_SECONDS" =~ ^[1-9][0-9]*$ ]] || fail "port wait must be a positive integer: $PORT_WAIT_SECONDS"
[[ -x "$BINARY" ]] || fail "release binary is missing or not executable: $BINARY"
[[ -s "$SHA_FILE" ]] || fail "release checksum file is missing: $SHA_FILE"
[[ -d "$ROOT_DIR" ]] || fail "install root does not exist: $ROOT_DIR"
[[ -d "$ROOT_DIR/bin" ]] || fail "install bin directory does not exist: $ROOT_DIR/bin"
[[ -w "$ROOT_DIR/bin" ]] || fail "install bin directory is not writable: $ROOT_DIR/bin"
[[ -w "$ROOT_DIR" ]] || fail "install root is not writable: $ROOT_DIR"
if [[ "$SKIP_RESTART" == "1" && "$SKIP_HTTP_CHECK" != "1" ]]; then
  fail '--skip-restart requires --skip-http-check because the candidate is not running'
fi

# Validate the service contract before touching the installed executable. This
# keeps a bad unit configuration from leaving a new binary on disk.
if [[ "$SKIP_RESTART" != "1" ]]; then
  command -v systemctl >/dev/null 2>&1 || fail 'systemctl is unavailable'
  exec_start="$(systemctl show "$UNIT_NAME" -p ExecStart --value 2>/dev/null || true)"
  [[ "$exec_start" == *"$ROOT_DIR/bin/workmesh-server"* ]] ||
    fail "$UNIT_NAME ExecStart does not reference $ROOT_DIR/bin/workmesh-server: ${exec_start:-empty}"
  if systemctl is-active --quiet "$UNIT_NAME"; then
    SERVICE_WAS_ACTIVE=1
  fi
fi

LOCK_DIR="$ROOT_DIR/.activate-release.lock"
if ! mkdir "$LOCK_DIR" 2>/dev/null; then
  fail "another release activation is already running or lock remains: $LOCK_DIR"
fi
trap 'rmdir "$LOCK_DIR" 2>/dev/null || true' EXIT

expected_sha="$(awk '{print $1; exit}' "$SHA_FILE")"
actual_sha="$(sha256sum "$BINARY" | awk '{print $1}')"
[[ "$actual_sha" == "$expected_sha" ]] ||
  fail "release checksum mismatch: expected $expected_sha, got $actual_sha"

target="$ROOT_DIR/bin/workmesh-server"
old_sha=""
backup=""
target_was_absent=1
if [[ -e "$target" ]]; then
  [[ -f "$target" ]] || fail "target is not a regular file: $target"
  target_was_absent=0
  old_sha="$(sha256sum "$target" | awk '{print $1}')"
  backup="$ROOT_DIR/bin/workmesh-server.bak.$(date +%Y%m%d%H%M%S)-$$"
  cp -a "$target" "$backup"
  log "backup created: $backup"
fi

staged_target="$ROOT_DIR/bin/.workmesh-server.$$.new"
install -m 0750 "$BINARY" "$staged_target"
mv -f "$staged_target" "$target"
REPLACED=1

rollback_internal() {
  local reason="$1"
  local rollback_target=""
  local rollback_failed=0
  ROLLBACK_DONE=1

  if [[ -n "$backup" && -f "$backup" ]]; then
    rollback_target="$ROOT_DIR/bin/.workmesh-server.$$.rollback"
    if ! install -m 0750 "$backup" "$rollback_target" || ! mv -f "$rollback_target" "$target"; then
      rollback_failed=1
      rm -f "$rollback_target" 2>/dev/null || true
    fi
  elif [[ "$target_was_absent" == "1" ]]; then
    if ! rm -f "$target"; then
      rollback_failed=1
    fi
  else
    rollback_failed=1
  fi

  if [[ "$rollback_failed" == "0" && -n "$old_sha" && -f "$target" ]]; then
    restored_sha="$(sha256sum "$target" 2>/dev/null | awk '{print $1}' || true)"
    if [[ "$restored_sha" != "$old_sha" ]]; then
      rollback_failed=1
      log "rollback checksum mismatch: expected $old_sha, got ${restored_sha:-empty}"
    fi
  fi

  if [[ "$SERVICE_TOUCHED" == "1" ]] && command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || rollback_failed=1
    if [[ "$SERVICE_WAS_ACTIVE" == "1" ]]; then
      systemctl restart "$UNIT_NAME" >/dev/null 2>&1 || rollback_failed=1
    else
      systemctl stop "$UNIT_NAME" >/dev/null 2>&1 || true
    fi
  fi

  if [[ "$rollback_failed" == "0" ]]; then
    log "rollback restored previous release after: $reason"
  else
    log "rollback FAILED after: $reason; manual intervention required"
  fi
  return "$rollback_failed"
}

rollback_and_fail() {
  local reason=$1
  if rollback_internal "$reason"; then
    fail "$reason; previous release restored"
  fi
  fail "$reason; previous release could not be restored"
}

# Catch unexpected failures or signals after the atomic rename. Expected
# failures call rollback_and_fail directly and mark the rollback complete.
on_exit() {
  local status=$?
  if [[ "$status" != "0" && "$REPLACED" == "1" && "$ROLLBACK_DONE" != "1" ]]; then
    rollback_internal "unexpected activation failure"
  fi
  if [[ -n "$COOKIE_JAR" ]]; then
    rm -f "$COOKIE_JAR" 2>/dev/null || true
  fi
  rmdir "$LOCK_DIR" 2>/dev/null || true
  exit "$status"
}
trap on_exit EXIT

new_sha="$(sha256sum "$target" | awk '{print $1}')"
expected_installed_sha="$(sha256sum "$BINARY" | awk '{print $1}')"
[[ "$new_sha" == "$expected_installed_sha" ]] ||
  rollback_and_fail "installed checksum mismatch: expected $expected_installed_sha, got $new_sha"

log "installed binary: $target"
log "old sha256: ${old_sha:-none}"
log "new sha256: $new_sha"

if [[ "$SKIP_RESTART" != "1" ]]; then
  if ! systemctl daemon-reload; then
    rollback_and_fail "$UNIT_NAME daemon-reload failed"
  fi
  SERVICE_TOUCHED=1
  if ! systemctl restart "$UNIT_NAME"; then
    rollback_and_fail "$UNIT_NAME restart failed"
  fi
  if ! systemctl is-active --quiet "$UNIT_NAME"; then
    systemctl status "$UNIT_NAME" --no-pager || true
    rollback_and_fail "$UNIT_NAME is not active after restart"
  fi
  log "systemd service restarted: $UNIT_NAME"

  if [[ "$SKIP_PROCESS_CHECK" != "1" ]]; then
    main_pid="$(systemctl show "$UNIT_NAME" -p MainPID --value 2>/dev/null || true)"
    [[ "$main_pid" =~ ^[0-9]+$ && "$main_pid" != "0" ]] ||
      rollback_and_fail "$UNIT_NAME did not report a valid MainPID"
    running_exe="$(readlink -f "/proc/$main_pid/exe" 2>/dev/null || true)"
    expected_exe="$(readlink -f "$target")"
    [[ "$running_exe" == "$expected_exe" ]] ||
      rollback_and_fail "$UNIT_NAME MainPID is not running $target: ${running_exe:-unreadable}"
    log "process check passed: MainPID=$main_pid exe=$running_exe"

    if [[ "$SKIP_PORT_CHECK" != "1" ]]; then
      command -v ss >/dev/null 2>&1 || rollback_and_fail 'ss is unavailable'
      port_owned=0
      for _ in $(seq 1 "$PORT_WAIT_SECONDS"); do
        if ss -ltnp 2>/dev/null | grep -E '[:.]9999[[:space:]]' | grep -Fq "pid=$main_pid,"; then
          port_owned=1
          break
        fi
        sleep 1
      done
      if [[ "$port_owned" != "1" ]]; then
        ss -ltnp 2>/dev/null | grep -E '[:.]9999[[:space:]]' || true
        rollback_and_fail "port 9999 is not owned by $UNIT_NAME MainPID=$main_pid"
      fi
      log "port check passed: 9999 owned by MainPID=$main_pid"
    fi
  fi
fi

if [[ "$SKIP_HTTP_CHECK" != "1" ]]; then
  command -v curl >/dev/null 2>&1 || rollback_and_fail 'curl is unavailable'
  if ! curl --fail --silent --show-error --max-time "$CURL_TIMEOUT" "$ADDR/health" >/dev/null ||
    ! curl --fail --silent --show-error --max-time "$CURL_TIMEOUT" "$ADDR/ready" >/dev/null; then
    rollback_and_fail "health checks failed: $ADDR"
  fi
  COOKIE_JAR="$(mktemp "${TMPDIR:-/tmp}/workmesh-release-cookie.XXXXXX")"
  security_entrance=""
  if [[ -f "$DATA_DIR/workmesh.db" ]] && command -v python3 >/dev/null 2>&1; then
    security_entrance="$(python3 - "$DATA_DIR/workmesh.db" <<'PY'
import json
import sqlite3
import sys

try:
    with sqlite3.connect(sys.argv[1], timeout=2) as connection:
        row = connection.execute(
            "SELECT payload FROM functional_domain_state WHERE id=1"
        ).fetchone()
    document = json.loads(row[0]) if row else {}
    value = document.get("settings", {}).get("securityEntrance", "")
    if isinstance(value, str):
        print(value.strip("/"))
except Exception:
    pass
PY
)"
  fi
  if [[ -n "$security_entrance" ]]; then
    if ! curl --fail --silent --show-error --max-time "$CURL_TIMEOUT" \
      --cookie-jar "$COOKIE_JAR" "$ADDR/$security_entrance" >/dev/null; then
      rollback_and_fail "security entrance check failed: $ADDR/$security_entrance"
    fi
  fi
  if ! index_html="$(curl --fail --silent --show-error --max-time "$CURL_TIMEOUT" \
    --cookie "$COOKIE_JAR" "$ADDR/advanced/waf")"; then
    rollback_and_fail "frontend route check failed: $ADDR/advanced/waf"
  fi
  app_js="$(printf '%s' "$index_html" | sed -n 's#.*src="\(/assets/js/index-[^"]*\.js\)".*#\1#p' | head -1)"
  [[ -n "$app_js" ]] || rollback_and_fail 'frontend index did not contain the app bundle'
  if ! app_content="$(curl --fail --silent --show-error --max-time "$CURL_TIMEOUT" "$ADDR$app_js")"; then
    rollback_and_fail "frontend app bundle check failed: $ADDR$app_js"
  fi
  waf_js="$(printf '%s' "$app_content" | grep -o 'waf-[A-Za-z0-9_.-]*\.js' | head -1 || true)"
  if [[ -n "$waf_js" ]]; then
    if ! waf_content="$(curl --fail --silent --show-error --max-time "$CURL_TIMEOUT" "$ADDR/assets/js/$waf_js")"; then
      rollback_and_fail "frontend WAF chunk check failed: $ADDR/assets/js/$waf_js"
    fi
    printf '%s' "$waf_content" | grep -Fq '/advanced/waf/overview' ||
      rollback_and_fail "WAF chunk is not the routed overview build: $waf_js"
    printf '%s' "$waf_content" | grep -Fq 'xpack.waf.overview' ||
      rollback_and_fail "WAF chunk is missing the overview tab label: $waf_js"
    if printf '%s' "$waf_content" | grep -Fq 'waf-tabs'; then
      rollback_and_fail "WAF chunk still contains the old waf-tabs layout: $waf_js"
    fi
  else
    # Vite may place the WAF route in an indirect dynamic-import chunk, so the
    # entry chunk is not required to contain the WAF filename itself.
    grep -aFq '/advanced/waf/overview' "$target" ||
      rollback_and_fail 'release binary is missing the WAF overview route'
    grep -aFq 'xpack.waf.overview' "$target" ||
      rollback_and_fail 'release binary is missing the WAF overview label'
    if grep -aFq 'waf-tabs' "$target"; then
      rollback_and_fail 'release binary still contains the old waf-tabs layout'
    fi
  fi
  log "HTTP checks passed: $ADDR app=$app_js waf=$waf_js"
fi

log 'release activation complete'
