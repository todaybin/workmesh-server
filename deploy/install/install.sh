#!/usr/bin/env bash
set -Eeuo pipefail

# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

# 新服务安装脚本默认只执行预检；生产写入必须显式传入 --apply。
readonly ROOT_DIR="${WORKMESH_SERVER_ROOT:-/opt/workmesh-server}"
readonly UNIT_NAME="workmesh-server.service"
readonly UNIT_SOURCE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/systemd/$UNIT_NAME"

log() { printf '[WorkMesh Server] %s\n' "$*"; }
fail() { printf '[WorkMesh Server] 操作失败：%s\n' "$*" >&2; exit 1; }
require_root() { [[ "$(id -u)" -eq 0 ]] || fail '请使用 root 执行'; }

apply=0
for arg in "$@"; do
  [[ "$arg" == '--apply' ]] && apply=1
done

require_root
command -v systemctl >/dev/null 2>&1 || fail '当前系统未提供 systemctl'
[[ -x "${WORKMESH_SERVER_BINARY:-}" ]] || log '未指定 WORKMESH_SERVER_BINARY，安装阶段只执行环境预检。'
[[ -f "$UNIT_SOURCE" ]] || fail "缺少 systemd 模板：$UNIT_SOURCE"

if systemctl is-active --quiet workmesh-node-core.service || systemctl is-active --quiet workmesh-node-agent.service; then
  fail '检测到旧 WorkMesh Node 服务仍在运行；必须先完成备份、验收和人工确认后再切换。'
fi

if (( apply == 0 )); then
  log '预检通过，未写入系统。确认新包、配置和回滚备份后重新执行：$0 --apply'
  exit 0
fi

[[ -n "${WORKMESH_SERVER_BINARY:-}" && -x "$WORKMESH_SERVER_BINARY" ]] || fail '正式安装需要可执行文件 WORKMESH_SERVER_BINARY'
install -d -m 0750 "$ROOT_DIR/bin" "$ROOT_DIR/config" "$ROOT_DIR/data"
install -m 0750 "$WORKMESH_SERVER_BINARY" "$ROOT_DIR/bin/workmesh-server"
install -m 0644 "$UNIT_SOURCE" "/etc/systemd/system/$UNIT_NAME"
systemctl daemon-reload
systemctl enable --now "$UNIT_NAME"
log 'WorkMesh Server 已安装并启动；请继续执行 /health、/ready 和功能验收。'
