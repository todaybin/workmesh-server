<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WorkMesh Server

WorkMesh Server 是独立的单进程节点服务，Git 权威仓库为
`https://github.com/todaybin/workmesh-server.git`，Go 模块为
`github.com/todaybin/workmesh-server`。

当前目录提供统一 HTTP、状态存储、缓存、日志、调度、节点角色和 Gateway 对接边界。Core 与 Agent 的完整业务功能按迁移清单逐域迁移，原 `apps/workmesh-node` 项目保持不变且不作为运行时依赖。

## 运行

```powershell
go run ./cmd/workmesh-server
```

默认监听 `:9999`。可通过 `WORKMESH_SERVER_ADDR`、`WORKMESH_DATA_DIR`、`WORKMESH_NODE_ID`、`WORKMESH_NODE_ROLE`、`WORKMESH_GATEWAY_URL`、`WORKMESH_GATEWAY_ID`、`WORKMESH_GATEWAY_SECRET`、`WORKMESH_GATEWAY_USERNAME` 和 `WORKMESH_GATEWAY_PASSWORD` 配置。Gateway 用户名和密码仅用于启动时换取短期 JWT，不写入日志或响应。首次运行会在数据目录写入节点状态文件，敏感凭据不得写入日志或普通配置。

## 验证

```powershell
go test ./...
go build ./cmd/workmesh-server
```

`/health` 不依赖数据库或 Gateway；`/ready` 用于后续依赖就绪检查。

## 迁移规则

- 新服务与 `apps/workmesh-node` 完全解耦，不复制旧项目的 Git 元数据和构建产物。
- 所有旧 API、WebSocket、SSE、文件传输、系统命令、容器、计划任务等必须逐项登记并通过契约测试后才能标记完成。
- 新增和修改的代码注释使用中文；许可证和第三方 NOTICE 按 `docs/legal` 记录。
