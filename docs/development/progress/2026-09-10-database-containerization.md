<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 数据库容器化并行实施（2026-09-10）

## 目标

将 PostgreSQL、Redis、MySQL/MariaDB 的面板管理路径统一切换到独立 Docker 容器，同时保留旧 ZNMP 数据库容器不变：

- `WorkMesh-postgresql-ZNMP`
- `WorkMesh-redis-ZNMP`

宿主机不依赖 `psql`、`mysql`、`mariadb` 或 `redis-cli`。

## 并行任务

- [x] Compose 模板：固定容器、独立网络、named volume、secret、healthcheck、无宿主端口。
- [x] 数据库执行器：容器模式使用 `docker exec`，密码不进入 Docker 参数；PostgreSQL、Redis、MySQL/MariaDB 统一使用容器内固定脚本读取 stdin。
- [x] SQLite 资源模型：`databases.container_name`、旧表兼容和版本化迁移 `0007-database-container-name`。
- [x] 目标解析：优先使用 `ContainerName`，旧本地资源仅做安全兼容推断。
- [x] Compose runtime：`config/up/status/stop/restart`、固定容器名校验、状态解析和 SQLite 最近状态表。
- [x] 备份计划：PostgreSQL/MariaDB 流式备份恢复；Redis BGSAVE 等待、RDB 保护、恢复失败补偿和健康检查。
- [x] 备份元数据/运行状态迁移：`0008-database-backup-metadata`、`0009-database-runtime-states`。
- [x] 备份真实执行器/API 已补同容器互斥锁、真实 symlink/越界检查、Redis `docker cp` 临时目录原子落盘和响应脱敏。
- [x] 隔离黑盒脚本：`deploy/database/acceptance.sh`，只使用 `workmesh-acceptance-*` 资源，覆盖 Compose、健康检查、无宿主机端口、容器内 CLI、PostgreSQL/MariaDB 流式备份恢复和 Redis RDB 恢复。
- [x] 2026-09-10 已完成隔离数据库 Docker 黑盒：MariaDB、PostgreSQL、Redis、MongoDB 的真实容器生命周期、健康检查、容器内操作和备份/恢复路径均通过；临时资源已清理。
- [ ] 生产维护窗口部署和回滚。

## 当前改动

主要文件：

- `deploy/database/docker-compose.yml.tmpl`
- `deploy/database/README.md`
- `node/service/mysql_executor.go`
- `node/service/postgres_executor.go`
- `node/service/redis_executor.go`
- `node/service/database.go`
- `node/service/database_backup.go`
- `node/service/database_backup_executor.go`
- `node/service/database_runtime_service.go`
- `node/api/database_routes.go`
- `node/api/database_postgres_redis.go`
- `node/api/database_runtime.go`
- `node/api/database_backup_routes.go`
- `cmd/workmesh-server/main.go`

## 已执行验证

通过：

```text
GOWORK=off GOCACHE=/tmp/workmesh-go-build GOMAXPROCS=1 go test ./... -run '^$' -count=1
GOWORK=off GOCACHE=/tmp/workmesh-go-build GOMAXPROCS=1 go vet ./...
node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project /www/apps/workmesh-server --manifest docs/inventory/route-inventory-1panel.json
git diff --check
```

数据库相关定向测试通过：

```text
go test ./node/service -run 'TestDatabaseBackup|TestDatabaseRedisRestore' -count=1
go test ./node/api -run 'Test(DatabaseRuntime|DatabaseBackup|DatabaseTargetsPrefer|DatabaseGenericOperation|PostgresAndRedis)' -count=1
go test ./deploy/database -run TestDockerComposeTemplateKeepsDatabaseResourceContract -count=1
go test ./cmd/workmesh-server -run 'UnifiedSchemaMigrations|IsolatedMigrationRehearsal' -count=1
```

完整 `node/service` 测试在当前沙箱被已有网络测试阻断：`httptest.NewServer` 监听 IPv6 `::1` 返回 `socket: operation not permitted`。这不是数据库代码编译错误。

## 2026-09-10 后续动作

- 已新增 `deploy/database/acceptance.sh`，拿到 Docker Socket 权限后执行真实隔离验收。
- 当前工作区执行 `docker info` 仍返回 `permission denied`，因此上述证据不能替代生产 Compose、健康检查、备份恢复和发布回滚验收。
- 验收脚本不会停止、删除、重启、改端口或复用 `WorkMesh-postgresql-ZNMP`、`WorkMesh-redis-ZNMP`。

## 2026-09-12 无 Docker 权限补强

- 只读复核 `deploy/database/docker-compose.yml.tmpl`、`acceptance.sh`、`README.md`、数据库 Runtime API 和备份/运行状态代码后，确认仍不能执行真实容器生命周期，但可以补强静态生产契约。
- 已修正验收脚本对非敏感初始化变量的覆盖范围：`WORKMESH_POSTGRES_DB`、`WORKMESH_POSTGRES_USER`、`WORKMESH_MARIADB_DATABASE`、`WORKMESH_MARIADB_USER` 现在同时用于 Compose 渲染、容器内 CLI、备份和恢复命令，避免生产自定义库名/用户名时验收脚本仍硬编码 `workmesh`。
- 已修正文档命令与 Runtime API 的路径/项目契约：生产人工命令统一使用 `$WORKMESH_DATA_DIR/database/docker-compose.yml`，不再额外指定 `--project-name workmesh-panel`，避免人工部署和 API `status/stop/restart/up` 使用不同 Compose project。
- 已明确隔离验收脚本默认只清理临时容器和网络，按安全策略保留临时 named volume；这与脚本行为一致，避免误以为脚本会自动删除卷。
- 已补 `deploy/database` 静态契约测试，覆盖模板 healthcheck、secret 文件、internal 网络、无宿主端口、验收脚本自定义变量和 README Runtime API 路径契约。
- 已新增 `deploy/database/preflight.sh` 生产迁移前只读预检，检查模板固定资源名、secret 文件权限、旧 ZNMP 资源保护、运行 Compose 文件和 Docker/Compose 可用性；不执行 `up`、`stop`、`restart`、删除卷或清理操作。当前环境运行结果为 `warnings=4 failures=0`，严格模式按预期返回非零。

本轮通过：

```text
bash -n deploy/database/acceptance.sh
GOWORK=off GOCACHE=/tmp/workmesh-go-db-contract go test ./deploy/database -count=1
GOWORK=off GOCACHE=/tmp/workmesh-go-db-contract go test ./node/service -run 'Test(DatabaseRuntime|DatabaseBackup|DatabaseRedisRestore|ValidateDatabaseBackupPath|DatabaseMigration)' -count=1
GOWORK=off GOCACHE=/tmp/workmesh-go-db-contract go test ./node/api -run 'Test(DatabaseRuntime|DatabaseBackup|DatabaseTargetsPrefer|DatabaseGenericOperation|PostgresAndRedis)' -count=1
```

## 安全边界

- 当前沙箱没有 Docker Socket 权限，不能执行真实容器生命周期。
- 不得停止、删除、重启、改端口或复用旧 ZNMP 容器/卷。
- 不得使用 `docker compose down -v`、`docker volume rm` 或 `docker system prune` 作为普通回滚。
- 密码不得出现在 Docker 参数、宿主机环境快照、HTTP 响应、审计日志或进度文档。
