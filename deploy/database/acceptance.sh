#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEMPLATE="${REPO_ROOT}/deploy/database/docker-compose.yml.tmpl"
PREFIX="${WORKMESH_ACCEPTANCE_PREFIX:-workmesh-acceptance-db}"
PROJECT="${WORKMESH_ACCEPTANCE_PROJECT:-${PREFIX}-$(date +%s)-$$}"
ROOT="${WORKMESH_ACCEPTANCE_ROOT:-$(mktemp -d "${TMPDIR:-/tmp}/workmesh-db-acceptance.XXXXXX")}"
SECRETS="${ROOT}/secrets"
BACKUPS="${ROOT}/backups"
COMPOSE_FILE="${ROOT}/docker-compose.yml"
KEEP="${WORKMESH_ACCEPTANCE_KEEP:-0}"

POSTGRES_CONTAINER="${PREFIX}-postgres"
REDIS_CONTAINER="${PREFIX}-redis"
MARIADB_CONTAINER="${PREFIX}-mariadb"
RESTORE_REDIS_CONTAINER="${PREFIX}-redis-restore"
NETWORK="${PREFIX}-network"
POSTGRES_VOLUME="${PREFIX}-postgres-data"
REDIS_VOLUME="${PREFIX}-redis-data"
MARIADB_VOLUME="${PREFIX}-mariadb-data"
POSTGRES_DB="${WORKMESH_POSTGRES_DB:-workmesh}"
POSTGRES_USER="${WORKMESH_POSTGRES_USER:-workmesh}"
MARIADB_DATABASE="${WORKMESH_MARIADB_DATABASE:-workmesh}"
MARIADB_USER="${WORKMESH_MARIADB_USER:-workmesh}"

COMPOSE=(docker compose --project-name "$PROJECT" --file "$COMPOSE_FILE")
CREATED=0

log() {
  printf '[database-acceptance] %s\n' "$*" >&2
}

fail() {
  log "FAIL: $*"
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "缺少命令: $1"
}

cleanup() {
  local status=$?
  if [[ "$KEEP" != "1" && "$CREATED" == "1" ]]; then
    log "清理临时 Compose 资源"
    "${COMPOSE[@]}" rm --force --stop postgres redis mariadb >/dev/null 2>&1 || true
    docker rm --force "$RESTORE_REDIS_CONTAINER" >/dev/null 2>&1 || true
    docker network rm "$NETWORK" >/dev/null 2>&1 || true
    log "保留临时 named volume（安全策略禁止脚本自动删除卷）: $POSTGRES_VOLUME $REDIS_VOLUME $MARIADB_VOLUME"
  fi
  if [[ "$KEEP" != "1" && -z "${WORKMESH_ACCEPTANCE_ROOT:-}" ]]; then
    rm -rf "$ROOT"
  else
    log "保留验收目录: $ROOT"
  fi
  exit "$status"
}
trap cleanup EXIT

assert_no_legacy_mutation_target() {
  local name
  for name in WorkMesh-postgresql-ZNMP WorkMesh-redis-ZNMP; do
    if docker ps -a --format '{{.Names}}' | grep -Fxq "$name"; then
      log "检测到旧生产容器 ${name}，本脚本不会对其执行任何写操作"
    fi
  done
}

assert_unique_resources() {
  local resource
  for resource in "$POSTGRES_CONTAINER" "$REDIS_CONTAINER" "$MARIADB_CONTAINER" "$RESTORE_REDIS_CONTAINER"; do
    if docker ps -a --format '{{.Names}}' | grep -Fxq "$resource"; then
      fail "临时容器名称已存在: $resource"
    fi
  done
  if docker network ls --format '{{.Name}}' | grep -Fxq "$NETWORK"; then
    fail "临时网络名称已存在: $NETWORK"
  fi
  for resource in "$POSTGRES_VOLUME" "$REDIS_VOLUME" "$MARIADB_VOLUME"; do
    if docker volume ls --format '{{.Name}}' | grep -Fxq "$resource"; then
      fail "临时卷名称已存在: $resource"
    fi
  done
}

write_secret() {
  local path=$1
  umask 077
  openssl rand -hex 24 >"$path"
  chmod 0600 "$path"
}

