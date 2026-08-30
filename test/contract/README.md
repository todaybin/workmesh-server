<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 路由契约测试

`route-scan.mjs` 从旧 Core/Agent 的 Go 路由源码生成快照，再扫描新服务源码进行严格差异校验。它不会启动旧服务，也不会因为新服务没有路由而假通过。

首次更新旧版本基线（仅在确认旧版本变更后执行）：

```powershell
node test/contract/route-scan.mjs generate --legacy ../workmesh-node --out test/contract/routes.json
```

迁移验收：

```powershell
node test/contract/route-scan.mjs check --legacy ../workmesh-node --project . --manifest test/contract/routes.json
```

校验按 HTTP 方法和完整路径比较。缺少任一路由、错误前缀或未审查的额外路由都会返回非零退出码；WebSocket/SSE 的 HTTP 升级入口仍按对应 HTTP 方法纳入清单。

## 实现状态扫描

`implementation-scan.mjs` 在同一份旧 Core/Agent 路由基线上，静态检查新服务源码是否存在对应实现，并识别 `MIGRATION_PENDING`、`StatusNotImplemented`、`compatibilityHandler`、`TODO` 和固定空列表等迁移缺口。输出字段包括 `method/path/source`、`new.status/source`、`domain`、`auth`、`persistence`、`test` 和 `gap`。

```powershell
node test/contract/implementation-scan.mjs `
  --legacy ../workmesh-node `
  --project . `
  --out .tmp/implementation-status.json
```

状态值：`implemented`（发现业务实现）、`partial`（实现但含固定空列表）、`compatibility`（兼容占位）、`pending`（迁移待完成）和 `missing`（未发现新路由）。脚本只读源码，不启动服务；报告中的启发式字段需结合对应 `evidence` 和源码复核。
