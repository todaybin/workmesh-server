<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WorkMesh 功能迁移清单

本清单以旧 `apps/workmesh-node/core` 与 `agent` 的 863 条路由为基线（含隐藏 helper 注册和去品牌化静态入口）。状态必须以真实副作用或端到端响应确认，不能仅以路由注册作为完成依据。

非路由的初始化、后台作业、中间件、国际化、日志、任务和协议升级能力见 [`hidden-function-checklist.md`](./hidden-function-checklist.md)，两份清单必须同步维护。

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
| 计划任务 | `/api/v2/cronjobs/*` | 任务持久化、启停、单次执行、执行记录、清理、导入导出 | `go test ./node/service ./node/api` |
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

## 2026-08-30 AI 账户与沙盒批次

| 功能名称 | 旧源码位置 | 旧路由或入口 | 新源码位置 | 接口方法和路径 | 鉴权方式 | 数据来源 | 持久化方式 | 测试文件 | 测试命令 | 部署验证 | 当前状态 | 剩余缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| AI 提供商目录与账户分页 | `apps/workmesh-node/agent/app/provider/catalog.go`、`agent/app/service/agents.go` | `GET /api/v2/ai/accounts/providers`、`POST /api/v2/ai/accounts`、`POST /api/v2/ai/accounts/update`、`POST /api/v2/ai/accounts/search`、`POST /api/v2/ai/accounts/counts`、`POST /api/v2/ai/accounts/delete` | `node/api/ai_execution.go:aiProviders`、`handleAccountRoute` | 同左 | 上层 Session/Bearer；账户写入需节点权限 | 内置提供商元数据和请求体 | `.workmesh-data/ai.json` 原子 rename | `node/api/ai_execution_test.go:TestAIAccountModelsAndValidation` | `go test ./node/api -run AIAccount` | 待下一批制品部署 | implemented | 真实 Gateway 账户同步待凭据接入 |
| 账户模型管理与远程发现 | `apps/workmesh-node/agent/app/service/agents.go:1201-1345` | `POST /api/v2/ai/accounts/models`、`models/create`、`models/update`、`models/delete`、`models/discover`、`verify` | `node/api/ai_execution.go:handleAccountRoute`、`discoverAIModels` | 同左 | Session/Bearer；API Key 仅用于上游请求且响应脱敏 | 账户持久化模型或上游 `/models` JSON | `ai.json` 原子 rename；HTTP 8 秒超时 | `node/api/ai_execution_test.go:TestAIAccountModelsAndValidation`、`TestAIAccountModelDiscoveryAndSandboxPersistence` | `go test ./node/api -run 'AIAccount|AI.*Discovery'` | 待下一批制品部署 | 不同厂商专用鉴权协议需按 provider 扩展 |
| GPU 能力探测 | `apps/workmesh-node/agent/api/v2/monitor.go` | `GET /api/v2/ai/gpu/load`、`GET /api/v2/ai/gpu/options`、`POST /api/v2/ai/gpu/search` | `node/api/ai_execution.go:detectGPU` | 同左 | Session/Bearer | `nvidia-smi` 设备信息和 Go 运行时内存 | 无状态采集 | `node/api/ai_execution_test.go`（CPU 降级路径） | `go test ./node/api -run AI` | 待下一批制品部署 | AMD/NPU/XPU 驱动适配待补充 |
| CubeSandbox 生命周期与状态 | `apps/workmesh-node/agent/router/ro_cubesandbox.go`、`agent/app/api/v2/workmesh_task.go` | `GET /api/v2/cubesandbox/health`、`GET /api/v2/cubesandbox/status`、`POST /api/v2/cubesandbox/start`、`stop`、`reconcile` | `node/api/ai_execution.go:sandboxHandler` | 同左 | Session/Bearer；启动受 KVM 能力限制 | `/dev/kvm` 能力探测与实例状态 | `ai.json` Sandboxes 数组原子 rename | `node/api/ai_execution_test.go:TestAIAccountModelDiscoveryAndSandboxPersistence` | `go test ./node/api -run Sandbox` | 待下一批制品部署 | MicroVM 实际进程编排和镜像校验待接入 |
