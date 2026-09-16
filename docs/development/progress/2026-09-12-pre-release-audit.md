<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 上线前整体审计清单（2026-09-12）

状态：`[!] blocked`。本轮只读审计 `/www/apps/workmesh-server`，没有修改业务代码、生产
`/opt`、`/www/wwwroot` 或 `/www/apps/workmesh-node`。`/www/apps/1Panel` 仅作为只读业务参考。

## 审计依据

- `docs/development/STATUS.md`
- `docs/development/progress/2026-09-12-deployment-domain-env-directory-audit.md`
- `docs/development/progress/2026-09-12-deployment-current.md`
- `docs/development/progress/2026-09-12-site-reconcile-current.md`
- `docs/development/progress/2026-09-12-waf-image-page-audit.md`
- `docs/development/progress/2026-09-12-waf-follow-up.md`
- `docs/development/progress/2026-09-10-database-containerization.md`
- `docs/development/progress/2026-09-08-acme-acceptance.md`
- `deploy/acceptance/README.md`
- `deploy/database/README.md`

本轮复跑只读命令：

```text
STRICT=0 bash deploy/acceptance/preflight.sh
summary: warnings=5 failures=1

STRICT=0 bash deploy/database/preflight.sh
summary: warnings=4 failures=0

WORKMESH_SQLITE_DB=/opt/workmesh-server/data/workmesh.db \
WORKMESH_WEBSITE_ROOT=/www/wwwroot \
bash deploy/acceptance/site-reconcile.sh --out /tmp/workmesh-site-reconcile-audit-current-<pid>
summary: warnings=0 mismatches=19
sqlite_websites=0
sqlite_domains=0
filesystem_site_confs=19
orphan_site_confs=19
```

## 上线阻塞优先级

| 优先级 | 任务 | 当前真实状态 | 未完成内容 | 放行标准 |
| --- | --- | --- | --- | --- |
| P0 | 修复生产预检硬失败 | `preflight.sh` 当前 `warnings=5 failures=1` | `/opt/workmesh-server/openresty-waf/docker-compose.yml` 仍引用旧 `workmesh/openresty-waf:20260904`，且生产 WAF 挂载缺 `generated/standard-rules.conf`；Docker socket 不可访问、域名未能解析、浏览器依赖缺失 | 旧 WAF 镜像引用替换为当前仓库构建并固定 digest 的镜像；`standard-rules.conf` 存在；`STRICT=1 deploy/acceptance/preflight.sh` 通过 |
| P0 | Docker/OpenResty 真实环境测试 | 当前只有 Docker CLI/Compose，daemon/socket 不可访问；本机无 `openresty/nginx` | 不能执行容器内 `nginx -t`、OpenResty reload、reload 后复检、WAF 请求和数据库 Compose 真实生命周期 | 在目标环境保存 Docker info、容器 `nginx -t`、reload、健康检查、WAF block/observe 请求和失败回滚证据 |
| P0 | 站点数据库与目录对账 | SQLite `websites=0`、`website_domains=0`，但 `/www/wwwroot` 有 19 个 `nginx/site.conf` | 19 个文件系统站点均未匹配 SQLite `site_dir`；`znmp.sopvip.com` 需要优先确认是否生产站点；未授权前不得删除、移动、导入或 reload | 每个 `site.conf` 有归属结论；生产站点通过 API/迁移恢复 SQLite 记录或重建；测试残留经授权清理；`site-reconcile.sh --strict` 通过 |
| P0 | 域名部署测试 | 已指定域名 `workmesh.cs.sopvip.com`，期望公网 IPv4 为 `61.184.12.165`；当前受限网络无法可靠解析 | DNS A/AAAA、Host 路由、HTTP 80、HTTPS 443、SNI 证书、站点 `server_name` 未形成同一份真实证据 | 从公网和目标主机分别确认 DNS；使用 `curl --resolve` 验证 Host 路由、HTTP/HTTPS、响应头、证书 SAN 和站点配置一致 |
| P0 | ACME/证书真实验收 | 代码和脚本具备，真实签发仍 `not-run` | 未执行 Let's Encrypt staging/production HTTP-01、DNS-01、手动 TXT finalize、续期和失败回滚 | challenge 文件公网可读；TXT 写入/传播/清理可证；证书、私钥、SQLite、站点 HTTPS 配置和 OpenResty reload 一致 |
| P1 | 数据库镜像/Compose 生产部署 | 模板和隔离黑盒已有；生产预检 `warnings=4 failures=0` | `/opt/workmesh-server/data/database/secrets` 和运行 Compose 不存在；daemon 不可用；固定容器/网络/卷冲突不能检查；模板当前使用 `postgres:18-alpine`、`redis:8-alpine`、`mariadb:11` tag，尚无本次发布的镜像 digest 清单 | secret 四个文件非空且 `0600`；运行 Compose `config --quiet` 通过；记录并核对三个实际镜像 digest；`workmesh-panel-postgres`、`workmesh-panel-redis`、`workmesh-panel-mariadb` 健康；备份/恢复/回滚通过；旧 ZNMP 容器不受影响 |
| P1 | WAF 镜像重新构建与发布 | 当前仓库已包含 `deploy/openresty-waf/`、运行时契约和发布静态门禁 | 本环境无法 build/push；生产仍用旧 `20260904`；没有新镜像 digest、运行容器健康和请求矩阵证据 | 使用当前仓库 Dockerfile 构建新镜像；`image-release-contract.sh` 通过；部署文件绑定不可变 digest；容器启动后生成 `generated/*.conf` 且 WAF 请求矩阵通过 |
| P1 | 站点测试 | 隔离黑盒历史通过，但当前生产 SQLite 与目录不一致 | 静态、反代、运行时、PHP、部署、子站点、TCP/UDP 需要在当前目标环境用真实域名和真实上游重建证据 | 创建、访问、日志、配置、停止/恢复、删除清理全链路均有 HTTP、SQLite、`site.conf`、OpenResty 和文件系统证据 |
| P1 | WAF 七页页面与保存后生效验收 | 7 张图对应 7 个页面和 API 链；但当前缺浏览器自动化依赖 | 浏览器截图、交互保存、保存后 `effective`、真实请求结果尚未在当前环境串起来 | 对 7 个 WAF 路由逐页截图；每个可写设置保存后确认 `effective=true`，并通过实际 OpenResty/WAF 请求验证 |
| P2 | 目录结构物理迁移 | repository/事务边界和职责拆分已有，顶层物理目录仍未整体迁移 | `control`、`node`、`runtime` 仍是生产兼容入口；website/runtime 剩余资源状态和兼容层仍待拆 | 完成 P0/P1 真实资源验收后再分批迁移；每批保持 759 路由、389 前端调用、systemd 工作目录、数据目录和发布回滚不变 |
| P2 | 全量登录后 HTTP/WS 与主次节点 | 路由扫描显示 759/759 implemented，但 206 条功能域路由仍是 `not-run` | 管理员会话、CSRF、4 条 WS、主次 Gateway、跨节点任务透传未全部实测 | 登录后保存每条关键 API/WS 的状态码、脱敏响应、副作用、SQLite/日志证据；主次节点完成心跳、断线恢复、签名和 epoch/fencing |
| P2 | 发布与回滚演练 | 清单存在，生产替换未执行 | 候选制品验签、SQLite/WAL/SHM 备份、隔离迁移两次 noop、systemd 原子替换、失败回滚未形成当前证据 | 发布清单每项都有时间戳、摘要、日志和回滚结果；运行二进制 SHA-256 与候选一致 |

