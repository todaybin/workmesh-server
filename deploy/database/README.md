<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WorkMesh 面板数据库 Compose 模板

本目录提供生产数据库容器的静态 Compose 模板，不包含真实密码，不负责创建、启动或删除真实容器。

拿到 Docker 权限后，可用隔离验收脚本执行完整的临时资源黑盒。脚本只使用
`workmesh-acceptance-*` 资源，默认结束后清理临时容器和网络；临时 named volume
按安全策略保留，需由操作者核对后单独清理：

```bash
deploy/database/acceptance.sh
```

生产迁移前先执行只读预检。它只检查模板、运行目录、secret 权限、固定资源名、
旧 ZNMP 资源保护和 Compose 静态配置，不会创建、启动、停止、删除容器或卷：

```bash
deploy/database/preflight.sh
```

严格模式用于发布门禁：

```bash
STRICT=1 deploy/database/preflight.sh
```

可通过 `WORKMESH_DATA_DIR`、`WORKMESH_DB_ROOT`、`WORKMESH_DB_COMPOSE_FILE` 和
`WORKMESH_DB_SECRETS_DIR` 指向实际运行目录。预检通过不等于数据库已健康；真实
`up`、健康检查、备份恢复和回滚仍需在维护窗口执行。

如需保留失败现场，设置 `WORKMESH_ACCEPTANCE_KEEP=1`；此时只能由操作者按脚本输出的
临时资源名称单独清理，不得清理旧的 ZNMP 资源。

模板定义三个独立服务：

| 服务 | 固定容器名 | 镜像 | named volume |
| --- | --- | --- | --- |
| PostgreSQL | `workmesh-panel-postgres` | `postgres:18-alpine` | `workmesh-panel-postgres-data` |
| Redis | `workmesh-panel-redis` | `redis:8-alpine` | `workmesh-panel-redis-data` |
| MariaDB | `workmesh-panel-mariadb` | `mariadb:11` | `workmesh-panel-mariadb-data` |

这里的 MariaDB 是当前模板提供的 MySQL 协议兼容实现，供前端“ MySQL 或
MariaDB ”数据库能力使用；模板没有提供 Oracle MySQL `mysql:*` 服务。若生产
要求必须使用 Oracle MySQL，应在独立受管环境部署并通过外部数据库登记流程接入，
不要修改本模板的固定服务名、容器名、网络或数据卷契约。

三个服务只加入独立网络 `workmesh-panel-db`，模板不发布宿主机端口。服务之间通过 Docker 网络访问，不能使用宿主机 `3306`、`5432` 或 `6379` 作为管理接口。

## 安装前检查

安装操作必须在明确的维护窗口执行，并由有 Docker 权限的运维人员完成。当前仓库任务只新增模板，不执行下面的生产命令。

1. 确认 Docker 和 Compose 可用：

   ```bash
   docker version
   docker compose version
   ```

2. 确认旧的 WorkMesh 数据库容器不被触碰：

   ```bash
   docker ps -a \
     --filter name=WorkMesh-postgresql-ZNMP \
     --filter name=WorkMesh-redis-ZNMP \
     --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'
   ```

   `WorkMesh-postgresql-ZNMP` 和 `WorkMesh-redis-ZNMP` 是现有生产资源。禁止停止、删除、重启、改端口、改挂载或复用其数据卷。

3. 检查宿主机端口仅用于确认没有错误地准备端口映射：

   ```bash
   ss -ltnH | grep -E ':(3306|5432|6379)([[:space:]]|$)' || true
   ```

   本模板不需要这些宿主机端口处于空闲状态，因为没有任何 `ports:` 配置。发现端口占用时不能通过修改模板增加端口映射。

4. 检查固定名称是否已有资源：

   ```bash
   docker ps -a --format '{{.Names}}' | grep -E \
     '^(workmesh-panel-postgres|workmesh-panel-redis|workmesh-panel-mariadb)$' || true
   docker network ls --format '{{.Name}}' | grep '^workmesh-panel-db$' || true
   docker volume ls --format '{{.Name}}' | grep -E \
     '^workmesh-panel-(postgres|redis|mariadb)-data$' || true
   ```

   如果这些名称已经存在，先完成资源归属和数据核对；禁止强制覆盖或删除未知资源。

5. 准备运行目录和 secret 文件。secret 文件由外部 secret manager 或维护人员安全写入，内容必须是单行密码并设置为 `0600`。本仓库不提供密码值：

   ```bash
   export WORKMESH_DATA_DIR=/opt/workmesh-server/data
   export WORKMESH_DB_ROOT="$WORKMESH_DATA_DIR/database"
   export WORKMESH_DB_SECRETS_DIR="$WORKMESH_DB_ROOT/secrets"
   install -d -m 0700 "$WORKMESH_DB_ROOT" "$WORKMESH_DB_SECRETS_DIR"
   ```

   必须存在且非空的文件：

   ```text
   $WORKMESH_DB_SECRETS_DIR/postgres_password
   $WORKMESH_DB_SECRETS_DIR/redis_password
   $WORKMESH_DB_SECRETS_DIR/mariadb_password
   $WORKMESH_DB_SECRETS_DIR/mariadb_root_password
   ```

   检查文件权限和非空状态时不要打印内容：

   ```bash
   for secret in \
     postgres_password \
     redis_password \
     mariadb_password \
     mariadb_root_password; do
     test -s "$WORKMESH_DB_SECRETS_DIR/$secret" || {
       printf 'missing or empty secret: %s\n' "$secret" >&2
       exit 1
     }
     chmod 0600 "$WORKMESH_DB_SECRETS_DIR/$secret"
   done
   ```

