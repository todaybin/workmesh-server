<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WorkMesh Server 开发状态

这是开发恢复的唯一快速入口。先看本页，再打开对应任务详情；不要默认重新扫描全部迁移文档。

更新时间：2026-09-04

网站域名目录与独立配置代码已完成，WorkMesh Server 与自有 WAF 已部署；`znmp.sopvip.com` 使用独立域名目录和 80/443 入口，详见 [`2026-09-04-website-paths.md`](progress/2026-09-04-website-paths.md)。

本次开发：已补充全局写请求操作审计回调及 SQLite 操作日志查询/清理同步；前端页面与动态 import 完整性核对通过，并统一 API、文件下载和应用图标的同源传输；脚本/应用同步、受控 systemd 重启、运行时/AI/主机监控/数据库管理 SQLite 迁移已补齐；六类运行环境的目录搜索、归档部署、Compose 生命周期、PHP 构建/FastCGI/Supervisor/慢日志、63 项 PHP 扩展目录及精确卸载事务和 Node 模块管理已完成代码与自动化门禁。本轮修复运行时 Compose 默认变量、站点类型切换、字符串运行时引用、扩展任务动态日志和旧运行时生命周期兼容，并补齐自动生成任务 ID 的 Docker 输出流，详见 [`2026-09-03-website-runtime-switch.md`](progress/2026-09-03-website-runtime-switch.md) 与 [`2026-09-04-operation-log.md`](progress/2026-09-04-operation-log.md)。修复制品已部署，生产二进制 SHA256=`44c22e8af2179433dd9b03abe8b92fd78a5d6fe06a3412008bdf3b7c34ffb5a3`，前端 `dist/index.html` SHA256=`5ad783ab57cafaa68cb1e9768eea40952f644640a5da30ab4f1556adbb7eaf88`；真实 Docker 安装验收仍等待授权。详见 [`2026-09-02-operation-audit.md`](progress/2026-09-02-operation-audit.md)、[`2026-09-03-frontend-parity.md`](progress/2026-09-03-frontend-parity.md)、[`2026-09-03-backend-routes.md`](progress/2026-09-03-backend-routes.md)、[`2026-09-03-php-runtime-parity.md`](progress/2026-09-03-php-runtime-parity.md)、[`2026-09-03-runtime-parity.md`](progress/2026-09-03-runtime-parity.md) 与 [`2026-09-03-deployment.md`](progress/2026-09-03-deployment.md)。

## 已完成

| 状态 | 领域 | 完成摘要 | 证据 |
| --- | --- | --- | --- |
| [x] | 开发会话连续性规则 | 已增加状态索引、进度模板和断线恢复读取顺序 | [`2026-09-02-session-continuity.md`](progress/2026-09-02-session-continuity.md) |
| [x] | 单进程启动、健康检查和数据目录初始化 | 统一服务入口、`/health`、`/ready` 和运行目录初始化已具备测试 | [`README.md`](../../README.md)、[`cmd/workmesh-server/cli_test.go`](../../cmd/workmesh-server/cli_test.go) |
| [x] | 本地 Session、Cookie、Bearer 和 API Key 鉴权 | 控制面与节点执行面统一鉴权，未授权请求和 CSRF 场景有测试 | [`core-auth-session.md`](../api/core-auth-session.md)、[`security_middleware_test.go`](../../control/api/security_middleware_test.go) |
| [x] | 状态文件原子写入与重启恢复基础能力 | 已迁移领域使用临时文件和原子替换，关键服务具备持久化测试 | [`unified-server.md`](../architecture/unified-server.md)、[`core_persistence_test.go`](../../control/service/core_persistence_test.go) |
| [x] | OpenResty 容器探测与状态数组契约 | 容器化探测、版本/端口状态和空数组契约已验证 | [`website-openresty-waf.md`](../api/website-openresty-waf.md)、[`website_test.go`](../../node/service/website_test.go) |
| [x] | 控制面状态 SQLite 迁移补充 | AI、主机监控、文件辅助/分享、共享功能域和数据库管理旧 JSON 已一次性导入并归档；重启从 SQLite 恢复 | [`2026-09-03-backend-routes.md`](progress/2026-09-03-backend-routes.md) |
| [x] | PHP 创建表单与扩展模板对齐 | 扩展源及五组默认模板与 1Panel 一致，模板支持多选合并，CRUD 和重启恢复使用 SQLite | [`2026-09-03-php-runtime-parity.md`](progress/2026-09-03-php-runtime-parity.md) |
| [x] | 六类运行环境代码与自动化门禁 | 应用目录隔离、安全归档、真实 Compose 命令、SQLite、PHP FastCGI/Supervisor/慢日志、63 项扩展精确卸载事务和 Node 容器模块操作已实现并通过全量测试、vet、前端构建与契约扫描 | [`2026-09-03-runtime-parity.md`](progress/2026-09-03-runtime-parity.md) |

