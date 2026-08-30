<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WorkMesh Server 部署与功能验收清单（2026-08-30）

本清单记录本次单进程部署的真实验证结果。路由契约扫描只证明路径注册，不作为功能完成证明；功能必须有真实响应、状态变化或副作用后才能标记为完成。

## 已完成并验证

| 功能 | 关键接口 | 验证结果 | 验收证据 |
| --- | --- | --- | --- |
| 单进程服务 | `/health`、`/ready` | 主节点、次节点 HTTP 200 | systemd `active`；本机与主节点请求通过 |
| 前端静态资源 | `/assets/*.js`、`/assets/*.css` | MIME 正确，返回真实文件 | JS `text/javascript`，CSS `text/css` |
| 节点列表与节点登记 | `POST /api/v2/core/nodes/list`、`GET /api/v2/core/nodes/simple/all`、`POST /api/v2/core/nodes/add`、`POST /api/v2/core/nodes/update`、`POST /api/v2/core/nodes/del`、`POST /api/v2/core/xpack/nodes/favorite` | 节点信息持久化到 `nodes.json`，支持筛选、添加、更新、删除和收藏 | 已通过节点添加/收藏往返测试；主节点当前 `primary-main`，次节点 `secondary-gateway-162` |
| 节点角色 | `GET /api/v2/core/nodes/role` | 返回当前角色和 epoch | 主/次节点真实状态 |
| 基础设置 | `POST /api/v2/core/settings/search/base` | 返回语言、主题等设置 | HTTP 200，JSON data |
| 执行中任务计数 | `GET /api/v2/logs/tasks/executing/count` | 返回数字计数 | HTTP 200，`data: 0` |
| Gateway 状态 | `GET /api/v2/workmesh/gateway/status` | 状态可查询 | HTTP 200，未配置凭证时明确 `pending` |
| Docker/Compose | 容器、Compose 操作接口 | 已有单元/HTTP 测试 | `go test ./...` |
| 节点链路 | handshake、heartbeat、sync、fencing | 已有签名、防重放、epoch 测试 | `go test ./runtime/link ./control/api` |

## 当前阻断或未完成

| 功能 | 状态 | 原因与处理 |
| --- | --- | --- |
| Gateway 节点注册与授权 | `blocked` | 客户端已改为携带 Ed25519 公钥并按 Gateway Runner 契约签名；远端 `server.env` 目前只配置 Gateway URL 和节点角色，缺少 `WORKMESH_GATEWAY_USERNAME/PASSWORD/ID/SECRET`，当前返回业务 401。补齐凭证后重新执行注册和心跳验收。 |
| 主节点到次节点跨机链路 | `blocked` | `61.184.12.165 -> 162.14.96.198:9999` 当前被安全组/防火墙阻断。只放行主节点来源地址后执行 link E2E。 |
| 旧接口完整迁移 | `partial` | `legacy_routes.go` 仍有约 751 个 `MIGRATION_PENDING`，必须逐条关联旧 handler/service 并实现真实行为。 |
| CubeSandbox MicroVM E2E | `degraded` | 主节点无 `/dev/kvm`，只能验证受限模式，不得标记 MicroVM E2E 完成。 |

## 重复验收命令

```powershell
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test ./..."
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go vet ./..."
node scripts/with-dev-env.mjs -- node test/contract/route-scan.mjs check --legacy apps/workmesh-node --project apps/workmesh-server --manifest apps/workmesh-server/test/contract/routes.json
```

最新部署制品 SHA256：`60f6fe33a8282047fdcef8eabed18c4123115a09284bf24cc227be8611f884d3`。
