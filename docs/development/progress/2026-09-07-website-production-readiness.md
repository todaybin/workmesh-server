<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 网站正式使用准备与验收清单（2026-09-07）

状态：`[>]` 真实生产入口和静态 HTTPS 站点已观察到可用；网站类型、运行环境上游、设置全量、WAF 攻防、ACME 续期和历史配置清理尚未完成，不能宣称网站模块整体上线验收通过。

本文件只记录 `/www/apps/workmesh-server` 的真实证据和可执行验收步骤。没有管理员会话、隔离域名或可清理上游时，项目保持 `blocked`/`not-run`，不使用固定 JSON、假上游或模拟成功响应。`/www/apps/1Panel` 只作为契约参考，本轮未修改；没有创建站点、重启服务、签发证书或修改 OpenResty 配置。

## 1. 当前真实环境快照

| 项目 | 真实观察 | 结论 |
| --- | --- | --- |
| WorkMesh 服务 | `systemd active`；`ExecStart=/opt/workmesh-server/bin/workmesh-server`；当前主进程正常 | `pass`（仅运行状态） |
| API 配置 | `/opt/workmesh-server/config/server.json`：`127.0.0.1:9999`、SQLite `./data`、任务并发 4、日志上限 8 MiB | `observed`；登录后业务未测 |
| SQLite | `/opt/workmesh-server/data/workmesh.db`，约 40 MiB；只读查询得到 `websites=1`、`website_domains=1`、`website_ssls=1`；`quick_check=ok`、`integrity_check=ok`、外键违规 0 | `pass`（完整性和存量摘要） |
| 正式网站 | `id=2`，`znmp.sopvip.com`，`type=static`，`status=running`，目录 `/www/wwwroot/znmp.sopvip.com`，HTTPS、SSL ID 1 | `observed`；CRUD 未在本轮执行 |
| 证书 | SQLite `id=1`，provider=`http`，status=`ready`，auto-renew=1，到期 `2026-12-03T13:13:43Z`；证书 SAN 为 `znmp.sopvip.com` | `pass`（当前加载证书） |
| OpenResty/WAF | 容器 `workmesh-openresty-waf` healthy，80/443 映射，`nofile=65536`；`nginx -t` 成功，`openresty -T` 成功 | `pass`（配置可加载） |
| DNS/访问 | `znmp.sopvip.com` 和任意测试前缀解析到 `61.184.12.165`；`http://znmp.sopvip.com/`、`https://znmp.sopvip.com/` 均真实返回 200，TLS 校验结果为 0 | `pass`（正式站点） |
| 通配符域名 | `website-readiness.cs.sopvip.com` 等前缀解析正常，但当前 `openresty -T` 没有对应 `server_name`；带 Host 的 200 可能命中默认站点 | `blocked`，不能视为该前缀已绑定 |
| certbot | `/usr/bin/certbot` 2.9.0；`certbot.timer=active` | `observed`；本轮未执行签发/续期 |

SQLite 只读查询还确认：WAF 关系表为 1 个站点、0 条规则、1 个全局配置、0 个访问控制条目。`website_settings` 的实际字段是 `config_type/content/updated_at`，不是旧文档中的 `scope/payload`；后续报告必须按真实 schema 查询。

## 2. 当前 OpenResty/WAF 文件和一致性风险

顶层配置实际加载：

```text
include /www/wwwroot/*/nginx/site.conf;
include /www/wwwroot/*/nginx/stream.conf;
```

当前 `openresty -T` 加载 19 组 `site.conf` 和 19 组 `stream.conf`。SQLite 仅登记 `znmp.sopvip.com`，但仍存在 `advanced.example`、`aliases.example`、`types-static.example`、`types-proxy.example`、`types-runtime.example`、`types-deployment.example`、`types-php.example`、`types-stream.example`、`types-subsite.example` 等历史/测试配置。它们不应通过“文件存在”直接认定为可上线网站；清理必须使用网站 API 或经批准的受控发布任务逐一确认归属后进行。

真实上游配置和当前结果：