## 证据冲突与取舍

1. 较早网站文档曾记录 `znmp.sopvip.com` 在 SQLite 中有站点记录；最新只读对账显示当前
   `/opt/workmesh-server/data/workmesh.db` 中 `websites=0`、`website_domains=0`。上线判断以最新
   `site-reconcile.sh` 结果为准，先查明数据库路径、迁移或清理差异。
2. WAF 跟进文档有“七张参考图逐页截图对齐完成”的表述；同期 `STATUS.md` 和图片审计仍明确
   缺浏览器/Playwright/Puppeteer，无法提供当前运行页面截图证据。上线判断以缺截图验收为准。
3. 路由扫描 `implemented` 只表示处理器存在，不表示真实业务通过；域名、数据库、WAF、ACME、
   OpenResty reload 和登录后 WS 必须保留 `not-run/blocked`。

## 建议下一步执行顺序

1. 先处理 P0 预检硬失败：构建并固定当前仓库 WAF 镜像 digest，准备 `standard-rules.conf`，
   让 `STRICT=1 deploy/acceptance/preflight.sh` 从硬失败变为可进入维护窗口。
2. 在有 Docker socket 的目标环境执行数据库预检，创建生产 secret 和运行 Compose 文件，但先只跑
   `config --quiet`，不启动容器。
3. 完成 19 个 `site.conf` 归属分类，优先确认 `znmp.sopvip.com` 是否生产站点；没有人工确认前
   不做 OpenResty reload、删除或导入。
4. 配置并验证 `workmesh.cs.sopvip.com -> 61.184.12.165`，再执行域名、HTTPS、ACME 和 WAF 请求矩阵。
5. 数据库容器 `up -d`、健康检查、备份/恢复和回滚通过后，再做 WorkMesh Server 发布/回滚演练。
6. P0/P1 全部有当前环境证据后，再推进目录物理迁移和登录后 206 条 `not-run` 路由/WS 的全量黑盒。

## 当前结论

不能上线。真正阻塞不是缺少某个前端页面或单个接口，而是当前目标环境仍缺可复核的
Docker/OpenResty、域名、站点目录一致性、ACME、数据库 Compose 和发布回滚证据。
代码侧已有大量实现和隔离测试，但这些不能替代生产现场验收。