## 生成并校验 Compose 文件

将模板复制到运行目录；不要直接修改仓库中的模板：

```bash
install -m 0644 \
  deploy/database/docker-compose.yml.tmpl \
  "$WORKMESH_DB_ROOT/docker-compose.yml"
```

可选环境变量只用于非敏感的数据库初始化名称和用户名称：

```bash
export WORKMESH_POSTGRES_DB=workmesh
export WORKMESH_POSTGRES_USER=workmesh
export WORKMESH_MARIADB_DATABASE=workmesh
export WORKMESH_MARIADB_USER=workmesh
```

执行静态配置校验。该命令会检查 Compose 语法和 secret 文件引用，不启动容器：

```bash
docker compose \
  -f "$WORKMESH_DB_ROOT/docker-compose.yml" \
  config --quiet
```

校验展开后的配置时，不要把输出写入公共日志，因为 Compose 配置可能包含环境变量值。不要使用 `docker compose config --environment` 作为生产日志输出。
人工命令和 WorkMesh Runtime API 都必须使用同一个 `$WORKMESH_DATA_DIR/database/docker-compose.yml`；不要额外指定 `--project-name`，否则人工启动的 Compose 项目可能和 API 后续的 `status/stop/restart/up` 操作不一致。

## 启动与状态检查

只有完成安装前检查、secret 检查和 `config --quiet` 后才允许启动：

```bash
docker compose \
  -f "$WORKMESH_DB_ROOT/docker-compose.yml" \
  up -d
```

查看服务和健康状态：

```bash
docker compose \
  -f "$WORKMESH_DB_ROOT/docker-compose.yml" \
  ps
```

确认没有宿主机端口发布：

```bash
docker inspect \
  workmesh-panel-postgres \
  workmesh-panel-redis \
  workmesh-panel-mariadb \
  --format '{{.Name}} ports={{json .NetworkSettings.Ports}}'
```

预期三个容器的 `ports` 都为空对象。若健康检查失败，先查看脱敏后的容器日志和 `docker inspect` 状态；不要把 secret 内容复制到日志、工单或命令参数中。

## WorkMesh Runtime API

服务启动时会按以下顺序执行数据库相关迁移：

```text
0006-website-relational
0007-database-container-name
0008-database-backup-metadata
0009-database-runtime-states
```

每个迁移的 ID 和 checksum 都是 SQLite 兼容性契约。已应用迁移不能修改 SQL、ID 或 checksum；重复启动只会执行 noop 审计，不会重复创建表或索引。运行时状态 store 中的 `CREATE TABLE IF NOT EXISTS` 只是隔离调用方的防御性保护，正式启动的权威 schema 来源是 `0009-database-runtime-states`。

数据库 Compose 运行时接口如下：

```text
GET  /api/v2/databases/runtime/status
POST /api/v2/databases/runtime/status
POST /api/v2/databases/runtime/config
POST /api/v2/databases/runtime/up
POST /api/v2/databases/runtime/stop
POST /api/v2/databases/runtime/restart
```

请求体可以传入相对于 `$WORKMESH_DATA_DIR/database` 的 `path`；省略时使用 `docker-compose.yml`。路径不能越过数据库运行目录、不能通过符号链接逃逸，也不能指向目录。`config`、`up`、`stop` 和 `restart` 都会先校验 Compose 配置；校验失败时不会执行 `up`、`stop` 或 `restart`。`status` 会读取 Compose `ps` 和容器 inspect，返回三个固定容器的 `status`、`health`、镜像、端口映射、退出码和错误；缺失容器会明确标记为 `missing`。

运行时 API 的边界：

- `config` 只返回配置有效，不通过 HTTP 返回展开后的 Compose 内容，避免环境值或 secret 路径进入响应。
- 允许的操作只有 `config`、`up`、`stop`、`restart` 和 `status`；不提供 `down`、`down --volumes` 或卷删除操作。
- Docker 不可用、配置无效、路径不安全或命令非零退出时返回错误；这不等同于数据库已经健康。
- API 只管理本模板的三个固定容器，不管理 `WorkMesh-postgresql-ZNMP`、`WorkMesh-redis-ZNMP` 或其他 Compose 项目。

## 备份和恢复

数据库备份接口当前是同步执行接口：

```text
POST /api/v2/databases/backup
POST /api/v2/databases/restore
GET  /api/v2/databases/backups
```