| 类型 | 配置/端口 | 真实访问结果 | 阻塞原因 |
| --- | --- | --- | --- |
| 静态 | `types-static.example`，root `/www/wwwroot/types-static.example/app` | HTTP Host 访问 200 | 不是 SQLite 登记的正式站点；需 API 重建并绑定隔离域名 |
| 反向代理 | `types-proxy.example`，`proxy_pass http://127.0.0.1:28080` | 502；28080 无监听 | 必须提供真实上游并验证 WebSocket、头、超时 |
| 运行环境 | `types-runtime.example`，`proxy_pass http://127.0.0.1:28082` | 502；28082 无监听 | 必须创建真实运行时、端口和进程 |
| 一键部署 | `types-deployment.example`，`proxy_pass http://127.0.0.1:28081` | 502；28081 无监听 | 必须准备真实应用包/任务和上游 |
| PHP | `types-php.example`，`fastcgi_pass 127.0.0.1:28085` | 未执行 PHP 请求；28085 无监听 | 必须准备 PHP-FPM、扩展和脚本 |
| TCP/UDP | `types-stream.example` 的 `stream.conf` 只有历史注释，无 listener | 未执行 | 必须通过 stream API 写入真实端口和 upstream |
| 正式静态 HTTPS | `znmp.sopvip.com`，80/443、证书路径均存在 | HTTP/HTTPS 200 | 当前单站点证据，不覆盖其他类型 |

已扫描当前加载配置，没有发现活动配置中的 `proxy_pass http://;` 或 `proxy_pass 127.0.0.1:9;`。备份目录中的旧无效样例不属于当前 include 范围，不能混入运行配置结论。

## 3. 正式验收前置条件

1. 唯一集成测试负责人冻结后端源码、前端和部署制品，记录 Go 源码集合哈希、二进制 SHA-256、SQLite `schema_migrations` 和 OpenResty `nginx -T` 摘要。
2. 由资源负责人提供管理员 Token 或 Cookie（只通过受保护环境变量注入），并准备 CSRF 配对值；证据中只记录存在性，不记录秘密。
3. 为每种网站类型分配不同的 `*.cs.sopvip.com` 前缀、真实目录和清理责任人。不能复用 `znmp.sopvip.com`，不能手工覆盖 `/www/wwwroot/*` 下既有目录。
4. 反代/运行时/部署/PHP 测试必须先提供真实可监听上游、容器或 FPM；没有监听端口就保持 blocked，不能将 502 视为通过。
5. TCP/UDP 需要独立端口、真实 upstream、协议和算法；必须有连接客户端与清理窗口。
6. HTTPS HTTP-01 需要 DNS 指向测试前缀、公网 TCP 80 可达、certbot 账户和 Let's Encrypt 限额；测试证书或 staging 结果不能代替正式签发结论。
7. WAF 攻防测试要准备可回滚的站点规则和允许的测试请求，确认 block/observe 行为及访问日志，不得直接改生产全局策略。

## 4. 网站类型验收矩阵

所有请求统一使用 `/api/v2`。同一路径的不同 HTTP 方法和 `operate/type` 分支必须分别记录；一次列表请求不能代表创建、启停或删除通过。

### 4.1 静态网站（第一上线优先级）

前置：隔离前缀 DNS、空目录、真实 `index.html` 文件、可回滚 SQLite/目录备份。

路由序列：

```text
POST /api/v2/websites                         # type=static、primaryDomain、siteDir/domains
GET  /api/v2/websites/:id
GET  /api/v2/websites/domains/:websiteId
POST /api/v2/websites/config                  # operate=get/update，按 type/websiteId
POST /api/v2/websites/operate/:operate       # stop/start/restart
POST /api/v2/websites/log/search
POST /api/v2/websites/del
```

验收：创建响应为真实 `code/data`，SQLite 有网站和域名行，`site.conf` 的 `server_name/root/index` 正确；执行 `docker exec workmesh-openresty-waf nginx -t`；用 `curl --resolve <prefix>:80:<ip>` 和 `curl --resolve <prefix>:443:<ip> https://<prefix>/` 验证正文、404、日志和停止后的访问状态；删除后确认 SQLite、目录和 include 不留孤儿。当前 `znmp.sopvip.com` 的 HTTP/HTTPS 200 仅证明已有静态站点，不替代该 CRUD 序列。

### 4.2 反向代理网站

前置：真实上游监听（HTTP 和 WebSocket 各至少一条可观测路径）、独立前缀、上游响应标识。

路由序列：静态序列加：

```text
POST /api/v2/websites                    # type=proxy 或 proxy 配置
POST /api/v2/websites/proxy/config/:id
POST /api/v2/websites/proxies/update
POST /api/v2/websites/proxies/status
POST /api/v2/websites/proxies/delete
POST /api/v2/websites/config/update      # nginx 受控配置
```

验收：确认配置只含真实 `proxy_pass`，请求头 `Host/X-Real-IP/X-Forwarded-For` 正确，HTTP 状态和响应体来自上游；通过 WebSocket Upgrade、超时、上游停止后的 502 和恢复后 200；每次改动后 `nginx -t` 和域名访问均通过。当前 28080 无监听，`types-proxy.example` 的 502 是 blocked，不是实现通过。

