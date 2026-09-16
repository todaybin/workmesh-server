<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 网站环境上线基础（2026-09-06）

状态：`[>]` 网站运行基础已部署；登录后全量业务和外部证书资源仍需真实验收。

## 已完成

- 网站元数据、域名、网站设置、WAF 和证书元数据统一使用 SQLite；旧 JSON 仅作为一次性迁移来源。
- 服务启动时从 SQLite 恢复运行站点的空/缺失 `site.conf`、`stream.conf` 和结构化设置；停止站点不会因重启自动启用。
- 网站根目录保持 `WORKMESH_WEBSITE_ROOT` 和现有 `/www/wwwroot/{domain}` 规则，不改变自定义 WAF 挂载。
- 主域名改名同步实际目录、`server_name`、日志/证书绝对路径和 `website_domains` 记录，并支持失败回滚。
- TCP/UDP 站点支持真实端口、协议、算法、上游、权重、失败参数和最大连接数配置；未配置真实上游时只生成注释，不写入假代理。
- 清理当前 OpenResty include 范围内的历史 `proxy_pass 127.0.0.1:9` 残留；原文件已备份。
- `provider=http`/`letsencrypt` 使用真实 certbot HTTP-01 webroot，签发结果经过证书域名、公钥、有效期校验后才写入 SQLite。
- 已将现有有效 Let’s Encrypt 证书关联到 `znmp.sopvip.com`，SQLite 网站记录为 HTTPS，OpenResty 443 TLS server 已加载并通过真实域名访问验证。
- 证书申请失败会落为 `error` 并记录日志，证书、ACME、CA 删除与 SQLite 索引保持一致。
- 默认页面 `404`、`domain404`、`index`、`php`、`stop` 已从扩展状态中拆出，使用独立的 `website_default_html` SQLite 表；更新时可按请求的 `sync` 标志同步到已登记站点的真实 `app` 文件，并在文件写入失败时回滚。
- 模板 ZIP 上传已使用真实 multipart 文件落盘，保存到 `$WORKMESH_DATA_DIR/templates/files/`，校验压缩包大小、条目数量和文本扫描量；无效上传不会返回伪成功。

## 自动化与发布证据

- `GOWORK=off go test ./...`：通过。
- `GOWORK=off go test -race ./...`：通过。
- `GOWORK=off go vet ./...`：通过。
- Node 契约：7/7 通过。
- 1Panel 路由契约：759/759 通过，191 条扩展路由兼容允许。
- SQLite：`quick_check=ok`、`integrity_check=ok`、外键违规 0。
- systemd：`active`，`NRestarts=0`。
- `/health`、`/ready`：HTTP 200。
- OpenResty `nginx -t`：通过；发布后已将容器 `nofile` 提升到 `65536`，不再出现 `worker_connections` 资源限制警告。
- `Host: znmp.sopvip.com`：HTTP 200。
- `https://znmp.sopvip.com/`：HTTP 200，TLS 证书 CN=`znmp.sopvip.com`，有效期至 2026-12-03。

本次制品 SHA-256：

`b54a9bf395e0a9f2766a944390dca149baa76260bb24a95d9bf7369ac281f45b`

备份目录：

`/opt/workmesh-server/backups/deploy-20260906T152227+0800-website-acme`

HTTPS 关联补充备份：

`/opt/workmesh-server/backups/deploy-20260906T152227+0800-website-acme/https-enable`

其中保存了 HTTPS 关联前后的 SQLite、原始站点配置和证书副本。当前 SQLite 记录为
`znmp.sopvip.com -> HTTPS -> website_ssl_id=1`，服务重启后仍保持该状态。

OpenResty 资源补充：已备份并更新 `/opt/workmesh-server/openresty-waf/docker-compose.yml`，容器 `nofile=65536`；重建后 `nginx -t` 成功且 HTTP/HTTPS 入口仍返回 200。

## 尚未完成

- 真实公网 HTTP-01 签发与续期；需要 certbot、DNS 指向当前服务器和公网 TCP 80。
- HTTPS 入口已恢复并通过；后续仍需验证证书自动续期后的站点配置同步。
- 运行环境未就绪时，反向代理/运行时网站返回 502 属于真实上游未启动，不伪造成功。
- 终端登录后 WS、六类运行环境生命周期、主次节点和全量网站设置仍按测试矩阵标记 `not-run/blocked`。

