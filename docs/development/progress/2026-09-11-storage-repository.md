<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# SQLite Repository 抽象（2026-09-11～12）

状态：`[>]` 阶段 B 持续推进，运行期核心业务已完成一批 repository 接入

## 2026-09-12 运行期数据库边界补充

- 应用安装保存、运行时路径/元数据保存、Node 模块异步任务状态、设置/告警/快照、容器 Compose/仓库/模板以及网站扩展公共状态的运行期写失败边界已继续收口；失败时恢复内存快照并向 API 返回错误。
- Gateway 注册、登录、心跳、授权刷新和解绑的 repository 失败边界已收口；本地授权/绑定状态在保存失败时恢复，后台心跳与离线状态保存失败会记录错误。
- Website `Update` 已从域名变化专用补偿改为统一快照回滚：关系表、WAF 文件、内存状态、域名重命名目录和 `site.conf` 由同一条补偿路径恢复，补偿错误通过 `errors.Join` 暴露。
- Website 创建、更新、删除和 stream 更新的文件/内存/SQLite 回滚均有定向回归；新增域名重命名持久化失败和 WAF OpenResty 生效失败测试。
- `persistRuntimeInstallPath`、`persistRuntimeInstallFiles` 和 Node 模块后台任务不再静默忽略 `saveLocked`；新增关闭 SQLite 后的路径回滚和“状态无法落库则不执行外部 Docker 命令”测试。
- 网站扩展状态初始化不再忽略 repository/建表错误；扩展路由在公共状态保存失败时恢复 ACME、代理、认证、日志和 ID 计数快照。
- 数据库备份元数据的建表、保存和查询统一通过 `storage.SQLExecutor`；启动迁移 `0008-database-backup-metadata` 仍是生产权威建表入口，路由内幂等建表仅保留隔离调用兼容。
- PostgreSQL/Redis 运行时配置表的保存和读取统一通过 `storage.SQLExecutor`；不再从业务函数直接取得共享 `*sql.DB`。
- 验证通过：`GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./node/api -run 'Test(Database|Postgres|Redis|Query|Log|Website)' -count=1`、`go test ./node/service -run 'Test(Database|Website|SSL|ACME|DNS|Group|Cronjob)' -count=1`、编译检查和 `go vet ./...`。
- 当前沙箱的 `cmd/workmesh-server` 完整测试仍会因 `httptest.NewServer` 监听 IPv6 被拒绝而阻断；这不替代真实 HTTP、OpenResty、Docker、域名和发布验收。

## 本批完成

