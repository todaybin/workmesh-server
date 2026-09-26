<!-- SPDX-License-Identifier: GPL-3.0-only -->

## 2026-08-31 运行数据目录统一
| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 多功能域共享运行数据根目录 | `/www/apps/1Panel/core/init`、`agent/init` 及各 service 初始化 | 所有登录、网关绑定、主机、容器、数据库、文件、任务和网站接口 | `config/config.go` 与各模块 `WORKMESH_DATA_DIR` 回退逻辑 | 不新增 HTTP 路由；统一默认目录为 `./data` | 沿用各功能域原鉴权 | 本地配置和功能域状态文件 | 显式目录优先；未配置时全部写入 `./data`，文件使用原子 rename | `go test ./...` 覆盖各模块重启加载测试 | `node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test ./..."` | 待主次节点验证数据目录权限和重启恢复 | implemented | 既有部署若使用 `.workmesh-data`，需按部署手册迁移一次历史文件 |

## 2026-08-31 主机连接测试真实探测
| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 主机按参数连接测试 | `/www/apps/1Panel/agent/app/service/host.go:TestByInfo` | `POST /api/v2/hosts/test/byinfo` | `node/api/hosts.go:hostForConnectionTest`、`probeHost` | POST `/api/v2/hosts/test/byinfo` | 本地节点 Session/Bearer/API Key | 请求中的 addr/address 与 port，TCP DialContext 2 秒超时 | 无状态探测 | `node/api/hosts_connection_test.go:TestHostConnectionTestByInfoProbesTCPPort`、`TestHostConnectionTestRejectsInvalidPort` | `node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test ./node/api -run HostConnection"` | 待主次节点网络联调 | implemented | 当前仅验证 TCP 可达性；SSH 用户、密码和密钥认证需配置受控 SSH 适配器后启用 |
| 已保存主机连接测试 | `/www/apps/1Panel/agent/app/service/host.go:TestLocalConn` | `POST /api/v2/hosts/test/byid` | `node/api/hosts.go:hostForConnectionTest`、`probeHost` | POST `/api/v2/hosts/test/byid` | 本地节点 Session/Bearer/API Key | `WORKMESH_DATA_DIR/hosts.json` 中已保存主机地址和端口 | 主机记录原子替换保存；测试本身无状态 | `node/api/hosts_connection_test.go` | 同上 | 待主次节点网络联调 | implemented | 当前仅验证 TCP 可达性；不会读取或回传保存的敏感凭据 |

## 2026-08-31 首页快速入口持久化
| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 快速跳转和应用启动器配置 | `/www/apps/1Panel/agent/app/service/dashboard.go:ChangeQuick`、`ChangeShow`、`ListLauncherOption` | `POST /api/v2/dashboard/quick/change`、`POST /api/v2/dashboard/app/launcher/show`、`POST /api/v2/dashboard/app/launcher/option`、`GET /api/v2/dashboard/quick/option` | `node/api/dashboard.go:handleDashboardMutation`、`dashboardQuickJumps`、`handleDashboardLauncherOption` | 同左 | 全局节点 Session/Bearer/API Key；写请求受 CSRF 中间件保护 | 请求中的快速入口数组、启动器 key/status | `WORKMESH_DATA_DIR/domains.json.settings`，原子临时文件 rename；读取后重启可恢复 | `node/api/dashboard_test.go:TestDashboardQuickJumpChangePersistsAndFiltersLauncher`、`TestDashboardLauncherOptionIncludesHiddenState`、`TestDashboardQuickJumpChangeRejectsInvalidVisibleCount` | `node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test ./node/api -run Dashboard"` | 待主次节点页面联调 | implemented | 应用目录接入真实安装记录后可扩展启动器详情；当前配置和过滤行为已完整持久化 |

## 2026-08-31 网站监控访问日志统计
| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 访问日志统计与访客排行 | `/www/apps/1Panel/agent/app/service/website_monitor.go:Stat/QPS/Rank/Visitors` | `POST /api/v2/stat`、`/qps`、`/rank`、`/visitors`、`/visitors/loc`、`/attack/stat`、`/block/search`、`/relation/stat` | `node/api/analytics.go:loadAnalyticsEvents`、`analyticsDaily`、`analyticsRank` | 同左及 `/api/v2/websites/monitor/*`、`/api/v2/xpack/monitor/*` 别名 | 全局节点 Session/Bearer/API Key | 配置的 `WORKMESH_ANALYTICS_LOG` 或数据目录/var/log Nginx/OpenResty combined access.log | 无状态读取；日志由 Web 服务器持久化；单次最多 8 MiB/50000 条 | `node/api/analytics_test.go:TestAnalyticsReadsBoundedAccessLogAndAggregatesMetrics` | `node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test ./node/api -run Analytics"` | 待主节点 OpenResty 日志路径联调 | implemented | 访客地理位置当前返回 IP，需部署 GeoIP 数据库后增强 |

## 2026-08-31 全局安全中间件
| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 全局 Session、CSRF、域名绑定与密码过期策略 | `/www/apps/1Panel/core/middleware/session.go`、`csrf_protect.go`、`bind_domain.go`、`password_expired.go` | 所有 `/api/v2/*` 请求及前端安全入口 | `control/api/security_middleware.go:NewSecurityMiddleware`、`cmd/workmesh-server/main.go` | API 全路径；前端 `/{securityEntrance}` | 本地 Session/Bearer/API Key/节点令牌；Cookie 写请求要求 `pcsrftoken` 与 `X-CSRF-Token` | `WORKMESH_DATA_DIR/domains.json.settings`（`bindDomain`、`securityEntrance`、`expirationDays`、`expirationTime`）及环境变量覆盖 | 读取设置采用 mtime/大小缓存；会话和令牌由 CoreService 管理 | `control/api/security_middleware_test.go`、`cmd/workmesh-server/main_test.go` | `go test -count=1 ./control/api ./cmd/workmesh-server -run 'Security|HTTPMux'` | 本地主进程包装器、健康检查和未登录控制面已验证；HTTPS/反向代理需部署验收 | implemented | 生产 `Secure` Cookie 和真实域名需部署配置 |

## 2026-08-31 服务端错误国际化与安全错误码
| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Accept-Language 通用错误响应与稳定错误码 | `/www/apps/1Panel/core/i18n`、`agent/i18n`、`core/middleware/*.go` | 全部 `/api/v2/*` JSON 错误及安全拒绝响应 | `i18n/i18n.go`、`runtime/http/server.go`、`control/api/helpers.go`、`node/api/errors.go` | 所有 JSON `ERR` envelope；`GET/POST /api/v2/*` | 原有 Session/Bearer/API Key/节点令牌校验不变；仅本地化错误文案 | Accept-Language 与内置 12 种语言目录 | 语言目录 Go embed 只读缓存；无新增用户状态 | `i18n/i18n_test.go`、`runtime/http/server_test.go`、`control/api/security_middleware_test.go` | `node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test ./i18n ./runtime/http ./control/api ./node/api"` | 本地英文/中文拒绝路径已验证；生产 HTTPS/代理语言头需部署验收 | partial | 业务域仍有少量直接 `wmhttp.JSON` 错误未迁移专用语言键；需按错误目录逐域补齐 |

## 2026-08-31 容器日志与下载进度闭环
| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 容器日志实时跟随与下载 | `/www/apps/1Panel/agent/app/api/v2/container.go:ContainerStreamLogs`、`service/container.go:DownloadContainerLogs` | `GET /api/v2/containers/search/log`、`POST /api/v2/containers/download/log` | `node/api/container_log_stream.go`、`node/api/containers.go` | GET SSE `/api/v2/containers/search/log`；POST `/api/v2/containers/download/log` | 流令牌或本地会话；节点 API 鉴权 | Docker CLI 日志输出，Compose 支持多文件 | 流接口无状态；下载响应临时内存受限 | `node/api/container_log_stream_test.go`、`node/api/stream_protocol_test.go` | `go test -count=1 ./node/api -run ContainerLog` | 待 Docker 节点联调 | implemented | 需生产 Docker/Compose 实例验证日志格式 |
| wget 下载实时进度与停止 | `/www/apps/1Panel/agent/app/api/v2/file.go:WgetProcess/StopWget` | `GET /api/v2/files/wget/process`、`GET .../keys`、`POST /api/v2/files/wget/stop` | `node/api/files_routes.go`、`node/api/wget_progress_stream.go` | WebSocket 进度、HTTP keys、POST stop（支持 `key`） | 节点会话；WebSocket 流令牌或会话 | HTTP(S) 响应流实时累计字节 | 有界进程内状态，目标文件临时写入后原子 rename | `node/api/wget_progress_test.go`、`node/api/files_routes_test.go` | `go test -count=1 ./node/api -run Wget` | 待远程大文件和断开联调 | implemented | 服务重启后进行中的任务不恢复 |

## 2026-08-31 路由别名与文件高级操作补齐

| 功能名称 | 旧源码位置 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|
| xpack 监控/WAF 别名 | `/www/apps/1Panel/agent/router/ro_website.go` | `node/api/website.go` | GET/POST `/api/v2/xpack/monitor/*`、`/api/v2/xpack/waf/*` | 节点会话 | analytics、网站 WAF 服务 | 网站配置文件 | `node/api/website_test.go` | implemented | 外部 OpenResty 可用性依赖部署环境 |
| 网站负载均衡与资源查询 | `/www/apps/1Panel/agent/app/api/v2/website.go` | `node/api/website.go`、`node/api/website_extensions.go` | GET `/api/v2/websites/:id/lbs`、`/api/v2/websites/resource/:id` | 节点会话 | 网站配置与域名记录 | 网站状态文件 | `node/api/website_test.go` | implemented | 数据库资源关联需凭据后接入 |
| 文件分片、历史与高级操作 | `/www/apps/1Panel/agent/app/api/v2/file.go` | `node/api/files_routes.go` | POST `/api/v2/files/chunkupload`、`history/*`、`depth/size`、`mode`、`read/:type`、`share/detail`、`mount`、`user/group` | 节点会话 | 本地文件系统 | 原子文件与 file-aux 状态 | `node/api/files_routes_test.go` | implemented | Windows owner 修改明确不支持 |
| 媒体文件转换 | `/www/apps/1Panel/agent/app/api/v2/file.go:ConvertFile` | `node/api/files_routes.go` | POST `/api/v2/files/convert`、`/convert/log` | 节点会话 | 配置的媒体转换器与本地文件 | 输出原子替换，转换日志写入 `files.json` | `node/api/files_routes_test.go` | implemented | 转换器需通过 `WORKMESH_MEDIA_CONVERTER` 配置；未配置时返回 503 |

## 2026-08-30 数据服务与 OpenResty 真实运行时补齐

## 2026-08-31 数据库管理控制面

| 功能名称 | 来源模块 | 来源入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 数据库用户生命周期 | 数据库服务 | `/databases/users*` | `node/service/database_admin.go`, `node/api/database_admin_routes.go` | POST `/api/v2/databases/users`, `/users/search`, `/users/update`, `/users/del`, `/users/password`, `/users/password/save` | 节点会话鉴权 | 本地数据库管理元数据，密码仅保存状态 | `WORKMESH_DATA_DIR/database-admin.json` 原子替换 | `node/api/database_admin_routes_test.go` | `go test ./node/api -run DatabaseAdmin` | 待节点联调 | implemented | 尚未连接远程数据库 SQL 执行器 |
| 数据库授权管理 | 数据库服务 | `/databases/grants*` | `node/service/database_admin.go`, `node/api/database_admin_routes.go` | POST `/api/v2/databases/grants`, `/grants/search`, `/grants/summary`, `/grants/del` | 节点会话鉴权 | 授权主体、数据库和权限集合 | `database-admin.json` 原子替换 | `node/api/database_admin_routes_test.go` | 同上 | 待节点联调 | implemented | 远程授权执行器待按凭据启用 |
| 数据库变量、配置和状态 | 数据库服务 | `/databases/variables*`, `/databases/common/*`, `/databases/status` | `node/service/database_admin.go`, `node/api/database_admin_routes.go` | POST `/api/v2/databases/variables`, `/variables/update`, `/common/info`, `/common/load/file`, `/common/update/conf`, `/format/options`, `/status`, `/remote`; `/description/update` | 节点会话鉴权 | 变量、配置文本及 TCP 状态探测 | `database-admin.json` 与数据库登记仓库原子替换 | `node/api/database_admin_routes_test.go`, `node/api/database_test.go` | `go test ./node/api -run Database` | 待节点联调 | implemented | 远程实例 SQL/配置文件写入需凭据和驱动后启用 |

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| MySQL/PostgreSQL/Redis/Mongo 数据库端点检查 | `/www/apps/1Panel/agent/router/ro_database.go`、`agent/app/service/database*.go` | `/databases/db/check`、`/databases/redis/check`、`/databases/status` | `node/api/database_routes.go`、`node/service/database.go` | `POST /api/v2/databases/db/check`、`POST /api/v2/databases/redis/check`、`POST /api/v2/databases/status` | 节点会话鉴权 | 目标主机 TCP 连接，按类型默认端口 3306/5432/6379/27017 | 无状态探测；操作审计 `database-operations.json` | `node/api/database_test.go` | `go test ./node/api -run Database` | 待主次节点人力联调 | implemented | 未提供账号时仅验证网络可达性，不执行 SQL 登录 |
| 数据库元数据新增、查询、分页、删除 | `/www/apps/1Panel/agent/router/ro_database.go` | `/databases/db`、`/databases/db/search`、`/databases/db/:name`、`/databases/db/del` | `node/api/database.go`、`node/api/database_routes.go`、`node/service/database.go` | `POST/GET /api/v2/databases/db*` | 节点会话鉴权；密码字段不落库 | 本地数据库登记信息 | `WORKMESH_DATA_DIR/databases.json` 原子替换 | `node/api/database_test.go`, `node/service/database_test.go` | `go test ./node/api ./node/service -run Database` | 待主次节点重启恢复验证 | implemented | 用户/授权/变量等 DB 专属 SQL 管理仍需按凭据启用 |
| OpenResty 版本与配置语法探测 | `/www/apps/1Panel/agent/app/api/v2/nginx.go`、`agent/app/service/nginx.go` | `GET /openresty/status`、`GET /openresty/https` | `node/service/website.go:ProbeOpenResty`、`node/api/website.go:registerOpenRestyRoutes` | `GET /api/v2/openresty/status`、`GET /api/v2/openresty/https` | 节点会话鉴权 | 受限 `openresty -v/-t` 或 `nginx -v/-t`，命令超时 2 秒 | `openresty.json` 保存控制面配置 | `node/service/website_test.go`, `node/api/website_test.go` | `go test ./node/service ./node/api -run OpenResty` | 待主节点安装 OpenResty 后实测 | implemented | 未安装二进制时明确返回 `available=false`，不伪造运行状态 |

