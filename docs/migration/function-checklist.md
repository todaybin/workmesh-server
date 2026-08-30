<!-- SPDX-License-Identifier: GPL-3.0-only -->

## 2026-08-30 数据服务与 OpenResty 真实运行时补齐

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| MySQL/PostgreSQL/Redis/Mongo 数据库端点检查 | `apps/workmesh-node/agent/router/ro_database.go`、`agent/app/service/database*.go` | `/databases/db/check`、`/databases/redis/check`、`/databases/status` | `node/api/database_routes.go`、`node/service/database.go` | `POST /api/v2/databases/db/check`、`POST /api/v2/databases/redis/check`、`POST /api/v2/databases/status` | 节点会话鉴权 | 目标主机 TCP 连接，按类型默认端口 3306/5432/6379/27017 | 无状态探测；操作审计 `database-operations.json` | `node/api/database_test.go` | `go test ./node/api -run Database` | 待主次节点人力联调 | implemented | 未提供账号时仅验证网络可达性，不执行 SQL 登录 |
| 数据库元数据新增、查询、分页、删除 | `apps/workmesh-node/agent/router/ro_database.go` | `/databases/db`、`/databases/db/search`、`/databases/db/:name`、`/databases/db/del` | `node/api/database.go`、`node/api/database_routes.go`、`node/service/database.go` | `POST/GET /api/v2/databases/db*` | 节点会话鉴权；密码字段不落库 | 本地数据库登记信息 | `WORKMESH_DATA_DIR/databases.json` 原子替换 | `node/api/database_test.go`, `node/service/database_test.go` | `go test ./node/api ./node/service -run Database` | 待主次节点重启恢复验证 | implemented | 用户/授权/变量等 DB 专属 SQL 管理仍需按凭据启用 |
| OpenResty 版本与配置语法探测 | `apps/workmesh-node/agent/app/api/v2/nginx.go`、`agent/app/service/nginx.go` | `GET /openresty/status`、`GET /openresty/https` | `node/service/website.go:ProbeOpenResty`、`node/api/website.go:registerOpenRestyRoutes` | `GET /api/v2/openresty/status`、`GET /api/v2/openresty/https` | 节点会话鉴权 | 受限 `openresty -v/-t` 或 `nginx -v/-t`，命令超时 2 秒 | `openresty.json` 保存控制面配置 | `node/service/website_test.go`, `node/api/website_test.go` | `go test ./node/service ./node/api -run OpenResty` | 待主节点安装 OpenResty 后实测 | implemented | 未安装二进制时明确返回 `available=false`，不伪造运行状态 |

## 2026-08-30 核心认证、脚本与进程批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Passkey 凭据元数据管理 | `apps/workmesh-node/core/app/service/auth.go` | `GET /api/v2/core/auth/passkey/list`、`POST /api/v2/core/auth/passkey/register/*`、`POST /api/v2/core/auth/passkey/del` | `control/service/core.go`、`node/api/core_handlers.go` | 同左 | Session/Cookie 或 Bearer；注册挑战 5 分钟有效 | 凭据 ID、名称、创建时间 | `WORKMESH_DATA_DIR/passkeys.json`，临时文件原子替换 | `node/api/core_handlers_test.go:TestCorePasskeyRegistrationLifecycle` | `go test ./node/api -run CorePasskey` | 待双节点制品部署 | implemented | 未接入浏览器 WebAuthn 验证器时，空 credentialId 会明确拒绝 |
| 脚本库受控执行 | `apps/workmesh-node/core/app/api/v2/script_library.go:RunScript` | `GET /api/v2/core/script/run` | `node/api/core_resources.go:handleScriptRun` | `GET /api/v2/core/script/run` | `X-WorkMesh-Token` 与 `WORKMESH_COMMAND_TOKEN` | 已登记脚本库记录，不接受直接 command 参数 | core resource store | `node/api/core_resources_test.go` | `go test ./node/api -run ScriptRun` | 待真实脚本库部署验证 | implemented | 仅允许 script_id 对应脚本，禁止任意宿主命令 |
| 进程详情采集 | `apps/workmesh-node/agent/app/service/process.go:GetProcessInfoByPID` | `GET /api/v2/process/:pid` | `node/api/process.go:handleProcessByID` | `GET /api/v2/process/:pid` | 节点会话鉴权 | `/proc/<pid>/cmdline`、`/proc/<pid>/status` | 无状态实时采集 | `node/api/process_test.go` | `go test ./node/api -run Process` | 待 Linux 节点验证 | implemented | Windows 无 procfs 时仅返回可访问字段 |
<!-- Copyright (c) 2026 WorkMesh contributors -->

