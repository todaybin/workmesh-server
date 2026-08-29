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

## 完整性校验

`test/contract/routes.json` 是从旧 Core/Agent 路由源码生成的基线（当前 825 条）。使用以下命令生成或校验：

```powershell
node test/contract/route-scan.mjs check --legacy ../workmesh-node --project . --manifest test/contract/routes.json
```

缺失、路径前缀错误或未审查的额外路由会返回非零退出码。迁移完成前该命令失败是预期状态，不能删除清单来获得通过。
