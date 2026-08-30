<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

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
| [x] | 节点角色与 epoch/fencing | `core/utils/xpack/providers/multi_node.go`、`agent/utils/xpack/providers/multi_node.go` | `runtime/role`、`runtime/link`；链路单元测试 | 旧 epoch 请求返回 409，禁止双主写入 |
| [~] | 启动配置、数据目录和资源限制 | `core/global/config.go`、`agent/global/config.go` | `config/config.go`、`WORKMESH_*` 环境变量 | 补充生产配置校验和资源上限 E2E |

## 安全中间件与授权

| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [~] | Session/Cookie/Bearer/API Key 认证 | `core/middleware/session.go` | `control/service/core.go`、`node/api/core_handlers.go`；认证 HTTP 测试 | 所有需保护路由逐条验证未登录 401/403 |
| [~] | CSRF 防护 | `core/middleware/csrf_protect.go` | 前端请求头与后端写接口校验仍需统一 | 增加 token 生成、轮换、失败审计和跨站测试 |
| [~] | Demo/只读模式限制 | `core/middleware/demo_handle.go` | 新服务暂以角色权限承接 | 覆盖旧白名单并验证所有写接口被拒绝 |
| [~] | 域名绑定与密码过期 | `core/middleware/bind_domain.go`、`password_expired.go` | 设置接口已有字段，专用中间件待接入 | 绑定域名、过期密码和例外路由 E2E |
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
| [x] | 异步任务、取消、重试、超时和日志 | `core/app/task/task.go`、`agent/global/global.go` | `node/service/cronjob.go`、任务 API；Go 单测 | 取消请求可终止执行，重启后记录可恢复 |
| [x] | 任务隔离 Provider 生命周期与 CLI 白名单 | `apps/workmesh-node/agent/app/api/v2/workmesh_task.go`、`agent/utils/cubesandbox/task.go`、`forgevm_task_backend.go` | `node/service/taskruntime/taskruntime.go`、`node/api/ai_execution.go:taskHandler`；固定 sha256 CLI、argv 校验、状态转换和 30 分钟超时 | 未配置真实 CLI 时返回明确 503；配置摘要后 create/start/exec/collect/cancel/destroy 均调用受控 Provider，禁止宿主 Shell |
| [x] | 任务日志滚动与清理 | `core/log`、`agent/log` | `runtime/log/logger.go`、`runtime/log/logger_test.go` | 按大小轮转并保留 5 个历史文件，写入线程安全，关闭时刷新 |

## 后台作业与数据维护

| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [x] | Cron 调度器与启停恢复 | `agent/cron/cron.go` | `node/service/cronjob.go`、`node/api/host_container_cron.go` | 从持久化状态恢复启用任务，按五字段 Spec 每分钟执行并支持停止 |
| [~] | 网站/SSL 定时作业 | `agent/cron/job/website.go`、`ssl.go` | 网站 API 已迁移，后台作业未等价接入 | 使用测试证书和失败重试完成 E2E |
| [~] | 备份账号 token 刷新 | `agent/cron/job/backup.go` | 备份 API 可记录和恢复；云账号刷新待接入 | OneDrive/阿里云 token 刷新及失败告警 |
| [x] | 状态文件原子写入和恢复 | 旧 DB 初始化/迁移钩子 | `node/api` 各域 JSON store 使用临时文件+rename | 并发写入和断电恢复测试通过 |

## 协议与执行通道

| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [~] | 本地/SSH/容器终端 WebSocket | `core/middleware/demo_handle.go`、Agent terminal routers | `node/api/process.go`、`deployment_runtime.go` | 完成真实双向帧、关闭码和权限测试 |
| [~] | SSE/流式任务输出 | Agent 执行与日志路由 | Gateway/link 与任务 API 边界已建 | 增加断线续传、心跳和背压测试 |
| [x] | 节点 handshake/heartbeat/sync | xpack multi-node provider | `runtime/link`；HMAC、timestamp、nonce、防重放测试 | 双节点公网链路和 fencing 验收 |
| [~] | Gateway 登录、注册、心跳和解绑 | WorkMesh gateway router | `runtime/gateway/http_client.go` | 写入真实 Gateway 凭据后状态必须为 `registered/connected` |

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
| [x] | 备份连接检查、Bucket 查询和 token 刷新状态 | `handleBackupConnCheck`、`handleBackupBuckets`、`handleBackupRefreshToken` | 连接检查单测；刷新状态写入 `domains.json` |
| [~] | OneDrive/阿里云等云端真实 token 刷新和远端 Bucket 操作 | 轻量实现返回本地状态，未引入云 SDK | 生产凭据和云端集成测试完成后才能标记 `[x]` |

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

扫描器已修正为：忽略未被主路由调用的 `registerUnmigratedRoutes`，过滤函数中的路径常量不再作为实现证据；只有直接 `HandleFunc`、明确注册辅助函数或实际注册循环才计入。当前报告（基于 `test/contract/routes.json` 共 871 条）为 `implemented 826`、`partial 2`、`pending 43`。该报告不把兼容占位当作完成，重新生成命令为：

```powershell
node scripts/with-dev-env.mjs -- node apps/workmesh-server/test/contract/implementation-scan.mjs --legacy apps/workmesh-node --project apps/workmesh-server --manifest apps/workmesh-server/test/contract/routes.json --out .tmp/implementation-status.json --markdown apps/workmesh-server/docs/migration/function-checklist-generated.md
```