## 2026-08-30 日志读取批次
| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 系统日志文件枚举 | `apps/workmesh-node/agent/app/service/logs.go:ListSystemLogFile` | `GET /api/v2/logs/system/files` | `apps/workmesh-server/node/api/functional_domains.go:listSystemLogFiles` | `GET /api/v2/logs/system/files` | 节点会话鉴权 | `WORKMESH_DATA_DIR/logs`、Linux `/var/log` | 无状态读取 | `node/api/logs_runtime_test.go` | `go test ./node/api -run SystemLog` | 待制品部署 | implemented | 仅枚举日志文件 |
| 系统日志服务状态 | `apps/workmesh-node/agent/app/service/logs.go:GetSystemLogStatus` | `GET /api/v2/logs/system/status` | `apps/workmesh-server/node/api/functional_domains.go:systemLogStatus` | `GET /api/v2/logs/system/status` | 节点会话鉴权 | `journalctl --version` 或文件降级 | 无状态读取 | `node/api/logs_runtime_test.go` | `go test ./node/api -run SystemLog` | 待制品部署 | implemented | 无 journalctl 时返回 file 状态 |
| 运行中系统服务 | `apps/workmesh-node/agent/app/service/logs.go:ListRunningServices` | `GET /api/v2/logs/system/services` | `apps/workmesh-server/node/api/functional_domains.go:listRunningSystemServices` | `GET /api/v2/logs/system/services` | 节点会话鉴权 | `systemctl` 或 Windows `tasklist`，5 秒超时 | 无状态读取 | `node/api/logs_runtime_test.go` | `go test ./node/api -run SystemLog` | 待制品部署 | implemented | 命令不可用时返回空集合 |
| 主机系统日志读取 | `apps/workmesh-node/agent/app/service/logs.go:ReadSystemLog` | `POST /api/v2/logs/system/read` | `apps/workmesh-server/node/api/functional_domains.go:readLogFile` | `POST /api/v2/logs/system/read` | 节点会话鉴权 | 允许目录内日志文件，单次最多 2 MiB | 无状态读取 | `node/api/logs_runtime_test.go` | `go test ./node/api -run SystemLog` | 待制品部署 | implemented | journalctl 过滤参数待扩展 |
| 任务日志分页读取 | `apps/workmesh-node/agent/app/service/task.go:ReadByLine` | `POST /api/v2/logs/tasks/read` | `apps/workmesh-server/node/api/functional_domains.go:readTaskLog` | `POST /api/v2/logs/tasks/read` | 节点会话鉴权 | 任务日志路径，分页最多 500 行 | 无状态读取 | `node/api/logs_runtime_test.go` | `go test ./node/api -run SystemLog` | 待制品部署 | implemented | 任务仓库接入待任务域完成 |
| 执行中任务计数 | `apps/workmesh-node/agent/app/service/task.go:CountExecutingTask` | `GET /api/v2/logs/tasks/executing/count` | `apps/workmesh-server/node/api/functional_domains.go:registerLogRoutes` | `GET /api/v2/logs/tasks/executing/count` | 节点会话鉴权 | domains.json 中 running/executing 任务日志 | `domains.json` | `node/api/logs_runtime_test.go` | `go test ./node/api -run SystemLog` | 待制品部署 | implemented | 可切换任务仓库实时计数 |

## 2026-08-30 运行时详情批次
| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| PHP 运行时详情 | `apps/workmesh-node/agent/app/service/runtime.go` | `GET /api/v2/runtimes/:id` | `apps/workmesh-server/node/api/runtime_toolbox.go:registerRuntimeRoutes` | `GET /api/v2/runtimes/{id}` | 节点会话鉴权 | `runtime.json` 运行时记录 | `WORKMESH_DATA_DIR/runtime.json` | `node/api/runtime_toolbox_test.go` | `go test ./node/api -run Runtime` | 待制品部署 | implemented | 运行时安装器由运行时域负责 |
| PHP 扩展及配置查询 | `apps/workmesh-node/agent/app/service/runtime.go` | `GET /api/v2/runtimes/php/:id/extensions`、`config`、`container`、`fpm/config`、`fpm/status` | `apps/workmesh-server/node/api/runtime_toolbox.go:registerRuntimeSubroutes` | 同左 | 节点会话鉴权 | 运行时记录及扩展列表 | `runtime.json` | `node/api/runtime_toolbox_test.go` | `go test ./node/api -run Runtime` | 待制品部署 | implemented | FPM 深度指标待接入系统探针 |
| Supervisor 进程详情 | `apps/workmesh-node/agent/app/service/runtime.go` | `GET /api/v2/runtimes/supervisor/process/:id` | `apps/workmesh-server/node/api/runtime_toolbox.go:registerRuntimeSubroutes` | 同左 | 节点会话鉴权 | `runtime.json` supervisor 配置 | `runtime.json` | `node/api/runtime_toolbox_test.go` | `go test ./node/api -run Runtime` | 待制品部署 | implemented | 未配置进程返回 not_configured |

# WorkMesh 功能迁移清单

