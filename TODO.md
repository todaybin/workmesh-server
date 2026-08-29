<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 迁移任务

- [x] 完成 Core 与 Agent 路由的机器可读盘点（当前基线 831 条）；Controller、Service、Model 仍需按功能域迁移。
- [ ] 合并数据库迁移、Session、RBAC、日志、缓存和调度器。
- [ ] 迁移系统命令、主机、Docker、应用、网站、数据库、文件和计划任务能力。
- [ ] 迁移终端 WebSocket、SSE、上传下载、备份、告警、AI、MCP、CubeSandbox 和部署任务。
- [ ] 实现主节点/次节点通信、同步、角色 epoch、fencing 和回滚。
- [ ] 实现所有节点独立 Gateway 注册、授权、心跳和任务透传。
- [ ] 接入完整前端并清理旧产品品牌和 import 路径。
- [x] 建立新旧路由差异、权限、安全、性能和双节点验收测试入口；迁移完成前差异检查必须保持失败。
