<!-- SPDX-License-Identifier: GPL-3.0-only -->

## 2026-08-31 别名与文件系统隐藏能力

| 隐藏能力 | 发现位置 | 实现位置 | 状态 | 说明 |
|---|---|---|---|---|
| xpack 监控/WAF 别名方法路由 | `apps/workmesh-node/agent/router/ro_website.go` | `node/api/website.go` | implemented | 修复 ServeMux 方法模式拼接，别名进入真实 analytics/WAF 处理器 |
| 网站资源与负载均衡查询 | `apps/workmesh-node/agent/app/service/website.go` | `node/api/website.go`、`website_extensions.go` | implemented | 读取网站配置、域名并返回资源列表 |
| 文件 owner、挂载点、用户组查询 | `apps/workmesh-node/agent/app/api/v2/file.go` | `node/api/files_routes.go` | implemented | Linux 使用 os/user 与 Chown，Windows 返回明确不支持 |
| 媒体文件转换任务 | `apps/workmesh-node/agent/app/service/file.go:Convert` | `node/api/files_routes.go` | implemented | 通过受控 `WORKMESH_MEDIA_CONVERTER` 执行并设置 5 分钟超时；输出原子替换，日志持久化并支持分页筛选 |
<!-- Copyright (c) 2026 WorkMesh contributors -->

## 2026-08-31 首页配置状态持久化

| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [x] | 快速跳转数组与应用启动器显示状态持久化 | `apps/workmesh-node/agent/app/service/dashboard.go:ChangeQuick`、`ChangeShow`、`ListLauncherOption` | `node/api/dashboard.go:handleDashboardMutation` 将配置写入 `domains.json`，`dashboardQuickJumps` 在请求和重启后恢复，启动器选项保留隐藏项并返回 `isShow` | 至少一个快速入口可见、最多四个可见；非法 key/status/JSON 拒绝；持久化重载测试通过 |

## 2026-08-31 网站监控访问日志采集

| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [x] | Nginx/OpenResty combined access.log 增量统计 | `apps/workmesh-node/agent/app/service/website_monitor.go:collectWebsiteLog`、`Stat`、`QPS`、`Rank` | `node/api/analytics.go:loadAnalyticsEvents`、`analyticsDaily`、`analyticsRank`；路径可配置且单次限制 8 MiB/50000 条 | 无日志返回真实空结果；请求时间范围、状态码、流量、UV、排行均由日志计算；样例日志测试通过 |

## 2026-08-30 核心认证与执行入口
| 状态 | 隐藏能力 | 旧源码证据 | 新实现证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [x] | Passkey 注册挑战、凭据列表与删除 | `apps/workmesh-node/core/app/service/auth.go` | `control/service/core.go`、`node/api/core_handlers.go`，挑战 5 分钟过期并持久化元数据 | 需要会话鉴权、重复凭据拒绝和重启后加载 |
| [x] | 脚本库运行入口 | `apps/workmesh-node/core/app/api/v2/script_library.go:RunScript` | `node/api/core_resources.go:handleScriptRun`，仅接受已登记 `script_id` 且有令牌 | 未配置令牌或脚本时明确错误，禁止任意命令 |
| [x] | 进程 PID 详情采集 | `apps/workmesh-node/agent/app/service/process.go` | `node/api/process.go:readProcessDetails`，读取 procfs 内存和用户 | PID 校验、资源不存在 404、平台降级 |

## 2026-08-30 命令执行入口审计

| 状态 | 隐藏能力 | 旧源码证据 | 新实现证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [x] | 命令模板和脚本库受控执行 | `apps/workmesh-node/core/app/api/v2/script_library.go`、`agent/app/service/command.go` | `node/api/core_resources.go`、`node/service/taskruntime`；脚本 ID 白名单、令牌校验、超时和输出上限 | 禁止任意命令，外部进程失败可观测，长任务可查询 |

## 2026-08-30 日志后台能力
| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [x] | system 日志文件枚举与服务探测 | `apps/workmesh-node/agent/app/service/logs.go` | `node/api/functional_domains.go:listSystemLogFiles/listRunningSystemServices`，外部命令 5 秒超时 | 日志目录和 systemctl/tasklist 均有明确降级 |
| [x] | 任务日志分页与路径安全 | `apps/workmesh-node/agent/app/service/task.go:ReadByLine` | `node/api/functional_domains.go:readTaskLog`，路径白名单、单页 500 行 | 正常读取、分页、越权 403 测试通过 |
| [x] | 执行中任务计数 | `apps/workmesh-node/agent/app/service/task.go:CountExecutingTask` | `node/api/functional_domains.go:registerLogRoutes`，从持久化日志状态统计 | 写入 running/executing 后计数准确 |

## 2026-08-30 运行时详情能力
| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [x] | PHP 运行时扩展、配置及 FPM 状态查询 | `apps/workmesh-node/agent/app/service/runtime.go` | `node/api/runtime_toolbox.go:registerRuntimeSubroutes` 从 runtime.json 返回记录 | 创建运行时后详情可查询，未知 ID 返回 404 |
| [x] | Supervisor 进程配置查询 | `apps/workmesh-node/agent/app/service/runtime.go` | `node/api/runtime_toolbox.go:registerRuntimeSubroutes` 读取 supervisor 配置 | 未配置进程返回 not_configured，不伪造运行状态 |

# 隐藏功能迁移清单

## 2026-08-30 仪表盘采集

