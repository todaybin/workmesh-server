<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# sp.sopvip.com 部署登记

状态：`[>] 进行中`

## 目标

为 `sp.sopvip.com` 建立与 `znmp.sopvip.com` 相同的宿主机 Go + Supervisor 部署模式，但使用独立的 PostgreSQL 数据库、数据库账号、密码和 Gateway `AccessSecret`。面板权威数据必须进入 SQLite，不能依赖 JSON 文件。

## 已完成

- `deploy/production/provision-sp-site.sh` 现在只是兼容包装器，所有正式操作转发到 API 部署脚本。
- API 脚本支持 `--apply --activate`；网站和 Go runtime 的管理状态由面板接口刷新，nginx 仅在最后按需 reload。
- API 脚本新增显式 `--recreate`：只按精确 ID/域名删除旧 `sp.sopvip.com` 网站、`sp.sopvip.com-gateway` runtime、`sp_sopvip_com` 数据库和 `sp-postgres` 面板登记，再按同一面板流程重建；默认 `--apply` 仍为幂等模式，历史归档不会删除。
- 新增 `deploy/production/provision-sp-site-api.py`：正式部署路径通过 `workmesh-server` 的 `/api/v2/databases/*`、`/api/v2/websites` 和 `/api/v2/runtimes` 接口创建资源；不再直接写 SQLite、Supervisor 配置或网站关系文件。
- SQLite 事务登记 `runtime_records`、`runtime_attributes`、`websites`、`website_domains`、`website_settings`、`databases`、`database_users` 和 `database_configs`。
- 新 runtime 为 `sp.sopvip.com-gateway`，类型 `go`、模式 `host`、端口 `8090`；网站按标准反向站点登记为 `type=proxy`，目标为 `http://127.0.0.1:8090`，不把反向站点误登记为 runtime 网站。
- `website_settings` 写入 `proxy:root`，并生成 `nginx/proxy/root.conf`，所以反向代理菜单可以读取完整代理配置。
- `sp.sopvip.com` 与 `znmp.sopvip.com` 使用两个完全不同的域名目录、网站记录、Go runtime、数据库实例登记、数据库名/账号/密码和 `AccessSecret`；`znmp` 仅作为应用代码/资源基线，不复制其运行时目录、数据库文件、日志、证书、回滚目录或其他站点状态。
- 应用制品先在 `.tmp` 临时目录组装，再通过面板 `/api/v2/files`、`/api/v2/files/upload`、`/api/v2/files/mode` 接口写入面板登记的 `sp.sopvip.com/app`；脚本不直接创建站点目录、不写 SQLite、Supervisor、nginx/WAF/SSL/logs，也不再生成根目录 `.secrets` 或 `app.backup.*`。
- 网站、nginx、WAF、日志、SSL 和 `.workmesh` 目录全部由创建网站的面板服务维护；Go runtime 通过面板登记为 `mode=host`，由面板负责 Supervisor 生命周期。
- PostgreSQL 管理密码和应用密码按面板数据库 API 契约以 Base64 传输（服务端再解码）；不再直接发送明文，避免密码被二次解码后导致数据库创建或连接失败。
- 生产执行前备份 SQLite/WAL/SHM；数据库管理连接必须由调用者通过 `PGADMIN_URL` 显式提供，脚本不在仓库保存密码。

## 验证证据

```bash
python3 deploy/production/provision-sp-site-api.py
```

在临时 SQLite 副本和临时站点目录执行 `--apply` 已回读确认：网站 `type=proxy`、域名、`proxy:root`、runtime、`mode=host`、端口 8090、独立数据库和数据库用户均已写入关系表；未触碰生产目录或生产 SQLite。

`GET /api/v2/containers/docker/status` 属于面板通用管理工具箱，只返回当前节点 Docker CLI/daemon 健康状态，不属于任何固定域名，也不是部署清单。此站点采用宿主机 Go 模式，面板数据应从 Go runtime 和网站反向代理页面查看，不应为该通用接口伪造域名专属 Docker 容器记录。

## 现场复核与阻塞

2026-09-22 复核结果（生产 SQLite 通过只读挂载读取，未写入生产数据）：

- `websites` 只有 `znmp.sopvip.com`，`runtime_records` 只有 `znmp.sopvip.com-gateway`；`databases` 和 `database_users` 当前为空。
- `/www/wwwroot/sp.sopvip.com` 不存在；`/etc/supervisor/conf.d/` 只有 `znmp.sopvip.com-gateway.conf` 及其历史备份。
- 当前沙盒访问不到生产 `127.0.0.1:9999`、Supervisor/Docker socket、PostgreSQL、systemd、DNS 和公网网络，因此没有执行 `--apply`，也没有直接改 SQLite、Supervisor、nginx 或站点目录。
- `sp.sopvip.com` 从用户侧仍可访问，说明实际响应来自当前面板快照之外的旧生产入口、其他网络命名空间或外部代理；在未确认归属前不能执行 `--recreate`，否则可能中断现有服务。

## 维护主机执行顺序

先只读确认旧入口：

```bash
ps -eo pid,ppid,user,etime,args | grep -E 'sp\.sopvip|workmesh-gateway|8090' | grep -v grep
ss -ltnp | grep ':8090'
supervisorctl status
docker ps --format '{{.Names}}\t{{.Image}}\t{{.Ports}}\t{{.Status}}'
grep -RInE 'sp\.sopvip|127\.0\.0\.1:8090' /etc/nginx /etc/openresty /etc/supervisor /opt /www 2>/dev/null
```

确认旧服务归属、备份生产 SQLite/WAL/SHM 和旧配置后，再使用独立凭据正式部署。当前面板快照没有 sp 资源，首次正式部署使用幂等模式，不需要 `--recreate`：

```bash
WORKMESH_API_TOKEN='面板令牌' \
PGADMIN_URL='postgresql://管理员:密码@实际PG主机:5432/postgres' \
WORKMESH_SP_DB_PASSWORD='独立应用密码' \
python3 /www/apps/workmesh-server/deploy/production/provision-sp-site-api.py \
  --apply --activate
```

只有在面板回读确认已经存在且目录、域名、runtime ID 均精确匹配的旧 sp 资源时，才追加 `--recreate`；公网旧入口归属未确认前不得追加。

完成后必须在面板回读 `sp.sopvip.com` 反向站点、`sp.sopvip.com-gateway` Go host runtime 和 `sp_sopvip_com` PostgreSQL 记录，并验证 Supervisor、Gateway `/health`、反向代理及 HTTPS。旧 SQLite 直写脚本仅保留为历史 dry-run 证据，不作为正式部署入口。