## 2026-08-30 核心认证、脚本与进程批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Passkey 凭据元数据管理 | `/www/apps/1Panel/core/app/service/auth.go` | `GET /api/v2/core/auth/passkey/list`、`POST /api/v2/core/auth/passkey/register/*`、`POST /api/v2/core/auth/passkey/del` | `control/service/core.go`、`node/api/core_handlers.go` | 同左 | Session/Cookie 或 Bearer；注册挑战 5 分钟有效 | 凭据 ID、名称、创建时间 | `WORKMESH_DATA_DIR/passkeys.json`，临时文件原子替换 | `node/api/core_handlers_test.go:TestCorePasskeyRegistrationLifecycle` | `go test ./node/api -run CorePasskey` | 待双节点制品部署 | implemented | 未接入浏览器 WebAuthn 验证器时，空 credentialId 会明确拒绝 |
| 脚本库受控执行 | `/www/apps/1Panel/core/app/api/v2/script_library.go:RunScript` | `GET/WS /api/v2/core/script/run` | `node/api/core_resources.go:handleScriptRun` | `GET/WS /api/v2/core/script/run` | 管理员 Session；服务调用可使用 `X-WorkMesh-Token` | 已登记且已审核脚本（系统脚本固定白名单），不接受直接 command 参数 | core resource store | `node/api/core_resources_test.go`、`node/api/terminal_ws_contract_test.go` | `GOWORK=off go test ./node/api -run 'ScriptRun|Terminal'` | 待真实脚本库部署验证 | implemented | 复用终端 WebSocket 流式输出，保留脚本审批、来源和节点权限边界 |
| 进程详情采集 | `/www/apps/1Panel/agent/app/service/process.go:GetProcessInfoByPID` | `GET /api/v2/process/:pid` | `node/api/process.go:handleProcessByID` | `GET /api/v2/process/:pid` | 节点会话鉴权 | `/proc/<pid>/cmdline`、`/proc/<pid>/status` | 无状态实时采集 | `node/api/process_test.go` | `go test ./node/api -run Process` | 待 Linux 节点验证 | implemented | Windows 无 procfs 时仅返回可访问字段 |

## 2026-08-31 控制面会话与 API 凭据持久化

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 控制面写接口会话、Bearer/API Key 与 CSRF 校验 | `/www/apps/1Panel/core/middleware`、`core/app/api/v2/auth.go` | Gateway、节点角色和节点注册写入口 | `node/api/core_handlers.go`、`control/api/router.go`、`control/api/gateway.go`、`control/api/role.go` | `POST /api/v2/gateway/{register,login,heartbeat,authorization/refresh,unbind}`、`POST /api/v2/core/nodes/{add,update,del,delete,role/prepare,role/commit,role/abort}` | 本地 Session/Cookie 同源校验；Bearer、`X-API-Key`、`X-WorkMesh-Token` | 本地 CoreService 会话和用户凭据 | 用户 API 配置写入 `users.json`，原子 rename；会话保持内存态 | `control/api/auth_middleware_test.go`、`node/api/core_handlers_test.go` | `go test -count=1 ./control/api ./node/api ./control/service` | 待双节点 HTTPS 联调 | implemented | 生产环境仍需配置真实管理员凭据和 Gateway 凭据 |
| 节点执行面统一 API 鉴权 | `/www/apps/1Panel/core/middleware`、`agent/middleware` | 除健康检查和登录初始化外的 `/api/v2/*` | `cmd/workmesh-server/main.go:authenticateNodeAPI` | 节点执行面全部已注册路由 | 本地 Session/Cookie 同源校验、Bearer/API Key；WebSocket/SSE 使用各自短期 Token 和 Origin 校验 | CoreService 会话与流接口环境凭据 | 会话内存态、API Key 原子持久化 | `cmd/workmesh-server/main_test.go:TestHTTPMuxProtectsNodeAPIsAndAllowsLogin` | `go test -count=1 ./cmd/workmesh-server ./node/api` | 本地 HTTP 已验证 | implemented | 生产需通过 HTTPS 下发 Secure Cookie，并配置独立流 Token |
<!-- Copyright (c) 2026 WorkMesh contributors -->

## 2026-08-30 日志读取批次
| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 系统日志文件枚举 | `/www/apps/1Panel/agent/app/service/logs.go:ListSystemLogFile` | `GET /api/v2/logs/system/files` | `apps/workmesh-server/node/api/functional_domains.go:listSystemLogFiles` | `GET /api/v2/logs/system/files` | 节点会话鉴权 | `WORKMESH_DATA_DIR/logs`、Linux `/var/log` | 无状态读取 | `node/api/logs_runtime_test.go` | `go test ./node/api -run SystemLog` | 待制品部署 | implemented | 仅枚举日志文件 |
| 系统日志服务状态 | `/www/apps/1Panel/agent/app/service/logs.go:GetSystemLogStatus` | `GET /api/v2/logs/system/status` | `apps/workmesh-server/node/api/functional_domains.go:systemLogStatus` | `GET /api/v2/logs/system/status` | 节点会话鉴权 | `journalctl --version` 或文件降级 | 无状态读取 | `node/api/logs_runtime_test.go` | `go test ./node/api -run SystemLog` | 待制品部署 | implemented | 无 journalctl 时返回 file 状态 |
| 运行中系统服务 | `/www/apps/1Panel/agent/app/service/logs.go:ListRunningServices` | `GET /api/v2/logs/system/services` | `apps/workmesh-server/node/api/functional_domains.go:listRunningSystemServices` | `GET /api/v2/logs/system/services` | 节点会话鉴权 | `systemctl` 或 Windows `tasklist`，5 秒超时 | 无状态读取 | `node/api/logs_runtime_test.go` | `go test ./node/api -run SystemLog` | 待制品部署 | implemented | 命令不可用时返回空集合 |
| 主机系统日志读取 | `/www/apps/1Panel/agent/app/service/logs.go:ReadSystemLog` | `POST /api/v2/logs/system/read` | `apps/workmesh-server/node/api/functional_domains.go:readLogFile` | `POST /api/v2/logs/system/read` | 节点会话鉴权 | 允许目录内日志文件，单次最多 2 MiB | 无状态读取 | `node/api/logs_runtime_test.go` | `go test ./node/api -run SystemLog` | 待制品部署 | implemented | journalctl 过滤参数待扩展 |
| 任务日志分页读取 | `/www/apps/1Panel/agent/app/service/task.go:ReadByLine` | `POST /api/v2/logs/tasks/read` | `apps/workmesh-server/node/api/functional_domains.go:readTaskLog` | `POST /api/v2/logs/tasks/read` | 节点会话鉴权 | 任务日志路径，分页最多 500 行 | 无状态读取 | `node/api/logs_runtime_test.go` | `go test ./node/api -run SystemLog` | 待制品部署 | implemented | 任务仓库接入待任务域完成 |
| 执行中任务计数 | `/www/apps/1Panel/agent/app/service/task.go:CountExecutingTask` | `GET /api/v2/logs/tasks/executing/count` | `apps/workmesh-server/node/api/functional_domains.go:registerLogRoutes` | `GET /api/v2/logs/tasks/executing/count` | 节点会话鉴权 | domains.json 中 running/executing 任务日志 | `domains.json` | `node/api/logs_runtime_test.go` | `go test ./node/api -run SystemLog` | 待制品部署 | implemented | 可切换任务仓库实时计数 |

## 2026-08-30 运行时详情批次
| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| PHP 运行时详情 | `/www/apps/1Panel/agent/app/service/runtime.go` | `GET /api/v2/runtimes/:id` | `apps/workmesh-server/node/api/runtime_toolbox.go:registerRuntimeRoutes` | `GET /api/v2/runtimes/{id}` | 节点会话鉴权 | `runtime.json` 运行时记录 | `WORKMESH_DATA_DIR/runtime.json` | `node/api/runtime_toolbox_test.go` | `go test ./node/api -run Runtime` | 待制品部署 | implemented | 运行时安装器由运行时域负责 |
| PHP 扩展及配置查询 | `/www/apps/1Panel/agent/app/service/runtime.go` | `GET /api/v2/runtimes/php/:id/extensions`、`config`、`container`、`fpm/config`、`fpm/status` | `apps/workmesh-server/node/api/runtime_toolbox.go:registerRuntimeSubroutes` | 同左 | 节点会话鉴权 | 运行时记录及扩展列表 | `runtime.json` | `node/api/runtime_toolbox_test.go` | `go test ./node/api -run Runtime` | 待制品部署 | implemented | FPM 深度指标待接入系统探针 |
| Supervisor 进程详情 | `/www/apps/1Panel/agent/app/service/runtime.go` | `GET /api/v2/runtimes/supervisor/process/:id` | `apps/workmesh-server/node/api/runtime_toolbox.go:registerRuntimeSubroutes` | 同左 | 节点会话鉴权 | `runtime.json` supervisor 配置 | `runtime.json` | `node/api/runtime_toolbox_test.go` | `go test ./node/api -run Runtime` | 待制品部署 | implemented | 未配置进程返回 not_configured |

## 2026-08-30 Node 运行时包管理批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Node package.json 脚本读取 | `/www/apps/1Panel/agent/app/service/runtime.go:GetNodePackageRunScript` | `POST /api/v2/runtimes/node/package` | `node/api/runtime_toolbox.go:registerNodeRuntimeRoutes` | `POST /api/v2/runtimes/node/package` | 节点会话鉴权；工作目录受 `WORKMESH_WORKSPACE_ROOT` 限制 | 目标目录 `package.json`，大小上限 2 MiB | 无状态读取 | `node/api/runtime_toolbox_test.go:TestNodeRuntimePackageAndModules` | `go test ./node/api -run NodeRuntime` | 待节点制品部署 | implemented | 不执行 package.json 中的脚本，仅返回 scripts 清单 |
| Node modules 元数据扫描 | `/www/apps/1Panel/agent/app/service/runtime.go:GetNodeModules` | `POST /api/v2/runtimes/node/modules` | `node/api/runtime_toolbox.go:registerNodeRuntimeRoutes` | `POST /api/v2/runtimes/node/modules` | 节点会话鉴权；运行时 ID 归属校验 | `node_modules/*/package.json`，最多 500 项 | 无状态读取 | `node/api/runtime_toolbox_test.go:TestNodeRuntimePackageAndModules` | `go test ./node/api -run NodeRuntime` | 待节点制品部署 | implemented | 仅读取包元数据，不加载或执行包代码 |
| Node 模块安装/更新/卸载 | `/www/apps/1Panel/agent/app/service/runtime.go:OperateNodeModules` | `POST /api/v2/runtimes/node/modules/operate` | `node/api/runtime_toolbox.go:registerNodeRuntimeRoutes/runNodeModuleTask` | `POST /api/v2/runtimes/node/modules/operate`、`GET /api/v2/runtimes/node/tasks/:id` | 节点会话；包管理器和模块名白名单 | 本机 npm/yarn，工作目录校验 | `runtime.json` 的任务状态原子保存 | `node/api/runtime_toolbox_test.go` | `go test ./node/api -run NodeRuntime` | 需人工确认 npm/yarn 与外网源后验证 | implemented | 任务执行受 20 分钟超时；生产环境需配置命令白名单和镜像源 |