| 功能名称 | 旧源码入口 | 新源码入口 | 数据来源 | 测试 | 状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- |
| 主机网络与挂载点采集 | `apps/workmesh-node/agent/api/v2/dashboard.go` | `node/api/dashboard.go:dashboardNetwork`、`dashboardDisks` | `/proc/net/dev`、`/proc/mounts` | `node/api/dashboard_test.go` | implemented | 非 Linux 环境无内核接口时返回 supported=false；硬件加速器需驱动适配 |
| CPU、内存、交换区与块设备 I/O 采集 | `apps/workmesh-node/agent/api/v2/dashboard.go`、`agent/app/service/system.go` | `node/api/dashboard.go:dashboardCPUInfo`、`dashboardIO`、`dashboardCurrent` | `/proc/stat`、`/proc/meminfo`、`/proc/diskstats` | `node/api/dashboard_test.go` | implemented | Linux 提供累计 CPU/内存/I/O 指标；无 procfs 平台返回稳定零值和非空数组，避免前端 NaN |
| 挂载点容量与系统识别信息 | `apps/workmesh-node/agent/app/service/system.go` | `node/api/dashboard.go:handleDashboardOS`、`handleDashboardBase` | `node/api/dashboard_disk_unix.go`、`dashboard_disk_windows.go`、`/etc/os-release`、`net.Interfaces` | `node/api/dashboard_test.go` | implemented | Unix 使用 statfs，Windows 明确标记容量 unavailable；硬件加速器待驱动适配 |

本清单覆盖旧 `apps/workmesh-node/core` 与 `agent` 中不一定表现为 HTTP 路由的能力。当前路由清单为 871 条（包含 helper 注册、公共备份账号空路径和去品牌化静态入口）；本文件用于防止初始化钩子、后台作业、中间件和协议升级能力在迁移时遗漏。

状态定义：

- `[x]` 已有新实现，并有自动化或部署证据。
- `[~]` 已有边界或兼容实现，但仍缺少旧系统等价副作用，不能宣称完成。
- `[ ]` 尚未完成，必须在发布前补齐。

## 运行时初始化与生命周期

## 2026-08-30 文件域运行时能力

| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [x] | wget 下载任务上下文、超时和取消 | `apps/workmesh-node/agent/app/service/file.go:Wget`、`StopWget` | `node/api/files_routes.go:handleFileWget` 使用 30 分钟 context、临时文件和原子 rename | process/keys 可查询，stop 可取消，失败状态可见 |
| [x] | AI 文件内容搜索扫描限制 | `apps/workmesh-node/agent/app/service/file.go:AISearch`、`utils/files/ai_content_search.go` | `node/api/files_routes.go:handleFileAISearch` 限制 500 文件、500 命中、8 MiB 单文件 | 参数错误、目录不存在、正则错误均返回结构化错误 |
| [x] | 批量文件操作路径校验 | `apps/workmesh-node/agent/app/service/file.go:BatchDelete/BatchCheckFiles/BatchChangeModeAndOwner` | `node/api/files_routes.go:fileAdvancedHandler` 每路径 clean/stat，数量和 mode 有上限 | 非法路径不执行，部分失败逐项返回 |

| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [x] | 单进程 HTTP 服务、静态资源和优雅停机 | `core/init`、`agent/init` | `cmd/workmesh-server/main.go`、`runtime/http/server.go`；`go test ./...` | SIGTERM 在超时内停止监听并刷新状态 |
| [x] | 健康与就绪探针 | `core/init/router` | `/health` 不访问依赖，`/ready` 可检查依赖；双节点 HTTP 200 | 探针响应保持 200/ERR envelope 契约 |
| [x] | 节点角色与 epoch/fencing | `core/utils/xpack/providers/multi_node.go`、`agent/utils/xpack/providers/multi_node.go` | `runtime/role`、`runtime/link`、`control/api/role.go`；链路和重启恢复测试 | 旧 epoch 请求返回 409，角色状态通过 `role-state.json` 原子保存并在重启后恢复，禁止双主写入 |
| [x] | 启动配置、数据目录和资源限制 | `core/global/config.go`、`agent/global/config.go` | `config/config.go`、`cmd/workmesh-server/cli.go:initializeDataDir`；数据目录和 releases/apps/backups/logs/runtime/uploads 子目录启动时创建，制品读取上限 512 MiB | 本地初始化、文件类型和上限测试通过；生产硬件资源 E2E 仍需部署验证 |

## 安全中间件与授权

| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [x] | Session/Cookie/Bearer/API Key 认证 | `core/middleware/session.go` | `control/api/security_middleware.go`、`control/service/core.go`、`node/api/core_handlers.go`；认证 HTTP 测试 | 主进程已统一挂载，控制面与节点执行面均拒绝未授权请求 |
| [x] | CSRF 防护 | `core/middleware/csrf_protect.go` | `control/api/security_middleware.go`；双提交 token、Origin、Sec-Fetch-Site 测试 | Cookie 会话写请求要求 `pcsrftoken` 与 `X-CSRF-Token`，跨站请求拒绝 |
| [~] | Demo/只读模式限制 | `core/middleware/demo_handle.go` | 新服务暂以角色权限承接 | 覆盖旧白名单并验证所有写接口被拒绝 |
| [x] | 域名绑定与密码过期 | `core/middleware/bind_domain.go`、`password_expired.go` | `control/api/security_middleware.go`；domains.json 与环境变量覆盖 | 绑定域名、密码过期 313、重置例外均有测试 |
| [x] | 敏感字段脱敏与请求体上限 | 旧 controller/service 约束 | AI、Gateway、兼容入口均限制 2 MiB 并脱敏 | 安全扫描不得出现明文 secret |

## 国际化、错误和任务基础设施

### 2026-08-30 双服务语言资源完整性

| 状态 | 隐藏能力 | 旧源码证据 | 新实现证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [x] | Core 与 Agent 语言键集合合并 | `apps/workmesh-node/core/i18n/lang/*.yaml`、`apps/workmesh-node/agent/i18n/lang/*.yaml` | `i18n/lang/*.yaml` 每种语言 1037 个键；`i18n/i18n.go` 启动一次解析并缓存 | 12 种语言键集合一致，模板占位符一致，未知语言回退中文 |
| [x] | 前端语言选择入口 | `apps/workmesh-node/frontend/src/lang`、登录/设置/分享入口 | `web/src/lang`、`App.vue`、登录页两个菜单、设置页、分享页 | 12 种语言模块键结构一致且可从每个入口选择 |

| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [~] | 服务端 i18n/localizer | `core/i18n`、`agent/i18n` | 前端 `vue-i18n`；Go 错误仍有中文固定文本 | 提供 zh/en 资源加载、按请求语言返回错误 |
| [~] | 业务错误、多错误聚合和错误码 | `core/buserr/*.go` | `runtime/http` ERR envelope | 完成错误码目录和逐域映射测试 |

