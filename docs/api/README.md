<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# API 规范与完整清单

兼容接口继续使用 `/api/v2/core/*`（控制面）和 `/api/v2/*`（节点执行面）。WebSocket、SSE、终端、文件传输、流式输出、分页、错误 envelope 和 HTTP 方法必须保持原行为。

## 新增边界

- `/api/v2/gateway/*`：每个节点的 Gateway 登录、注册、授权刷新、心跳和同步。
- `/api/v2/gateway/inbound/*`：Gateway 向节点派发任务、策略和授权撤销。
- `/api/v2/link/*`：主节点与次节点握手、心跳、角色切换和增量同步。

节点链路的签名、重试、游标和 fencing 约束见 [link.md](link.md)。
- `/api/v2/system/agent/*`：Agent 沙盒和在线开发统一生命周期。
- `/api/v2/agent-runtime/*`、`/api/v2/projects/{projectId}/tasks`、`/api/v2/projects/{projectId}/events/stream`、`/api/v2/projects/{projectId}/artifacts/reconcile`、`/api/v2/projects/{projectId}/artifacts/reclaim-plan`、`/api/v2/projects/{projectId}/artifacts/reclaim-plans`、`/api/v2/projects/{projectId}/deployments/dry-run`、`/api/v2/projects/{projectId}/deployments/plans`、`/api/v2/dev/tasks/{taskId}/artifacts`、`/api/v2/dev/tasks/{taskId}/evidence`、`/api/v2/dev/tasks/{taskId}/artifacts/verify`、`/api/v2/dev/tasks/{taskId}/artifacts/reconcile`、`/api/v2/dev/tasks/{taskId}/artifacts/{artifactId}`：项目 Agent runtime 注册、任务、Artifact 元数据/文件、只读回收计划、非生产部署计划和 SSE 团队事件，详见 [agent-team.md](agent-team.md)。回收与部署计划均需要人工审批，当前不会删除文件或执行部署。
- `/api/v2/workmesh/tasks/*`：受控 Sandbox 任务生命周期接口，详见 [ai-tasks.md](ai-tasks.md)。

## 完整性校验

`test/contract/routes.json` 是从只读参考 `/www/apps/1Panel` 的 Core/Agent 路由源码生成的兼容基线（当前 759 条）；同一份来源和字段也归档在 `docs/inventory/route-inventory-1panel.json`。已废弃的 `apps/workmesh-node` 不再作为正式基线。使用以下命令校验：

```powershell
node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest docs/inventory/route-inventory-1panel.json
```

缺失、路径前缀错误或未审查的额外路由会返回非零退出码。迁移完成前该命令失败是预期状态，不能删除清单来获得通过。
