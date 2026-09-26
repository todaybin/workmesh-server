<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 数据库资源管理（首批）

数据库资源以节点本地仓储登记，数据持久化到 `WORKMESH_DATA_DIR/databases.json`（未设置时为 `./data`），敏感密码不落库、不回显。接口保留 `/api/v2/databases/db/*` 路径：

- `POST /api/v2/databases/db`：校验类型、主机、端口并登记资源。
- `POST /api/v2/databases/db/check`：检查连接参数格式（不执行破坏性连接）。
- `POST /api/v2/databases/db/search`：按类型和名称过滤，返回分页兼容 envelope。
- `GET /api/v2/databases/db/list/:type`：返回数据库实例数组，不是分页对象。`type` 支持逗号分隔，并展开同族别名（例如 `postgresql,postgresql-cluster` 同时包含 `postgres`）。元素字段为 `id`、`from`、`type`、`database`、`version`、`address`。只返回实例，不返回实例内的库。已安装且状态为运行或停止的 PostgreSQL、Redis、MySQL/MariaDB、MongoDB 应用会在列表时补登记为 `from=local` 的实例。
- `GET /api/v2/databases/db/item/:type`：返回实例内数据库数组，字段为 `id`、`from`、`database`、`name`。
- `POST /api/v2/databases/search`、`/databases/pg/search`、`/databases/mongodb/search`：按 `database` 返回该实例内的库，不把实例自身当成一条库记录。
- `POST /api/v2/databases/common/info`：返回实例的 `name`、`port`、`containerName`、`password`、`remoteConn`。
- `POST /api/v2/databases/common/load/file` 与 `common/update/conf`：读写安装目录中的 `postgresql.conf`、`my.cnf` 或 `redis.conf`；保存后重启对应 Compose。
- `POST /api/v2/databases/db/search`：远程服务器列表只返回 `from=remote` 的实例，字段包含 `address` 和 `password`，并按 `info` 过滤名称。
- 数据库终端：本地实例进入已登记容器；远程 Redis/MySQL/PostgreSQL/MongoDB 使用本机客户端连接实例地址。Redis 页面的 `source=redis` 同样走该逻辑。
- `POST /api/v2/apps/installed/check`、`installed/op`、`installed/port/change`、`installed/conninfo`：数据库应用按键和实例名定位安装记录，停止、重启、改端口和连接信息都使用该记录。
- `POST /api/v2/databases/db/del`：按 `id` 删除登记。
- `POST /api/v2/databases/db/update`：按 `id` 更新登记，保留创建时间。

搜索分页参数 `page` 从 1 开始，`pageSize` 默认 50、最大 100；名称、类型和主机字段会去除首尾空白，类型按小写匹配。

当前实现聚焦轻量控制面资源登记；实际 MySQL/PostgreSQL/MongoDB/Redis 建库、备份和账号权限操作应在后续批次接入对应 CLI/驱动，并继续沿用该 service/repository 边界。