### 2026-08-31 通用错误语言协商与安全拒绝

| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [x] | Accept-Language 语言协商基础设施 | `core/i18n`、`agent/i18n` | `i18n.LocaleFromRequest`、`runtime/http.WithLocale`，普通 JSON 响应按请求语言渲染，未知语言回退中文 | zh/en 请求测试通过，流式和 WebSocket 不被包装器破坏 |
| [~] | 通用错误 envelope 与稳定错误码 | `core/buserr/*.go`、`core/middleware/*.go` | `i18n.ErrorCode/LocalizeError`、`runtime/http.JSON` 自动补充 `details.errCode`；控制面和节点面 `writeError` 已接入 | 基础安全错误已覆盖；业务域仍需逐项迁移专用语言键和多错误聚合 |
| [x] | Session/Bearer/API Key/CSRF/域名/密码过期拒绝文案 | `core/middleware/session.go`、`csrf_protect.go`、`bind_domain.go`、`password_expired.go` | `control/api/security_middleware.go` 统一执行，错误按 Accept-Language 本地化；英文未授权、CSRF、域名和过期测试通过 | 未授权请求不得伪造成功，Cookie 会话写请求仍需 CSRF 双提交 |
| [x] | 异步任务、取消、重试、超时和日志 | `core/app/task/task.go`、`agent/global/global.go` | `node/service/cronjob.go`、任务 API；Go 单测 | 取消请求可终止执行，重启后记录可恢复 |
| [x] | 任务隔离 Provider 生命周期与 CLI 白名单 | `apps/workmesh-node/agent/app/api/v2/workmesh_task.go`、`agent/utils/cubesandbox/task.go`、`forgevm_task_backend.go` | `node/service/taskruntime/taskruntime.go`、`node/api/ai_execution.go:taskHandler`；固定 sha256 CLI、argv 校验、状态转换和 30 分钟超时 | 未配置真实 CLI 时返回明确 503；配置摘要后 create/start/exec/collect/cancel/destroy 均调用受控 Provider，禁止宿主 Shell |
| [x] | 任务日志滚动与清理 | `core/log`、`agent/log` | `runtime/log/logger.go`、`runtime/log/logger_test.go` | 按大小轮转并保留 5 个历史文件，写入线程安全，关闭时刷新 |

## 后台作业与数据维护

| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [x] | Cron 调度器与启停恢复 | `agent/cron/cron.go` | `node/service/cronjob.go`、`node/api/host_container_cron.go` | 从持久化状态恢复启用任务，按五字段 Spec 每分钟执行并支持停止 |
| [x] | 网站/SSL 定时作业 | `agent/cron/job/website.go`、`ssl.go` | `node/api/host_container_cron.go:StartBackgroundTasks` 每小时扫描并续期即将到期的本地 self-signed 证书；`node/service/website_security.go:RenewDueCertificates` 原子持久化并记录失败 | `node/service/ssl_test.go:TestWebsiteSecurityRenewsDueSelfSignedCertificate`；ACME 云端挑战仍需显式授权 API |
| [~] | 备份账号 token 刷新 | `agent/cron/job/backup.go` | 备份 API 可记录和恢复；云账号刷新待接入 | OneDrive/阿里云 token 刷新及失败告警 |
| [x] | 状态文件原子写入和恢复 | 旧 DB 初始化/迁移钩子 | `node/api` 各域 JSON store 使用临时文件+rename | 并发写入和断电恢复测试通过 |

## 协议与执行通道

| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [~] | 本地/SSH/容器终端 WebSocket | `core/middleware/demo_handle.go`、Agent terminal routers | `node/api/process.go`、`deployment_runtime.go` | 完成真实双向帧、关闭码和权限测试 |
| [~] | SSE/流式任务输出 | Agent 执行与日志路由 | Gateway/link 与任务 API 边界已建 | 增加断线续传、心跳和背压测试 |
| [x] | 节点 handshake/heartbeat/sync | xpack multi-node provider | `runtime/link`；HMAC、timestamp、nonce、防重放测试 | 双节点公网链路和 fencing 验收 |
| [~] | Gateway 登录、注册、心跳和解绑 | WorkMesh gateway router | `runtime/gateway/http_client.go`、`control/api/gateway.go` | 前端绑定字段、Bearer 注册、Ed25519/HMAC 心跳、解绑和地址持久化均有真实处理；云端未提供 authorization refresh/revoke 标准端点时刷新只能返回明确错误 |

## 路由扫描盲区与隐藏注册

`route-scan.mjs` 已展开 helper 调用中的 Group 前缀，并将旧品牌 Swagger 路径映射为 `/swagger/*any`；Gin 的 `StaticFS` 仍需单独验收以下静态行为：

| 状态 | 隐藏注册 | 旧源码证据 | 当前风险与完成条件 |
| --- | --- | --- | --- |
| [~] | `xpack/monitor` 监控别名 15 条：`GET /api/v2/xpack/monitor/status`、`POST /api/v2/xpack/monitor/{stat,visitors,visitors/loc,qps,rank,trend,logs/search,logs/stat,logs/detail,logs/clear,websites,config/global,config/site,config/site/update}` | `apps/workmesh-node/agent/router/ro_website.go:130-154`；新 `node/api/website.go` 已显式注册 | 当前由兼容处理器防止 404；应与 `/api/v2/websites/monitor/*` 使用同一真实监控服务并增加 E2E。 |
| [~] | `xpack/waf` WAF 别名 15 条：`GET /api/v2/xpack/waf/{status,standard-rules,sites,sites/:id/rules,access-lists}`、`POST /api/v2/xpack/waf/{test,global,sites,rules,rules/delete,attack/stat,log/search,block/search,relation/stat,access-lists}` | `apps/workmesh-node/agent/router/ro_website.go:131,157-172`；新 `node/api/website.go` 已显式注册 | 当前由兼容处理器防止 404；应绑定 `WebsiteService` 的 WAF 存储并覆盖读写测试。 |
| [~] | Swagger 文档 `GET /swagger/*any` | `apps/workmesh-node/core/init/router/router.go:77-79`；新服务 `/swagger/{any...}` | 已提供 `/swagger/*any` JSON 入口；尚未接入文档文件和 SessionAuth，生产发布前必须补齐鉴权。 |
| [~] | 静态文件 `GET/HEAD /public/*filepath`、`GET/HEAD /favicon.ico/*filepath`、`GET/HEAD /assets/*filepath` | `apps/workmesh-node/core/init/router/router.go:25-39` | 新服务仅显式托管 `/assets/{filepath...}`、`/api/v2/images/*`、`/api/v2/static/*`；需验证 favicon/public 和 HEAD 响应的 MIME、缓存及路径穿越策略。 |
| [~] | 动态安全入口 `GET /{securityEntrance}` 与根页面安全检查 | `apps/workmesh-node/core/init/router/router.go:43-63` | 新服务根 Handler 对任意路径直接提供 SPA，未复刻 security entrance、Cookie 设置和安全检查；需在认证 E2E 中验证未授权访问行为。 |