请求按已登记的数据库资源选择目标容器，备份文件只能写入受控目录：

```text
$WORKMESH_DATA_DIR/backups/databases
```

未设置 `WORKMESH_DATA_DIR` 时使用 `./data/backups/databases`。备份文件名不能是绝对路径、不能越界、不能是符号链接；备份记录保存操作、状态、大小和 SHA-256，不保存明文密码。

当前备份流程的数据库类型边界：

- PostgreSQL 使用容器内 `pg_dump` / `pg_restore`，备份输出流入文件，恢复文件通过 stdin 进入容器。
- MariaDB 使用容器内 `mariadb-dump` / `mariadb`，备份和恢复同样使用受控文件流。
- Redis 先执行 `BGSAVE`，轮询 `INFO persistence` 确认保存完成且成功，再复制 `dump.rdb`。
- 密码不放入 Docker 参数；执行输出和 API 执行结果会做脱敏。
- 当前接口在请求完成前同步等待命令结束，不应把 HTTP 请求当作独立的长期任务队列。

## 回滚边界

Compose 配置回滚和数据库内容恢复是两种不同操作：

- Compose 文件回滚只恢复服务定义，然后重新执行 `config` 和 `up`；它不回滚数据库内容，也不自动切换 named volume。
- 停止或移除本模板容器必须保留 `workmesh-panel-postgres-data`、`workmesh-panel-redis-data` 和 `workmesh-panel-mariadb-data`。普通回滚禁止 `down -v`、`docker volume rm` 和 `docker system prune`。
- Redis 恢复计划会在替换 RDB 前保存 `.pre-restore` 文件；复制、启动或健康检查失败时尝试停止容器、写回旧 RDB、重启并执行 `PING`。这只覆盖本次 RDB 恢复流程，不能保证外部手工修改或卷损坏时可恢复。
- PostgreSQL 和 MariaDB 恢复使用 `--clean` 或 SQL 导入修改目标数据库；失败后没有通用的数据库事务级自动回滚，必须先保留可验证的独立备份。
- 备份文件、secret、named volume 和旧 ZNMP 容器不属于同一个回滚边界。删除它们需要单独的人工变更批准和已验证的恢复方案。

## 停止、升级和回滚

正常停止只停止本模板管理的三个服务，保留 named volumes：

```bash
docker compose \
  -f "$WORKMESH_DB_ROOT/docker-compose.yml" \
  stop postgres redis mariadb
```

恢复运行：

```bash
docker compose \
  -f "$WORKMESH_DB_ROOT/docker-compose.yml" \
  start postgres redis mariadb
```

升级模板前先保留当前文件和数据库备份：

```bash
install -m 0644 \
  "$WORKMESH_DB_ROOT/docker-compose.yml" \
  "$WORKMESH_DB_ROOT/docker-compose.yml.bak"
```

回滚 Compose 配置时，恢复之前的文件并重新校验、启动：

```bash
install -m 0644 \
  "$WORKMESH_DB_ROOT/docker-compose.yml.bak" \
  "$WORKMESH_DB_ROOT/docker-compose.yml"
docker compose \
  -f "$WORKMESH_DB_ROOT/docker-compose.yml" \
  config --quiet
docker compose \
  -f "$WORKMESH_DB_ROOT/docker-compose.yml" \
  up -d
```

如果需要撤销本次容器部署但保留数据卷，只移除本模板声明的容器：

```bash
docker compose \
  -f "$WORKMESH_DB_ROOT/docker-compose.yml" \
  stop postgres redis mariadb
docker compose \
  -f "$WORKMESH_DB_ROOT/docker-compose.yml" \
  rm --force postgres redis mariadb
```

禁止执行以下破坏性操作作为普通回滚步骤：

```text
docker compose down -v
docker volume rm workmesh-panel-postgres-data
docker volume rm workmesh-panel-redis-data
docker volume rm workmesh-panel-mariadb-data
docker system prune
```

这些操作可能永久删除数据或影响其他 Docker 资源。删除 named volumes、清理旧备份或修改现有 ZNMP 容器必须有单独的人工变更批准和可验证的恢复方案。

## 资源和安全约束

- 不要给三个服务添加 `ports:`、`network_mode: host` 或宿主机数据库 CLI 依赖。
- 不要把密码放入 `.env`、Compose 环境变量、命令参数、Git 文件、日志或工单。
- `POSTGRES_PASSWORD_FILE`、`MARIADB_*_PASSWORD_FILE` 和 Docker secret 文件路径是模板接口；Redis 启动命令从 `/run/secrets/redis_password` 读取密码。
- 不要复用 `WorkMesh-postgresql-ZNMP`、`WorkMesh-redis-ZNMP` 的网络、容器、卷或配置。
- 固定容器名冲突、固定网络冲突、固定卷归属不明或 Docker 权限不足时，应停止操作并先处理冲突。
- 该模板只负责三个数据库容器的基础生命周期；Runtime API、备份恢复流程和 SQLite 资源登记由 WorkMesh Server 负责。
