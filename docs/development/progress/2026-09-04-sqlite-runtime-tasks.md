<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# SQLite 运行时任务化

更新时间：2026-09-04

## 状态

- [x] 运行时任务状态、阶段进度和任务日志写入共享 SQLite。
- [x] 任务日志接口优先从 SQLite 读取，进程重启后不依赖日志文件。
- [x] Docker 状态同步不会覆盖带有后台任务上下文的 Creating/Building/Starting 等状态。
- [x] 角色 fencing epoch、节点清单、Gateway 绑定和 Link SyncStore 提供 SQLite 实现。
- [x] 核心 Passkey、分组和设置支持 SQLite 恢复。

## 验证

- `GOWORK=off go test ./cmd/workmesh-server ./control/... ./runtime/...` 通过。
- `GOWORK=off go vet ./...` 通过。
- `GOWORK=off go test ./...` 暴露既有 `node/api` Supervisor 临时路径和网站日志默认目录用例失败。

## 后续

仍需将其余功能域的 JSON payload 表拆分为明确关系字段，并完成真实 Docker、Gateway 和 ACME 环境验收。

## 2026-09-04 SSL/DNS 兼容部署

- [x] `/api/v2/files/read/ssl` 按 1Panel 的 `ID/page/pageSize` 请求读取 SQLite 证书对应的 SSL 日志，不再要求请求携带 `path`；支持 `id`、`ID`、`sslID` 和 `websiteSSLId` 别名。
- [x] `/api/v2/websites/dns` 与 `/api/v2/websites/dns/update` 同时兼容 1Panel 的 `type/authorization` 及旧版 `provider/credentials` 字段，凭据只保存到 SQLite 且不在响应中回显。
- [x] 新增 SSL 日志和 DNS 字段回归测试；`go vet ./...`、前端 `type-check`、`build:pro` 通过。
- [x] 生产部署时间：2026-09-04 19:51 CST；二进制 SHA256=`09b3ea6c390fe3c3a8b410db1928477958993dba12ad724c8caba0e771ea55fb`；备份目录 `/opt/workmesh-server/backups/deploy-20260904T195133-frontend`（前端）及 `/opt/workmesh-server/backups/deploy-20260904T194932-ssl-dns`（后端）。服务重启后 `active`，`/health`、`/ready` 返回 200。
- [x] DNS 编辑表单的脱敏 `authorization:{}` 结构修复已于 2026-09-04 19:55 CST 部署；当前二进制 SHA256=`f9f3f397b75ce07bc69d783d4ea621e9442c069710eb8580e88f3e30bd526823`，备份目录 `/opt/workmesh-server/backups/deploy-20260904T195523-ssl-dns-v2`；服务保持 `active`，`/health`、`/ready` 返回 200。
- [x] ACME HTTP-01 webroot 执行器已接入：`/api/v2/websites/ssl/obtain` 对 `provider=http` 异步调用 certbot，状态、证书和日志写回 SQLite/受控文件资源；自动续期扫描复用同一执行器。
- [x] ACME 执行器部署时间：2026-09-04 20:10 CST；当前二进制 SHA256=`78d5912335f363e2e779c8afdefdc29c773e60b4f2a728b9accf8361d9125731`，备份目录 `/opt/workmesh-server/backups/deploy-20260904T201051-acme-obtain`；服务保持 `active`，`/health`、`/ready` 返回 200。
- [x] 后台调度启动条件修复已于 2026-09-04 20:15 CST 部署：只要后台任务总开关开启，即使没有普通 Cronjob 也会运行 SSL 自动续期扫描；当前二进制 SHA256=`75bb81fa62f38adf4f57c5fea2e7c80fd0a2991e93de6039ce4b248d0bcffc6a`，备份目录 `/opt/workmesh-server/backups/deploy-20260904T201532-acme-scheduler`。
- [x] 定时周期已按 1Panel 的 `0 */6 * * *` 对齐并于 2026-09-04 20:39 CST 部署；当前二进制 SHA256=`a98e1f84ce4271fbe5e07ef3c85921da4bd8500ca961abc9dbc6ace8cc5064d0`，备份目录 `/opt/workmesh-server/backups/deploy-20260904T203952-schedule-1panel-parity`。WorkMesh 未引用或调用 `/www/apps/1Panel`。
- [x] SSL 编辑、推送和上传页面对缺失或非字符串 `nodes` 字段做类型保护，后端 `WebsiteSSL.nodes` 稳定返回空字符串；前端 `type-check`、`build:pro` 通过。
- [x] 2026-09-04 21:56 CST 已部署该修复：二进制 SHA256=`eaf1ae06b9630fbf4bbcbc7eba8cb04ef7e45709752a8349181d8d3f8bd54f3f`，备份目录 `/opt/workmesh-server/backups/deploy-20260904T-ssl-nodes-fix`；服务、`/health`、`/ready` 均正常。
- [x] `znmp.sopvip.com` 的实际 OpenResty `nginx/site.conf` 为空导致 HTTP-01 返回 404，已补齐 `server_name`、站点 root 和 `app/.well-known/acme-challenge` alias；本机及公网 challenge 读取验证通过。
- [x] 2026-09-04 22:12 CST 通过 `/api/v2/websites/ssl/obtain` 真实申请成功：证书状态 `ready`，Let’s Encrypt 证书有效期至 2026-12-03；随后部署空配置初始化修复，二进制 SHA256=`4cbbc675b9a2e00fb1c5d7afc3254c025a6e9635376fffbba03de1844fe65eba`，备份目录 `/opt/workmesh-server/backups/deploy-20260904T-acme-site-fix`。
- [ ] DNS-01 各云厂商插件尚未安装；DNS 账户申请会返回明确的服务不可用错误，待安装对应 certbot 插件或接入原 1Panel lego provider 后启用。