- `internal/storage` 新增 `SQLExecutor`，统一查询、单行查询和带上下文写入入口。
- `internal/storage` 新增 `Transactional`，以事务闭包统一提交和失败回滚边界。
- `SQLiteRepository` 适配共享 `*sql.DB`，保留 `DB()` 作为启动、迁移和旧代码兼容出口。
- `Store.Repository()` 提供共享 SQLite 的 repository 构造入口。
- `runtime/link.SQLiteSyncStore` 已改为依赖 `Transactional`，不再直接持有连接池；compare-and-set、幂等重试和冲突游标语义保持不变。
- `node/api` 日志分页、日志详情和任务日志查询已改为依赖 `SQLExecutor`；日志保留清理已通过 `Transactional` 执行，保留旧的 `*sql.DB` 入口用于兼容测试和启动代码。
- 新增 `AuditLogWriter` 和 `SQLiteAuditLogWriter`，登录尝试与 HTTP 操作审计统一走 storage writer。
- 新增 `TaskLogWriter`、`TaskStateWriter` 及 SQLite 实现，应用任务、运行时任务状态和任务输出统一走 storage writer。
- 日志清理接口的数据库删除已改为 repository 事务闭包，内存快照回滚行为保持不变。
- 新增 repository 提交/回滚测试和链路同步 repository 接入测试。
- 网站 `website_settings`、stream 配置、域名删除、HTTPS 证书读取和连接限流配置已改为使用 repository/`SQLExecutor`；数据库写入失败仍会恢复已写入的站点配置文件。
- 应用安装与运行时安装增加 checked 任务状态入口：应用状态/任务状态/首条任务日志的 SQLite 写入失败会返回或中止任务，不再静默忽略；应用任务状态和首条日志在同一事务中提交。运行时 Docker 安装进度回调也支持错误返回，持久化失败时不会继续执行下一条 Docker 命令。
- 新增 `internal/logsource` 只读文件日志 source 接口，统一多路径回退、文件/行数上限、上下文取消和文件关闭错误处理；网站访问日志、WAF JSONL、SSH 日志和系统日志文件读取已接入。
- 任务日志文件回退已从 `os.ReadFile` 改为 `internal/logsource.FileSource`，保持现有分页、`latest`、脱敏和 `taskStatus` 响应字段，同时限制单文件读取 64 MiB/100000 行。
- DNS 账户 CRUD 改为使用 `WebsiteService` repository：列表走 `SQLExecutor`，新增/更新走事务，元数据更新不会覆盖既有凭据，删除前引用检查和删除在同一事务内完成。
- `WebsiteSecurityService` 的 ACME/CA/自签证书加载与持久化改为 repository/事务闭包；ACME 删除会通过 repository 检查 `website_ssls` 引用。
- `SSLService` 的证书记录加载、批量持久化、公开 DTO 补齐、ACME/DNS 账户引用校验、HTTP-01 签发输入、DNS 签发账户读取和签发后站点证书同步改为 repository/`SQLExecutor`。
- `WebsiteService.load` 的站点、域名、网站设置、OpenResty 配置和模块读取改为 repository；HTTPS 证书读取、默认 HTML、网站删除、stream、域名限流和运行时元数据路径不再直接查询 `*sql.DB`。
- `runtimeRepository` 增加 `storage.Transactional` 适配，运行时清单/设置读取、批量保存和重启任务恢复通过 repository；保留 `db` 字段只用于旧测试构造、迁移兼容和实例身份判断。
- `DatabaseRepository` 的列表、创建、更新、删除和完整连接信息读取已改为 repository；`container_name` 迁移同时保留旧表结构兼容分支，通用 `/api/v2/databases/db` 登记入口不再强制真实连接探测。
- `DatabaseAdminStore` 的用户、授权、变量和配置业务读写已改为 repository；`*sql.DB` 仅保留给表初始化、旧 `database-admin.json` 一次性导入和离线 SQLite 连接池。
- `CronjobService` 的计划任务创建、更新、状态切换、删除、执行记录和立即执行后的运行态保存已改为 repository；执行结果持久化失败会回滚内存记录，启动建表与旧 `cronjobs.json` 一次性导入仍保留在初始化兼容边界。
- `runtime/role.Manager` 的角色切换 compare-and-set 更新已改为 repository；`role_state` 建表、首次读取和首次插入继续保留在 `NewSQLite` 初始化兼容边界。
- `GatewayStateStore` 的 SQLite 绑定状态加载、保存、刷新和解绑已改为 repository；增加本地授权快照类型，使数据库路径重启后仍能恢复 access token，token 不进入 API 响应。
- `CoreService` 的分组/设置、用户凭据和 Passkey 运行期读写以及 `node/api` 脚本库、快捷命令的查询和事务保存已改为 repository；旧 JSON/Passkey 一次性导入和表初始化仍保留兼容边界。
- `node/api/core_commands.go` 的快捷命令表初始化、查询和事务保存已通过 `storage.Transactional`；`core_resources.go` 的脚本库初始化、查询、事务保存和旧 JSON 一次性导入已通过 repository。脚本新增、更新、远程同步在保存失败时恢复内存快照。
- `CoreService` 的分组增删和设置批量更新不再忽略 repository 错误；SQLite 持久化失败时不会先修改内存快照，API 会返回明确错误。新增关闭数据库后的失败回滚测试。
- Node 运行时模块异步任务入队现在检查 `runtimeStore.saveLocked`；SQLite 不可用时返回 `503` 并回滚任务快照，不会在未持久化的情况下启动模块操作。
- 控制面 `RoleController` 的 `role_nodes` 加载、节点增删改/收藏和快照保存已改为 repository 事务；节点表建表以及无共享数据库时的 `nodes.json` 回退仍保留兼容边界。
- 共享状态层的 `loadJSONState/saveJSONState`、节点设置、应用关系和容器 Compose/镜像仓库/模板关系写入已改为 repository 查询或事务；未注入公共数据库时保持原文件/内存兼容行为。
- 主机列表、详情/凭据读取、SSH 密钥证书 CRUD/同步/搜索和 `GroupService` 运行期列表/增删改已改为 repository；主机/分组表初始化及旧分组 JSON 导入仍留在兼容边界。
- 数据库操作记录、数据库运行时配置读写和数据库删除后的配置清理已改为 repository；运行时配置表的建表检查仍使用启动兼容入口。
- 后台计划任务启用计数、数据库备份记录查询/保存和 MongoDB 删除依赖预检已改为 shared repository；备份元数据表建表仍保留路由初始化兼容入口。
- 新增 `internal/logsource.FileMaintenance`，统一 WAF JSONL、SSH auth/secure 及站点 access/error 文件清理的截断、删除、缺失文件和普通文件安全边界。
- website `persist` 在写入关系表前对旧 `payload` 列的兼容探测已改为 repository 查询；启动建表、逐列补齐和 migration 回调继续使用 `*sql.DB`/`*sql.Tx`。
- 网站模板和模板产物的分页查询、详情、创建、更新、删除、最新模板读取和名称冲突检查已改为 repository；建表和旧 `website_extension_state` BLOB 一次性导入仍留在 migration 边界。

## 验证证据

通过：