## 2026-08-30 网站配置别名批次
| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 网站 HTTPS/LBS/CORS 配置查询 | `apps/workmesh-node/agent/router/ro_website.go` | `GET /api/v2/websites/:id/https`, `GET /api/v2/websites/:id/lbs`, `GET /api/v2/websites/cors/:id` | `node/api/website.go:registerDomainRoutes/registerWebsiteConfigRoutes` | GET 双段动态分发 | 节点会话 | WebsiteService 配置 | `website-configs.json` 原子保存 | `node/api/website_test.go` | `go test ./node/api -run Website` | 待双节点部署 | implemented | 无 |
| 网站代理与真实 IP 配置查询 | `apps/workmesh-node/agent/router/ro_website.go` | `GET /api/v2/websites/proxy/config/:id`, `GET /api/v2/websites/realip/config/:id` | `node/api/website.go:registerWebsiteConfigRoutes` | GET 指定配置 | 节点会话 | WebsiteService 配置 | `website-configs.json` | `node/api/website_test.go` | `go test ./node/api -run WebsiteConfigAliases` | 待双节点部署 | implemented | 无 |
| 网站 DNS/CORS/LBS/代理/流配置更新 | `apps/workmesh-node/agent/router/ro_website.go` | `POST /api/v2/websites/dns/update`, `/cors/update`, `/lbs/create`, `/lbs/update`, `/lbs/file`, `/proxy/clear`, `/stream/update` | `node/api/website.go:registerWebsiteAdvancedRoutes` | POST 配置写入 | 节点会话、资源归属校验 | 请求 JSON | `website-configs.json` 按类型隔离 | `node/api/website_test.go:TestWebsiteConfigAliasesPersist` | `go test ./node/api -run WebsiteConfigAliases` | 待双节点部署 | implemented | DNS 记录解析器待后续增强 |
| 网站 DNS 查询与删除 | `apps/workmesh-node/agent/router/ro_website.go` | `POST /api/v2/websites/dns/search`, `/dns/del` | `node/api/website.go:registerWebsiteAdvancedRoutes` | POST | 节点会话、网站 ID 校验 | WebsiteService 配置 | `website-configs.json` | `node/api/website_test.go` | `go test ./node/api -run WebsiteConfigAliases` | 待双节点部署 | implemented | 删除采用标记并保留审计字段 |
| 网站监控配置与统计别名 | `apps/workmesh-node/agent/router/ro_website.go` | `GET/POST /api/v2/websites/monitor/config/*`, `/monitor/{stat,qps,rank,trend,visitors}` | `node/api/website.go:registerWebsiteAdvancedRoutes`、`node/api/analytics.go` | GET/POST | 节点会话 | analytics 状态采集 | `domains.json` 设置区 | `node/api/website_test.go:TestWebsiteConfigAliasesPersist` | `go test ./node/api -run WebsiteConfigAliases` | 待双节点部署 | implemented | 统计采集器接入真实访问日志后增强 |

本清单以旧 `apps/workmesh-node/core` 与 `agent` 的 863 条路由为基线（含隐藏 helper 注册和去品牌化静态入口）。状态必须以真实副作用或端到端响应确认，不能仅以路由注册作为完成依据。

非路由的初始化、后台作业、中间件、国际化、日志、任务和协议升级能力见 [`hidden-function-checklist.md`](./hidden-function-checklist.md)，两份清单必须同步维护。

