<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-08-30 第二批迁移验收

## 制品

- 目标平台：Linux amd64
- SHA256：`cc994e9d09f806468c2d9fb7ef4e6f64d3fb167db5ba2ed98f854a17c824a0e0`
- 主节点：`/opt/workmesh-server/bin/workmesh-server`
- 次节点：`/opt/workmesh-server-secondary/bin/workmesh-server`
- 两台节点二进制摘要一致，替换前均已生成带 UTC 时间戳的备份文件。

## 功能验收

| 功能 | 验证 | 结果 |
| --- | --- | --- |
| 快捷命令创建/搜索/删除 | 主节点 POST `/api/v2/core/commands`、`search`、`del` | 真实写入并清理成功 |
| 计划任务创建 | 主节点 POST `/api/v2/cronjobs` | 返回持久化任务 ID 和时间戳 |
| 计划任务执行记录 | `handle` 后查询 `search/records` | 记录 stdout、stderr、退出码和耗时 |
| 次节点服务 | SSH 本机访问 `127.0.0.1:9999/health` 与命令搜索 | HTTP 200 |

## 自动化验证

```powershell
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test ./..."
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go vet ./..."
node scripts/with-dev-env.mjs -- node apps/workmesh-server/test/contract/route-scan.mjs check --legacy apps/workmesh-node --project apps/workmesh-server --manifest apps/workmesh-server/test/contract/routes.json
node scripts/with-dev-env.mjs -- node apps/workmesh-server/test/contract/implementation-scan.mjs --legacy apps/workmesh-node --project apps/workmesh-server --out .tmp/implementation-status.json
```

实现状态报告：831 条路由，`implemented 126`、`partial 13`、`compatibility 5`、`pending 682`、`missing 5`。剩余状态必须继续逐条迁移，当前不能宣称全量功能完成。

## 未完成外部条件

- Gateway 节点注册仍缺少真实账号/授权凭据，状态保持 `pending`。
- 主节点到次节点 `9999` 端口仍受安全组阻断，跨机链路暂未做 E2E。
- 主节点无 `/dev/kvm`，CubeSandbox 只能按 `degraded/restricted` 验收。