render_compose() {
  install -d -m 0700 "$ROOT" "$SECRETS" "$BACKUPS"
  write_secret "$SECRETS/postgres_password"
  write_secret "$SECRETS/redis_password"
  write_secret "$SECRETS/mariadb_password"
  write_secret "$SECRETS/mariadb_root_password"

  # The production template intentionally has fixed names. The acceptance copy
  # rewrites only names and volume/network identifiers into an isolated namespace.
  sed \
    -e "s/workmesh-panel-postgres-data/${POSTGRES_VOLUME}/g" \
    -e "s/workmesh-panel-redis-data/${REDIS_VOLUME}/g" \
    -e "s/workmesh-panel-mariadb-data/${MARIADB_VOLUME}/g" \
    -e "s/workmesh-panel-db/${NETWORK}/g" \
    -e "s/workmesh-panel-postgres/${POSTGRES_CONTAINER}/g" \
    -e "s/workmesh-panel-redis/${REDIS_CONTAINER}/g" \
    -e "s/workmesh-panel-mariadb/${MARIADB_CONTAINER}/g" \
    "$TEMPLATE" >"$COMPOSE_FILE"
  chmod 0600 "$COMPOSE_FILE"
}

compose_config() {
  WORKMESH_DB_SECRETS_DIR="$SECRETS" \
    WORKMESH_POSTGRES_DB="$POSTGRES_DB" \
    WORKMESH_POSTGRES_USER="$POSTGRES_USER" \
    WORKMESH_MARIADB_DATABASE="$MARIADB_DATABASE" \
    WORKMESH_MARIADB_USER="$MARIADB_USER" \
    "${COMPOSE[@]}" config --quiet
}

compose_up() {
  CREATED=1
  WORKMESH_DB_SECRETS_DIR="$SECRETS" \
    WORKMESH_POSTGRES_DB="$POSTGRES_DB" \
    WORKMESH_POSTGRES_USER="$POSTGRES_USER" \
    WORKMESH_MARIADB_DATABASE="$MARIADB_DATABASE" \
    WORKMESH_MARIADB_USER="$MARIADB_USER" \
    "${COMPOSE[@]}" up -d
}

wait_for_health() {
  local container=$1
  local deadline=$((SECONDS + ${WORKMESH_ACCEPTANCE_HEALTH_TIMEOUT:-180}))
  local health status
  while ((SECONDS < deadline)); do
    health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$container" 2>/dev/null || true)"
    status="$(docker inspect --format '{{.State.Status}}' "$container" 2>/dev/null || true)"
    if [[ "$health" == "healthy" ]]; then
      return 0
    fi
    if [[ "$status" == "exited" || "$status" == "dead" ]]; then
      docker logs --tail 40 "$container" >&2 || true
      fail "容器未运行: $container ($status/$health)"
    fi
    sleep 2
  done
  docker inspect "$container" >&2 || true
  fail "容器健康检查超时: $container"
}

assert_no_published_ports() {
  local container ports
  for container in "$POSTGRES_CONTAINER" "$REDIS_CONTAINER" "$MARIADB_CONTAINER"; do
    ports="$(docker inspect --format '{{json .NetworkSettings.Ports}}' "$container")"
    [[ "$ports" == "{}" || "$ports" == "null" ]] || fail "容器发布了宿主机端口: $container"
  done
}

container_stdin() {
  local container=$1
  shift
  docker exec -i "$container" "$@"
}

postgres_sql() {
  local sql=$1
  {
    cat "$SECRETS/postgres_password"
    printf '\n%s\n' "$sql"
  } | container_stdin "$POSTGRES_CONTAINER" env \
    WORKMESH_POSTGRES_DB="$POSTGRES_DB" \
    WORKMESH_POSTGRES_USER="$POSTGRES_USER" \
    sh -ec '
    IFS= read -r password
    export PGPASSWORD="$password"
    psql --no-psqlrc --set ON_ERROR_STOP=1 -h 127.0.0.1 -U "$WORKMESH_POSTGRES_USER" -d "$WORKMESH_POSTGRES_DB"
  '
}

mariadb_sql() {
  local sql=$1
  {
    cat "$SECRETS/mariadb_password"
    printf '\n%s\n' "$sql"
  } | container_stdin "$MARIADB_CONTAINER" env \
    WORKMESH_MARIADB_USER="$MARIADB_USER" \
    WORKMESH_MARIADB_DATABASE="$MARIADB_DATABASE" \
    sh -ec '
    IFS= read -r password
    mariadb --protocol=socket --user="$WORKMESH_MARIADB_USER" --password="$password" --database="$WORKMESH_MARIADB_DATABASE"
  '
}

redis_cli() {
  local command=$1
  {
    cat "$SECRETS/redis_password"
    printf '\n%s\n' "$command"
  } | container_stdin "$REDIS_CONTAINER" sh -ec '
    IFS= read -r password
    export REDISCLI_AUTH="$password"
    redis-cli --no-auth-warning --raw
  '
}