## 未完成

| 状态 | 领域 | 当前缺口 | 下一步 | 详情 |
| --- | --- | --- | --- | --- |
| [>] | 主机、容器和计划任务完整迁移 | 仍需补齐生产平台差异、资源配额、远程驱动和大任务异步化 | 按首批迁移清单逐项补齐并验证 | [`first-batch-host-container-cron.md`](../api/first-batch-host-container-cron.md) |
| [>] | Gateway 注册、心跳和跨节点任务透传 | 本地绑定可恢复；真实 Gateway 心跳、授权和跨节点任务仍需继续复验 | 配置真实凭据后复验心跳和任务透传 | [`deployment-verification-2026-08-31.md`](../migration/deployment-verification-2026-08-31.md)、[`link.md`](../api/link.md) |
| [>] | 终端与流式连接恢复 | 容器/SSH PTY 和跨重启 SSE 重放仍为 `partial` | 先完成容器/SSH PTY 联调，再验证代理断线重放 | [`function-checklist.md`](../migration/function-checklist.md) |
| [>] | AI、应用和在线开发剩余路由 | 已补齐脚本远端同步、已安装应用 Docker 状态同步、自定义应用归档同步和受控 systemd 重启；AI/运行时/ACME 等仍存在 `partial`、`pending` 项 | 按清单逐路由补齐真实行为、持久化和测试 | [`2026-09-03-backend-routes.md`](progress/2026-09-03-backend-routes.md)、[`ai-apps.md`](../api/ai-apps.md)、[`function-checklist.md`](../migration/function-checklist.md) |
| [>] | 构建、部署与正式验收 | 本轮 Linux amd64 制品已完成备份、原子部署、systemd 重启及健康检查；Gateway 凭据、次节点链路、KVM 和 ACME 仍阻塞完整验收 | 补齐外部凭据/网络后完成双节点和真实 ACME 回路 | [`2026-09-03-deployment.md`](progress/2026-09-03-deployment.md) |
| [!] | 六类运行环境真实安装验收 | 最新代码已部署，但八个指定版本实例及其编辑、终端、日志、扩展、模块和生命周期尚未在生产 Docker 上执行 | 获得 Docker 操作授权后逐实例验收；任一步失败立即恢复 | [`2026-09-03-runtime-parity.md`](progress/2026-09-03-runtime-parity.md) |
| [ ] | 全量完成门槛与最终发布验收 | TODO 中的全量迁移、双节点生产验收和完整测试门禁尚未全部满足 | 完成各未完成领域后运行全套门禁 | [`TODO.md`](../../TODO.md) |

## 使用规则

- `[x]` 只表示有真实实现、错误处理、持久化依据和测试/验收证据的事项。
- `[>]`、`[ ]` 和 `[!]` 都表示下次应继续处理的事项；进入任务后再创建或更新 `progress/` 下的详情文件。
- 完成里程碑后同步更新本页的状态、摘要、证据和下一步；不重复复制详情文档内容。