# WorkMesh 功能迁移清单

## 2026-08-30 网站配置别名批次
| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 网站 HTTPS/LBS/CORS 配置查询 | `/www/apps/1Panel/agent/router/ro_website.go` | `GET /api/v2/websites/:id/https`, `GET /api/v2/websites/:id/lbs`, `GET /api/v2/websites/cors/:id` | `node/api/website.go:registerDomainRoutes/registerWebsiteConfigRoutes` | GET 双段动态分发 | 节点会话 | WebsiteService 配置 | `website-configs.json` 原子保存 | `node/api/website_test.go` | `go test ./node/api -run Website` | 待双节点部署 | implemented | 无 |
| 网站代理与真实 IP 配置查询 | `/www/apps/1Panel/agent/router/ro_website.go` | `GET /api/v2/websites/proxy/config/:id`, `GET /api/v2/websites/realip/config/:id` | `node/api/website.go:registerWebsiteConfigRoutes` | GET 指定配置 | 节点会话 | WebsiteService 配置 | `website-configs.json` | `node/api/website_test.go` | `go test ./node/api -run WebsiteConfigAliases` | 待双节点部署 | implemented | 无 |
| 网站 DNS/CORS/LBS/代理/流配置更新 | `/www/apps/1Panel/agent/router/ro_website.go` | `POST /api/v2/websites/dns/update`, `/cors/update`, `/lbs/create`, `/lbs/update`, `/lbs/file`, `/proxy/clear`, `/stream/update` | `node/api/website.go:registerWebsiteAdvancedRoutes` | POST 配置写入 | 节点会话、资源归属校验 | 请求 JSON | `website-configs.json` 按类型隔离 | `node/api/website_test.go:TestWebsiteConfigAliasesPersist` | `go test ./node/api -run WebsiteConfigAliases` | 待双节点部署 | implemented | DNS 记录解析器待后续增强 |
| 网站 DNS 查询与删除 | `/www/apps/1Panel/agent/router/ro_website.go` | `POST /api/v2/websites/dns/search`, `/dns/del` | `node/api/website.go:registerWebsiteAdvancedRoutes` | POST | 节点会话、网站 ID 校验 | WebsiteService 配置 | `website-configs.json` | `node/api/website_test.go` | `go test ./node/api -run WebsiteConfigAliases` | 待双节点部署 | implemented | 删除采用标记并保留审计字段 |
| 网站监控配置与统计别名 | `/www/apps/1Panel/agent/router/ro_website.go` | `GET/POST /api/v2/websites/monitor/config/*`, `/monitor/{stat,qps,rank,trend,visitors}` | `node/api/website.go:registerWebsiteAdvancedRoutes`、`node/api/analytics.go` | GET/POST | 节点会话 | analytics 状态采集 | `domains.json` 设置区 | `node/api/website_test.go:TestWebsiteConfigAliasesPersist` | `go test ./node/api -run WebsiteConfigAliases` | 待双节点部署 | implemented | 统计采集器接入真实访问日志后增强 |

本清单以只读参考 `apps/1Panel/core` 与 `agent` 的 759 条路由为现行基线（含隐藏 helper 注册和去品牌化静态入口）。历史条目中的 `/www/apps/1Panel` 仅表示过渡期证据，不再作为实现或校验来源。状态必须以真实副作用或端到端响应确认，不能仅以路由注册作为完成依据。

迁移决策：759 条 1Panel 路由契约继续保留并在其上迭代，不整体废弃。清单中的未完成项必须继续实现和验证；生产主节点、次节点及真实 Gateway 凭据未完成联调前，不得把本地扫描结果当作部署完成。

非路由的初始化、后台作业、中间件、国际化、日志、任务和协议升级能力见 [`hidden-function-checklist.md`](./hidden-function-checklist.md)，两份清单必须同步维护。

## 2026-08-30 网站扩展与后端语言包批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| ACME 账户与自签 CA 生命周期 | `/www/apps/1Panel/agent/app/api/v2/website_acme_account.go`、`website_ssl.go` | `/websites/acme/*`、`/websites/ca/*` | `node/service/website_security.go`、`node/api/website_cert_routes.go` | `POST /api/v2/websites/acme*`、`POST/GET /api/v2/websites/ca*` | Session/Cookie、Bearer、节点时间戳 | 本地 ACME/CA/签发证书元数据 | `website-acme.json`、`website-ca.json`、`website-ca-ssls.json`，原子替换 | `node/api/website_cert_routes_test.go` | `go test ./node/api -run WebsiteCertificate` | 主次节点 `/health` `/ready` 已验证 | implemented | 外部 ACME DNS 提供商需显式配置后启用 |
| 网站扩展元数据与批量操作 | `/www/apps/1Panel/agent/app/api/v2/website.go`、`website_proxy.go`、`website_template.go` | `/websites/auths*`、`batch/*`、`templates/*`、`proxies*`、`exec/composer` | `node/api/website_extensions.go` | 统一 `GET/POST /api/v2/websites/{rest...}` | Session/Cookie、资源 ID 校验 | 本地模板、代理、认证、日志、数据库元数据 | `website-extensions.json`，列表最多 500 条 | `node/api/website_extensions_test.go` | `go test ./node/api -run WebsiteExtension` | 主次节点已部署并返回 200 | implemented | Composer 只校验文件，不执行任意命令 |
| 后端国际化资源 | `/www/apps/1Panel/core/i18n/lang/*.yaml`、`core/i18n/i18n.go`；`/www/apps/1Panel/agent/i18n/lang/*.yaml`、`agent/i18n/i18n.go` | 控制面和执行面的任务、日志、告警、认证、节点、授权消息本地化入口 | `i18n/i18n.go`、`i18n/lang/*.yaml` | `i18n.Load`、`i18n.Message`、`i18n.Format` | 进程内部调用；HTTP Accept-Language 规范化 | 12 个 UTF-8 YAML 语言包，每种 1037 个合并键 | Go embed，启动时一次解析并只读缓存 | `i18n/i18n_test.go`、`test/contract/i18n-scan.mjs` | `go test ./i18n`；`node scripts/with-dev-env.mjs -- node apps/workmesh-server/test/contract/i18n-scan.mjs --legacy /www/apps/1Panel --project apps/workmesh-server` | 二进制构建验证 | implemented | 同名 Core/Agent 键按执行面语义合并；调用方使用 `Format` 传入模板参数 |

## 2026-08-30 双节点部署与节点管理验收

| 项目 | 主节点 | 次节点 | 验证结果 |
| --- | --- | --- | --- |
| 制品 | `61.184.12.165:/opt/workmesh-server` | `162.14.96.198:/opt/workmesh-server-secondary` | Linux amd64 ELF 已替换，旧版本保存在 `backups/release-*` |
| 服务 | `workmesh-server.service` | `workmesh-server-secondary.service` | systemd active，端口 9999 |
| 健康检查 | `/health` HTTP 200 | `/health` HTTP 200 | 通过 |
| 就绪检查 | `/ready` HTTP 200 | `/ready` HTTP 200 | 通过 |
| 前端资源 | `/` HTTP 200，JS `text/javascript` | `/` HTTP 200，JS `text/javascript` | 通过 |
| 登录与节点新增 | 登录后新增 `secondary-gateway-162` | 当前节点列表可查询 | 主节点重启后节点仍存在，持久化通过 |
| Gateway 注册 | `registration=pending`，Gateway 401 | `registration=pending`，Gateway 404 `WORKMESH_NODE_NOT_FOUND` | 真实 Gateway 凭据/登记缺失，禁止伪造为 registered |

## 已完成

| 功能域 | 接口范围 | 真实行为 | 验证 |
| --- | --- | --- | --- |
| 健康检查 | `/health`、`/ready` | 返回服务和依赖就绪状态 | 主次节点 HTTP 200 |
| 认证会话 | `/api/v2/core/auth/*` | 登录、登出、当前用户、Cookie/Bearer/API Key | `go test ./node/api` |
| 节点管理 | `/api/v2/core/nodes/*`、`xpack/nodes/*` | 节点增删改查、收藏、持久化 | 节点 HTTP 测试与 `nodes.json` |
| 设置 | `/api/v2/core/settings/*`、`/api/v2/config/global` | 默认配置、键值更新、持久化 | 设置 HTTP 测试 |
| 网站/WAF/OpenResty | `/api/v2/websites/*`、`waf/*`、`openresty/*` | 网站和规则 CRUD、黑白名单、配置状态 | 网站域测试 |
| 备份/告警/日志 | `/api/v2/backups/*`、`alert/*`、`logs/*` | 轻量持久化、查询、更新和日志读取 | 功能域测试 |
| 容器与 Compose | `/api/v2/containers/*` | Docker argv 操作、镜像、网络、卷、Compose、统计和 inspect | `go test ./node/api` |
| 快捷命令 | `/api/v2/core/commands/*` | 命令模板增删改查、分页、树、CSV 上传、JSON 导入导出 | `go test ./node/api -run CoreCommands` |
| 计划任务 | `/api/v2/cronjobs/*` | 任务持久化、启停、单次执行、按五字段 Spec 每分钟调度、执行记录、清理、导入导出和下一次执行计算 | `go test ./node/service ./node/api` |
| Gateway 客户端 | `/api/v2/workmesh/gateway/*` | 登录、注册、心跳、解绑、授权刷新、Ed25519 签名 | Gateway 契约测试 |
| 节点链路 | handshake/heartbeat/sync/fencing | HMAC、时间戳、nonce、防重放和角色 fencing | `go test ./runtime/link ./control/api` |

### 2026-08-31 Gateway 绑定与角色状态闭环

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 前端 Gateway 绑定注册 | `/www/apps/1Panel/agent/app/api/v2/workmesh_gateway.go:RegisterWorkMeshGateway` | `POST /api/v2/workmesh/gateway/register` | `control/api/gateway.go:registerHandler` | POST `/api/v2/workmesh/gateway/register`、`/api/v2/gateway/register` | 本机会话；Gateway Bearer registrationToken；地址白名单 | Gateway `/workmesh/node/register` 返回的节点记录 | `WORKMESH_DATA_DIR/gateway-binding.json` 原子替换，地址和授权摘要可恢复 | `control/api/gateway_test.go:TestGatewayRegisterAcceptsFrontendBindingPayloadAndRestoresURL` | `go test ./control/api -run GatewayRegister` | 待真实 Gateway 凭据联调 | implemented | 远端 Gateway 必须已启用 WorkMeshCore 节点注册接口 |
| Gateway 账号登录并幂等复用绑定 | `/www/apps/1Panel/agent/app/api/v2/workmesh_gateway.go:LoginWorkMeshGateway` | `POST /api/v2/workmesh/gateway/login` | `control/api/gateway.go:loginHandler` | POST `/api/v2/workmesh/gateway/login` | Gateway 用户名密码仅 TLS 传输；旧绑定通过心跳验证 | Gateway `/workmesh/auth/login`、`/workmesh/node/heartbeat` | `gateway-binding.json`，令牌不通过 HTTP 响应返回 | `control/api/gateway_test.go:TestGatewayLoginRegistersNodeAndPersistsBinding` | `go test ./control/api -run GatewayLogin` | 待真实 Gateway 凭据联调 | implemented | Gateway 授权刷新接口需云端提供对应端点 |
| 角色切换 epoch 跨重启恢复 | `/www/apps/1Panel/core/utils/xpack/providers/multi_node.go` | `/api/v2/core/nodes/role/*`、`/api/v2/link/fencing/*` | `runtime/role/manager.go`、`control/api/role.go` | POST `/api/v2/core/nodes/role/{check,prepare,commit,abort}`、`/api/v2/link/fencing/{check,prepare,commit,abort}` | HMAC 链路签名；本机管理面会话 | 角色状态和 compare-and-set epoch | `WORKMESH_DATA_DIR/role-state.json` 原子替换 | `runtime/role/manager_test.go`、`control/api/link_test.go` | `go test ./runtime/role ./control/api -run Role` | 待双节点切换演练 | implemented | 生产部署需确保两个节点使用独立、受保护的数据目录 |
| 文件/数据库首批扩展 | `/api/v2/files/share/*`、`/api/v2/databases/db/update` | 分享 token 与数据库登记持久化、输入校验、分页和更新 | `go test ./node/api ./node/service` |
| 脚本资源 | `/api/v2/core/script`、`search`、`update`、`del`、`sync` | 统一资源存储提供脚本 CRUD 和同步兼容行为 | `go test ./node/api` |
| AI 执行面 | `/api/v2/ai/ollama/*`、`mcp/*`、`tensorrt/*`、`gpu/*` | 模型、MCP、TensorRT-LLM、GPU 状态和域名绑定均使用 `ai.json` 持久化；无硬件时返回可识别降级状态 | `go test ./node/api -run AI` |
| AI 账号与 Agent | `/api/v2/ai/accounts/*`、`agents/*`、`agents/channel/*`、`agents/plugins/*`、`agents/skills/*` | 账号/Agent/渠道/插件/Skill 的 CRUD、配置和搜索；敏感字段脱敏；角色和会话配置可恢复 | `go test ./node/api -run AI` |
| 应用目录 | `/api/v2/apps/search`、`detail/*`、`tags`、`checkupdate`、`services/*` | 应用目录搜索、详情、标签、更新状态和服务信息；目录为空时按原 1Panel 方式懒加载远程 `1panel.json.zip`，并缓存到 `apps.json` | `go test ./node/api -run App` |
| 已安装应用 | `/api/v2/apps/install`、`installed/*`、`ignored/*` | 安装幂等、启停/重启/卸载、端口和参数更新、排序、连接信息、升级忽略均写入 `apps.json` | `go test ./node/api -run App` |
| 自定义应用商店 | `/api/v2/custom/app/*`、`/api/v2/core/xpack/sync/app/install` | 自定义商店配置、同步任务和多节点安装兼容入口 | `go test ./node/api -run App` |