## 2026-09-06 Q5 代码审阅补充

本轮只审阅和修复 `apps/workmesh-server` 后端，未修改 `apps/1Panel/frontend`，未执行全量测试，未修改生产配置。

已修复：

- 运行时/部署网站没有代理目标时的条件优先级错误。此前可能生成 `proxy_pass http://;`；现在只有存在真实代理目标时才生成反向代理块，空目标保持可加载的运行时入口。
- 新增、更新、删除 HTTP 网站域名时，同步真实站点目录中的 `nginx/site.conf` `server_name`，并在 SQLite 持久化失败时恢复文件和内存状态。主域名记录可更新端口/SSL 标志，但禁止删除。
- 创建网站时校验全部附加域名，防止空域名、非法标签、重复域名和控制字符进入配置或 SQLite。支持单级 DNS 通配符及 IPv4；HTTP-01 是否支持该域名仍由证书流程单独判定。
- 自定义 WAF 镜像的 OpenResty 操作兼容 `nginx`、`openresty` 及固定绝对路径；容器探测不再直接把 `ConfigValid` 写死为成功，而是执行真实 `-t` 检查。
- SQLite 存在时，WAF `site.json` 只保留兼容导出用途，重启恢复不再用 sidecar 覆盖 SQLite 中的真实 WAF 状态。

网站基础接口分组（统一 `/api/v2`）：

| 分组 | 主要路由 | 真实职责 |
| --- | --- | --- |
| 站点生命周期 | `GET/POST /websites`、`POST /websites/update`、`POST /websites/del`、`POST /websites/operate` | SQLite 网站记录、目录布局、启动/停止/重启 |
| 类型配置 | `POST /websites/config`、`POST /websites/config/update`、`POST /websites/stream` | 静态、运行时、部署、反代、PHP、TCP/UDP 的真实配置文件 |
| 域名与 HTTPS | `GET/POST /websites/domains/{id}`、`POST /websites/ssl`、`POST /websites/https` | 域名表、`server_name`、证书匹配、443 配置 |
| 网站设置 | `/websites/dir*`、`/websites/rewrite*`、`/websites/proxy*`、`/websites/redirect*`、`/websites/cors*`、`/websites/limit-conn*` | 站点目录、重写、代理、跳转、CORS、限流等托管 include |
| WAF | `/websites/waf/*`、`/xpack/waf/*` | WAF 状态、规则、访问列表和攻击统计兼容路由，持久化 SQLite |
| OpenResty | `/openresty`、`/openresty/file`、`/openresty/scope`、`/openresty/operate` | 受控配置读取、作用域更新、真实探测和容器/主机操作 |

未覆盖且必须由统一集成测试负责人复测：

- 修复后的 Go 编译、race、vet 和网站定向测试；
- 通过真实 API 创建静态、反向代理、运行时、部署、子站点、TCP、UDP，并验证域名访问和 `nginx -t`；
- 真实上游未启动的站点不能标记成功：当前 `aliases.example`、`types-deployment.example`、`types-proxy.example`、`types-runtime.example` 需先准备真实上游；
- `types-stream.example` 的真实 29001 TCP/UDP 监听与上游连通性；
- 公网 DNS、TCP 80 和 certbot 可用后再执行 Let's Encrypt HTTP-01 签发/续期；不能用模拟证书或固定响应替代。

## 2026-09-06 生产 SQLite / OpenResty 只读核验

核验时间：2026-09-06 16:11（Asia/Shanghai）。本次只读连接 `/opt/workmesh-server/data/workmesh.db`，未写入数据库、站点文件或 OpenResty 配置。

### SQLite 真实记录