assert_database_cli() {
  log "验证 PostgreSQL 容器内 CLI"
  postgres_sql "CREATE TABLE IF NOT EXISTS acceptance_marker (id integer primary key, value text not null); TRUNCATE acceptance_marker; INSERT INTO acceptance_marker VALUES (1, 'postgres-ok');"
  [[ "$(postgres_sql 'SELECT value FROM acceptance_marker WHERE id = 1;')" == "postgres-ok" ]] || fail "PostgreSQL 读写失败"

  log "验证 MariaDB 容器内 CLI"
  mariadb_sql "CREATE TABLE IF NOT EXISTS acceptance_marker (id INT PRIMARY KEY, value VARCHAR(64) NOT NULL); DELETE FROM acceptance_marker; INSERT INTO acceptance_marker VALUES (1, 'mariadb-ok');"
  [[ "$(mariadb_sql 'SELECT value FROM acceptance_marker WHERE id = 1;')" == "mariadb-ok" ]] || fail "MariaDB 读写失败"

  log "验证 Redis 容器内 CLI"
  [[ "$(redis_cli "SET workmesh:acceptance redis-ok")" == "OK" ]] || fail "Redis 写入失败"
  [[ "$(redis_cli "GET workmesh:acceptance")" == "redis-ok" ]] || fail "Redis 读取失败"
}

backup_postgres() {
  log "验证 PostgreSQL 流式备份与恢复"
  local artifact="${BACKUPS}/postgres.dump"
  {
    cat "$SECRETS/postgres_password"
    printf '\n'
  } | container_stdin "$POSTGRES_CONTAINER" env \
    WORKMESH_POSTGRES_DB="$POSTGRES_DB" \
    WORKMESH_POSTGRES_USER="$POSTGRES_USER" \
    sh -ec '
    IFS= read -r password
    export PGPASSWORD="$password"
    pg_dump --no-password --format=custom --no-owner --no-acl -h 127.0.0.1 -U "$WORKMESH_POSTGRES_USER" -d "$WORKMESH_POSTGRES_DB"
  ' >"$artifact"
  [[ -s "$artifact" ]] || fail "PostgreSQL 备份为空"
  postgres_sql "UPDATE acceptance_marker SET value = 'postgres-mutated' WHERE id = 1;"
  {
    cat "$SECRETS/postgres_password"
    cat "$artifact"
  } | container_stdin "$POSTGRES_CONTAINER" env \
    WORKMESH_POSTGRES_DB="$POSTGRES_DB" \
    WORKMESH_POSTGRES_USER="$POSTGRES_USER" \
    sh -ec '
    IFS= read -r password
    export PGPASSWORD="$password"
    pg_restore --no-password --clean --if-exists --no-owner --no-acl -h 127.0.0.1 -U "$WORKMESH_POSTGRES_USER" -d "$WORKMESH_POSTGRES_DB"
  '
  [[ "$(postgres_sql 'SELECT value FROM acceptance_marker WHERE id = 1;')" == "postgres-ok" ]] || fail "PostgreSQL 恢复未还原数据"
}

backup_mariadb() {
  log "验证 MariaDB 流式备份与恢复"
  local artifact="${BACKUPS}/mariadb.sql"
  {
    cat "$SECRETS/mariadb_root_password"
    printf '\n'
  } | container_stdin "$MARIADB_CONTAINER" env \
    WORKMESH_MARIADB_DATABASE="$MARIADB_DATABASE" \
    sh -ec '
    IFS= read -r password
    mariadb-dump --protocol=socket --user=root --password="$password" --single-transaction --routines --events --triggers --databases "$WORKMESH_MARIADB_DATABASE"
  ' >"$artifact"
  [[ -s "$artifact" ]] || fail "MariaDB 备份为空"
  mariadb_sql "UPDATE acceptance_marker SET value = 'mariadb-mutated' WHERE id = 1;"
  {
    cat "$SECRETS/mariadb_root_password"
    cat "$artifact"
  } | container_stdin "$MARIADB_CONTAINER" sh -ec '
    IFS= read -r password
    mariadb --protocol=socket --user=root --password="$password"
  '
  [[ "$(mariadb_sql 'SELECT value FROM acceptance_marker WHERE id = 1;')" == "mariadb-ok" ]] || fail "MariaDB 恢复未还原数据"
}