```text
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./internal/storage -count=1
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./runtime/link -run '^TestSQLiteSyncStoreUsesRepositoryTransactionBoundary$' -count=1
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./runtime/link -run '^$' -count=1
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./node/api -run 'Test(Query|RunLogRetention|LogRetention|LoginLog|TaskLog)' -count=1
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./node/service ./node/api -run 'Test(SSL|ACME|DNSAccountRepository|WebsiteSecurityRepository|TaskLogFileFallback|SystemLog)' -count=1
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./... -run '^$' -count=1
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go vet ./internal/storage ./runtime/link ./node/api
node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project /www/apps/workmesh-server --manifest docs/inventory/route-inventory-1panel.json
git diff --check
```

2026-09-12 本轮验证：

```text
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./node/api -run 'Test(Script|Command)' -count=1    PASS
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./node/service -run 'Test(Group|Cronjob|Database|Website|SSL|ACME|DNS)' -count=1    PASS
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./control/service ./node/api -run 'Test(CoreGroup|CoreAPI|CoreAuth|CorePasskey)' -count=1    PASS
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./node/api -run 'Test(NodeModuleQueue|Runtime|Core)' -count=1    PASS
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./... -run '^$' -count=1    PASS
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go vet ./internal/storage ./runtime/link ./node/api ./node/service    PASS
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./node/service -run 'TestWebsite(Update|Delete|Create|Stream)' -count=1    PASS
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go vet ./node/service    PASS
node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project /www/apps/workmesh-server --manifest docs/inventory/route-inventory-1panel.json    PASS (759)
npm run type-check    PASS
git diff --check    PASS
```

补充复核：`GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./...` 的非编译业务执行仍被当前沙箱禁止 IPv6 loopback 阻断，多个既有 `httptest.NewServer` 用例在 `listen tcp6 [::1]:0` 处失败；不是本轮保存回滚代码的断言失败。`go vet ./...`、`go test ./... -run '^$'`、前端类型检查和 759 条路由扫描通过。

`TestScriptSyncUsesConfiguredRemoteAndSQLite` 在当前沙箱因 loopback 监听被内核拒绝而明确跳过；这不是脚本 repository 代码失败。完整业务测试仍按下方外部环境限制处理。

`runtime/link` 完整业务测试仍受当前沙箱禁止 IPv6 `httptest` 监听影响；这是环境限制，不是本批 repository 编译错误。

2026-09-12 外部验收条件复核：当前沙箱可执行 Docker CLI，但访问 `/var/run/docker.sock` 被拒绝；未找到 `openresty`/`nginx` 命令；`znmp.sopvip.com` 无法解析。因此真实 OpenResty reload、域名 HTTPS/Host 路由、ACME 和 Docker Compose 黑盒仍不能标记完成。

## 尚未完成

- 业务域仍有少量 `*sql.DB` 直接依赖，主要集中在网站/运行时启动建表、逐列迁移、旧 JSON/BLOB 导入和测试夹具；website 和 runtime 的完整 application/repository 边界仍未全部抽离。
- website 启动建表、逐列补齐和 migration 回调仍直接使用 `*sql.DB`/`*sql.Tx`，本批只迁移运行期业务读写与旧表兼容探测。
- 文件型日志已统一到 `internal/logsource.Source` 的读取边界，WAF、SSH、站点日志清理已统一到 `FileMaintenance`；各领域解析/筛选策略仍保留在对应 application/service 中，`journalctl` 读取和轮转策略保持显式外部依赖。
- 网站和 runtime 资源状态写入仍有直接 SQLite 调用，尚未全部迁移到 application/repository 接口。
- `DatabaseAdminStore` 的启动建表和旧 JSON 导入仍然直接使用 `*sql.DB`；这属于迁移/兼容边界，尚未抽成独立 migration package。
- control/application 其余状态仓库仍有直接 `*sql.DB` 读写，尚未进入本批次；主机、角色节点、Gateway/Core 分组设置、脚本库、快捷命令和数据库运行时配置已通过 repository 读取。
- Gateway 与网站本轮运行期错误边界已完成，但真实双节点 Gateway 联调、生产 OpenResty reload、域名/ACME、Docker daemon 和发布回滚仍未执行；这些保持 `not-run`，不能由隔离测试替代。
- `go test ./... -run '^$'`、`go vet ./...`、路由契约和 `git diff --check` 已通过；完整业务/race 测试仍受当前沙箱 IPv6 监听限制，真实 Docker/OpenResty、域名/ACME 和生产数据库验收仍未执行。
- 目录物理迁移仍需等接口边界稳定后分批进行，不能因新增接口就删除现有 `control`、`node`、`runtime` 兼容包。

## 下一批

1. 将 runtime 外部资源状态和其他业务域剩余 SQLite 访问迁移到 repository，保持现有启动迁移和兼容出口。
2. 评估文件型日志的轮转/容量维护是否由 WorkMesh 托管；当前清理动作已统一，外部 logrotate/journalctl 仍保持显式外部依赖。
3. 每批执行对应包测试、编译、路由契约和差异检查，再考虑物理目录迁移。
