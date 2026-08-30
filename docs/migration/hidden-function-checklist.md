<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 隐藏功能迁移清单

本清单覆盖旧 `apps/workmesh-node/core` 与 `agent` 中不一定表现为 HTTP 路由的能力。当前路由清单为 863 条（包含 helper 注册和去品牌化静态入口）；本文件用于防止初始化钩子、后台作业、中间件和协议升级能力在迁移时遗漏。

状态定义：

- `[x]` 已有新实现，并有自动化或部署证据。
- `[~]` 已有边界或兼容实现，但仍缺少旧系统等价副作用，不能宣称完成。
- `[ ]` 尚未完成，必须在发布前补齐。

## 运行时初始化与生命周期

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

| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [~] | 服务端 i18n/localizer | `core/i18n`、`agent/i18n` | 前端 `vue-i18n`；Go 错误仍有中文固定文本 | 提供 zh/en 资源加载、按请求语言返回错误 |
| [~] | 业务错误、多错误聚合和错误码 | `core/buserr/*.go` | `runtime/http` ERR envelope | 完成错误码目录和逐域映射测试 |
| [x] | 异步任务、取消、重试、超时和日志 | `core/app/task/task.go`、`agent/global/global.go` | `node/service/cronjob.go`、任务 API；Go 单测 | 取消请求可终止执行，重启后记录可恢复 |
| [~] | 任务日志滚动与清理 | `core/log`、`agent/log` | `runtime/log/logger.go`、日志 API | 增加滚动文件、保留周期和磁盘上限验收 |

## 后台作业与数据维护

| 状态 | 隐藏能力 | 旧源码证据 | 新实现/证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| [~] | Cron 调度器与启停恢复 | `agent/cron/cron.go` | `runtime/schedule/scheduler.go`、cronjob service | 从持久化状态恢复启用任务并避免重复执行 |
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
| [~] | Swagger 文档 `GET /1panel/swagger/*any` | `apps/workmesh-node/core/init/router/router.go:77-79`；新服务 `/swagger/{any...}` | 已提供去品牌化 `/swagger/*any` JSON 入口；尚未接入文档文件和 SessionAuth，生产发布前必须补齐鉴权。 |
| [~] | 静态文件 `GET/HEAD /public/*filepath`、`GET/HEAD /favicon.ico/*filepath`、`GET/HEAD /assets/*filepath` | `apps/workmesh-node/core/init/router/router.go:25-39` | 新服务仅显式托管 `/assets/{filepath...}`、`/api/v2/images/*`、`/api/v2/static/*`；需验证 favicon/public 和 HEAD 响应的 MIME、缓存及路径穿越策略。 |
| [~] | 动态安全入口 `GET /{securityEntrance}` 与根页面安全检查 | `apps/workmesh-node/core/init/router/router.go:43-63` | 新服务根 Handler 对任意路径直接提供 SPA，未复刻 security entrance、Cookie 设置和安全检查；需在认证 E2E 中验证未授权访问行为。 |

实现状态扫描结果见逐路由清单，当前基线为 863 条路径；不能替代本节隐藏注册验收。特别关注以下固定/降级响应：`POST /api/v2/ai/agents/agent/list`、`POST /api/v2/ai/agents/agent/channels`、`POST /api/v2/ai/agents/overview`、GPU 无硬件时的空设备列表、`GET /api/v2/process/:pid`、文件回收站/收藏/上传查询、PHP/Node 运行时详情和工具箱配置。这些路径虽有处理器，仍需真实副作用或明确的能力不可用契约后才能将 `[~]` 改为 `[x]`。

## 迁移验收规则

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