实现状态扫描结果见逐路由清单，当前基线为 870 条路径；不能替代本节隐藏注册验收。特别关注以下固定/降级响应：`POST /api/v2/ai/agents/agent/list`、`POST /api/v2/ai/agents/agent/channels`、`POST /api/v2/ai/agents/overview`、GPU 无硬件时的空设备列表、`GET /api/v2/process/:pid`、文件回收站/收藏/上传查询、PHP/Node 运行时详情和工具箱配置。这些路径虽有处理器，仍需真实副作用或明确的能力不可用契约后才能将 `[~]` 改为 `[x]`。

## 迁移验收规则

### 备份域逐功能验收（2026-08-30）

| 状态 | 功能 | 接口/代码 | 验证证据 |
| --- | --- | --- | --- |
| [x] | 备份账号创建、更新、删除和脱敏列表 | `node/api/functional_domains.go:registerBackupRoutes`、`handleBackupAccount*` | `go test ./node/api -run TestBackupAccountAndRecordLifecycle` |
| [x] | 本地目录、账号选项和账号占用检查 | `GET /api/v2/backups/local`、`GET /api/v2/backups/options`、`GET /api/v2/backups/check/{name}` | `TestBackupUploadAndConnectionChecks`、JSON 状态文件复读 |
| [x] | 备份任务记录创建、分页搜索、Cronjob 过滤、批量删除和描述更新 | `handleBackupCreateRecord`、`handleBackupRecordSearch`、`handleBackupRecordDelete` | `TestBackupAccountAndRecordLifecycle` |
| [x] | 记录大小、下载路径和受控文件清单 | `handleBackupRecordSize`、`handleBackupRecordDownload`、`handleBackupFiles` | 记录源文件复制后大小与路径断言 |
| [x] | 本地文件上传、恢复和上传后恢复 | `handleBackupUpload`、`handleBackupRecover` | `TestBackupUploadAndConnectionChecks`、`TestBackupAccountAndRecordLifecycle` |
| [x] | 备份连接检查和本地 Bucket 查询 | `handleBackupConnCheck`、`handleBackupBuckets` | 本地目录真实读取；未配置云端时返回 `BACKUP_PROVIDER_UNAVAILABLE`，不伪造空列表 |
| [x] | 云端 OAuth token 刷新和远端 Bucket 操作 | `handleBackupRefreshTokenV2`、`handleBackupBucketsV2`、`node/service/backup_provider.go` | 显式端点执行 OAuth refresh_token、Bucket 查询、multipart 上传和 JSON 删除；Bearer/API Key 脱敏、15 秒/30 分钟超时、3 次有限重试；账号 Vars 原子持久化 | `node/service/backup_provider_test.go`、`node/api/functional_domains_test.go:TestBackupCloudUploadAndDeleteUseProvider`；未配置端点返回明确 503 |
| [x] | SSL 自动续期失败重试与状态报告 | `apps/workmesh-node/agent/cron/job/website.go`、`ssl.go` | `node/service/website_security.go:RenewDueCertificates` 对到期 self-signed 证书执行最多 3 次指数退避，记录 `Retries` 和失败上下文；无 ACME 凭据不伪造成功 | `node/service/ssl_test.go:TestWebsiteSecurityRenewRetriesAndReportsFailure` |

### 2026-08-30 隐藏路由批次

- [x] `GET /api/v2/core/script/run`：受 `WORKMESH_COMMAND_TOKEN` 保护，支持 30 秒超时、退出码和输出回传；未配置令牌时明确返回 `COMMAND_AUTH_REQUIRED`。
- [x] `GET /api/v2/process/ws`：实现 RFC6455 文本帧长度编码，支持超过 125 字节的进程快照，连接断开后释放 ticker 和 socket。
- [~] 网站统计与 WAF 统计接口：统一由 `analyticsHandler` 返回契约化数据并持久化监控配置；真实访问日志采集器尚未接入，统计数值不能宣称等价旧系统。
- [x] 分组 CRUD 别名：`/api/v2/groups/*` 与 `/api/v2/core/groups/*` 共用 `coreResourceStore`，具备新增、查询和删除的可重复测试路径。

### 2026-08-30 AI 执行面批次

- [x] 提供商目录和 AI 账户 CRUD：内置提供商元数据、分页/计数、账户更新删除写入 `ai.json`，API Key 等敏感字段脱敏；测试 `TestAIAccountModelsAndValidation`。
- [x] 账户模型增删改查与远程发现：模型记录绑定账户，重复和不存在返回明确错误；`/models` 请求使用 8 秒超时和 2 MiB 响应上限；测试 `TestAIAccountModelsAndValidation`、`TestAIAccountModelDiscoveryAndSandboxPersistence`。
- [x] GPU 能力探测：`nvidia-smi` 通过 3 秒 `CommandContext` 执行，无法使用时返回 CPU 降级和真实原因，不伪造设备。
- [x] CubeSandbox 健康、状态和生命周期：检查 Linux KVM，实例 start/stop/reconcile 状态持久化；缺少 KVM 或实例不存在时返回明确错误。