- `websites`：1 条，`id=2`，主域名 `znmp.sopvip.com`，状态 `running`，协议 `HTTPS`，类型 `static`，`website_ssl_id=1`，站点目录 `/www/wwwroot/znmp.sopvip.com`，代理为空。
- `website_domains`：1 条，域名 `znmp.sopvip.com`，端口 `443`，`ssl=1`，无孤立网站域名记录。
- `website_settings`：14 条；HTTPS 设置为 `{"enabled":true,"websiteSSLId":1,"httpConfig":""}`，其余历史设置项当前内容长度为 14 字节的空内容对象，实际运行配置以站点目录中的配置文件为准。
- `website_ssls`：1 条，`id=1`，主域名 `znmp.sopvip.com`，提供商 `http`，状态 `ready`，自动续期开启，到期时间 `2026-12-03T13:13:43Z`，证书 4817 字节，私钥 241 字节；网站引用的证书 ID 存在，无孤立引用。
- `PRAGMA quick_check`、`integrity_check` 均为 `ok`，`foreign_key_check` 为 0 条。
- 当前数据库仍保留旧兼容列 `payload`，且 `websites.runtime_id` 物理类型为 `INTEGER`；这是存量兼容结构，不影响当前静态 HTTPS 记录，但后续正式迁移仍应保留兼容读取策略，不能直接重建或丢弃用户数据。

### OpenResty 当前运行配置

- 顶层配置实际加载：`include /www/wwwroot/*/nginx/site.conf;` 和 `include /www/wwwroot/*/nginx/stream.conf;`。
- 当前 `openresty -T` 成功，加载 19 个站点 `site.conf` 和 19 个站点 `stream.conf`；80/443 由 `workmesh-openresty-waf` 容器映射到宿主机，宿主机均有 TCP 监听。
- `znmp.sopvip.com` 当前同时加载 `listen 80` 和 `listen 443 ssl`，证书路径为 `/www/wwwroot/znmp.sopvip.com/ssl/fullchain.pem` 与 `/www/wwwroot/znmp.sopvip.com/ssl/privkey.pem`。本地 SNI 握手返回 Let's Encrypt 链，CN 为 `znmp.sopvip.com`，有效期为 2026-09-04 至 2026-12-03；HTTP 和 HTTPS 入口均返回真实站点页面。
- 当前实际加载范围内未发现 `proxy_pass http://;` 或 `proxy_pass 127.0.0.1:9;`。历史备份目录仍保留旧无效样例，但不在当前 `nginx.conf` include 范围内，不应把备份扫描结果误报为运行配置成功或失败。
- `/www/wwwroot/znmp.sopvip.com/app/.well-known/acme-challenge/` 目录存在；不存在的 challenge 返回 404，空目录请求返回 403，说明 HTTP-01 location 已加载，但本轮没有执行公网签发。

### 发现的待处理一致性项

- SQLite 只有 `znmp.sopvip.com` 1 个正式网站，但 OpenResty 当前还加载 18 个不在该 SQLite 记录中的测试/历史站点配置（包括 `types-*`、`example.*` 等）。本轮未删除这些文件，避免未经用户确认破坏现有 WAF/网站环境；应由网站管理 API 或发布清理任务逐一确认后清理。
- `aliases.example`、`types-deployment.example`、`types-proxy.example`、`types-runtime.example` 的配置包含真实上游地址，但对应上游当前未监听，HTTP 返回 502。这是上游未就绪，不是可用性通过，也不是用空响应替代。
- `/opt/workmesh-server/openresty-waf/conf/ssl/` 中存在一份与 `/www/wwwroot/znmp.sopvip.com/ssl/` 哈希不同的证书副本；当前 OpenResty `-T` 明确引用 `/www/wwwroot/znmp.sopvip.com/ssl/`，因此前者不是当前服务加载路径。后续证书续期应统一同步策略，避免两份证书漂移。

## 2026-09-06 HTTPS 测试契约修复

定向测试发现，`TestWebsiteAdvancedRoutes` 原先直接提交 `{"enabled":true}`，没有携带真实证书，和正式接口“启用 HTTPS 必须关联有效证书”的安全约束冲突。测试现已通过真实 API 导入临时 PEM 证书和私钥，再使用返回的 `websiteSSLId` 启用 HTTPS；证书 SAN 覆盖 `advanced.example` 和 `www.advanced.example`，不使用固定证书或模拟成功响应。

已验证的最小范围测试：

```text
GOWORK=off go test ./node/api -run 'TestWebsite(WAFRoutesCRUD|AdvancedRoutes)$' -count=1 -v
PASS
```

该结果只证明两个受影响 API 测试通过，不替代唯一集成测试负责人后续执行的完整 Go、race、vet 和发布门禁。