## 2026-08-30 网站扩展与后端语言包批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| ACME 账户与自签 CA 生命周期 | `apps/workmesh-node/agent/app/api/v2/website_acme_account.go`、`website_ssl.go` | `/websites/acme/*`、`/websites/ca/*` | `node/service/website_security.go`、`node/api/website_cert_routes.go` | `POST /api/v2/websites/acme*`、`POST/GET /api/v2/websites/ca*` | Session/Cookie、Bearer、节点时间戳 | 本地 ACME/CA/签发证书元数据 | `website-acme.json`、`website-ca.json`、`website-ca-ssls.json`，原子替换 | `node/api/website_cert_routes_test.go` | `go test ./node/api -run WebsiteCertificate` | 主次节点 `/health` `/ready` 已验证 | implemented | 外部 ACME DNS 提供商需显式配置后启用 |
| 网站扩展元数据与批量操作 | `apps/workmesh-node/agent/app/api/v2/website.go`、`website_proxy.go`、`website_template.go` | `/websites/auths*`、`batch/*`、`templates/*`、`proxies*`、`exec/composer` | `node/api/website_extensions.go` | 统一 `GET/POST /api/v2/websites/{rest...}` | Session/Cookie、资源 ID 校验 | 本地模板、代理、认证、日志、数据库元数据 | `website-extensions.json`，列表最多 500 条 | `node/api/website_extensions_test.go` | `go test ./node/api -run WebsiteExtension` | 主次节点已部署并返回 200 | implemented | Composer 只校验文件，不执行任意命令 |
| 后端国际化资源 | `apps/workmesh-node/core/i18n/lang/*.yaml`、`core/i18n/i18n.go`；`apps/workmesh-node/agent/i18n/lang/*.yaml`、`agent/i18n/i18n.go` | 控制面和执行面的任务、日志、告警、认证、节点、授权消息本地化入口 | `i18n/i18n.go`、`i18n/lang/*.yaml` | `i18n.Load`、`i18n.Message`、`i18n.Format` | 进程内部调用；HTTP Accept-Language 规范化 | 12 个 UTF-8 YAML 语言包，每种 1037 个合并键 | Go embed，启动时一次解析并只读缓存 | `i18n/i18n_test.go`、`test/contract/i18n-scan.mjs` | `go test ./i18n`；`node scripts/with-dev-env.mjs -- node apps/workmesh-server/test/contract/i18n-scan.mjs --legacy apps/workmesh-node --project apps/workmesh-server` | 二进制构建验证 | implemented | 同名 Core/Agent 键按执行面语义合并；调用方使用 `Format` 传入模板参数 |

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
| 文件/数据库首批扩展 | `/api/v2/files/share/*`、`/api/v2/databases/db/update` | 分享 token 与数据库登记持久化、输入校验、分页和更新 | `go test ./node/api ./node/service` |
| 脚本资源 | `/api/v2/core/script`、`search`、`update`、`del`、`sync` | 统一资源存储提供脚本 CRUD 和同步兼容行为 | `go test ./node/api` |
| AI 执行面 | `/api/v2/ai/ollama/*`、`mcp/*`、`tensorrt/*`、`gpu/*` | 模型、MCP、TensorRT-LLM、GPU 状态和域名绑定均使用 `ai.json` 持久化；无硬件时返回可识别降级状态 | `go test ./node/api -run AI` |
| AI 账号与 Agent | `/api/v2/ai/accounts/*`、`agents/*`、`agents/channel/*`、`agents/plugins/*`、`agents/skills/*` | 账号/Agent/渠道/插件/Skill 的 CRUD、配置和搜索；敏感字段脱敏；角色和会话配置可恢复 | `go test ./node/api -run AI` |
| 应用目录 | `/api/v2/apps/search`、`detail/*`、`tags`、`checkupdate`、`services/*` | 应用目录搜索、详情、标签、更新状态和服务信息，支持 `WORKMESH_APP_CATALOG` | `go test ./node/api -run App` |
| 已安装应用 | `/api/v2/apps/install`、`installed/*`、`ignored/*` | 安装幂等、启停/重启/卸载、端口和参数更新、排序、连接信息、升级忽略均写入 `apps.json` | `go test ./node/api -run App` |
| 自定义应用商店 | `/api/v2/custom/app/*`、`/api/v2/core/xpack/sync/app/install` | 自定义商店配置、同步任务和多节点安装兼容入口 | `go test ./node/api -run App` |

## 进行中