每完成一批接口，必须同时更新 `function-checklist-generated.md`、本节状态、测试文件和部署验证记录；扫描器报告中的 `partial`、`compatibility` 不得直接改写为完成。

## 2026-08-31 系统环境探测

| 状态 | 隐藏能力 | 旧源码证据 | 新实现证据 | 完成条件 |
|---|---|---|---|---|
| [x] | 应用安装、版本和运行状态探测 | `apps/workmesh-node/agent/app/api/v2/app_install.go:CheckAppInstalled`、`agent/app/service/nginx.go`、`database*.go` | `node/service/environment.go:ProbeApplication`、`node/api/apps.go:handleAppPost`；OpenResty/MySQL/PostgreSQL/Redis/Docker 使用受限探针，3-10 秒超时，返回 `isExist/isActive/status/version/error` | 已安装与未安装明确区分；daemon 不可用返回 stopped；无固定空数据；模拟二进制与 API 契约测试通过 |
| [x] | 云端备份 Bucket 标准接口 | `apps/workmesh-node/agent/cron/job/backup.go`、备份提供商适配器 | `node/api/functional_domains.go:handleBackupBuckets`、`normalizeBuckets`；从账号 Vars 读取显式 HTTPS 端点，Bearer 鉴权、15 秒超时、2 MiB 响应和 500 项上限 | 配置端点返回真实 Bucket 列表；错误、未配置和非法 URL 明确失败；不返回固定空列表；`TestBackupBucketsUsesConfiguredProviderEndpoint` 通过 |

1. 每勾选一项，必须在本表“新实现/证据”列写入代码路径、测试命令或部署记录。
2. `[~]` 和 `[ ]` 项不得在发布说明中描述为“完整迁移”；必须关联缺口任务和责任人。
3. 路由逐项状态以 [`function-checklist-generated.md`](./function-checklist-generated.md) 为准；隐藏能力与路由存在交叉时，两份清单都必须更新。
4. 最低验证命令：

```powershell
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test ./..."
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go vet ./..."
node scripts/with-dev-env.mjs -- node apps/workmesh-server/test/contract/route-scan.mjs check --legacy apps/workmesh-node --project apps/workmesh-server --manifest apps/workmesh-server/test/contract/routes.json
node scripts/with-dev-env.mjs -- node apps/workmesh-server/test/contract/implementation-scan.mjs --legacy apps/workmesh-node --project apps/workmesh-server --out .tmp/implementation-status.json
```

5. 发现旧源码中新增 `init`、`cron/job`、`middleware`、`i18n`、`log`、`ws`、`sse` 或命令入口时，先补充本清单，再实现代码。

## 2026-08-30 实现扫描器可信度审计

扫描器已修正为：忽略未被主路由调用的 `registerUnmigratedRoutes`，过滤函数中的路径常量不再作为实现证据；只有直接 `HandleFunc`、明确注册辅助函数或实际注册循环才计入。当前报告（基于 `test/contract/routes.json` 共 871 条）为 `implemented 871`、`partial 0`、`pending 0`。该报告不把兼容占位当作完成，重新生成命令为：

```powershell
node scripts/with-dev-env.mjs -- node apps/workmesh-server/test/contract/implementation-scan.mjs --legacy apps/workmesh-node --project apps/workmesh-server --manifest apps/workmesh-server/test/contract/routes.json --out .tmp/implementation-status.json --markdown apps/workmesh-server/docs/migration/function-checklist-generated.md
```

| 状态 | 路由 | 当前证据 | 剩余缺口 |
| --- | --- | --- | --- |
| implemented | `GET /api/v2/apps/checkupdate` | `node/api/apps.go:refreshCatalogLocked`、`latestCatalogVersion` | 从 `WORKMESH_APP_CATALOG` 有界读取应用元数据，按 key/id/name 取最高版本与已安装版本比较；目录版本、lastModified、同步时间原子持久化，配置错误返回明确 502 |
| implemented | `GET /api/v2/containers/search/log` | `node/api/container_log_stream.go` 使用 Docker 日志流；默认 SSE message 事件与前端 EventSource 兼容 | `since=all`、`tail=0`、Compose 多文件、心跳、断开取消、输出背压和参数安全均有测试 |

### 路由实现状态

当前实现扫描没有发现 pending、missing 或 partial 路由。应用目录升级检查和容器日志 SSE 已有真实处理逻辑及自动化测试；生产环境仍需使用实际应用目录和 Docker/Compose 实例做部署联调。

## 2026-08-30 非路由隐藏功能补充核对

| 状态 | 功能 | 旧源码证据 | 新项目证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [ ] | Agent/Core 启动初始化钩子 | `apps/workmesh-node/agent/init/hook/hook.go`、`agent/init/business/business.go` | `cmd/workmesh-server/main.go` 当前仅初始化 Store、Cache、Role、Scheduler、Gateway | 逐项接入全局数据、计划任务状态、运行时/SSL/Task 恢复、ACME 默认账户、Docker Compose 探测，并有重启测试 |
| [x] | Cobra/等价 CLI 管理入口 | `apps/workmesh-node/core/cmd/server/cmd/*.go` | `cmd/workmesh-server/cli.go`、`cli_test.go` | 已实现 `version`、`user-list`、`user-info`、`reset`、`listen-ip`、`app init`、签名制品 `restore/update`；Ed25519、SHA-256、目标路径限制和原子替换均有测试 |
| [~] | 全局 Session/CSRF/域名绑定/密码过期中间件 | `apps/workmesh-node/core/middleware/*.go`、`agent/middleware/certificate.go` | 新服务主要由 handler 自行校验 Token | 统一挂载 HTTP middleware，覆盖 Cookie/Bearer、CSRF、节点证书、Allow IP、Demo 只读和操作日志 |
| [x] | 日志文件输出、滚动和保留 | `apps/workmesh-node/core/log/*.go`、`agent/log/*` | `runtime/log/logger.go`、`cmd/workmesh-server/main.go` | 默认写入 `WORKMESH_DATA_DIR/logs/server.log`，按大小轮转并保留历史文件，支持显式路径 |
| [~] | 本地/SSH/容器终端双向 WebSocket | `apps/workmesh-node/agent/app/api/v2/hosts.go`、`core/app/api/v2/process.go` | `node/api/terminal_stream.go`、`websocket_stream.go` 已有流式实现草案 | 完成 PTY/SSH/容器会话、输入输出帧、鉴权、关闭码、超时和断线资源回收验收 |
| [x] | 容器日志 SSE | `apps/workmesh-node/agent/app/api/v2/container.go:935-966` | `node/api/container_log_stream.go` | 完成 `since/follow/tail/timestamp`、容器/Compose 过滤、心跳、断开取消和背压测试；日志使用默认 `message` 事件 |
### 2026-08-30 网站高级操作

