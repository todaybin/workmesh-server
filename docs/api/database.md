<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 数据库资源管理（首批）

数据库资源以节点本地仓储登记，敏感密码不落库、不回显。接口保留旧 `/api/v2/databases/db/*` 路径：

- `POST /api/v2/databases/db`：校验类型、主机、端口并登记资源。
- `POST /api/v2/databases/db/check`：检查连接参数格式（不执行破坏性连接）。
- `POST /api/v2/databases/db/search`：按类型和名称过滤，返回分页兼容 envelope。
- `POST /api/v2/databases/db/del`：按 `id` 删除登记。

当前实现聚焦轻量控制面资源登记；实际 MySQL/PostgreSQL/MongoDB/Redis 建库、备份和账号权限操作应在后续批次接入对应 CLI/驱动，并继续沿用该 service/repository 边界。
