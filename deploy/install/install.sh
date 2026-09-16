#!/usr/bin/env bash
set -Eeuo pipefail

# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

# WorkMesh Server 安装脚本默认只执行预检；生产写入必须显式传入 --apply。
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
readonly ROOT_DIR="${WORKMESH_SERVER_ROOT:-/opt/workmesh-server}"
readonly UNIT_NAME="workmesh-server.service"
readonly UNIT_SOURCE="$SCRIPT_DIR/../systemd/$UNIT_NAME"
readonly UPDATE_UNIT_NAME="workmesh-server-update.service"
readonly UPDATE_UNIT_SOURCE="$SCRIPT_DIR/../systemd/$UPDATE_UNIT_NAME"
readonly UPDATE_TIMER_NAME="workmesh-server-update.timer"
readonly UPDATE_TIMER_SOURCE="$SCRIPT_DIR/../systemd/$UPDATE_TIMER_NAME"
readonly UPDATE_ENV_EXAMPLE="$SCRIPT_DIR/update.env.example"
readonly CONFIG_SOURCE="$REPO_ROOT/config/server.json"

log() { printf '[WorkMesh Server] %s\n' "$*"; }
fail() { printf '[WorkMesh Server] 操作失败：%s\n' "$*" >&2; exit 1; }
require_root() { [[ "$(id -u)" -eq 0 ]] || fail '请使用 root 执行'; }

