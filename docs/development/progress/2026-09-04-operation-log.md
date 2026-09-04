<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-09-04 操作日志与扩展任务修复

状态：`[x] 已完成`

## 修改

- `/api/v2/core/logs/operation` 查询完整返回 `source/user/node/ip/path/method/userAgent/latency/status/message/detailZH/detailEN/createdAt`。
- 操作日志 ID 保持旧接口数字语义；写入时规范化来源、路径、方法、状态、节点和客户端 IP，耗时使用 `time.Duration` 纳秒语义。
- 读取旧记录时兼容修正固定 `server` 来源、带端口 IP、`/api/v2` 前缀、大小写状态和空详情。
- 操作日志搜索支持 `source`、`node`、`status` 和 `operation` 条件。
- PHP 扩展异步安装登记统一 `app_install_tasks`，任务日志读取支持 runtime 日志路径。
- 运行时扩展 Docker stdout/stderr 实时写入任务日志；旧运行时记录自动补齐 Compose 路径、容器名和 PHP 镜像，停止/重启及二次安装可复用历史记录。

## 验证与部署

- `GOWORK=off go test ./...` 通过。
- `GOWORK=off go vet ./...` 通过。
- 新增 `TestOperationLogsReturnCompleteContract`，覆盖完整字段、数字 ID、IP/路径/状态规范化。
- 生产备份：`/opt/workmesh-server/backups/deploy-20260904T-runtime-task-website-v7`、`/opt/workmesh-server/backups/deploy-20260904T-runtime-legacy-hydrate-v8`、`/opt/workmesh-server/backups/deploy-20260904T-runtime-task-stream-v9`。
- 生产二进制 SHA256：`44c22e8af2179433dd9b03abe8b92fd78a5d6fe06a3412008bdf3b7c34ffb5a3`；前端 `dist/index.html` SHA256：`5ad783ab57cafaa68cb1e9768eea40952f644640a5da30ab4f1556adbb7eaf88`。
- `workmesh-server.service` 为 `active/running`，`/health` 与 `/ready` 均 HTTP 200。

## 说明

- 已使用生产管理员 Bearer 会话验证日志列表返回；未认证请求仍按安全策略返回 401。完整字段和历史兼容同时由隔离 SQLite 回归测试验证。
