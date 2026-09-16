#!/usr/bin/env bash
set -Eeuo pipefail

# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

ROOT_DIR="${WORKMESH_SERVER_ROOT:-/opt/workmesh-server}"
UNIT_NAME="${WORKMESH_SERVER_UNIT:-workmesh-server.service}"
CONFIRM="${WORKMESH_CONFIRM_PRODUCTION_UNINSTALL:-}"
APPLY=0
PURGE_DATA=0

CONTAINERS=(
  workmesh-openresty-waf
  workmesh-panel-postgres
  workmesh-panel-redis
  workmesh-panel-mariadb
)
VOLUMES=(
  workmesh-panel-postgres-data
  workmesh-panel-redis-data
  workmesh-panel-mariadb-data
)
NETWORKS=(workmesh-panel-db)

log() { printf '[uninstall-production] %s\n' "$*"; }
fail() { printf '[uninstall-production] failed: %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'USAGE'
Uninstall the WorkMesh production installation.

Default behavior is a dry-run. With --apply it removes only WorkMesh service
and runtime resources:
  - workmesh-server.service
  - WorkMesh WAF/database containers
  - WorkMesh database network
  - the /opt/workmesh-server directory is moved to a timestamped backup

It never removes /www/wwwroot, the old /opt/workmesh directory, or the legacy
WorkMesh-postgresql-ZNMP / WorkMesh-redis-ZNMP resources.

Usage:
  deploy/install/uninstall-production.sh [options]

Options:
  --apply             Execute the uninstall. Requires the exact confirmation environment variable.
  --purge-data        Also remove WorkMesh database volumes and delete the runtime backup.
  --root DIR          Production root. Default: WORKMESH_SERVER_ROOT or /opt/workmesh-server.
  --help              Show this help.

Required confirmation for --apply:
  WORKMESH_CONFIRM_PRODUCTION_UNINSTALL=REMOVE_WORKMESH_PRODUCTION

Additional confirmation for --purge-data:
  WORKMESH_CONFIRM_PRODUCTION_UNINSTALL=PURGE_WORKMESH_PRODUCTION
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply)
      APPLY=1
      shift
      ;;
    --purge-data)
      PURGE_DATA=1
      shift
      ;;
    --root)
      ROOT_DIR="${2:?missing value for --root}"
      shift 2
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