validate_root() {
  [[ "$ROOT_DIR" == /* ]] || fail "WORKMESH_SERVER_ROOT 必须是绝对路径：$ROOT_DIR"
  [[ "$ROOT_DIR" != *[[:space:]]* ]] || fail 'WORKMESH_SERVER_ROOT 不能包含空白字符'
  case "$ROOT_DIR" in
    *'|'*|*'&'*|*'\'*|*'%'*|*$'\n'*)
      fail 'WORKMESH_SERVER_ROOT 不能包含 |、&、反斜杠、百分号或换行'
      ;;
  esac
}

require_runtime_env() {
  local env_file="$ROOT_DIR/config/server.env"
  [[ -s "$env_file" ]] || fail "缺少运行环境文件：$env_file；请先执行 deploy/install/prepare-runtime.sh --apply"
  for key in WORKMESH_SERVER_ADDR WORKMESH_DATA_DIR; do
    grep -Eq "^${key}=.+$" "$env_file" ||
      fail "运行环境文件缺少 ${key}：$env_file；请重新执行 prepare-runtime.sh --apply"
  done
}

install_default_config() {
  local target="$ROOT_DIR/config/server.json"
  if [[ -e "$target" ]]; then
    [[ -f "$target" ]] || fail "配置目标不是普通文件：$target"
    log "保留现有服务配置：$target"
    return
  fi
  install -m 0644 "$CONFIG_SOURCE" "$target"
}

install_update_config() {
  local target="$ROOT_DIR/config/update.env"
  if [[ -e "$target" ]]; then
    [[ -f "$target" ]] || fail "自动更新配置不是普通文件：$target"
    return
  fi
  install -m 0600 "$UPDATE_ENV_EXAMPLE" "$target"
}

install_rendered_unit() {
  local source="$1" target="$2" tmp root_escaped
  root_escaped="$ROOT_DIR"
  root_escaped="${root_escaped//\\/\\\\}"
  root_escaped="${root_escaped//|/\\|}"
  root_escaped="${root_escaped//&/\\&}"
  tmp="$(mktemp "${TMPDIR:-/tmp}/workmesh-server.service.XXXXXX")"
  if ! sed "s|@WORKMESH_SERVER_ROOT@|$root_escaped|g" "$source" >"$tmp"; then
    rm -f "$tmp"
    fail "渲染 systemd 单元失败"
  fi
  if ! install -m 0644 "$tmp" "$target"; then
    rm -f "$tmp"
    fail "安装 systemd 单元失败"
  fi
  rm -f "$tmp"
}

apply=0
for arg in "$@"; do
  [[ "$arg" == '--apply' ]] && apply=1
done

require_root
validate_root
command -v systemctl >/dev/null 2>&1 || fail '当前系统未提供 systemctl'
[[ -x "${WORKMESH_SERVER_BINARY:-}" ]] || log '未指定 WORKMESH_SERVER_BINARY，安装阶段只执行环境预检。'
[[ -f "$UNIT_SOURCE" ]] || fail "缺少 systemd 模板：$UNIT_SOURCE"
[[ -f "$UPDATE_UNIT_SOURCE" ]] || fail "缺少自动更新 systemd 模板：$UPDATE_UNIT_SOURCE"
[[ -f "$UPDATE_TIMER_SOURCE" ]] || fail "缺少自动更新 timer 模板：$UPDATE_TIMER_SOURCE"
[[ -x "$SCRIPT_DIR/activate-release.sh" ]] || fail "缺少发布切换脚本：$SCRIPT_DIR/activate-release.sh"
[[ -x "$SCRIPT_DIR/auto-update.sh" ]] || fail "缺少自动更新脚本：$SCRIPT_DIR/auto-update.sh"
[[ -f "$UPDATE_ENV_EXAMPLE" ]] || fail "缺少自动更新配置示例：$UPDATE_ENV_EXAMPLE"
[[ -f "$CONFIG_SOURCE" ]] || fail "缺少默认服务配置：$CONFIG_SOURCE"

if (( apply == 0 )); then
  if [[ ! -s "$ROOT_DIR/config/server.env" ]]; then
    log '提示：未发现 server.env；正式 --apply 前必须先执行 prepare-runtime.sh --apply。'
  fi
  log '预检通过，未写入系统。确认新包、配置和回滚备份后重新执行：$0 --apply'
  exit 0
fi

[[ -n "${WORKMESH_SERVER_BINARY:-}" && -x "$WORKMESH_SERVER_BINARY" ]] || fail '正式安装需要可执行文件 WORKMESH_SERVER_BINARY'
install -d -m 0750 "$ROOT_DIR/bin" "$ROOT_DIR/config" "$ROOT_DIR/data"
require_runtime_env
install -m 0750 "$SCRIPT_DIR/activate-release.sh" "$ROOT_DIR/bin/activate-release.sh"
install -m 0750 "$SCRIPT_DIR/auto-update.sh" "$ROOT_DIR/bin/workmesh-auto-update"
install_default_config
install_update_config
install_rendered_unit "$UNIT_SOURCE" "/etc/systemd/system/$UNIT_NAME"
install_rendered_unit "$UPDATE_UNIT_SOURCE" "/etc/systemd/system/$UPDATE_UNIT_NAME"
install -m 0644 "$UPDATE_TIMER_SOURCE" "/etc/systemd/system/$UPDATE_TIMER_NAME"
systemctl daemon-reload
if [[ -e "$ROOT_DIR/bin/workmesh-server" ]]; then
  sha_file="${WORKMESH_SERVER_SHA_FILE:-$WORKMESH_SERVER_BINARY.sha256}"
  [[ -s "$sha_file" ]] || fail "已有服务切换需要 SHA-256 文件：$sha_file"
  WORKMESH_SERVER_ROOT="$ROOT_DIR" \
  WORKMESH_SERVER_BINARY="$WORKMESH_SERVER_BINARY" \
  WORKMESH_SERVER_SHA_FILE="$sha_file" \
    "$ROOT_DIR/bin/activate-release.sh"
  systemctl enable "$UNIT_NAME"
else
  install -m 0750 "$WORKMESH_SERVER_BINARY" "$ROOT_DIR/bin/workmesh-server"
  systemctl enable --now "$UNIT_NAME"
fi
systemctl enable --now "$UPDATE_TIMER_NAME"
log 'WorkMesh Server 已安装并启动；自动更新 timer 已安装（默认由 update.env 禁用）。'