redis_command_with_password() {
  local container=$1
  local command=$2
  {
    cat "$SECRETS/redis_password"
    printf '\n%s\n' "$command"
  } | container_stdin "$container" sh -ec '
    IFS= read -r password
    export REDISCLI_AUTH="$password"
    redis-cli --no-auth-warning --raw
  '
}

backup_redis() {
  log "验证 Redis BGSAVE、RDB 备份和恢复"
  local artifact="${BACKUPS}/redis.rdb"
  local restore_dir="${ROOT}/redis-restore"
  install -d -m 0700 "$restore_dir"
  redis_cli "SET workmesh:acceptance redis-rdb-ok"
  redis_command_with_password "$REDIS_CONTAINER" "BGSAVE" >/dev/null

  local deadline=$((SECONDS + ${WORKMESH_ACCEPTANCE_REDIS_TIMEOUT:-120}))
  local status
  while ((SECONDS < deadline)); do
    status="$(redis_command_with_password "$REDIS_CONTAINER" "INFO persistence" | awk -F: '/^rdb_bgsave_in_progress:/{active=$2} /^rdb_last_bgsave_status:/{result=$2} END{gsub(/\r/,"",active); gsub(/\r/,"",result); if (active == "0" && result == "ok") print "ok"}')"
    [[ "$status" == "ok" ]] && break
    sleep 2
  done
  [[ "$status" == "ok" ]] || fail "Redis BGSAVE 未成功完成"

  docker cp "$REDIS_CONTAINER:/data/dump.rdb" "$artifact"
  [[ -s "$artifact" ]] || fail "Redis RDB 备份为空"

  # Restore into a separate disposable Redis process so the acceptance test
  # does not stop or rewrite the Compose-managed database during verification.
  docker run -d -i --name "$RESTORE_REDIS_CONTAINER" \
    --network "$NETWORK" \
    -v "$restore_dir:/data" \
    redis:8-alpine sh -ec '
      password="$(cat)"
      exec redis-server --appendonly no --requirepass "$password"
    ' <"$SECRETS/redis_password" >/dev/null
  local restore_deadline=$((SECONDS + 60))
  while ((SECONDS < restore_deadline)); do
    if [[ "$(docker inspect --format '{{.State.Status}}' "$RESTORE_REDIS_CONTAINER" 2>/dev/null || true)" == "running" ]]; then
      break
    fi
    sleep 1
  done
  docker cp "$artifact" "$RESTORE_REDIS_CONTAINER:/data/dump.rdb"
  docker restart "$RESTORE_REDIS_CONTAINER" >/dev/null
  local restore_status
  restore_status="$(docker inspect --format '{{.State.Status}}' "$RESTORE_REDIS_CONTAINER" 2>/dev/null || true)"
  [[ "$restore_status" == "running" ]] || fail "Redis 恢复容器未运行: $restore_status"
  local restored
  restored="$(docker exec -i "$RESTORE_REDIS_CONTAINER" sh -ec '
    password="$(cat)"
    export REDISCLI_AUTH="$password"
    redis-cli --no-auth-warning --raw GET workmesh:acceptance
  ' <"$SECRETS/redis_password")"
  [[ "$restored" == "redis-rdb-ok" ]] || fail "Redis RDB 恢复未还原数据"
}

assert_runtime_status() {
  local container status health
  for container in "$POSTGRES_CONTAINER" "$REDIS_CONTAINER" "$MARIADB_CONTAINER"; do
    status="$(docker inspect --format '{{.State.Status}}' "$container")"
    health="$(docker inspect --format '{{.State.Health.Status}}' "$container")"
    [[ "$status" == "running" && "$health" == "healthy" ]] || fail "容器最终状态异常: $container ($status/$health)"
  done
}

main() {
  require_command docker
  require_command openssl
  [[ -f "$TEMPLATE" ]] || fail "Compose 模板不存在: $TEMPLATE"
  docker info >/dev/null || fail "当前用户没有 Docker Socket 权限"
  assert_no_legacy_mutation_target
  assert_unique_resources
  render_compose
  log "验证 Compose 静态配置"
  compose_config
  log "启动隔离数据库 Compose"
  compose_up
  wait_for_health "$POSTGRES_CONTAINER"
  wait_for_health "$REDIS_CONTAINER"
  wait_for_health "$MARIADB_CONTAINER"
  assert_no_published_ports
  assert_database_cli
  backup_postgres
  backup_mariadb
  backup_redis
  assert_runtime_status
  log "PASS: 数据库 Compose、容器内 CLI、备份和恢复验收完成"
}

main "$@"