## 进行中

### 2026-08-30 文件域批次（批量、分享、搜索与下载）

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 回收站状态 | `/www/apps/1Panel/agent/app/api/v2/recycle_bin.go:GetRecycleStatus` | `GET /files/recycle/status` | `node/api/files_routes.go:fileAdvancedHandler` | `GET /api/v2/files/recycle/status` | 节点会话 | `files.json` 回收站条目 | `WORKMESH_DATA_DIR/files.json` | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileFavorite` | 待制品部署 | implemented | 仅返回本地回收站计数 |
| 分享校验 | `/www/apps/1Panel/agent/app/api/v2/file.go:CheckFileShare` | `GET /files/share/check` | `node/api/files_routes.go:fileAdvancedHandler` | `GET /api/v2/files/share/check` | token/code 公共校验 | 分享 token 与目标文件 | `file-shares.json` | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileShare` | 待制品部署 | implemented | 支持 code/token 别名 |
| 分享信息 | `/www/apps/1Panel/agent/app/api/v2/file.go:GetPublicFileShareInfo` | `GET /files/share/info` | `node/api/files_routes.go:fileAdvancedHandler` | `GET /api/v2/files/share/info` | token/code 公共查询 | 分享记录、文件 stat | `file-shares.json` | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileShare` | 待制品部署 | implemented | 文件不存在返回 exists=false |
| 分享下载 | `/www/apps/1Panel/agent/app/api/v2/file.go:DownloadFileShare` | `GET /files/share/download` | `node/api/files_routes.go:fileAdvancedHandler` | `GET /api/v2/files/share/download` | token/code 公共校验 | 分享记录与文件内容 | 无状态读取 | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileShare` | 待制品部署 | implemented | 目标删除返回 404 |
| 分享二维码数据 | `/www/apps/1Panel/agent/app/api/v2/file.go:GetFileShareQRCode` | `GET /files/share/qrcode` | `node/api/files_routes.go:fileAdvancedHandler` | `GET /api/v2/files/share/qrcode` | token/code 公共校验 | 分享 URL | 无状态 | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileShare` | 待制品部署 | implemented | 返回 URL 数据，前端可编码展示 |
| wget 进度查询 | `/www/apps/1Panel/agent/app/api/v2/file.go:WgetProcess` | `GET /files/wget/process` | `node/api/files_routes.go:fileAdvancedHandler` | `GET /api/v2/files/wget/process` | 节点会话 | 进程内下载状态 | 进程内有界 map | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileShare` | 待制品部署 | implemented | 服务重启后任务不恢复 |
| wget 任务 key | `/www/apps/1Panel/agent/app/api/v2/file.go:ProcessKeys` | `GET /files/wget/process/keys` | `node/api/files_routes.go:fileAdvancedHandler` | `GET /api/v2/files/wget/process/keys` | 节点会话 | 下载任务 map | 进程内 | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileShare` | 待制品部署 | implemented | 仅返回当前进程任务 |
| AI 文件内容搜索 | `/www/apps/1Panel/agent/app/service/file.go:AISearch` | `POST /files/ai-search` | `node/api/files_routes.go:handleFileAISearch` | `POST /api/v2/files/ai-search` | 节点会话 | 目录文件内容、查询参数 | 无状态 | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileBatch` | 待制品部署 | implemented | grep 模式；LLM 摘要待独立 AI 域接入 |
| 批量文件存在检查 | `/www/apps/1Panel/agent/app/service/file.go:BatchCheckFiles` | `POST /files/batch/check` | `node/api/files_routes.go:fileAdvancedHandler` | `POST /api/v2/files/batch/check` | 节点会话 | `os.Stat` | 无状态 | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileBatch` | 待制品部署 | implemented | 单次最多 500 路径 |
| 批量删除 | `/www/apps/1Panel/agent/app/service/file.go:BatchDelete` | `POST /files/batch/del` | `node/api/files_routes.go:fileAdvancedHandler` | `POST /api/v2/files/batch/del` | 节点会话、路径校验 | 文件系统 | 直接删除 | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileBatch` | 待制品部署 | implemented | 每次最多 200 路径 |
| 批量权限更新 | `/www/apps/1Panel/agent/app/service/file.go:BatchChangeModeAndOwner` | `POST /files/batch/role` | `node/api/files_routes.go:fileAdvancedHandler` | `POST /api/v2/files/batch/role` | 节点会话、mode 白名单 | 文件系统 chmod | 无状态 | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileBatch` | 待制品部署 | implemented | 当前跨平台仅保证 mode，owner 字段回显 |
| 单文件存在检查 | `/www/apps/1Panel/agent/app/api/v2/file.go:CheckFile` | `POST /files/check` | `node/api/files_routes.go:fileAdvancedHandler` | `POST /api/v2/files/check` | 节点会话 | `os.Stat` / Mkdir | 文件系统副作用（withInit） | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileBatch` | 待制品部署 | implemented | withInit 只创建目录 |
| 收藏分页查询 | `/www/apps/1Panel/agent/app/api/v2/favorite.go:SearchFavorite` | `POST /files/favorite/search` | `node/api/files_routes.go:fileAdvancedHandler` | `POST /api/v2/files/favorite/search` | 节点会话 | `files.json` favorites | `WORKMESH_DATA_DIR/files.json` | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileFavorite` | 待制品部署 | implemented | page/pageSize 上限 200 |

实现扫描器当前结果以 [`function-checklist-generated.md`](./function-checklist-generated.md) 和 `.tmp/implementation-status.json` 为准（基于只读 `/www/apps/1Panel` 的 759 条路由：implemented 759、partial 0、pending 0）。隐藏初始化文件中的路由也已纳入去重统计。专用领域处理器优先承接业务；尚未接入专用模型的低频契约使用持久化兜底状态，禁止返回固定空数据冒充成功。

- 主机与系统：主机列表、连接测试、系统信息、命令历史和终端。
- 文件：分享、回收站、压缩/解压、上传下载、权限和内容搜索。
- 数据库：实例、用户、备份恢复、配置和状态。
- 应用与运行时：应用目录、安装升级、PHP/Node 扩展、运行时配置。
- AI 与工具箱：模型账号、MCP、GPU、SSH、脚本和沙盒。
- 日志与任务：登录/操作日志、任务取消重试、审计检索和清理。

## 每个功能域的完成门槛

1. 路由与旧接口逐条映射，记录旧 handler/service 来源。
2. 新实现具备输入校验、认证/权限、中间件和错误码。
3. 写操作产生真实副作用并持久化，列表支持分页或明确上限。
4. 增加 Go 单元测试、HTTP 测试；跨节点功能增加 E2E 证据。
5. 更新本清单、API 文档和部署验收记录。
6. 使用 `go test ./...`、`go vet ./...`、路由契约扫描和实现扫描器验证。
7. 构建 Linux amd64，备份远端二进制后原子替换主/次节点，并记录 SHA256、服务状态和回滚路径。

## 扫描命令

```powershell
node scripts/with-dev-env.mjs -- node test/contract/implementation-scan.mjs --legacy /www/apps/1Panel --project apps/workmesh-server --out .tmp/implementation-status.json
node scripts/with-dev-env.mjs -- node test/contract/hidden-function-scan.mjs --legacy /www/apps/1Panel --project apps/workmesh-server --out .tmp/hidden-function-status.json
```

## 2026-08-30 仪表盘采集批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 仪表盘主机资源采集 | `/www/apps/1Panel/agent/api/v2/dashboard.go` | `/api/v2/dashboard/base/*`、`/api/v2/dashboard/current/*` | `apps/workmesh-server/node/api/dashboard.go` | `GET /api/v2/dashboard/base/{ioOption}/{netOption}`、`GET /api/v2/dashboard/current/{ioOption}/{netOption}` | 节点会话鉴权（由上层中间件执行） | `/proc/loadavg`、`/proc/meminfo`、`/proc/net/dev`、`/proc/mounts`、运行时信息 | 无状态实时采集 | `node/api/dashboard_test.go` | `go test ./node/api -run Dashboard` | 待下一批制品部署 | implemented | Windows 无 `/proc` 时返回 supported=false，GPU/NPU/XPU 需驱动适配 |

## 2026-08-31 仪表盘真实指标补齐
| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| CPU、内存、交换区、磁盘、块设备 I/O 与主机识别信息 | `/www/apps/1Panel/agent/api/v2/dashboard.go`、`agent/app/service/system.go` | `/api/v2/dashboard/base/*`、`/api/v2/dashboard/current/*`、`/api/v2/dashboard/base/os` | `node/api/dashboard.go`、`node/api/dashboard_disk_unix.go`、`node/api/dashboard_disk_windows.go` | `GET /api/v2/dashboard/base/{ioOption}/{netOption}`、`GET /api/v2/dashboard/current/{ioOption}/{netOption}`、`GET /api/v2/dashboard/base/os` | 节点会话鉴权（健康及登录预检除外） | Linux `/proc/stat`、`/proc/meminfo`、`/proc/diskstats`、`/proc/net/dev`、`/proc/mounts`、`statfs`、`/etc/os-release`；Windows 无 procfs 时返回可用字段和明确降级 | 无状态实时采集，短单次读取，不写入项目数据 | `node/api/dashboard_test.go` | `node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test -count=1 ./node/api -run Dashboard"` | 本地 Windows/Linux 编译与单元测试；生产节点需真实挂载点和容器环境验收 | implemented | GPU/NPU/XPU 仍需对应驱动适配；Windows 磁盘容量需接入 GetDiskFreeSpaceEx |

## 2026-08-31 容器仓库、模板与 Compose 管理

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 镜像仓库 CRUD、搜索和状态 | `/www/apps/1Panel/agent/app/api/v2/image_repo.go`、`service/image_repo.go` | `/api/v2/containers/repo*` | `node/api/containers.go:registerContainerRepositoryRoutes` | `GET/POST /api/v2/containers/repo`、`POST /repo/{search,status,update,del}` | 节点会话/上层权限 | 用户提交的仓库元数据 | `WORKMESH_DATA_DIR/containers.json` 原子 rename，密码脱敏 | `node/api/container_features_test.go` | `go test ./node/api -run ContainerRepository` | 本地已验证 | implemented | 远程镜像推送/拉取仍由 Docker CLI 按需执行 |
| Compose 模板 CRUD 与批量导入 | `/www/apps/1Panel/agent/app/service/compose_template.go` | `/api/v2/containers/template*` | `node/api/containers.go:registerContainerTemplateRoutes` | `GET /template`、`POST /template/{search,update,batch,del}` | 节点会话/上层权限 | 模板正文和描述 | `containers.json` 原子 rename，正文 4 MiB 上限 | `node/api/container_features_test.go` | `go test ./node/api -run ContainerRepository` | 本地已验证 | implemented | 无外部模板市场同步 |
| Compose 创建、更新、置顶和环境读取 | `/www/apps/1Panel/agent/app/api/v2/container.go` | `/api/v2/containers/compose*` | `node/api/containers.go:handleComposeCreate/Update/Pin` | `POST /compose`、`/compose/update`、`/compose/pin`、`/compose/env` | 节点会话/上层权限 | Compose 文件、项目 `.env` | 文件临时写入后原子 rename；记录保存至 `containers.json` | `node/api/container_features_test.go` | `go test ./node/api -run ComposeCreate` | 本地已验证 | implemented | Docker 编排执行依赖主机 Docker 服务 |
| 容器用户与尺寸统计 | `/www/apps/1Panel/agent/app/api/v2/container.go` | `/api/v2/containers/users`、`item/stats` | `node/api/containers.go:handleContainerPost` | `POST /users`、`POST /item/stats` | 节点会话/命令白名单 | 容器内 `/etc/passwd`、Docker inspect --size | 无状态实时查询 | `node/api/containers.go` 参数校验 | `go test ./node/api` | 待 Docker 主机验收 | implemented | Docker 不可用时返回明确命令错误 |
| 镜像安全导入导出 | `/www/apps/1Panel/agent/app/api/v2/container.go` | `/api/v2/containers/image/load`、`image/save` | `node/api/containers.go:handleImageOperation` | `POST /image/load`、`POST /image/save` | 节点会话/上层权限 | 受校验的宿主路径与 Docker CLI | Docker 负责镜像归档，路径拒绝穿越 | `node/api/container_features_test.go` | `go test ./node/api -run ImageImport` | 待 Docker 主机验收 | implemented | 不支持无路径把二进制写入 JSON 响应 |
| 备份云端令牌刷新和 Bucket 能力边界 | `/www/apps/1Panel/agent/app/service/backup.go`、`cronjob_backup.go` | `/api/v2/backups/refresh/token`、`/api/v2/backups/buckets` | `node/api/functional_domains.go:handleBackupRefreshToken/handleBackupBuckets` | `POST /api/v2/backups/refresh/token`、`POST /api/v2/backups/buckets` | 节点会话；令牌只在服务端保存 | OAuth `refresh_url` 外部端点（15 秒超时）；本地备份目录 | `domains.json` 原子写入，令牌不返回；云端未配置返回 503 | `node/api/functional_domains_test.go:TestBackupCloudEndpointsDoNotFakeSuccess` | `go test ./node/api -run BackupCloud` | 未配置云凭据时验证明确错误 | partial | 各云厂商远端 Bucket SDK 和生产凭据接入仍待完成 |

## 2026-08-30 节点部署入口

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 前端添加部署节点 | `/www/apps/1Panel/agent/views/setting/node` | 节点管理页面 | `web/src/views/advanced/multi-node/index.vue`、`web/src/api/modules/setting.ts` | `POST /api/v2/core/nodes/add`、`POST /api/v2/core/nodes/list` | 登录会话、CSRF（前端请求拦截器注入） | 表单节点 ID、名称、HTTP(S) 地址、角色 | `WORKMESH_DATA_DIR/nodes.json` 原子写入 | `control/api/link_test.go`、生产构建 | `npm.cmd run type-check`、`npm.cmd run build:pro`、`go test ./control/api` | 公网主节点添加/列表/删除验收通过；新前端待授权部署 | implemented | 云端 Gateway 注册仍需真实凭据；添加动作不代替云端授权 |

## 2026-08-30 工具箱主机信息批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 工具箱系统用户、时区、Fail2ban、FTP 状态 | `/www/apps/1Panel/agent/api/v2/toolbox.go` | `/api/v2/toolbox/*` | `node/api/runtime_toolbox.go:toolboxGetData` | `GET /api/v2/toolbox/device/users`、`GET /api/v2/toolbox/device/zone/options`、`GET /api/v2/toolbox/fail2ban/base`、`GET /api/v2/toolbox/fail2ban/load/conf`、`GET /api/v2/toolbox/ftp/base` | 节点会话鉴权（由上层中间件执行） | `/etc/passwd`、`/etc/fail2ban/jail.local`、`time.Local`、`runtime.json` | FTP 配置写入 `WORKMESH_DATA_DIR/runtime.json` | `node/api/runtime_toolbox_test.go` | `go test ./node/api -run Toolbox`、`go vet ./...` | 待下一批制品部署 | implemented | 非 Linux 主机无系统配置文件时返回 supported/installed 状态，不执行外部命令 |

## 2026-08-30 告警发现批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 告警磁盘列表 | `/www/apps/1Panel/agent/app/service/alert.go:GetDisks` | `GET /api/v2/alert/disks/list` | `node/api/functional_domains.go:listAlertDisks` | `GET /api/v2/alert/disks/list` | 节点会话鉴权（上层中间件） | `/proc/mounts` 挂载点清单；跨平台容量字段显式标记 `capacitySupported` | 无状态采集 | `node/api/functional_domains_test.go:TestAlertDiskAndClamDiscovery` | `go test ./node/api -run AlertDisk` | 本地 HTTP 单测 | implemented | Windows 无 `/proc` 时返回空集合，Linux 容量采集可由专用采集器扩展 |
| ClamAV 服务发现 | `/www/apps/1Panel/agent/app/service/alert.go:GetClams` | `GET /api/v2/alert/clams/list` | `node/api/functional_domains.go:detectClamServices` | `GET /api/v2/alert/clams/list` | 节点会话鉴权（上层中间件） | PATH 中 `clamdscan`、`freshclam` 可执行文件探测 | 无状态采集 | `node/api/functional_domains_test.go:TestAlertDiskAndClamDiscovery` | `go test ./node/api -run AlertDisk` | 本地 HTTP 单测 | implemented | 未引入 ClamAV 管理 SDK，服务启停仍由运行时工具域负责 |
| 告警配置与系统计划任务搜索 | `/www/apps/1Panel/agent/app/api/v2/alert.go` | `POST /api/v2/alert/config/search`、`POST /api/v2/alert/cronjob/list` | `node/api/functional_domains.go` | 同左 | 节点会话鉴权（上层中间件） | `domains.json` 告警配置、`/etc/cron.*` 目录 | 配置沿用 `domains.json` 原子写入；cron 只读 | `node/api/functional_domains_test.go:TestAlertConfigSearchAndCronListAreDynamic` | `go test ./node/api -run AlertConfig` | 本地 HTTP 单测 | implemented | 未接入系统级 cron 编辑和执行，写操作仍由计划任务域负责 |

## 2026-08-30 AI 账户与沙盒批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| AI 提供商目录与账户分页 | `/www/apps/1Panel/agent/app/provider/catalog.go`、`agent/app/service/agents.go` | `GET /api/v2/ai/accounts/providers`、`POST /api/v2/ai/accounts`、`POST /api/v2/ai/accounts/update`、`POST /api/v2/ai/accounts/search`、`POST /api/v2/ai/accounts/counts`、`POST /api/v2/ai/accounts/delete` | `node/api/ai_execution.go:aiProviders`、`handleAccountRoute` | 同左 | 上层 Session/Bearer；账户写入需节点权限 | 内置提供商元数据和请求体 | `WORKMESH_DATA_DIR/ai.json`（未设置时 `./data/ai.json`）原子 rename | `node/api/ai_execution_test.go:TestAIAccountModelsAndValidation` | `go test ./node/api -run AIAccount` | 待下一批制品部署 | implemented | 真实 Gateway 账户同步待凭据接入 |
| 账户模型管理与远程发现 | `/www/apps/1Panel/agent/app/service/agents.go:1201-1345` | `POST /api/v2/ai/accounts/models`、`models/create`、`models/update`、`models/delete`、`models/discover`、`verify` | `node/api/ai_execution.go:handleAccountRoute`、`discoverAIModels` | 同左 | Session/Bearer；API Key 仅用于上游请求且响应脱敏 | 账户持久化模型或上游 `/models` JSON | `ai.json` 原子 rename；HTTP 8 秒超时 | `node/api/ai_execution_test.go:TestAIAccountModelsAndValidation`、`TestAIAccountModelDiscoveryAndSandboxPersistence` | `go test ./node/api -run 'AIAccount|AI.*Discovery'` | 待下一批制品部署 | 不同厂商专用鉴权协议需按 provider 扩展 |
| GPU 能力探测 | `/www/apps/1Panel/agent/api/v2/monitor.go` | `GET /api/v2/ai/gpu/load`、`GET /api/v2/ai/gpu/options`、`POST /api/v2/ai/gpu/search` | `node/api/ai_execution.go:detectGPU` | 同左 | Session/Bearer | `nvidia-smi` 设备信息和 Go 运行时内存 | 无状态采集 | `node/api/ai_execution_test.go`（CPU 降级路径） | `go test ./node/api -run AI` | 待下一批制品部署 | AMD/NPU/XPU 驱动适配待补充 |
| CubeSandbox 生命周期与状态 | `/www/apps/1Panel/agent/router/ro_cubesandbox.go`、`agent/app/api/v2/workmesh_task.go` | `GET /api/v2/cubesandbox/health`、`GET /api/v2/cubesandbox/status`、`POST /api/v2/cubesandbox/start`、`stop`、`reconcile` | `node/api/ai_execution.go:sandboxHandler` | 同左 | Session/Bearer；启动受 KVM 能力限制 | `/dev/kvm` 能力探测与实例状态 | `ai.json` Sandboxes 数组原子 rename | `node/api/ai_execution_test.go:TestAIAccountModelDiscoveryAndSandboxPersistence` | `go test ./node/api -run Sandbox` | 待下一批制品部署 | MicroVM 实际进程编排和镜像校验待接入 |
| MCP 服务真实连接测试 | `/www/apps/1Panel/agent/app/service/mcp_server.go:TestConnection`、`agent/app/api/v2/mcp_server.go` | `POST /api/v2/ai/mcp/server/connection/test` | `node/api/ai_execution.go:testMCPConnection` | POST `/api/v2/ai/mcp/server/connection/test` | 节点会话；上游凭据不得写入 URL | MCP 记录中的 baseUrl、传输类型和协议版本；实际 HTTP/SSE/JSON-RPC 响应 | 无状态探测；10 秒超时 | `node/api/ai_execution_test.go:TestMCPConnectionTestPerformsNetworkProbe`、`TestMCPSSEConnectionRequiresEventStream` | `go test ./node/api -run MCP` | 待真实 MCP 服务联调 | 仅支持 SSE 和 Streamable HTTP，其他传输返回明确错误 |
| Agent 渠道配对确认 | `/www/apps/1Panel/agent/app/service/agents_channels.go:ApproveChannelPairing` | `POST /api/v2/ai/agents/channel/pairing/approve` | `node/api/ai_execution.go:handleAgentPairingApprove` | POST `/api/v2/ai/agents/channel/pairing/approve` | 节点会话；Agent 容器归属校验 | 固定 Docker `exec` 参数调用已登记 Agent 容器 | 无状态命令；20 秒超时 | `node/api/ai_execution_test.go` | `go test ./node/api -run Pairing` | 待 Agent 容器制品联调 | 未绑定容器时明确返回 `AGENT_RUNTIME_UNAVAILABLE` |
## 2026-08-30 任务隔离执行批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 任务沙盒创建 | `/www/apps/1Panel/agent/app/api/v2/workmesh_task.go:CreateWorkMeshTask`、`agent/utils/cubesandbox/task.go` | `POST /api/v2/workmesh/tasks/create` | `node/api/ai_execution.go:taskHandler`、`node/service/taskruntime/taskruntime.go:TaskProvider.Create` | `POST /api/v2/workmesh/tasks/create` | 节点 Token（配置 `WORKMESH_TASK_TOKEN`）；Provider CLI 摘要和能力证明 | 镜像 sha256、工作区、入口、隔离策略、资源 profile | `ai.json` tasks 原子 rename；沙盒由受控 CLI 持久化 | `node/api/ai_execution_test.go:TestWorkMeshTaskRoutesUseIsolatedProvider`、`node/service/taskruntime/taskruntime_test.go` | `go test ./node/api ./node/service/taskruntime` | 真实 CLI 配置后验收 | implemented | CLI 必须实现 `--json task capabilities {}` 并声明工作区/网络隔离和 CPU、内存、PID、磁盘硬限制；未提供时接口返回 503 |
| 任务沙盒启动、取消、销毁 | `/www/apps/1Panel/agent/app/api/v2/workmesh_task.go:StartWorkMeshTask/CancelWorkMeshTask/DestroyWorkMeshTask` | `POST /api/v2/workmesh/tasks/start`、`cancel`、`destroy` | `node/api/ai_execution.go:taskHandler`、`node/service/taskruntime/taskruntime.go:transition` | 同左 | 节点 Token；任务状态机校验 | Provider 沙盒句柄与生命周期 | `ai.json` tasks 状态原子更新 | 同上 | 同上 | 真实 CLI 配置后验收 | implemented | 服务重启后需重新关联 Provider 句柄；能力探测失败时不创建任务 |
| 任务沙盒执行与结果收集 | `/www/apps/1Panel/agent/app/api/v2/workmesh_task.go:ExecWorkMeshTask/CollectWorkMeshTask` | `POST /api/v2/workmesh/tasks/exec`、`collect` | `node/api/ai_execution.go:taskHandler`、`node/service/taskruntime/taskruntime.go:Exec/Collect` | 同左 | 节点 Token；仅允许 argv，不允许宿主 Shell | Provider CLI 返回 exitCode/stdout/stderr | 任务状态和结果由沙盒后端保存；状态摘要写入 `ai.json` | 同上 | 同上 | 真实 CLI 配置后验收 | implemented | 结果历史分页和断点流式输出待在线开发域接入 |

## 2026-08-30 网站高级操作批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 网站运行状态与可用性检查 | `/www/apps/1Panel/agent/router/ro_website.go` | `POST /api/v2/websites/operate`、`POST /api/v2/websites/check` | `node/api/website.go`、`node/service/website.go` | `POST /api/v2/websites/operate`、`POST /api/v2/websites/check` | 节点会话鉴权 | WebsiteService 网站元数据 | `websites.json` 原子写入 | `node/api/website_test.go`、`node/service/website_test.go` | `go test ./node/api ./node/service -run Website` | 本地 HTTP 已验证 | implemented | 未接入 OpenResty 进程重载 |
| 网站类型和站点选项 | `/www/apps/1Panel/agent/router/ro_website.go` | `POST /api/v2/websites/options` | `node/api/website.go` | `POST /api/v2/websites/options` | 节点会话鉴权 | 网站列表与内置类型 | 无状态派生 | `node/api/website_test.go` | `go test ./node/api -run WebsiteAdvanced` | 本地 HTTP 已验证 | implemented | 类型选项待按运行时扩展 |
| 网站域名增删改查 | `/www/apps/1Panel/agent/router/ro_website.go` | `GET /api/v2/websites/domains/:websiteId`、`POST /api/v2/websites/domains*` | `node/api/website.go`、`node/service/website.go` | 同旧路由 | 节点会话鉴权、网站归属校验 | WebsiteDomain | `website-domains.json` 原子写入 | `node/api/website_test.go`、`node/service/website_test.go` | `go test ./node/api ./node/service -run 'WebsiteAdvanced|Domain'` | 本地 HTTP 已验证 | implemented | 未执行 DNS 自动解析 |
| Nginx、重写、目录、跳转、防盗链配置 | `/www/apps/1Panel/agent/router/ro_website.go` | `/websites/{config,rewrite,dir,redirect,leech}*` | `node/api/website.go`、`node/service/website.go` | 11 个 POST 配置接口及 `GET /websites/rewrite/custom` | 节点会话鉴权、网站归属校验 | 请求配置对象 | `website-configs.json` 原子写入 | `node/api/website_test.go` | `go test ./node/api -run WebsiteAdvanced` | 本地 HTTP 已验证 | implemented | 未调用 OpenResty reload |
| 网站 HTTPS 配置 | `/www/apps/1Panel/agent/router/ro_website.go` | `GET/POST /api/v2/websites/:id/https` | `node/api/website.go`、`node/service/website.go` | `GET/POST /api/v2/websites/:id/https` | 节点会话鉴权、网站归属校验 | enabled/operate 参数 | `website-configs.json` 原子写入 | `node/api/website_test.go` | `go test ./node/api -run WebsiteAdvanced` | 本地 HTTP 已验证 | implemented | 证书签发由 SSL 域负责 |
| 网站/SSL 自动续期扫描 | `/www/apps/1Panel/agent/cron/job/website.go`、`ssl.go` | 后台每小时任务 | `node/api/host_container_cron.go:StartBackgroundTasks`、`node/service/website_security.go:RenewDueCertificates` | 无 HTTP；到期前 30 天扫描 self-signed 证书 | 进程生命周期；不接受外部命令 | `website-ca-ssls.json` 与 CA 私钥 | 原子 JSON rename | `node/service/ssl_test.go:TestWebsiteSecurityRenewsDueSelfSignedCertificate` | `go test ./node/service -run WebsiteSecurityRenews` | 本地验证；ACME 需显式授权 | implemented | 云端 ACME 挑战仍由手动 API 负责 |

## 2026-08-30 应用目录详情批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 应用详情与运行服务 | `/www/apps/1Panel/agent/app/api/v2/app.go:GetApp*` | `GET /api/v2/apps/:key`、`GET /api/v2/apps/detail/*`、`GET /api/v2/apps/services/:key` | `node/api/apps.go:appCatalogGet` | 同左 | 节点会话鉴权（上层中间件） | `apps.json` catalog/apps 记录及配置中的 services/params；目录为空时从原 1Panel `1panel.json.zip` 懒加载 | `WORKMESH_DATA_DIR/apps.json` 原子写入 | `node/api/apps_test.go:TestRemoteAppCatalogLazyLoadAndDetails`、`TestAppDerivedDetailsAndDeleteCheck` | `go test ./node/api -run App` | 本地 HTTP、远程 ZIP/Compose/图标模拟已验证 | implemented | 生产环境需按部署网络策略配置仓库覆盖地址 |
| 已安装应用信息与删除检查 | `/www/apps/1Panel/agent/app/api/v2/app.go` | `GET /api/v2/apps/installed/info/:appInstallId`、`GET /api/v2/apps/installed/params/:appInstallId`、`GET /api/v2/apps/installed/delete/check/:appInstallId` | `node/api/apps.go:appInstalledGet` | 同左 | 节点会话鉴权（上层中间件） | 安装记录、容器名称和参数 | `apps.json` 原子写入 | `node/api/apps_test.go:TestAppDerivedDetailsAndDeleteCheck` | `go test ./node/api -run AppDerived` | 本地 HTTP 已验证 | implemented | 容器资源删除仍需容器域执行 |
| 应用版本更新查询 | `/www/apps/1Panel/agent/app/api/v2/app.go` | `POST /api/v2/apps/installed/update/versions` | `node/api/apps.go:handleAppPost` | `POST /api/v2/apps/installed/update/versions` | 节点会话鉴权（上层中间件） | catalog 中匹配应用的版本记录 | 无额外写入 | `node/api/apps_test.go` | `go test ./node/api -run App` | 本地 HTTP 已验证 | implemented | 远程版本同步依赖 Gateway 配置 |
| 应用目录升级检查 | `/www/apps/1Panel/agent/app/api/v2/app.go:GetAppListUpdate`、`agent/app/service/app.go:GetAppUpdate` | `GET /api/v2/apps/checkupdate` | `node/api/apps.go:refreshCatalogLocked`、`latestCatalogVersion` | `GET /api/v2/apps/checkupdate` | 节点会话鉴权（上层中间件）；目录文件错误返回 502 | `WORKMESH_APP_CATALOG` 配置文件与 `apps.json` 已安装记录；按 key/id/name 匹配并取最高版本 | `apps.json` 原子 rename，保存目录版本、lastModified、同步时间 | `node/api/apps_test.go:TestAppCheckUpdateUsesConfiguredCatalogAndPersistsMetadata`、`TestAppCheckUpdateNoUpdateForEquivalentVersions`、`TestAppCheckUpdateReportsCatalogError` | `node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test -count=1 ./node/api -run 'AppCheckUpdate|CompareAppVersion'"` | 本地 HTTP、重启后目录快照恢复已验证；生产双节点待部署凭据 | implemented | 远程商店同步由显式 sync 路由触发；目录文件需可读且不超过 8 MiB |

## 2026-08-31 应用环境探测批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| OpenResty、MySQL、PostgreSQL、Redis 和 Docker 安装/运行探测 | `/www/apps/1Panel/agent/app/api/v2/app_install.go:CheckAppInstalled`、`agent/app/service/nginx.go`、`database*.go` | `POST /apps/installed/check` | `node/service/environment.go:ProbeApplication`、`node/api/apps.go:handleAppPost` | `POST /api/v2/apps/installed/check` 返回 `isExist`、`isActive`、`status`、`version` 和上下文错误 | 节点会话鉴权（上层中间件） | 受限可执行文件探测、版本命令和本机 TCP 默认端口；Docker 使用 daemon 探测 | 无状态探测；已有安装记录仅作为未知/容器化应用的回退 | `node/service/environment_test.go`、`node/api/apps_test.go:TestAppInstalledCheckUsesEnvironmentProbe` | `node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test -count=1 ./node/service ./node/api -run 'ProbeApplication|AppInstalledCheck'"` | 本地模拟二进制已验证；主次节点需安装对应运行时后验收 | implemented | 未配置探测器的第三方应用仍依赖 apps.json 登记；数据库凭据登录验证需显式配置后启用 |
| 云端备份 Bucket 通用查询 | `/www/apps/1Panel/agent/cron/job/backup.go`、备份提供商适配器 | `POST /backups/buckets` | `node/api/functional_domains.go:handleBackupBuckets`、`normalizeBuckets` | `POST /api/v2/backups/buckets` 支持 `buckets_url/bucket_url/endpoint`、Bearer、分页上限 500 | 节点会话鉴权（上层中间件）；凭据仅从账号 Vars 读取 | 显式配置的 HTTP(S) 提供商列表接口，响应可为数组或 `buckets/items/data` | 账号 Vars 原子保存，访问令牌不返回前端 | `node/api/functional_domains_test.go:TestBackupBucketsUsesConfiguredProviderEndpoint` | `node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test -count=1 ./node/api -run BackupBuckets"` | 本地模拟提供商已验证；生产厂商 SDK 需按凭据验收 | implemented | 尚未内置各云厂商 SDK，需配置标准 Bucket URL；未配置时明确返回 `BACKUP_PROVIDER_UNAVAILABLE` |

## 2026-08-31 云备份写入与 SSL 自动续期增强

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 云备份 Provider OAuth、Bucket、上传和删除 | `/www/apps/1Panel/agent/app/service/backup.go`、`agent/utils/cloud_storage/*` | `POST /api/v2/backups/buckets`、`refresh/token`、`backup`、`record/del` | `node/service/backup_provider.go`、`node/api/functional_domains.go` | POST `/api/v2/backups/buckets`、`/api/v2/backups/refresh/token`、`/api/v2/backups/backup`、`/api/v2/backups/record/del` | 节点会话；云端 Bearer/API Key 仅服务端 | 账号 Vars 显式 `*_url` 端点；multipart 流式文件 | `domains.json` 原子写入；OAuth 令牌仅保存服务端 | `node/service/backup_provider_test.go`、`node/api/functional_domains_test.go:TestBackupCloudUploadAndDeleteUseProvider` | `go test ./node/service ./node/api -run 'Backup(Cloud|Buckets)'` | 本地模拟 Provider 已验证；生产凭据待配置 | implemented | 具体厂商 SDK/S3 签名需按部署凭据接入；无端点时明确失败 |
| SSL 后台续期失败重试 | `/www/apps/1Panel/agent/cron/job/website.go`、`ssl.go` | 后台每小时续期扫描 | `node/service/website_security.go:RenewDueCertificates` | 后台任务，无 HTTP | 进程生命周期；不接受外部命令 | `website-ca-ssls.json` 与 CA 私钥 | 原子 JSON rename；最多 3 次指数退避重试 | `node/service/ssl_test.go:TestWebsiteSecurityRenewRetriesAndReportsFailure` | `go test ./node/service -run WebsiteSecurityRenew` | 本地验证；ACME 外部挑战仍需显式授权 | implemented | ACME 云端挑战与 DNS API 需配置适配器后启用 |

| 媒体文件异步转换 | `/www/apps/1Panel/agent/app/service/file.go:Convert`、`utils/convert/convert.go` | `POST /api/v2/files/convert` | `node/api/files_routes.go:runMediaConversion` | 支持 files 批量、任务 ID、转换器超时、输出原子提交、删除源文件选项 | 节点会话/HMAC（上层中间件） | 本地输入文件与 `WORKMESH_MEDIA_CONVERTER` | `WORKMESH_DATA_DIR/files.json` 的 ConvertLogs，最多 2000 条 | `node/api/files_routes_test.go:TestConvertAndConvertLogPersistence` | `go test ./node/api -run TestConvertAndConvertLogPersistence` | 本地 HTTP 已验证 | implemented | 转换器需由部署配置提供 |
| 媒体转换日志分页 | `/www/apps/1Panel/agent/app/service/file.go:ConvertLog` | `POST /api/v2/files/convert/log` | `node/api/files_routes.go:fileAdvancedHandler` | 按 taskID、status、type 过滤并分页返回真实成功/失败记录 | 节点会话/HMAC（上层中间件） | ConvertLogs 持久化记录 | 同 files.json 原子写入 | `node/api/files_routes_test.go:TestConvertAndConvertLogPersistence` | `go test ./node/api -run TestConvertAndConvertLogPersistence` | 本地 HTTP 已验证 | implemented | 大规模日志归档策略待补充 |

## 文件、备份与日志增强批次（2026-08-31）

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 文件分片上传与断点合并 | `/www/apps/1Panel/agent/app/api/v2/file.go:UploadChunkFiles` | `POST /api/v2/files/chunkupload` | `node/api/files_routes.go:handleChunkUpload` | multipart 分片、偏移校验、完成后原子 rename | 节点会话/HMAC（上层中间件） | 本地文件分片 | `WORKMESH_DATA_DIR/chunks` 临时分片，目标文件原子提交 | `node/api/files_routes_test.go:TestChunkUploadAndDownload` | `go test ./node/api -run Chunk` | 本地 HTTP 已验证 | implemented | 跨节点分片同步待接入 |
| 文件分片下载 | `/www/apps/1Panel/agent/app/api/v2/file.go:DownloadChunkFiles` | `POST /api/v2/files/chunkdownload` | `node/api/files_routes.go:handleChunkDownload` | Range/offset 流式读取 | 节点会话/HMAC | 本地文件 | 无状态 | `node/api/files_routes_test.go:TestChunkUploadAndDownload` | `go test ./node/api -run Chunk` | 本地 HTTP 已验证 | implemented | 下载审计日志待接入 |
| 文件历史版本与备注 | `/www/apps/1Panel/agent/app/service/file_history.go`、`file.go` | `/api/v2/files/history/*`、`/remarks`、`/remark` | `node/api/files.go`、`node/api/files_routes.go` | 保存前快照、查询、恢复、删除、备注读写 | 节点会话/HMAC | 文件内容与请求参数 | `WORKMESH_DATA_DIR/files.json` 原子写入，最多保留 200 条历史 | `node/api/files_routes_test.go:TestFileHistoryAndAdvancedOperations` | `go test ./node/api -run History` | 本地 HTTP 已验证 | implemented | 大文件历史快照按大小跳过 |
| 压缩/解压原子写入 | `/www/apps/1Panel/agent/app/api/v2/file.go` | `POST /api/v2/files/compress`、`decompress` | `node/api/files_routes.go:zipPath/unzipPath` | ZIP 创建、路径穿越和符号链接拒绝 | 节点会话/HMAC | 本地文件 | 同目录临时文件后原子 rename | `node/api/files_routes_test.go:TestZipAndUnzipPath` | `go test ./node/api -run Zip` | 本地 HTTP 已验证 | implemented | 长任务取消接口待接入任务调度器 |
| 日志检索分页与按类型清理 | `/www/apps/1Panel/agent/app/api/v2/logs.go`、`core/app/api/v2/logs.go` | `/api/v2/logs/*`、`/api/v2/core/logs/*` | `node/api/functional_domains.go:registerLogRoutes` | 关键字、类型、级别过滤及分页；清理支持 logType | 节点会话/HMAC | 运行时日志状态与受控日志文件 | `WORKMESH_DATA_DIR/domains.json` 原子写入 | `node/api/functional_domains_test.go`、`logs_runtime_test.go` | `go test ./node/api -run 'Log|Backup'` | 本地 HTTP 已验证 | implemented | 生产日志采集器需按部署启用 |

## 2026-08-31 Agent 资源语义批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Agent 备注、令牌重置和网站绑定 | `/www/apps/1Panel/agent/app/api/v2/agents.go:UpdateAgentRemark/ResetAgentToken/BindAgentWebsite` | `POST /api/v2/ai/agents/remark`、`/token/reset`、`/website/bind`、`/website/unbind` | `node/api/ai_execution.go:handleAgentRoute` | 同左 | Session/Bearer；上层节点鉴权与 CSRF | Agent 持久化记录，令牌使用随机字节生成 | `ai.json` 原子 rename；响应令牌脱敏 | `node/api/ai_execution_test.go:TestAgentResourceMutationsAndSessionLifecycle` | `go test ./node/api -run AgentResource` | 本地 HTTP 已验证 | implemented | 网站资源存在性由网站域后续联动校验 |
| Agent 角色创建、绑定、解绑、删除 | `/www/apps/1Panel/agent/app/api/v2/agents.go:CreateAgentRole/BindAgentRole/UnbindAgentRole/DeleteAgentRole` | `POST /api/v2/ai/agents/agent/{create,bind,unbind,delete}` | `node/api/ai_execution.go:handleAgentRoute` | 同左 | Session/Bearer；校验父 Agent 归属 | Agent.roles 角色数组 | `ai.json` 原子 rename，重复角色返回冲突 | `node/api/ai_execution_test.go:TestAgentResourceMutationsAndSessionLifecycle` | `go test ./node/api -run AgentResource` | 本地 HTTP 已验证 | implemented | 渠道运行时重启需外部 Agent 容器支持 |
| Hermes 会话重命名与删除 | `/www/apps/1Panel/agent/app/api/v2/agents.go:RenameHermesChatSession/DeleteHermesChatSession` | `POST /api/v2/ai/agents/hermes/chat/sessions/{rename,delete}` | `node/api/ai_execution.go:handleSessionMutation` | 同左 | Session/Bearer；校验 Agent 和会话归属 | `ai.json` Sessions | 原子 rename；不存在资源返回 404 | `node/api/ai_execution_test.go:TestAgentResourceMutationsAndSessionLifecycle` | `go test ./node/api -run AgentResource` | 本地 HTTP 已验证 | implemented | 会话消息正文仍由 Agent 运行时保存 |
| Ollama/MCP 启停和模型详情 | `/www/apps/1Panel/agent/app/api/v2/ai.go`、`mcp_server.go` | `POST /api/v2/ai/ollama/close`、`/ollama/model/load`、`/ollama/model/recreate`、`/mcp/server/op` | `node/api/ai_execution.go:handleAIResourceOperation` | 同左 | Session/Bearer；资源 ID/名称必填 | 本地 AI 状态记录 | `ai.json` 原子 rename；不存在资源不创建伪记录 | `node/api/ai_execution_test.go:TestAIResourceOperationsRequireExistingResource` | `go test ./node/api -run AIResource` | 本地 HTTP 已验证 | implemented | 真实 Ollama/MCP 进程编排需部署运行时配置 |

## 2026-08-31 AI 实时协议与语言包补齐

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| AI 错误按请求语言返回 | `/www/apps/1Panel/core/i18n`、`agent/app/api/v2/ai.go` | `/api/v2/ai/*` 错误响应 | `node/api/errors.go`、`node/api/ai_execution.go` | 同左，依据 `Accept-Language` | 上层节点会话/Bearer | 内嵌 12 种语言 YAML 与稳定错误码 | 只读内存缓存 | `node/api/ai_execution_test.go:TestAIErrorUsesRequestLocale` | `go test ./node/api -run AIErrorUsesRequestLocale` | 本地验证；待生产多语言验收 | implemented | 未映射的新错误码回退原文 |
| 容器日志 SSE 断线游标 | `/www/apps/1Panel/agent/app/api/v2/container.go:ContainerStreamLogs` | `GET /api/v2/containers/search/log` | `node/api/container_log_stream.go` | GET SSE，支持 `Last-Event-ID`、事件 id、心跳和取消 | 流令牌或本地会话 | Docker/Compose 日志输出 | 流式无状态；连接内序号 | `node/api/container_log_stream_test.go:TestContainerSSELastEventIDContinuesSequence` | `go test ./node/api -run ContainerLog` | 待真实 Docker/反代断线验收 | implemented | 断线后的历史行依赖 Docker `since`，不保存无界缓存 |
| 终端 WebSocket 控制帧和关闭握手 | `/www/apps/1Panel/agent/app/api/v2/terminal.go`、`core/app/api/v2/process.go` | `GET /api/v2/hosts/terminal/{local,ssh,container}` | `node/api/websocket_stream.go`、`node/api/terminal_stream.go` | GET WebSocket，Ping/Pong、Close code、控制帧上限 | 流令牌/登录会话、Origin 校验 | 本地 Shell、SSH、Docker exec | 连接生命周期态，不落盘 | `node/api/stream_protocol_test.go` | `go test ./node/api -run WebSocket` | 待生产 PTY/SSH 验收 | implemented | 标准库无法跨平台 PTY resize，仍接受 resize 消息 |
# 计划任务与命令脚本补齐记录（2026-08-31）

| 功能名称 | 来源模块 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|
| 计划任务模型与类型 | agent cronjob | node/model/command.go、node/service/cronjob.go | POST /api/v2/cronjobs | 节点会话/HMAC | 请求参数与本地资源 | WORKMESH_DATA_DIR/cronjobs.json | node/service/cronjob_test.go | implemented | 网站/数据库/应用备份适配器待接入 |
| 计划任务调度 | agent cronjob_helper | node/service/cronjob.go | POST /api/v2/cronjobs/next | 节点会话 | cron 表达式 | 任务定义 | node/service/cronjob_test.go | implemented | 时区配置待补充 |
| 任务重试超时 | agent task runtime | node/service/cronjob.go | POST /api/v2/cronjobs/handle | 节点会话/HMAC | RetryTimes/Timeout | 执行记录 | node/service/cronjob_test.go | implemented | 分布式 fencing 待接入 |
| 记录分页清理 | agent cronjobRepo | node/service/cronjob.go、node/api/host_container_cron.go | POST /api/v2/cronjobs/search/records | 节点会话 | 本地执行记录 | cronjobs.json（最多 1000 条/任务） | node/service/cronjob_test.go | implemented | 记录文件日志关联待补充 |
| 脚本库持久化与审核执行 | core script library | node/api/core_resources.go | POST /api/v2/core/script、GET/WS /api/v2/core/script/run | 管理员 Session；服务调用可使用 `X-WorkMesh-Token` | SQLite `script_library` | 事务写入 | node/api/core_resources_test.go | implemented | 远程签名同步待接入 |
| 命令执行白名单 | core command | node/service/cronjob.go、node/service/command.go | POST /api/v2/system/command | X-WorkMesh-Token | 白名单程序与参数 | 审计日志 | node/service/command_test.go | implemented | 完整审计查询待补充 |
## 主机与容器功能补齐
| 功能名称 | 来源模块 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|
| 主机信息与诊断 | hosts | `node/api/hosts.go` | GET `/api/v2/hosts/info`、`/diagnostics/summary` | 节点会话 | runtime、系统主机信息 | 无状态实时采集 | `node/api/hosts_containers_test.go` | implemented | 防火墙和 SSH 专用驱动待补 |
| 主机记录 CRUD 与分组 | hosts | `node/api/hosts.go` | POST `/api/v2/hosts`、`/info`、`/update`、`/del`、`/search`、`/tree` | 节点会话/HMAC | 主机记录 | `WORKMESH_DATA_DIR/hosts.json` 原子写入 | `node/api/hosts_containers_test.go` | implemented | 远程连通性需接入 SSH 驱动 |
| Docker 容器生命周期 | containers | `node/api/containers.go`、`node/service/docker.go` | POST `/api/v2/containers`、`/operate`、`/update`、`/rename`、`/commit`、`/prune` | 节点会话/HMAC | Docker CLI | Docker daemon | `node/api/hosts_containers_test.go` | implemented | 资源配额字段需按平台扩展 |
| Docker 服务状态探测 | containers | `/www/apps/1Panel/agent/app/api/v2/container.go`、`service/docker.go` | `node/api/host_container_cron.go`、`node/api/containers.go`、`node/service/docker.go` | GET `/api/v2/containers/docker/status`、`/api/v2/containers/status` | 节点会话/HMAC | `exec.LookPath` 与 `docker version`（10 秒超时） | 无状态实时探测 | `node/api/hosts_containers_test.go:TestDockerStatusContract`、`node/service/docker_status_test.go` | `go test ./node/api ./node/service -run DockerStatus` | 本地已验证 | implemented | 仍需在生产主机验证 Docker CLI/daemon 版本字段 |
| Docker 容器文件 | containers | `node/api/containers.go` | POST `/api/v2/containers/files/{search,content,size,del,upload,download}` | 节点会话/HMAC | Docker exec/cp | 容器文件系统 | `node/api/hosts_containers_test.go` | implemented | 大文件下载需流式响应 |
| Docker 镜像、网络、卷 | containers | `node/api/containers.go` | GET/POST `/api/v2/containers/image*`、`network*`、`volume*` | 节点会话/HMAC | Docker CLI | Docker daemon | `node/api/hosts_containers_test.go` | implemented | 仓库与模板管理待接入持久化 |

## 2026-08-31 启动初始化与 CLI 制品安全

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 服务启动数据目录初始化 | `/www/apps/1Panel/core/init`、`agent/init` | 进程启动入口 | `cmd/workmesh-server/main.go`、`cmd/workmesh-server/cli.go:initializeDataDir` | 无 HTTP；启动前初始化 `apps/backups/logs/releases/runtime/uploads` | 本地进程权限 | `WORKMESH_DATA_DIR` | 目录 0750，状态文件由各域原子写入 | `cmd/workmesh-server/cli_test.go:TestInitializeDataDirCreatesRuntimeLayout` | `go test ./cmd/workmesh-server -run InitializeDataDir` | 待 Linux 权限验收 | implemented | 生产挂载点和磁盘配额需部署确认 |
| 签名制品更新与恢复 | `/www/apps/1Panel/core/cmd/server/cmd/restore.go`、`update.go` | `restore`、`update` CLI | `cmd/workmesh-server/cli.go:installSignedArtifact`、`node/api/deployment_runtime.go:RecoverDeploymentState` | CLI：`restore|update <artifact> [--signature] [--public-key] [--target] [--sha256] [--version]` | 本地管理员调用；Ed25519 签名必须有效 | 制品文件、SHA-256、签名与公钥 | `deployment-artifact.json` 原子写入；目标保留 previous 备份；重启校验后恢复活动状态 | `cmd/workmesh-server/cli_test.go:TestCLIUpdateVerifiesSignatureAndAtomicallyInstalls`、`node/api/deployment_runtime_test.go:TestRecoverDeploymentStateRestoresValidArtifact` | `go test ./cmd/workmesh-server ./node/api -run 'CLI|RecoverDeploymentState'` | 云端发布凭据待配置 | implemented | 真实发布服务和跨节点同步需网关凭据 |

## 2026-08-31 节点身份与透传链路批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Gateway 节点 Ed25519 身份持久化 | `/www/apps/1Panel/agent/utils/nodeclient/identity.go`、`app/api/v2/workmesh_gateway.go` | Gateway 节点注册与心跳 | `runtime/gateway/identity.go`、`runtime/gateway/http_client.go`、`control/api/gateway.go` | 启动加载/独占创建 `gateway-identity.ed25519`；注册必须返回 bindingId | 注册接口使用配置 Gateway 凭据；私钥文件权限 0600 | 本地生成 Ed25519 公私钥及节点配置 | `WORKMESH_DATA_DIR/gateway-identity.ed25519` 原子创建并校验权限 | `runtime/gateway/identity_test.go`、`runtime/gateway/http_client_test.go` | `go test ./runtime/gateway ./control/api` | 待配置真实 Gateway 后验证绑定持久化 | implemented | 无 Gateway 凭据时明确报错，不伪造绑定成功 |
| CurrentNode/operateNode 节点通用透传 | `/www/apps/1Panel/core/init/router/proxy.go`、`agent/utils/nodeclient/client.go` | `/api/v2/**?operateNode=<nodeId>`、`CurrentNode` 请求头 | `node/api/node_relay.go`、`cmd/workmesh-server/main.go` | 保留原 METHOD/PATH/查询参数（移除 operateNode），支持 CurrentNode；请求/响应上限 8 MiB | 节点 HMAC、timestamp、nonce、role epoch；目标节点验签后进入处理器 | `nodes.json` 节点地址及角色状态 | nonce 进程内短期缓存；节点清单文件缓存 | `node/api/node_relay_test.go`、`control/api/security_middleware_test.go` | `go test ./node/api ./control/api ./cmd/workmesh-server` | 待主/次节点配置共享密钥后验证跨机透传 | implemented | 生产必须配置 `WORKMESH_LINK_SECRET` 并确认 role epoch 同步 |
| 节点同步快照跨重启恢复 | `/www/apps/1Panel/core/utils/xpack/helper/multi_node_helper.go` | 节点同步状态查询/更新 | `runtime/link/file_store.go`、`control/api/link.go` | 同步游标 compare-and-set，冲突返回错误 | 本地节点会话/HMAC（控制面） | 同步流 ID、版本及快照数据 | `WORKMESH_DATA_DIR/link-sync.json` 原子 rename，最多 128 流/8 MiB | `runtime/link/file_store_test.go` | `go test ./runtime/link ./control/api` | 待主/次节点重启后验证游标连续性 | implemented | 需在生产磁盘配额和多节点并发下复测 |
## 2026-08-31 终端 PTY 与流式协议

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 本地终端真实 PTY、双向输入输出和窗口调整 | `/www/apps/1Panel/core/utils/terminal/local_cmd.go`、`ws_local_session.go` | `GET /api/v2/hosts/terminal/local` | `node/api/terminal_pty.go`、`terminal_stream.go` | WebSocket；`cmd`/`heartbeat`/`resize`，列行范围 1-500 | 流令牌或本地 Session、Origin | 本地 Shell PTY | 会话生命周期态，不落盘 | `node/api/stream_protocol_test.go` | `go test ./node/api -run 'Terminal|WebSocket'` | Unix 主机需真实 PTY 冒烟 | implemented | Windows 使用管道回退，resize 明确返回不可用 |
| 容器终端 TTY 与断开清理 | `/www/apps/1Panel/agent/app/api/v2/terminal.go`、`utils/terminal/local_cmd.go` | `GET /api/v2/hosts/terminal/container` | `node/api/terminal_stream.go`、`terminal_pty.go` | Docker `exec -i -t`，输入输出/关闭码/超时 | 流令牌或本地 Session、Origin | Docker exec PTY | 会话生命周期态，不落盘 | `node/api/stream_protocol_test.go` | `go test ./node/api -run Terminal` | 需运行 Docker 容器验收 | partial | 生产 Docker daemon 和容器终端信号尚未联调 |
| SSH 终端远端 PTY | `/www/apps/1Panel/agent/app/api/v2/terminal.go`、`utils/terminal/ws_session.go` | `GET /api/v2/hosts/terminal/ssh` | `node/api/terminal_stream.go` | SSH `-tt`，BatchMode、ConnectTimeout、双向帧 | 流令牌/节点鉴权；凭据来自 SSH Agent 或主机配置 | 远端 SSH 会话 | 会话生命周期态，不落盘 | `node/api/stream_protocol_test.go` | `go test ./node/api -run TerminalCommand` | 需 SSH 凭据和远端主机验收 | partial | 当前不接收 URL 密码/私钥，远端 WindowChange 需 SSH 库或代理支持 |
| 容器日志 SSE 背压与断线重放 | `/www/apps/1Panel/agent/app/api/v2/container.go` | `GET /api/v2/containers/search/log` | `node/api/container_log_stream.go` | SSE `id`、`Last-Event-ID`、ready/heartbeat/close、10 秒写超时、有界 128 KiB/256 事件缓存 | 流令牌或本地 Session | Docker/Compose 日志 | 进程内有界重放缓存 | `node/api/container_log_stream_test.go` | `go test ./node/api -run 'ContainerSSE|ContainerLog'` | 需反向代理慢连接和 Docker 实例验收 | partial | 重放缓存不跨进程重启，生产反代断线需 E2E 验证 |
| 网站监控访问日志统计 | `/www/apps/1Panel/agent/app/api/v2/website.go`、`app/service/website.go` | `POST /api/v2/websites/monitor/{stat,qps,rank,trend,visitors,visitors/loc}` | `node/api/analytics.go` | 读取 Nginx/OpenResty combined access log，支持时间范围、PV/UV、状态码、流量、爬虫、浏览器/系统/设备聚合；读取上限 8 MiB、事件上限 50000 | Session/Cookie 或 Bearer/API Key（由统一中间件校验） | `WORKMESH_ANALYTICS_LOG` 或数据目录/系统日志 | 无状态读取，日志由 OpenResty 持久化 | `node/api/analytics_test.go` | `go test ./node/api -run Analytics` | 需生产 OpenResty 日志格式和轮转策略验收 | implemented | 未接入 GeoIP，访客地域仅返回 IP |
| 进程监听端口查询 | `/www/apps/1Panel/agent/app/api/v2/process.go`、`app/service/process.go` | `POST /api/v2/process/listening` | `node/api/process.go` | 受控执行 `ss -lntup`，失败回退 `netstat -ano`，解析前两地址字段并限制最多 1024 条 | Session/Cookie 或 `X-WorkMesh-Token` | 主机网络栈命令输出 | 无状态 | `node/api/process_test.go` | `go test ./node/api -run ParseListening` | 需 Linux/Windows 主机命令可用性验收 | implemented | Windows `netstat` 输出字段需在目标系统抽样验证 |

## 2026-08-31 主机运维与容器镜像批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 主机监控网络和 IO 选项 | `/www/apps/1Panel/agent/router/ro_host.go`、`agent/app/api/v2/host.go` | `GET /api/v2/hosts/monitor/netoptions`、`GET /api/v2/hosts/monitor/iooptions` | `node/api/host_container_cron.go:registerHostOperationalRoutes` | GET 同左；返回真实网络接口、可用 IO 维度 | 节点会话/HMAC | `net.Interfaces` 与固定协议选项 | 无状态 | `node/api/host_container_cron_operational_test.go` | `go test ./node/api -run HostOperational` | 待主/次节点系统接口核验 | implemented | IO 统计采集需按平台扩展 |
| 主机监控配置与清理 | `/www/apps/1Panel/agent/app/service/monitor.go` | `GET /api/v2/hosts/monitor/setting`、`POST /api/v2/hosts/monitor/setting/update`、`POST /api/v2/hosts/monitor/clean` | `node/api/host_monitor.go` | 五个监控设置字段的读取、校验和历史清空 | 节点会话/HMAC | 请求中的 key/value | `node_settings.host_operational`，无 SQLite 时回退 JSON | `node/api/host_container_cron_operational_test.go:TestHostMonitorSettingsPersistAndValidate` | `go test ./node/api -run 'TestHostMonitor'` | 待生产设置验收 | implemented | Disable 会停止采样循环 |
| 主机历史监控 | `/www/apps/1Panel/agent/app/service/monitor.go` | `POST /api/v2/hosts/monitor/search` | `node/api/host_monitor.go` | 按 param、网卡、磁盘和时间范围返回 base、io、network 序列 | 节点会话/HMAC | SQLite 历史行 | `monitor_bases`、`monitor_ios`、`monitor_networks` | `node/api/host_monitor_test.go:TestHostMonitorSampleSearchAndClean` | `go test ./node/api -run 'TestHostMonitor'` | 待生产曲线验收 | implemented | 单次最多 20000 点，超期行在采样时删除 |
| 主机磁盘状态 | `/www/apps/1Panel/agent/router/ro_host.go` | `GET /api/v2/hosts/disks` | `node/api/host_container_cron.go:hostDiskInfo` | GET 同左；返回可访问挂载路径与目录属性 | 节点会话/HMAC | 本机文件系统 | 无状态 | `node/api/host_container_cron_operational_test.go` | `go test ./node/api -run HostOperational` | 待真实挂载点容量采集验证 | implemented | 详细容量字段需平台 statfs 适配 |
| 防火墙能力探测 | `/www/apps/1Panel/agent/router/ro_host.go` | `GET /api/v2/hosts/firewall/settings`、`POST /api/v2/hosts/firewall/base` | `node/api/host_container_cron.go:firewallStatus` | GET/POST 同左；探测 ufw/firewall-cmd/iptables，不伪造规则 | 节点会话/HMAC | `exec.LookPath` | 无状态 | `node/api/host_container_cron_operational_test.go` | `go test ./node/api -run HostOperational` | 待目标系统防火墙联调 | implemented | 规则增删操作需独立驱动授权 |
| 主机工具状态 | `/www/apps/1Panel/agent/router/ro_host.go`、`host_tool.go` | `POST /api/v2/hosts/tool/status` | `node/api/host_container_cron.go:registerHostOperationalRoutes` | POST 同左；按请求类型探测本机程序 | 节点会话/HMAC | PATH 中的工具 | 无状态 | `node/api/host_container_cron_operational_test.go` | `go test ./node/api -run HostOperational` | 待 Supervisor/systemd 版本核验 | implemented | 工具配置写入与进程操作需后续白名单实现 |
| Docker 镜像归档导入 | `/www/apps/1Panel/agent/router/ro_container.go` | `POST /api/v2/containers/image/load` | `node/api/host_container_cron.go` | POST 同左；受限绝对路径调用 `docker load -i` | 节点会话/HMAC | Docker CLI 与归档文件 | Docker daemon | `node/api/host_container_cron_operational_test.go:TestContainerImageLoadRejectsUnsafeArchivePath` | `go test ./node/api -run ContainerImageLoad` | 待 Docker 主机导入实测 | implemented | 大文件导入进度需异步任务 |

| 容器化 OpenResty 安装/运行探测 | `/www/apps/1Panel/agent/app/service/nginx.go`、`agent/app/api/v2/app_install.go` | `POST /api/v2/apps/installed/check`、`GET /api/v2/openresty/status` | `node/service/environment.go:probeOpenRestyContainer`、`node/service/website.go:ProbeOpenResty`、`node/api/apps.go` | 固定参数读取 Docker 运行容器，返回容器名、镜像版本、80/443 端口和运行状态 | 节点会话/HMAC | Docker 容器清单和镜像标签，命令超时 3 秒 | 无状态探测 | `node/service/website_test.go:TestParseOpenRestyContainerList`、线上主节点冒烟 | `go test ./node/service ./node/api -run OpenResty` | 主节点 `WorkMesh-openresty-0WEK` 已验证 `isExist=true`、`Running`、版本 `1.21.4.3-3-3-focal` | implemented | 容器启停仍复用既有应用操作接口 |
| OpenResty 状态数组契约 | `/www/apps/1Panel/agent/app/api/v2/nginx.go` | `GET /api/v2/openresty/status`、`GET /api/v2/openresty/modules` | `node/service/website.go`、`node/api/website.go` | 无模块时返回 `[]` 而非 `null`，避免前端表格 `reduce`/迭代异常 | 节点会话/HMAC | OpenResty 配置状态 | `openresty.json` | `node/service/website_test.go`、`node/api/website_test.go` | `go test ./node/service ./node/api -run OpenResty` | 主节点已部署 | implemented | 无 |
