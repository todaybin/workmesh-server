<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# SEC-02 前端菜单路由缺口矩阵（2026-09-08）

本清单为只读静态审计，不发送 HTTP 请求、不修改业务代码。前端参数来源是
`web/src/api/modules/*.ts`，参考接口总表是
`docs/inventory/frontend-api-inventory.json`；后端实现来源是 `node/api` 的显式
`HandleFunc` 注册和 `legacy_routes_*.go` 兼容注册。

## 状态定义

| 状态 | 含义 | 验收要求 |
| --- | --- | --- |
| `implemented` | 已发现专用处理器，且返回真实 SQLite/主机资源 | 用真实库和真实资源做 HTTP 黑盒测试 |
| `partial` | 有处理器但仍有兼容别名、固定空值、外部资源未验证或同一路径多操作未覆盖 | 补参数变体、失败回滚和持久化测试 |
| `501` | legacy 契约声明为 `fallbackRouteHandler`，或显式处理器明确返回 `NOT_IMPLEMENTED` | 必须替换为真实处理器，禁止伪造成功 |
| `not-run` | 静态上有处理器，但尚未执行真实 HTTP/WS 测试 | 纳入发布前黑盒矩阵 |

注意：`legacy_routes_*.go` 的声明数量是兼容债务数量，不等于线上实际 501 数量。
`legacyFilterMux` 会按 `isWebsiteFunctionalRoute`、`isRuntimeToolboxRoute` 等规则过滤重复
注册；必须通过集成请求确认最终 ServeMux 命中的处理器。

## 分域总览

| 前端菜单/领域 | 前端唯一接口 | 前端方法数 | legacy fallback 声明 | 当前静态判断 | 参数来源 | 真实实现入口 | 下一步 |
| --- | ---: | ---: | ---: | --- | --- | --- | --- |
| 网站 | 79 | 81 | 120 | `partial` / `not-run` | `web/src/api/modules/website.ts`、`website-monitor.ts` | `registerWebsiteFunctionalRoutes`、`website_*` | 优先验证建站、域名、SSL、OpenResty、WAF、监控和日志 |
| 运行时 | 23 | 23 | 29 | `partial` / `not-run` | `web/src/api/modules/runtime.ts` | `registerRuntimeRoutes`、`runtime_*` | Go/Node/Python/Java/.NET/PHP 生命周期闭环 |
| 数据库 | 17 | 17 | 53 | `partial` / `not-run` | `web/src/api/modules/database.ts` | `registerDatabaseRoutes`、`RegisterDatabaseAdminRoutes` | SQLite 元数据与真实数据库操作逐项验证 |
| 容器 | 23 | 23 | 70 | `partial` / `not-run` | `web/src/api/modules/container.ts` | `registerContainerRoutes`、`containers_*` | Docker 可用时验证容器、镜像、Compose、日志 SSE |
| 系统/主机 | 44 | 42 | 69 | `partial` / `not-run` | `web/src/api/modules/host.ts`、`firewall.ts`、`terminal.ts` | `registerHostRoutes`、`registerHostOperationalRoutes` | 主机监控、防火墙、SSH、终端 WS 真实验证 |
| 计划任务 | 4 | 4 | 12 | `partial` / `not-run` | `web/src/api/modules/cronjob.ts` | `registerCronRoutes` | 创建、执行、停止、记录和清理验证 |
| 日志审计 | 5 | 5 | 11 | `partial` / `not-run` | `web/src/api/modules/log.ts` | `registerLogRoutes`、`logs_queries.go`、`functional_system_logs.go` | 操作、访问、系统、任务、登录日志分页和清理验证 |
| 面板设置 | 11 | 11 | 26 | `partial` / `not-run` | `web/src/api/modules/setting.ts`、`terminal.ts` | `registerSettingsRoutes`、`functional_settings.go` | 设置读写、SSH、快照、文件历史和升级契约验证 |

## 501 缺口与优先级

逐条路由映射（前端方法、推断 HTTP 方法、真实处理器文件、fallback 文件、参数来源和静态测试证据）已由 [SEC-03 明细](/www/apps/workmesh-server/docs/development/progress/2026-09-08-route-gap-detail.md) 生成。该明细覆盖本矩阵中全部 P0/P1 领域；`not-run` 仍必须通过真实 HTTP/WS/SSE 验收后才能升级为完成。