### 4.3 运行环境网站（Go/Node/Python/Java/.NET/PHP）

前置：先由运行时负责人创建真实运行时并记录容器/进程/端口，再创建网站；运行时 ID 必须来自 SQLite，禁止填固定 ID。

路由序列：

```text
POST /api/v2/websites                    # type=runtime、runtimeID、runtimeType
GET  /api/v2/websites/:id
POST /api/v2/websites/operate/:operate
POST /api/v2/websites/config             # runtime/php/nginx 分支
POST /api/v2/websites/php/version        # PHP 站点额外检查
POST /api/v2/websites/log/search
```

六类环境分别执行创建、详情、停止、启动、重启、日志/任务、删除、重建和域名访问。PHP 还必须执行 FPM socket/端口、扩展安装、配置、Supervisor、慢日志和真实 PHP 页面。运行时进程停止时网站必须返回真实错误（通常 502），恢复后重新 200；不能返回缓存成功。

### 4.4 一键部署网站

前置：真实应用商店安装记录或模板产物、可执行任务、真实应用端口和隔离前缀。

路由序列：

```text
POST /api/v2/websites                    # type=deployment、appInstallId/templateOutputID/taskID
GET  /api/v2/websites/:id
POST /api/v2/tasks/search                # 记录部署任务
POST /api/v2/websites/operate/:operate
POST /api/v2/websites/del
```

验收：任务从 queued/running 到真实 completed，应用进程可访问；失败必须记录 error 并清理半成品；不能因为生成了 `site.conf` 就判定部署成功。当前 28081 无监听，历史 `types-deployment.example` 只能标记 blocked。

### 4.5 子网站/主次关系

前置：真实已登记主站点、独立子域名和父站点 ID；不得把任意 Host 200 当作子站点绑定。

路由序列：

```text
POST /api/v2/websites                    # parentWebsiteID + primaryDomain
GET  /api/v2/websites/:id
GET  /api/v2/websites/domains/:websiteId
POST /api/v2/websites/update/:id
POST /api/v2/websites/del
```

验收：SQLite 外键/父子关系正确，子站点拥有独立 `site.conf`、日志和目录，主站点删除/停止策略符合产品契约；父站点配置不能被子站点覆盖。当前仅有一个 `znmp.sopvip.com` 行，没有真实子站点证据，状态 `not-run`。

### 4.6 TCP/UDP Stream

前置：独立 TCP/UDP 端口、至少一个真实 upstream、允许的算法/权重/最大连接数和客户端。

路由序列：

```text
POST /api/v2/websites                    # type=stream、streamPorts、udp、servers
POST /api/v2/websites/stream
POST /api/v2/websites/config             # stream get/update/delete
POST /api/v2/websites/operate/:operate
POST /api/v2/websites/del
```

验收：检查 `stream.conf` 的真实 `listen`（TCP/UDP）和 upstream；`nginx -t`；使用 `nc`/专用 UDP 客户端验证双向数据和停止/恢复；检查端口冲突、错误处理、日志和 SQLite 状态。隔离黑盒已完成 TCP 返回、UDP 回显、配置回填和删除清理；端口冲突、停止/恢复和生产 stream 仍为 `not-run`。

## 5. HTTPS、WAF 和网站设置验收

### 5.1 HTTPS / Let's Encrypt HTTP-01

路由：`POST /api/v2/websites/ssl`、`POST /api/v2/websites/ssl/upload`、`POST /api/v2/websites/ssl/obtain`、`POST /api/v2/websites/ssl/renew`（或前端对应的 `ca`/`acme` 分支）、`POST /api/v2/websites/:id/https`、`GET /api/v2/websites/ssl/:id`。

具体命令：

```bash
dig +short <prefix>.cs.sopvip.com
curl -i --resolve <prefix>.cs.sopvip.com:80:<public-ip> \
  http://<prefix>.cs.sopvip.com/.well-known/acme-challenge/<real-token>
certbot certificates
openssl x509 -in /www/wwwroot/<prefix>.cs.sopvip.com/ssl/fullchain.pem \
  -noout -subject -issuer -dates -ext subjectAltName
docker exec workmesh-openresty-waf nginx -t
curl --resolve <prefix>.cs.sopvip.com:443:<public-ip> \
  --cacert <trusted-chain> https://<prefix>.cs.sopvip.com/
```

