#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATA_DIR="${WORKMESH_DATA_DIR:-/opt/workmesh-server/data}"
DB_ROOT="${WORKMESH_DB_ROOT:-$DATA_DIR/database}"
COMPOSE_FILE="${WORKMESH_DB_COMPOSE_FILE:-$DB_ROOT/docker-compose.yml}"
TEMPLATE="${WORKMESH_DB_TEMPLATE:-$SCRIPT_DIR/docker-compose.yml.tmpl}"
SECRETS_DIR="${WORKMESH_DB_SECRETS_DIR:-$DB_ROOT/secrets}"
STRICT="${STRICT:-0}"

WARNINGS=0
FAILURES=0

note() {
  printf '[database-preflight] %s\n' "$*"
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

check_path() {
  if [[ -e "$1" ]]; then
    pass "path exists: $1"
  else
    warn "path missing: $1"
  fi
}

check_template_static_contract() {
  note "checking Compose template safety"
  check_path "$TEMPLATE"
  if [[ ! -f "$TEMPLATE" ]]; then
    return 0
  fi

  local fragment
  for fragment in \
    "container_name: workmesh-panel-postgres" \
    "container_name: workmesh-panel-redis" \
    "container_name: workmesh-panel-mariadb" \
    "name: workmesh-panel-postgres-data" \
    "name: workmesh-panel-redis-data" \
    "name: workmesh-panel-mariadb-data" \
    "name: workmesh-panel-db"; do
    if grep -Fq "$fragment" "$TEMPLATE"; then
      pass "template contract: $fragment"
    else
      fail "template contract missing: $fragment"
    fi
  done

  if grep -Eq '^[[:space:]]+ports:' "$TEMPLATE"; then
    fail "template must not publish database host ports"
  else
    pass "template has no host port publishing"
  fi
  for legacy in WorkMesh-postgresql-ZNMP WorkMesh-redis-ZNMP; do
    if grep -Fq "$legacy" "$TEMPLATE"; then
      fail "template references protected legacy resource: $legacy"
    fi
  done
}

check_docker() {
  note "checking Docker access"
  if ! has_command docker; then
    warn "Docker CLI missing"
    return
  fi
  pass "Docker CLI available: $(command -v docker)"
  if docker compose version >/dev/null 2>&1; then
    pass "docker compose available"
  else
    warn "docker compose unavailable"
  fi
  if docker version --format '{{.Server.Version}}' >/dev/null 2>&1; then
    pass "Docker daemon accessible"
  else
    warn "Docker daemon/socket not accessible"
  fi
}

check_secret() {
  local name="$1"
  local path="$SECRETS_DIR/$name"
  if [[ ! -s "$path" ]]; then
    warn "secret missing or empty: $path"
    return
  fi
  local mode
  mode="$(stat -c '%a' "$path" 2>/dev/null || true)"
  case "$mode" in
    600|400)
      pass "secret exists with restricted mode: $path ($mode)"
      ;;
    *)
      fail "secret must have mode 0600 or 0400: $path ($mode)"
      ;;
  esac
}

check_secrets() {
  note "checking secret files without printing contents"
  check_path "$SECRETS_DIR"
  if [[ ! -d "$SECRETS_DIR" ]]; then
    return 0
  fi
  for secret in postgres_password redis_password mariadb_password mariadb_root_password; do
    check_secret "$secret"
  done
}

check_existing_resources() {
  note "checking fixed names and protected legacy resources"
  if ! has_command docker || ! docker version --format '{{.Server.Version}}' >/dev/null 2>&1; then
    warn "Docker daemon unavailable; container/network/volume collision checks skipped"
    return
  fi

  local name
  for name in \
    workmesh-panel-postgres \
    workmesh-panel-redis \
    workmesh-panel-mariadb \
    workmesh-panel-db \
    workmesh-panel-postgres-data \
    workmesh-panel-redis-data \
    workmesh-panel-mariadb-data; do
    if docker ps -a --format '{{.Names}}' | grep -Fxq "$name" ||
      docker network ls --format '{{.Name}}' | grep -Fxq "$name" ||
      docker volume ls --format '{{.Name}}' | grep -Fxq "$name"; then
      warn "fixed resource already exists; verify ownership before deployment: $name"
    else
      pass "fixed resource name is unused: $name"
    fi
  done

  for name in WorkMesh-postgresql-ZNMP WorkMesh-redis-ZNMP; do
    if docker ps -a --format '{{.Names}}' | grep -Fxq "$name"; then
      pass "protected legacy resource detected and will not be managed: $name"
    else
      warn "protected legacy resource not visible: $name"
    fi
  done
}

check_compose() {
  note "checking runtime Compose file"
  check_path "$COMPOSE_FILE"
  if [[ ! -f "$COMPOSE_FILE" ]]; then
    return 0
  fi
  if ! has_command docker || ! docker compose version >/dev/null 2>&1; then
    warn "Compose unavailable; runtime config validation skipped"
    return
  fi
  if [[ ! -d "$SECRETS_DIR" ]]; then
    warn "secret directory missing; runtime config validation skipped"
    return
  fi
  local missing=0 secret
  for secret in postgres_password redis_password mariadb_password mariadb_root_password; do
    if [[ ! -s "$SECRETS_DIR/$secret" ]]; then
      missing=1
    fi
  done
  if [[ "$missing" -ne 0 ]]; then
    warn "one or more secrets missing; runtime config validation skipped"
    return
  fi
  if WORKMESH_DB_SECRETS_DIR="$SECRETS_DIR" docker compose -f "$COMPOSE_FILE" config --quiet; then
    pass "runtime Compose config is valid"
  else
    fail "runtime Compose config is invalid"
  fi
}

main() {
  note "database migration preflight started"
  note "DATA_DIR=$DATA_DIR"
  note "DB_ROOT=$DB_ROOT"
  note "COMPOSE_FILE=$COMPOSE_FILE"
  note "SECRETS_DIR=$SECRETS_DIR"
  check_docker
  check_template_static_contract
  check_secrets
  check_existing_resources
  check_compose
  note "summary: warnings=$WARNINGS failures=$FAILURES"
  if [[ "$FAILURES" -gt 0 ]] || [[ "$STRICT" == "1" && "$WARNINGS" -gt 0 ]]; then
    exit 1
  fi
}

main "$@"
