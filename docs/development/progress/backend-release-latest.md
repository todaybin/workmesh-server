<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 后端优先发布记录

状态：`[x]` 最新后端版本已通过统一门禁并完成原子部署；登录后真实业务验收仍按矩阵保留未覆盖项。

## 本次后端改动

- 应用商店和容器状态在接入共享 SQLite 后不再直接读取旧版 `apps.json`、`containers.json`；旧 JSON 只保留为首次迁移/无数据库测试输入。
- 计划任务按执行、调度、记录、传输职责拆分，保持 `/api/v2/cronjobs` 参数、响应和 SQLite 记录格式。
- CLI 启动代码按分发、帮助、用户、设置、存储、制品职责拆分，单文件均低于 500 行。
- 未修改 `apps/1Panel/frontend`；该目录仅作为只读接口和菜单参考项目。

## 发布制品

| 项目 | 结果 |
| --- | --- |
| 二进制 | `/opt/workmesh-server/bin/workmesh-server` |
| SHA-256 | `bfd9f0d6dd74a0133774e5081e2f7f43a0a49cb8d7656311390457c64f6d21c9` |
| 备份目录 | `/opt/workmesh-server/backups/deploy-20260906T155453+0800-migration-audit` |
| 部署方式 | 原子替换后重启 systemd 单进程 |

备份包含旧二进制、`server.json`、`server.env`、SQLite 主库及 WAL/SHM 文件（存在时）。

## 已执行证据

- `GOWORK=off go test ./...`：宿主机网络命名空间执行通过。
- `GOWORK=off go test -race ./...`：通过，未发现 race。
- `GOWORK=off go vet ./...`：通过。
- `git diff --check`：通过。
- `node --test test/contract/*.test.mjs`：7/7 通过。
- `node test/contract/route-scan.mjs check ...`：759 条路由通过，191 条扩展路由兼容允许。
- `GET /health`：HTTP 200，`status=ok`。
- `GET /ready`：HTTP 200，`status=ready`。
- `systemctl is-active workmesh-server.service`：`active`，`NRestarts=0`。
- OpenResty `nginx -t`：syntax/test successful；容器 `nofile=65536`，无 worker_connections 资源警告。
- `Host: znmp.sopvip.com` 经 OpenResty 80 端口访问：HTTP 200。
- 网站启动恢复：空 `znmp.sopvip.com/nginx/site.conf` 已从 SQLite/站点类型恢复为可加载配置。
- 历史 stream 占位清理：`types-stream.example` 的 `127.0.0.1:9` 已备份并替换为注释文件。
- HTTPS 关联：`znmp.sopvip.com` 已关联 SQLite 证书记录 `website_ssl_id=1`，443 TLS server 通过真实域名访问验证。
- 2026-09-06 15:22 发布后核对：新制品 systemd `active`、`NRestarts=0`，`MainPID=2552852`；`/health`、`/ready`、HTTP/HTTPS 均返回成功；SQLite `quick_check=ok`、`integrity_check=ok`、外键违规 0；OpenResty `nginx -t` 成功；运行配置未发现空 `proxy_pass` 或 `127.0.0.1:9` 占位。
- 2026-09-06 之后补充 OpenResty 容器资源限制：备份并更新 `/opt/workmesh-server/openresty-waf/docker-compose.yml`，将 `nofile` soft/hard 设置为 `65536`；容器重建后 `ulimit -n=65536`，`nginx -t` 无资源警告，`znmp.sopvip.com` HTTP/HTTPS 均 HTTP 200。
- 2026-09-06 15:55 发布迁移审计修复：启动后 `migration_runs=4`，最新记录为 `status=noop`、`to_version=0013-log-audit-v2`、制品 SHA 长度 64、备份根目录 `/opt/workmesh-server/backups`；`quick_check`、`integrity_check` 和外键检查均通过。

## 尚待最后确认

- 质量报告需在后续质量债务批次完成后重新生成；若仍有历史注释违规，保留 `fail`，不能用批量空注释掩盖。
- 没有有效管理员会话、次节点凭据、Docker/ACME 外部资源时，登录后 385 条 HTTP、4 条 WS、六类运行环境、主次节点和 Let's Encrypt HTTP-01 继续标记 `not-run/blocked`。