[[ "$ROOT_DIR" == /* ]] || fail "root must be absolute: $ROOT_DIR"
[[ "$ROOT_DIR" != "/" && "$ROOT_DIR" != "/opt" && "$ROOT_DIR" != "/www" ]] ||
  fail "refusing unsafe production root: $ROOT_DIR"
[[ "$(basename "$ROOT_DIR")" == "workmesh-server" ]] ||
  fail "refusing unexpected production root name: $ROOT_DIR"

if (( PURGE_DATA == 1 )); then
  [[ "$CONFIRM" == "PURGE_WORKMESH_PRODUCTION" ]] ||
    fail 'purge requires WORKMESH_CONFIRM_PRODUCTION_UNINSTALL=PURGE_WORKMESH_PRODUCTION'
elif (( APPLY == 1 )); then
  [[ "$CONFIRM" == "REMOVE_WORKMESH_PRODUCTION" ]] ||
    fail 'apply requires WORKMESH_CONFIRM_PRODUCTION_UNINSTALL=REMOVE_WORKMESH_PRODUCTION'
fi
if (( PURGE_DATA == 1 && APPLY == 0 )); then
  fail '--purge-data requires --apply'
fi

if (( APPLY == 0 && PURGE_DATA == 0 )); then
  log "dry-run; no production state will be changed"
  log "target root: $ROOT_DIR"
  log "remove service: $UNIT_NAME"
  log "remove containers: ${CONTAINERS[*]}"
  log "remove network: ${NETWORKS[*]}"
  log "preserve runtime directory as: ${ROOT_DIR}.uninstalled-<timestamp>"
  log "preserve: /www/wwwroot, /opt/workmesh, WorkMesh-postgresql-ZNMP, WorkMesh-redis-ZNMP"
  log "use --apply with WORKMESH_CONFIRM_PRODUCTION_UNINSTALL=REMOVE_WORKMESH_PRODUCTION to execute"
  log "use --purge-data only when deleting WorkMesh SQLite/config/WAF data and named volumes is intended"
  exit 0
fi

[[ "$(id -u)" -eq 0 ]] || fail 'run as root on the production host'
command -v systemctl >/dev/null 2>&1 || fail 'systemctl is unavailable'
command -v docker >/dev/null 2>&1 || fail 'docker is unavailable'

parent_dir="$(dirname "$ROOT_DIR")"
[[ -d "$parent_dir" ]] || fail "production root parent does not exist: $parent_dir"
[[ -w "$parent_dir" ]] || fail "production root parent is not writable: $parent_dir"
if [[ -e "$ROOT_DIR" ]]; then
  [[ -d "$ROOT_DIR" ]] || fail "production root is not a directory: $ROOT_DIR"
  [[ -w "$ROOT_DIR" ]] || fail "production root is not writable: $ROOT_DIR"
fi

# 所有能力检查必须在修改前完成，避免受限环境先停服务再删除文件时中途失败。
if ! systemctl show "$UNIT_NAME" -p LoadState --value >/dev/null 2>&1; then
  fail "systemd is not accessible; run this on the real production host"
fi
if ! docker version --format '{{.Server.Version}}' >/dev/null 2>&1; then
  fail "Docker daemon is not accessible; run this on the real production host"
fi
[[ -d /etc/systemd/system && -w /etc/systemd/system ]] ||
  fail '/etc/systemd/system is not writable'

if (( PURGE_DATA == 1 )); then
  log 'purge mode enabled: WorkMesh data and named database volumes will be deleted'
else
  log 'uninstall mode enabled: runtime directory will be preserved as a timestamped backup'
fi

if systemctl is-active --quiet "$UNIT_NAME"; then
  systemctl stop "$UNIT_NAME"
  log "stopped systemd unit: $UNIT_NAME"
fi
systemctl disable "$UNIT_NAME" >/dev/null 2>&1 || true

for container in "${CONTAINERS[@]}"; do
  ids="$(docker ps -aq --filter "name=^${container}$")"
  if [[ -n "$ids" ]]; then
    docker rm -f $ids
    log "removed container: $container"
  fi
done

for network in "${NETWORKS[@]}"; do
  if docker network inspect "$network" >/dev/null 2>&1; then
    docker network rm "$network" >/dev/null
    log "removed network: $network"
  fi
done

if (( PURGE_DATA == 1 )); then
  for volume in "${VOLUMES[@]}"; do
    if docker volume inspect "$volume" >/dev/null 2>&1; then
      docker volume rm "$volume" >/dev/null
      log "removed volume: $volume"
    fi
  done
fi

unit_path="/etc/systemd/system/$UNIT_NAME"
rm -f -- "$unit_path"
systemctl daemon-reload
systemctl reset-failed "$UNIT_NAME" >/dev/null 2>&1 || true
log "removed systemd unit: $unit_path"

if [[ -d "$ROOT_DIR" ]]; then
  backup_root="${ROOT_DIR}.uninstalled-$(date +%Y%m%d%H%M%S)-$$"
  mv -- "$ROOT_DIR" "$backup_root"
  log "runtime directory moved to: $backup_root"
  if (( PURGE_DATA == 1 )); then
    rm -rf -- "$backup_root"
    log "deleted runtime backup and WorkMesh data"
  fi
fi

if systemctl show "$UNIT_NAME" -p LoadState --value 2>/dev/null | grep -qv '^not-found$'; then
  fail "systemd unit still exists: $UNIT_NAME"
fi
for container in "${CONTAINERS[@]}"; do
  if docker inspect "$container" >/dev/null 2>&1; then
    fail "container still exists: $container"
  fi
done

log 'production WorkMesh uninstall complete'
