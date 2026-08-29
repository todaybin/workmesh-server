#!/usr/bin/env bash
set -Eeuo pipefail

# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

# 旧版本卸载是破坏性操作；默认只输出检查结果，不删除程序或数据。
readonly OLD_ROOT="${WORKMESH_OLD_ROOT:-/opt/workmesh}"
readonly CONFIRM="${WORKMESH_CONFIRM_OLD_UNINSTALL:-}"

fail() { printf '[WorkMesh Server] 操作失败：%s\n' "$*" >&2; exit 1; }
[[ "$(id -u)" -eq 0 ]] || fail '请使用 root 执行'
command -v systemctl >/dev/null 2>&1 || fail '当前系统未提供 systemctl'

for unit in workmesh-node-core.service workmesh-node-agent.service; do
  if systemctl is-active --quiet "$unit"; then
    fail "旧服务仍在运行：$unit；先完成新服务验收并停止旧服务。"
  fi
done

printf '旧程序目录：%s\n' "$OLD_ROOT"
printf '将移除：旧 systemd 单元、旧二进制和服务管理脚本\n'
printf '默认保留：配置、项目、运行时、备份、日志和数据库数据\n'

[[ "$CONFIRM" == 'REMOVE_OLD_WORKMESH_NODE' ]] || {
  printf '这是预检，没有执行删除。确认完成后设置 WORKMESH_CONFIRM_OLD_UNINSTALL=REMOVE_OLD_WORKMESH_NODE 再执行。\n'
  exit 0
}

systemctl disable --now workmesh-node-core.service workmesh-node-agent.service 2>/dev/null || true
rm -f -- /etc/systemd/system/workmesh-node-core.service /etc/systemd/system/workmesh-node-agent.service
systemctl daemon-reload
rm -f -- "$OLD_ROOT/bin/workmesh-node-core" "$OLD_ROOT/bin/workmesh-node-agent" "$OLD_ROOT/bin/workmesh-node-service" "$OLD_ROOT/bin/wh"
printf '旧程序和 systemd 单元已移除；数据目录仍保留：%s\n' "$OLD_ROOT"
