<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# HTTP 操作审计链路

状态：`[>] 进行中`

## 本次完成

- 全局安全中间件支持写请求完成后的操作日志回调。
- 审计包装不读取或保存请求体，避免改变上传/流式语义并防止凭据泄漏。
- 搜索、GET 和流式接口不记录操作日志。
- 操作日志写入 SQLite `operation_logs`，记录路径、方法、用户、IP、User-Agent、耗时、状态和消息。
- `/api/v2/core/logs/operation` 从 SQLite 查询操作记录；清理接口同步删除 SQLite 记录。
- 路由、实现和隐藏能力扫描均通过。

## 验证与阻塞

- `node test/contract/route-scan.mjs check --legacy ../workmesh-node --project . --manifest test/contract/routes.json`：通过，871 条路由。
- `node test/contract/implementation-scan.mjs ...`：通过，871 条实现记录。
- `node test/contract/hidden-function-scan.mjs ...`：通过。
- 当前容器未安装 Go，无法运行 `go test ./...` 或 `go vet ./...`。
- 通过隔离的 Node.js 20.20.2 运行 `npm run type-check` 和 `npm run build:pro`：均通过。

## 下一步

- 在 Go 1.26 环境运行完整后端门禁。
- 将备份、告警、系统设置等低频 `domains.json` 状态继续迁移到 SQLite 表，保留只读一次性导入。
- 在真实登录、写请求和日志查询回路中核对操作人字段、响应 code 判定和清理分页契约。