验收必须证明 challenge 文件由真实 API/任务创建并可从公网 80 读取，证书 SAN 覆盖所有申请域名，证书/私钥公钥匹配，SQLite `status=ready` 与站点文件一致，443 返回 200；续期后再次核对 SQLite、文件和 OpenResty reload。当前 `znmp.sopvip.com` 证书和 443 已通过，但本轮未执行新的公网签发、续期或失败回滚，因此新域名 ACME 仍 `blocked`。

### 5.2 WAF

路由：`GET/POST /api/v2/websites/waf/access-lists`、`GET/POST /api/v2/websites/waf/sites`、`GET /api/v2/websites/waf/sites/:id/rules`、`POST /api/v2/websites/waf/rules`、`POST /api/v2/websites/waf/rules/delete`、`GET/POST /api/v2/websites/waf/global`、`GET /api/v2/websites/waf/standard-rules`、`POST /api/v2/websites/waf/test`。

验收：SQLite 关系表发生预期变化且不生成新的 WAF sidecar；observe 模式记录不阻断，block 模式对真实匹配 URI 返回拒绝；白名单优先级、CIDR、规则删除、全局/站点开关和攻击日志均需验证；每次规则变更后 `nginx -t`、真实域名访问和回滚。当前数据库仅有 1 个空规则站点和 1 个全局配置，属于存量状态，不是完整攻防验收。

### 5.3 网站设置

按以下路由逐项执行读、写、撤销和真实访问，不得将统一 `config` 一次请求当成所有设置通过：

```text
/api/v2/websites/dir                 目录、权限、默认文档
/api/v2/websites/rewrite             伪静态、custom
/api/v2/websites/proxy*              代理、负载均衡、配置文件
/api/v2/websites/redirect            重定向
/api/v2/websites/auths               基础认证、路径认证
/api/v2/websites/cors                CORS
/api/v2/websites/realip              真实 IP
/api/v2/websites/leech               防盗链
/api/v2/websites/config               limit-conn、nginx、PHP、stream 多操作
/api/v2/websites/resource/:id         资源/流量统计
/api/v2/websites/log/search           访问/错误日志读取
```

每项验收证据包括：请求方法和 body 摘要、HTTP 状态、响应 `code/data`、SQLite 行变化、生成的 include 路径、`nginx -t`、域名实际结果、恢复原配置后的结果。密码只验证哈希/存在性字段，不能写入报告。

## 6. 放行门槛和当前阻塞

### 已有真实通过证据

- `workmesh-server.service` active，OpenResty/WAF healthy。
- `docker exec workmesh-openresty-waf nginx -t` 和 `openresty -T` 成功。
- SQLite `quick_check`、`integrity_check` 成功，外键违规为 0。
- `znmp.sopvip.com` HTTP/HTTPS 均返回真实 200，Let's Encrypt 证书 SAN、有效期和链可验证。
- 当前加载配置中没有空 `proxy_pass` 或 `127.0.0.1:9` 残留。

### 未达到正式上线放行的项目

- 没有管理员会话下的完整 `/api/v2/websites*` 黑盒 CRUD 证据；契约清单中的网站案例仍为 `not-run`。
- 只有一个 SQLite 静态站点；反代、运行时、部署、子站点、TCP/UDP、PHP-FPM 均无完整真实闭环。
- 18 组历史/测试站点配置未与 SQLite 对账清理，不能把它们当作可用站点。
- 28080/28081/28082/28085 当前无监听，导致反代、部署、运行时、PHP 测试 blocked。
- `*.cs.sopvip.com` 目前仅 DNS 解析；没有对应 `server_name` 的 API 绑定证据，Host 200 可能是默认站点。
- WAF 规则攻防、网站高级设置逐项访问、ACME 新域名签发/续期失败回滚尚未执行。
- 真实 WS 终端、运行环境生命周期、主次节点和升级迁移属于其他验收组，本文件不替代它们。

### 网站第一阶段放行条件

仅当“隔离静态站点 API CRUD + 域名访问 + 目录/日志 + HTTPS（已有证书或正式 HTTP-01）+ `nginx -t` + 删除清理”全部有独立证据，才可先放行静态网站；反向代理、运行时、部署、PHP、TCP/UDP 和 WAF 高级能力必须分别通过，不得由静态站点 200 推断。

## 7. 证据文件要求

唯一集成测试负责人应为每个案例保存独立 JSON/文本结果（仓库外或指定证据目录），至少包括：时间、制品哈希、SQLite 路径和迁移版本、域名前缀、HTTP 方法/路由、状态码、脱敏请求摘要、脱敏响应摘要、配置文件路径、`nginx -t` 结果、域名访问结果、日志/任务 ID、清理结果和失败原因。测试开始与结束计算源码集合哈希；哈希变化时整批结果作废并重新执行。