### 2026-08-30 文件域批次（批量、分享、搜索与下载）

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 回收站状态 | `apps/workmesh-node/agent/app/api/v2/recycle_bin.go:GetRecycleStatus` | `GET /files/recycle/status` | `node/api/files_routes.go:fileAdvancedHandler` | `GET /api/v2/files/recycle/status` | 节点会话 | `files.json` 回收站条目 | `WORKMESH_DATA_DIR/files.json` | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileFavorite` | 待制品部署 | implemented | 仅返回本地回收站计数 |
| 分享校验 | `apps/workmesh-node/agent/app/api/v2/file.go:CheckFileShare` | `GET /files/share/check` | `node/api/files_routes.go:fileAdvancedHandler` | `GET /api/v2/files/share/check` | token/code 公共校验 | 分享 token 与目标文件 | `file-shares.json` | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileShare` | 待制品部署 | implemented | 支持 code/token 别名 |
| 分享信息 | `apps/workmesh-node/agent/app/api/v2/file.go:GetPublicFileShareInfo` | `GET /files/share/info` | `node/api/files_routes.go:fileAdvancedHandler` | `GET /api/v2/files/share/info` | token/code 公共查询 | 分享记录、文件 stat | `file-shares.json` | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileShare` | 待制品部署 | implemented | 文件不存在返回 exists=false |
| 分享下载 | `apps/workmesh-node/agent/app/api/v2/file.go:DownloadFileShare` | `GET /files/share/download` | `node/api/files_routes.go:fileAdvancedHandler` | `GET /api/v2/files/share/download` | token/code 公共校验 | 分享记录与文件内容 | 无状态读取 | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileShare` | 待制品部署 | implemented | 目标删除返回 404 |
| 分享二维码数据 | `apps/workmesh-node/agent/app/api/v2/file.go:GetFileShareQRCode` | `GET /files/share/qrcode` | `node/api/files_routes.go:fileAdvancedHandler` | `GET /api/v2/files/share/qrcode` | token/code 公共校验 | 分享 URL | 无状态 | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileShare` | 待制品部署 | implemented | 返回 URL 数据，前端可编码展示 |
| wget 进度查询 | `apps/workmesh-node/agent/app/api/v2/file.go:WgetProcess` | `GET /files/wget/process` | `node/api/files_routes.go:fileAdvancedHandler` | `GET /api/v2/files/wget/process` | 节点会话 | 进程内下载状态 | 进程内有界 map | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileShare` | 待制品部署 | implemented | 服务重启后任务不恢复 |
| wget 任务 key | `apps/workmesh-node/agent/app/api/v2/file.go:ProcessKeys` | `GET /files/wget/process/keys` | `node/api/files_routes.go:fileAdvancedHandler` | `GET /api/v2/files/wget/process/keys` | 节点会话 | 下载任务 map | 进程内 | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileShare` | 待制品部署 | implemented | 仅返回当前进程任务 |
| AI 文件内容搜索 | `apps/workmesh-node/agent/app/service/file.go:AISearch` | `POST /files/ai-search` | `node/api/files_routes.go:handleFileAISearch` | `POST /api/v2/files/ai-search` | 节点会话 | 目录文件内容、查询参数 | 无状态 | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileBatch` | 待制品部署 | implemented | grep 模式；LLM 摘要待独立 AI 域接入 |
| 批量文件存在检查 | `apps/workmesh-node/agent/app/service/file.go:BatchCheckFiles` | `POST /files/batch/check` | `node/api/files_routes.go:fileAdvancedHandler` | `POST /api/v2/files/batch/check` | 节点会话 | `os.Stat` | 无状态 | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileBatch` | 待制品部署 | implemented | 单次最多 500 路径 |
| 批量删除 | `apps/workmesh-node/agent/app/service/file.go:BatchDelete` | `POST /files/batch/del` | `node/api/files_routes.go:fileAdvancedHandler` | `POST /api/v2/files/batch/del` | 节点会话、路径校验 | 文件系统 | 直接删除 | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileBatch` | 待制品部署 | implemented | 每次最多 200 路径 |
| 批量权限更新 | `apps/workmesh-node/agent/app/service/file.go:BatchChangeModeAndOwner` | `POST /files/batch/role` | `node/api/files_routes.go:fileAdvancedHandler` | `POST /api/v2/files/batch/role` | 节点会话、mode 白名单 | 文件系统 chmod | 无状态 | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileBatch` | 待制品部署 | implemented | 当前跨平台仅保证 mode，owner 字段回显 |
| 单文件存在检查 | `apps/workmesh-node/agent/app/api/v2/file.go:CheckFile` | `POST /files/check` | `node/api/files_routes.go:fileAdvancedHandler` | `POST /api/v2/files/check` | 节点会话 | `os.Stat` / Mkdir | 文件系统副作用（withInit） | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileBatch` | 待制品部署 | implemented | withInit 只创建目录 |
| 收藏分页查询 | `apps/workmesh-node/agent/app/api/v2/favorite.go:SearchFavorite` | `POST /files/favorite/search` | `node/api/files_routes.go:fileAdvancedHandler` | `POST /api/v2/files/favorite/search` | 节点会话 | `files.json` favorites | `WORKMESH_DATA_DIR/files.json` | `node/api/files_routes_test.go` | `go test ./node/api -run TestFileFavorite` | 待制品部署 | implemented | page/pageSize 上限 200 |