| 状态 | 路由 | 当前证据 | 剩余缺口 |
| --- | --- | --- | --- |
| partial | `GET /api/v2/apps/checkupdate` | `node/api/apps.go` 返回固定 `canUpdate=false` | 接入应用目录同步状态、版本比较和异步更新任务 |
| partial | `GET /api/v2/containers/search/log` | `node/api/containers.go` 已有入口；旧 SSE 语义需以 `Accept: text/event-stream` 持续输出 | 容器日志跟随、since/tail/timestamp、断开释放和背压测试 |

### 仍为 pending 的路由

以下路由目前只能由迁移占位或未接入的旧契约承接，禁止在发布说明中描述为“已迁移”：

| 功能域 | 路由 |
| --- | --- |
| toolbox | `GET /api/v2/toolbox/device/users`、`GET /api/v2/toolbox/device/zone/options`、`GET /api/v2/toolbox/fail2ban/base`、`GET /api/v2/toolbox/fail2ban/load/conf`、`GET /api/v2/toolbox/ftp/base` |
| runtimes | `POST /api/v2/runtimes/node/modules`、`POST /api/v2/runtimes/node/modules/operate`、`POST /api/v2/runtimes/node/package` |
| websites | `GET /api/v2/websites/proxy/config/:id`、`GET /api/v2/websites/realip/config/:id`、`POST /api/v2/websites/cors/update`、`POST /api/v2/websites/default/html/update`、`POST /api/v2/websites/default/server`、`POST /api/v2/websites/dir`、`POST /api/v2/websites/dir/permission`、`POST /api/v2/websites/dir/update`、`POST /api/v2/websites/dns/update`、`POST /api/v2/websites/lbs/create`、`POST /api/v2/websites/lbs/file`、`POST /api/v2/websites/lbs/update`、`POST /api/v2/websites/leech`、`POST /api/v2/websites/leech/update`、`POST /api/v2/websites/monitor/config/global`、`POST /api/v2/websites/monitor/config/site`、`POST /api/v2/websites/monitor/config/site/update`、`POST /api/v2/websites/monitor/qps`、`POST /api/v2/websites/monitor/rank`、`POST /api/v2/websites/monitor/stat`、`POST /api/v2/websites/monitor/trend`、`POST /api/v2/websites/monitor/visitors`、`POST /api/v2/websites/monitor/visitors/loc`、`POST /api/v2/websites/monitor/websites`、`POST /api/v2/websites/proxy/clear`、`POST /api/v2/websites/proxy/config`、`POST /api/v2/websites/realip/config`、`POST /api/v2/websites/redirect`、`POST /api/v2/websites/redirect/file`、`POST /api/v2/websites/redirect/update`、`POST /api/v2/websites/rewrite`、`POST /api/v2/websites/rewrite/custom`、`POST /api/v2/websites/rewrite/update`、`POST /api/v2/websites/stream/update`、`POST /api/v2/websites/waf/test` |

## 2026-08-30 非路由隐藏功能补充核对

| 状态 | 功能 | 旧源码证据 | 新项目证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [ ] | Agent/Core 启动初始化钩子 | `apps/workmesh-node/agent/init/hook/hook.go`、`agent/init/business/business.go` | `cmd/workmesh-server/main.go` 当前仅初始化 Store、Cache、Role、Scheduler、Gateway | 逐项接入全局数据、计划任务状态、运行时/SSL/Task 恢复、ACME 默认账户、Docker Compose 探测，并有重启测试 |
| [x] | Cobra/等价 CLI 管理入口 | `apps/workmesh-node/core/cmd/server/cmd/*.go` | `cmd/workmesh-server/cli.go`、`cli_test.go` | 实现 `version`、`user-list`、`user-info`、`reset`、`listen-ip`、`app init`；`restore/update` 对未验证路径明确拒绝 |
| [~] | 全局 Session/CSRF/域名绑定/密码过期中间件 | `apps/workmesh-node/core/middleware/*.go`、`agent/middleware/certificate.go` | 新服务主要由 handler 自行校验 Token | 统一挂载 HTTP middleware，覆盖 Cookie/Bearer、CSRF、节点证书、Allow IP、Demo 只读和操作日志 |
| [x] | 日志文件输出、滚动和保留 | `apps/workmesh-node/core/log/*.go`、`agent/log/*` | `runtime/log/logger.go`、`cmd/workmesh-server/main.go` | 默认写入 `WORKMESH_DATA_DIR/logs/server.log`，按大小轮转并保留历史文件，支持显式路径 |
| [~] | 本地/SSH/容器终端双向 WebSocket | `apps/workmesh-node/agent/app/api/v2/hosts.go`、`core/app/api/v2/process.go` | `node/api/terminal_stream.go`、`websocket_stream.go` 已有流式实现草案 | 完成 PTY/SSH/容器会话、输入输出帧、鉴权、关闭码、超时和断线资源回收验收 |
| [~] | 容器日志 SSE | `apps/workmesh-node/agent/app/api/v2/container.go:935-966` | `node/api/container_log_stream.go` 已有实现草案 | 完成 `since/follow/tail/timestamp`、容器/Compose 过滤、心跳、断线续传和背压测试 |
### 2026-08-30 网站高级操作

- [x] 站点运行状态切换和可用性检查：`POST /api/v2/websites/operate`、`POST /api/v2/websites/check`，状态写入 `websites.json` 并拒绝未知操作。
- [x] 站点域名管理：`GET /api/v2/websites/domains/:websiteId`、`POST /api/v2/websites/domains*`，域名/端口校验后原子写入 `website-domains.json`。
- [x] 站点配置隐藏入口：Nginx、rewrite、目录、跳转、防盗链、HTTPS 配置统一持久化到 `website-configs.json`，网站不存在时返回 404。

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