当前静态结果：网站 81 条（501/缺失 33、partial 34、not-run 14），运行时 23 条（8、14、1），数据库 17 条（8、7、2），容器 23 条（17、5、1），系统/主机 42 条（35、5、2），计划任务 4 条（1、2、1），日志 5 条（4、1、0），设置 11 条（8、3、0）。这些数字按前端方法展开，和上方按唯一 URL 统计的菜单总览可能不同。

下表列出当前最影响正式上线的路由族。路径中的 `{id}`、`{type}` 等是前端实际
参数变体；同一 URL 的 `operate`、`type`、`source`、`scope` 必须作为独立测试案例。

| 优先级 | 菜单 | 方法与路由/路由族 | 静态状态 | 前端参数来源 | 处理器/缺口 | 下一步验收 |
| --- | --- | --- | --- | --- | --- | --- |
| P0 | 网站 | `POST /api/v2/websites`、`/search`、`/update`、`/del`、`/operate` | `partial` | `website.ts`: `WebsiteCreate`, `WebsiteUpdate`, `WebsiteOperate` | CRUD 已有专用实现；legacy 同路径声明仍存在 | 真实 SQLite 创建、主/次站点、启停/重启/删除后重建 |
| P0 | 网站 | `POST /api/v2/websites/{id}/https`、`GET /api/v2/websites/{id}/https` | `partial` | `website.ts`: `enable`, `websiteSSLId`, `httpConfig`, `operate` | SSL/HTTPS 处理器存在，ACME 及 OpenResty 生效未完成黑盒验证 | Let's Encrypt HTTP 签发、nginx -t、域名访问、失败回滚 |
| P0 | 网站 | `POST /api/v2/websites/domains*`、`GET /api/v2/websites/domains/{websiteId}` | `partial` | `website.ts`: `domain`, `websiteId`, `operate` | 域名处理器存在，主/次站点组合场景未验证 | 独立 `*.cs.sopvip.com` 前缀绑定并访问 |
| P0 | 网站 | `POST /api/v2/websites/templates/*` | `partial` / `501` | `website.ts`: template、upload、preview、outputs | 模板部分有专用仓储；legacy 仍声明多条 fallback | 上传真实 zip、预览、输出持久化和删除 |
| P0 | 网站 | `POST /api/v2/websites/waf/*`、`GET /api/v2/websites/waf/*` | `partial` | `website.ts`、`waf.ts` | WAF 专用路由存在；攻击/封禁/关系统计部分仍是兼容路由 | 规则增删、站点绑定、真实 OpenResty/WAF 请求验证 |
| P0 | 网站 | `POST /api/v2/websites/monitor/*`、`/log/*` | `partial` | `website-monitor.ts` | 监控别名和日志处理器存在；统计数据源和清理未完整验收 | 访问日志、QPS、趋势、访客、清理和分页黑盒测试 |
| P0 | 运行时 | `POST /api/v2/runtimes`、`/operate`、`/del`、`GET /runtimes/{id}` | `partial` | `runtime.ts`: `RuntimeCreate`, `RuntimeOperate` | 通用运行时处理器存在；六类环境尚未全生命周期实测 | 每类创建/详情/停止/启动/重启/日志/删除/重建 |
| P0 | 运行时 | `POST /api/v2/runtimes/php/extensions/*`、`/php/fpm/*` | `partial` | `runtime.ts`: PHP extension/FPM 参数 | PHP 路由存在，FPM、扩展和站点访问未闭环 | 安装/卸载扩展、配置保存、FPM 重启和访问 |
| P1 | 数据库 | `POST /api/v2/databases`、`/del`、`/load`、`/change/*` | `partial` | `database.ts`: `DatabaseCreate`, `DatabaseOperate` | 管理路由存在；MySQL/PostgreSQL/Mongo 等外部服务依赖未验证 | 真实数据库创建、凭据变更、删除确认和恢复 |
| P1 | 数据库 | `POST /api/v2/databases/users/*`、`grants/*`、`variables*` | `implemented` / `not-run` | `database.ts` 用户、权限、变量请求 | `database_admin_routes.go` 有专用处理器 | 逐项验证参数名、权限结果和 SQLite 元数据 |
| P1 | 容器 | `POST /api/v2/containers`、`/operate`、`/update`、`/del` | `partial` | `container.ts`: `ContainerCreate`, `ContainerOperate` | 容器主路由存在；依赖 Docker 守护进程 | 创建、启停、重命名、删除、重建持久化 |
| P1 | 容器 | `POST /api/v2/containers/compose/*`、`GET/POST /containers/search/log` | `partial` | `container.ts`、日志组件 query | Compose 和 SSE 有处理器；SSE 断线/续传尚未验证 | 真实 Compose、日志 `Last-Event-ID`、超时和权限 |
| P1 | 系统 | `POST /api/v2/hosts/firewall/*`、`GET /hosts/firewall/*` | `partial` / `not-run` | `firewall.ts`: operation、rule、policy | 防火墙处理器存在，系统命令和回滚未验证 | 规则增删、Docker 端口策略、失败回滚 |
| P1 | 系统 | `GET /api/v2/hosts/terminal/{local,container,ssh}` | `partial` | `terminal.ts`、终端 WS query | WS 处理器和节点 relay 存在 | 未授权、Origin、命令参数、断线和资源释放 |
| P1 | 计划任务 | `POST /api/v2/cronjobs`、`/update`、`/del`、`/handle`、`/stop` | `partial` | `cronjob.ts`: `CronjobCreate`, `CronjobOperate` | CRUD/执行处理器存在；记录和停止仍有 legacy 声明 | 真任务执行、任务日志、停止、重启后恢复 |
| P1 | 日志 | `POST /api/v2/core/logs/{operation,login,clean}` | `501` / `partial` | `log.ts` 和日志页面 | legacy core 日志仍有 fallback；统一日志接口另有实现 | 统一迁移到真实 SQLite 日志表并保持前端字段 |
| P1 | 日志 | `POST /api/v2/logs/{search,detail,stat,clear}` | `partial` / `not-run` | `log.ts`、各日志页面分页参数 | `functional_logs.go` 有处理器，真实日志源和分页未验收 | 操作/访问/系统/任务/登录五类真实记录、筛选、清理 |
| P1 | 设置 | `POST /api/v2/settings/update`、`search`、`/core/settings/*` | `partial` | `setting.ts`: key/value、search、scope | 基础设置处理器存在；core 兼容项仍有 fallback | 读写后重启持久化，字段和错误 envelope 对齐 |
| P2 | 设置 | `GET/POST /api/v2/settings/snapshot*`、`file-history*` | `501` / `partial` | `setting.ts`: snapshot/file-history 参数 | legacy 声明多，专用处理器覆盖不完整 | SQLite 快照、导入、恢复、回滚真实验证 |

