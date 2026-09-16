<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 候选版本部署后回归记录

本记录只包含实际执行的命令和响应，不把静态路由清单或未携带凭据的边界测试扩大解释为完整业务验收。

## 制品与备份

- 当前生产二进制：`/opt/workmesh-server/bin/workmesh-server`
- SHA-256：`818cfae4341fd515d204325634d97d6c22d80a852189932af8ac6521a23489a0`
- 最近备份：`/opt/workmesh-server/backups/deploy-20260905T235900+0800-sqlite-legacy-fix`
- 备份内容：旧二进制、`server.json`、`server.env`、SQLite 在线备份。

## 服务与站点

| 检查 | 实际结果 |
| --- | --- |
| `systemctl is-active workmesh-server.service` | `active` |
| `NRestarts` | `0` |
| `GET http://127.0.0.1:9999/health` | HTTP 200，`status=ok` |
| `GET http://127.0.0.1:9999/ready` | HTTP 200，`status=ready` |
| `Host: znmp.sopvip.com` HTTP 入口 | HTTP 200 |
| `docker exec workmesh-openresty-waf nginx -t` | syntax/test successful |

## 自动化门禁

- `GOWORK=off go test ./...`：通过。
- `GOWORK=off go test -race ./...`：通过；此前发现的应用安装任务共享 `Config` map race 已修复。
- `GOWORK=off go vet ./...`：通过。
- `git diff --check`：通过。
- `node --test test/contract/*.test.mjs`：7/7 通过。
- `node test/contract/http-smoke.mjs`：8/8 通过。
- `WORKMESH_SCOPE=boundary node test/contract/frontend-contract-executor.mjs`：7/7 通过。
- `node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest docs/inventory/route-inventory-1panel.json`：759 条路由契约通过，191 条扩展路由兼容允许。

## SQLite 运行库

使用只读连接检查 `/opt/workmesh-server/data/workmesh.db`：

- 表数量：63。
- `operation_logs`：2024 条。
- `runtime_records`：6 条。
- `runtime_tasks`：31 条。
- `PRAGMA integrity_check`：`ok`。

## 尚未完成的验收

当前环境没有有效管理员 Cookie/Token、次节点凭据或外部 Docker/ACME 测试资源，因此以下仍不能标记通过：登录后的 385 条 HTTP、4 条 WS 业务消息序列、六类运行环境生产生命周期、网站全部设置、主次节点 Gateway、HTTP-01 续期，以及 1Panel 759 条路由的逐接口参数/响应/副作用闭环。它们继续在接口矩阵中保持 `not-run` 或 `blocked`，不使用模拟数据填充。
