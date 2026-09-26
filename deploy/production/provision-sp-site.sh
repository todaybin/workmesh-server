#!/usr/bin/env bash
set -Eeuo pipefail

# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

# 兼容入口：正式部署必须经过 workmesh-server API，禁止脚本直接写 SQLite、
# Supervisor 配置或网站关系文件。
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/provision-sp-site-api.py" "$@"