实现扫描器当前结果以 [`function-checklist-generated.md`](./function-checklist-generated.md) 和 `.tmp/implementation-status.json` 为准（基于 863 条路由）。隐藏初始化文件中的路由也已纳入去重统计。`partial` 与 `compatibility` 仍需按真实副作用逐项验收，不得仅凭路由注册宣称完成。

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
node scripts/with-dev-env.mjs -- node test/contract/implementation-scan.mjs --legacy apps/workmesh-node --project apps/workmesh-server --out .tmp/implementation-status.json
node scripts/with-dev-env.mjs -- node test/contract/hidden-function-scan.mjs --legacy apps/workmesh-node --project apps/workmesh-server --out .tmp/hidden-function-status.json
```

## 2026-08-30 仪表盘采集批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 仪表盘主机资源采集 | `apps/workmesh-node/agent/api/v2/dashboard.go` | `/api/v2/dashboard/base/*`、`/api/v2/dashboard/current/*` | `apps/workmesh-server/node/api/dashboard.go` | `GET /api/v2/dashboard/base/{ioOption}/{netOption}`、`GET /api/v2/dashboard/current/{ioOption}/{netOption}` | 节点会话鉴权（由上层中间件执行） | `/proc/loadavg`、`/proc/meminfo`、`/proc/net/dev`、`/proc/mounts`、运行时信息 | 无状态实时采集 | `node/api/dashboard_test.go` | `go test ./node/api -run Dashboard` | 待下一批制品部署 | implemented | Windows 无 `/proc` 时返回 supported=false，GPU/NPU/XPU 需驱动适配 |

## 2026-08-30 节点部署入口

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 前端添加部署节点 | `apps/workmesh-node/agent/views/setting/node` | 节点管理页面 | `web/src/views/advanced/multi-node/index.vue`、`web/src/api/modules/setting.ts` | `POST /api/v2/core/nodes/add`、`POST /api/v2/core/nodes/list` | 登录会话、CSRF（前端请求拦截器注入） | 表单节点 ID、名称、HTTP(S) 地址、角色 | `WORKMESH_DATA_DIR/nodes.json` 原子写入 | `control/api/link_test.go`、生产构建 | `npm.cmd run type-check`、`npm.cmd run build:pro`、`go test ./control/api` | 公网主节点添加/列表/删除验收通过；新前端待授权部署 | implemented | 云端 Gateway 注册仍需真实凭据；添加动作不代替云端授权 |

## 2026-08-30 工具箱主机信息批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 工具箱系统用户、时区、Fail2ban、FTP 状态 | `apps/workmesh-node/agent/api/v2/toolbox.go` | `/api/v2/toolbox/*` | `node/api/runtime_toolbox.go:toolboxGetData` | `GET /api/v2/toolbox/device/users`、`GET /api/v2/toolbox/device/zone/options`、`GET /api/v2/toolbox/fail2ban/base`、`GET /api/v2/toolbox/fail2ban/load/conf`、`GET /api/v2/toolbox/ftp/base` | 节点会话鉴权（由上层中间件执行） | `/etc/passwd`、`/etc/fail2ban/jail.local`、`time.Local`、`runtime.json` | FTP 配置写入 `WORKMESH_DATA_DIR/runtime.json` | `node/api/runtime_toolbox_test.go` | `go test ./node/api -run Toolbox`、`go vet ./...` | 待下一批制品部署 | implemented | 非 Linux 主机无系统配置文件时返回 supported/installed 状态，不执行外部命令 |

## 2026-08-30 告警发现批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 告警磁盘列表 | `apps/workmesh-node/agent/app/service/alert.go:GetDisks` | `GET /api/v2/alert/disks/list` | `node/api/functional_domains.go:listAlertDisks` | `GET /api/v2/alert/disks/list` | 节点会话鉴权（上层中间件） | `/proc/mounts` 挂载点清单；跨平台容量字段显式标记 `capacitySupported` | 无状态采集 | `node/api/functional_domains_test.go:TestAlertDiskAndClamDiscovery` | `go test ./node/api -run AlertDisk` | 本地 HTTP 单测 | implemented | Windows 无 `/proc` 时返回空集合，Linux 容量采集可由专用采集器扩展 |
| ClamAV 服务发现 | `apps/workmesh-node/agent/app/service/alert.go:GetClams` | `GET /api/v2/alert/clams/list` | `node/api/functional_domains.go:detectClamServices` | `GET /api/v2/alert/clams/list` | 节点会话鉴权（上层中间件） | PATH 中 `clamdscan`、`freshclam` 可执行文件探测 | 无状态采集 | `node/api/functional_domains_test.go:TestAlertDiskAndClamDiscovery` | `go test ./node/api -run AlertDisk` | 本地 HTTP 单测 | implemented | 未引入 ClamAV 管理 SDK，服务启停仍由运行时工具域负责 |
| 告警配置与系统计划任务搜索 | `apps/workmesh-node/agent/app/api/v2/alert.go` | `POST /api/v2/alert/config/search`、`POST /api/v2/alert/cronjob/list` | `node/api/functional_domains.go` | 同左 | 节点会话鉴权（上层中间件） | `domains.json` 告警配置、`/etc/cron.*` 目录 | 配置沿用 `domains.json` 原子写入；cron 只读 | `node/api/functional_domains_test.go:TestAlertConfigSearchAndCronListAreDynamic` | `go test ./node/api -run AlertConfig` | 本地 HTTP 单测 | implemented | 未接入系统级 cron 编辑和执行，写操作仍由计划任务域负责 |

## 2026-08-30 AI 账户与沙盒批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| AI 提供商目录与账户分页 | `apps/workmesh-node/agent/app/provider/catalog.go`、`agent/app/service/agents.go` | `GET /api/v2/ai/accounts/providers`、`POST /api/v2/ai/accounts`、`POST /api/v2/ai/accounts/update`、`POST /api/v2/ai/accounts/search`、`POST /api/v2/ai/accounts/counts`、`POST /api/v2/ai/accounts/delete` | `node/api/ai_execution.go:aiProviders`、`handleAccountRoute` | 同左 | 上层 Session/Bearer；账户写入需节点权限 | 内置提供商元数据和请求体 | `.workmesh-data/ai.json` 原子 rename | `node/api/ai_execution_test.go:TestAIAccountModelsAndValidation` | `go test ./node/api -run AIAccount` | 待下一批制品部署 | implemented | 真实 Gateway 账户同步待凭据接入 |
| 账户模型管理与远程发现 | `apps/workmesh-node/agent/app/service/agents.go:1201-1345` | `POST /api/v2/ai/accounts/models`、`models/create`、`models/update`、`models/delete`、`models/discover`、`verify` | `node/api/ai_execution.go:handleAccountRoute`、`discoverAIModels` | 同左 | Session/Bearer；API Key 仅用于上游请求且响应脱敏 | 账户持久化模型或上游 `/models` JSON | `ai.json` 原子 rename；HTTP 8 秒超时 | `node/api/ai_execution_test.go:TestAIAccountModelsAndValidation`、`TestAIAccountModelDiscoveryAndSandboxPersistence` | `go test ./node/api -run 'AIAccount|AI.*Discovery'` | 待下一批制品部署 | 不同厂商专用鉴权协议需按 provider 扩展 |
| GPU 能力探测 | `apps/workmesh-node/agent/api/v2/monitor.go` | `GET /api/v2/ai/gpu/load`、`GET /api/v2/ai/gpu/options`、`POST /api/v2/ai/gpu/search` | `node/api/ai_execution.go:detectGPU` | 同左 | Session/Bearer | `nvidia-smi` 设备信息和 Go 运行时内存 | 无状态采集 | `node/api/ai_execution_test.go`（CPU 降级路径） | `go test ./node/api -run AI` | 待下一批制品部署 | AMD/NPU/XPU 驱动适配待补充 |
| CubeSandbox 生命周期与状态 | `apps/workmesh-node/agent/router/ro_cubesandbox.go`、`agent/app/api/v2/workmesh_task.go` | `GET /api/v2/cubesandbox/health`、`GET /api/v2/cubesandbox/status`、`POST /api/v2/cubesandbox/start`、`stop`、`reconcile` | `node/api/ai_execution.go:sandboxHandler` | 同左 | Session/Bearer；启动受 KVM 能力限制 | `/dev/kvm` 能力探测与实例状态 | `ai.json` Sandboxes 数组原子 rename | `node/api/ai_execution_test.go:TestAIAccountModelDiscoveryAndSandboxPersistence` | `go test ./node/api -run Sandbox` | 待下一批制品部署 | MicroVM 实际进程编排和镜像校验待接入 |
## 2026-08-30 任务隔离执行批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 任务沙盒创建 | `apps/workmesh-node/agent/app/api/v2/workmesh_task.go:CreateWorkMeshTask`、`agent/utils/cubesandbox/task.go` | `POST /api/v2/workmesh/tasks/create` | `node/api/ai_execution.go:taskHandler`、`node/service/taskruntime/taskruntime.go:TaskProvider.Create` | `POST /api/v2/workmesh/tasks/create` | 节点 Token（配置 `WORKMESH_TASK_TOKEN`）；Provider CLI 摘要 | 镜像 sha256、工作区、入口、隔离策略 | `ai.json` tasks 原子 rename；沙盒由受控 CLI 持久化 | `node/api/ai_execution_test.go:TestWorkMeshTaskRoutesUseIsolatedProvider`、`node/service/taskruntime/taskruntime_test.go` | `go test ./node/api ./node/service/taskruntime` | 真实 CLI 配置后验收 | implemented | 需要部署环境配置已签名任务 CLI |
| 任务沙盒启动、取消、销毁 | `apps/workmesh-node/agent/app/api/v2/workmesh_task.go:StartWorkMeshTask/CancelWorkMeshTask/DestroyWorkMeshTask` | `POST /api/v2/workmesh/tasks/start`、`cancel`、`destroy` | `node/api/ai_execution.go:taskHandler`、`node/service/taskruntime/taskruntime.go:transition` | 同左 | 节点 Token；任务状态机校验 | Provider 沙盒句柄与生命周期 | `ai.json` tasks 状态原子更新 | 同上 | 同上 | 真实 CLI 配置后验收 | implemented | 服务重启后需重新关联 Provider 句柄 |
| 任务沙盒执行与结果收集 | `apps/workmesh-node/agent/app/api/v2/workmesh_task.go:ExecWorkMeshTask/CollectWorkMeshTask` | `POST /api/v2/workmesh/tasks/exec`、`collect` | `node/api/ai_execution.go:taskHandler`、`node/service/taskruntime/taskruntime.go:Exec/Collect` | 同左 | 节点 Token；仅允许 argv，不允许宿主 Shell | Provider CLI 返回 exitCode/stdout/stderr | 任务状态和结果由沙盒后端保存；状态摘要写入 `ai.json` | 同上 | 同上 | 真实 CLI 配置后验收 | implemented | 结果历史分页和断点流式输出待在线开发域接入 |

## 2026-08-30 网站高级操作批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 网站运行状态与可用性检查 | `apps/workmesh-node/agent/router/ro_website.go` | `POST /api/v2/websites/operate`、`POST /api/v2/websites/check` | `node/api/website.go`、`node/service/website.go` | `POST /api/v2/websites/operate`、`POST /api/v2/websites/check` | 节点会话鉴权 | WebsiteService 网站元数据 | `websites.json` 原子写入 | `node/api/website_test.go`、`node/service/website_test.go` | `go test ./node/api ./node/service -run Website` | 本地 HTTP 已验证 | implemented | 未接入 OpenResty 进程重载 |
| 网站类型和站点选项 | `apps/workmesh-node/agent/router/ro_website.go` | `POST /api/v2/websites/options` | `node/api/website.go` | `POST /api/v2/websites/options` | 节点会话鉴权 | 网站列表与内置类型 | 无状态派生 | `node/api/website_test.go` | `go test ./node/api -run WebsiteAdvanced` | 本地 HTTP 已验证 | implemented | 类型选项待按运行时扩展 |
| 网站域名增删改查 | `apps/workmesh-node/agent/router/ro_website.go` | `GET /api/v2/websites/domains/:websiteId`、`POST /api/v2/websites/domains*` | `node/api/website.go`、`node/service/website.go` | 同旧路由 | 节点会话鉴权、网站归属校验 | WebsiteDomain | `website-domains.json` 原子写入 | `node/api/website_test.go`、`node/service/website_test.go` | `go test ./node/api ./node/service -run 'WebsiteAdvanced|Domain'` | 本地 HTTP 已验证 | implemented | 未执行 DNS 自动解析 |
| Nginx、重写、目录、跳转、防盗链配置 | `apps/workmesh-node/agent/router/ro_website.go` | `/websites/{config,rewrite,dir,redirect,leech}*` | `node/api/website.go`、`node/service/website.go` | 11 个 POST 配置接口及 `GET /websites/rewrite/custom` | 节点会话鉴权、网站归属校验 | 请求配置对象 | `website-configs.json` 原子写入 | `node/api/website_test.go` | `go test ./node/api -run WebsiteAdvanced` | 本地 HTTP 已验证 | implemented | 未调用 OpenResty reload |
| 网站 HTTPS 配置 | `apps/workmesh-node/agent/router/ro_website.go` | `GET/POST /api/v2/websites/:id/https` | `node/api/website.go`、`node/service/website.go` | `GET/POST /api/v2/websites/:id/https` | 节点会话鉴权、网站归属校验 | enabled/operate 参数 | `website-configs.json` 原子写入 | `node/api/website_test.go` | `go test ./node/api -run WebsiteAdvanced` | 本地 HTTP 已验证 | implemented | 证书签发由 SSL 域负责 |

## 2026-08-30 应用目录详情批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 应用详情与运行服务 | `apps/workmesh-node/agent/app/api/v2/app.go:GetApp*` | `GET /api/v2/apps/:key`、`GET /api/v2/apps/detail/*`、`GET /api/v2/apps/services/:key` | `node/api/apps.go:appCatalogGet` | 同左 | 节点会话鉴权（上层中间件） | `apps.json` catalog/apps 记录及配置中的 services/params | `WORKMESH_DATA_DIR/apps.json` 原子写入 | `node/api/apps_test.go:TestAppDerivedDetailsAndDeleteCheck` | `go test ./node/api -run App` | 本地 HTTP 已验证 | implemented | 未接入远程应用商店 SDK |
| 已安装应用信息与删除检查 | `apps/workmesh-node/agent/app/api/v2/app.go` | `GET /api/v2/apps/installed/info/:appInstallId`、`GET /api/v2/apps/installed/params/:appInstallId`、`GET /api/v2/apps/installed/delete/check/:appInstallId` | `node/api/apps.go:appInstalledGet` | 同左 | 节点会话鉴权（上层中间件） | 安装记录、容器名称和参数 | `apps.json` 原子写入 | `node/api/apps_test.go:TestAppDerivedDetailsAndDeleteCheck` | `go test ./node/api -run AppDerived` | 本地 HTTP 已验证 | implemented | 容器资源删除仍需容器域执行 |
| 应用版本更新查询 | `apps/workmesh-node/agent/app/api/v2/app.go` | `POST /api/v2/apps/installed/update/versions` | `node/api/apps.go:handleAppPost` | `POST /api/v2/apps/installed/update/versions` | 节点会话鉴权（上层中间件） | catalog 中匹配应用的版本记录 | 无额外写入 | `node/api/apps_test.go` | `go test ./node/api -run App` | 本地 HTTP 已验证 | implemented | 远程版本同步依赖 Gateway 配置 |
