<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 路由契约测试

`route-scan.mjs` 从旧 Core/Agent 的 Go 路由源码生成快照，再扫描新服务源码进行严格差异校验。它不会启动旧服务，也不会因为新服务没有路由而假通过。

首次从只读 1Panel 参考生成接口基线（仅在确认参考版本变更后执行）：

```powershell
node test/contract/route-scan.mjs generate --legacy /www/apps/1Panel --out docs/inventory/route-inventory-1panel.json
```

迁移验收：

```powershell
node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest docs/inventory/route-inventory-1panel.json
```

校验按 HTTP 方法和完整路径比较。缺少任一路由、错误前缀或未审查的额外路由都会返回非零退出码；WebSocket/SSE 的 HTTP 升级入口仍按对应 HTTP 方法纳入清单。

## 实现状态扫描

`implementation-scan.mjs` 在同一份旧 Core/Agent 路由基线上，静态检查新服务源码是否存在对应实现，并识别 `MIGRATION_PENDING`、`StatusNotImplemented`、`compatibilityHandler`、`TODO` 和固定空列表等迁移缺口。输出字段包括 `method/path/source`、`new.status/source`、`domain`、`auth`、`persistence`、`test` 和 `gap`。

```powershell
node test/contract/implementation-scan.mjs `
  --legacy /www/apps/1Panel `
  --project . `
  --out .tmp/implementation-status.json
```

状态值：`implemented`（发现业务实现）、`partial`（实现但含固定空列表）、`compatibility`（兼容占位）、`pending`（迁移待完成）和 `missing`（未发现新路由）。脚本只读源码，不启动服务；报告中的启发式字段需结合对应 `evidence` 和源码复核。

## 隐藏能力扫描

`hidden-function-scan.mjs` 盘点旧 Core/Agent 的 `init`、`middleware`、`i18n`、`log` 和 `cron` 目录，检查隐藏能力清单章节是否仍存在，并可输出 JSON 报告：

```powershell
node test/contract/hidden-function-scan.mjs `
  --legacy /www/apps/1Panel `
  --project . `
  --out .tmp/hidden-function-status.json
```
