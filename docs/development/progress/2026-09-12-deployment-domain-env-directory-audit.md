<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 部署、域名、环境测试和目录结构审计（2026-09-12）

状态：`[!]` 只读审计已完成，生产写入和真实公网验收仍被外部资源阻断

## 审计范围

本次只审计 `/www/apps/workmesh-server` 内部署、域名、环境测试和目录结构相关文档/脚本，并执行只读命令。当前主项目的前端和后端均位于 `/www/apps/workmesh-server`，业务行为只参考只读 `/www/apps/1Panel`。未修改生产 `/opt`，未删除或改写 `/www/wwwroot` 内容。

边界结论：`/www/apps/workmesh-server` 是唯一主项目；不得把已移除的 `workmesh-node`
作为当前前端、后端、部署、WAF 或测试来源。

覆盖入口：

- `deploy/acceptance/preflight.sh`
- `deploy/acceptance/readonly-evidence.sh`
- `deploy/acceptance/site-reconcile.sh`
- `deploy/database/preflight.sh`
- `deploy/database/acceptance.sh`
- `deploy/install/install.sh`
- `deploy/openresty-waf/`
- `docs/operations/waf-domain-acceptance.md`
- `docs/architecture/onion-migration-2026-09-05.md`
- `docs/development/STATUS.md`
- `docs/development/progress/2026-09-11-waf-page-runtime.md`
- `docs/development/progress/2026-09-12-waf-follow-up.md`

## 本次只读证据

```text
deploy/acceptance/preflight.sh
warnings=5 failures=1
```

当前预检结论：

- Docker CLI 和 Compose 可用，但 Docker daemon/socket 不可访问。
- `/opt/workmesh-server/openresty-waf/waf/generated/standard-rules.conf` 缺失。
- 验收域名已确定为 `workmesh.cs.sopvip.com`，期望公网 IPv4 为 `61.184.12.165`；当前受限网络仍无法完成 DNS 和 Host 路由验证。
- 旧生产 WAF 镜像 `workmesh/openresty-waf:20260904` 被严格预检判定为不可发布。
- 缺少 Chromium/Chrome/Firefox 与 Playwright/Puppeteer，无法执行页面渲染截图验收。
- ACME 基础工具 `curl`、`openssl`、`certbot` 和 `/etc/letsencrypt` 存在。

```text
deploy/database/preflight.sh
warnings=4 failures=0
```

当前数据库预检结论：

- Docker daemon/socket 不可访问。
- `/opt/workmesh-server/data/database/secrets` 不存在。
- `/opt/workmesh-server/data/database/docker-compose.yml` 不存在。
- 由于没有 daemon，固定容器、网络、卷名称和旧 ZNMP 资源可见性无法复核。

```text
deploy/acceptance/site-reconcile.sh --out /tmp/workmesh-site-reconcile-audit-20260912
sqlite_available=1
sqlite_websites=0
sqlite_domains=0
filesystem_site_confs=19
warnings=0
mismatches=19
```

当前站点对账结论：

- Go SQLite fallback 可读取 `/opt/workmesh-server/data/workmesh.db`，不依赖系统 `sqlite3`。
- SQLite `websites` 和 `website_domains` 当前均为 0 行。
- `/www/wwwroot` 下发现 19 个 `nginx/site.conf`。
- 19 个 `site.conf` 均无法匹配 SQLite `site_dir`，属于待人工分类的孤立文件系统站点配置。
- 本次没有删除、移动或改写任何站点目录。

发现的孤立 `site.conf`：

```text
/www/wwwroot/advanced.example/nginx/site.conf
/www/wwwroot/aliases.example/nginx/site.conf
/www/wwwroot/example.com/nginx/site.conf
/www/wwwroot/example.org/nginx/site.conf
/www/wwwroot/extended.example/nginx/site.conf
/www/wwwroot/legacy-domain.example/nginx/site.conf
/www/wwwroot/resource.example/nginx/site.conf
/www/wwwroot/static.example/nginx/site.conf
/www/wwwroot/types-deployment.example/nginx/site.conf
/www/wwwroot/types-php.example/nginx/site.conf
/www/wwwroot/types-proxy.example/nginx/site.conf
/www/wwwroot/types-runtime.example/nginx/site.conf
/www/wwwroot/types-static.example/nginx/site.conf
/www/wwwroot/types-stream.example/nginx/site.conf
/www/wwwroot/types-subsite.example/nginx/site.conf
/www/wwwroot/upgrade.example/nginx/site.conf
/www/wwwroot/valid.example/nginx/site.conf
/www/wwwroot/waf-test.example/nginx/site.conf
/www/wwwroot/znmp.sopvip.com/nginx/site.conf
```

## 未完成任务

