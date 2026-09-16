# MySQL、PostgreSQL、Redis 首版验收记录

日期：2026-09-08

## 已实现

- MySQL/MariaDB 通过受控 CLI 或本地容器 `docker exec` 完成建库、用户、授权、改密、撤销和删除；远程成功后写 SQLite，写入失败执行补偿。
- PostgreSQL 通过 `psql` 标准输入执行 SQL，密码只通过 `PGPASSWORD` 传递；支持建库、角色、绑定、超级用户权限、改密和删除。
- Redis 通过 `redis-cli` 参数数组执行 `PING`、`INFO`、白名单 `CONFIG GET/SET/REWRITE`、AOF/RDB 配置和改密；密码不回显。
- MongoDB 通过受控 `mongosh` 执行器完成创建、绑定、改密、权限、root 密码、删除和数据库列表同步；stdin 密码提示已标准化，使用 `_init` 合法占位 collection，并按目标库清理用户后删除。
- PostgreSQL 和 MySQL/MariaDB 已增加真实数据库列表同步；MySQL 变量查询/更新和 root 访问权限也通过真实 CLI 执行并保留失败回滚。
- `/api/v2/databases/db/check` 对三类数据库执行真实认证检查，不再仅验证 TCP 端口。
- CLI 缺失、实例未登记或依赖不可达统一返回 `503 + code=ERR`；不自动安装系统包。

## 隔离黑盒结果

执行命令：

```bash
WORKMESH_DATABASE_EXTERNAL_TEST=1 GOWORK=off \
  go test ./node/api -run '^TestExternalDatabaseLifecycles$' -count=1 -v
```

结果：通过。使用的隔离镜像为 `mariadb:11`、`postgres:18-alpine`、`redis:8-alpine`，容器名称只使用 `workmesh-acceptance-*` 前缀。

- MariaDB：真实 schema、用户、改密、撤销与重新授权均成功；删除后 schema、远程用户、SQLite 用户和授权均为零。
- PostgreSQL：真实数据库和角色创建成功；绑定新角色、改权和改密成功；删除后数据库、新旧角色及 SQLite 运行配置均无残留。
- Redis：状态、普通配置、AOF/RDB 配置、改密及新密码重连成功；停止容器后接口返回真实 `503`；删除登记后 SQLite 运行配置无残留。
- MongoDB：隔离 `mongo:8-noble` 容器中真实建库、绑定、改密、权限、root 密码、删除和 SQLite 清理通过；远程数据库列表同步也已在该生命周期中验证。
- PostgreSQL：隔离容器中真实数据库列表同步已加入并通过定向执行；MySQL/MariaDB 列表同步的代码和定向解析测试通过。
- 同一隔离黑盒还验证了 MySQL/MariaDB 变量真实查询、数值变量更新（`max_connections`）和 root `%` 访问权限修改；执行后所有临时容器和 2026-09-09 匿名测试卷均已清理。
- 故障注入：mock executor 已验证 PostgreSQL 建库中途失败的回滚顺序，以及 Redis 部分配置失败后的逆序恢复。
- 测试结束未保留任何 `workmesh-acceptance-*` 容器。

## 仍未执行

- 未对现有生产数据库执行写入、改密或删除。
- MongoDB、PostgreSQL、MySQL/MariaDB 的基础列表同步已实现；仍未验证 PostgreSQL/Redis 集群编排、MongoDB 备份恢复和跨节点数据库操作。
- 未执行生产二进制替换、生产 SQLite 迁移或发布回滚演练。
- 网站高级设置仍按各自计划保持 `not-run`；容器基础生命周期黑盒已通过，复杂编排及生产 daemon 仍未执行。
