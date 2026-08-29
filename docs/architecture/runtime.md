<!-- SPDX-License-Identifier: LicenseRef-WorkMesh-Pending -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WorkMesh Server 运行时架构

WorkMesh Server 是独立 Git 仓库 `https://github.com/todaybin/workmesh-server.git` 中的单进程服务。它将 Core 控制面与 Agent 执行面放入同一 HTTP 生命周期，避免两个进程重复创建日志、数据库连接、HTTP Transport、缓存和调度器。

## 运行边界

- `control` 负责本机登录、Session、MFA、Passkey、RBAC、设置、审计和多机管理。
- `node` 负责系统命令、主机、容器、应用、网站、数据库、文件、计划任务、备份、告警、AI、沙盒和部署。
- `runtime` 只提供共享基础设施、角色协议、节点链路和 Gateway 适配器。
- `web` 嵌入完整前端；本机控制面到执行面优先使用进程内 service 调用。

## 生命周期

启动顺序为配置 -> 日志 -> 状态存储 -> 角色校验 -> 调度器 -> HTTP Server。关闭时先停止接收新请求，再停止调度器和后台任务，最后在超时上下文内关闭 HTTP 和存储。

未启用的 AI、MCP、扫描、WAF、CubeSandbox 和大型备份任务不得创建常驻 Worker。`/health` 不依赖数据库或 Gateway；`/ready` 只做短超时依赖检查。

## 数据与安全

节点使用本地 SQLite（WAL）；Redis 仅显式配置时启用。主次角色由 `role_epoch` 和 fencing 保护，Gateway 不能绕过本机角色协议直接选主。所有节点必须独立注册 Gateway，Gateway 凭证和节点私钥只保存加密引用，禁止写入日志或前端响应。