| 领域 | 当前状态 | 未完成内容 | 完成标准 |
| --- | --- | --- | --- |
| 生产 OpenResty/WAF | `[!] blocked` | 当前环境无 Docker socket、本机无 `openresty/nginx`，无法执行真实 `-t`、reload、reload 后复检和 WAF 请求阻断 | 在可访问 OpenResty 的环境保存 `nginx -t`、reload、健康请求、攻击请求和失败回滚证据 |
| 域名部署测试 | `[!] not-run` | 已指定隔离域名 `workmesh.cs.sopvip.com` 和期望公网 IPv4 `61.184.12.165`；当前受限环境仍无可靠 DNS 解析和公网链路证据 | DNS A/AAAA、Host 路由、HTTP 80、HTTPS 443、SNI 证书和站点 `server_name` 全部一致 |
| ACME/证书 | `[!] not-run` | HTTP-01、DNS-01、续期和失败回滚仍未在真实域名执行 | Let's Encrypt staging 或授权域名完成签发/续期，证书文件、SQLite、站点 HTTPS 配置一致 |
| 数据库镜像/Compose | `[!] blocked` | 模板和隔离验收脚本存在，但生产运行目录、secret 和 Docker daemon 不可用 | secret `0600`、runtime Compose `config --quiet`、三个容器健康、备份恢复和回滚通过 |
| WAF 镜像构建/发布 | `[!] not-run` | 当前仓库已具备 `deploy/openresty-waf/` Dockerfile、Compose、入口脚本和静态发布门禁，但本环境无 Docker daemon，未完成新镜像 build、digest 固化和部署 | 在隔离环境通过镜像静态门禁、构建期 `nginx -t`、运行时 `nginx -t`、健康检查和 WAF 请求矩阵 |
| 站点目录对账 | `[!] incomplete` | SQLite 站点记录为 0，但文件系统存在 19 个 `site.conf` | 逐个确认是测试残留、历史导入对象还是生产站点；获得授权后执行导入或清理方案 |
| 前端页面渲染验收 | `[!] blocked` | 缺少浏览器和 Playwright/Puppeteer，无法在本机跑截图/页面交互验收 | 安装浏览器自动化依赖后，对 WAF 七页和域名/站点设置页面执行截图与交互保存验证 |
| 目录结构迁移 | `[>] in-progress` | 已完成职责拆分和 repository 边界，顶层物理目录仍保留 `control`、`node`、`runtime` 兼容壳 | 在真实资源验收稳定后，按洋葱架构分批移动包并保持 759 路由、前端调用和发布路径不变 |
| 登录后 HTTP/WS | `[!] not-run` | 公开 smoke 和边界测试已有，登录后 389 条前端 HTTP/WS 调用未全部实测 | 管理员会话下记录状态码、脱敏响应、副作用、SQLite/OpenResty/文件证据 |
| 主次节点/Gateway | `[!] blocked` | Gateway 凭据、节点授权材料和跨机链路仍未提供 | 注册、授权刷新、心跳、断线恢复、主次角色切换和回滚均有证据 |

## 当前文档范围内的下一步

本轮只做文档审计，不执行代码、脚本或生产变更。后续执行顺序：

1. 先补齐 Docker/OpenResty、`workmesh.cs.sopvip.com -> 61.184.12.165` 的 DNS、公网链路、ACME staging、登录会话和浏览器自动化依赖。
2. 严格执行 `preflight.sh`、`deploy/database/preflight.sh` 和 `site-reconcile.sh --strict`；任一未通过，不进入生产 reload 或目录处理。
3. 对 19 个孤立 `site.conf` 完成逐项分类，人工确认导入 SQLite 或授权清理。
4. 完成七页截图/交互验收、域名/HTTPS/ACME/WAF 请求矩阵、数据库 Compose 和发布回滚证据。
5. 上述真实资源验收通过后，才推进物理目录迁移；每批迁移必须复跑路由数、前端调用清单、systemd 工作目录和生产数据目录门禁。

## 需要授权后才能执行的任务

以下任务会触碰真实外部资源或生产状态，本轮不能执行：

1. 通过当前仓库 `deploy/openresty-waf/` 的发布流程生成或覆盖 `/opt/workmesh-server/openresty-waf/waf/generated/standard-rules.conf`。
2. 对生产 OpenResty 执行 reload、restart 或容器重启。
3. 启动、停止、删除或重建数据库容器、网络、卷。
4. 写入 DNS TXT/A/AAAA 记录或执行真实证书签发。
5. 导入、移动、删除 `/www/wwwroot` 下的 19 个孤立站点配置。
6. 切换 systemd、替换生产二进制或关闭旧回滚窗口。

## 下一步顺序

1. 先补只读验收文档和导出器测试，确保后续接手者不会把当前 19 个孤立站点误当成已入库站点。
2. 准备隔离验收环境：Docker socket、OpenResty/WAF 容器、隔离域名、期望公网 IP、ACME staging 账户、浏览器自动化依赖。
3. 执行 `STRICT=1 deploy/acceptance/preflight.sh`、`STRICT=1 deploy/database/preflight.sh` 和 `deploy/acceptance/site-reconcile.sh --strict`；三者未通过前不进入生产维护窗口。
4. 对 19 个孤立 `site.conf` 逐个分类，形成“导入 SQLite”或“授权清理”的人工确认清单。
5. 通过 WAF/域名/数据库真实验收后，再推进物理目录迁移，避免用目录搬迁掩盖未完成的生产闭环。