- [x] 站点运行状态切换和可用性检查：`POST /api/v2/websites/operate`、`POST /api/v2/websites/check`，状态写入 `websites.json` 并拒绝未知操作。
- [x] 站点域名管理：`GET /api/v2/websites/domains/:websiteId`、`POST /api/v2/websites/domains*`，域名/端口校验后原子写入 `website-domains.json`。
- [x] 站点配置隐藏入口：Nginx、rewrite、目录、跳转、防盗链、HTTPS 配置统一持久化到 `website-configs.json`，网站不存在时返回 404。

### 2026-08-30 Node 运行时包管理

- [x] `POST /api/v2/runtimes/node/package` 从受限工作目录读取 `package.json` 的 scripts，限制文件大小并拒绝不存在目录。
- [x] `POST /api/v2/runtimes/node/modules` 扫描 `node_modules` 元数据，限制最多 500 项，不加载包代码。
- [x] `POST /api/v2/runtimes/node/modules/operate` 仅允许 npm/yarn 与 install/update/uninstall，异步执行并持久化任务状态；任务查询使用 `GET /api/v2/runtimes/node/tasks/:id`。

### 2026-08-30 应用目录详情

- [x] 应用详情、服务状态和安装参数从 `apps.json` 真实记录派生；详情中的 params/compose 不再使用固定空数组。
- [x] 安装删除检查返回应用及容器资源清单；应用版本查询按 catalog 记录过滤并限制在内存状态范围内。

### 2026-08-30 双节点部署验收

- [x] `61.184.12.165` 的 `workmesh-server.service` 已部署 Linux amd64 制品并验证 `/health`、`/ready`、首页和 JavaScript MIME。
- [x] `162.14.96.198` 的 `workmesh-server-secondary.service` 已部署 Linux amd64 制品并验证 `/health`、`/ready`、首页和 JavaScript MIME。
- [x] 主节点登录后可新增、查询、删除节点；新增 `secondary-gateway-162` 后重启主节点仍可查询，证明节点状态持久化。
- [~] Gateway 注册仍需真实云端凭据和节点登记：主节点返回 HTTP 401，次节点返回 `WORKMESH_NODE_NOT_FOUND`；当前仅能显示 pending，不能伪造已注册。

### 2026-08-30 国际化资源完整性

- [x] 服务端 12 个语言包已从旧 Agent 全量迁移至 `apps/workmesh-server/i18n/lang/*.yaml`，并替换原品牌标识。
- [x] `apps/workmesh-server/i18n/i18n.go` 提供嵌入式资源加载、未知语言回退中文和标量消息查询；`go test ./i18n` 已通过。
- [x] 前端 `web/src/lang/modules/*.ts` 保留 12 个语言模块；`npm.cmd run type-check` 与 `npm.cmd run build:pro` 已通过。

## 2026-08-31 启动初始化与 CLI 制品安全

| 状态 | 隐藏能力 | 旧源码证据 | 新实现证据 | 测试与剩余缺口 |
|---|---|---|---|---|
| [x] | 单进程启动时初始化数据目录与运行子目录 | `apps/workmesh-node/core/init`、`agent/init` | `cmd/workmesh-server/main.go` 调用 `initializeDataDir`；`apps/backups/logs/releases/runtime/uploads` 目录使用 0750 创建 | `cmd/workmesh-server/cli_test.go:TestInitializeDataDirCreatesRuntimeLayout`；生产目录权限需部署验收 |
| [x] | CLI restore/update 签名制品校验 | `apps/workmesh-node/core/cmd/server/cmd/restore.go`、`update.go` | `cmd/workmesh-server/cli.go:installSignedArtifact`；Ed25519 公钥、SHA-256 摘要、签名文件和大小上限校验，无签名材料明确报错 | `TestCLIUpdateVerifiesSignatureAndAtomicallyInstalls`、`TestCLIRestoreRejectsTamperedArtifactAndUnsafeTarget`；云端发布服务仍需真实凭据 |
| [x] | CLI 制品原子替换与回滚备份 | `apps/workmesh-node/core/cmd/server/cmd/restore.go` | `cmd/workmesh-server/cli.go:atomicInstall/saveArtifactResult`；同目录临时文件、Sync、rename，旧版本保存为 `.previous.<timestamp>`；`node/api/deployment_runtime.go:RecoverDeploymentState` 启动时校验并恢复活动制品 | `cmd/workmesh-server/node/api/deployment_runtime_test.go`；跨文件系统目标被拒绝并返回上下文错误 |
## 数据库后台能力（2026-08-31）

| 能力 | 入口 | 新实现 | 状态 | 说明 |
| --- | --- | --- | --- | --- |
| 数据库用户和授权元数据 | 数据库管理接口 | `node/service/database_admin.go` | implemented | 使用原子 JSON 持久化，密码不回显 |
| 数据库变量和配置文件 | 数据库管理接口 | `node/api/database_admin_routes.go` | implemented | 限制配置大小，支持重启恢复 |
# 计划任务隐藏能力核对

| 主机信息采集 | hosts 初始化 | node/api/hosts.go | implemented | 实时采集主机名、系统、架构、CPU、内存和运行时诊断 |
| Docker CLI 适配 | containers 初始化 | node/service/docker.go | implemented | 所有命令使用独立参数、超时和输出上限 |
| 容器文件操作 | containers service | node/api/containers.go | implemented | exec/cp 操作限制绝对路径并拒绝路径穿越 |