## 方法与参数契约重点

1. 前端所有 URL 通过 `web/src/api/transport.ts` 归一到 `/api/v2`；服务端不再新增 v1 路由。
2. `operate`、`type`、`source`、`scope`、`logType` 等共享 URL 参数必须分别记录请求摘要和响应摘要，不能只测 URL 是否返回 200。
3. `DOWNLOAD`、`UPLOAD`、`POSTLOCALNODE`、`POSTWITHCONFIG` 是前端传输层语义，不是 HTTP 方法；验收时分别映射到真实 GET/POST/multipart/节点 relay，并记录最终 HTTP 方法。
4. 所有写接口必须使用真实 SQLite 或真实主机/Docker/OpenResty 资源；`501`、空列表或固定对象不能标记为 `implemented`。

## 执行顺序

1. P0 网站与运行时：先完成域名、HTTPS、OpenResty/WAF、六类运行环境和 WebSocket/SSE。
2. P1 数据库、容器、系统、计划任务、日志：依赖真实主机和 SQLite，按生命周期闭环执行。
3. P2 设置与快照：完成字段迁移、重启持久化和升级兼容后再发布。
4. 每项测试记录到发布矩阵：路由、HTTP 方法、状态码、请求摘要、响应摘要、真实资源结果、配置路径、失败原因和未覆盖项。

## 静态复核命令

```bash
# 前端请求定义及来源
rg -n 'http\\.(get|post|put|delete)\\(' web/src/api/modules

# legacy 兼容声明
rg -n 'fallbackRouteHandler' node/api/legacy_routes_*.go

# 专用处理器注册
rg -n 'HandleFunc\\("|func register' node/api/{website,runtime,database,containers,hosts,cron,functional_logs,functional_settings}*.go
```