| 隐藏能力 | 发现位置 | 实现位置 | 状态 | 说明 |
|---|---|---|---|---|
| cron 后台轮询 | agent service/entry.go | node/service/cronjob.go | implemented | 单实例分钟调度，避免重复执行 |
| 任务失败重试 | agent service/cronjob_helper.go | node/service/cronjob.go | implemented | RetryTimes 与 Timeout 生效 |
| 脚本库持久化 | core script library | node/api/core_resources.go | implemented | scripts.json 原子写入，审核后执行 |
| 任务记录上限 | agent cronjobRepo | node/service/cronjob.go | implemented | 每任务最多保留 1000 条 |
| MCP 连接协议探测 | `apps/workmesh-node/agent/app/service/mcp_server.go:TestConnection` | `node/api/ai_execution.go:testMCPConnection` | implemented | 实际执行 Streamable HTTP initialize 或 SSE Content-Type 校验，失败不返回成功 |
| Agent 渠道配对命令 | `apps/workmesh-node/agent/app/service/agents_channels.go:ApproveChannelPairing` | `node/api/ai_execution.go:handleAgentPairingApprove` | implemented | 仅对已登记容器执行固定 Docker 参数，容器缺失返回不可用 |

| 文件分片上传状态 | `apps/workmesh-node/agent/app/api/v2/file.go:UploadChunkFiles` | `node/api/files_routes.go:handleChunkUpload` | implemented | 分片目录受 `WORKMESH_DATA_DIR` 控制，偏移和总大小校验，完成后原子提交 |
| 文件历史版本快照 | `apps/workmesh-node/agent/app/service/file_history.go` | `node/api/files.go:handleFilesSave`、`files_routes.go:history/*` | implemented | 保存前记录最多 200 条快照，支持恢复和删除 |
| 日志分页与类型清理 | `apps/workmesh-node/agent/app/api/v2/task.go`、`core/app/api/v2/logs.go` | `node/api/functional_domains.go:registerLogRoutes` | implemented | 日志检索支持关键字/类型/级别和分页，清理按类型过滤 |
| 压缩包安全解压 | `apps/workmesh-node/agent/app/service/file.go` | `node/api/files_routes.go:unzipPath` | implemented | 拒绝路径穿越与符号链接，条目临时文件原子替换 |
| 媒体转换后台任务 | `apps/workmesh-node/agent/app/service/file.go:Convert` | `node/api/files_routes.go:runMediaConversion` | implemented | 每个输入文件独立执行、5 分钟超时、输出文件校验并写入持久化日志 |
| 媒体转换 JSON 日志 | `apps/workmesh-node/agent/utils/convert/convert.go:appendJSONLog` | `node/api/files_routes.go:appendConvertLog`、`convert/log` | implemented | ConvertLogs 上限 2000，支持 taskID/status/type 过滤和分页 |

## 2026-08-31 AI 流式与 MCP 隐藏能力

| 隐藏能力 | 发现位置 | 实现位置 | 状态 | 说明 |
|---|---|---|---|---|
| MCP Streamable HTTP initialize 探测 | `apps/workmesh-node/agent/app/service/mcp_server.go:TestConnection` | `node/api/ai_execution.go:testMCPConnection` | implemented | 发送 JSON-RPC initialize，10 秒超时，网络失败返回明确错误 |
| MCP SSE 响应类型校验 | `apps/workmesh-node/agent/app/service/mcp_server.go:TestConnection` | `node/api/ai_execution.go:testMCPConnection` | implemented | 要求 `text/event-stream`，拒绝伪造成功 |

## 2026-08-31 容器管理隐藏能力

| 状态 | 隐藏能力 | 旧源码证据 | 新项目证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [x] | 镜像仓库配置持久化与密码脱敏 | `apps/workmesh-node/agent/app/service/image_repo.go` | `node/api/containers.go:containerStore`、`registerContainerRepositoryRoutes`；`containers.json` 原子写入 | CRUD、搜索、删除、状态接口测试通过 |
| [x] | Compose 模板持久化与批量导入 | `apps/workmesh-node/agent/app/service/compose_template.go` | `node/api/containers.go:registerContainerTemplateRoutes`；正文 4 MiB 上限 | 新增/更新/批量/删除/搜索和重启复读测试通过 |
| [x] | Compose 文件创建、更新、置顶及 `.env` 读取 | `apps/workmesh-node/agent/app/api/v2/container.go` | `node/api/containers.go:handleComposeCreate/Update/Pin/Env`；临时文件原子 rename | 路径穿越拒绝、文件内容和环境变量测试通过 |
| [x] | 容器用户及尺寸查询 | `apps/workmesh-node/agent/app/api/v2/container.go` | `node/api/containers.go:handleContainerPost`；固定 Docker argv 调用 `exec /etc/passwd`、`inspect --size` | 参数校验和 Docker 不可用错误可观测 |
| [x] | 镜像归档导入导出路径安全 | `apps/workmesh-node/agent/app/api/v2/container.go` | `node/api/containers.go:handleImageOperation`；`docker load -i`、`save -o`，拒绝 `..` | 无路径/穿越参数测试通过 |

## 2026-08-31 Agent 资源语义核对

| 状态 | 隐藏能力 | 旧源码证据 | 新项目证据 | 完成条件 |
|---|---|---|---|---|
| [x] | Agent 资源级备注、令牌重置和网站绑定 | `apps/workmesh-node/agent/app/api/v2/agents.go` | `node/api/ai_execution.go:handleAgentRoute`；随机令牌、目标资源归属校验、原子保存和脱敏 | `node/api/ai_execution_test.go:TestAgentResourceMutationsAndSessionLifecycle` |
| [x] | Agent 角色嵌套 CRUD 与频道聚合 | `apps/workmesh-node/agent/app/api/v2/agents.go` | `node/api/ai_execution.go:handleAgentRoute`；roles 持久化、重复冲突、父 Agent 校验 | 同上 |
| [x] | Hermes 会话生命周期 | `apps/workmesh-node/agent/app/api/v2/agents.go` | `node/api/ai_execution.go:handleSessionMutation`；重命名/删除位于通用删除分支之前，不存在返回 404 | 同上 |
| [x] | Ollama/MCP 资源状态操作不伪造记录 | `apps/workmesh-node/agent/app/api/v2/ai.go`、`mcp_server.go` | `node/api/ai_execution.go:handleAIResourceOperation`；资源 ID/名称必填，不存在返回 404，状态原子写入 | `node/api/ai_execution_test.go:TestAIResourceOperationsRequireExistingResource` |

| [x] | Docker CLI/daemon 状态 DTO 探测 | node/service/docker.go、node/api/host_container_cron.go | GET /api/v2/containers/docker/status 返回 isExist/isActive/version/error，10 秒超时并区分未安装与 daemon 不可用 | node/api/hosts_containers_test.go:TestDockerStatusContract |

## 2026-08-31 AI 错误本地化与实时通道协议补齐

| 状态 | 隐藏能力 | 旧源码证据 | 新项目证据 | 完成条件 |
|---|---|---|---|---|
| [x] | AI 错误按请求语言本地化 | `apps/workmesh-node/core/i18n`、`agent/app/api/v2/ai.go` | `node/api/errors.go:localizeErrorMessage` 与 `aiHandler` 的 Accept-Language 包装；稳定错误码映射到 12 个服务端语言包 | `TestAIErrorUsesRequestLocale` 验证英文请求不返回固定中文，未知语言回退中文 |
| [x] | SSE 事件编号与断线续传游标 | `apps/workmesh-node/agent/app/api/v2/container.go:ContainerStreamLogs` | `node/api/container_log_stream.go:containerSSEWriter` 输出 `id`，解析 `Last-Event-ID` 并延续序号，保留心跳与取消 | `TestContainerSSELastEventIDContinuesSequence`、容器日志流回归测试 |
| [x] | WebSocket 控制帧和正常关闭握手 | `apps/workmesh-node/agent/app/api/v2/terminal.go`、`core/app/api/v2/process.go` | `node/api/websocket_stream.go:closeWithCode/readFrame` 校验控制帧上限、掩码、关闭码；终端回送 Close/Pong | `TestWebSocketRejectsInvalidControlFrames`、`TestWebSocketCloseFrameIncludesCode` |

## 2026-08-31 节点透传安全隐藏能力

| 状态 | 隐藏能力 | 旧源码证据 | 新项目证据 | 完成条件 |
|---|---|---|---|---|
| [x] | 透传请求绕过目标本地 Session 的受信上下文 | `apps/workmesh-node/core/init/router/proxy.go`、`agent/utils/nodeclient/client.go` | `node/api/node_relay.go:IsForwardedRequestVerified` 注入进程内上下文；`cmd/workmesh-server/main.go:authenticateNodeAPI` 仅信任该上下文；外层 `control/api/security_middleware.go` 将透传交由 NodeRelay 验签 | `node/api/node_relay_test.go:TestNodeRelayForwardsSignedOperateNodeRequest`、`control/api/security_middleware_test.go:TestSecurityMiddlewareAllowsSignedRelayToReachNodeRelay` |
| [x] | 空请求体透传防御与大小限制 | `apps/workmesh-node/agent/utils/nodeclient/client.go` | `node/api/node_relay.go:forward/serveForwarded` 对 nil Body 使用 `http.NoBody`，请求/响应均限制 8 MiB | 节点透传测试覆盖请求体读取和超限错误 |
## 2026-08-31 终端 PTY 与 SSE 流式补齐

| 状态 | 隐藏能力 | 旧源码证据 | 新实现 | 验收说明 |
|---|---|---|---|---|
| [x] | Unix 本地终端真实 PTY、输入输出和 resize | `apps/workmesh-node/core/utils/terminal/local_cmd.go`、`ws_local_session.go` | `node/api/terminal_pty.go`、`terminal_stream.go` | 使用 `creack/pty.StartWithSize` 与 `pty.Setsize`，尺寸 1-500，断开杀进程并释放 PTY；`stream_protocol_test.go` 覆盖边界和回调 |
| [~] | 容器 `docker exec -it` 终端 | `apps/workmesh-node/agent/app/api/v2/terminal.go:WsContainerTerminal` | `node/api/terminal_stream.go` | 真实 PTY、输入输出、关闭码和闲置超时；需生产 Docker daemon/容器冒烟及信号联调 |
| [~] | SSH `-tt` 终端与远端窗口调整 | `apps/workmesh-node/agent/app/api/v2/terminal.go:WsHostSSH`、`utils/terminal/ws_session.go` | `node/api/terminal_stream.go` | 使用 BatchMode 与 10 秒连接超时，凭据仅来自 SSH 配置/Agent；远端 WindowChange 需 SSH 库或代理，未伪造成功 |
| [~] | SSE 断线重放、背压和写入超时 | `apps/workmesh-node/agent/app/api/v2/container.go:ContainerStreamLogs` | `node/api/container_log_stream.go` | `Last-Event-ID` 后重放最多 256 事件，缓存最多 128 流；积压上限 128 KiB，写入超时 10 秒；生产反向代理断线和跨重启行为待 E2E |
| [x] | OpenResty combined access log 监控聚合 | `apps/workmesh-node/agent/app/service/website.go`、`app/api/v2/website.go` | `node/api/analytics.go:loadAnalyticsEvents` 受限读取并解析访问日志，按日期、状态码、IP、UA 聚合 | `analytics_test.go` 覆盖时间过滤、流量、PV/UV、4xx 和爬虫统计；缺少 GeoIP 时明确返回原始 IP |
| [x] | 进程监听输出跨 ss/netstat 格式解析 | `apps/workmesh-node/agent/app/service/process.go:GetListeningProcess` | `node/api/process.go:parseListeningOutput` 识别前两个地址字段并限制 1024 条，保留进程元数据 | `process_test.go` 覆盖字段解析和上限；外部命令缺失返回明确 503 |
